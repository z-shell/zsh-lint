package parse

import (
	"bytes"
	"os"
	"testing"
)

// Issue #382: a flagged subscript whose pattern the parser cut short was
// accepted or rejected according to what the leftover bytes happened to mean in
// their context, so an outer double quote silently selected the verdict.
//
// mvdan/sh reads a flagged pattern as one raw literal and ends it at the first
// `]` whatever encloses that byte, so `${m[(i)${Z[a]}]]}` is read with the
// pattern `${Z[a` and `]]}` left over. Unquoted, those bytes form a word ending
// in `}` and rejectCloseBraceWords refuses them; inside a double quote they are
// ordinary text and nothing looks at them. Both spellings are `bad substitution`
// natively, so both must be rejected.
//
// Each source is verified against `zsh -f -n` and, where Zsh defers the error
// to evaluation, against running it under `zsh -f`. They are native-invalid, so
// they live in `testdata/invalid-382-*.txt` rather than the `.zsh` corpus, whose
// repository-wide gate requires every tracked source to be valid Zsh.
func TestParseRejectsFlagPatternCuts(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		col     uint
	}{
		// The issue's own rows. Native: `bad substitution` at parse time.
		{"invalid-382-nested-subscript-stray-close.txt", flagPatternCutError, 20},
		{"invalid-382-substitution-close-bracket.txt", flagPatternCutError, 20},
		{"invalid-382-substitution-open-bracket.txt", flagPatternCutError, 20},
		{"invalid-382-double-quoted-key.txt", flagPatternCutError, 24},

		// The cut is visible in every context, not only inside a quote.
		// `zsh -n` parses these two and reports `bad substitution` only
		// when the expansion is reached, which is why the runtime oracle
		// is load-bearing for this issue.
		{"invalid-382-assignment-context.txt", flagPatternCutError, 15},

		// Only the bracket-scan arm sees these: the pattern holds a
		// single quote, which the ends-open arm declines because Zsh's
		// quote rule inside a nested subscript is asymmetric and the
		// single-quoted spelling is often the valid one. Here the scan
		// bounds the pattern anyway and finds the cut. Measured, the
		// scan arm alone rejects 36 rows of the quote probe.
		{"invalid-382-single-quote-before-nested.txt", flagPatternCutError, 23},
		{"invalid-382-single-quote-assignment.txt", flagPatternCutError, 18},

		// Second-level subscript rows, where the misread leaves no
		// flagged pattern literal in the tree at all.
		{"invalid-382-second-subscript-stray-close.txt", flagPatternCutError, 23},
		{"invalid-382-second-subscript-assignment.txt", flagPatternCutError, 18},

		// The same shape outside quotes was already rejected before this
		// guard existed, by the close-brace guard, which runs first and
		// reports the leftover `}`. Pinned so a change that moves these
		// rows to this guard's error is visible rather than silent.
		{"invalid-382-unquoted-stray-close.txt", closeBraceWordError, 23},
		{"invalid-382-case-word-context.txt", closeBraceWordError, 22},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			assertParseErrorAt(t, src, test.text, 1, test.col)
		})
	}
}

// Rows the guard must leave alone. A pattern that merely holds a nested
// expansion is not cut, and `zsh -f -n` accepts every row here.
func TestParseAcceptsUncutFlagPatterns(t *testing.T) {
	const pre = "typeset -A Z=( a 1 )\ntypeset -A Y=( 'a b' 7 )\nlocal -a m=( a1 abx abc )\n"
	sources := []string{
		// The issue's "must keep parsing" table.
		`print "${m[(i)${Z[a]}x]}"`,
		`print "${m[(i)${Z[a]}$(echo x)]}"`,
		`print "${m[(i)a_bc_$(echo x)]}"`,

		// A single-quoted key inside the nested subscript is the valid
		// spelling of the shape the fourth issue row rejects, verified
		// against both oracles. The guard must not treat the two alike.
		`print "${m[(i)${Y['a b']}]}"`,
		`v=${m[(i)${Y['a b']}]}`,
		`print "${m[(i)${Z[${Y['a b']}]}]}"`,

		// Patterns with no nesting at all, including quoted ones.
		`print "${m[(i)abc]}"`,
		`print "${m[(i)"abc"]}"`,
		`print "${m[(r)a[^:]##]}"`,

		// A nested expansion that closes what it opens, in a range and in
		// a second-level subscript.
		`print "${m[(r)${Z[a]},3]}"`,
		`print "${m[(i)${Z[a]}]}"`,
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			full := []byte(pre + src + "\n")
			if _, err := Parse(bytes.NewReader(full), "flag-pattern.zsh"); err != nil {
				t.Errorf("Parse(%q) = %v, want success", src, err)
			}
		})
	}
}

// litEndsInsideExpansion is the second arm's whole decision, so its rule is
// pinned directly: a literal that ends while a `${` or `$(` opened inside it is
// unclosed was cut, unless a single quote makes the row one Zsh may well accept.
func TestLitEndsInsideExpansion(t *testing.T) {
	tests := []struct {
		lit  string
		want bool
	}{
		{"${Z[a", true},
		{"${Z[a]}$(echo ", true},
		{"a${b", true},
		{"$(echo ", true},

		{"${Z[a]}", false},
		{"$(echo x)", false},
		{"abc", false},
		{"", false},

		// A single quote declines the row whatever else it holds.
		{"${Y['a b'", false},
		{"'${x", false},

		// A double quote does not: the double-quoted key inside a nested
		// subscript is the invalid spelling, measured across contexts.
		{`${Y["a b"`, true},

		// An escaped `$` opens nothing.
		{`\${x`, false},
	}
	for _, test := range tests {
		t.Run(test.lit, func(t *testing.T) {
			if got := litEndsInsideExpansion(test.lit); got != test.want {
				t.Errorf("litEndsInsideExpansion(%q) = %v, want %v", test.lit, got, test.want)
			}
		})
	}
}

// The repair's retry must use the reading path that produced the tree it is
// repairing. A file holding an anonymous function invocation parses only
// through parseAnonymousFunctionArgs, so with the bare adapter chain as the
// retry every flagged pattern cut in such a file went unrepaired, and this
// valid source was rejected once the guard above began reporting those cuts.
func TestFlagPatternRepairRunsBesideAnonymousInvocation(t *testing.T) {
	src := []byte("local -a m=( a1 abx abc )\n" +
		"print \"${m[(r)a[^:]##]}\"\n" +
		"() { print \"$1\" } arg\n")
	if _, err := Parse(bytes.NewReader(src), "flag-pattern.zsh"); err != nil {
		t.Fatalf("Parse = %v, want success", err)
	}
}
