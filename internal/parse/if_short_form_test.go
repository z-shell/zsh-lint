package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// ifShortForms returns every IfClause in tree whose keyword is a source `if`
// (not an elif), in source order.
func ifShortForms(tree *syntax.File, src string) []*syntax.IfClause {
	var clauses []*syntax.IfClause
	syntax.Walk(tree, func(node syntax.Node) bool {
		if clause, ok := node.(*syntax.IfClause); ok && strings.HasPrefix(src[clause.Position.Offset():], "if") {
			clauses = append(clauses, clause)
		}
		return true
	})
	return clauses
}

// Issue #210: `if list sublist`, the short form of the alternate if. Every
// source is `zsh -f -n` valid. The tree must be an IfClause at the `if` word
// whose Then holds the one sublist statement at its original position, and
// whose `then` and `fi` map to the sublist's first byte and to the byte after
// its last one.
func TestParseIfShortForm(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// then is the position of Then[0]; fi is the position of the
		// synthetic `fi`, the byte after the sublist.
		then, fi string
		// body is the source text of Then[0], from its position to the fi.
		body string
	}{
		{"arithmetic condition", "if (( i > 420 )) continue\n", "1:18", "1:26", "continue"},
		{"conditional expression", "if [[ -n $x ]] print yes\n", "1:16", "1:25", "print yes"},
		{"brace group condition", "if { false } print a; print b\n", "1:14", "1:21", "print a"},
		{"subshell condition", "if ( false ) print a; print b\n", "1:14", "1:21", "print a"},
		{"negated condition", "if ! (( 0 )) print a\n", "1:14", "1:21", "print a"},
		{"chained conditions", "if (( 1 )) && [[ -n $x ]] || ! (( 0 )) print a\n", "1:40", "1:47", "print a"},
		{"chain continues on next line", "if (( 1 )) &&\n  (( 1 )) print a; print b\n", "2:11", "2:18", "print a"},
		{"tab before sublist", "if (( 1 ))\tprint a\n", "1:12", "1:19", "print a"},
		{"continuation before sublist", "if (( 1 )) \\\n  print a; print b\n", "2:3", "2:10", "print a"},
		{"continuation glued to condition", "if (( 1 ))\\\nprint a; print b\n", "2:1", "2:8", "print a"},
		{"and chain", "if (( 0 )) print a && print b; print c\n", "1:12", "1:30", "print a && print b"},
		{"or chain", "if (( 1 )) print a || print b\n", "1:12", "1:30", "print a || print b"},
		{"pipeline", "if (( 0 )) print a | cat; print b\n", "1:12", "1:25", "print a | cat"},
		{"pipeline continues on next line", "if (( 0 )) print a |\n  cat; print b\n", "1:12", "2:6", "print a |\n  cat"},
		{"and chain continues on next line", "if (( 0 )) print a &&\n  print b; print c\n", "1:12", "2:10", "print a &&\n  print b"},
		{"negated sublist", "if (( 0 )) ! print a; print b\n", "1:12", "1:21", "! print a"},
		{"semicolon terminator", "if (( 0 )) print a; print b\n", "1:12", "1:19", "print a"},
		{"background terminator", "if (( 0 )) print a & print b\n", "1:12", "1:19", "print a"},
		{"newline terminator", "if (( 0 )) print a\nprint b\n", "1:12", "1:19", "print a"},
		{"blank lines after", "if (( 0 )) print a\n\n\nprint b\n", "1:12", "1:19", "print a"},
		{"end of input", "if (( 0 )) print a", "1:12", "1:19", "print a"},
		{"trailing comment stays with the sublist", "if (( 0 )) print a # note\nprint b\n", "1:12", "1:19", "print a"},
		{"comment line after is not the sublist", "if (( 0 )) print a\n# standalone\nprint b\n", "1:12", "1:19", "print a"},
		{"redirection", "if (( 0 )) print a >/dev/null; print b\n", "1:12", "1:30", "print a >/dev/null"},
		{"assignment", "if (( 0 )) x=1; print b\n", "1:12", "1:15", "x=1"},
		{"line continuation inside sublist", "if (( 0 )) print a \\\n  b; print c\n", "1:12", "2:4", "print a \\\n  b"},
		{"heredoc body", "if (( 0 )) cat <<EOF\nbody\nEOF\nprint b\n", "1:12", "3:4", "cat <<EOF\nbody\nEOF"},
		{"heredoc with a statement after it on its line", "if (( 0 )) cat <<EOF; print x\nbody\nEOF\nprint b\n", "1:12", "1:21", "cat <<EOF"},
		{"heredoc with a trailing comment", "if (( 0 )) cat <<EOF # c\nbody\nEOF\n", "1:12", "3:4", "cat <<EOF # c\nbody\nEOF"},
		{"heredoc at end of input", "if (( 0 )) cat <<EOF\nbody\nEOF", "1:12", "3:4", "cat <<EOF\nbody\nEOF"},
		{"loop as sublist", "if (( 0 )) for i in x y; do print $i; done; print b\n", "1:12", "1:43", "for i in x y; do print $i; done"},
		{"case as sublist", "if (( 0 )) case x in x) print a;; esac; print b\n", "1:12", "1:39", "case x in x) print a;; esac"},
		{"function as sublist", "if (( 0 )) function f { print a }; print b\n", "1:12", "1:34", "function f { print a }"},
		{"anonymous function as sublist", "if (( 0 )) () { print a }; print b\n", "1:12", "1:26", "() { print a }"},
		{"time as sublist", "if (( 0 )) time true; print b\n", "1:12", "1:21", "time true"},
		{"then as an argument", "if (( 1 )) print then\n", "1:12", "1:22", "print then"},
		{"inside a brace block", "{ if (( 0 )) print a }\nprint b\n", "1:14", "1:21", "print a"},
		{"inside a brace block with newline", "{ if (( 0 )) print a\n}\nprint b\n", "1:14", "1:21", "print a"},
		{"inside a subshell", "( if (( 0 )) print a )\n", "1:14", "1:21", "print a"},
		{"inside a command substitution", "x=$(if (( 1 )) print a); print $x\n", "1:16", "1:23", "print a"},
		{"inside a case item", "case x in x) if (( 0 )) print a;; esac; print b\n", "1:25", "1:32", "print a"},
		{"inside a classic if", "if true; then\n  if (( 0 )) print a\nelse\n  print b\nfi\n", "2:14", "2:21", "print a"},
		{"inside a brace-form if", "if [[ 1 ]] {\n  if (( 0 )) print a\n}\n", "2:14", "2:21", "print a"},
		{"inside a loop", "for i in 1 2; do\n  if (( i > 1 )) continue\n  print $i\ndone\n", "2:18", "2:26", "continue"},
		{"after a pipe", "true | if (( 1 )) print a; print b\n", "1:19", "1:26", "print a"},
		{"after and", "true && if (( 1 )) print a; print b\n", "1:20", "1:27", "print a"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			clauses := ifShortForms(file.AST(), test.src)
			var clause *syntax.IfClause
			for _, candidate := range clauses {
				if len(candidate.Cond) > 0 && candidate.Then != nil && candidate.ThenPos.String() == test.then {
					clause = candidate
				}
			}
			if clause == nil {
				t.Fatalf("no IfClause with then at %s; clauses: %d", test.then, len(clauses))
			}
			if clause.Else != nil {
				t.Errorf("Else = %v, want nil", clause.Else.Position)
			}
			if len(clause.Then) != 1 {
				t.Fatalf("len(Then) = %d, want 1", len(clause.Then))
			}
			if got := clause.Then[0].Pos().String(); got != test.then {
				t.Errorf("Then[0] position = %s, want %s", got, test.then)
			}
			if got := clause.FiPos.String(); got != test.fi {
				t.Errorf("fi position = %s, want %s", got, test.fi)
			}
			if got := test.src[clause.Then[0].Pos().Offset():clause.FiPos.Offset()]; got != test.body {
				t.Errorf("sublist text = %q, want %q", got, test.body)
			}
			if last := clause.Cond[len(clause.Cond)-1]; last.Semicolon.IsValid() {
				t.Errorf("condition keeps a synthetic separator at %s", last.Semicolon)
			}
			assertLiteralsMatchSource(t, file.AST(), test.src)
		})
	}
}

// Every short form in a file is rewritten in one pass, including one whose
// sublist is another short form; the nested clause is the whole sublist of
// the enclosing one.
func TestParseIfShortFormNestedAndRepeated(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// then and fi list the positions per clause in source order.
		then, fi []string
	}{
		{"two on consecutive lines", "if (( 0 )) print a\nif (( 0 )) print c\nprint b\n",
			[]string{"1:12", "2:12"}, []string{"1:19", "2:19"}},
		{"two on one line", "if (( 0 )) print a; if (( 0 )) print c; print b\n",
			[]string{"1:12", "1:32"}, []string{"1:19", "1:39"}},
		{"nested", "if (( 0 )) if (( 1 )) print a; print b\n",
			[]string{"1:12", "1:23"}, []string{"1:30", "1:30"}},
		{"nested with chain", "if (( 1 )) if (( 1 )) print a | tr a b && print c; print d\n",
			[]string{"1:12", "1:23"}, []string{"1:50", "1:50"}},
		{"nested after time", "if (( 0 )) time if (( 1 )) print a; print b\n",
			[]string{"1:12", "1:28"}, []string{"1:35", "1:35"}},
		{"nested in a loop sublist", "if (( 0 )) for i in x; do if (( 1 )) print $i; done; print b\n",
			[]string{"1:12", "1:38"}, []string{"1:52", "1:46"}},
		{"many", strings.Repeat("if (( 0 )) continue\n", 40),
			nil, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			clauses := ifShortForms(file.AST(), test.src)
			if test.then == nil {
				if len(clauses) != strings.Count(test.src, "if ") {
					t.Fatalf("clauses = %d, want %d", len(clauses), strings.Count(test.src, "if "))
				}
				return
			}
			if len(clauses) != len(test.then) {
				t.Fatalf("clauses = %d, want %d", len(clauses), len(test.then))
			}
			for index, clause := range clauses {
				if len(clause.Then) != 1 || clause.Else != nil {
					t.Errorf("clause %d: then=%d else=%v, want one statement and no else", index, len(clause.Then), clause.Else != nil)
					continue
				}
				if got := clause.Then[0].Pos().String(); got != test.then[index] {
					t.Errorf("clause %d Then[0] position = %s, want %s", index, got, test.then[index])
				}
				if got := clause.FiPos.String(); got != test.fi[index] {
					t.Errorf("clause %d fi position = %s, want %s", index, got, test.fi[index])
				}
			}
			assertLiteralsMatchSource(t, file.AST(), test.src)
		})
	}
}

// The retry inserts text, so every comment must keep its original position:
// the suppression pass reads comment nodes and the lines they sit on.
func TestParseIfShortFormKeepsComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"trailing", "if (( 0 )) print a # c\nprint b # d\n", []string{"1:20  c", "2:9  d"}},
		{"line after", "if (( 0 )) print a\n# c\nprint b\n", []string{"2:1  c"}},
		{"in brace condition", "if { true # c\n } print a\n", []string{"1:11  c"}},
		{"after chain operator", "if (( 1 )) && # c\n  (( 1 )) print a\n", []string{"1:15  c"}},
		{"in nested body", "if (( 0 )) if (( 1 )) print a # c\nprint b\n", []string{"1:31  c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var got []string
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if comment, ok := node.(*syntax.Comment); ok {
					got = append(got, comment.Pos().String()+" "+comment.Text)
				}
				return true
			})
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("comments = %v, want %v", got, test.want)
			}
		})
	}
}

// A retry error keeps pointing into the original file: the short form is
// accepted and the error names the gap after it.
func TestParseIfShortFormRetryErrorKeepsOriginalPosition(t *testing.T) {
	src := "if (( 0 )) print a\nprint ok\ncase x in\n  (x|y))) : ;;\nesac\n"
	_, err := Parse(strings.NewReader(src), "retry.zsh")
	if err == nil {
		t.Fatal("expected the parse error after the short form")
	}
	var parseErr syntax.ParseError
	if !asParseError(err, &parseErr) {
		t.Fatalf("error is %T, want syntax.ParseError: %v", err, err)
	}
	if parseErr.Pos.Line() != 4 {
		t.Errorf("error position = %s, want line 4: %v", parseErr.Pos, err)
	}
}

// Native Zsh rejects every source below (`zsh -f -n`), and so must the front
// end: an undelimited condition, `then`-less classic forms, an `else` after a
// short form, and a `do` sublist. The bare-`else` case matters because
// mvdan/sh reads a bare `else` in command position as a command name.
func TestParseIfShortFormRejectsInvalid(t *testing.T) {
	tests := map[string]string{
		"undelimited condition":      "if true print yes\n",
		"test builtin condition":     "if [ 1 = 1 ] print a\n",
		"newline before sublist":     "if (( 0 ))\nprint a\n",
		"else after short form":      "if (( 0 )) print a; else print b\n",
		"else on next line":          "if (( 0 )) print a\nelse\nprint b\nfi\n",
		"else after or in sublist":   "if (( 0 )) print a || else print b\n",
		"else after and in sublist":  "if (( 0 )) print a && else print b\n",
		"else after pipe in sublist": "if (( 0 )) print a | else print b\n",
		"do as sublist":              "if (( 1 )) do print a; done\n",
		"case terminator after":      "if (( 0 )) print a;; print b\n",
		"unclosed block after":       "if (( 1 )) x=1\n{ print hi\n",
		"unterminated expansion":     "if (( 1 )) x=1\np=${~\n",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), name+".zsh"); err == nil {
				t.Fatalf("invalid Zsh must be rejected:\n%s", src)
			}
		})
	}
}

// The word is the reserved word only in command position; these rows keep
// their ordinary tree and record no clause.
func TestParseIfShortFormKeepsOrdinaryUses(t *testing.T) {
	for _, src := range []string{
		"print if [[ 1 ]] x\n",
		"x='if (( 1 )) print a'\n",
		"# if (( 1 )) print a\n",
		"cat <<EOF\nif (( 1 )) print a\nEOF\n",
	} {
		file, err := Parse(strings.NewReader(src), "ordinary.zsh")
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		if got := ifShortForms(file.AST(), src); len(got) != 0 {
			t.Errorf("Parse(%q) produced %d if clauses, want 0", src, len(got))
		}
	}
}

func asParseError(err error, target *syntax.ParseError) bool {
	parseErr, ok := err.(syntax.ParseError)
	if ok {
		*target = parseErr
	}
	return ok
}
