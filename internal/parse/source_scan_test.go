package parse

import (
	"strings"
	"testing"
)

// A comment is a word that begins with `#` (zshmisc COMMENTS). Every row's
// verdict was checked against `zsh -f`: a comment hides the rest of its line,
// a word does not. A `{` that is an argument, as in `print {#b`, is a word in
// Zsh but is read as a group opener here, so no row puts one after a blank in
// argument position. (#643)
func TestStartsSourceComment(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"#c", true},
		{": #c", true},
		{":\t#c", true},
		{":\n#c", true},
		{":;#c", true},
		{"{#c", true},
		{": ; {#c", true},
		{":;\t{#c", true},
		{":\n{#c", true},
		{":;{#c", true},
		{":&{#c", true},
		{": |{#c", true},
		{"({#c", true},
		{"f(){#c", true},
		{"{{#c", true},
		{"x=`{#c", true},
		// Glued to word text, the `#` is part of the word.
		{"print ${#x", false},
		{"print ${x}#y", false},
		{"print a}#b", false},
		{": ;}#c", false},
		{"print a{#b", false},
		{"print \\{#b", false},
		{"print '1'{#c", false},
		{"print \"2\"{#c", false},
		{"print x={#c", false},
		{"print >{#c", false},
		{"print <{#c", false},
		{"!{#c", false},
		{"print a}{#c", false},
		{"print $#x", false},
		{"print a#b", false},
	}
	for _, test := range tests {
		src := []byte(test.src)
		i := len(src) - 1
		for i >= 0 && src[i] != '#' {
			i--
		}
		if got := startsSourceComment(src, i); got != test.want {
			t.Errorf("startsSourceComment(%q, %d) = %t, want %t", test.src, i, got, test.want)
		}
	}
}

// A `$` that is the last byte of the source opens nothing, and neither
// scanner that tracks `${...}` may read past it. Both sources are valid Zsh
// without a final newline.
func TestParameterWordTrackerDollarAtEndOfSource(t *testing.T) {
	var tracker parameterWordTracker
	tracker.open([]byte("a$"), 1)
	if tracker.inWord(1) {
		t.Fatalf("a final `$` opened an expansion")
	}

	for _, src := range []string{
		"{ :; } always { : }; print $",
		"{ local x=1 }; print $",
	} {
		if _, err := Parse(strings.NewReader(src), "eof.zsh"); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
}
