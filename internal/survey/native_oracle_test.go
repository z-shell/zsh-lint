package survey

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// invalidFixtureDir holds the native-invalid regression sources the parser
// tests read (docs/project/parser-gap-workflow.md, "Native-invalid regression
// sources").
var invalidFixtureDir = filepath.Join("..", "parse", "testdata")

// runtimeTierInvalidFixtures are native-invalid sources that `zsh -f -n`
// accepts. Zsh rejects them only when the line runs, because `-n` neither
// evaluates arithmetic nor expands assignment words (#287). They are named
// here rather than executed: the workflow contract never runs an invalid
// source, so their verdict stays the one recorded in their parser tests.
var runtimeTierInvalidFixtures = map[string]string{
	"invalid-233-blank-before-parenthesis.txt":        "bad math expression",
	"invalid-233-numeric-name.txt":                    "bad math expression",
	"invalid-368-missing-operand.txt":                 "bad math expression",
	"invalid-368-operand-after-pattern.txt":           "bad math expression",
	"invalid-382-assignment-context.txt":              "bad substitution",
	"invalid-382-case-word-context.txt":               "bad substitution",
	"invalid-382-second-subscript-assignment.txt":     "bad substitution",
	"invalid-382-single-quote-assignment.txt":         "bad substitution",
	"invalid-384-single-quoted-bracket-rebalance.txt": "bad substitution",
}

// nativeSyntaxError runs `zsh -f -n` on path and returns its diagnostic, or
// "" when Zsh accepts the file. The verdict is whether stderr is empty, not
// the exit status: `zsh -n` exits 1 without a diagnostic for a valid negated
// pipeline such as `! true`. Files, not `-c` strings, are judged, because an
// unterminated loop header at end of input is valid in a file only.
func nativeSyntaxError(t *testing.T, zsh, path string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, zsh, "-f", "-n", path)
	cmd.Stderr = &stderr
	_ = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s: zsh -f -n did not finish: %v", path, ctx.Err())
	}
	return strings.TrimSpace(stderr.String())
}

func fixtureNames(t *testing.T, dir, prefix, suffix string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Fatalf("no %s*%s fixtures in %s", prefix, suffix, dir)
	}
	return names
}

// TestCorpusFixturesAgreeWithNativeZsh re-checks every fixture's recorded
// native verdict, so a fixture cannot drift from the oracle it was proven
// against. It skips when zsh is not installed; Go CI installs it.
func TestCorpusFixturesAgreeWithNativeZsh(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the native oracle test")
	}

	// Every corpus fixture is valid Zsh: an ok-* fixture parses, and a gap-*
	// fixture is valid Zsh the parser does not read yet. TestMinimizedCorpus
	// rejects any other name.
	corpusDir := filepath.Join("testdata", "corpus")
	for _, name := range fixtureNames(t, corpusDir, "", ".zsh") {
		if msg := nativeSyntaxError(t, zsh, filepath.Join(corpusDir, name)); msg != "" {
			t.Errorf("%s: zsh -f -n rejects a fixture recorded as valid Zsh: %s", name, msg)
		}
	}

	seen := map[string]bool{}
	for _, name := range fixtureNames(t, invalidFixtureDir, "invalid-", ".txt") {
		seen[name] = true
		msg := nativeSyntaxError(t, zsh, filepath.Join(invalidFixtureDir, name))
		reason, runtimeTier := runtimeTierInvalidFixtures[name]
		switch {
		case msg == "" && !runtimeTier:
			t.Errorf("%s: zsh -f -n accepts a fixture recorded as native-invalid; "+
				"if Zsh rejects it only at run time, add it to runtimeTierInvalidFixtures with the error", name)
		case msg != "" && runtimeTier:
			t.Errorf("%s: zsh -f -n now rejects it (%s), so it is no longer runtime-tier (%s); "+
				"remove it from runtimeTierInvalidFixtures", name, msg, reason)
		}
	}

	var stale []string
	for name := range runtimeTierInvalidFixtures {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("%s: listed in runtimeTierInvalidFixtures but not present in %s", name, invalidFixtureDir)
	}
}
