package parse

import (
	"bytes"
	"os"
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
	tests := []struct {
		name string
		src  string
		text string
	}{
		{
			name: "unmatched parenthesis after bare key",
			src:  "print -r -- ${functions[.foo]}\n)\n",
			text: "`)` can only be used to close a subshell",
		},
		{
			name: "unterminated if after expansion key",
			src:  "x=${map[${M}:b]}\nif true\n",
			text: "`if <cond>` must be followed by `then`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, []byte(test.src), test.text, 2, 1)
		})
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

// keyPart is one expected part of a subscript Word: a `$` parameter
// expansion or a literal, with the column where it starts.
type keyPart struct {
	param string
	lit   string
	col   uint
}

func TestAssociativeSubscriptKeyKeepsExpansionParts(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		parts []keyPart
	}{
		{
			name:  "bare parameter before colon",
			src:   "x=${map[$M:b]}\n",
			parts: []keyPart{{param: "M", col: 9}, {lit: ":b", col: 11}},
		},
		{
			name:  "braced parameter before trailing colon",
			src:   "x=${map[${M}:]}\n",
			parts: []keyPart{{param: "M", col: 9}, {lit: ":", col: 13}},
		},
		{
			name:  "colon between bare parameters",
			src:   "x=${map[$M:$N]}\n",
			parts: []keyPart{{param: "M", col: 9}, {lit: ":", col: 11}, {param: "N", col: 12}},
		},
		{
			name:  "double colon between braced parameters",
			src:   "x=${map[${M}::${N}]}\n",
			parts: []keyPart{{param: "M", col: 9}, {lit: "::", col: 13}, {param: "N", col: 15}},
		},
		{
			name:  "literal before colon before parameter",
			src:   "x=${map[a:${M}]}\n",
			parts: []keyPart{{lit: "a:", col: 9}, {param: "M", col: 11}},
		},
		{
			name:  "expansion with its own colon inside",
			src:   "print -r -- ${map[${MATCH%:}:]}\n",
			parts: []keyPart{{param: "MATCH", col: 19}, {lit: ":", col: 29}},
		},
		{
			name:  "zi snippet key with positional and nested subscript",
			src:   "print -r -- ${ZI_SNIPPETS[PZT::modules/$1${ICE[svn]-/init.zsh}]}\n",
			parts: []keyPart{{lit: "PZT::modules/", col: 27}, {param: "1", col: 40}, {param: "ICE", col: 42}},
		},
		{
			name:  "leading angle before parameter",
			src:   "x=${map[<styles>_free${style}]-}\n",
			parts: []keyPart{{lit: "<styles>_free", col: 9}, {param: "style", col: 22}},
		},
		{
			name:  "command substitution before colon",
			src:   "x=${map[$(cmd):b]}\n",
			parts: []keyPart{{param: "$(cmd)", col: 9}, {lit: ":b", col: 15}},
		},
		{
			name:  "default value after the subscript",
			src:   "x=${map[$M:b]:-d}\n",
			parts: []keyPart{{param: "M", col: 9}, {lit: ":b", col: 11}},
		},
		{
			name:  "assignment with bare parameter",
			src:   "map[$M:b]=1\n",
			parts: []keyPart{{param: "M", col: 5}, {lit: ":b", col: 7}},
		},
		{
			name:  "assignment with braced parameter",
			src:   "map[${M}:]=1\n",
			parts: []keyPart{{param: "M", col: 5}, {lit: ":", col: 9}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			index := singleSubscriptIndex(t, file.AST())
			word, ok := index.(*syntax.Word)
			if !ok {
				t.Fatalf("Index = %T, want *syntax.Word", index)
			}
			if len(word.Parts) != len(test.parts) {
				t.Fatalf("Index parts = %d, want %d", len(word.Parts), len(test.parts))
			}
			for i, want := range test.parts {
				part := word.Parts[i]
				if got := part.Pos().Col(); got != want.col {
					t.Errorf("part %d column = %d, want %d", i, got, want.col)
				}
				switch {
				case want.lit != "":
					lit, ok := part.(*syntax.Lit)
					if !ok {
						t.Fatalf("part %d = %T, want *syntax.Lit", i, part)
					}
					if lit.Value != want.lit {
						t.Errorf("part %d literal = %q, want %q", i, lit.Value, want.lit)
					}
					if got := lit.End().Col(); got != want.col+uint(len(want.lit)) {
						t.Errorf("part %d end column = %d, want %d", i, got, want.col+uint(len(want.lit)))
					}
				case strings.HasPrefix(want.param, "$("):
					if _, ok := part.(*syntax.CmdSubst); !ok {
						t.Fatalf("part %d = %T, want *syntax.CmdSubst", i, part)
					}
				default:
					exp, ok := part.(*syntax.ParamExp)
					if !ok {
						t.Fatalf("part %d = %T, want *syntax.ParamExp", i, part)
					}
					if exp.Param.Value != want.param {
						t.Errorf("part %d parameter = %q, want %q", i, exp.Param.Value, want.param)
					}
				}
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if got := strings.TrimSuffix(rendered.String(), "\n"); got != strings.TrimSuffix(test.src, "\n") {
				t.Errorf("printed AST = %q, want the original source", got)
			}
		})
	}
}

// singleSubscriptIndex returns the only subscript in the tree, whether it
// belongs to a parameter expansion or to an assignment.
func singleSubscriptIndex(t *testing.T, file *syntax.File) syntax.ArithmExpr {
	t.Helper()
	var indexes []syntax.ArithmExpr
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.ParamExp:
			if n.Index != nil {
				indexes = append(indexes, n.Index)
				return false
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
	return indexes[0]
}

func TestAssociativeSubscriptLeavesUncertainExpansionKeysAlone(t *testing.T) {
	tests := []struct {
		name string
		src  string
		col  uint
	}{
		{name: "backtick substitution", src: "x=${map[`echo`:b]}\n", col: 15},
		{name: "space inside key", src: "x=${map[$M :b]}\n", col: 12},
		{name: "quoted text", src: "x=${map[$M:\"b\"]}\n", col: 11},
		{name: "special parameter", src: "x=${map[$?:b]}\n", col: 11},
		{name: "newline inside key", src: "x=${map[$M:b\n]}\n", col: 11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertParseErrorAt(t, []byte(test.src), invalidSubscriptTernary, 1, test.col)
		})
	}
}

func TestAssociativeSubscriptRejectsUnclosedExpansionKeys(t *testing.T) {
	tests := []struct {
		fixture string
		col     uint
	}{
		{"testdata/invalid-235-unclosed-brace-after-bare-expansion-key.txt", 14},
		{"testdata/invalid-235-unclosed-brace-after-braced-expansion-key.txt", 15},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			assertParseErrorAt(t, src, "not a valid parameter expansion operator: \"\\n\"", 1, test.col)
		})
	}
}
