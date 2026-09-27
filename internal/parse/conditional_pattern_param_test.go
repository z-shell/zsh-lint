package parse

import (
	"strings"
	"testing"
)

// Issue #513: Zsh counts no parentheses inside a parameter expansion
// (in_brace_param in gettokstr, Src/lex.c), so a `(` or `)` in an unquoted
// `${...}` in a conditional pattern opens or closes no group. The nested
// conditional pattern scan skips the expansion whole, as it already does
// inside double quotes. Native verdicts use newline-terminated files and
// zsh -f -n.
func TestParseConditionalPatternSkipsParameterExpansion(t *testing.T) {
	for _, src := range []string{
		"[[ x == a(${x:-(}) ]]",
		"[[ x == a(${x:-(}|b) ]]",
		"[[ x == (${x:-(}) ]]",
		"[[ x == a(${(j:(:)y}) ]]",
		"[[ x == ${x:-(} ]]",
		"[[ x == *${x:-(}* ]]",
		"[[ x = a(${x:-(}) ]]",
		"[[ x != a(${x:-(}) ]]",
		"[[ x == a(${x:-${y:-(}}) ]]",
		"[[ x == a(${x:-{(}}) ]]",
		"[[ x == a(${x/(/y}) ]]",
		// A nested alternation, the adapter's own construct, around one.
		"[[ x == (a|(b|${x:-(})) ]]",
		"[[ x == (a|(b|${x:-)})) ]]",
		"[[ x == ((${x:-(})|b) ]]",
		"[[ x == (a|(b)${x:-(}) ]]",
		// Operator bytes inside the expansion are its own text.
		"[[ x == (a|(b|${x:-;})) ]]",
		"[[ x == (a|(b|${x:-<})) ]]",
		// Outside double quotes a `'` in the word quotes, so this `}` does
		// not close the expansion. These parse on base too; they pin that
		// the skip scans the word unquoted.
		"[[ x == a(${x:-'}'}) ]]",
		"[[ x == (a|(b|${x:-'}'})) ]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src+"\n"), "param-in-pattern.zsh"); err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
		})
	}
}

// The skip changes nothing outside the expansion: an unbalanced group, an
// operator after the expansion, or an expansion the shared rule cannot
// close is still rejected, as native Zsh rejects each of these.
func TestParseConditionalPatternParameterExpansionStillRejects(t *testing.T) {
	for _, src := range []string{
		"[[ x == (a|(${x:-(}) ]]",
		"[[ x == a(${x:-(} ]]",
		"[[ x == a${x:-(}) ]]",
		"[[ x == a(${x};b) ]]",
		"[[ x == (a|(b|${x});c) ]]",
		"[[ x == a(${x) ]]",
		"[[ x == (a|(b|${x) ]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src+"\n"), "param-in-pattern-invalid.zsh"); err == nil {
				t.Fatal("Parse() unexpectedly accepted native-invalid source")
			}
		})
	}
}
