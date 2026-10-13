package syntax

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// The reading of `${##...}` depends on the byte after the second `#`, which a
// slow reader may not have delivered yet (#659). Each source passes
// `zsh -f -n`; Zsh 5.9.2 prints `$#` with the prefix removed for every form
// but `${##}`, which is the length of `$#`.
func TestZshHashParamPrefixRemoval(t *testing.T) {
	tests := []struct {
		src    string
		length bool
		op     ParExpOperator
		word   string
	}{
		{"print ${##x}\n", false, RemSmallPrefix, "x"},
		{"print \"${##x}\"\n", false, RemSmallPrefix, "x"},
		{"print ${###1}\n", false, RemLargePrefix, "1"},
		{"print ${##*}\n", false, RemSmallPrefix, "*"},
		{"print ${##$y}\n", false, RemSmallPrefix, "$y"},
		{"print ${##?}\n", false, RemSmallPrefix, "?"},
		{"print ${##}\n", true, 0, ""},
		{"print ${#x}\n", true, 0, ""},
	}
	for _, tc := range tests {
		for _, slow := range []bool{false, true} {
			var r io.Reader = strings.NewReader(tc.src)
			if slow {
				r = iotest.OneByteReader(r)
			}
			f, err := NewParser(Variant(LangZsh)).Parse(r, "")
			if err != nil {
				t.Errorf("%q slow=%v: %v", tc.src, slow, err)
				continue
			}
			var pe *ParamExp
			Walk(f, func(node Node) bool {
				if p, ok := node.(*ParamExp); ok && pe == nil {
					pe = p
				}
				return true
			})
			if pe == nil {
				t.Fatalf("%q: no parameter expansion", tc.src)
			}
			if pe.Length != tc.length {
				t.Errorf("%q slow=%v: Length=%v, want %v", tc.src, slow, pe.Length, tc.length)
			}
			if tc.length {
				continue
			}
			if pe.Param == nil || pe.Param.Value != "#" || pe.Exp == nil || pe.Exp.Op != tc.op {
				t.Errorf("%q slow=%v: got %#v, want parameter # with %v", tc.src, slow, pe, tc.op)
				continue
			}
			if got := tc.src[pe.Exp.Word.Pos().Offset():pe.Exp.Word.End().Offset()]; got != tc.word {
				t.Errorf("%q slow=%v: removal word %q, want %q", tc.src, slow, got, tc.word)
			}
		}
	}
}
