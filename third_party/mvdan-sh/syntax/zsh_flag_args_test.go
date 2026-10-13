package syntax

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// The flags group of `${(flags)name}` ends at the `)` that follows the last
// flag, not at the first `)` in the group (#644). Zsh's paramsubst reads the
// argument of `I`, `g`, `j`, `s`, `Z` and `_`, and up to three arguments of
// `l` and `r`, with get_strarg: the argument runs to the first closing
// delimiter, which is the partner of `(`, `[`, `{` or `<` and otherwise the
// opening rune itself, without nesting. Each source below sits in a function
// body, where `zsh -f -n` checks only the lexer (#287); every one passes it
// and Zsh 5.9.2 runs each with the expected result.
// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion-Flags
func TestZshParamFlagArgs(t *testing.T) {
	tests := []struct {
		expansion string
		flags     string
		param     string
	}{
		{"${(j(|))w}", "j(|)", "w"},
		{"${(s())w}", "s()", "w"},
		{"${(j(][))w}", "j(][)", "w"},
		{"${(l.(5)..x.)w}", "l.(5)..x.", "w"},
		{"${(l(5)(x)(y))w}", "l(5)(x)(y)", "w"},
		{"${(l.((${COLUMNS}-5))..-.)w}", "l.((${COLUMNS}-5))..-.", "w"},
		{"${(pl:$(((c - r) / 2)):: :)w}", "pl:$(((c - r) / 2)):: :", "w"},
		{"${(l.${#d[1,(r)p]}..?.)w}", "l.${#d[1,(r)p]}..?.", "w"},
		{"${(j.).)w}", "j.).", "w"},
		{"${(j.(.)w}", "j.(.", "w"},
		{"${(j.')'.)w}", "j.')'.", "w"},
		{"${(j[|])w}", "j[|]", "w"},
		{"${(j<)>)w}", "j<)>", "w"},
		{"${(I.2.)w}", "I.2.", "w"},
		{"${(g:o:)w}", "g:o:", "w"},
		{"${(Z:n:)w}", "Z:n:", "w"},
		{"${(l:3::x:)w}", "l:3::x:", "w"},
		{"${(l:3:.x.)w}", "l:3:.x.", "w"},
		{"${(@)w}", "@", "w"},
		{"${(oi)w}", "oi", "w"},
		{"${(j: :)${(f)x}}", "j: :", ""},
	}
	for _, tc := range tests {
		for _, quoted := range []bool{false, true} {
			word := tc.expansion
			if quoted {
				word = `"` + word + `"`
			}
			src := "f() {\n  print -r -- " + word + "\n}\n"
			for mode, wrap := range flagArgReaders {
				f, err := NewParser(Variant(LangZsh)).Parse(wrap(strings.NewReader(src)), "")
				if err != nil {
					t.Errorf("%q %s: %v", src, mode, err)
					continue
				}
				var pe *ParamExp
				Walk(f, func(node Node) bool {
					if p, ok := node.(*ParamExp); ok && pe == nil {
						pe = p
					}
					return true
				})
				if pe == nil || pe.Flags == nil {
					t.Errorf("%q %s: no flagged parameter expansion", src, mode)
					continue
				}
				if pe.Flags.Value != tc.flags {
					t.Errorf("%q %s: flags %q, want %q", src, mode, pe.Flags.Value, tc.flags)
				}
				if tc.param != "" && (pe.Param == nil || pe.Param.Value != tc.param) {
					t.Errorf("%q %s: param %#v, want %q", src, mode, pe.Param, tc.param)
				}
				if got := src[pe.Flags.Pos().Offset():pe.Flags.End().Offset()]; got != tc.flags {
					t.Errorf("%q %s: flags position covers %q, want %q", src, mode, got, tc.flags)
				}
			}
		}
	}
}

// A group whose arguments do not all close before its `)` keeps the earlier
// reading, which ends the group at its first `)`. Each source passes
// `zsh -f -n` in a function body and fails with "error in flags" only when
// it runs, except `${(j.}.)w}`, where a `}` in the argument ends the
// expansion and `zsh -f -n` reports a parse error; the base accepted that one
// too, so it stays a parked false accept rather than a new one.
func TestZshParamFlagArgsFallback(t *testing.T) {
	tests := []struct {
		src, flags, param string
	}{
		{"f() { print ${(j)x}; }\n", "j", "x"},
		{"f() { print ${(s.x)w}; }\n", "s.x", "w"},
		{"f() { print ${(_.x)w}; }\n", "_.x", "w"},
		{"f() { print ${(l(5)w}; }\n", "l(5", "w"},
		{"f() { print ${(j.}.)w}; }\n", "j.}.", "w"},
	}
	for _, tc := range tests {
		for mode, wrap := range flagArgReaders {
			f, err := NewParser(Variant(LangZsh)).Parse(wrap(strings.NewReader(tc.src)), "")
			if err != nil {
				t.Errorf("%q %s: %v", tc.src, mode, err)
				continue
			}
			var pe *ParamExp
			Walk(f, func(node Node) bool {
				if p, ok := node.(*ParamExp); ok && pe == nil {
					pe = p
				}
				return true
			})
			if pe == nil || pe.Flags == nil || pe.Flags.Value != tc.flags || pe.Param == nil || pe.Param.Value != tc.param {
				t.Errorf("%q %s: got %#v, want flags %q and param %q", tc.src, mode, pe, tc.flags, tc.param)
			}
		}
	}
	const eof = "print ${(j.x"
	for mode, wrap := range flagArgReaders {
		_, err := NewParser(Variant(LangZsh)).Parse(wrap(strings.NewReader(eof)), "")
		if want := "1:9: reached EOF without matching `(` with `)`"; err == nil || err.Error() != want {
			t.Errorf("%q %s: error %v, want %q", eof, mode, err, want)
		}
	}
}

// flagArgReaders deliver the source whole, a byte at a time and in halves,
// so the group lookahead crosses input buffers (#644).
var flagArgReaders = map[string]func(io.Reader) io.Reader{
	"whole":   func(r io.Reader) io.Reader { return r },
	"onebyte": iotest.OneByteReader,
	"half":    iotest.HalfReader,
}
