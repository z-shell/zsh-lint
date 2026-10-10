package syntax

import (
	"strings"
	"testing"
)

// TestZshAndOrAcrossSeparators checks that a `&&` or `||` takes the first
// statement after any `;` and newline separators as its right operand, as
// Zsh's par_sublist skips every separator token before reading it
// (zsh-lint #548). `false && ; print b` is the sublist `false && print b`,
// and Zsh prints nothing when it runs.
func TestZshAndOrAcrossSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, src string
		want      string // the first statement, printed on one line
	}{
		{"newline semicolon newline", "false &&\n;\nprint b\n", "false && print b"},
		{"same line", "false && ; print b\n", "false && print b"},
		{"or", "true ||\n;\nprint b\n", "true || print b"},
		{"two semicolons", "false && ; ; print b\n", "false && print b"},
		{"semicolons on their own lines", "false &&\n;\n;\nprint b\n", "false && print b"},
		{"blank line and comment", "false && ;\n\n# c\nprint b\n", "false && print b"},
		{"chain", "false && ; print a && ; print b\n", "false && print a && print b"},
		{"mixed chain", "false && ; true || ; print c\n", "false && true || print c"},
		{"pipeline operand", "false && ; print b | cat\n", "false && print b | cat"},
		{"compound operand", "false && ; { print b; }\n", "false && { print b; }"},
		{"here-document before the separator", "cat <<E && ;\nbody\nE\nprint x\n", "cat <<E && print x"},
	}
	p := NewParser(Variant(LangZsh))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := p.Parse(strings.NewReader(tc.src), "")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			if len(f.Stmts) != 1 {
				t.Fatalf("Parse(%q) gave %d statements, want 1", tc.src, len(f.Stmts))
			}
			if _, ok := f.Stmts[0].Cmd.(*BinaryCmd); !ok {
				t.Fatalf("Parse(%q) gave a %T, want a *BinaryCmd", tc.src, f.Stmts[0].Cmd)
			}
			var sb strings.Builder
			if err := NewPrinter(SingleLine(true)).Print(&sb, f.Stmts[0]); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			if got := strings.TrimSpace(sb.String()); !strings.HasPrefix(got, tc.want) {
				t.Errorf("printed %q, want it to start with %q", got, tc.want)
			}
		})
	}
}

// TestZshAndOrAcrossSeparatorsInBodies checks the same join inside compound
// commands, where the list is a body rather than the file.
func TestZshAndOrAcrossSeparatorsInBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, src string
		body      func(*File) []*Stmt
	}{
		{"function body", "f() { false &&\n;\nprint b\n}\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*FuncDecl).Body.Cmd.(*Block).Stmts
		}},
		{"brace group", "{ false && ; print b; }\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*Block).Stmts
		}},
		{"subshell", "( false && ; print b )\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*Subshell).Stmts
		}},
		{"if condition", "if false && ; true; then :; fi\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*IfClause).Cond
		}},
		{"then body", "if true; then false && ; print b; fi\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*IfClause).Then
		}},
		{"case item", "case a in a) false && ; print b ;; esac\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*CaseClause).Items[0].Stmts
		}},
	}
	p := NewParser(Variant(LangZsh))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := p.Parse(strings.NewReader(tc.src), "")
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			stmts := tc.body(f)
			if len(stmts) != 1 {
				t.Fatalf("Parse(%q) gave %d statements in the body, want 1", tc.src, len(stmts))
			}
			if _, ok := stmts[0].Cmd.(*BinaryCmd); !ok {
				t.Fatalf("Parse(%q) gave a %T, want a *BinaryCmd", tc.src, stmts[0].Cmd)
			}
		})
	}
}

// TestAndOrAcrossSeparatorsDialectGate checks that only Zsh joins across a
// separator: Bash rejects `false && ; print b` at the operator.
func TestAndOrAcrossSeparatorsDialectGate(t *testing.T) {
	t.Parallel()

	for _, lang := range []LangVariant{LangBash, LangPOSIX, LangMirBSDKorn} {
		t.Run(lang.String(), func(t *testing.T) {
			for _, src := range []string{"false && ; print b\n", "{ false && ; }\n"} {
				_, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), "")
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want an error", src)
				}
				// Upstream reports the operator too: getStmt reads
				// nothing at the `;`.
				if want := "`&&` must be followed by a statement"; !strings.Contains(err.Error(), want) {
					t.Errorf("Parse(%q) error %q, want it to contain %q", src, err.Error(), want)
				}
			}
		})
	}
}
