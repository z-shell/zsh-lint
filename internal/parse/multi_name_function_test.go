package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// multiNameFuncDecls returns every function declaration in the tree in
// source order, so a test can assert on the one the adapter restored.
func multiNameFuncDecls(file *File) []*syntax.FuncDecl {
	var decls []*syntax.FuncDecl
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if decl, ok := node.(*syntax.FuncDecl); ok {
			decls = append(decls, decl)
		}
		return true
	})
	return decls
}

func funcDeclNames(decl *syntax.FuncDecl) []string {
	if decl.Name != nil {
		return []string{decl.Name.Value}
	}
	names := make([]string, 0, len(decl.Names))
	for _, name := range decl.Names {
		names = append(names, name.Value)
	}
	return names
}

func TestParseMultiNameFunction(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
		wantPos   string
	}{
		{
			name:      "spaced parens",
			src:       "a b () { : }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "glued parens",
			src:       "a b() { : }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "ztst harness",
			src:       "ZTST_prep ZTST_clean () {\n  print prep\n}\n",
			wantNames: []string{"ZTST_prep", "ZTST_clean"},
			wantPos:   "1:1",
		},
		{
			name:      "four names",
			src:       "a b c d () { : }\n",
			wantNames: []string{"a", "b", "c", "d"},
			wantPos:   "1:1",
		},
		{
			name:      "punctuated names",
			src:       "k-l m.n:o p/q () { : }\n",
			wantNames: []string{"k-l", "m.n:o", "p/q"},
			wantPos:   "1:1",
		},
		{
			name:      "simple command body",
			src:       "a b () print hi\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "comment before the body",
			src:       "a b () # note\n{\n  :\n}\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "after then",
			src:       "if true; then a b () { : }; fi\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:15",
		},
		{
			name:      "under time",
			src:       "time a b () { : }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:6",
		},
		{
			name:      "inside a brace group",
			src:       "{ a b () { : } }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:3",
		},
		{
			name:      "after a separator on the same line",
			src:       "print x; a b () { : }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:10",
		},
		{
			name:      "names continued across lines",
			src:       "a b\\\n c () { : }\n",
			wantNames: []string{"a", "b", "c"},
			wantPos:   "1:1",
		},
		{
			name:      "tab separated",
			src:       "a\tb () { : }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "as an if condition",
			src:       "if a b () { : }; then :; fi\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:4",
		},
		{
			name:      "as a while condition",
			src:       "while a b () { : }; do :; done\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:7",
		},
		{
			name:      "after do",
			src:       "while true; do a b () { : }; done\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:16",
		},
		{
			name:      "after elif",
			src:       "if true; then :; elif a b () { : }; then :; fi\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:23",
		},
		{
			name:      "after else",
			src:       "if true; then :; else a b () { : }; fi\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:23",
		},
		{
			name:      "in a pipeline",
			src:       "a b () { : } | cat\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "before an and list",
			src:       "a b () { : } && print y\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "inside a command substitution",
			src:       "x=$(a b () { : })\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:5",
		},
		{
			name:      "inside a subshell",
			src:       "( a b () { : } )\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:3",
		},
		{
			name:      "with a redirect",
			src:       "a b () { : } > /dev/null\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "reserved word after the first name",
			src:       "a then b () { : }\n",
			wantNames: []string{"a", "then", "b"},
			wantPos:   "1:1",
		},
		{
			name:      "after a repeat count",
			src:       "repeat 2 a b () { : }\n",
			wantNames: []string{"a", "b"},
			wantPos:   "1:10",
		},
		{
			name:      "on a later line",
			src:       "print x\n\nZTST_prep ZTST_clean () { : }\n",
			wantNames: []string{"ZTST_prep", "ZTST_clean"},
			wantPos:   "3:1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "multi-name.zsh")
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.src, err)
			}
			decls := multiNameFuncDecls(file)
			if len(decls) != 1 {
				t.Fatalf("function declarations = %d, want 1", len(decls))
			}
			decl := decls[0]
			if got := funcDeclNames(decl); strings.Join(got, " ") != strings.Join(test.wantNames, " ") {
				t.Fatalf("names = %q, want %q", got, test.wantNames)
			}
			if decl.Name != nil {
				t.Fatalf("Name = %q, want nil with every name in Names", decl.Name.Value)
			}
			if decl.RsrvWord || !decl.Parens {
				t.Fatalf("RsrvWord, Parens = %v, %v, want false, true", decl.RsrvWord, decl.Parens)
			}
			if got := decl.Pos().String(); got != test.wantPos {
				t.Fatalf("declaration position = %s, want %s", got, test.wantPos)
			}
			if decl.Body == nil {
				t.Fatal("Body = nil, want the shared body")
			}
		})
	}
}

func TestParseMultiNameFunctionPreservesPositions(t *testing.T) {
	const src = "print x\nZTST_prep ZTST_clean () {\n  print prep\n}\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	stmt := file.AST().Stmts[1]
	decl, ok := stmt.Cmd.(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("Stmts[1].Cmd is %T, want *syntax.FuncDecl", stmt.Cmd)
	}
	if got, want := stmt.Pos().String(), "2:1"; got != want {
		t.Errorf("statement position = %s, want %s", got, want)
	}
	if got, want := decl.Pos().String(), "2:1"; got != want {
		t.Errorf("declaration position = %s, want %s", got, want)
	}
	wantSpans := []struct{ value, pos, end string }{
		{"ZTST_prep", "2:1", "2:10"},
		{"ZTST_clean", "2:11", "2:21"},
	}
	if len(decl.Names) != len(wantSpans) {
		t.Fatalf("Names = %d, want %d", len(decl.Names), len(wantSpans))
	}
	for i, want := range wantSpans {
		name := decl.Names[i]
		if name.Value != want.value {
			t.Errorf("Names[%d].Value = %q, want %q", i, name.Value, want.value)
		}
		if got := name.Pos().String(); got != want.pos {
			t.Errorf("Names[%d].Pos() = %s, want %s", i, got, want.pos)
		}
		if got := name.End().String(); got != want.end {
			t.Errorf("Names[%d].End() = %s, want %s", i, got, want.end)
		}
		if got := src[name.Pos().Offset():name.End().Offset()]; got != want.value {
			t.Errorf("Names[%d] source bytes = %q, want %q", i, got, want.value)
		}
	}
	body, ok := decl.Body.Cmd.(*syntax.Block)
	if !ok {
		t.Fatalf("Body.Cmd is %T, want *syntax.Block", decl.Body.Cmd)
	}
	if got, want := body.Lbrace.String(), "2:25"; got != want {
		t.Errorf("Lbrace = %s, want %s", got, want)
	}
	if got, want := body.Rbrace.String(), "4:1"; got != want {
		t.Errorf("Rbrace = %s, want %s", got, want)
	}
	if got, want := decl.End().String(), "4:2"; got != want {
		t.Errorf("declaration end = %s, want %s", got, want)
	}
}

func TestParseMultiNameFunctionPrintsOriginalNames(t *testing.T) {
	const src = "ZTST_prep ZTST_clean () {\n  print prep\n}\n"
	file, err := Parse(strings.NewReader(src), "print.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	printed := &strings.Builder{}
	if err := syntax.NewPrinter().Print(printed, file.AST()); err != nil {
		t.Fatalf("printing tree: %v", err)
	}
	if !strings.HasPrefix(printed.String(), "ZTST_prep ZTST_clean() {") {
		t.Fatalf("printed tree does not carry every name:\n%s", printed.String())
	}
}

func TestParseMultiNameFunctionRestoresEverySite(t *testing.T) {
	const src = "a b () { : }; c d () { : }\ne f () {\n  :\n}\n"
	file, err := Parse(strings.NewReader(src), "sites.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decls := multiNameFuncDecls(file)
	want := []struct {
		names string
		pos   string
	}{
		{"a b", "1:1"},
		{"c d", "1:15"},
		{"e f", "2:1"},
	}
	if len(decls) != len(want) {
		t.Fatalf("function declarations = %d, want %d", len(decls), len(want))
	}
	for i, decl := range decls {
		if got := strings.Join(funcDeclNames(decl), " "); got != want[i].names {
			t.Errorf("decls[%d] names = %q, want %q", i, got, want[i].names)
		}
		if got := decl.Pos().String(); got != want[i].pos {
			t.Errorf("decls[%d] position = %s, want %s", i, got, want[i].pos)
		}
	}
}

func TestParseMultiNameFunctionKeepsSingleNameShape(t *testing.T) {
	// A single-name definition needs no adapter and keeps the parser's own
	// shape, Name set and Names empty, so consumers see the two forms the
	// way upstream defines them.
	file, err := Parse(strings.NewReader("a () { : }\nfunction b c () { : }\n"), "single.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decls := multiNameFuncDecls(file)
	if len(decls) != 2 {
		t.Fatalf("function declarations = %d, want 2", len(decls))
	}
	if decls[0].Name == nil || decls[0].Name.Value != "a" || len(decls[0].Names) != 0 {
		t.Errorf("single-name declaration = %q / %d names, want Name a", funcDeclNames(decls[0]), len(decls[0].Names))
	}
	if !decls[1].RsrvWord || strings.Join(funcDeclNames(decls[1]), " ") != "b c" {
		t.Errorf("reserved-word declaration = %q, RsrvWord %v", funcDeclNames(decls[1]), decls[1].RsrvWord)
	}
}

func TestParseMultiNameFunctionRejectsInvalidSources(t *testing.T) {
	tests := []struct {
		fixture string
		text    string
		line    uint
		col     uint
	}{
		// An assignment before the names is not a command start, so the
		// gate does not fire and the parser's own error stands.
		{"invalid-251-assignment-prefix.txt", multiNameFunctionError, 1, 9},
		// `( )` with a blank is a glob qualifier to the parser and a parse
		// error to Zsh; the gate's `()` check keeps it out of the retry.
		{"invalid-251-spaced-parens.txt", "`}` can only be used to close a block", 1, 13},
		// The retry accepts the definition and reports the real defect
		// after it, at its original position.
		{"invalid-251-trailing-word.txt", "statements must be separated by &, ; or a newline", 1, 14},
		{"invalid-251-missing-body.txt", "`foo()` must be followed by a statement", 1, 3},
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

func TestParseMultiNameFunctionLeavesUnrecognisedSitesUntouched(t *testing.T) {
	// Each source fails the bare parser with the gated error, and none is a
	// run of plain names at a command start, so the adapter must not retry.
	sources := map[string]string{
		"assignment prefix":  "x=1 a b () { : }\n",
		"negated":            "! a b () { : }\n",
		"quoted name":        "a \"b\" () { : }\n",
		"expanded name":      "a $b () { : }\n",
		"redirect in names":  "a >x b () { : }\n",
		"reserved word only": "then b () { : }\n",
		"repeat count only":  "repeat 3 () { print $1 } a b\n",
		"repeat one name":    "repeat 2 f() { : }\n",
		"select header":      "select a b () { : }\n",
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(src), "untouched.zsh")
			if firstErr == nil {
				t.Fatal("parseTree() unexpectedly accepted the source")
			}
			calls := 0
			_, err := parseMultiNameFunctionWithParser([]byte(src), "untouched.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
				calls++
				return nil, nil
			})
			if err != firstErr {
				t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
			}
			if calls != 0 {
				t.Fatalf("parser called %d times, want 0", calls)
			}
		})
	}
}

func TestParseMultiNameFunctionKeepsUnsupportedPrefixesRejected(t *testing.T) {
	// Native Zsh accepts each source, but the parser rejects the same prefix
	// before a single-name definition too, so the adapter must not turn the
	// error into a silently wrong tree. They are separate gaps if ever
	// needed; nothing in the surveyed corpus uses them.
	sources := map[string]string{
		"coproc":              "coproc a b () { : }\n",
		"nocorrect":           "nocorrect a b () { : }\n",
		"declaration builtin": "local a b () { : }\n",
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), "prefix.zsh"); err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", src)
			}
		})
	}
}

func TestParseMultiNameFunctionLeavesOtherErrorsUntouched(t *testing.T) {
	src := []byte("print x }\n")
	_, firstErr := parseTree(src, "other.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted a stray brace")
	}
	calls := 0
	_, err := parseMultiNameFunctionWithParser(src, "other.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		calls++
		return nil, nil
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
	if calls != 0 {
		t.Fatalf("parser called %d times, want 0", calls)
	}
}

func TestParseMultiNameFunctionMasksOnlyLeadingNames(t *testing.T) {
	src := []byte("if true; then a b c () { : }; fi\n")
	_, firstErr := parseTree(src, "mask.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the source")
	}
	var masked []byte
	_, _ = parseMultiNameFunctionWithParser(src, "mask.zsh", firstErr, func(retry []byte, name string) (*syntax.File, error) {
		masked = retry
		return parseTree(retry, name)
	})
	if got, want := string(masked), "if true; then     c () { : }; fi\n"; got != want {
		t.Fatalf("masked source = %q, want %q", got, want)
	}
}

func TestParseMultiNameFunctionFailsClosedWithoutRestoredDeclaration(t *testing.T) {
	src := []byte("a b () { : }\n")
	_, firstErr := parseTree(src, "closed.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the source")
	}
	// A retry whose tree holds no single-name declaration at the last name
	// must return the original error rather than the foreign tree.
	_, err := parseMultiNameFunctionWithParser(src, "closed.zsh", firstErr, func([]byte, string) (*syntax.File, error) {
		return parseTree([]byte("print x\n"), "closed.zsh")
	})
	if err != firstErr {
		t.Fatalf("error = %v, want the incoming error %v", err, firstErr)
	}
}

func TestParseMultiNameFunctionReportsLaterErrorAtOriginalPosition(t *testing.T) {
	const src = "a b () { : }\nprint )\n"
	assertParseErrorAt(t, []byte(src), "a command can only contain words and redirects; encountered `)`", 2, 7)
}
