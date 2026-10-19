package typedjson_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax/typedjson"
)

func TestRoundtripNestedAssignmentTarget(t *testing.T) {
	const src = `a[${b[(ie)${c[d]}]}]=()` + "\n"
	file, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := typedjson.Encode(&encoded, file); err != nil {
		t.Fatal(err)
	}
	decoded, err := typedjson.Decode(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(file, decoded) {
		t.Fatal("typed JSON lost nested assignment children or positions")
	}
	var printed bytes.Buffer
	if err := syntax.NewPrinter().Print(&printed, decoded); err != nil || printed.String() != src {
		t.Fatalf("printed assignment = %q, %v", printed.String(), err)
	}
}
