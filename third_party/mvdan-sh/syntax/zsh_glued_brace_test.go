package syntax

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func TestZshGluedBraceWordAPI(t *testing.T) {
	for _, test := range []struct {
		src  string
		want []string
	}{
		{`{a,b}`, []string{`{a,b}`}},
		{`foo {a,b}`, []string{`foo`, `{a,b}`}},
		{"foo\n{a,b}", []string{`foo`, `{a,b}`}},
		{`${v:-x} {a,b}`, []string{"", `{a,b}`}},
		{`$({print hi}) {a,b}`, []string{"", `{a,b}`}},
	} {
		t.Run(test.src, func(t *testing.T) {
			p := NewParser(Variant(LangZsh))
			var got []string
			for word, err := range p.WordsSeq(strings.NewReader(test.src)) {
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, word.Lit())
				if strings.HasPrefix(test.src, "$(") && len(got) == 1 {
					sub := word.Parts[0].(*CmdSubst)
					if _, ok := sub.Stmts[0].Cmd.(*Block); !ok {
						t.Fatal("command substitution lost its glued group")
					}
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("words=%q, want %q", got, test.want)
			}
			file, err := p.Parse(strings.NewReader(`{print hi}`), "")
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := file.Stmts[0].Cmd.(*Block); !ok {
				t.Fatal("word-only context leaked into Parse")
			}
		})
	}
}

// Zsh recognizes an opening brace in command position and a trailing closing
// brace outside assignment values, even when glued to a word (#276).
// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Reserved-Words
func TestZshGluedBraces(t *testing.T) {
	for _, src := range []string{
		`{print hi}`, `{ print hi}`, `{print "$value"}`, `{print ${value:-hi}}`,
		`{print $(print hi)}`, `{print a{b,c}}`, `{print {a,b}}`,
		`{print a}b}`, `{print "{"}`, `{print \}}`, `{print a${value}b}`,
		`{print hi}}`, `{print hi}>out`, `{print hi} | cat`,
	} {
		t.Run(src, func(t *testing.T) {
			f, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src+"\nprint after\n"), "")
			if err != nil {
				t.Fatal(err)
			}
			stmt := f.Stmts[0]
			if pipe, ok := stmt.Cmd.(*BinaryCmd); ok {
				stmt = pipe.X
			}
			block, ok := stmt.Cmd.(*Block)
			if !ok || len(block.Stmts) != 1 || block.Lbrace.Offset() != 0 || block.Rbrace.Offset() != uint(strings.LastIndex(src, "}")) {
				t.Fatalf("block or source positions changed: %#v", stmt.Cmd)
			}
			if len(f.Stmts) != 2 || f.Stmts[1].Pos().Offset() != uint(len(src)+1) {
				t.Fatal("following statement lost or moved")
			}
			var out bytes.Buffer
			if err := NewPrinter().Print(&out, f); err != nil {
				t.Fatal(err)
			}
			if _, err := NewParser(Variant(LangZsh)).Parse(&out, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, src := range []string{
		`if true; then {print hi} fi`, `repeat 2 {print hi}`, `while false; {print hi}`,
		`f() {print hi}`, `function f() {print hi}`, `true || {print hi}`,
	} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), ""); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestZshGluedBraceBuffersAndReuse(t *testing.T) {
	p := NewParser(Variant(LangZsh))
	for _, pad := range []int{0, bufSize - 2, bufSize - 1, bufSize, bufSize * 2} {
		for _, slow := range []bool{false, true} {
			for _, end := range []string{"", "\n"} {
				src := strings.Repeat(" ", pad) + `{print ${x:-hi} $(print hi)}}` + end
				var r io.Reader = strings.NewReader(src)
				if slow {
					r = iotest.OneByteReader(r)
				}
				f, err := p.Parse(r, "")
				if err != nil {
					t.Fatalf("pad=%d slow=%v end=%q: %v", pad, slow, end, err)
				}
				block := f.Stmts[0].Cmd.(*Block)
				if block.Lbrace.Offset() != uint(pad) || block.Rbrace.Offset() != uint(strings.LastIndex(src, "}")) {
					t.Fatal("buffer boundary moved braces")
				}
			}
		}
	}
	if _, err := p.Parse(strings.NewReader(`{print hi`), ""); err == nil {
		t.Fatal("unclosed group accepted")
	}
	if _, err := p.Parse(strings.NewReader(`print {a,b}`), ""); err != nil {
		t.Fatalf("state leaked across Parse: %v", err)
	}
}

func TestZshGluedBraceWords(t *testing.T) {
	for _, src := range []string{
		`print {a,b} a{b,c} a}b a}${value} "}" \}`,
		`"print" {a,b}`, `$command {a,b}`, `"print" {fd}<file`,
		`x=a} y=${value}}`, `{x=a}; print hi}`, `{local x=1}; print hi}`,
		`print {a${value},b} {"a",b} {$(print a),b}`,
		"cat <<EOF\n{print hi}\nEOF\n",
	} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), ""); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	for _, src := range []string{`print hi}`, `{print hi`, `{local x=1}`, `{{print hi}}`, `while false {print hi}`} {
		if _, err := NewParser(Variant(LangZsh)).Parse(strings.NewReader(src), ""); err == nil {
			t.Errorf("accepted invalid source %q", src)
		}
	}
	for _, lang := range []LangVariant{LangBash, LangBats, LangPOSIX, LangMirBSDKorn} {
		f, err := NewParser(Variant(lang)).Parse(strings.NewReader(`{print hi}`), "")
		if err != nil {
			t.Fatal(err)
		}
		call, ok := f.Stmts[0].Cmd.(*CallExpr)
		if !ok || call.Args[0].Lit() != "{print" || call.Args[1].Lit() != "hi}" {
			t.Fatalf("%s literal brace words changed: %#v", lang, f.Stmts[0].Cmd)
		}
	}
}
