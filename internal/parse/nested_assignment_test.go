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
// compatibility repair; only an unquoted `${...}` opened by the target itself
// is typed (#735). Escaped brackets after an expansion stay pattern text, a
// direct target inside a command substitution is still direct, and a
// double-quoted nested target keeps main's path.
func TestDirectAssignmentTargetKeepsRawPattern(t *testing.T) {
	for _, src := range []string{
		`a[(ie)${c}\]]=()`,
		`a[(ie)${c}\[x\]]=()`,
		`a[(ie)x\]]=()`,
		`a[(ie)${c[d]}]=()`,
		`a[(ie)${c}$(print x,y)]=()`,
		`a[${b[$(a[(ie)${c}\[x\]]=())]}]=()`,
		`a[${b[$(a[(ie)x\[x\]]=())]}]=()`,
		`a[$(a[(ie)${c}\[x\]]=())]=()`,
		`a[${b[$(a[${d[(ie)${c[e]}]}]=())]}]=()`,
		`a["${b[(ie)${c}\]]}"]=()`,
		`a["${b[(ie)${c}\[x\]]}"]=()`,
		`a["${b[(ie)x\]]}"]=()`,
		`a["${b[(ie)${c[d]}]}"]=()`,
	} {
		t.Run(src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(src+"\n"), "direct.zsh")
			if err != nil || file.AST() == nil {
				t.Fatalf("native-valid direct target rejected: %v", err)
			}
		})
	}
}

// A file the typed reading rejects is parsed again with the raw reading,
// whose result is returned, so its error is the raw reading's (#735).
func TestNestedAssignmentRejectionKeepsRawError(t *testing.T) {
	for _, src := range []string{
		`a[${b[(ie)${c[d]}]}=()`,
		`a[${b[(ie)${c[d]}]}]=(`,
	} {
		t.Run(src, func(t *testing.T) {
			_, err := Parse(strings.NewReader(src+"\n"), "invalid.zsh")
			if err == nil || err.Error() != "invalid.zsh:1:1: `a[b]` must be followed by `=`" {
				t.Fatalf("error = %v, want the raw reading's missing-`=` error", err)
			}
		})
	}
}
