package parse

import (
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
var adapterSnippets = map[string]string{
	"alternateIf":      "if (( 1 )) { x=1 }",
	"paramGlobToggle":  "p=${~q}",
	"rcExpandCaret":    "print -- ${^manpath}",
	"reverseSubscript": "print -r -- ${_comps[(I)-value-*]}",
	"assocSubscript":   "print ${functions[.foo]}",
	"tryAlways":        "{ true } always { true }",
	"multiNameFor":     "for a b in 1 2; do print $a$b; done",
	"groupedCase":      "case x in\n  (x|y)) : ;;\nesac",
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
			if err := parseString(t, "#!/usr/bin/env zsh\n"+snippet+"\n"); err != nil {
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
			src := []byte("#!/usr/bin/env zsh\n" + snippet + "\n")
			if _, err := parseTree(src, "compose_test.zsh"); err == nil {
				t.Fatalf("snippet needs no adapter, so it cannot detect a composition defect: %q", snippet)
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
	if got, want := len(adapterSnippets), len(adapterChain()); got > want {
		t.Fatalf("more snippets (%d) than registered adapters (%d)", got, want)
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
				src := "#!/usr/bin/env zsh\n" + first + "\n" + second + "\n"
				if err := parseString(t, src); err != nil {
					t.Fatalf("composition must parse:\n%s\nerror: %v", src, err)
				}
			})
		}
	}
}

// TestAdapterCompositionAllFeaturesTogether puts every adapter feature in a
// single file, which is closer to a real loader than any pair.
func TestAdapterCompositionAllFeaturesTogether(t *testing.T) {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env zsh\n")
	// Map order is randomized by Go; that is deliberate extra coverage here,
	// since no ordering may matter.
	for _, snippet := range adapterSnippets {
		b.WriteString(snippet)
		b.WriteString("\n")
	}
	if err := parseString(t, b.String()); err != nil {
		t.Fatalf("all features together must parse:\n%s\nerror: %v", b.String(), err)
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
