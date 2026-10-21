package syntax

import (
	"bytes"
	"strings"
	"testing"
	"testing/iotest"
)

func TestZshAnonymousBraceTailOwnership(t *testing.T) {
	p := NewParser(Variant(LangZsh), KeepComments(true))
	for _, head := range []string{"()", "function"} {
		for _, tail := range []string{" && print tail", " || print tail", " | cat", " |& cat", " > out", " & print tail", " # retained"} {
			source := head + " { :; }" + tail + "\n"
			f, err := p.Parse(iotest.OneByteReader(strings.NewReader(source)), "tail.zsh")
			if err != nil {
				t.Fatalf("%q: %v", source, err)
			}
			stmt := f.Stmts[0]
			if binary, ok := stmt.Cmd.(*BinaryCmd); ok {
				stmt = binary.X
			}
			fd, ok := stmt.Cmd.(*FuncDecl)
			if !ok {
				t.Fatalf("%q: invocation = %T", source, stmt.Cmd)
			}
			if len(fd.Body.Redirs) != 0 {
				t.Fatalf("%q: redirect moved into body", source)
			}
			block, ok := fd.Body.Cmd.(*Block)
			if !ok || int(fd.End().Offset()) != strings.Index(source, "}")+1 {
				t.Fatalf("%q: body includes tail: %#v", source, fd.Body)
			}
			if block.Rbrace.Line() != 1 || int(block.Rbrace.Col()) != strings.Index(source, "}")+1 {
				t.Fatalf("%q: body closer moved", source)
			}
			var out bytes.Buffer
			if err := NewPrinter().Print(&out, f); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Parse(iotest.OneByteReader(strings.NewReader(out.String())), "roundtrip.zsh"); err != nil {
				t.Fatalf("%q: printed %q: %v", source, out.String(), err)
			}
		}
	}
}
