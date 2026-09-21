package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Zsh reads a list as zero or more sublists, so a `;` standing in command
// position terminates an empty sublist and does nothing (#332). Every source
// here was checked with `zsh -f -n` as a file, and the executable ones were
// run to confirm the `;` is a no-op rather than merely accepted.
func TestParseRedundantSeparatorZshAccepts(t *testing.T) {
	sources := []string{
		";",
		"; ; ;",
		"print x; ;",
		"print run; ; print again",
		"while true; ;",
		"until true; ;",
		"while (( 1 )); ;",
		"f() { while true; ; }",
		"f() { print body; ; }",
		"f() { : ; ; }",
		"f() { print a; ; print b; ; }",
		"print 'quoted;' ; ;",
		`print "also;" ; ;`,
		"while true;\n;",
	}

	for _, src := range sources {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
	}
}

// The adapter must not turn the linter into one that accepts anything
// containing a `;`. Each source here is rejected by `zsh -f -n` as a file.
func TestParseRedundantSeparatorZshRejects(t *testing.T) {
	sources := []string{
		"if true; ;",
		"true | ;",
		"true; &",
	}

	for _, src := range sources {
		if err := parseString(t, src); err == nil {
			t.Errorf("invalid Zsh must stay rejected: %q", src)
		}
	}
}

// A `;;`, `;&` or `;|` is a case terminator, not a separator, so it must
// never be treated as a redundant-separator site. Were it masked, `case x in
// y) :;; esac` would lose its terminator and change meaning.
func TestScanRedundantSeparatorSitesSkipsCaseTerminators(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "case terminator", src: "case x in y) :;; esac", want: 0},
		{name: "case fallthrough", src: "case x in y) :;& z) :;; esac", want: 0},
		// A redundant `;` immediately before a case terminator: the scanner
		// finds no site, because the first `;` closes the (empty) case body
		// as an ordinary terminator and the `;;` is then stepped over
		// whole. What matters is that the `;;` survives — treating it as
		// two separate bytes would mask the second and delete the
		// terminator, which is what the mutation test pins.
		{name: "redundant then terminator", src: "case x in y) ; ;; esac", want: 0},
		{name: "quoted semicolon", src: "print 'a;b'", want: 0},
		{name: "double quoted", src: `print "a;b"`, want: 0},
		{name: "escaped", src: `print a\;b`, want: 0},
		{name: "comment", src: "print a # ; b", want: 0},
		{name: "ordinary terminator", src: "print a; print b", want: 0},
		{name: "one site", src: "print a; ;", want: 1},
		{name: "run of three", src: "; ; ;", want: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := scanRedundantSeparatorSites([]byte(test.src))
			if len(got) != test.want {
				t.Fatalf("scanRedundantSeparatorSites(%q) = %v, want %d site(s)",
					test.src, got, test.want)
			}
		})
	}
}

// The adapter must decline an error it does not own, and must decline a file
// whose reported `;` is not one of the sites it found. Otherwise a file that
// merely happens to contain a redundant separator could mask an unrelated
// defect elsewhere.
func TestParseRedundantSeparatorDeclinesForeignErrors(t *testing.T) {
	src := []byte("print x; ;")

	t.Run("other error text", func(t *testing.T) {
		foreign := syntax.ParseError{Text: "some other complaint"}
		if _, err := parseRedundantSeparator(src, "t.zsh", foreign); err == nil {
			t.Fatal("adapter must decline an error it does not own")
		}
	})

	t.Run("no sites", func(t *testing.T) {
		owned := syntax.ParseError{Text: redundantSeparatorError}
		if _, err := parseRedundantSeparator([]byte("print x"), "t.zsh", owned); err == nil {
			t.Fatal("adapter must decline when there is no site")
		}
	})

}

// The adapter blanks every site in one pass rather than one per re-entry, so
// a file with many redundant separators costs one extra parse, not one per
// separator. This pins the contract: the retry parser is called once.
func TestParseRedundantSeparatorParsesOnce(t *testing.T) {
	var calls int
	parse := func(src []byte, name string) (*syntax.File, error) {
		calls++
		return syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(
			strings.NewReader(string(src)), name)
	}

	src := []byte("print a; ;\nprint b; ;\nprint c; ;\nprint d; ;")

	// Use the parser's real complaint rather than a hand-built position, so
	// the site check is exercised against the offset Zsh actually reports.
	_, firstErr := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(
		strings.NewReader(string(src)), "t.zsh")
	if firstErr == nil {
		t.Fatal("source must fail the unadapted parser for this test to mean anything")
	}

	if _, err := parseRedundantSeparatorWithParser(src, "t.zsh", firstErr, parse); err != nil {
		t.Fatalf("must parse: %v", err)
	}
	if calls != 1 {
		t.Fatalf("retry parser called %d times, want 1", calls)
	}
}
