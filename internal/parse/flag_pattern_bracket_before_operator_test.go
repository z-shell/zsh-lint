package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #283: a bracket expression before `#`, `*` or `,` in a flagged
// subscript pattern. mvdan/sh ends the raw pattern at the first `]`, and the
// byte after that premature close decides what happens next:
//
//   - `#` and `##` are valid Zsh expansion operators, so the parse succeeds
//     and the rest of the subscript becomes the operator's word. No error
//     exists, so no error-gated adapter can see the cut; resolveFlagPatternCuts
//     recognises it on the tree instead.
//   - `,` is bash's case-modification operator, reported as a LangError rather
//     than a ParseError, which #237's adapter did not accept.
//   - `*` is no operator at all; that error reached #237's adapter, but its
//     scanner stopped at the depth-0 `,` after the cut.
//
// Every row is `zsh -f -n` valid. The pattern must come back whole, with its
// original bytes at their original offsets and no `Expansion` operator the
// source does not have.
func TestParseFlagPatternBracketBeforeOperator(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
		col     uint
	}{
		// The rows that parse silently with the wrong tree.
		{"bracket before `##`", "print ${m[(r)a[^:]##]}\n", "r", "a[^:]##", 14},
		{"leading bracket before `##` and a range", "print ${m[(r)[^:]##,a]}\n", "r", "[^:]##", 14},
		{"bracket before `#` and a range", "print ${m[(r)a[^:]#,b]}\n", "r", "a[^:]#", 14},
		{"bracket before `##` in an assignment", "m[(r)a[^:]##]=1\n", "r", "a[^:]##", 6},
		{"bracket before `##` with an operator after the subscript", "print ${m[(r)a[^:]##]:-none}\n", "r", "a[^:]##", 14},
		{"nested expansion before the bracket", "print ${m[(r)${x}[^:]##]}\n", "r", "${x}[^:]##", 14},
		// The rows that raise a language error at the `,`.
		{"bracket before a range comma", "print ${m[(r)[^:],a]}\n", "r", "[^:]", 14},
		{"index flag, bracket before a range comma", "print ${m[(i)[a],3]}\n", "i", "[a]", 14},
		{"bracket before a range comma in an assignment", "m[(r)[^:],a]=1\n", "r", "[^:]", 6},
		{"bracket before a range comma in an append", "m[(r)[^:],a]+=1\n", "r", "[^:]", 6},
		// The row that raises the #237 error but stopped at the `,`.
		{"bracket before `*` and a range", "print ${m[(r)[^:]*,a]}\n", "r", "[^:]*", 14},
		{"bracket before `*` and a range in an assignment", "m[(r)[^:]*,a]=1\n", "r", "[^:]*", 6},
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
			// The pattern is compared as source so a nested expansion,
			// which stays its own node, round-trips through its offsets.
			if got := wordSource(test.src, flagged.X); got != test.pattern {
				t.Errorf("pattern = %q, want %q", got, test.pattern)
			}
			start := int(flagged.X.Pos().Offset())
			if start+1 != int(test.col) {
				t.Errorf("pattern column = %d, want %d", start+1, test.col)
			}
			if got := test.src[start : start+len(test.pattern)]; got != test.pattern {
				t.Errorf("source at pattern offsets = %q, want %q", got, test.pattern)
			}
		})
	}
}

// The silent rows' defect is an `Expansion` operator the source does not have:
// `${m[(r)a[^:]##]}` parsed as the pattern `a[^:` plus a `##` operator whose
// word was `]`. Asserting the pattern alone would not catch a repair that left
// the phantom operator behind, so the operator's absence is pinned separately,
// together with the expansion's own extent.
func TestFlagPatternCutLeavesNoPhantomOperator(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want syntax.ParExpOperator // 0 when the subscript carries no operator
		word string
	}{
		{"bracket before `##`", "print ${m[(r)a[^:]##]}\n", 0, ""},
		{"bracket before `#`", "print ${m[(r)a[^:]#]}\n", 0, ""},
		{"a real operator after the subscript is kept", "print ${m[(r)a[^:]##]:-none}\n", syntax.DefaultUnsetOrNull, "none"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var expansions []*syntax.ParamExp
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if exp, ok := node.(*syntax.ParamExp); ok {
					expansions = append(expansions, exp)
				}
				return true
			})
			if len(expansions) != 1 {
				t.Fatalf("ParamExp nodes = %d, want 1", len(expansions))
			}
			exp := expansions[0]
			switch {
			case test.want == 0:
				if exp.Exp != nil {
					t.Errorf("Exp = %v, want none", exp.Exp)
				}
			case exp.Exp == nil:
				t.Fatalf("Exp = nil, want operator %v", test.want)
			default:
				if exp.Exp.Op != test.want {
					t.Errorf("Exp.Op = %v, want %v", exp.Exp.Op, test.want)
				}
				if got := wordLiteral(t, exp.Exp.Word); got != test.word {
					t.Errorf("Exp.Word = %q, want %q", got, test.word)
				}
			}
			// The expansion must end at the real `}`, not at the byte the
			// cut reading stopped on.
			if got := int(exp.End().Offset()); got != len(test.src)-1 {
				t.Errorf("expansion end = %d, want %d", got, len(test.src)-1)
			}
		})
	}
}

func wordLiteral(t *testing.T, word *syntax.Word) string {
	t.Helper()
	if word == nil || len(word.Parts) != 1 {
		t.Fatalf("word = %v, want one part", word)
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		t.Fatalf("word part = %T, want *syntax.Lit", word.Parts[0])
	}
	return lit.Value
}

// A cut whose pattern is repaired must not disturb the range expression after
// it: the index stays a `,` comparison whose left operand is the flagged
// pattern and whose right operand is the endpoint, the shape #277 fixed.
func TestFlagPatternCutKeepsRangeEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		pattern  string
		endpoint string
	}{
		{"silent cut", "print ${m[(r)[^:]##,a]}\n", "[^:]##", "a"},
		{"language error", "print ${m[(r)[^:],a]}\n", "[^:]", "a"},
		{"operator error", "print ${m[(r)[^:]*,a]}\n", "[^:]*", "a"},
		{"numeric endpoint", "print ${m[(i)[a],3]}\n", "[a]", "3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			indexes := subscriptIndexes(file.AST())
			if len(indexes) != 1 {
				t.Fatalf("subscripts = %d, want 1", len(indexes))
			}
			binary, ok := indexes[0].(*syntax.BinaryArithm)
			if !ok || binary.Op != syntax.Comma {
				t.Fatalf("Index = %T %v, want *syntax.BinaryArithm with Op `,`", indexes[0], indexes[0])
			}
			flagged, ok := binary.X.(*syntax.FlagsArithm)
			if !ok {
				t.Fatalf("X = %T, want *syntax.FlagsArithm", binary.X)
			}
			if got := wordSource(test.src, flagged.X); got != test.pattern {
				t.Errorf("pattern = %q, want %q", got, test.pattern)
			}
			if got := wordSource(test.src, binary.Y); got != test.endpoint {
				t.Errorf("endpoint = %q, want %q", got, test.endpoint)
			}
		})
	}
}

// Two cut sites in one file are repaired in the same pass, and a site inside
// anonymous-function invocation words, which are typed metadata rather than
// tree nodes, is repaired on its own island.
func TestFlagPatternCutMultipleSites(t *testing.T) {
	t.Run("two sites on one line", func(t *testing.T) {
		const src = "print ${m[(r)a[^:]##]} ${n[(r)b[x]#,z]}\n"
		file, err := Parse(strings.NewReader(src), "two-sites.zsh")
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		var patterns []string
		syntax.Walk(file.AST(), func(node syntax.Node) bool {
			if flagged, ok := node.(*syntax.FlagsArithm); ok {
				patterns = append(patterns, wordSource(src, flagged.X))
			}
			return true
		})
		want := []string{"a[^:]##", "b[x]#"}
		if len(patterns) != len(want) {
			t.Fatalf("patterns = %q, want %q", patterns, want)
		}
		for i, got := range patterns {
			if got != want[i] {
				t.Errorf("pattern %d = %q, want %q", i, got, want[i])
			}
		}
	})

	t.Run("anonymous function invocation word", func(t *testing.T) {
		const src = "() { print $1 } ${m[(r)a[^:]##]}\n"
		file, err := Parse(strings.NewReader(src), "anonymous.zsh")
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		invocations := file.AnonymousInvocations()
		if len(invocations) != 1 || len(invocations[0].Words) != 1 {
			t.Fatalf("AnonymousInvocations = %v, want one invocation with one word", invocations)
		}
		var patterns []string
		syntax.Walk(invocations[0].Words[0], func(node syntax.Node) bool {
			if flagged, ok := node.(*syntax.FlagsArithm); ok {
				patterns = append(patterns, wordSource(src, flagged.X))
			}
			return true
		})
		if len(patterns) != 1 || patterns[0] != "a[^:]##" {
			t.Errorf("invocation word patterns = %q, want [\"a[^:]##\"]", patterns)
		}
	})

	t.Run("second subscript", func(t *testing.T) {
		const src = "print ${m[a][(r)b[x]##]}\n"
		file, err := Parse(strings.NewReader(src), "second.zsh")
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		flagged := soleFlagsArithm(t, file)
		if got := wordSource(src, flagged.X); got != "b[x]##" {
			t.Errorf("pattern = %q, want %q", got, "b[x]##")
		}
	})
}

// The repair must not widen what the front end accepts. Each row is either
// invalid Zsh, or valid Zsh whose extent the scanner declines to decide, and
// must keep the verdict it has on main.
func TestFlagPatternCutKeepsUnchangedVerdicts(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantError bool
	}{
		// `zsh -f -n` rejects this one: `invalid subscript`.
		{"unbalanced bracket in the pattern", "print ${m[(r)a[b]}\n", false},
		// Valid Zsh the scanner declines: it cannot bound a command
		// substitution, which is #237's documented limit.
		{"command substitution in the pattern", "print ${m[(r)$(echo [x])##]}\n", true},
		{"backtick in the pattern", "print ${m[(r)`echo [x]`##]}\n", true},
		// No bracket expression, so no cut: the `##` is a real operator.
		{"plain pattern with an operator", "print ${m[(r)a]##}\n", false},
		// A flagged pattern with no bracket at all is untouched.
		{"plain flagged pattern", "print ${m[(r)abc]}\n", false},
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

// `print ${m[(r)a[b]}` is the one row above that native Zsh rejects. The
// pattern scanner runs off the end looking for the `]` that balances the inner
// `[`, so it reports false and the parser's own (cut) reading stands. Nothing
// new is accepted, which is what matters; the tree is still the cut one, and
// this pins that so a later widening of the scanner has a test to flip.
func TestFlagPatternCutLeavesUnbalancedBracketAlone(t *testing.T) {
	const src = "print ${m[(r)a[b]}\n"
	file, err := Parse(strings.NewReader(src), "unbalanced.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	flagged := soleFlagsArithm(t, file)
	if got := wordSource(src, flagged.X); got != "a[b" {
		t.Errorf("pattern = %q, want %q (the cut reading native Zsh rejects)", got, "a[b")
	}
	t.Log("known limit: an unbalanced bracket keeps the parser's cut reading")
}

// The silent cut is repaired after the chain has run, not by an adapter in it,
// so the chain's own composition matrix does not cover it. This is that
// coverage: the cut must be repaired with any other adapter feature present in
// the same file, in either order, and that feature must still parse.
//
// The rows are taken from adapterSnippets, so an adapter added to the chain
// extends this too.
func TestFlagPatternCutComposesWithEveryAdapter(t *testing.T) {
	const cut = "print ${m[(r)a[^:]##]}"
	for name, snippet := range adapterSnippets {
		t.Run(name, func(t *testing.T) {
			for _, order := range []struct {
				label string
				src   string
			}{
				{"cut first", "#!/usr/bin/env zsh\n" + cut + "\n" + snippet.source + "\n"},
				{"cut second", "#!/usr/bin/env zsh\n" + snippet.source + "\n" + cut + "\n"},
			} {
				file, err := Parse(strings.NewReader(order.src), "compose.zsh")
				if err != nil {
					t.Fatalf("%s: composition must parse:\n%s\nerror: %v", order.label, order.src, err)
				}
				// The pattern must be whole: a composition that parses
				// but leaves the cut in place would pass on error alone.
				var repaired bool
				syntax.Walk(file.AST(), func(node syntax.Node) bool {
					if flagged, ok := node.(*syntax.FlagsArithm); ok {
						if wordSource(order.src, flagged.X) == "a[^:]##" {
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

// The guards below live in helpers the repaired tree cannot distinguish: a
// mutation of each one leaves every end-to-end row above passing, because
// another check downstream happens to reject the same input. They are tested
// at their own layer for the same reason TestFlagGroupOpener is, and each row
// was chosen by probing which inputs actually reach the guard.
//
// scanFlagPatternBrackets decides where a flagged pattern ends. It returns the
// mask, that end offset, and whether it could decide at all.
func TestScanFlagPatternBrackets(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantOK    bool
		wantClose int
		wantEdits int
	}{
		// The pattern ends at the `]` that closes the subscript, or at the
		// `,` that separates it from a range's second endpoint.
		{"bracket expression before the close", "print ${m[(r)a[^:]##]}", true, 20, 2},
		{"bracket expression before a range comma", "print ${m[(r)a[^:]##,b]}", true, 20, 2},
		// A `}` at depth 0 means the subscript was never closed. Zsh reads
		// the `]` by bracket nesting, so a `}` reached first is a shape this
		// scanner cannot bound, whatever it has masked so far.
		{"close brace at depth 0 with edits", "print ${m[(r)a[^:]##}", false, 0, 0},
		{"close brace at depth 0, bracket only", "print ${m[(r)[a]}", false, 0, 0},
		{"close brace at depth 0 without edits", "print ${m[(r)abc}", false, 0, 0},
		// A pattern with no bracket expression has no cut to repair; the
		// parser already read it correctly, and masking nothing would make
		// the retry a pointless reparse of identical bytes.
		{"no bracket, closes normally", "print ${m[(r)abc]}", false, 0, 0},
		{"no bracket, range comma", "print ${m[(r)abc,d]}", false, 0, 0},
		// An unbalanced `[` runs off the end: the scanner declines rather
		// than guess an extent, which is #237's documented limit.
		{"unbalanced bracket", "print ${m[(r)a[b]}", false, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Every source above opens its flagged pattern at the same
			// offset, just past `${m[(r)`.
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

// holdsFlagPattern verifies the masked retry before its tree is accepted. Both
// bounds matter: a literal that starts at the pattern but ends early is the cut
// reading the retry was supposed to fix, and one that ends at the right byte
// but starts elsewhere is a different node entirely.
func TestHoldsFlagPattern(t *testing.T) {
	// The mask the adapter builds for `print ${m[(r)a[^:]##]}`: the pattern
	// spans bytes 13 to 20.
	tree, err := parseWithAdapters([]byte("print ${m[(r)a_^:_##]}\n"), "holds.zsh")
	if err != nil {
		t.Fatalf("masked retry must parse: %v", err)
	}
	tests := []struct {
		name       string
		start, end int
		want       bool
	}{
		{"exact span", 13, 20, true},
		{"end one byte short", 13, 19, false},
		{"start one byte late", 14, 20, false},
		{"unrelated span", 0, 5, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := holdsFlagPattern(tree, test.start, test.end); got != test.want {
				t.Errorf("holdsFlagPattern(_, %d, %d) = %v, want %v",
					test.start, test.end, got, test.want)
			}
		})
	}
}

// flagPatternCutEdits is the resolver's recognizer: it must find a mask for a
// pattern the parser cut, and none for one already whole. Without the second
// half the resolver would reparse every flagged subscript in every file, which
// is cost, not correctness, and so is invisible to an output comparison.
func TestFlagPatternCutEdits(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"cut pattern", "print ${m[(r)a[^:]##]}\n", 2},
		{"cut pattern before a range comma", "print ${m[(r)a[^:]#,b]}\n", 2},
		// Already whole, by the base parser or by the error-gated adapter.
		{"no bracket", "print ${m[(r)abc]}\n", 0},
		{"no bracket, range comma", "print ${m[(r)abc,d]}\n", 0},
		{"bracket already whole", "print ${m[(r)[^:]]}\n", 0},
		{"after-comma shape", "print ${m[(r)a,b]}\n", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, err := parseWithAdapters([]byte(test.src), "edits.zsh")
			if err != nil {
				t.Fatalf("chain must parse %q: %v", test.src, err)
			}
			if got := len(flagPatternCutEdits([]byte(test.src), tree)); got != test.want {
				t.Errorf("edits = %d, want %d", got, test.want)
			}
		})
	}
}

// The LangError gate must stay narrow. mvdan/sh reports every bash-only
// expansion operator with the same feature text, so the gate cannot be the
// text alone; what makes it specific is the structure behind it. These rows
// carry the same text from an unrelated operator and must not reach the
// flagged-subscript retry.
func TestFlagPatternLanguageErrorGateStaysNarrow(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"case modification", "print ${x^^}\n"},
		{"case modification, lower", "print ${x,,}\n"},
		{"parameter transformation", "print ${x@Q}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Each is invalid Zsh and must stay rejected: an adapter that
			// accepted the feature text alone would mask bytes here.
			if _, err := Parse(strings.NewReader(test.src), test.name+".zsh"); err == nil {
				t.Errorf("Parse(%q) unexpectedly succeeded", test.src)
			}
		})
	}
}
