package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #517. Zsh lexes an `=~` operand as an ordinary word (gettokstr in
// Src/lex.c), so, as in any other word (#511), `;`, `&`, and a `<` or `>`
// that starts neither a process substitution nor a numeric glob end it
// inside a group. Native verdicts use newline-terminated files and
// zsh -f -n.
func TestZshRegexGroupWordEndsReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ x =~ a(b;c) ]]", "1:12: not a valid test operator: `;`"},
		{"[[ x =~ a(b&c) ]]", "1:12: not a valid test operator: `&`"},
		{"[[ x =~ a(b<c) ]]", "1:12: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ x =~ a(b>c) ]]", "1:12: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ x =~ (b;c) ]]", "1:11: not a valid test operator: `;`"},
		{"[[ x =~ a(b<<c) ]]", "1:12: not a valid test operator: `<<`"},
		{"[[ x =~ a(b>|c) ]]", "1:12: not a valid test operator: `>|`"},
		{"[[ x =~ (a(b;c)) ]]", "1:13: not a valid test operator: `;`"},
		{"[[ x =~ a(b;c) && y ]]", "1:12: not a valid test operator: `;`"},
		{"[[ y && x =~ a(b;c) ]]", "1:17: not a valid test operator: `;`"},
		{"[[ ( x =~ a(b;c) ) ]]", "1:14: not a valid test operator: `;`"},
		{"f() { [[ x =~ a(b;c) ]]; }", "1:18: not a valid test operator: `;`"},
		{"[[ x =~ a(b\\\nc;d) ]]", "2:2: not a valid test operator: `;`"},
		{"[[ x =~ a(b;c ]]", "1:12: not a valid test operator: `;`"},
		// A process substitution stays in the word; a `;` after it does not.
		{"[[ x =~ a(<(y);c) ]]", "1:15: not a valid test operator: `;`"},
		// Numeric globs: only a complete `<m-n>` stays in the word.
		{"[[ x =~ a(<x>) ]]", "1:11: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ x =~ a(<1-2) ]]", "1:11: expected `&&`, `||` or `]]` after complex expr"},
		{"[[ x =~ a(<1-2;c) ]]", "1:11: expected `&&`, `||` or `]]` after complex expr"},
		// A `&&` is a connective, so the group's `)` is reported where it
		// stands, naming the outermost `(` that is still open.
		{"[[ x =~ a(b&&c) ]]", "1:15: `)` matches no `(`: the word ended inside the glob group at 1:10"},
		{"[[ x =~ a(b(c)d&&e) ]]", "1:19: `)` matches no `(`: the word ended inside the glob group at 1:10"},
		{"[[ x =~ (b&&c) ]]", "1:14: `)` matches no `(`: the word ended inside the glob group at 1:9"},
		{"[[ x =~ a(\"x\"(b&&c)) ]]", "1:19: `)` matches no `(`: the word ended inside the glob group at 1:10"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshRegexGroupWordAccept(t *testing.T) {
	for _, src := range []string{
		"[[ x =~ a(b|c) ]]",
		"[[ x =~ a(b||c) ]]",
		"[[ x =~ a(b c) ]]",
		"[[ x =~ a(b\nc) ]]",
		"[[ x =~ ^(a|b)$ ]]",
		"[[ x =~ (a) && y ]]",
		"[[ x =~ a(<1-2>) ]]",
		"[[ x =~ a(<->b) ]]",
		"[[ x =~ a(<(y)) ]]",
		"[[ x =~ a(>(y)) ]]",
		"[[ x =~ a(<(y)|c) ]]",
		"[[ x =~ '(b;c)' ]]",
		"[[ x =~ a\\(b\\;c\\) ]]",
		"[[ x =~ a(\"b;c\") ]]",
		"[[ x =~ a($(b;c)) ]]",
		"[[ x =~ a(${y:-;}) ]]",
		// The word ends at the `&&` with the group open, and the rest is
		// a connective and an operand: valid Zsh, which base rejected.
		"[[ x =~ a(b&&c ]]",
		"[[ x =~ a(b && y ]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A numeric glob longer than the parser's read buffer is read rather than
// peeked, as in any other word (#511).
func TestZshRegexGroupLongNumericGlob(t *testing.T) {
	digits := strings.Repeat("1", 1100)
	for _, src := range []string{
		"[[ x =~ a(<" + digits + "-2>) ]]",
		"[[ x =~ a(b<" + strings.Repeat("1", 5000) + "-2>c) ]]",
	} {
		if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
			t.Fatalf("%s: %v", src[:14], err)
		}
	}
	src := "[[ x =~ a(<" + digits + "-2) ]]"
	want := "1:1114: a numeric glob cannot contain `)`"
	if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
}

// Bash reads a regex operand with its own rules, where these characters stay
// inside a group.
func TestBashRegexGroupWordUnchanged(t *testing.T) {
	for _, src := range []string{"[[ x =~ a(b;c) ]]", "[[ x =~ a(b&&c) ]]", "[[ x =~ a(b<c) ]]"} {
		if _, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
	}
}
