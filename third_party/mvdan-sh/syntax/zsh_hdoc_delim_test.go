package syntax

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// Controlled Zsh fixtures, never corpus execution. Keep behavior and printer
// reuse here; shared file rows cover the exact trees and printer options.
func TestZshHdocPrintOwnership(t *testing.T) {
	cases := []string{
		"cat <<'E\nF'; echo x\nbody\n",
		"cat <<$'E\nF'; echo x\nbody\n",

		"cat <<$'E\\nF'; echo x\nbody\n",
		"echo before; cat <<$'E\\nF'; echo x\nbody\n",
		"{ cat <<$'E\\nF'; echo x; }\nbody\n",
		"cat <<$'E\\nF' && echo x\nbody\n",
		"cat <<$'E\\nF' | cat; echo x\nbody\n",
		"cat <<$'E\\nF'; echo x # retained\nbody\n",
		"cat <<A <<$'E\\nF'; echo x\na\nA\nbody\n",
		"cat <<$'E\\nF' <<B; echo x\nbody\n",
		"echo $(cat <<$'E\\nF'; echo hi; echo $(cat <<A\nx\nA\n))\necho after\n",
		"echo `echo \\`cat <<$'E\\nF'\nbody\n\\``\necho after\n",
		"echo `echo \\`cat <<$'E\\nF'\\``\n",
		"echo $(echo `cat <<$'E\\nF'\nbody\n`)\necho after\n",
		"cat <<-$'\\tEOF'\n\tbody\n\tEOF\necho after\n",
		"cat <<-$'\\t'\n\tbody\n\t\necho after\n",
		"echo ordinary\necho after\n",
		"echo \"`cat <<$'E\\nF'\nbody \"quoted\"\n`\"\necho after\n",
		"print -r -- `cat <<$'E\\nF'\nbody\\\\x \\\\$x\n`\necho after\n",
		"echo `echo \"literal\"; cat <<$'E\\nF'\nbody\n`\necho after\n",

		"cat <<A\nfirst\nA\ncat <<$'E\\nF'; echo x\nsecond\n",
		"echo `echo \\`cat <<$'E\\nF'\ninner\n\\`; cat <<$'G\\nH'\nouter\n`\necho after\n",
		"echo `echo '\\\\$x'; cat <<$'E\\nF'\nbody\\\\x\n`\necho after\n",
	}
	native := func(t *testing.T, src string, run bool) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "probe.zsh")
		if err := os.WriteFile(path, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args := []string{"-f", "-n", path}
		if run {
			args = []string{"-f", path}
		}
		cmd := exec.CommandContext(ctx, "zsh", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil || (!run && len(out) != 0) {
			t.Fatalf("native %v: %s; source %q", err, out, src)
		}
		return string(out)
	}
	printer := NewPrinter()
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			native(t, src, false)
			want := native(t, src, true)
			for pass := 0; pass < 2; pass++ {
				f, err := NewParser(Variant(LangZsh), KeepComments(true)).Parse(strings.NewReader(src), "")
				if err != nil {
					t.Fatal(err)
				}
				var out strings.Builder
				if err := printer.Print(&out, f); err != nil {
					t.Fatal(err)
				}
				printed := out.String()
				fresh, err := strPrint(NewPrinter(), f)
				if err != nil || fresh != printed {
					t.Fatalf("reused printer differs: %v; fresh %q; reused %q", err, fresh, printed)
				}
				if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(printed), ""); err != nil {
					t.Fatalf("reparse %v: %q", err, printed)
				}
				native(t, printed, false)
				if got := native(t, printed, true); got != want {
					t.Fatalf("want %q, got %q; print %q", want, got, printed)
				}
				// Print also supports a compound Command as the root, without
				// its enclosing File or Stmt to select lexical list layout.
				if len(f.Stmts) == 1 {
					if block, ok := f.Stmts[0].Cmd.(*Block); ok {
						var command strings.Builder
						if err := printer.Print(&command, block); err != nil {
							t.Fatal(err)
						}
						native(t, command.String(), false)
						if got := native(t, command.String(), true); got != want {
							t.Fatalf("command root: want %q, got %q; print %q", want, got, command.String())
						}
					}
				}
				src = printed
			}
		})
	}
}

// Zsh strips tabs from the decoded word as well as document lines. Other
// dialects retain their existing stop words and must not gain Zsh metadata.
func TestZshHdocTabStop(t *testing.T) {
	for _, delim := range []string{`$'\tEOF'`, "'\tEOF'", "\"\tEOF\"", ` $'\t\tEOF'`, `$'\t'`, "'\t'"} {
		for _, indent := range []string{"", "\t", "\t\t"} {
			for _, op := range []string{"<<", "<<-"} {
				stop := "EOF"
				if delim == `$'\t'` || delim == "'\t'" {
					stop = ""
				}
				if op == "<<" {
					stop = "\t" + stop
					if strings.Contains(delim, `\t\t`) {
						stop = "\t" + stop
					}
					if indent != "" {
						continue
					} // << requires the exact stop word.
				}
				src := "cat " + op + delim + "\n\tbody\n" + indent + stop + "\necho after\n"
				t.Run(src, func(t *testing.T) {
					t.Run("native", confirmParse(src, "zsh", false))
					f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
					if err != nil {
						t.Fatal(err)
					}
					if len(f.Stmts) != 2 {
						t.Fatalf("got %d statements", len(f.Stmts))
					}
					r := f.Stmts[0].Redirs[0]
					if r.ClosePos.Line() != 3 || r.ClosePos.Col() != uint(len(indent)+1) {
						t.Fatalf("ClosePos = %v", r.ClosePos)
					}
					if got := src[r.Word.Pos().Offset():r.Word.End().Offset()]; got != strings.TrimLeft(delim, " ") {
						t.Fatalf("opener = %q", got)
					}
					for pass := 0; pass < 2; pass++ {
						out, err := strPrint(NewPrinter(), f)
						if err != nil {
							t.Fatal(err)
						}
						f, err = NewParser(Variant(LangZsh)).Parse(strings.NewReader(out), "")
						if err != nil || len(f.Stmts) != 2 {
							t.Fatalf("reparse %v: %q", err, out)
						}
					}
				})
			}
		}
	}
	for _, lang := range []LangVariant{LangBash, LangPOSIX, LangMirBSDKorn} {
		src := "cat <<-'\tEOF'\n\tbody\nEOF\necho after\n"
		f, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), "")
		if lang == LangMirBSDKorn {
			if err == nil {
				t.Fatal("mksh accepted an unclosed document")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Stmts) != 1 || f.Stmts[0].Redirs[0].HdocStop != "" || f.Stmts[0].Redirs[0].ClosePos.IsValid() {
			t.Fatalf("%s gained Zsh tab stripping", lang)
		}
	}
}
