package syntax

import (
	"strings"
	"testing"
)

// A condition is not a command-substitution boundary: its closing ]] must
// expose pending outer here-documents before reading the following newline
// (#701). Keep a reused parser's state and multiple documents covered too.
func TestZshHdocBeforeCondition(t *testing.T) {
	parser := NewParser(Variant(LangZsh))
	for _, tc := range []struct {
		name, prefix, condition, bodies string
		want                            []string
	}{
		{"simple", "cat <<EOF", "[[ x == x ]]", "body\nEOF\n", []string{"body\n"}},
		{"quoted", "cat <<'EOF'", "[[ x == x ]]", "$literal\nEOF\n", []string{"$literal\n"}},
		// Hdoc retains source tabs, including the indentation before ClosePos.
		{"tab-stripped", "cat <<-EOF", "[[ x == x ]]", "	body\n	EOF\n", []string{"	body\n	"}},
		{"multiple", "cat <<A <<B", "[[ x == x ]]", "one\nA\ntwo\nB\n", []string{"one\n", "two\n"}},
		{"nested-substitution", "print $(cat <<EOF", "[[ x == x ]]", "body\nEOF\n)\n", []string{"body\n"}},
		{"following-command", "cat <<EOF", "[[ x == x ]]; echo same-line", "body\nEOF\n", []string{"body\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.prefix + "; " + tc.condition + "\n" + tc.bodies + "echo after\n"
			f, err := parser.Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatal(err)
			}
			var docs []*Redirect
			Walk(f, func(n Node) bool {
				if r, ok := n.(*Redirect); ok && (r.Op == Hdoc || r.Op == DashHdoc) {
					docs = append(docs, r)
				}
				return true
			})
			if len(docs) != len(tc.want) {
				t.Fatalf("got %d documents, want %d", len(docs), len(tc.want))
			}
			for i, r := range docs {
				if r.Hdoc == nil || !r.ClosePos.IsValid() {
					t.Fatalf("document %d lost its body or closer", i)
				}
				if got, want := r.ClosePos.Line(), uint(3+2*i); got != want {
					t.Fatalf("document %d closer line = %d, want %d", i, got, want)
				}
				var out strings.Builder
				if err := NewPrinter().Print(&out, r.Hdoc); err != nil {
					t.Fatal(err)
				}
				if out.String() != tc.want[i] {
					t.Fatalf("document %d body = %q, want %q", i, out.String(), tc.want[i])
				}
			}
			last := f.Stmts[len(f.Stmts)-1].Cmd.(*CallExpr)
			if got := last.Args[0].Lit(); got != "echo" {
				t.Fatalf("last command = %q, want echo", got)
			}
		})
	}
}
