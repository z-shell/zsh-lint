package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseNativeAssociativeSubscriptKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "punctuated assignment key",
			src:  "ZI[annex-before-load:new-@]=value\n",
			want: "annex-before-load:new-@",
		},
		{
			name: "leading dot expansion key",
			src:  "print -r -- ${functions[.foo]}\n",
			want: ".foo",
		},
		{
			name: "leading at expansion key",
			src:  "print -r -- ${functions[@zi-register-annex]}\n",
			want: "@zi-register-annex",
		},
		{
			name: "leading at expansion key foo",
			src:  "print -r -- ${functions[@foo]}\n",
			want: "@foo",
		},
		{
			name: "leading at hash expansion key",
			src:  "print -r -- ${myhash[@zi]}\n",
			want: "@zi",
		},
		{
			name: "annex guard parameter expansion",
			src:  "(( ${+functions[@zi-register-annex]} ))\n",
			want: "@zi-register-annex",
		},
		{
			name: "leading angle expansion key",
			src:  "x=${map[<a>]}\n",
			want: "<a>",
		},
		{
			name: "leading angle assignment key",
			src:  "map[<styles>_free]=1\n",
			want: "<styles>_free",
		},
		{
			name: "hyphen before dot assignment key",
			src:  "ZI[bkp-.]=\"${functions[.]}\"\n",
			want: "bkp-.",
		},
		{
			name: "hyphen before dot expansion key",
			src:  "(( ${+ZI[bkp-.]} )) && print -r -- ${ZI[bkp-.]}\n",
			want: "bkp-.",
		},
		{
			name: "trailing hyphen assignment key",
			src:  "A[a-]=x\n",
			want: "a-",
		},
		{
			name: "hyphen before slash assignment key",
			src:  "A[a-/]=x\n",
			want: "a-/",
		},
		{
			name: "trailing angle assignment key",
			src:  "A[a<]=x\n",
			want: "a<",
		},
		{
			name: "dot assignment key",
			src:  "functions[.]=':zi-tmp-subst-source \"$@\";'\n",
			want: "[.]",
		},
		{
			name: "leading dot assignment key",
			src:  "  ZI[.foo]=x\n",
			want: ".foo",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if !strings.Contains(rendered.String(), test.want) {
				t.Errorf("printed AST = %q, want original subscript key %q", rendered.String(), test.want)
			}
		})
	}
}

func TestAssociativeSubscriptKeyWordKeepsPositions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		key  string
		col  uint
	}{
		{name: "leading angle expansion", src: "x=${map[<a>]}\n", key: "<a>", col: 9},
		{name: "hyphen before dot assignment", src: "ZI[bkp-.]=x\n", key: "bkp-.", col: 4},
		{name: "hyphen before slash assignment", src: "A[a-/]=x\n", key: "a-/", col: 3},
		{name: "leading dot expansion", src: "print ${functions[.foo]}\n", key: ".foo", col: 19},
		{name: "dot assignment", src: "  functions[.]=x\n", key: ".", col: 13},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var indexes []syntax.ArithmExpr
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				switch n := node.(type) {
				case *syntax.ParamExp:
					if n.Index != nil {
						indexes = append(indexes, n.Index)
					}
				case *syntax.Assign:
					if n.Index != nil {
						indexes = append(indexes, n.Index)
					}
				}
				return true
			})
			if len(indexes) != 1 {
				t.Fatalf("subscripts = %d, want 1", len(indexes))
			}
			word, ok := indexes[0].(*syntax.Word)
			if !ok || len(word.Parts) != 1 {
				t.Fatalf("Index = %T, want *syntax.Word with one part", indexes[0])
			}
			lit, ok := word.Parts[0].(*syntax.Lit)
			if !ok {
				t.Fatalf("Index part = %T, want *syntax.Lit", word.Parts[0])
			}
			if lit.Value != test.key {
				t.Errorf("Index literal = %q, want %q", lit.Value, test.key)
			}
			if lit.Pos().Line() != 1 || lit.Pos().Col() != test.col {
				t.Errorf("Index position = %d:%d, want 1:%d", lit.Pos().Line(), lit.Pos().Col(), test.col)
			}
			if got := lit.End().Col(); got != test.col+uint(len(test.key)) {
				t.Errorf("Index end column = %d, want %d", got, test.col+uint(len(test.key)))
			}
		})
	}
}

func TestArithmeticSubscriptKeepsOperators(t *testing.T) {
	for _, src := range []string{"a[i-1]=x\n", "x=${a[i<j]}\n", "x=${a[i/2]}\n"} {
		file, err := Parse(strings.NewReader(src), "arithmetic.zsh")
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		var binary int
		syntax.Walk(file.AST(), func(node syntax.Node) bool {
			if _, ok := node.(*syntax.BinaryArithm); ok {
				binary++
			}
			return true
		})
		if binary != 1 {
			t.Errorf("Parse(%q) binary arithmetic nodes = %d, want 1", src, binary)
		}
	}
}

func TestAssociativeSubscriptCompatibilityPreservesLaterErrorPosition(t *testing.T) {
	const src = "print -r -- ${functions[.foo]}\n)\n"
	_, err := Parse(strings.NewReader(src), "later-error.zsh")
	if err == nil {
		t.Fatal("Parse() unexpectedly accepted a trailing unmatched parenthesis")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	if parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != 1 {
		t.Errorf("error position = %d:%d, want 2:1", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
}

func TestAssociativeSubscriptCompatibilityRejectsMalformedKeys(t *testing.T) {
	for _, src := range []string{
		"print -r -- ${functions[.foo}\n",
		"ZI[annex-before-load:new-@=value\n",
		"x=${map[<a>}\n",
		"functions[.=x\n",
		"(( < 1 ))\n",
	} {
		if _, err := Parse(strings.NewReader(src), "malformed.zsh"); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", src)
		}
	}
}
