package parse

import (
	"bytes"
	"fmt"
	"slices"
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
		// A `\` line continuation is removed before the line is read, so it
		// leaves the `;` after it in command position (#467).
		"print a; \\\n;",
		"print a || \\\n;",
		"{ print a || \\\n; }",
		"print a & \\\n;",
		"print a; \\\n\\\n;",
		"\\\n;\nprint a",
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
		"print a && \\\n&",
		"print a | \\\n;",
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
		// A line continuation neither ends nor starts a word (#467).
		{name: "continuation then separator", src: "print a; \\\n;", want: 1},
		{name: "continuation before a word", src: "print a; \\\nb;", want: 0},
		{name: "continuation inside a word", src: "print a\\\nb;\n;", want: 1},
		// A `\` before any other byte is still an escaped word character, so
		// the `;` after `a\;` follows a word and is an ordinary terminator.
		{name: "escaped separator then separator", src: "print a\\; ;", want: 0},
		// A `\` as the last byte has no newline to join, and must not be read
		// past the end of the source.
		{name: "trailing backslash", src: "; \\", want: 1},
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

// A command substitution inside a double-quoted string and a backquoted
// command each hold a list of their own, so a `;` opening that list ends an
// empty sublist as it does anywhere else (#569). Each source passes
// `zsh -f -n` as a file and runs cleanly under `zsh -f`.
func TestParseRedundantSeparatorInNestedLists(t *testing.T) {
	sources := []string{
		`echo "$( ; )"`,
		`echo "$( ; print a )"`,
		`x="$( ; print a )"; print $x`,
		`f() { echo "$( ; print a )"; }; f`,
		`echo "a $( ; print b ) c"`,
		`echo "$(print a; ; print b)"`,
		`echo "$(echo "$( ; print a )")"`,
		"echo `;`",
		"echo `; print a`",
		"echo \"`; print a`\"",
		`echo "${x:-y} $( ; print a )"`,
	}

	for _, src := range sources {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
	}
}

// Entering the nested lists must not accept what Zsh rejects there. Each
// source fails `zsh -f -n` as a file.
func TestParseRedundantSeparatorInNestedListsZshRejects(t *testing.T) {
	sources := []string{
		`echo "$(true | ; )"`,
		"echo `true | ;`",
		`echo "$(if true; ; )"`,
		// A `;` after a redirection operator is not in command position:
		// the operator lacks its word, whatever else in the file is fixed.
		`echo "$( ; )"; print a >& ; print b`,
		"echo `;`; print a >& ; print b",
		`echo "$( ; )" >& ; print b`,
		`echo "$( ; )"; print a >| ; print b`,
		`echo "$( ; )"; print a <& ; print b`,
		`echo "$( ; )"; print a 2>& ; print b`,
		`echo "$( ; )"; print a >&| ; print b`,
		`echo "$( ; )"; echo "$(print a >& ; print b)"`,
		`; print a >& ; print b`,
	}

	for _, src := range sources {
		if err := parseString(t, src); err == nil {
			t.Errorf("invalid Zsh must stay rejected: %q", src)
		}
	}
}

// Where each nested site is, and where none is: a `;` inside single quotes,
// an escaped one, and an arithmetic or parameter expansion stay text.
func TestScanRedundantSeparatorSitesInNestedLists(t *testing.T) {
	tests := []struct {
		src  string
		want []int
	}{
		{src: `echo "$( ; )"`, want: []int{9}},
		{src: "echo `; print a`", want: []int{6}},
		{src: "echo \"`;`\"", want: []int{7}},
		{src: `echo "$(echo "$( ; )")"`, want: []int{17}},
		{src: `echo "$(print a; print b)"`, want: nil},
		{src: `echo "$(print ';')"`, want: nil},
		{src: `echo "\$( ; )"`, want: nil},
		{src: `echo "$(( 1 ))"`, want: nil},
		{src: `echo "${x:-;}"`, want: nil},
		// A redirection operator's `&` or `|` does not open a sublist.
		{src: "print a >&2; ;", want: []int{13}},
		{src: "print a >& ;", want: nil},
		{src: "print a >&| ;", want: nil},
		{src: "print a <& ;", want: nil},
		// `>&|` at the start of the source: the `>` is byte 0.
		{src: ">&| ;", want: nil},
		// A body whose text the scanner cannot read as Zsh does (`$'...'`
		// escapes, here-documents) reports no nested site, so the retry
		// never blanks a byte inside a string or here-document body.
		{src: `; echo "$(print $'a\'; ;')"`, want: []int{0}},
		{src: "; echo \"`cat <<E\n;\nE\n`\"", want: []int{0}},
		{src: "; echo `cat <<E\n;\nE\n`", want: []int{0}},
		// An unterminated string has no certain extent, so it reports nothing.
		{src: `echo "$( ; )`, want: nil},
		{src: "echo `; print a", want: nil},
		// A `$` or `$(` as the last bytes must not be read past the end.
		{src: `echo "$(`, want: nil},
		{src: `echo "$`, want: nil},
		// An unclosed backquote inside the string ends the scan there, so a
		// later substitution is not scanned either.
		{src: "echo \"` $( ; )\"", want: nil},
		// A backquoted command is stepped over whole: a second one opens at its
		// own backquote, and a `;` after it follows a statement.
		{src: "echo `;` `;`", want: []int{6, 10}},
		{src: "`print a`; print b", want: nil},
		// The `;` after the first command is a terminator; only the second
		// is a site. Resuming at the closing backquote would read the text up
		// to the next one as a command list and report the first as well.
		{src: "echo `true`; ; echo `true`", want: []int{13}},
		// An expansion or substitution whose extent the shared scanners refuse
		// (arithmetic holding `$`, a substitution holding `case`) ends the
		// scan: nothing after it is reported.
		{src: `echo "$(( $x )) $( ; )"`, want: nil},
		{src: `echo "$(case x in x) :;; esac) $( ; )"`, want: nil},
		// An arithmetic expansion is not a command list.
		{src: `echo "$(( ; ))"`, want: nil},
		// A substitution is stepped over whole, so a `"` inside it does not end
		// the string.
		{src: `echo "$(echo "a") $( ; )"`, want: []int{21}},
	}

	for _, test := range tests {
		top, nested := scanSeparatorSites([]byte(test.src))
		got := slices.Sorted(slices.Values(append(top, nested...)))
		if len(got) == 0 {
			got = nil
		}
		if !slices.Equal(got, test.want) {
			t.Errorf("scanSeparatorSites(%q) = %v + %v, want %v", test.src, top, nested, test.want)
		}
	}
}

// A nested site is masked only with other nested sites, never with the
// top-level ones, so fixing it never lets the top-level mask accept a file
// that base rejects for an unrelated `;` (review of #570, #572). Each source
// fails `zsh -f -n`; with the nested `;` removed, base rejects it too.
func TestParseRedundantSeparatorNestedSiteDoesNotUnblockOthers(t *testing.T) {
	sources := []string{
		`echo "$( ; )"; print a(;)`,
		`echo "$( ; )"; print *(;)`,
		"echo `;`; print a(;)",
		`echo "$( ; )"; print a > ( ; print b)`,
		`echo "$( ; )"; [[ a == (;) ]]`,
		`f() { echo "$( ; )"; print a(;); }`,
		"f() { echo `;`; print a > ( ; print b); }",
		`f() { echo "$( ; )"; print *(|;); }`,
		// Review of fdc29dc: the same `;` inside the nested body, where the
		// scanner reads the glob group or pattern `(` as a list opener.
		"echo `; print a(;)`",
		`echo "$( ; [[ a == (;) ]] )"`,
		"echo \"`; print a(;)`\"",
		"echo `;` `print a(;)`",
		`echo "$( ; [[ a == (|;) ]] )"`,
		`; echo "$( [[ a == (;) ]] )"`,
		`; x="$( [[ a == (;) ]] )"`,
		// Review of a9cfa4e: a `;` after `||`, `&&` or `(` inside `[[ ]]`
		// in a nested body. Blanked, it falls between test operands, not
		// inside text, so only a structural check can refuse it.
		`echo "$( ; [[ a || ; b ]] )"`,
		`echo "$( ; [[ a && ; b ]] )"`,
		`echo "$( ; [[ ( ; a ) ]] )"`,
		`echo "$( ; [[ a && ( ; b ) ]] )"`,
		`echo "$( ; [[ ! ( ; a ) ]] )"`,
		`echo "$( ; [[ a == b || ; c ]] )"`,
		`f() { echo "$( ; [[ a || ; b ]] )"; }`,
		`x="$( ; [[ a || ; b ]] )"`,
		`echo "$(print a; ; [[ a || ; b ]] )"`,
		`; echo "$( ; [[ a || ; b ]] )"`,
		`if [[ -n "$( ; [[ a || ; b ]] && print y )" ]]; then :; fi`,
		// A `;` in a case pattern list after `|`.
		`echo "$( ; case a in (x | ; y) ;; esac )"`,
		// The same pattern-list `;` in backquotes, where the masked source
		// parses with the blank inside the arm's patterns: the case arm
		// check refuses it.
		"echo `; case a in x | ; y) ;; esac`",
		"echo `; case a in (x | ; y) ;; esac`",
		// A text site that is restored does not excuse a site in `[[ ]]`
		// found in the same retry: the second parse is checked too.
		"echo \"$( ; print a # (;\n[[ a || ; b ]] )\"",
		"echo \"$( ; print ${x:-\n;} ; [[ a || ; b ]] )\"",
	}
	for _, src := range sources {
		if err := parseString(t, src); err == nil {
			t.Errorf("invalid Zsh must stay rejected: %q", src)
		}
	}
}

// A nested site whose blank would fall inside an arithmetic expression is
// not a list gap either. `zsh -f -n` passes these (the error is raised when
// the expression is evaluated), but base rejects the same file with only
// the real leading `;` blanked, so the fixed build keeps base's verdict.
func TestParseRedundantSeparatorNestedSiteInArithmeticDeclines(t *testing.T) {
	for _, src := range []string{
		`echo "$( ; (( 1 | ; 2 )) )"`,
		`echo "$( ; print $(( 1 || ; 2 )) )"`,
		`echo "$( ; print $(( ( ; 1 ) )) )"`,
	} {
		if err := parseString(t, src); err == nil {
			t.Errorf("must keep base's rejection: %q", src)
		}
	}
}

// A nested site in a real list gap stays fixed in every list-owning
// construct: a pipeline or `&&` list, a subshell, a brace group, an `if`
// or `while` condition, a `for` word list and a process substitution.
// Each is valid Zsh that base rejects.
func TestParseRedundantSeparatorNestedSiteInListGaps(t *testing.T) {
	for _, src := range []string{
		`echo "$( ; print a | ; print b )"`,
		`echo "$( ; print a && ; print b )"`,
		`echo "$( ; ( ; ) )"`,
		`echo "$( ; { ; } )"`,
		`echo "$( ; if ; then :; fi )"`,
		`echo "$( ; while ; do :; done )"`,
		`echo "$( ; print <( ; ) )"`,
		`echo "$( ; print a; ; print b )"`,
		// The statement `print a &` ends where the blank starts: the gap
		// is after it, not inside it.
		`echo "$( ; print a &; print b )"`,
		`echo "$( ; print a |& ; print b )"`,
		// Gaps in the condition and body lists of if, elif, else, while,
		// until and for.
		`echo "$( ; if true; ; then :; fi )"`,
		`echo "$( ; if true; then :; ; fi )"`,
		`echo "$( ; if true; then :; elif true; ; then :; fi )"`,
		`echo "$( ; if true; then :; else ; fi )"`,
		`echo "$( ; while true; ; do :; done )"`,
		`echo "$( ; while true; do :; ; done )"`,
		`echo "$( ; until true; ; do :; done )"`,
		`echo "$( ; for x in a; do :; ; done )"`,
		// A text site restored beside a real one.
		"echo \"$( ; print ${x:-\n;}; ; print b )\"",
		// A gap inside a case arm body, after its pattern.
		"echo `case x in x) print a ; ; ;; esac`",
	} {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
	}
}

// Review of 5a3e075: a backquoted body the nested pass cannot use (it holds
// `$'`, `<<` or `$((... << ...))`, or its `;` sits in a list the gap check
// does not cover, such as `repeat`) keeps base's handling, which reads the
// body as top-level text. Each row is valid Zsh that base accepts.
func TestParseRedundantSeparatorBackquoteKeepsBaseVerdict(t *testing.T) {
	for _, src := range []string{
		"echo `print a; ; print $'b'`",
		"echo `print a; ; cat <<<b`",
		"echo `print a; ; cat <<E\nx\nE\n`",
		"echo `print $((1<<2)); ; print a`",
		"echo `print a; ; print ${x#$'b'}`",
		"echo `print a; ; x=$'b'`",
		"print x; ; echo `print $'b'; ; print c`",
		"; echo `print a; ; print $'b'`",
		"echo `repeat 2 do print a; ; done`",
		"echo `repeat 2 { : ; ; }`",
		"f() { echo `repeat 2 { : ; ; }`; }",
	} {
		if err := parseString(t, src); err != nil {
			t.Errorf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
	}
}

// When the parser reports a top-level `;`, the nested sites join the
// top-level pass: one retry, and the verdict base gives the file without
// them. With a counting parser that does not re-enter the adapters, the
// retry must already see the nested `;` blanked.
func TestParseRedundantSeparatorTopLevelPassIncludesNestedSites(t *testing.T) {
	src := []byte("; echo \"$( ; )\" `;`")
	var calls int
	var seen []byte
	parse := func(masked []byte, name string) (*syntax.File, error) {
		calls++
		seen = masked
		return syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(masked), name)
	}
	_, firstErr := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(src), "t.zsh")
	if firstErr == nil {
		t.Fatal("source must fail the unadapted parser for this test to mean anything")
	}
	if _, err := parseRedundantSeparatorWithParser(src, "t.zsh", firstErr, parse); err != nil {
		t.Fatalf("must parse in one retry: %v (retry source %q)", err, seen)
	}
	if calls != 1 || bytes.Contains(seen, []byte(";")) {
		t.Fatalf("retry parser called %d times with %q, want once with every `;` blanked", calls, seen)
	}
}

// The nested pass blanks only the nested sites: a top-level `;` the scanner
// misreads (#572) stays in the retry source, so the file keeps base's
// verdict for it. The counting parser does not re-enter the adapters, so
// the retry must fail on that `;`.
func TestParseRedundantSeparatorNestedPassLeavesTopLevelSites(t *testing.T) {
	src := []byte(`echo "$( ; print a )"; ; print b`)
	var seen []byte
	parse := func(masked []byte, name string) (*syntax.File, error) {
		seen = masked
		return syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(masked), name)
	}
	_, firstErr := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(src), "t.zsh")
	if firstErr == nil {
		t.Fatal("source must fail the unadapted parser for this test to mean anything")
	}
	_, _ = parseRedundantSeparatorWithParser(src, "t.zsh", firstErr, parse)
	if want := []byte(`echo "$(   print a )"; ; print b`); !bytes.Equal(seen, want) {
		t.Fatalf("nested pass retried %q, want %q", seen, want)
	}
}

// Where a nested site turns out to be text (a `;` inside `${...}`, a glob
// group, a quoted string or a comment in the nested body), its byte is put
// back and only the real sites stay blanked: the file parses and its tree
// equals the tree of the source with only the real nested `;` blanked by
// hand. Each source is valid Zsh that base rejects.
func TestParseRedundantSeparatorNestedTextSiteRestored(t *testing.T) {
	printTree := func(src string) string {
		file, err := Parse(strings.NewReader(src+"\n"), "t.zsh")
		if err != nil {
			t.Fatalf("must parse: %q\nerror: %v", src, err)
		}
		var out strings.Builder
		if err := syntax.NewPrinter().Print(&out, file.tree); err != nil {
			t.Fatal(err)
		}
		syntax.Walk(file.tree, func(node syntax.Node) bool {
			if c, ok := node.(*syntax.Comment); ok {
				out.WriteString("#" + c.Text + "\n")
			}
			return true
		})
		return out.String()
	}
	for src, blanked := range map[string]string{
		"echo \"$( ; print ${x:-\n;} )\"": "echo \"$(   print ${x:-\n;} )\"",
		"echo `; print a # (;`\nprint b":  "echo ` print a # (;`\nprint b",
		`echo "$( ; print a(;) )"`:        `echo "$(   print a(;) )"`,
		// The nested pass leaves the later top-level `;` for re-entry, which
		// masks it as base does.
		`echo "$( ; print a )"; ; print b`: `echo "$(   print a )";   print b`,
		// A text site in a single-quoted string: SglQuoted must be restored.
		`echo "$( ; print 'x; ;' )"`: `echo "$(   print 'x; ;' )"`,
		// Two text sites in one word: restoring one still leaves a blank in
		// the other until both are put back.
		"echo \"$( ; print ${x:-\n; ;} )\"": "echo \"$(   print ${x:-\n; ;} )\"",
	} {
		if got, want := printTree(src), printTree(blanked); got != want {
			t.Errorf("tree of %q:\n got %s\nwant %s", src, got, want)
		}
	}
}

// Masking the nested sites leaves every other byte of a nested body as
// written: a `;` inside `${...}`, a glob group, a comment, a quoted string
// or a here-document keeps its text when the top-level pass runs (review of
// fdc29dc). Each source parses on base, and its tree must equal the tree of
// the source with only its leading `;` blanked, which is base's retry. A
// `;` the top-level scan itself misreads beside a substitution is #572 and
// changes the same way on base.
func TestParseRedundantSeparatorNestedSiteKeepsOtherBytes(t *testing.T) {
	print := func(src string) string {
		file, err := Parse(strings.NewReader(src), "t.zsh")
		if err != nil {
			t.Fatalf("reference must parse: %q\nerror: %v", src, err)
		}
		var out strings.Builder
		if err := syntax.NewPrinter().Print(&out, file.tree); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	for _, src := range []string{
		"; echo \"$( print ${x:-\n;} )\"",
		"; echo \"$( print a # (;\n)\"",
		"; echo \"`print \\\"a; ;b\\\"`\"",
		"; cat <<'A'\n\"$( ; lit )\"\nA",
		"; echo \"$( print a(;) )\"",
		"; echo \"$( print a > ( ; print b) )\"",
	} {
		file, err := Parse(strings.NewReader(src+"\n"), "t.zsh")
		if err != nil {
			t.Fatalf("base parses it, so must this change: %q\nerror: %v", src, err)
		}
		var got strings.Builder
		if err := syntax.NewPrinter().Print(&got, file.tree); err != nil {
			t.Fatal(err)
		}
		// Only the leading `;` is a real site; every other `;` in these
		// sources is text and must stay.
		reference := " " + src[1:] + "\n"
		if want := print(reference); got.String() != want {
			t.Errorf("tree of %q changed:\n got %s\nwant %s", src, got.String(), want)
		}
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

// Nested sites are masked in one pass too (#366): a file holding many of
// them costs one retry, and the retry sees every nested `;` blanked.
func TestParseRedundantSeparatorNestedSitesParseOnce(t *testing.T) {
	src := []byte("echo \"$( ; )\" \"$( ; )\" `;` \"$(print a; ; print b)\"")
	var calls int
	var seen []byte
	parse := func(masked []byte, name string) (*syntax.File, error) {
		calls++
		seen = masked
		return syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(masked), name)
	}
	_, firstErr := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(src), "t.zsh")
	if firstErr == nil {
		t.Fatal("source must fail the unadapted parser for this test to mean anything")
	}
	if _, err := parseRedundantSeparatorWithParser(src, "t.zsh", firstErr, parse); err != nil {
		t.Fatalf("must parse: %v", err)
	}
	if calls != 1 {
		t.Fatalf("retry parser called %d times, want 1", calls)
	}
	if n := bytes.Count(seen, []byte(";")); n != 1 {
		t.Fatalf("retry source %q keeps %d `;`, want only the ordinary terminator", seen, n)
	}
}

// No blank a mask put in survives inside text: every comment and
// single-quoted string of an accepted tree reads the source's own bytes,
// and a `;` inside `${...}` or a quoted string in a nested body keeps its
// text (review of fdc29dc). `a(;)` inside a quoted `$( )` is a glob group
// the fork itself reads as `a`, `(`, ` )` whatever the adapter does (base
// and a source with no redundant `;` give the same tree), so it is not
// listed.
func TestParseRedundantSeparatorTreeTextMatchesSource(t *testing.T) {
	for _, src := range []string{
		`echo "$( ; print a(;) )"`,
		"echo \"$( ; print ${x:-\n;} )\"",
		"echo `; print a # (;`\nprint b",
		"; echo \"$( print a # (;\n)\"",
		"; echo \"$( print a(;) )\"",
		"; echo \"$( print a > ( ; print b) )\"",
		"; echo \"`print \\\"a; ;b\\\"`\"",
		`echo "$( ; print 'x;' )"`,
	} {
		text := src + "\n"
		file, err := Parse(strings.NewReader(text), "t.zsh")
		if err != nil {
			t.Fatalf("valid Zsh must parse: %q\nerror: %v", src, err)
		}
		syntax.Walk(file.tree, func(node syntax.Node) bool {
			var from, to syntax.Pos
			var value string
			switch node := node.(type) {
			case *syntax.SglQuoted:
				from, to, value = node.Left, node.Right, node.Value
			case *syntax.Comment:
				from, to, value = node.Hash, node.End(), node.Text
			default:
				return true
			}
			if a, b := from.Offset(), to.Offset(); a <= b && b <= uint(len(text)) &&
				!strings.Contains(text[a:b], value) {
				t.Errorf("text in %q at %d: source %q, tree %q", src, a, text[a:b], value)
			}
			return true
		})
		var printed strings.Builder
		if err := syntax.NewPrinter().Print(&printed, file.tree); err != nil {
			t.Fatal(err)
		}
		for _, word := range []string{"${x:-\n;}", "a; ;b", "'x;'"} {
			if strings.Contains(src, word) && !strings.Contains(printed.String(), word) {
				t.Errorf("tree of %q lost %q:\n%s", src, word, printed.String())
			}
		}
	}
}

// innermostNode picks the smallest node holding an offset, and treats a
// node's end offset as outside it. Unit rows, since which node wins at an
// exact boundary is decided before any source can reach sitesInListGaps.
func TestInnermostNode(t *testing.T) {
	src := "echo $(print a; b)\n"
	file, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		at   int
		want string
	}{
		{strings.Index(src, "a;"), "*syntax.Lit"},
		// A statement's range includes its own `;` terminator.
		{strings.Index(src, "; b"), "*syntax.Stmt"},
		// The byte after that `;` ends the statement: it is not inside it.
		{strings.Index(src, "; b") + 1, "*syntax.CmdSubst"},
		{strings.Index(src, ")"), "*syntax.CmdSubst"},
		// The final newline lies outside every node.
		{len(src) - 1, "<nil>"},
	} {
		if got := fmt.Sprintf("%T", innermostNode(file, uint(row.at))); got != row.want {
			t.Errorf("offset %d (%q): got %s, want %s", row.at, src[row.at:row.at+1], got, row.want)
		}
	}
}

// sitesInListGaps refuses a blank at the `)` that ends a case arm's
// patterns as well as among them; the arm's list starts after it.
func TestSitesInListGapsCaseArmBoundary(t *testing.T) {
	src := []byte("case a in (x) print a;; esac\n")
	file, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(bytes.NewReader(src), "t.zsh")
	if err != nil {
		t.Fatal(err)
	}
	item := file.Stmts[0].Cmd.(*syntax.CaseClause).Items[0]
	end := int(item.Patterns[0].End().Offset())
	masked := bytes.Clone(src)
	for _, row := range []struct {
		at   int
		want bool
	}{
		{end - 1, false}, // inside the pattern word
		{end, false},     // the `)` closing the patterns
		{end + 1, true},  // the blank before the arm's list
	} {
		masked[row.at] = ' ' // differs from src: a still-blanked site
		ref := bytes.Clone(masked)
		ref[row.at] = src[row.at] + 1
		if got := sitesInListGaps(file, []int{row.at}, ref, masked); got != row.want {
			t.Errorf("site at %d (%q): got %v, want %v", row.at, src[row.at:row.at+1], got, row.want)
		}
		masked[row.at] = src[row.at]
	}
}
