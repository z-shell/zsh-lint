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

// A direct assignment target keeps main's raw flag-pattern path and its
// compatibility repair; only a `${...}` nested in the target is typed
// (#735). Escaped brackets after an expansion stay pattern text.
func TestDirectAssignmentTargetKeepsRawPattern(t *testing.T) {
	for _, src := range []string{
		`a[(ie)${c}\]]=()`,
		`a[(ie)${c}\[x\]]=()`,
		`a[(ie)x\]]=()`,
		`a[(ie)${c[d]}]=()`,
		`a[(ie)${c}$(print x,y)]=()`,
	} {
		t.Run(src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(src+"\n"), "direct.zsh")
			if err != nil || file.AST() == nil {
				t.Fatalf("native-valid direct target rejected: %v", err)
			}
		})
	}
}
