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
		`a[(ie)${c}$(print x,y)]=()`,
		`a[${b[(ie)${c}$(print x,y)]}]=()`,
		`a[(ie)${c}$(print '[x]')]=()`,
		`a[(ie)${c}$(print xy)]=()`,
		`a[${b[(ie)${c}\[x\]]}]=()`,
		`a[${b[(ie)${c[d]}\[x\]]}]=()`,
		// A target nested inside a command substitution is still owned.
		`a[${b[$(a[${d[(ie)${c[e]}]}]=())]}]=()`,
		// Owned nested patterns without a typed child end at their own `]`.
		`a[${b[(ie)x]}]=()`,
		`a[${b[(ie)[xy]]}]=()`,
		`a[${b[(ie)x]}${c[(ie)y]}]=()`,
		// Inner parsers: a glob-group body and an arithmetic lookahead.
		`print x($(a[${b[(ie)${c[d]}]}]=()))`,
		`print $(( $(a[${b[(ie)${c[d]}]}]=(); print 1) + 1 ))`,
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

// Disabling the typed reading restores the raw pattern of the nested
// subscript, which ends the target at its first inner `]` (#735).
func TestZshNestedAssignTargetsDisabled(t *testing.T) {
	p := NewParser(Variant(LangZsh), ZshNestedAssignTargets(false))
	if _, err := p.Parse(strings.NewReader("a[${b[(ie)${c[d]}]}]=()\n"), ""); err == nil {
		t.Fatal("raw reading accepted the nested assignment target")
	}
	if _, err := p.Parse(strings.NewReader("a[${b[1]}]=x\n"), ""); err != nil {
		t.Fatalf("raw reading rejected a plain nested target: %v", err)
	}
	if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader("a[${b[(ie)${c[d]}]}]=()\n"), ""); err != nil {
		t.Fatalf("typed reading is not the default: %v", err)
	}
}

// ZshNestedAssignRead reports the typed reading only for the last parse that
// entered an owned nested target's flagged pattern (#735).
func TestZshNestedAssignRead(t *testing.T) {
	p := NewParser(Variant(LangZsh))
	for _, tc := range []struct {
		src  string
		read bool
	}{
		{"a[${b[(ie)${c[d]}]}]=()\n", true},
		{"a[${b[(ie)x]}]=()\n", true},
		{"a[${b[(ie)${c[d]}]}=()\n", true},
		{"a[(ie)${c}]=()\n", false},
		{"a[\"${b[(ie)${c}]}\"]=()\n", false},
		{"a[${b[$(a[(ie)${c}]=())]}]=()\n", false},
		{"print ${b[(ie)${c[d]}]}\n", false},
		// An inner glob-group body parser reports to its parent.
		{"print x($(a[${b[(ie)y]}]=()))\n", true},
		{"print x($(a[${b[1]}]=()))\n", false},
		{"print $(( $(a[${b[(ie)y]}]=(); print 1) + 1 ))\n", true},
	} {
		_, _ = p.Parse(strings.NewReader(tc.src), "")
		if got := p.ZshNestedAssignRead(); got != tc.read {
			t.Errorf("%q: ZshNestedAssignRead() = %v, want %v", tc.src, got, tc.read)
		}
	}
	off := NewParser(Variant(LangZsh), ZshNestedAssignTargets(false))
	_, _ = off.Parse(strings.NewReader("a[${b[(ie)${c[d]}]}]=()\n"), "")
	if off.ZshNestedAssignRead() {
		t.Error("disabled typed reading reported as read")
	}
	// The inner body parser keeps the disabled setting.
	_, _ = off.Parse(strings.NewReader("print x($(a[${b[(ie)y]}]=()))\n"), "")
	if off.ZshNestedAssignRead() {
		t.Error("inner body parser read the typed way under the disabled setting")
	}
	_, _ = off.Parse(strings.NewReader("print $(( $(a[${b[(ie)y]}]=(); print 1) + 1 ))\n"), "")
	if off.ZshNestedAssignRead() {
		t.Error("arithmetic lookahead read the typed way under the disabled setting")
	}
}

// Every inner parser the fork builds for a nested reading keeps the
// caller's nested-assignment setting and reports its typed reads back
// (#735).
func TestZshInnerParserCarriesSetting(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		p := NewParser(Variant(LangZsh), ZshNestedAssignTargets(enabled))
		inner := p.zshInnerParser()
		if inner.zshNestedAssignOff == enabled || !inner.lang.in(LangZsh) {
			t.Fatalf("enabled=%v: inner parser lost the setting or dialect", enabled)
		}
		p.zshFoldInner(inner)
		if p.ZshNestedAssignRead() {
			t.Fatalf("enabled=%v: an unused inner parser reported a read", enabled)
		}
		inner.zshAssignRead = true
		p.zshFoldInner(inner)
		inner.zshAssignRead = false
		p.zshFoldInner(inner)
		if !p.ZshNestedAssignRead() {
			t.Fatalf("enabled=%v: folding lost an inner read", enabled)
		}
	}
}

// In an owned nested pattern without a typed child, a `,` inside an open
// literal `[` stays pattern text, while a top-level `,` ends the pattern
// (#735).
func TestZshNestedAssignmentPatternComma(t *testing.T) {
	for _, tc := range []struct{ src, pattern string }{
		{`a[${b[(ie)[x,y]]}]=()`, `[x,y]`},
		{`a[${b[(ie)[[x,y]]]}]=()`, `[[x,y]]`},
		{`a[${b[(ie)x,3]}]=()`, `x`},
		{`a[${b[(ie)[x]y,3]}]=()`, `[x]y`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			src := tc.src + "\n"
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), "")
			if err != nil {
				t.Fatal(err)
			}
			var flags *FlagsArithm
			Walk(f, func(node Node) bool {
				if fa, ok := node.(*FlagsArithm); ok && flags == nil {
					flags = fa
				}
				return flags == nil
			})
			if flags == nil || flags.X == nil {
				t.Fatal("no flagged pattern in the nested target")
			}
			var got bytes.Buffer
			if err := NewPrinter().Print(&got, flags.X); err != nil {
				t.Fatal(err)
			}
			if got.String() != tc.pattern {
				t.Fatalf("pattern = %q, want %q", got.String(), tc.pattern)
			}
			t.Run("native", confirmParse(src, "zsh", false))
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
