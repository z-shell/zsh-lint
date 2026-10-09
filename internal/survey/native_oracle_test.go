package survey

import (
	"bytes"
	"context"
	"fmt"
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
// Each entry names the issue that owns the fixture, open or closed.
var runtimeTierInvalidFixtures = map[string]runtimeTierFixture{
	"invalid-233-blank-before-parenthesis.txt":        {issue: 233, reason: "bad math expression"},
	"invalid-233-numeric-name.txt":                    {issue: 233, reason: "bad math expression"},
	"invalid-363-assignment-context.txt":              {issue: 363, reason: "bad substitution"},
	"invalid-368-missing-operand.txt":                 {issue: 368, reason: "bad math expression"},
	"invalid-368-operand-after-pattern.txt":           {issue: 368, reason: "bad math expression"},
	"invalid-382-assignment-context.txt":              {issue: 382, reason: "bad substitution"},
	"invalid-382-case-word-context.txt":               {issue: 382, reason: "bad substitution"},
	"invalid-382-second-subscript-assignment.txt":     {issue: 382, reason: "bad substitution"},
	"invalid-382-single-quote-assignment.txt":         {issue: 382, reason: "bad substitution"},
	"invalid-384-single-quoted-bracket-rebalance.txt": {issue: 384, reason: "bad substitution"},
	"invalid-540-short-flag-runtime-parens.txt":       {issue: 540, reason: "invalid subscript"},
}

// runtimeTierFixture is why Zsh rejects a runtime-tier source: the issue that
// owns it and the error Zsh reports when the line runs.
type runtimeTierFixture struct {
	issue  int
	reason string
}

// TestRuntimeTierInvalidFixturesLinkIssues checks that every runtime-tier
// entry links its owning issue, the one its `invalid-<issue>-<slug>.txt` name
// carries, and keeps its runtime error.
func TestRuntimeTierInvalidFixturesLinkIssues(t *testing.T) {
	for name, entry := range runtimeTierInvalidFixtures {
		var issue int
		if _, err := fmt.Sscanf(name, "invalid-%d-", &issue); err != nil {
			t.Errorf("%s: name does not carry an issue number: %v", name, err)
			continue
		}
		if entry.issue != issue {
			t.Errorf("%s: links issue #%d, want #%d from its name", name, entry.issue, issue)
		}
		if entry.reason == "" {
			t.Errorf("%s: records no runtime error", name)
		}
	}
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
		entry, runtimeTier := runtimeTierInvalidFixtures[name]
		switch {
		case msg == "" && !runtimeTier:
			t.Errorf("%s: zsh -f -n accepts a fixture recorded as native-invalid; "+
				"if Zsh rejects it only at run time, add it to runtimeTierInvalidFixtures with its issue and the error", name)
		case msg != "" && runtimeTier:
			t.Errorf("%s: zsh -f -n now rejects it (%s), so it is no longer runtime-tier (#%d: %s); "+
				"remove it from runtimeTierInvalidFixtures", name, msg, entry.issue, entry.reason)
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
