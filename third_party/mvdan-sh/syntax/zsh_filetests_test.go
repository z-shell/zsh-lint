package syntax

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// These fork-local rows share upstream's tree, print, printer-option, native
// confirmation and fuzz-seed checks. Keep focused position and state tests too.
var zshFileTests = []fileTestCase{
	// Default printing uses the stop word known at parse time (#702).
	// https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
	fileTest(
		[]string{"cat <<$'E\\x4fF'\nbody\nEOF\necho after"},
		langFile([]*Stmt{
			{Cmd: litCall("cat"), Redirs: []*Redirect{{Op: Hdoc, Word: word(&SglQuoted{Dollar: true, Value: `E\x4fF`}), Hdoc: litWord("body\n"), HdocStop: "EOF"}}},
			litStmt("echo", "after"),
		}, LangZsh),
	),
	fileTest(
		[]string{"cat <<E$'\\x4f'F\nbody\nEOF\necho after"},
		langFile([]*Stmt{
			{Cmd: litCall("cat"), Redirs: []*Redirect{{Op: Hdoc, Word: word(lit("E"), &SglQuoted{Dollar: true, Value: `\x4f`}, lit("F")), Hdoc: litWord("body\n"), HdocStop: "EOF"}}},
			litStmt("echo", "after"),
		}, LangZsh),
	),
	fileTest(
		[]string{"cat <<$'E\\nF'\nbody\n"},
		printsAs("cat <<$'E\\nF'\nbody"),
		langFile(&Stmt{Cmd: litCall("cat"), Redirs: []*Redirect{{Op: Hdoc, Word: word(&SglQuoted{Dollar: true, Value: `E\nF`}), Hdoc: litWord("body\n"), HdocStop: "E\nF"}}}, LangZsh),
	),

	fileTest(
		[]string{"cat <<$'E\\nF'; echo x\nbody\n"},
		printsAs("cat <<$'E\\nF'; echo x\nbody"),
		langFile([]*Stmt{
			{Cmd: litCall("cat"), Redirs: []*Redirect{{Op: Hdoc, Word: word(&SglQuoted{Dollar: true, Value: `E\nF`}), Hdoc: litWord("body\n"), HdocStop: "E\nF"}}},
			litStmt("echo", "x"),
		}, LangZsh),
	),
	fileTest(
		[]string{"cat <<-$'\\tEOF'\n\tbody\nEOF\necho after"},
		langFile([]*Stmt{
			{Cmd: litCall("cat"), Redirs: []*Redirect{{Op: DashHdoc, Word: word(&SglQuoted{Dollar: true, Value: `\tEOF`}), Hdoc: litWord("\tbody\n"), HdocStop: "\tEOF"}}},
			litStmt("echo", "after"),
		}, LangZsh),
	),
	fileTest(
		[]string{"cat <<-$'\\t'\n\tbody\n\necho after"},
		langFile([]*Stmt{
			{Cmd: litCall("cat"), Redirs: []*Redirect{{Op: DashHdoc, Word: word(&SglQuoted{Dollar: true, Value: `\t`}), Hdoc: litWord("\tbody\n"), HdocStop: "\t"}}},
			litStmt("echo", "after"),
		}, LangZsh),
	),
	// Negation can return a nonzero native status without a syntax diagnostic.
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
	fileTest([]string{"! true"}, langFile(&Stmt{Negated: true, Cmd: litCall("true")}, LangZsh)),
	// Arithmetic-looking command substitution (#266).
	// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Command-Substitution
	fileTest(
		[]string{"print $(((1)) && print ok)"},
		printsAs("print $( ((1)) && print ok)"),
		langFile(call(litWord("print"), word(cmdSubst(stmt(&BinaryCmd{
			Op: AndStmt,
			X:  stmt(arithmCmd(litWord("1"))),
			Y:  litStmt("print", "ok"),
		})))), LangZsh),
	),
	// Full-string replacement from #625 keeps the same shared checks.
	fileTest(
		[]string{`${var:/a/b}`},
		langFile(&ParamExp{
			Param: lit("var"),
			Repl: &Replace{
				Whole: true,
				Orig:  litWord("a"),
				With:  litWord("b"),
			},
		}, LangZsh),
	),
	// A brace-form else must retain the else tree and print as else (#541).
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
	fileTest(
		[]string{"if [[ -n x ]] { print a } else { print b }"},
		printsAs("if [[ -n x ]]; then print a; else print b; fi"),
		langFile(&IfClause{
			Cond: stmts(&TestClause{X: &UnaryTest{Op: TsNempStr, X: litWord("x")}}),
			Then: stmts(litCall("print", "a")),
			Else: &IfClause{Then: stmts(litCall("print", "b"))},
		}, LangZsh),
	),
	// The right operand of a numeric or file comparison is one word with
	// its nested glob groups (#693).
	// https://zsh.sourceforge.io/Doc/Release/Conditional-Expressions.html
	fileTest(
		[]string{"[[ 5 -eq ((o+4)) ]]"},
		langFile(&TestClause{X: &BinaryTest{Op: TsEql, X: litWord("5"), Y: litWord("((o+4))")}}, LangZsh),
	),
	fileTest(
		[]string{"[[ x -nt a((b)c) ]]"},
		langFile(&TestClause{X: &BinaryTest{Op: TsNewer, X: litWord("x"), Y: word(lit("a"), lit("((b)c)"))}}, LangZsh),
	),
	// Empty sublists formerly needed separator adapters (#572).
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Simple-Commands-_0026-Pipelines
	fileTest([]string{"; foo"}, printsAs("foo"), langFile(litCall("foo"), LangZsh)),
	// A glued group is distinct from a literal brace word (#276).
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Reserved-Words
	fileTest([]string{"{print hi}"}, printsAs("{ print hi; }"), langFile(block(litStmt("print", "hi")), LangZsh)),
	// Short if bodies have source ends but no then/fi delimiters (#459).
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
	fileTest(
		[]string{"if ((1)) print yes"},
		printsAs("if ((1)); then print yes; fi"),
		langFile(&IfClause{Cond: stmts(arithmCmd(litWord("1"))), Then: stmts(litCall("print", "yes"))}, LangZsh),
	),
	// An unmatched subshell closer is syntax-invalid, not a runtime probe.
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
	fileTest([]string{")"}, langErr2("1:1: `)` can only be used to close a subshell", LangZsh)),
	// A `#` glued to an expansion or quote continues the word inside a
	// subshell or command substitution too, not only at the top level (#660).
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Comments
	fileTest(
		[]string{"( print ${x}#y )"},
		printsAs("(print ${x}#y)"),
		langFile(subshell(stmt(call(litWord("print"), word(&ParamExp{Param: lit("x")}, lit("#y"))))), LangZsh),
	),
	fileTest(
		[]string{"x=$( print 'a'#y )"},
		printsAs("x=$(print 'a'#y)"),
		langFile(&CallExpr{Assigns: []*Assign{{
			Name:  lit("x"),
			Value: word(cmdSubst(stmt(call(litWord("print"), word(sglQuoted("a"), lit("#y")))))),
		}}}, LangZsh),
	),
	// After `${#`, a second `#` names the parameter `#` and the rest is a
	// prefix removal, unless `}` follows at once: `${##x}` is `$#` without a
	// leading `x`, while `${##}` is the length of `$#` (#659).
	// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
	fileTest(
		[]string{"${##x}"},
		langFile(&ParamExp{Param: lit("#"), Exp: &Expansion{Op: RemSmallPrefix, Word: litWord("x")}}, LangZsh),
	),
	fileTest(
		[]string{"${###x}"},
		langFile(&ParamExp{Param: lit("#"), Exp: &Expansion{Op: RemLargePrefix, Word: litWord("x")}}, LangZsh),
	),
	fileTest(
		[]string{"${##:-q}"},
		langFile(&ParamExp{Param: lit("#"), Exp: &Expansion{Op: RemSmallPrefix, Word: litWord(":-q")}}, LangZsh),
	),
	fileTest([]string{"${##}"}, langFile(&ParamExp{Length: true, Param: lit("#")}, LangZsh)),
	// `${name:^arrayname}` and `${name:^^arrayname}` zip two arrays; the
	// colon starts an operator, not a slice (#671). Like `:|` and `:*`, the
	// argument is a word: only expansion checks that it names a parameter.
	// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion
	fileTest(
		[]string{"foo=force_expansion\necho ${foo:^bar}"},
		langErr2("2:11: ${name:^arg} is a zsh feature; tried parsing as LANG"),
		langFile(stmts(
			&CallExpr{Assigns: litAssigns("foo=force_expansion")},
			call(litWord("echo"), word(&ParamExp{
				Param: lit("foo"),
				Exp:   &Expansion{Op: ArrayZip, Word: litWord("bar")},
			})),
		), LangZsh),
	),
	fileTest(
		[]string{"foo=force_expansion\necho ${foo:^^bar}"},
		langErr2("2:11: ${name:^^arg} is a zsh feature; tried parsing as LANG"),
		langFile(stmts(
			&CallExpr{Assigns: litAssigns("foo=force_expansion")},
			call(litWord("echo"), word(&ParamExp{
				Param: lit("foo"),
				Exp:   &Expansion{Op: ArrayZipLongest, Word: litWord("bar")},
			})),
		), LangZsh),
	),
	// A third caret belongs to the argument word. An assignment word keeps the
	// native check to the lexer, which accepts it; expansion would reject `^b`.
	fileTest(
		[]string{"x=${a:^^^b}"},
		langFile(&CallExpr{Assigns: []*Assign{{
			Name:  lit("x"),
			Value: word(&ParamExp{Param: lit("a"), Exp: &Expansion{Op: ArrayZipLongest, Word: litWord("^b")}}),
		}}}, LangZsh),
	),
	fileTest(
		[]string{"${#a:^b}"},
		langFile(&ParamExp{Length: true, Param: lit("a"), Exp: &Expansion{Op: ArrayZip, Word: litWord("b")}}, LangZsh),
	),
	fileTest(
		[]string{"${a:^}"},
		langFile(&ParamExp{Param: lit("a"), Exp: &Expansion{Op: ArrayZip}}, LangZsh),
	),
	// A blank after the colon still starts an arithmetic slice.
	fileTest(
		[]string{"${a: ^b}"},
		langErr2("1:6: `^` must follow an expression", LangZsh),
	),
	// A `)` inside a delimited flag argument does not end the flags group (#644).
	// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion-Flags
	fileTest([]string{"${(j(|))w}"}, langFile(&ParamExp{Flags: lit("j(|)"), Param: lit("w")}, LangZsh)),
	fileTest([]string{"${(l.(5)..x.)w}"}, langFile(&ParamExp{Flags: lit("l.(5)..x."), Param: lit("w")}, LangZsh)),
	// An expansion at a blank may supply an arithmetic operator or operand (#672).
	// https://zsh.sourceforge.io/Doc/Release/Arithmetic-Evaluation.html#Arithmetic-Evaluation
	fileTest(
		[]string{"$((1 $op 2))", "$(( 1 $op 2 ))"},
		langFile(arithmExp(&DynamicArithm{Items: []ArithmExpr{
			litWord("1"), word(litParamExp("op")), litWord("2"),
		}}), LangZsh),
	),
	fileTest(
		[]string{"(((1) $op (2)))", "(( (1) $op (2) ))"},
		langFile(arithmCmd(&DynamicArithm{Items: []ArithmExpr{
			&ParenArithm{X: litWord("1")}, word(litParamExp("op")), &ParenArithm{X: litWord("2")},
		}}), LangZsh),
	),
	// A blank or newline inside the group that opens a case item is pattern
	// text. When the group is the whole pattern, the blanks next to its `(`,
	// `)` and each `|` are stripped; a group glued to more text keeps them
	// (#578).
	// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Complex-Commands
	fileTest(
		[]string{"case x in (a b) :;; esac", "case x in ( a b ) :;; esac"},
		printsAs("case x in (a b) : ;; esac"),
		langFile(&CaseClause{
			Word:  litWord("x"),
			Items: []*CaseItem{{Op: Break, Patterns: litWords("a b"), Stmts: litStmts(":")}},
		}, LangZsh),
	),
	fileTest(
		[]string{"case x in (*# SKIP*|b) :;; esac", "case x in ( *# SKIP* | b ) :;; esac"},
		printsAs("case x in (*# SKIP* | b) : ;; esac"),
		langFile(&CaseClause{
			Word:  litWord("x"),
			Items: []*CaseItem{{Op: Break, Patterns: litWords("*# SKIP*", "b"), Stmts: litStmts(":")}},
		}, LangZsh),
	),
	fileTest(
		[]string{"case x in (a | b)) :;; esac"},
		printsAs("case x in ((a | b)) : ;; esac"),
		langFile(&CaseClause{
			Word: litWord("x"),
			Items: []*CaseItem{{
				Op:       Break,
				Patterns: []*Word{word(lit("("), lit("a "), lit("|"), lit(" b"), lit(")"))},
				Stmts:    litStmts(":"),
			}},
		}, LangZsh),
	),
	// A newline in the group is pattern text too, which no single-line print
	// can drop; TestZshCaseGroupBlank covers it.
	// In an array assignment a bare `=` is a literal element; only a `=`
	// after an `[index]` assigns that index (#684).
	// https://zsh.sourceforge.io/Doc/Release/Parameters.html#Array-Parameters
	fileTest(
		[]string{"a=(x =)", "a=( x = )"},
		langFile(&CallExpr{Assigns: []*Assign{{
			Name:  lit("a"),
			Array: &ArrayExpr{Elems: []*ArrayElem{{Value: litWord("x")}, {Value: litWord("=")}}},
		}}}, LangZsh),
	),
	fileTest(
		[]string{"a=(=)"},
		langFile(&CallExpr{Assigns: []*Assign{{
			Name:  lit("a"),
			Array: &ArrayExpr{Elems: []*ArrayElem{{Value: litWord("=")}}},
		}}}, LangZsh),
	),
	fileTest(
		[]string{"a=([1]=x =)"},
		langFile(&CallExpr{Assigns: []*Assign{{
			Name: lit("a"),
			Array: &ArrayExpr{Elems: []*ArrayElem{
				{Index: litWord("1"), Value: litWord("x")},
				{Value: litWord("=")},
			}},
		}}}, LangZsh),
	),
	// A `;` separates array elements as a blank or newline does: Zsh reads
	// the list as words and separators (par_nl_wordlist in Src/parse.c).
	fileTest(
		[]string{"a=(a;b)", "a=(a ;b)", "a=(a; b)", "a=(;a;\n;b;)"},
		printsAs("a=(a b)"),
		langFile(&CallExpr{Assigns: []*Assign{{
			Name:  lit("a"),
			Array: &ArrayExpr{Elems: []*ArrayElem{{Value: litWord("a")}, {Value: litWord("b")}}},
		}}}, LangZsh),
	),
	fileTest(
		[]string{"a=(;)"},
		printsAs("a=()"),
		langFile(&CallExpr{Assigns: []*Assign{{Name: lit("a"), Array: &ArrayExpr{}}}}, LangZsh),
	),
	fileTest(
		[]string{"a=([1]=x;[2]=y)"},
		printsAs("a=([1]=x [2]=y)"),
		langFile(&CallExpr{Assigns: []*Assign{{
			Name: lit("a"),
			Array: &ArrayExpr{Elems: []*ArrayElem{
				{Index: litWord("1"), Value: litWord("x")},
				{Index: litWord("2"), Value: litWord("y")},
			}},
		}}}, LangZsh),
	),
	// Bash keeps rejecting the `;`, and other list terminators still end the
	// element list. So does a `;` that ends a word inside an open glob group:
	// Zsh accepts that syntax, but the word is a bad pattern when it runs.
	fileTest([]string{"a=(a;b)"}, langErr2("1:5: array element values must be words", LangBash)),
	fileTest([]string{"a=(a;;b)"}, langErr2("1:5: array element values must be words", LangZsh)),
	fileTest([]string{"a=(a;&b)"}, langErr2("1:5: array element values must be words", LangZsh)),
	fileTest([]string{"a=(a&b)"}, langErr2("1:5: array element values must be words", LangZsh)),
	// A here-document ends at the end of the file (#686), as upstream's
	// rows show for every shell but mksh. In Zsh, one whose body has not
	// started also ends at the parenthesis closing its command or process
	// substitution, as it does at a closing backquote: the lines after are
	// commands. The printer closes it inside the substitution.
	// https://zsh.sourceforge.io/Doc/Release/Redirection.html#Redirection
	fileTest(
		[]string{"echo $(cat <<EOF)\nhi\nEOF"},
		printsAs("echo $(\n\tcat <<EOF\nEOF\n)\nhi\nEOF"),
		langFile([]*Stmt{
			stmt(call(litWord("echo"), word(cmdSubst(&Stmt{
				Cmd:    litCall("cat"),
				Redirs: []*Redirect{{Op: Hdoc, Word: litWord("EOF")}},
			})))),
			litStmt("hi"),
			litStmt("EOF"),
		}, LangZsh),
	),
	fileTest(
		[]string{"cat <(cat <<EOF)\nhi"},
		printsAs("cat <(\n\tcat <<EOF\nEOF\n)\nhi"),
		langFile([]*Stmt{
			stmt(call(litWord("cat"), word(&ProcSubst{Op: CmdIn, Stmts: []*Stmt{{
				Cmd:    litCall("cat"),
				Redirs: []*Redirect{{Op: Hdoc, Word: litWord("EOF")}},
			}}}))),
			litStmt("hi"),
		}, LangZsh),
	),
	fileTest(
		[]string{"x=`cat <<EOF`"},
		printsAs("x=$(\n\tcat <<EOF\nEOF\n)"),
		langFile(&CallExpr{Assigns: []*Assign{{
			Name: lit("x"),
			Value: word(cmdSubst(&Stmt{
				Cmd:    litCall("cat"),
				Redirs: []*Redirect{{Op: Hdoc, Word: litWord("EOF")}},
			})),
		}}}, LangZsh),
	),
	// A body that started inside the substitution reads its `)` as text.
	fileTest([]string{"echo $(cat <<EOF\nhi\n)"}, langErr2("1:6: reached EOF without matching `$(` with `)`", LangZsh)),
}

func TestZshConfirmSyntaxOnly(t *testing.T) {
	externalShells[LangZsh].require(t)
	t.Run("nonzero-valid-status", confirmParse("! true", "zsh", false))
	t.Run("invalid-source-is-not-executed", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "must-not-exist")
		confirmParse(fmt.Sprintf("print touched > %q\n)", marker), "zsh", true)(t)
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("native syntax confirmation executed invalid source: %v", err)
		}
	})
}
