package parse

import (
	"os"
	"testing"
)

// The parser fork reads a `;` that opens a list as an empty sublist and a
// `&&` or `||` that ends its list as dangling (#238, #297, #332, #548), so
// no adapter handles them. Native Zsh rejects each fixture (`zsh -f -n`), so
// the front end must too, with the parser's error at the original position.
func TestEmptySublistRejectsInvalidSources(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		{"invalid-238-double-semicolon.txt", "`;;` can only be used in a case clause", 1, 15},
		{"invalid-238-ampersand-after-separator.txt", "`&` can only immediately follow a statement", 1, 17},
		{"invalid-238-do-outside-loop.txt", "`do` can only be used in a loop", 1, 1},
		{"invalid-238-unterminated-body.txt", "`while` statement must end with `done`", 1, 1},
		{"invalid-297-double-semicolon.txt", "`;;` can only be used in a case clause", 1, 14},
		{"invalid-297-ampersand-after-separator.txt", "`&` can only immediately follow a statement", 1, 16},
		{"invalid-297-then-outside-if.txt", "`then` can only be used in an `if`", 1, 1},
		{"invalid-297-unterminated-body.txt", "`if` statement must end with `fi`", 1, 1},
		{"invalid-297-else-double-semicolon.txt", "`;;` can only be used in a case clause", 1, 22},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			assertParseErrorAt(t, src, test.text, test.line, test.col)
		})
	}
}
