package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #379: a flagged subscript pattern whose bracket expression is followed
// by a command substitution, a grave-accent substitution or an arithmetic
// expansion, `${m[(i)a[bc]$(echo x)]}`.
//
// mvdan/sh reads a flagged subscript's pattern as one raw literal and ends it
// at the first `]`, so the bracket expression's own `]` cut the pattern and the
// `$` after it was read as a parameter-expansion operator. The substitution
// itself was never the problem: `${m[(i)ab$(echo x)]}`, with no bracket
// expression, parses on main and keeps the substitution's bytes inside that
// literal. scanFlagPatternBrackets simply refused any pattern holding a `$(` or
// a backquote, so the repair for the bracket never ran.
//
// Every row here is `zsh -f -n` valid and runs under `zsh -f`.
func TestParseFlagPatternCommandSubstitution(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
		col     uint
	}{
		{"command substitution", "print ${m[(i)a[bc]$(echo x)]}\n", "i", "a[bc]$(echo x)", 14},
		{"grave accent substitution", "print ${m[(i)a[bc]`echo x`]}\n", "i", "a[bc]`echo x`", 14},
		{"arithmetic expansion", "print ${m[(i)a[bc]$(( 1 ))]}\n", "i", "a[bc]$(( 1 ))", 14},
		{"reverse flag", "print ${m[(r)a[bc]$(echo c)]}\n", "r", "a[bc]$(echo c)", 14},
		{"bracket class then a glob", "print ${m[(i)[ab]$(echo x)*]}\n", "i", "[ab]$(echo x)*", 14},
		{"inside double quotes", "print \"${m[(i)a[bc]$(echo x)]}\"\n", "i", "a[bc]$(echo x)", 15},
		{"length prefix", "print ${#m[(i)a[bc]$(echo x)]}\n", "i", "a[bc]$(echo x)", 15},
		{"operator after the subscript", "print ${m[(i)a[bc]$(echo x)]:-none}\n", "i", "a[bc]$(echo x)", 14},
		{"assignment form", "m[(i)a[bc]$(echo x)]=1\n", "i", "a[bc]$(echo x)", 6},
		{"nested parameter expansion", "print ${m[(i)a[bc]$(echo $x)]}\n", "i", "a[bc]$(echo $x)", 14},
		{"nested command substitution", "print ${m[(i)a[bc]$(echo $(echo c))]}\n", "i", "a[bc]$(echo $(echo c))", 14},
		{"nested arithmetic expansion", "print ${m[(i)a[bc]$(echo $((1)))]}\n", "i", "a[bc]$(echo $((1)))", 14},
		{"parenthesized list inside", "print ${m[(i)a[bc]$( (echo x) )]}\n", "i", "a[bc]$( (echo x) )", 14},
		{"two substitutions", "print ${m[(i)a[bc]$(echo a)$(echo b)]}\n", "i", "a[bc]$(echo a)$(echo b)", 14},
		{"bracket between two substitutions", "print ${m[(i)a[bc]$(echo a)[de]$(echo b)]}\n", "i", "a[bc]$(echo a)[de]$(echo b)", 14},
		{"substitution before the bracket", "print ${m[(i)$(echo a)[bc]]}\n", "i", "$(echo a)[bc]", 14},
		{"text after the substitution", "print ${m[(i)a[bc]$(( 1 ))x]}\n", "i", "a[bc]$(( 1 ))x", 14},
		// The reported arithmetic context, where the bytes after the cut
		// are read as arithmetic rather than as an expansion operator, so
		// the adapter's positional arm is what fires.
		{"inside arithmetic", "print $(( m[(i)a[bc]$(echo x)] ))\n", "i", "a[bc]$(echo x)", 16},
		{"inside arithmetic, grave accent", "print $(( m[(i)a[bc]`echo x`] ))\n", "i", "a[bc]`echo x`", 16},
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
			// original offsets: the point of a byte-preserving mask.
			if got := test.src[start : start+len(test.pattern)]; got != test.pattern {
				t.Errorf("source at pattern offsets = %q, want %q", got, test.pattern)
			}
		})
	}
}

// The pattern must come back as the literal's own text, not merely span the
// right offsets. A repair that left the `_` mask in the tree would satisfy an
// offset comparison against the source and still hand every rule a pattern the
// script does not have.
func TestFlagPatternCommandSubstitutionRestoresLiteral(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"comma inside the substitution", "print ${m[(i)a[bc]$(echo a,b)]}\n", "a[bc]$(echo a,b)"},
		{"comma inside a grave accent substitution", "print ${m[(i)a[bc]`echo a,b`]}\n", "a[bc]`echo a,b`"},
		{"no comma to restore", "print ${m[(i)a[bc]$(echo x)]}\n", "a[bc]$(echo x)"},
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

// A `,` inside the substitution is the substitution's own byte, not the range
// separator of the enclosing subscript. mvdan/sh splits a subscript index at
// any `,` it sees in the literal, so without the mask
// `${m[(i)a[bc]$(echo a,b)]}` would parse into a `BinaryArithm` `,` — a range
// the source does not have — which an index-blind assertion would not catch.
func TestFlagPatternCommandSubstitutionKeepsIndexShape(t *testing.T) {
	t.Run("a comma in the substitution is not a range", func(t *testing.T) {
		const src = "print ${m[(i)a[bc]$(echo a,b)]}\n"
		file, err := Parse(strings.NewReader(src), "substitution-comma.zsh")
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

	t.Run("a real range after the substitution keeps both endpoints", func(t *testing.T) {
		const src = "print ${m[(r)a[bc]$(echo c),3]}\n"
		file, err := Parse(strings.NewReader(src), "substitution-range.zsh")
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
		if got := wordSource(src, flagged.X); got != "a[bc]$(echo c)" {
			t.Errorf("pattern = %q, want %q", got, "a[bc]$(echo c)")
		}
		if got := wordSource(src, binary.Y); got != "3" {
			t.Errorf("endpoint = %q, want %q", got, "3")
		}
	})
}

// Native-invalid rows the mask must keep rejected, each found by probing this
// change rather than by the rows above. They are the reason the helper refuses
// a bracket, a parenthesis inside a quote, a brace and a comment instead of
// counting through them, and each one was an introduced false accept in an
// earlier revision that treated a bracket inside the substitution as ordinary
// text.
//
// These are native-invalid sources, so they live in `testdata/invalid-379-*.txt`
// and are never executed, per docs/project/parser-gap-workflow.md. The error
// text and position are asserted, not merely `err != nil`, so a future change
// cannot keep rejecting them for an unrelated reason and still pass. Every one
// is the byte-for-byte error main gives.
func TestFlagPatternCommandSubstitutionRejectsNativeInvalid(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		col     uint
	}{
		// `$(echo [)` — Zsh counts a bracket inside the substitution as a
		// subscript delimiter, so the unbalanced `[` is `bad substitution`.
		{"testdata/invalid-379-bracket-in-substitution.txt", "not a valid parameter expansion operator: `$`", 19},
		// Quoting does not exempt it: `$(echo "]")` is `bad substitution`
		// even with no bracket expression in the pattern at all.
		{"testdata/invalid-379-quoted-close-bracket.txt", "not a valid parameter expansion operator: `\"`", 24},
		// A quoted `)` really does close the substitution natively, which
		// is why any quote refuses rather than being counted through.
		{"testdata/invalid-379-quoted-paren.txt", "not a valid parameter expansion operator: `$`", 19},
		// An unclosed `$(` inside the pattern, the issue's own criterion.
		{"testdata/invalid-379-unclosed-substitution.txt", "not a valid parameter expansion operator: `$`", 19},
		// A `#` comments out the rest of the line, delimiter included.
		{"testdata/invalid-379-comment-in-substitution.txt", "not a valid parameter expansion operator: `$`", 19},
		// An unquoted `}` closes the enclosing `${`, so `$(echo })` is a
		// native parse error where `$(echo "}")` is valid: brace handling
		// is quote-sensitive, so either brace refuses.
		{"testdata/invalid-379-brace-in-substitution.txt", "not a valid parameter expansion operator: `$`", 19},
		// The backquoted form counts brackets the same way.
		{"testdata/invalid-379-backquote-bracket.txt", "not a valid parameter expansion operator: \"`\"", 19},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-379.zsh")
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

// Valid Zsh the mask still refuses, pinned so the limit is visible and a later
// fix has a test to flip. Each row keeps the verdict it has on main, so nothing
// regresses; refusing is the contract's answer when an extent is not decidable,
// and the alternative here is guessing at Zsh's quote rules inside a subscript.
//
// A quote refuses because a byte count and a quote are not composable (#371),
// and Zsh does count a quoted bracket and a quoted parenthesis as delimiters
// here. A `$` inside the backquoted form refuses because a nested `$(` or `${`
// reintroduces a delimiter that arm does not count.
func TestFlagPatternCommandSubstitutionKnownLimits(t *testing.T) {
	for _, src := range []string{
		"print ${m[(i)a[bc]$(echo \"x\")]}\n",
		"print ${m[(i)a[bc]$(echo 'x')]}\n",
		"print ${m[(i)a[bc]$(echo \"a b\")]}\n",
		"print ${m[(i)a[bc]$(echo \"[]\")]}\n",
		"print ${m[(i)a[bc]$(echo a]b[c)]}\n",
		"print ${m[(i)a[bc]$(echo ${y[1]})]}\n",
		"print ${m[(i)a[bc]$(echo x`echo y`)]}\n",
		"print ${m[(i)a[bc]`echo ${x}`]}\n",
		"print ${m[(i)a[bc]`echo \"x\"`]}\n",
		"print ${m[(i)a[bc]`echo (x)`]}\n",
	} {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), "limit-379.zsh"); err == nil {
				t.Errorf("Parse(%q) succeeded; the known limit is that it is refused", src)
			}
		})
	}
	t.Log("known limit: a quote, a nested delimiter or a bracket inside the substitution refuses the pattern")
}

// maskPatternSubstitution's two arms are the changed helpers, and the
// end-to-end rows cannot tell their two jobs apart: a mutation that stopped
// reporting the mask but kept the right end offset would leave the `,` splitting
// the index, which reads as the gap being back, while one that reported a byte
// outside the substitution would be caught downstream by restorePatternEdits'
// own accounting rather than by any assertion here. Both are pinned at the
// helper's layer, where the refusals are visible as refusals.
func TestMaskPatternSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		start    int
		wantOK   bool
		wantEnd  int
		wantMask []int
	}{
		{"command substitution", "$(echo x)", 0, true, 9, nil},
		{"comma inside", "$(echo a,b)", 0, true, 11, []int{8}},
		{"two commas inside", "$(echo a,b,c)", 0, true, 13, []int{8, 10}},
		{"arithmetic expansion", "$(( 1 ))", 0, true, 8, nil},
		{"nested parentheses", "$( (echo x) )", 0, true, 13, nil},
		{"nested substitution", "$(echo $(echo c))", 0, true, 17, nil},
		{"grave accent", "`echo x`", 0, true, 8, nil},
		{"grave accent with a comma", "`echo a,b`", 0, true, 10, []int{7}},
		// Extents the helper declines to decide. Each keeps the verdict
		// the enclosing pattern has on main.
		{"unclosed command substitution", "$(echo x", 0, false, 0, nil},
		{"unclosed grave accent", "`echo x", 0, false, 0, nil},
		{"newline inside", "$(echo\nx)", 0, false, 0, nil},
		// A bracket is a subscript delimiter for Zsh regardless of the
		// substitution, so masking or skipping it would change a verdict.
		{"open bracket inside", "$(echo [)", 0, false, 0, nil},
		{"close bracket inside", "$(echo ])", 0, false, 0, nil},
		{"balanced brackets inside", "$(echo [])", 0, false, 0, nil},
		{"bracket in a grave accent", "`echo ]`", 0, false, 0, nil},
		// Quoting is not tracked: Zsh counts a quoted bracket and a
		// quoted parenthesis here, so a count cannot survive a quote.
		{"double quote inside", "$(echo \"x\")", 0, false, 0, nil},
		{"single quote inside", "$(echo 'x')", 0, false, 0, nil},
		{"quoted paren inside", "$(echo \")\")", 0, false, 0, nil},
		{"quote in a grave accent", "`echo \"x\"`", 0, false, 0, nil},
		// A brace is quote-sensitive natively, so either brace refuses.
		{"close brace inside", "$(echo })", 0, false, 0, nil},
		{"open brace inside", "$(echo {)", 0, false, 0, nil},
		// A comment would swallow the delimiter, and the adapter contract
		// forbids masking a byte a *syntax.Comment would hold.
		{"comment inside", "$(echo x # )", 0, false, 0, nil},
		{"comment in a grave accent", "`echo x # `", 0, false, 0, nil},
		// A backslash refuses: an escaped delimiter is not a delimiter,
		// so counting it is wrong and skipping it hides a `]` the outer
		// scan needs.
		{"backslash inside", "$(echo \\])", 0, false, 0, nil},
		// A nested backquote inside `$(` and a `$` inside a backquote
		// each reintroduce a delimiter the arm in question cannot count.
		{"grave accent inside a substitution", "$(echo `echo y`)", 0, false, 0, nil},
		{"expansion inside a grave accent", "`echo ${x}`", 0, false, 0, nil},
		{"parameter inside a grave accent", "`echo $x`", 0, false, 0, nil},
		{"parenthesis in a grave accent", "`echo (x)`", 0, false, 0, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var masked []int
			end, ok := maskPatternSubstitution([]byte(test.src), test.start, func(offset int) {
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
				if b := test.src[offset]; b != ',' {
					t.Errorf("mask[%d] points at %q, want a comma", i, b)
				}
			}
		})
	}
}

// A refused substitution must report no mask at all. The caller discards the
// edits on `!ok`, so a helper that reported bytes before running off the end
// would be invisible end-to-end; this pins that the two results agree.
func TestMaskPatternSubstitutionReportsNoMaskWhenRefused(t *testing.T) {
	for _, src := range []string{
		"$(echo a,b",
		"`echo a,b",
		"$(echo a,b\n)",
		"$(echo a,b [)",
		"$(echo a,\"b\")",
		"`echo a,b (`",
	} {
		t.Run(src, func(t *testing.T) {
			calls := 0
			if _, ok := maskPatternSubstitution([]byte(src), 0, func(int) { calls++ }); ok {
				t.Fatalf("maskPatternSubstitution(%q) = ok, want refused", src)
			}
			if calls != 0 {
				t.Errorf("mask calls = %d, want 0 on a refused extent", calls)
			}
		})
	}
}

// scanFlagPatternBrackets is what the substitution mask feeds. Its end offset
// must be the `]` that really closes the subscript, past the substitution, and
// its edits must cover the substitution's `,` as well as the pattern's own
// brackets.
func TestScanFlagPatternBracketsWithCommandSubstitution(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantOK    bool
		wantClose int
		wantEdits int
	}{
		// `print ${m[(i)` is 13 bytes, so every pattern starts at 13.
		{"command substitution", "print ${m[(i)a[bc]$(echo x)]}", true, 27, 2},
		{"comma inside the substitution", "print ${m[(i)a[bc]$(echo a,b)]}", true, 29, 3},
		{"grave accent", "print ${m[(i)a[bc]`echo x`]}", true, 26, 2},
		{"substitution before the bracket", "print ${m[(i)$(echo a)[bc]]}", true, 26, 2},
		{"bracket inside the substitution", "print ${m[(i)a[bc]$(echo [)]}", false, 0, 0},
		{"quote inside the substitution", "print ${m[(i)a[bc]$(echo \"x\")]}", false, 0, 0},
		// A range's `,` after the substitution still ends the pattern.
		{"range after the substitution", "print ${m[(r)a[bc]$(echo c),3]}", true, 27, 2},
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

// The repair must survive any other adapter feature in the same file, in either
// order, since the masked retry is an ordinary parse of a whole file. The rows
// come from adapterSnippets, so an adapter added to the chain extends this
// automatically.
func TestFlagPatternCommandSubstitutionComposesWithEveryAdapter(t *testing.T) {
	const gap = "print ${m[(i)a[bc]$(echo x)]}"
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
						if wordSource(order.src, flagged.X) == "a[bc]$(echo x)" {
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
