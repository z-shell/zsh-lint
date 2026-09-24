package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #371: a flagged subscript pattern holding a nested parameter expansion
// that itself carries a subscript, `${m[(r)${Z[a]}]}`.
//
// mvdan/sh reads a flagged subscript's pattern as one raw literal and ends it
// at the first `]`, whatever encloses that byte. The nested expansion is not
// its own node inside that literal, so its `]` cut the pattern and the outer
// `}` then closed nothing: “ `}` can only be used to close a block “. A
// nested `,` cut it the same way, splitting the index into a range the source
// does not have.
//
// scanFlagPatternBrackets already walked the pattern by bracket nesting; it
// skipped a nested expansion whole and left its bytes unmasked. Masking them
// like any other byte inside the pattern is what closes the gap, and
// restorePatternEdits puts them back from the same literal, so the masking is
// invisible to callers.
//
// Every row is `zsh -f -n` valid and runs under `zsh -f`.
func TestParseFlagPatternSubscriptedExpansion(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
		col     uint
	}{
		{"reverse flag", "print ${m[(r)${Z[a]}]}\n", "r", "${Z[a]}", 14},
		{"reverse exact flag", "print ${m[(re)${Z[a]}]}\n", "re", "${Z[a]}", 15},
		{"index flag", "print ${m[(i)${Z[a]}]}\n", "i", "${Z[a]}", 14},
		{"text after the expansion", "print ${m[(r)${Z[a]}x]}\n", "r", "${Z[a]}x", 14},
		{"quoting flag on the nested expansion", "print ${m[(r)${(q)Z[a]}]}\n", "r", "${(q)Z[a]}", 14},
		{"expansion inside the nested subscript", "print ${m[(r)${Z[${k}]}]}\n", "r", "${Z[${k}]}", 14},
		{"the zi.zsh idiom", "[[ -z ${manpath[(re)${ZI[MAN_DIR]}]} ]]\n", "re", "${ZI[MAN_DIR]}", 21},
		{"assignment form", "x=${m[(r)${Z[a]}]}\n", "r", "${Z[a]}", 10},
		{"inside double quotes", "print \"${m[(r)${Z[a]}]}\"\n", "r", "${Z[a]}", 15},
		{"operator after the subscript", "print ${m[(r)${Z[a]}]:-none}\n", "r", "${Z[a]}", 14},
		{"bracket expression after the expansion", "print ${m[(r)${Z[a]}[^:]##]}\n", "r", "${Z[a]}[^:]##", 14},
		{"two nested expansions", "print ${m[(r)${Z[a]}${Y[b]}]}\n", "r", "${Z[a]}${Y[b]}", 14},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			flagged := soleFlagsArithm(t, file)
			if flagged.Flags == nil || flagged.Flags.Value != test.flags {
				t.Errorf("Flags = %v, want %q", flagged.Flags, test.flags)
			}
			if got := wordSource(test.src, flagged.X); got != test.pattern {
				t.Errorf("pattern = %q, want %q", got, test.pattern)
			}
			start := int(flagged.X.Pos().Offset())
			if start+1 != int(test.col) {
				t.Errorf("pattern column = %d, want %d", start+1, test.col)
			}
			// The masked bytes must be the original ones again, at the
			// original offsets: the whole point of a byte-preserving mask.
			if got := test.src[start : start+len(test.pattern)]; got != test.pattern {
				t.Errorf("source at pattern offsets = %q, want %q", got, test.pattern)
			}
		})
	}
}

// The nested expansion's bytes must come back as the literal's own text, not
// merely span the right offsets. A repair that left the `_` mask in the tree
// would satisfy an offset comparison against the source and still hand every
// rule a pattern the script does not have.
func TestFlagPatternSubscriptedExpansionRestoresLiteral(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"subscript", "print ${m[(r)${Z[a]}]}\n", "${Z[a]}"},
		{"subscript with a range", "print ${m[(r)${Z[1,2]}]}\n", "${Z[1,2]}"},
		{"nested twice", "print ${m[(r)${Z[${k[j]}]}]}\n", "${Z[${k[j]}]}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			flagged := soleFlagsArithm(t, file)
			word, ok := flagged.X.(*syntax.Word)
			if !ok || len(word.Parts) != 1 {
				t.Fatalf("pattern = %T, want a one-part *syntax.Word", flagged.X)
			}
			lit, ok := word.Parts[0].(*syntax.Lit)
			if !ok {
				t.Fatalf("pattern part = %T, want *syntax.Lit", word.Parts[0])
			}
			if lit.Value != test.want {
				t.Errorf("literal = %q, want %q", lit.Value, test.want)
			}
		})
	}
}

// A `,` inside the nested expansion's own subscript is not the range separator
// of the outer one. Before the fix `${m[(r)${Z[1,2]}]}` parsed into a
// `BinaryArithm` `,` whose left operand was the cut pattern `${Z[1` — a range
// the source does not have — so the index shape is pinned separately from the
// pattern text, which an index-blind assertion would not catch.
func TestFlagPatternSubscriptedExpansionKeepsIndexShape(t *testing.T) {
	t.Run("nested comma is not a range", func(t *testing.T) {
		const src = "print ${m[(r)${Z[1,2]}]}\n"
		file, err := Parse(strings.NewReader(src), "nested-comma.zsh")
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		indexes := subscriptIndexes(file.AST())
		if len(indexes) != 1 {
			t.Fatalf("subscripts = %d, want 1", len(indexes))
		}
		if binary, ok := indexes[0].(*syntax.BinaryArithm); ok && binary.Op == syntax.Comma {
			t.Fatalf("Index = range %q,%q, want the flagged pattern alone",
				wordSource(src, binary.X), wordSource(src, binary.Y))
		}
		if _, ok := indexes[0].(*syntax.FlagsArithm); !ok {
			t.Fatalf("Index = %T, want *syntax.FlagsArithm", indexes[0])
		}
	})

	t.Run("a real range keeps both endpoints", func(t *testing.T) {
		const src = "print ${m[(r)${Z[a]},3]}\n"
		file, err := Parse(strings.NewReader(src), "real-range.zsh")
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		indexes := subscriptIndexes(file.AST())
		if len(indexes) != 1 {
			t.Fatalf("subscripts = %d, want 1", len(indexes))
		}
		binary, ok := indexes[0].(*syntax.BinaryArithm)
		if !ok || binary.Op != syntax.Comma {
			t.Fatalf("Index = %T, want *syntax.BinaryArithm with Op `,`", indexes[0])
		}
		flagged, ok := binary.X.(*syntax.FlagsArithm)
		if !ok {
			t.Fatalf("X = %T, want *syntax.FlagsArithm", binary.X)
		}
		if got := wordSource(src, flagged.X); got != "${Z[a]}" {
			t.Errorf("pattern = %q, want %q", got, "${Z[a]}")
		}
		if got := wordSource(src, binary.Y); got != "3" {
			t.Errorf("endpoint = %q, want %q", got, "3")
		}
	})

	t.Run("a range endpoint's own flagged pattern", func(t *testing.T) {
		const src = "print ${m[1,(i)${Z[a]}]}\n"
		file, err := Parse(strings.NewReader(src), "endpoint.zsh")
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		flagged := soleFlagsArithm(t, file)
		if got := wordSource(src, flagged.X); got != "${Z[a]}" {
			t.Errorf("pattern = %q, want %q", got, "${Z[a]}")
		}
	})
}

// The mask must not widen what the front end accepts. Each row keeps the
// verdict it has on main: the scanner declines a shape it cannot bound, and a
// pattern with no nested expansion is untouched.
func TestFlagPatternSubscriptedExpansionKeepsUnchangedVerdicts(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantError bool
	}{
		// An unbalanced brace runs off the end: the scanner declines
		// rather than guess an extent.
		{"unbalanced nested brace", "print ${m[(r)${Z[a]]}\n", true},
		// A command substitution inside the pattern is #237's documented
		// limit and stays refused.
		{"command substitution in the pattern", "print ${m[(r)$(echo [x])##]}\n", true},
		// No nested expansion, so nothing new is masked.
		{"plain flagged pattern", "print ${m[(r)abc]}\n", false},
		{"bracket expression only", "print ${m[(r)a[^:]##]}\n", false},
		// A nested expansion with no subscript already parsed on main.
		{"nested expansion without a subscript", "print ${m[(r)${Z}]}\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if test.wantError && err == nil {
				t.Errorf("Parse(%q) unexpectedly succeeded", test.src)
			}
			if !test.wantError && err != nil {
				t.Errorf("Parse(%q) error: %v", test.src, err)
			}
		})
	}
}

// Unbalanced brackets inside the nested expansion are the reason
// maskNestedExpansion counts them. Masking a byte hides it from the parser, so
// without that count an unbalanced `]` was masked into invisibility and the
// outer subscript closed over source native Zsh rejects as `bad substitution`.
// Each of these was a false accept found by probing the fix, not by the rows
// above.
//
// These are native-invalid sources, so they live in `testdata/invalid-371-*.txt`
// and are never executed, per docs/project/parser-gap-workflow.md. The position
// is asserted so a future change cannot keep rejecting them for an unrelated
// reason and still pass.
func TestFlagPatternSubscriptedExpansionRejectsUnbalancedBrackets(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		col     uint
	}{
		{"testdata/invalid-371-stray-close-bracket.txt", "`}` can only be used to close a block", 20},
		{"testdata/invalid-371-extra-close-bracket.txt", "not a valid parameter expansion operator: `]`", 20},
		{"testdata/invalid-371-extra-open-bracket.txt", "`}` can only be used to close a block", 23},
		{"testdata/invalid-371-close-before-open.txt", "`}` can only be used to close a block", 23},
		// A quoted `[` rebalancing an unquoted stray `]`: the reason the
		// stray-close refusal is on sight and a quote refuses outright.
		{"testdata/invalid-371-quoted-bracket-rebalance.txt", "not a valid parameter expansion operator: `]`", 20},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-371.zsh")
			if err == nil {
				t.Fatal("Parse() accepted a source native Zsh rejects")
			}
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error = %v (%T), want a syntax.ParseError", err, err)
			}
			if parseErr.Text != test.text {
				t.Errorf("error text = %q, want %q", parseErr.Text, test.text)
			}
			if parseErr.Pos.Line() != 1 || parseErr.Pos.Col() != test.col {
				t.Errorf("error at %d:%d, want 1:%d", parseErr.Pos.Line(), parseErr.Pos.Col(), test.col)
			}
		})
	}
}

// A quote inside the nested expansion refuses it, because a byte count and a
// quote are not composable: a quoted bracket still moves the count, so a
// quoted `[` can rebalance an unquoted stray `]` and make invalid source look
// balanced. That was a real false accept in an earlier revision of this
// change, found by adversarial review — `${m[(r)${Z[a]]"["}]}` is
// `bad substitution` natively and parsed clean.
//
// Tracking quotes properly means reproducing native Zsh's own rule for a quote
// inside a subscript, which is position-dependent and asymmetric between quote
// kinds: `x=${Z["a]b"]}` is valid while the same expansion in command position
// is not, and `${Z[${Y['a b']}]}` is valid while `${Z[${Y["a b"]}]}` is not.
// Refusing is the honest answer until something implements that rule.
//
// Every row here keeps the verdict it has on main rather than gaining one, so
// nothing regresses; this pins the limit so a later fix has a test to flip.
//
// The front end's wider divergence on that quoted family is separate and
// pre-existing: `${m[${Z["a b"]}]}` has no flagged pattern, never reaches this
// scanner, and is accepted on main although native Zsh rejects it (#374).
func TestFlagPatternSubscriptedExpansionQuotedBracketLimit(t *testing.T) {
	for _, src := range []string{
		"print ${m[(r)${Z[\"a]b\"]}]}\n",
		"print ${m[(r)${Z['a]b']}]}\n",
		"print ${m[(r)${Z[\"a\"]}]}\n",
		"print ${m[(r)${Z['a']}]}\n",
	} {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), "quoted-limit.zsh"); err == nil {
				t.Errorf("Parse(%q) succeeded; the known limit is that it is refused", src)
			}
		})
	}
	t.Log("known limit: a quoted bracket is counted as a bracket, so a quoted unbalanced one refuses")
}

// maskNestedExpansion is the changed helper, and the end-to-end rows above
// cannot tell its two jobs apart: a mutation that stopped reporting the mask
// but kept returning the right end offset would leave the pattern cut, which
// reads as the gap being back, while one that reported a byte outside the
// expansion would be caught downstream by restorePatternEdits' own accounting
// (`edit at byte N restored 0 times`) rather than by any assertion here.
// Both are pinned at the helper's own layer.
func TestMaskNestedExpansion(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		brace    int
		wantOK   bool
		wantEnd  int
		wantMask []int
	}{
		{"subscript", "${Z[a]}", 1, true, 7, []int{3, 5}},
		{"range inside the subscript", "${Z[1,2]}", 1, true, 9, []int{3, 5, 7}},
		{"no subscript", "${Z}", 1, true, 4, nil},
		{"nested expansion in the subscript", "${Z[${k[j]}]}", 1, true, 13, []int{3, 7, 9, 11}},
		// The extents the scanner declines to decide.
		{"unbalanced brace", "${Z[a]", 1, false, 0, nil},
		{"newline inside", "${Z[a]\n}", 1, false, 0, nil},
		// Unbalanced brackets: masking one would hide it from the parser,
		// so the whole expansion is refused and the parser error stands.
		// A `]` with nothing open refuses on sight, so no later `[` can
		// bring the count back to zero and make it look balanced.
		{"stray close bracket", "${Z]}", 1, false, 0, nil},
		{"extra close bracket", "${Z[a]]}", 1, false, 0, nil},
		{"extra open bracket", "${Z[[a]}", 1, false, 0, nil},
		{"close before open", "${Z][a]}", 1, false, 0, nil},
		// `zsh -n` accepts this one, but it is `bad substitution` at
		// runtime, so refusing costs nothing real and it keeps the
		// verdict it has on main.
		{"close then open, count balanced", "${Z]a[}", 1, false, 0, nil},
		// Quoting is not tracked, so a quote refuses the expansion: a
		// quoted bracket still moves a byte count, which is how a quoted
		// `[` could rebalance an unquoted stray `]`.
		{"double quote", "${Z[\"a\"]}", 1, false, 0, nil},
		{"single quote", "${Z['a']}", 1, false, 0, nil},
		{"quoted bracket rebalancing a stray one", "${Z[a]]\"[\"}", 1, false, 0, nil},
		// An escaped bracket is masked like the outer scan masks its own,
		// and does not move the count: the escape means it is not a
		// subscript delimiter.
		{"escaped close bracket", "${Z[a\\]b]}", 1, true, 10, []int{3, 6, 8}},
		{"escaped open bracket", "${Z[a\\[b]}", 1, true, 10, []int{3, 6, 8}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var masked []int
			end, ok := maskNestedExpansion([]byte(test.src), test.brace, func(offset int) {
				masked = append(masked, offset)
			})
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if end != test.wantEnd {
				t.Errorf("end = %d, want %d", end, test.wantEnd)
			}
			if len(masked) != len(test.wantMask) {
				t.Fatalf("mask = %v, want %v", masked, test.wantMask)
			}
			for i, offset := range masked {
				if offset != test.wantMask[i] {
					t.Errorf("mask[%d] = %d, want %d", i, offset, test.wantMask[i])
				}
				if b := test.src[offset]; b != '[' && b != ']' && b != ',' {
					t.Errorf("mask[%d] points at %q, want a bracket or a comma", i, b)
				}
			}
		})
	}
}

// A refused expansion must report no mask at all. The caller discards the
// edits on `!ok`, so a helper that reported bytes before running off the end
// would be invisible end-to-end; this pins that the two results agree.
func TestMaskNestedExpansionReportsNoMaskWhenRefused(t *testing.T) {
	for _, src := range []string{"${Z[a]", "${Z[a]\n}", "${Z[", "${Z[a]]}", "${Z]}"} {
		t.Run(src, func(t *testing.T) {
			calls := 0
			if _, ok := maskNestedExpansion([]byte(src), 1, func(int) { calls++ }); ok {
				t.Fatalf("maskNestedExpansion(%q) = ok, want refused", src)
			}
			if calls != 0 {
				t.Errorf("mask calls = %d, want 0 on a refused extent", calls)
			}
		})
	}
}

// scanFlagPatternBrackets is what the mask feeds. Its end offset must be the
// `]` that really closes the subscript, past the nested expansion, and its
// edit count must cover the nested bytes as well as the pattern's own.
func TestScanFlagPatternBracketsWithNestedExpansion(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantOK    bool
		wantClose int
		wantEdits int
	}{
		// `print ${m[(r)` is 13 bytes, so every pattern starts at 13.
		{"subscripted expansion", "print ${m[(r)${Z[a]}]}", true, 20, 2},
		{"range in the nested subscript", "print ${m[(r)${Z[1,2]}]}", true, 22, 3},
		{"expansion then a bracket expression", "print ${m[(r)${Z[a]}[^:]##]}", true, 26, 4},
		// No nested expansion: the pre-existing behavior is unchanged.
		{"bracket expression only", "print ${m[(r)a[^:]##]}", true, 20, 2},
		// A nested expansion with no bracket or comma masks nothing, so
		// the scanner has no cut to repair and declines as before.
		{"expansion without a subscript", "print ${m[(r)${Z}]}", false, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const patternStart = 13
			edits, close, ok := scanFlagPatternBrackets([]byte(test.src), patternStart)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if close != test.wantClose {
				t.Errorf("close = %d, want %d", close, test.wantClose)
			}
			if len(edits) != test.wantEdits {
				t.Errorf("edits = %d, want %d", len(edits), test.wantEdits)
			}
		})
	}
}

// The repair must survive any other adapter feature in the same file, in
// either order, since the masked retry is an ordinary parse of a whole file.
// The rows come from adapterSnippets, so an adapter added to the chain extends
// this automatically.
func TestFlagPatternSubscriptedExpansionComposesWithEveryAdapter(t *testing.T) {
	const gap = "print ${m[(r)${Z[a]}]}"
	for name, snippet := range adapterSnippets {
		t.Run(name, func(t *testing.T) {
			for _, order := range []struct {
				label string
				src   string
			}{
				{"gap first", "#!/usr/bin/env zsh\n" + gap + "\n" + snippet.source + "\n"},
				{"gap second", "#!/usr/bin/env zsh\n" + snippet.source + "\n" + gap + "\n"},
			} {
				file, err := Parse(strings.NewReader(order.src), "compose.zsh")
				if err != nil {
					t.Fatalf("%s: composition must parse:\n%s\nerror: %v", order.label, order.src, err)
				}
				// Parsing alone would pass with the pattern still cut,
				// so the pattern's text is what is asserted.
				var repaired bool
				syntax.Walk(file.AST(), func(node syntax.Node) bool {
					if flagged, ok := node.(*syntax.FlagsArithm); ok {
						if wordSource(order.src, flagged.X) == "${Z[a]}" {
							repaired = true
						}
					}
					return true
				})
				if !repaired {
					t.Errorf("%s: pattern not repaired in:\n%s", order.label, order.src)
				}
			}
		})
	}
}
