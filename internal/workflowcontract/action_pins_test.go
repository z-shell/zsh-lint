package workflowcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var immutableActionSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
var workflowUses = regexp.MustCompile(`^\s*(?:-\s*)?(?:uses|"uses"|'uses')\s*:\s*("[^"\\\r\n]*"|'[^'\r\n]*'|[^\s"'{}\[\],&#*!|>]+)(?:\s+#.*|\s*)$`)
var workflowUsesKey = regexp.MustCompile(`(?:^|[\s{,?])(?:uses|"uses"|'uses')\s*(?::|$)`)
var workflowIndirection = regexp.MustCompile(`(?:^|[\s\[{},:])(?:[&*][\pL\pN_.-]+|![A-Za-z!][^\s]*)`)
var escapedWorkflowKey = regexp.MustCompile(`(?:^|[\s{,?])"[^"\r\n]*\\[^"\r\n]*"\s*:`)
var explicitWorkflowKey = regexp.MustCompile(`(?:^|[\s{,])\?(?:\s|$)`)
var workflowExpression = regexp.MustCompile(`\$\{\{.*?\}\}`)
var workflowBlockScalar = regexp.MustCompile(`^\s*(?:-\s+)*(?:[A-Za-z_][A-Za-z0-9_-]*|"[^"\\]*"|'[^']*')\s*:\s*[|>][1-9+-]*(?:\s+#.*|\s*)$`)

// References must be single block-mapping scalars on the same line. Reject
// unsupported uses representations and indirection rather than silently
// omitting them. This is a conservative guard, not a YAML parser.
func workflowLineReference(line string) (string, error) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return "", nil
	}
	structure := workflowExpression.ReplaceAllString(line, "EXPRESSION")
	if explicitWorkflowKey.MatchString(structure) {
		return "", fmt.Errorf("explicit and multiline YAML keys are unsupported by the reference guard")
	}
	if workflowIndirection.MatchString(structure) || escapedWorkflowKey.MatchString(structure) {
		return "", fmt.Errorf("YAML anchors, aliases, tags and escaped keys are unsupported by the reference guard")
	}
	if !workflowUsesKey.MatchString(structure) {
		return "", nil
	}
	match := workflowUses.FindStringSubmatch(line)
	if match == nil || !immutableWorkflowReference(match[1]) {
		return "", fmt.Errorf("uses must contain exactly one same-line immutable reference without flow mappings or indirection")
	}
	return match[1], nil
}

func workflowReferenceViolations(name, body string) (int, []string) {
	checked := 0
	var violations []string
	blockIndent := -1
	for line, text := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(text)
		indent := len(text) - len(strings.TrimLeft(text, " "))
		if blockIndent >= 0 {
			if trimmed == "" || indent > blockIndent {
				continue
			}
			blockIndent = -1
		}
		ref, err := workflowLineReference(text)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s:%d: %v", name, line+1, err))
		} else if ref != "" {
			checked++
		}
		if !strings.HasPrefix(trimmed, "#") && workflowBlockScalar.MatchString(workflowExpression.ReplaceAllString(text, "EXPRESSION")) {
			blockIndent = indent
			// A sequence dash precedes the mapping key, not scalar content.
			// Sibling fields align with the key and must end scalar skipping.
			prefix := text[indent:]
			for strings.HasPrefix(prefix, "- ") {
				prefix = prefix[1:]
				spaces := len(prefix) - len(strings.TrimLeft(prefix, " "))
				blockIndent += 1 + spaces
				prefix = prefix[spaces:]
			}
		}
	}
	return checked, violations
}

// immutableWorkflowReference checks ref shape, not commit provenance. Only
// organization reusable workflows have the reviewed @main exception.
func immutableWorkflowReference(ref string) bool {
	if len(ref) >= 2 && (ref[0] == '"' || ref[0] == '\'') && ref[len(ref)-1] == ref[0] {
		ref = ref[1 : len(ref)-1]
	}
	if strings.HasPrefix(ref, "./") {
		return true
	}
	action, revision, ok := strings.Cut(ref, "@")
	if !ok || action == "" {
		return false
	}
	if immutableActionSHA.MatchString(revision) {
		return true
	}
	const organizationWorkflows = "z-shell/.github/.github/workflows/"
	if revision != "main" || !strings.HasPrefix(action, organizationWorkflows) {
		return false
	}
	name := strings.TrimPrefix(action, organizationWorkflows)
	return !strings.Contains(name, "/") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
}

func TestWorkflowRemoteReferencesAreImmutable(t *testing.T) {
	for _, body := range []string{
		"steps:\n  - name: |\n      Set up Go\n    uses: actions/setup-go@v7\n",
		"jobs: # label: |\n  build:\n    uses: actions/setup-go@v7\n",
	} {
		if _, violations := workflowReferenceViolations("boundary.yml", body); len(violations) == 0 {
			t.Errorf("block scalar boundary hid a mutable sibling: %s", body)
		}
	}
	ordinary := "    if: ${{ !cancelled() }}\n    if: \"${{ contains(github.event.head_commit.message, ' uses: ') }}\"\n    run: |\n      for f in *.zsh; do :; done\n      echo 'uses: actions/setup-go@v7'\n    uses: actions/checkout@0123456789012345678901234567890123456789\n"
	if checked, violations := workflowReferenceViolations("ordinary.yml", ordinary); checked != 1 || len(violations) != 0 {
		t.Errorf("ordinary expression and shell block must preserve reference discovery: checked=%d violations=%v", checked, violations)
	}
	for _, line := range []string{
		`        uses: actions/setup-go@v7`,
		`        "uses": actions/setup-go@v7`,
		`        'uses': actions/setup-go@v7`,
		`      - "uses" : "actions/setup-go@v7"`,
		`      - 'uses' : 'actions/setup-go@v7'`,
		`        uses:`,
		`      - {name: setup, uses: "actions/setup-go@v7"}`,
		`      - {"uses": actions/setup-go@v7}`,
		`      - &setup {uses: actions/setup-go@v7}`,
		`        uses: &action actions/setup-go@v7`,
		`        uses: *action`,
		`        uses: |-`,
		`        uses: >-`,
		`        uses: !<tag:yaml.org,2002:str> actions/setup-go@v7`,
		`        ? uses`,
		`      - {uses: actions/checkout@0123456789012345678901234567890123456789, uses: actions/setup-go@v7}`,
		`      - *setup`,
		`      - *setup.v1`,
		`      - *λ`,
		`        <<: *setup`,
		`        uses: actions/checkout@0123456789012345678901234567890123456789 unexpected`,
		`        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e#mutable`,
		`        uses: "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e'"`,
		`        uses: 'actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"'`,
		`        uses: "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e''"`,
		"        ? \"us\\\n          es\"\n        : actions/setup-go@v7",
		`        ? "us\`,
		`      - ? "us\`,
		`      - { ? "us\`,
		`        "\u0075ses": actions/setup-go@v7`,
		"        uses:\n          actions/setup-go@v7",
		"        uses:\n          \"actions/setup-go@v7\"",
	} {
		if _, err := workflowLineReference(line); err == nil {
			t.Errorf("active mutable or empty uses reference was not rejected: %s", line)
		}
		if _, violations := workflowReferenceViolations("unsupported.yml", line); len(violations) == 0 || !strings.HasPrefix(violations[0], "unsupported.yml:1:") {
			t.Errorf("unsupported workflow form must fail with file:line: %s: %v", line, violations)
		}
	}
	for _, line := range []string{
		`        uses: actions/checkout@0123456789012345678901234567890123456789 # v7`,
		`        "uses": "actions/checkout@0123456789012345678901234567890123456789"`,
		`      - 'uses': './.github/actions/local'`,
		`    uses: z-shell/.github/.github/workflows/org-routing.yml@main`,
	} {
		if ref, err := workflowLineReference(line); err != nil || ref == "" {
			t.Errorf("supported immutable reference rejected: %s: %v", line, err)
		}
	}
	for _, tt := range []struct {
		ref  string
		want bool
	}{
		{"actions/checkout@0123456789012345678901234567890123456789", true},
		{"actions/checkout@v7", false},
		{"actions/checkout@main", false},
		{"z-shell/.github/.github/workflows/org-routing.yml@main", true},
		{"z-shell/.github/.github/workflows/org-routing.yml@v1", false},
		{"z-shell/.github/actions/setup-zsh@main", false},
		{"other/.github/.github/workflows/check.yml@main", false},
		{"./.github/actions/local", true},
	} {
		if got := immutableWorkflowReference(tt.ref); got != tt.want {
			t.Errorf("immutableWorkflowReference(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}

	paths, err := filepath.Glob(repositoryFilePath(t, ".github", "workflows", "*"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, path := range paths {
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		count, violations := workflowReferenceViolations(filepath.Base(path), string(body))
		checked += count
		for _, violation := range violations {
			t.Error(violation)
		}
	}
	if checked == 0 {
		t.Fatal("no workflow uses references were checked")
	}
}

// wikiActionPins keeps the allowed action identities without duplicating
// Renovate-managed digests. The workflow remains the pin's single owner.
func wikiActionPins(workflow string) (map[string]string, []string) {
	allowed := map[string]bool{
		"actions/create-github-app-token": true,
		"actions/checkout":                true,
		"actions/setup-go":                true,
		"peter-evans/create-pull-request": true,
	}
	pins := make(map[string]string)
	var violations []string
	for _, line := range strings.Split(workflow, "\n") {
		text := strings.TrimSpace(line)
		if !strings.HasPrefix(text, "uses: ") {
			continue
		}
		pin := strings.TrimPrefix(text, "uses: ")
		ref := actionRef(pin)
		action, revision, ok := strings.Cut(ref, "@")
		if !ok || !allowed[action] || !immutableActionSHA.MatchString(revision) {
			violations = append(violations, "wiki sync action must retain its allowed identity and immutable SHA: "+ref)
			continue
		}
		if previous, found := pins[action]; found && previous != pin {
			violations = append(violations, "wiki sync repeated action must use one pin: "+action)
		} else {
			pins[action] = pin
		}
	}
	for action := range allowed {
		if _, ok := pins[action]; !ok {
			violations = append(violations, "wiki sync is missing pinned action: "+action)
		}
	}
	return pins, violations
}
