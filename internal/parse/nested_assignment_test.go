package parse

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestNestedAssignmentEscapedOpenerRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-735-escaped-nested-assignment.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(bytes.NewReader(src), "invalid.zsh"); err == nil {
		t.Fatal("escaped nested expansion opener accepted as executable expansion")
	}
	file, err := Parse(strings.NewReader(`a[${b[(ie)${c[d]}]}]=()`+"\n"), "valid.zsh")
	if err != nil || file.AST() == nil {
		t.Fatalf("valid control rejected: %v", err)
	}
}
