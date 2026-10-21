package parse

import (
	"strings"
	"testing"
)

// Masking multi-line argument words must also preserve an EOF boundary and
// errors on later physical lines, not only newline-terminated sources (#279).
func TestAnonymousInvocationMultilineEOFPositions(t *testing.T) {
	for _, suffix := range []string{"", "\n"} {
		source := "() { :; } 'first\n\nword' && print tail" + suffix
		file, err := Parse(strings.NewReader(source), "eof.zsh")
		if err != nil {
			t.Fatal(err)
		}
		if int(file.AST().End().Offset()) != len(strings.TrimSuffix(source, "\n")) {
			t.Fatalf("file end moved: %v", file.AST().End())
		}
		if file.AST().End().Line() != 3 {
			t.Fatalf("file end line = %d, want 3", file.AST().End().Line())
		}
	}
}
