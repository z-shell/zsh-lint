package workflowcontract

import (
	"os"
	"path/filepath"
	"runtime"
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

func workflowStep(t *testing.T, workflow, name, nextName string) string {
	t.Helper()

	start := strings.Index(workflow, "- name: "+name)
	if start == -1 {
		t.Fatalf("workflow is missing step %q", name)
	}
	end := strings.Index(workflow[start:], "- name: "+nextName)
	if end == -1 {
		t.Fatalf("workflow step %q is not followed by %q", name, nextName)
	}

	return workflow[start : start+end]
}

func TestWikiDocsSyncUsesHardenedGitHubAppContract(t *testing.T) {
	workflow := wikiDocsSyncWorkflow(t)
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
		t.Run(item.name, func(t *testing.T) {
			if !strings.Contains(workflow, item.snippet) {
				t.Errorf("workflow is missing %q", item.snippet)
			}
		})
	}

	if got := strings.Count(workflow, "secrets.WIKI_SYNC_APP_PRIVATE_KEY"); got != 1 {
		t.Errorf("private key secret must be consumed exactly once by token minting; got %d references", got)
	}

	tokenStep := workflowStep(t, workflow, "Mint wiki app token", "Check out zsh-lint")
	for _, snippet := range []string{"if:", "continue-on-error:"} {
		if strings.Contains(tokenStep, snippet) {
			t.Errorf("token minting must fail closed; found %q", snippet)
		}
	}

	reportStep := workflowStep(t, workflow, "Report sync PR operation", "Verify signed sync commits")
	for _, snippet := range []string{
		"PR_OPERATION: ${{ steps.sync-pr.outputs.pull-request-operation }}",
		"sync-pr-operation=${PR_OPERATION:-none}",
	} {
		if !strings.Contains(reportStep, snippet) {
			t.Errorf("operation report is missing %q", snippet)
		}
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
