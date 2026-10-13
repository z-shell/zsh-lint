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
