package syntax

import (
	"strings"
	"testing"
)

// Zsh removes the quoting of a $'...' here-document delimiter, decoding its
// escapes, so the body ends at the decoded word (#686, formerly the #122
// adapter in internal/parse). Each closer was checked by running the source
// under zsh -f, where the line after it prints "after".
// https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Quoting
func TestZshHdocDollarSglDelimiter(t *testing.T) {
	for _, tc := range []struct{ delim, closer string }{
		{`$'E\x4fF'`, "EOF"},
		{`$'E\x4F'`, "EO"},
		{`$'E\x4fFF'`, "EOFF"},
		{`$'E\117F'`, "EOF"},
		{`$'E\1171'`, "EO1"},
		{`$'\u00e9OF'`, "éOF"},
		{`$'\U000000e9OF'`, "éOF"},
		{`$'E\eF'`, "E\x1bF"},
		{`$'E\tF'`, "E\tF"},
		{`$'E\'F'`, "E'F"},
		{`$'E\\F'`, `E\F`},
		{`$'E\"F'`, `E"F`},
		{`$'E\?F'`, "E?F"},
		{`$'E\qF'`, "EqF"},
		{`x$'\x4f'"F"`, "xOF"},
		{`-$'E\x4fF'`, "EOF"},
	} {
		src := "cat <<" + tc.delim + "\nbody\n" + tc.closer + "\necho after\n"
		if strings.HasPrefix(tc.delim, "-") {
			src = "cat <<" + tc.delim + "\n\tbody\n\t" + tc.closer + "\necho after\n"
		}
		f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if len(f.Stmts) != 2 {
			t.Fatalf("%q: got %d statements, want the body closed and echo after", src, len(f.Stmts))
		}
		if r := f.Stmts[0].Redirs[0]; r.ClosePos.Line() != 3 {
			t.Fatalf("%q: ClosePos = %v, want line 3", src, r.ClosePos)
		}
	}
}

// A decoded newline never matches a line, so the body runs to the end of
// the file, as zsh -f shows by printing the rest of it.
func TestZshHdocDollarSglNewline(t *testing.T) {
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("cat <<$'E\\nF'\nbody\nE\nF\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 1 || f.Stmts[0].Redirs[0].ClosePos.IsValid() {
		t.Fatal("want one statement whose here-document ends at EOF")
	}
}

// Escapes whose decoded bytes are not modeled are an error, not a stop word
// that never matches and would hide the rest of the file. Other dialects
// keep upstream's raw delimiter.
func TestZshHdocDollarSglUnsupported(t *testing.T) {
	for _, delim := range []string{
		`$'E\cAF'`, `$'E\C-aF'`, `$'E\M-aF'`, `$'E\x00F'`, `$'E\0F'`,
		`$'E\400F'`, `$'E\xgF'`, `$'E\ugF'`, `$'E\uD800F'`,
	} {
		src := "cat <<" + delim + "\nbody\n"
		_, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
		want := "unsupported escape in here-document delimiter"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want %s", src, err, want)
		}
	}
	src := "cat <<$'E\\x4fF'\nbody\nE\\x4fF\necho after\n"
	f, err := NewParser(Variant(LangBash)).Parse(strings.NewReader(src), "")
	if err != nil || len(f.Stmts) != 2 {
		t.Fatalf("bash: %v, want the raw delimiter to close the body", err)
	}
}
