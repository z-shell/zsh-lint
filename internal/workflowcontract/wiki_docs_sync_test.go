package workflowcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func wikiDocsSyncWorkflow(t *testing.T) string {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate workflow contract test")
	}

	workflowPath := filepath.Clean(filepath.Join(
		filepath.Dir(testFile), "..", "..", ".github", "workflows", "wiki-docs-sync.yml",
	))
	contents, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}

	return string(contents)
}

type workflowLineSpan struct {
	start int
	end   int
	text  string
}

func workflowLines(contents string) []workflowLineSpan {
	var lines []workflowLineSpan
	for start := 0; start < len(contents); {
		newline := strings.IndexByte(contents[start:], '\n')
		if newline == -1 {
			lines = append(lines, workflowLineSpan{
				start: start,
				end:   len(contents),
				text:  strings.TrimSuffix(contents[start:], "\r"),
			})
			break
		}

		end := start + newline
		lines = append(lines, workflowLineSpan{
			start: start,
			end:   end + 1,
			text:  strings.TrimSuffix(contents[start:end], "\r"),
		})
		start = end + 1
	}

	return lines
}

func exactWorkflowLineSpans(contents, line string) []workflowLineSpan {
	var matches []workflowLineSpan
	for _, candidate := range workflowLines(contents) {
		if candidate.text == line {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func workflowStep(t *testing.T, workflow, name, nextName string) string {
	t.Helper()

	startLines := exactWorkflowLineSpans(workflow, "      - name: "+name)
	if len(startLines) != 1 {
		t.Fatalf("workflow step %q must occur exactly once; got %d", name, len(startLines))
	}
	nextLines := exactWorkflowLineSpans(workflow, "      - name: "+nextName)
	if len(nextLines) != 1 || nextLines[0].start <= startLines[0].start {
		t.Fatalf("workflow step %q is not followed exactly once by %q", name, nextName)
	}

	return workflow[startLines[0].start:nextLines[0].start]
}

func workflowBlock(t *testing.T, contents, header string, indent int) string {
	t.Helper()

	marker := strings.Repeat(" ", indent) + header
	markerLines := exactWorkflowLineSpans(contents, marker)
	if len(markerLines) != 1 {
		t.Fatalf(
			"workflow block %q at indent %d must occur exactly once; got %d",
			header, indent, len(markerLines),
		)
	}

	start := markerLines[0].start
	end := len(contents)
	for _, line := range workflowLines(contents) {
		if line.start < markerLines[0].end {
			continue
		}
		if strings.TrimSpace(line.text) != "" {
			lineIndent := len(line.text) - len(strings.TrimLeft(line.text, " "))
			if lineIndent <= indent {
				end = line.start
				break
			}
		}
	}

	return contents[start:end]
}

type workflowMappingField struct {
	name  string
	value string
}

func directWorkflowMapping(block string, indent int) map[string][]string {
	fields := make(map[string][]string)
	prefix := strings.Repeat(" ", indent)
	for _, line := range strings.Split(block, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		direct := strings.TrimSuffix(line[len(prefix):], "\r")
		if direct == "" || direct[0] == ' ' || strings.HasPrefix(direct, "#") {
			continue
		}

		name, value, ok := strings.Cut(direct, ":")
		if !ok {
			fields[direct] = append(fields[direct], "<missing colon>")
			continue
		}
		fields[name] = append(fields[name], strings.TrimSpace(value))
	}

	return fields
}

func exactWorkflowMappingViolations(label, block string, indent int, expected []workflowMappingField) []string {
	actual := directWorkflowMapping(block, indent)
	wanted := make(map[string]string, len(expected))
	var violations []string

	for _, field := range expected {
		wanted[field.name] = field.value
		values := actual[field.name]
		if len(values) != 1 || values[0] != field.value {
			violations = append(violations, fmt.Sprintf(
				"%s must set %s exactly once to %q; got %q", label, field.name, field.value, values,
			))
		}
	}

	var unexpected []string
	for name := range actual {
		if _, ok := wanted[name]; !ok {
			unexpected = append(unexpected, name)
		}
	}
	sort.Strings(unexpected)
	for _, name := range unexpected {
		violations = append(violations, fmt.Sprintf("%s has unexpected field %q", label, name))
	}

	return violations
}

func exactWorkflowLineViolations(label, block, line string) []string {
	if got := len(exactWorkflowLineSpans(block, line)); got != 1 {
		return []string{fmt.Sprintf("%s must contain %q exactly once; got %d", label, line, got)}
	}
	return nil
}

func exactWorkflowBlockViolations(label, block, expected string) []string {
	if block != expected {
		return []string{fmt.Sprintf("%s must match the hardened block exactly; got %q", label, block)}
	}
	return nil
}

func workflowTerminalStep(t *testing.T, workflow, name string) string {
	t.Helper()

	startLines := exactWorkflowLineSpans(workflow, "      - name: "+name)
	if len(startLines) != 1 {
		t.Fatalf("terminal workflow step %q must occur exactly once; got %d", name, len(startLines))
	}
	for _, line := range workflowLines(workflow) {
		if line.start <= startLines[0].start {
			continue
		}
		if strings.HasPrefix(line.text, "      - name: ") {
			t.Fatalf("workflow step %q must be terminal; found later step %q", name, line.text)
		}
	}

	return workflow[startLines[0].start:]
}

func exactActionUsesViolations(workflow string, expected []string) []string {
	var actual []string
	for _, line := range workflowLines(workflow) {
		if strings.HasPrefix(line.text, "        uses: ") {
			actual = append(actual, line.text)
		}
	}
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		return []string{fmt.Sprintf("action uses must match the five scoped pins exactly; got %q", actual)}
	}
	return nil
}

func exactWorkflowStepSequenceViolations(stepsBlock string, expected []string) []string {
	var actual []string
	for _, line := range workflowLines(stepsBlock) {
		if strings.HasPrefix(line.text, "      - ") {
			actual = append(actual, strings.TrimPrefix(line.text, "      - "))
		}
	}
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		return []string{fmt.Sprintf("workflow steps must match the eight named entries exactly; got %q", actual)}
	}
	return nil
}

func wikiDocsSyncContractViolations(t *testing.T, workflow string) []string {
	t.Helper()

	var violations []string
	required := []struct {
		name    string
		snippet string
	}{
		{"go.mod trigger", `- "go.mod"`},
		{"go.sum trigger", `- "go.sum"`},
		{"protected environment", "environment: wiki-sync"},
		{"serialized runs", "group: wiki-docs-sync"},
		{"non-cancelling runs", "cancel-in-progress: false"},
		{"client ID variable", "client-id: ${{ vars.WIKI_SYNC_APP_CLIENT_ID }}"},
		{"private key secret", "private-key: ${{ secrets.WIKI_SYNC_APP_PRIVATE_KEY }}"},
		{"contents down-scope", "permission-contents: write"},
		{"pull request down-scope", "permission-pull-requests: write"},
		{"organization scope", "owner: z-shell"},
		{"repository scope", "repositories: wiki"},
		{"app token action pin", "actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1"},
		{"checkout action pin", "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0"},
		{"setup Go action pin", "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16"},
		{"pull request action pin", "peter-evans/create-pull-request@5f6978faf089d4d20b00c7766989d076bb2fc7f1"},
		{"pull request step ID", "id: sync-pr"},
		{"generated path scope", "add-paths: community/04_zsh_lint/index.mdx"},
		{"verified commit output", "pull-request-commits-verified"},
		{"stable operation report", "sync-pr-operation=${PR_OPERATION:-none}"},
		{"fixed branch", "branch: docs-sync/zsh-lint"},
		{"next base", "base: next"},
		{"signed commits", "sign-commits: true"},
		{"branch cleanup", "delete-branch: true"},
		{"created verification condition", "steps.sync-pr.outputs.pull-request-operation == 'created'"},
		{"updated verification condition", "steps.sync-pr.outputs.pull-request-operation == 'updated'"},
		{"verification failure", "::error::The created or updated sync PR contains an unverified commit."},
		{"nonzero verification exit", "exit 1"},
	}

	for _, item := range required {
		if !strings.Contains(workflow, item.snippet) {
			violations = append(violations, fmt.Sprintf("%s: workflow is missing %q", item.name, item.snippet))
		}
	}

	onBlock := workflowBlock(t, workflow, "on:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"top-level triggers",
		onBlock,
		2,
		[]workflowMappingField{
			{name: "push", value: ""},
			{name: "workflow_dispatch", value: "{}"},
		},
	)...)
	pushBlock := workflowBlock(t, onBlock, "push:", 2)
	violations = append(violations, exactWorkflowMappingViolations(
		"push trigger",
		pushBlock,
		4,
		[]workflowMappingField{
			{name: "branches", value: "[main]"},
			{name: "paths", value: ""},
		},
	)...)
	pathsBlock := workflowBlock(t, pushBlock, "paths:", 4)
	violations = append(violations, exactWorkflowBlockViolations(
		"push paths",
		pathsBlock,
		"    paths:\n"+
			"      - \"**/*.go\"\n"+
			"      - \"go.mod\"\n"+
			"      - \"go.sum\"\n"+
			"      - \".gomarkdoc.yml\"\n"+
			"      - \"internal/wikidoc/**\"\n"+
			"      - \"cmd/wikidoc/**\"\n"+
			"      - \".github/workflows/wiki-docs-sync.yml\"\n",
	)...)

	permissionsBlock := workflowBlock(t, workflow, "permissions:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"top-level permissions",
		permissionsBlock,
		2,
		[]workflowMappingField{{name: "contents", value: "read"}},
	)...)
	concurrencyBlock := workflowBlock(t, workflow, "concurrency:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"concurrency",
		concurrencyBlock,
		2,
		[]workflowMappingField{
			{name: "group", value: "wiki-docs-sync"},
			{name: "cancel-in-progress", value: "false"},
		},
	)...)
	jobsBlock := workflowBlock(t, workflow, "jobs:", 0)
	syncJobBlock := workflowBlock(t, jobsBlock, "sync:", 2)
	violations = append(violations, exactWorkflowMappingViolations(
		"sync job",
		syncJobBlock,
		4,
		[]workflowMappingField{
			{name: "runs-on", value: "ubuntu-latest"},
			{name: "environment", value: "wiki-sync"},
			{name: "steps", value: ""},
		},
	)...)
	stepsBlock := workflowBlock(t, syncJobBlock, "steps:", 4)
	violations = append(violations, exactWorkflowStepSequenceViolations(
		stepsBlock,
		[]string{
			"name: Mint wiki app token",
			"name: Check out zsh-lint",
			"name: Check out wiki (next)",
			"name: Set up Go",
			"name: Generate and inject reference",
			"name: Open or update sync PR",
			"name: Report sync PR operation",
			"name: Verify signed sync commits",
		},
	)...)
	violations = append(violations, exactActionUsesViolations(
		workflow,
		[]string{
			"        uses: actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0",
			"        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0",
			"        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0",
			"        uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0",
			"        uses: peter-evans/create-pull-request@5f6978faf089d4d20b00c7766989d076bb2fc7f1 # v8.1.1",
		},
	)...)

	if got := strings.Count(workflow, "secrets.WIKI_SYNC_APP_PRIVATE_KEY"); got != 1 {
		violations = append(violations, fmt.Sprintf(
			"private key secret must be consumed exactly once by token minting; got %d references", got,
		))
	}

	tokenStep := workflowStep(t, workflow, "Mint wiki app token", "Check out zsh-lint")
	violations = append(violations, exactWorkflowMappingViolations(
		"token minting step fields",
		tokenStep,
		8,
		[]workflowMappingField{
			{name: "id", value: "app-token"},
			{name: "uses", value: "actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0"},
			{name: "with", value: ""},
		},
	)...)
	violations = append(violations, exactWorkflowLineViolations(
		"token minting step",
		tokenStep,
		"        uses: actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0",
	)...)
	tokenInputs := workflowBlock(t, tokenStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"token minting inputs",
		tokenInputs,
		10,
		[]workflowMappingField{
			{name: "client-id", value: "${{ vars.WIKI_SYNC_APP_CLIENT_ID }}"},
			{name: "private-key", value: "${{ secrets.WIKI_SYNC_APP_PRIVATE_KEY }}"},
			{name: "owner", value: "z-shell"},
			{name: "repositories", value: "wiki"},
			{name: "permission-contents", value: "write"},
			{name: "permission-pull-requests", value: "write"},
		},
	)...)
	for _, snippet := range []string{"if:", "continue-on-error:"} {
		if strings.Contains(tokenStep, snippet) {
			violations = append(violations, fmt.Sprintf("token minting must fail closed; found %q", snippet))
		}
	}

	sourceCheckoutStep := workflowStep(t, workflow, "Check out zsh-lint", "Check out wiki (next)")
	violations = append(violations, exactWorkflowMappingViolations(
		"source checkout step fields",
		sourceCheckoutStep,
		8,
		[]workflowMappingField{
			{name: "uses", value: "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0"},
			{name: "with", value: ""},
		},
	)...)
	violations = append(violations, exactWorkflowLineViolations(
		"source checkout step",
		sourceCheckoutStep,
		"        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0",
	)...)
	sourceCheckoutInputs := workflowBlock(t, sourceCheckoutStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"source checkout inputs",
		sourceCheckoutInputs,
		10,
		[]workflowMappingField{{name: "path", value: "zsh-lint"}},
	)...)

	wikiCheckoutStep := workflowStep(t, workflow, "Check out wiki (next)", "Set up Go")
	violations = append(violations, exactWorkflowMappingViolations(
		"wiki checkout step fields",
		wikiCheckoutStep,
		8,
		[]workflowMappingField{
			{name: "uses", value: "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0"},
			{name: "with", value: ""},
		},
	)...)
	violations = append(violations, exactWorkflowLineViolations(
		"wiki checkout step",
		wikiCheckoutStep,
		"        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0",
	)...)
	wikiCheckoutInputs := workflowBlock(t, wikiCheckoutStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"wiki checkout inputs",
		wikiCheckoutInputs,
		10,
		[]workflowMappingField{
			{name: "repository", value: "z-shell/wiki"},
			{name: "ref", value: "next"},
			{name: "path", value: "wiki"},
			{name: "token", value: "${{ steps.app-token.outputs.token }}"},
			{name: "persist-credentials", value: "false"},
		},
	)...)

	setupGoStep := workflowStep(t, workflow, "Set up Go", "Generate and inject reference")
	violations = append(violations, exactWorkflowMappingViolations(
		"setup Go step fields",
		setupGoStep,
		8,
		[]workflowMappingField{
			{name: "uses", value: "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0"},
			{name: "with", value: ""},
		},
	)...)
	violations = append(violations, exactWorkflowLineViolations(
		"setup Go step",
		setupGoStep,
		"        uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0",
	)...)
	setupGoInputs := workflowBlock(t, setupGoStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"setup Go inputs",
		setupGoInputs,
		10,
		[]workflowMappingField{
			{name: "go-version", value: `"1.25"`},
			{name: "cache-dependency-path", value: "zsh-lint/go.sum"},
		},
	)...)

	pullRequestStep := workflowStep(t, workflow, "Open or update sync PR", "Report sync PR operation")
	violations = append(violations, exactWorkflowMappingViolations(
		"pull request step fields",
		pullRequestStep,
		8,
		[]workflowMappingField{
			{name: "id", value: "sync-pr"},
			{name: "uses", value: "peter-evans/create-pull-request@5f6978faf089d4d20b00c7766989d076bb2fc7f1 # v8.1.1"},
			{name: "with", value: ""},
		},
	)...)
	violations = append(violations, exactWorkflowLineViolations(
		"pull request step",
		pullRequestStep,
		"        uses: peter-evans/create-pull-request@5f6978faf089d4d20b00c7766989d076bb2fc7f1 # v8.1.1",
	)...)
	pullRequestInputs := workflowBlock(t, pullRequestStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"pull request inputs",
		pullRequestInputs,
		10,
		[]workflowMappingField{
			{name: "token", value: "${{ steps.app-token.outputs.token }}"},
			{name: "path", value: "wiki"},
			{name: "add-paths", value: "community/04_zsh_lint/index.mdx"},
			{name: "sign-commits", value: "true"},
			{name: "base", value: "next"},
			{name: "branch", value: "docs-sync/zsh-lint"},
			{name: "title", value: `"docs(zsh-lint): sync generated reference"`},
			{name: "commit-message", value: `"docs(zsh-lint): sync generated reference from zsh-lint"`},
			{name: "body", value: "|"},
			{name: "delete-branch", value: "true"},
		},
	)...)

	if got := strings.Count(workflow, "${{ steps.app-token.outputs.token }}"); got != 2 {
		violations = append(violations, fmt.Sprintf(
			"app token must be consumed exactly twice; got %d references", got,
		))
	}

	reportStep := workflowStep(t, workflow, "Report sync PR operation", "Verify signed sync commits")
	for _, snippet := range []string{
		"PR_OPERATION: ${{ steps.sync-pr.outputs.pull-request-operation }}",
		"sync-pr-operation=${PR_OPERATION:-none}",
	} {
		if !strings.Contains(reportStep, snippet) {
			violations = append(violations, fmt.Sprintf("operation report is missing %q", snippet))
		}
	}

	verificationStep := workflowTerminalStep(t, syncJobBlock, "Verify signed sync commits")
	violations = append(violations, exactWorkflowMappingViolations(
		"signed verification step fields",
		verificationStep,
		8,
		[]workflowMappingField{
			{name: "if", value: ">-"},
			{name: "env", value: ""},
			{name: "run", value: "|"},
		},
	)...)
	verificationIfBlock := workflowBlock(t, verificationStep, "if: >-", 8)
	violations = append(violations, exactWorkflowBlockViolations(
		"signed verification condition",
		verificationIfBlock,
		"        if: >-\n"+
			"          steps.sync-pr.outputs.pull-request-operation == 'created' ||\n"+
			"          steps.sync-pr.outputs.pull-request-operation == 'updated'\n",
	)...)
	verificationEnvBlock := workflowBlock(t, verificationStep, "env:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"signed verification environment",
		verificationEnvBlock,
		10,
		[]workflowMappingField{
			{name: "COMMITS_VERIFIED", value: "${{ steps.sync-pr.outputs.pull-request-commits-verified }}"},
		},
	)...)
	verificationRunBlock := workflowBlock(t, verificationStep, "run: |", 8)
	violations = append(violations, exactWorkflowBlockViolations(
		"signed verification run",
		verificationRunBlock,
		"        run: |\n"+
			"          if [ \"$COMMITS_VERIFIED\" != \"true\" ]; then\n"+
			"            echo \"::error::The created or updated sync PR contains an unverified commit.\"\n"+
			"            exit 1\n"+
			"          fi\n",
	)...)

	return violations
}

func mutateWorkflow(t *testing.T, workflow, old, replacement string) string {
	t.Helper()

	if got := strings.Count(workflow, old); got != 1 {
		t.Fatalf("mutation target %q must occur exactly once; got %d occurrences", old, got)
	}

	return strings.Replace(workflow, old, replacement, 1)
}

func TestWikiDocsSyncUsesHardenedGitHubAppContract(t *testing.T) {
	for _, violation := range wikiDocsSyncContractViolations(t, wikiDocsSyncWorkflow(t)) {
		t.Error(violation)
	}
}

func TestWikiDocsSyncRejectsPrivilegeAndTokenMutations(t *testing.T) {
	workflow := wikiDocsSyncWorkflow(t)
	tests := []struct {
		name        string
		old         string
		replacement string
	}{
		{
			name:        "top-level contents write",
			old:         "permissions:\n  contents: read",
			replacement: "permissions:\n  contents: write",
		},
		{
			name:        "extra repository",
			old:         "repositories: wiki",
			replacement: "repositories: wiki,zsh-lint",
		},
		{
			name:        "extra permission",
			old:         "permission-pull-requests: write",
			replacement: "permission-pull-requests: write\n          permission-issues: write",
		},
		{
			name: "wiki checkout token miswire",
			old: "          token: ${{ steps.app-token.outputs.token }}\n" +
				"          persist-credentials: false",
			replacement: "          token: ${{ github.token }}\n" +
				"          persist-credentials: false",
		},
		{
			name:        "persist credentials true",
			old:         "persist-credentials: false",
			replacement: "persist-credentials: true",
		},
		{
			name: "pull request token miswire",
			old: "          token: ${{ steps.app-token.outputs.token }}\n" +
				"          path: wiki",
			replacement: "          token: ${{ github.token }}\n" +
				"          path: wiki",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := mutateWorkflow(t, workflow, tt.old, tt.replacement)
			if violations := wikiDocsSyncContractViolations(t, mutated); len(violations) == 0 {
				t.Error("mutated workflow unexpectedly satisfies the hardened contract")
			}
		})
	}
}

func TestWikiDocsSyncRejectsTriggerIdentityActionAndVerificationMutations(t *testing.T) {
	workflow := wikiDocsSyncWorkflow(t)
	tests := []struct {
		name        string
		old         string
		replacement string
	}{
		{
			name:        "signed verification accepts false",
			old:         `if [ "$COMMITS_VERIFIED" != "true" ]; then`,
			replacement: `if [ "$COMMITS_VERIFIED" == "true" ]; then`,
		},
		{
			name:        "token action mutable ref",
			old:         "        uses: actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0",
			replacement: "        uses: actions/create-github-app-token@v3",
		},
		{
			name: "source checkout mutable ref",
			old: "      - name: Check out zsh-lint\n" +
				"        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0",
			replacement: "      - name: Check out zsh-lint\n" +
				"        uses: actions/checkout@v7",
		},
		{
			name: "wiki checkout mutable ref",
			old: "      - name: Check out wiki (next)\n" +
				"        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0",
			replacement: "      - name: Check out wiki (next)\n" +
				"        uses: actions/checkout@v7",
		},
		{
			name:        "setup Go action mutable ref",
			old:         "        uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0",
			replacement: "        uses: actions/setup-go@v6",
		},
		{
			name:        "pull request action mutable ref",
			old:         "        uses: peter-evans/create-pull-request@5f6978faf089d4d20b00c7766989d076bb2fc7f1 # v8.1.1",
			replacement: "        uses: peter-evans/create-pull-request@v8",
		},
		{
			name:        "broken app token step ID",
			old:         "        id: app-token",
			replacement: "        id: app-token-broken",
		},
		{
			name:        "prefix-spoofed pull request step ID",
			old:         "        id: sync-pr",
			replacement: "        id: sync-pr-broken",
		},
		{
			name:        "push includes next",
			old:         "    branches: [main]",
			replacement: "    branches: [main, next]",
		},
		{
			name:        "pull request trigger added",
			old:         "  workflow_dispatch: {}",
			replacement: "  workflow_dispatch: {}\n  pull_request: {}",
		},
		{
			name:        "concurrency group broadened by suffix",
			old:         "  group: wiki-docs-sync",
			replacement: "  group: wiki-docs-sync-${{ github.ref }}",
		},
		{
			name:        "in-progress runs cancelled",
			old:         "  cancel-in-progress: false",
			replacement: "  cancel-in-progress: true",
		},
		{
			name: "unnamed action step inserted",
			old: "          path: zsh-lint\n\n" +
				"      - name: Check out wiki (next)",
			replacement: "          path: zsh-lint\n\n" +
				"      - uses: attacker/action@1111111111111111111111111111111111111111\n\n" +
				"      - name: Check out wiki (next)",
		},
		{
			name:        "unnamed terminal run step appended",
			old:         "          fi\n",
			replacement: "          fi\n      - run: echo attacker\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := mutateWorkflow(t, workflow, tt.old, tt.replacement)
			if violations := wikiDocsSyncContractViolations(t, mutated); len(violations) == 0 {
				t.Error("mutated workflow unexpectedly satisfies the hardened contract")
			}
		})
	}
}

func TestWikiDocsSyncRejectsLegacyCredentialContract(t *testing.T) {
	workflow := wikiDocsSyncWorkflow(t)
	forbidden := []string{
		"app-id:",
		"WIKI_SYNC_APP_ID",
		"WIKI_SYNC_TOKEN",
		"steps.app.outputs.present",
		"skip-token-revoke: true",
		"continue-on-error: true",
	}

	for _, snippet := range forbidden {
		t.Run(snippet, func(t *testing.T) {
			if strings.Contains(workflow, snippet) {
				t.Errorf("workflow still contains legacy contract %q", snippet)
			}
		})
	}
}
