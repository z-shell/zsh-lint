package syntax

import (
	"strings"
	"testing"
)

// TestZshPipeAcrossSeparators checks that a `|` or `|&` takes the first
// command after any `;` and newline separators as its right operand, as Zsh's
// par_pline skips every separator token before reading it (zsh-lint #553).
// `print a | ; cat` is the pipeline `print a | cat`, which Zsh deparses
// that way and which prints `a` when it runs.
func TestZshPipeAcrossSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, src string
		want      string // the first statement, printed on one line
	}{
		{"same line", "print a | ; cat\n", "print a | cat"},
		{"newline semicolon newline", "print a |\n;\ncat\n", "print a | cat"},
		{"stderr pipe", "print a |& ; cat\n", "print a |& cat"},
		{"two semicolons", "print a | ; ; cat\n", "print a | cat"},
		{"semicolons on their own lines", "print a |\n;\n;\ncat\n", "print a | cat"},
		{"blank line and comment", "print a | ;\n\n# c\ncat\n", "print a | cat"},
		{"chain", "print a | ; cat | ; wc -l\n", "print a | cat | wc -l"},
		{"and-or after", "print a | ; cat && print c\n", "print a | cat && print c"},
		{"and-or before", "print a && print b | ; cat\n", "print a && print b | cat"},
		{"negated", "! print a | ; cat\n", "! print a | cat"},
		{"compound operand", "print a | ; { cat; }\n", "print a | { cat; }"},
		{"redirect first", "print a | ; >/dev/null cat\n", "print a | >/dev/null cat"},
		{"background", "print a | ; cat &\n", "print a | cat &"},
		{"here-document before the separator", "cat <<E | ;\nbody\nE\ncat\n", "cat <<E | cat"},
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

// TestZshPipeAcrossSeparatorsInBodies checks the same join inside compound
// commands and substitutions, where the list is a body rather than the file.
func TestZshPipeAcrossSeparatorsInBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, src string
		body      func(*File) []*Stmt
	}{
		{"function body", "f() { print a |\n;\ncat\n}\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*FuncDecl).Body.Cmd.(*Block).Stmts
		}},
		{"brace group", "{ print a | ; cat; }\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*Block).Stmts
		}},
		{"subshell", "( print a | ; cat )\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*Subshell).Stmts
		}},
		{"then body", "if true; then print a | ; cat; fi\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*IfClause).Then
		}},
		{"case item", "case a in a) print a | ; cat ;; esac\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*CaseClause).Items[0].Stmts
		}},
		{"command substitution", "echo $(print a | ; cat)\n", func(f *File) []*Stmt {
			return f.Stmts[0].Cmd.(*CallExpr).Args[1].Parts[0].(*CmdSubst).Stmts
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
			if b, ok := stmts[0].Cmd.(*BinaryCmd); !ok || (b.Op != Pipe && b.Op != PipeAll) {
				t.Fatalf("Parse(%q) gave a %T, want a pipe *BinaryCmd", tc.src, stmts[0].Cmd)
			}
		})
	}
}

// TestZshPipeBeforeSeparatorsNeedsOperand checks that a pipe followed only by
// separators and then the end of its list stays an error at the operator.
// Unlike `&&` and `||`, Zsh never lets a pipe dangle, so every row here is a
// native error. The last two rows show that without a `;` nothing changes.
func TestZshPipeBeforeSeparatorsNeedsOperand(t *testing.T) {
	t.Parallel()

	tests := []struct{ src, want string }{
		{"print a | ;\n", "1:9: `|` must be followed by a statement"},
		{"print a |& ;\n", "1:9: `|&` must be followed by a statement"},
		{"print a |\n;\n", "1:9: `|` must be followed by a statement"},
		{"print a | ; ;\n", "1:9: `|` must be followed by a statement"},
		{"{ print a | ; }\n", "1:11: `|` must be followed by a statement"},
		{"( print a | ; )\n", "1:11: `|` must be followed by a statement"},
		{"if true; then print a | ; fi\n", "1:23: `|` must be followed by a statement"},
		{"if true; then print a | ; else :; fi\n", "1:23: `|` must be followed by a statement"},
		{"if print a | ; then :; fi\n", "1:12: `|` must be followed by a statement"},
		{"for x in a; do print a | ; done\n", "1:24: `|` must be followed by a statement"},
		{"foreach x (a)\nprint a | ;\nend\n", "2:9: `|` must be followed by a statement"},
		{"case a in a) print a | ; ;; esac\n", "1:22: `|` must be followed by a statement"},
		{"case a in a) print a | ; esac\n", "1:22: `|` must be followed by a statement"},
		{"print `print a | ; `\n", "1:16: `|` must be followed by a statement"},
		{"print a | ; &\n", "1:9: `|` must be followed by a statement"},
		{"print a | ; && cat\n", "1:9: `|` must be followed by a statement"},
		{"print a | ; | cat\n", "1:9: `|` must be followed by a statement"},
		{"{ print a | }\n", "1:13: `}` can only be used to close a block"},
		{"print a |\n", "1:9: `|` must be followed by a statement"},
	}
	p := NewParser(Variant(LangZsh))
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			_, err := p.Parse(strings.NewReader(tc.src), "")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want %q", tc.src, tc.want)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("Parse(%q) error %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestPipeAcrossSeparatorsDialectGate checks that only Zsh joins a pipe
// across a separator: Bash, POSIX and mksh reject `print a | ; cat` at the
// operator.
func TestPipeAcrossSeparatorsDialectGate(t *testing.T) {
	t.Parallel()

	for _, lang := range []LangVariant{LangBash, LangPOSIX, LangMirBSDKorn} {
		t.Run(lang.String(), func(t *testing.T) {
			for _, src := range []string{"print a | ; cat\n", "{ print a | ; cat; }\n"} {
				_, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), "")
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want an error", src)
				}
				if want := "`|` must be followed by a statement"; !strings.Contains(err.Error(), want) {
					t.Errorf("Parse(%q) error %q, want it to contain %q", src, err.Error(), want)
				}
			}
		})
	}
}

// TestZshPipeBeforeSeparatorsRecovers checks that error recovery treats a
// Zsh pipe before separators and a list end as it treats one at the end of
// input: one missing right operand, recovered in place.
func TestZshPipeBeforeSeparatorsRecovers(t *testing.T) {
	t.Parallel()

	for _, src := range []string{"print a | ;\n", "{ print a |& ; }\n"} {
		t.Run(src, func(t *testing.T) {
			f, err := NewParser(Variant(LangZsh), RecoverErrors(3)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatalf("Parse(%q) with recovery failed: %v", src, err)
			}
			missing := 0
			Walk(f, func(n Node) bool {
				if b, ok := n.(*BinaryCmd); ok && b.Y != nil && b.Y.Pos() == recoveredPos {
					missing++
				}
				return true
			})
			if missing != 1 {
				t.Errorf("Parse(%q) recovered %d right operands, want 1", src, missing)
			}
		})
	}
}
