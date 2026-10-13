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
