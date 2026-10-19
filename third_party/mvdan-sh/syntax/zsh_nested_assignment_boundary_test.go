package syntax

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// Contrasting contexts for #735: the assignment owns its index, but a command
// substitution inside it starts an independent parsing context.
// https://zsh.sourceforge.io/Doc/Release/Parameters.html#Subscript-Parsing
func TestZshNestedAssignmentBoundaries(t *testing.T) {
	for _, src := range []string{
		`a[${b[(ie)${c[d]}]}]=()`,
		`a[${b[${c[d]}]}]=()`,
		`f() { a[${b[(ie)${c[d]}]}]=(); }`,
		`a[${b[(ie)${c[d]}${e[f]}]}]=()`,
		`a[${b[(ie)pre${c[d]}post]}]=()`,
		`a[${b[(ie)${c[$(print -r -- d)]}]}]=()`,
		`a[${b[(ie)${c["d"]}]}]=()`,
		`a[${b[(ie)"${c[d]}"]}]=()`,
		`a[${b[(ie)${c[d]}\]]}]=()`,
		`a[${b[(ie)${c[d]}[xy]]}]=()`,
		`a[${b[(ie)[xy]${c[d]}]}]=()`,
		`a[${b[(ie)${c[d]},3]}]=()`,
		`a[(ie)${c[d]}]=()`,
	} {
		t.Run(src, func(t *testing.T) {
			src += "\n"
			p := NewParser(Variant(LangZsh))
			f, err := p.Parse(iotest.OneByteReader(strings.NewReader(src)), "")
			if err != nil {
				t.Fatal(err)
			}
			var printed bytes.Buffer
			if err := NewPrinter().Print(&printed, f); err != nil {
				t.Fatal(err)
			}
			again, err := p.Parse(strings.NewReader(printed.String()), "")
			if err != nil {
				t.Fatalf("printed source rejected: %s: %v", &printed, err)
			}
			if diff := cmp.Diff(f, again, cmpopts.IgnoreTypes(Pos{})); diff != "" {
				t.Fatalf("print changed the tree (-want +got):\n%s", diff)
			}
			t.Run("native-source", confirmParse(src, "zsh", false))
			t.Run("native-printed", confirmParse(printed.String(), "zsh", false))
		})
	}
}

func TestZshNestedAssignmentInvalidAndReuse(t *testing.T) {
	p := NewParser(Variant(LangZsh))
	for _, src := range []string{
		`a[${b[(ie)${c[d]}]}=()`,
		`a[${b[(ie)${c[d]}]]=()`,
		`a[${b[(ie)${c[d]}]}]=(`,
		`a[${b[(ie)\${c[d]}]}]=()`,
		`a[${b[(ie){c[d]}]}]=()`,
		`a[${b[(ie){${c[d]}}]}]=()`,
		`a[${b[(ie){a,${c[d]}}]}]=()`,
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := p.Parse(iotest.OneByteReader(strings.NewReader(src+"\n")), ""); err == nil {
				t.Fatal("invalid assignment accepted")
			}
			t.Run("native", confirmParse(src+"\n", "zsh", true))
			f, err := p.Parse(strings.NewReader("a[1]=x\nprint after\n"), "")
			if err != nil || len(f.Stmts) != 2 {
				t.Fatalf("parser state leaked after error: %v", err)
			}
		})
	}
	for _, lang := range []LangVariant{LangBash, LangBats, LangMirBSDKorn} {
		if _, err := NewParser(Variant(lang)).Parse(strings.NewReader(`a[${b[(ie)${c[d]}]}]=()`+"\n"), ""); err == nil {
			t.Fatalf("%s gained Zsh subscript flags", lang)
		}
	}
}

func TestZshNestedAssignmentRemoval(t *testing.T) {
	externalShells[LangZsh].require(t)
	const src = `builtin emulate -R zsh
local -a items=(keep remove tail)
local -A state=(key remove)
items[${items[(ie)${state[key]}]}]=()
print -r -- ${(j:,:)items}
`
	f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatal(err)
	}
	var printed bytes.Buffer
	if err := NewPrinter().Print(&printed, f); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"source": src, "printed": printed.String()} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "removal.zsh")
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "zsh", "-f", path)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir)
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != "keep,tail\n" {
				t.Fatalf("deletion = %q, %v", out, err)
			}
		})
	}
}
