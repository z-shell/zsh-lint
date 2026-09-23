package parse

import (
	"bytes"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #250: a flagged subscript pattern holding a bracket expression, in
// every position a flagged pattern may stand. #237 handled only the pattern
// that opens its own subscript, `name[(i)pat]`; the other two positions reach
// the same scanner and were rejected.
//
// The second-subscript adapter (#215) masks its `][` boundary as `, `, so a
// flagged second subscript arrives here through the range-comma position.
// That is the composition #250 reports, and it is fixed by handling the
// grammar position rather than by recognising the mask: the range form fails
// identically with no second subscript anywhere in the source.
func TestParseFlaggedSubscriptBracketPositions(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		flags   string
		pattern string
	}{
		{
			name:    "second subscript, through the masked boundary",
			src:     "print -r -- ${a[b][(i)[x]]}\n",
			flags:   "i",
			pattern: "[x]",
		},
		{
			name:    "range endpoint carries flags",
			src:     "print -r -- ${a[1,(i)[x]]}\n",
			flags:   "i",
			pattern: "[x]",
		},
		{
			name:    "subscript on a nested expansion",
			src:     "print -r -- ${${a[b]}[(i)[x]]}\n",
			flags:   "i",
			pattern: "[x]",
		},
		{
			name:    "range endpoint in a subscript on a nested expansion",
			src:     "print -r -- ${${a[b]}[1,(i)[x]]}\n",
			flags:   "i",
			pattern: "[x]",
		},
		{
			name:    "third subscript",
			src:     "print -r -- ${a[b][c][(i)[x]]}\n",
			flags:   "i",
			pattern: "[x]",
		},
		{
			name:    "second subscript on a nested expansion",
			src:     "print -r -- ${${a[b]}[c][(i)[x]]}\n",
			flags:   "i",
			pattern: "[x]",
		},
		{
			name:    "nested expansion inside the pattern",
			src:     "print -r -- ${a[b][(i)[^x]${s}[^x]]}\n",
			flags:   "i",
			pattern: "[^x]${s}[^x]",
		},
		{
			name:    "the zi corpus shape, backreference group and expansion",
			src:     "integer i=\"${f[$n][(i)(#b)([^x]${s}[^x])]}\"\n",
			flags:   "i",
			pattern: "(#b)([^x]${s}[^x])",
		},
		{
			name:    "escaped bracket in a second subscript",
			src:     "print -r -- ${a[b][(i)\\]x]}\n",
			flags:   "i",
			pattern: "\\]x",
		},
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
			// The pattern is reassembled from the word's parts so a nested
			// expansion, which stays its own node, is compared as source.
			if got := wordSource(test.src, flagged.X); got != test.pattern {
				t.Errorf("pattern = %q, want %q", got, test.pattern)
			}
			// Every masked byte must be restored: the pattern's own text has
			// to index back into the original source at its own offsets.
			start := int(flagged.X.Pos().Offset())
			end := int(flagged.X.End().Offset())
			if got := test.src[start:end]; got != test.pattern {
				t.Errorf("source at pattern offsets = %q, want %q", got, test.pattern)
			}
		})
	}
}

// soleFlagsArithm returns the one FlagsArithm the file holds, failing if there
// is not exactly one, so a test cannot pass by finding some other subscript.
//
// A flagged subscript that follows a first one is detached from the tree into
// SecondSubscripts, which syntax.Walk does not reach, so both are searched.
func soleFlagsArithm(t *testing.T, file *File) *syntax.FlagsArithm {
	t.Helper()
	var found []*syntax.FlagsArithm
	collect := func(node syntax.Node) bool {
		if flagged, ok := node.(*syntax.FlagsArithm); ok {
			found = append(found, flagged)
		}
		return true
	}
	syntax.Walk(file.AST(), collect)
	for _, second := range file.SecondSubscripts() {
		for _, subscript := range second.Subscripts {
			syntax.Walk(subscript, collect)
		}
	}
	if len(found) != 1 {
		t.Fatalf("FlagsArithm nodes = %d, want 1", len(found))
	}
	return found[0]
}

// wordSource returns the original source the expression spans.
func wordSource(src string, expr syntax.ArithmExpr) string {
	start := int(expr.Pos().Offset())
	end := int(expr.End().Offset())
	if start < 0 || end > len(src) || start > end {
		return ""
	}
	return src[start:end]
}

// The second subscript must still be retained as its own node: this shape
// reaches the pattern scanner through #215's mask, so a fix that satisfied the
// pattern while losing the subscript split would pass the rows above.
func TestFlaggedSecondSubscriptRetainsSubscripts(t *testing.T) {
	file, err := Parse(strings.NewReader("print -r -- ${a[b][(i)[x]]}\n"), "retain.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(file.SecondSubscripts()) != 1 {
		t.Fatalf("SecondSubscripts = %d, want 1", len(file.SecondSubscripts()))
	}
	second := file.SecondSubscripts()[0]
	if len(second.Subscripts) != 1 {
		t.Fatalf("Subscripts = %d, want 1", len(second.Subscripts))
	}
	// The retained subscript is the flagged one, at its original offset.
	flagged, ok := second.Subscripts[0].(*syntax.FlagsArithm)
	if !ok {
		t.Fatalf("Subscripts[0] = %T, want *syntax.FlagsArithm", second.Subscripts[0])
	}
	if flagged.Flags == nil || flagged.Flags.Value != "i" {
		t.Errorf("Flags = %v, want \"i\"", flagged.Flags)
	}
	// The expansion keeps the first subscript in its own Index.
	lit, ok := second.Expansion.Index.(*syntax.Word)
	if !ok || len(lit.Parts) != 1 {
		t.Fatalf("Index = %T, want a single-part *syntax.Word", second.Expansion.Index)
	}
}

// The widened opener scan must not accept a `(` that is not a flags group.
//
// This is tested directly rather than through Parse: the scan runs only after
// a specific parse error, and no source in the corpus reaches it with a comma
// outside a subscript, so an end-to-end test would pass whether or not the
// guard existed. The guard is kept because the scan walks backwards over
// arbitrary bytes and a `,` there says nothing on its own about being in a
// subscript.
func TestFlagGroupOpener(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		// The two real openers.
		{"subscript's own opener", "${a[(i)x]}", true},
		{"range comma", "${a[1,(i)x]}", true},
		{"second-subscript mask, space after comma", "${a[b, (i)x]}", true},
		{"subscript on a nested expansion", "${${a[b]}[(i)x]}", true},
		{"range comma in a subscript on a nested expansion", "${${a[b]}[1,(i)x]}", true},
		// A closed bracket pair before the comma is skipped by the depth
		// count, so the walk reaches the subscript's own `[` rather than
		// stopping at the inner one.
		{"bracket pair before the comma", "${a[ [b] ,(i)x]}", true},
		// A comma that is not inside a subscript.
		{"arithmetic comma", "$(( a, (b) ))", false},
		{"comma in a command", "print a, (b)", false},
		// A `[` that is not a subscript opener.
		{"bracket not after a name", "print [(i)x]", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(test.src)
			paren := bytes.IndexByte(src, '(')
			if paren < 0 {
				t.Fatalf("no `(` in %q", test.src)
			}
			if got := isFlagGroupOpener(src, paren); got != test.want {
				t.Errorf("isFlagGroupOpener(%q, %d) = %v, want %v", test.src, paren, got, test.want)
			}
		})
	}
}

// `${a[1,(2+3)]}` is a range whose endpoint is arithmetic, not a flagged
// pattern: it selects elements 1 to 5. The front end reads the `(2+3)` as a
// flags group, which is wrong, but it is wrong on main too and this change
// neither causes nor worsens it — the opener scan runs only after a parse
// error, and this source parses.
//
// It is pinned as the current behaviour so the bug is visible and a later fix
// has a failing test to flip, rather than being silently attributed here.
func TestFlaggedSubscriptArithmeticRangeEndpointIsMisread(t *testing.T) {
	file, err := Parse(strings.NewReader("print ${a[1,(2+3)]}\n"), "arith-range.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	flagged := soleFlagsArithm(t, file)
	if flagged.Flags == nil || flagged.Flags.Value != "2+3" {
		t.Fatalf("Flags = %v, want \"2+3\" (the known-wrong reading)", flagged.Flags)
	}
	t.Log("known defect: an arithmetic range endpoint is read as a flags group")
}

// A pattern whose extent the scanner cannot decide keeps the base front end's
// error rather than a guessed mask.
//
// None of these reaches the bracket scanner: the retry is seeded by an error
// whose previous byte is the `]` that closed a pattern early, and each of
// these fails at the `$` or the backtick first. They are pinned as sources
// this adapter must not start accepting, not as evidence that the scanner
// refuses them.
//
// The command-substitution rows are native-valid Zsh that the adapter still
// rejects: deciding their extent is #237's documented limit, unchanged here.
func TestFlaggedSubscriptRejectsUndecidablePatterns(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unbalanced nested expansion", "print -r -- ${a[b][(i)[x]${s]}\n"},
		{"command substitution in a second subscript", "print -r -- ${a[b][(i)$(echo [x])]}\n"},
		{"backtick in a second subscript", "print -r -- ${a[b][(i)`echo [x]`]}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(test.src), test.name+".zsh"); err == nil {
				t.Errorf("Parse(%q) unexpectedly succeeded", test.src)
			}
		})
	}
}
