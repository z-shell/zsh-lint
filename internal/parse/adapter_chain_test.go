package parse

import (
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Each snippet below is valid Zsh that requires exactly one compatibility
// adapter. Every one parses correctly on its own. Composition is what breaks:
// before the unified adapter chain, an adapter's retry re-parsed the masked
// source through a hardcoded short list of other adapters, so a second
// unrelated Zsh feature elsewhere in the same file failed to parse.
//
// Discovered in z-shell/zi zi.zsh, where `${~ZI[BIN_DIR]}` failed only because
// an alternate brace-form `if` appeared earlier in the file.
// See docs/project/2026-08-28-discovery-survey.md family F.
//
// Each snippet must actually trigger its adapter. A snippet that the base
// parser already accepts silently tests nothing: an earlier version of this
// table used a plain `(a|b)` case arm, which needs no adapter, and so missed a
// real composition failure in the grouped-case adapter. TestAdapterSnippets-
// RequireAnAdapter pins that property.
type adapterSnippet struct {
	attempt adapterAttempt
	source  string
}

var adapterSnippets = map[string]adapterSnippet{
	"nestedConditional": {attempt: parseNestedConditionalAlternation, source: "[[ $line == ((a|b)|(x\")\"y)) ]]"},
	"ifShortForm":       {attempt: parseIfShortForm, source: "if (( 1 )) x=1"},
	"assocSubscript":    {attempt: parseAssociativeSubscript, source: "print ${functions[.foo]}"},
	"secondSubscript":   {attempt: parseSecondSubscript, source: "print ${a[b][1,50]}"},
	"flagBracket":       {attempt: parseSubscriptFlagBracketPattern, source: "print ${line[(i)[a]]}"},
	"patternAfterComma": {attempt: parseSubscriptPatternAfterComma, source: "print ${line[(r)a,--[^:]##]}"},
	"lengthOperator":    {attempt: parseLengthOperator, source: "print ${#reply[@]:#skip}"},
	"tryAlways":         {attempt: parseTryAlways, source: "{ true } always { true }"},
	"multiNameFor":      {attempt: parseMultiNameFor, source: "for a b in 1 2; do print $a$b; done"},
	"ansiCHeredoc":      {attempt: parseANSICHeredocDelimiter, source: "cat <<$'E\\x4fF'\nbody\nEOF"},
	"functionSemicolon": {attempt: parseFunctionSemicolonBody, source: "function f; { print hi }"},
	"multiNameFunction": {attempt: parseMultiNameFunction, source: "a b () { print hi }"},
	"assignAlways":      {attempt: parseAssignAlways, source: "print ${x::=value}"},
	"declBraceClose":    {attempt: parseDeclarationBraceClose, source: "{ typeset -g C=1 }"},
	"doSeparator":       {attempt: parseDoLeadingSeparator, source: "while (( $# )); do; shift; done"},
	"thenSeparator":     {attempt: parseThenLeadingSeparator, source: "if true; then; print x; fi"},
	"repeat":            {attempt: parseRepeat, source: "repeat 3; do print hi; done"},
	"selectShortForm":   {attempt: parseSelectShortForm, source: "select o in a b c; break"},
	"selectParenList":   {attempt: parseSelectParenList, source: "select o (a b c) break"},
	"whileShortForm":    {attempt: parseWhileShortForm, source: "while (( i < 3 )) (( i++ ))"},
	"forShortForm":      {attempt: parseForShortForm, source: "for t in 1 2 3; print $t"},
	"arithForSublist":   {attempt: parseArithForSublist, source: "for (( i = 1; i < 3; i++ )) print $i"},
	"mathFunctionCall":  {attempt: parseMathFunctionCall, source: "print $(( sqrt(4) ))"},
	"nestedArithmetic":  {attempt: parseNestedArithmetic, source: "print \"${(l:5:)$(( a[1] ))}\""},
	// A redundant `;` needs a statement before it so the snippet composes
	// wherever it lands; a bare `;` opening the composed file would also be
	// a site, which is true but would not exercise the adapter in place.
	"redundantSeparator": {attempt: parseRedundantSeparator, source: "print composed; ;"},
	// The dangling operator must be closed by a brace inside the snippet.
	// Snippets are concatenated, so a trailing `&&` at the end of the source
	// would take the next snippet as its right operand and never be dangling.
	"danglingAndOr":  {attempt: parseDanglingAndOr, source: "c() { print composed &&\n}"},
	"whileEmptyBody": {attempt: parseWhileEmptyBody, source: "{ while (( i < 3 )) }"},
}

func parseString(t *testing.T, src string) error {
	t.Helper()
	_, err := Parse(strings.NewReader(src), "compose_test.zsh")
	return err
}

// TestAdapterSnippetsParseAlone is the control: every snippet must parse on
// its own. A failure here means the snippet is wrong, not the chain.
func TestAdapterSnippetsParseAlone(t *testing.T) {
	for name, snippet := range adapterSnippets {
		t.Run(name, func(t *testing.T) {
			if err := parseString(t, "#!/usr/bin/env zsh\n"+snippet.source+"\n"); err != nil {
				t.Fatalf("snippet must parse alone: %v", err)
			}
		})
	}
}

// TestAdapterSnippetsRequireAnAdapter guards the test data itself. Each
// snippet must FAIL the bare parser and only succeed through the adapter
// chain. Without this, a snippet the base parser already handles would make
// its composition cases vacuously pass and hide a real defect.
func TestAdapterSnippetsRequireAnAdapter(t *testing.T) {
	for name, snippet := range adapterSnippets {
		t.Run(name, func(t *testing.T) {
			src := []byte("#!/usr/bin/env zsh\n" + snippet.source + "\n")
			if _, err := parseTree(src, "compose_test.zsh"); err == nil {
				t.Fatalf("snippet needs no adapter, so it cannot detect a composition defect: %q", snippet.source)
			}
			if _, err := parseWithAdapters(src, "compose_test.zsh"); err != nil {
				t.Fatalf("snippet must parse through the adapter chain: %v", err)
			}
		})
	}
}

// TestAdapterChainCoversEveryAdapter keeps the snippet table honest as the
// chain grows: every registered adapter should be represented above.
func TestAdapterChainCoversEveryAdapter(t *testing.T) {
	if got, want := len(adapterSnippets), len(adapterChain()); got != want {
		t.Fatalf("adapter snippets = %d, registered adapters = %d", got, want)
	}
	represented := make(map[uintptr]string, len(adapterSnippets))
	for name, snippet := range adapterSnippets {
		pointer := reflect.ValueOf(snippet.attempt).Pointer()
		if previous := represented[pointer]; previous != "" {
			t.Fatalf("adapter snippets %q and %q represent the same adapter", previous, name)
		}
		represented[pointer] = name
	}
	for _, attempt := range adapterChain() {
		if represented[reflect.ValueOf(attempt).Pointer()] == "" {
			t.Fatal("registered adapter has no composition snippet")
		}
	}
}

// TestAdapterCompositionAllOrderedPairs is the regression gate. Every ordered
// pair of distinct adapter features in one file must parse, in both orders.
// Zsh imposes no ordering constraint between these constructs, so neither may
// the front end.
func TestAdapterCompositionAllOrderedPairs(t *testing.T) {
	for firstName, first := range adapterSnippets {
		for secondName, second := range adapterSnippets {
			if firstName == secondName {
				continue
			}
			t.Run(firstName+"_then_"+secondName, func(t *testing.T) {
				src := "#!/usr/bin/env zsh\n" + first.source + "\n" + second.source + "\n"
				if err := parseString(t, src); err != nil {
					t.Fatalf("composition must parse:\n%s\nerror: %v", src, err)
				}
			})
		}
	}
}

// TestAdapterCompositionAllFeaturesTogether puts every adapter feature in a
// single file, which is closer to a real loader than any pair.
//
// The order is sorted rather than map order. Iterating the map directly made
// this test fail on roughly one run in seven, because an ordering did matter:
// an alternate-form `if`, a `while` with a non-brace body, and a `try`/`always`
// block in that relative order were rejected (#337). A defect that appears in
// 15% of runs and prints a different file each time reads as flakiness, and the
// failing order was recoverable only from the printed source. Sorting fixes the
// order so a failure here always reproduces; TestAdapterCompositionRandomOrders
// keeps the randomized coverage and reports a seed that replays it.
func TestAdapterCompositionAllFeaturesTogether(t *testing.T) {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env zsh\n")
	for _, name := range sortedAdapterSnippetNames() {
		b.WriteString(adapterSnippets[name].source)
		b.WriteString("\n")
	}
	if err := parseString(t, b.String()); err != nil {
		t.Fatalf("all features together must parse:\n%s\nerror: %v", b.String(), err)
	}
}

// sortedAdapterSnippetNames gives the snippet names in a fixed order, so a
// composition failure is reproducible from the test name alone.
func sortedAdapterSnippetNames() []string {
	names := make([]string, 0, len(adapterSnippets))
	for name := range adapterSnippets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestAdapterCompositionRandomOrders keeps the coverage that map iteration used
// to provide by accident, without the unreproducibility. Each iteration derives
// its order from an explicit seed, and a failure reports the seed and the
// order, so the exact case can be replayed.
//
// n! orderings cannot be enumerated at this size, so this samples. The
// iteration count is kept low deliberately: each parse of a 24-construct file
// walks the adapter chain, so this dominates the package's test time. Forty
// seeds cost about three seconds and would have caught #337, whose failing
// orderings were roughly 1 in 300.
func TestAdapterCompositionRandomOrders(t *testing.T) {
	names := sortedAdapterSnippetNames()

	const iterations = 40
	for seed := int64(0); seed < iterations; seed++ {
		order := append([]string(nil), names...)
		rng := rand.New(rand.NewSource(seed))
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

		var b strings.Builder
		b.WriteString("#!/usr/bin/env zsh\n")
		for _, name := range order {
			b.WriteString(adapterSnippets[name].source)
			b.WriteString("\n")
		}
		if err := parseString(t, b.String()); err != nil {
			t.Fatalf(
				"composition must parse in any order; replay with seed %d\norder: %v\nsource:\n%s\nerror: %v",
				seed, order, b.String(), err,
			)
		}
	}
}

// TestGlobSubstAfterAlternateIf pins the exact minimized reproduction from
// zi.zsh so the original defect cannot regress silently.
func TestGlobSubstAfterAlternateIf(t *testing.T) {
	src := "#!/usr/bin/env zsh\nif (( 1 )) { x=1 }\np=${~q}\n"
	if err := parseString(t, src); err != nil {
		t.Fatalf("GLOB_SUBST after alternate brace-form if must parse: %v", err)
	}
}

// TestCompositionPreservesOriginalText proves the chain restores every masked
// byte. A composition that parses but returns rewritten literals would corrupt
// downstream rules and suppression handling, which is worse than a parse error.
func TestCompositionPreservesOriginalText(t *testing.T) {
	src := "#!/usr/bin/env zsh\nif (( 1 )) { x=1 }\np=${~q}\nprint -- ${^manpath}\n"
	file, err := Parse(strings.NewReader(src), "compose_test.zsh")
	if err != nil {
		t.Fatalf("composition must parse: %v", err)
	}
	printed := &strings.Builder{}
	if err := syntax.NewPrinter().Print(printed, file.AST()); err != nil {
		t.Fatalf("printing tree: %v", err)
	}
	for _, want := range []string{"${~q}", "${^manpath}"} {
		if !strings.Contains(printed.String(), want) {
			t.Errorf("original text %q not restored in AST; got:\n%s", want, printed.String())
		}
	}
}

// TestAdapterSelfRecursionDoesNotWidenGrammar pins the second defect found
// while building the chain. Composition must not let an adapter apply its own
// masking repeatedly until invalid Zsh parses. Each source below is rejected
// by native `zsh -f -n` and must stay rejected.
func TestAdapterSelfRecursionDoesNotWidenGrammar(t *testing.T) {
	cases := map[string]string{
		"triple closing paren": "case x in\n  (x|y))) : ;;\nesac\n",
		"quadruple paren":      "case x in\n  (x|y)))) : ;;\nesac\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if err := parseString(t, src); err == nil {
				t.Fatalf("adapter self-recursion widened the accepted grammar:\n%s", src)
			}
		})
	}
}

// TestInvalidSyntaxStillRejectedAfterComposition guards the opposite contract:
// a more permissive chain must not start accepting invalid Zsh. Each source
// below is rejected by native `zsh -f -n`.
func TestInvalidSyntaxStillRejectedAfterComposition(t *testing.T) {
	cases := map[string]string{
		"unclosed brace group":   "if (( 1 )) { x=1 }\n{ print hi\n",
		"unterminated expansion": "if (( 1 )) { x=1 }\np=${~\n",
		"bare reserved word":     "if (( 1 )) { x=1 }\nfi\n",
		"unclosed subscript":     "if (( 1 )) { x=1 }\nprint ${a[(I)x\n",
		"stray closing paren":    "if (( 1 )) { x=1 }\nprint )\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if err := parseString(t, "#!/usr/bin/env zsh\n"+src); err == nil {
				t.Fatalf("invalid Zsh must still be rejected:\n%s", src)
			}
		})
	}
}
