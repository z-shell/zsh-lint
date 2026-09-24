package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #384: a single-quoted key inside a nested parameter expansion, where
// the enclosing subscript carries a pattern flag: `${m[(i)${Z['ab']}]}`.
//
// maskNestedExpansion (#371) refused any quote outright, because a byte count
// and a quote are not composable: a quoted bracket still moves the count, so a
// quoted `[` could rebalance an unquoted stray `]`. That refusal is right for
// the double-quoted spelling, which native Zsh rejects here anyway, but it also
// cost every single-quoted key, which native Zsh accepts and runs.
//
// The relaxation is narrow: a single quote standing inside the nested
// subscript's own brackets and holding no delimiter byte is stepped over. Every
// other quoted shape keeps the verdict it has on main.
//
// Every row is `zsh -f -n` valid and runs clean under `zsh -f`.
func TestParseFlagPatternQuotedNestedKey(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
	}{
		{"index flag", "print ${m[(i)${Z['ab']}]}\n", "i", "${Z['ab']}"},
		{"reverse flag", "print ${m[(r)${Z['ab']}]}\n", "r", "${Z['ab']}"},
		{"space in the key", "print ${m[(i)${Z['a b']}]}\n", "i", "${Z['a b']}"},
		{"comma in the key", "print ${m[(i)${Z['a,b']}]}\n", "i", "${Z['a,b']}"},
		{"hash in the key", "print ${m[(i)${Z['a#b']}]}\n", "i", "${Z['a#b']}"},
		{"dollar in the key", "print ${m[(i)${Z['a$b']}]}\n", "i", "${Z['a$b']}"},
		{"empty key", "print ${m[(i)${Z['']}]}\n", "i", "${Z['']}"},
		{"quoted run beside plain text", "print ${m[(i)${Z[a'b'c]}]}\n", "i", "${Z[a'b'c]}"},
		{"two quoted runs", "print ${m[(i)${Z['a''b']}]}\n", "i", "${Z['a''b']}"},
		{"text after the expansion", "print ${m[(i)${Z['ab']}x]}\n", "i", "${Z['ab']}x"},
		{"quoted key one level down", "print ${m[(i)${Z[${Y['ab']}]}]}\n", "i", "${Z[${Y['ab']}]}"},
		{"beside an unquoted nested expansion", "print ${m[(i)${Z['ab']}${Z[a]}]}\n", "i", "${Z['ab']}${Z[a]}"},
		{"bracket expression after it", "print ${m[(i)${Z['ab']}[^:]##]}\n", "i", "${Z['ab']}[^:]##"},
		{"inside double quotes", "print \"${m[(i)${Z['ab']}]}\"\n", "i", "${Z['ab']}"},
		{"assignment form", "x=${m[(i)${Z['ab']}]}\n", "i", "${Z['ab']}"},
		{"range endpoint", "print ${m[1,(i)${Z['ab']}]}\n", "i", "${Z['ab']}"},
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
			// The mask is byte-preserving, so the source at the
			// pattern's own offsets must still read as the pattern.
			start := int(flagged.X.Pos().Offset())
			if got := test.src[start : start+len(test.pattern)]; got != test.pattern {
				t.Errorf("source at pattern offsets = %q, want %q", got, test.pattern)
			}
		})
	}
}

// The quoted bytes must come back as the literal's own text. A repair that left
// the `_` mask in the tree would satisfy an offset comparison against the
// source and still hand every rule a key the script does not have.
func TestFlagPatternQuotedNestedKeyRestoresLiteral(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"plain key", "print ${m[(i)${Z['ab']}]}\n", "${Z['ab']}"},
		{"comma in the key", "print ${m[(i)${Z['a,b']}]}\n", "${Z['a,b']}"},
		{"nested twice", "print ${m[(i)${Z[${Y['a,b']}]}]}\n", "${Z[${Y['a,b']}]}"},
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

// A `,` inside the quoted key is key text, not the outer subscript's range
// separator. mvdan/sh splits a subscript index at any `,` it sees in the raw
// literal, so without the mask `${m[(i)${Z['a,b']}]}` would parse into a range
// the source does not have — which an index-blind assertion would not catch.
func TestFlagPatternQuotedNestedKeyKeepsIndexShape(t *testing.T) {
	const src = "print ${m[(i)${Z['a,b']}]}\n"
	file, err := Parse(strings.NewReader(src), "quoted-comma.zsh")
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
}

// The relaxation must not widen what the front end accepts. Each row keeps the
// verdict it has on main: the double-quoted spelling, a quote outside the
// nested subscript's brackets, a delimiter inside the quotes, and the
// rebalancing attack the quote refusal was written for.
//
// The verdicts below were measured with `zsh -f -n` AND `zsh -f`, because a
// parse check alone is not a validity check: `${Z[a]'x'}` passes `zsh -f -n`
// and is `bad substitution` when run, so accepting it would be a false accept
// no parse-only oracle would show.
func TestFlagPatternQuotedNestedKeyKeepsUnchangedVerdicts(t *testing.T) {
	for _, src := range []string{
		// Native `bad substitution`: the asymmetry between quote kinds
		// is the whole reason only one of them is stepped over.
		"print ${m[(i)${Z[\"ab\"]}]}\n",
		"print ${m[(i)${Z[\"a b\"]}]}\n",
		// A quote outside the nested subscript's brackets is not a key:
		// `zsh -f -n` accepts this and running it is `bad substitution`.
		"print ${m[(i)${Z[a]'x'}]}\n",
		// A delimiter inside the quotes: native Zsh's verdict varies by
		// enclosing construct, so the extent stays undecidable here.
		"print ${m[(i)${Z['a[b']}]}\n",
		"print ${m[(i)${Z['a]b']}]}\n",
		"print ${m[(i)${Z['a(b']}]}\n",
		"print ${m[(i)${Z['a)b']}]}\n",
		"print ${m[(i)${Z['a{b']}]}\n",
		"print ${m[(i)${Z['a}b']}]}\n",
		// The #371 rebalancing attack, spelled with a single quote: a
		// quoted `[` must not cancel an unquoted stray `]`.
		"print ${m[(r)${Z[a]]'['}]}\n",
		// An unterminated quote has no extent at all.
		"print ${m[(i)${Z['ab]}]}\n",
		// A stray quote after a repairable nested expansion. This row is
		// what makes flagPatternQuotedCommaBeforeError's masked-comma
		// requirement load-bearing rather than decorative: the pattern
		// holds no quoted key, so the unclosed-quote error must not be
		// treated as this family's comma cut. Measured — with that
		// requirement disabled the row is accepted, and it is
		// `bad substitution` natively.
		"print ${m[(i)${Z[a]}']}\n",
	} {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), "unchanged.zsh"); err == nil {
				t.Errorf("Parse(%q) succeeded; it is refused on main and must stay refused", src)
			}
		})
	}
}

// The rebalancing attack needs its own fixture with a position assertion, so a
// later change cannot keep rejecting it for an unrelated reason and still pass.
// Native Zsh reports `bad substitution`; this source is never executed, per
// docs/project/parser-gap-workflow.md.
func TestFlagPatternQuotedNestedKeyRejectsQuotedRebalance(t *testing.T) {
	const fixture = "testdata/invalid-384-single-quoted-bracket-rebalance.txt"
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-384.zsh")
	if err == nil {
		t.Fatal("Parse() accepted a source native Zsh rejects")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v (%T), want a syntax.ParseError", err, err)
	}
	const want = "not a valid parameter expansion operator: `]`"
	if parseErr.Text != want {
		t.Errorf("error text = %q, want %q", parseErr.Text, want)
	}
	if parseErr.Pos.Line() != 1 || parseErr.Pos.Col() != 20 {
		t.Errorf("error at %d:%d, want 1:20", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}

// maskSingleQuotedKey is the new helper, and the end-to-end rows cannot tell
// its two jobs apart: a mutation that returned the right end offset without
// reporting the `,` mask would leave the index split into a range, and one that
// reported a byte outside the quotes would be caught downstream by
// restorePatternEdits' own accounting rather than by any assertion above.
func TestMaskSingleQuotedKey(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		quote    int
		wantOK   bool
		wantEnd  int
		wantMask []int
	}{
		// `${Z[` is 4 bytes, so a key's opening quote sits at 4.
		{"plain key", "${Z['ab']}", 4, true, 7, nil},
		{"empty key", "${Z['']}", 4, true, 5, nil},
		{"comma is masked", "${Z['a,b']}", 4, true, 8, []int{6}},
		{"two commas", "${Z['a,b,c']}", 4, true, 10, []int{6, 8}},
		{"space is not masked", "${Z['a b']}", 4, true, 8, nil},
		// A delimiter refuses: its native reading is not the literal one
		// this step-over implies.
		{"open bracket", "${Z['a[b']}", 4, false, 0, nil},
		{"close bracket", "${Z['a]b']}", 4, false, 0, nil},
		{"open brace", "${Z['a{b']}", 4, false, 0, nil},
		{"close brace", "${Z['a}b']}", 4, false, 0, nil},
		{"open paren", "${Z['a(b']}", 4, false, 0, nil},
		{"close paren", "${Z['a)b']}", 4, false, 0, nil},
		// No extent to decide.
		{"unterminated", "${Z['ab]}", 4, false, 0, nil},
		{"newline inside", "${Z['a\nb']}", 4, false, 0, nil},
		// A backslash is an ordinary byte inside single quotes, which is
		// exactly how Zsh reads it, so it neither escapes nor refuses.
		{"backslash is literal", "${Z['a\\b']}", 4, true, 8, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var masked []int
			end, ok := maskSingleQuotedKey([]byte(test.src), test.quote, &masked)
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
				if b := test.src[offset]; b != ',' {
					t.Errorf("mask[%d] points at %q, want a comma", i, b)
				}
			}
			if test.wantOK && test.src[end] != '\'' {
				t.Errorf("end points at %q, want the closing quote", test.src[end])
			}
		})
	}
}

// A refused key must report no mask at all. The caller discards the edits on
// `!ok`, so a helper that reported bytes before refusing would be invisible
// end-to-end; this pins that the two results agree.
func TestMaskSingleQuotedKeyReportsNoMaskWhenRefused(t *testing.T) {
	for _, src := range []string{"${Z['a,b[c']}", "${Z['a,b}", "${Z['a,b\nc']}"} {
		t.Run(src, func(t *testing.T) {
			var masked []int
			if _, ok := maskSingleQuotedKey([]byte(src), 4, &masked); ok {
				t.Fatalf("maskSingleQuotedKey(%q) = ok, want refused", src)
			}
			if len(masked) != 0 {
				t.Errorf("mask = %v, want none on a refused extent", masked)
			}
		})
	}
}

// The quote is only decidable inside the nested subscript's brackets, so
// maskNestedExpansion gates on its own bracket depth before stepping over one.
// Outside the brackets the quote is not a key: `${Z[a]'x'}` passes `zsh -f -n`
// and is `bad substitution` when run. That gate is invisible to the helper's
// own unit test, which is always called at a quote inside the brackets.
func TestMaskNestedExpansionQuotePosition(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantOK  bool
		wantEnd int
	}{
		{"single quote inside the subscript", "${Z['ab']}", true, 10},
		{"single quote after the subscript", "${Z[a]'x'}", false, 0},
		{"single quote before the subscript", "${Z'x'[a]}", false, 0},
		{"single quote with no subscript", "${Z'x'}", false, 0},
		{"double quote inside the subscript", "${Z[\"ab\"]}", false, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			end, ok := maskNestedExpansion([]byte(test.src), 1, func(int) {})
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if end != test.wantEnd {
				t.Errorf("end = %d, want %d", end, test.wantEnd)
			}
		})
	}
}

// The repair must survive any other adapter feature in the same file, in either
// order, since the masked retry is an ordinary parse of a whole file. The rows
// come from adapterSnippets, so an adapter added to the chain extends this
// automatically.
func TestFlagPatternQuotedNestedKeyComposesWithEveryAdapter(t *testing.T) {
	const gap = "print ${m[(i)${Z['ab']}]}"
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
				// Parsing alone would pass with the pattern still
				// cut, so the pattern's text is what is asserted.
				var repaired bool
				syntax.Walk(file.AST(), func(node syntax.Node) bool {
					if flagged, ok := node.(*syntax.FlagsArithm); ok {
						if wordSource(order.src, flagged.X) == "${Z['ab']}" {
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
