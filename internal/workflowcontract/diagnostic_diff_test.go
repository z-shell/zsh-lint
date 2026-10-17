package workflowcontract

import (
	"os/exec"
	"testing"
)

// Exercise the diagnostic comparison's evidence and failure contracts in the
// default lane; these do not build corpus CLIs or run mutation campaigns.
func TestDiagnosticDiffEvidence(t *testing.T) {
	cmd := exec.Command("go", "test",
		repositoryFilePath(t, ".github", "scripts", "verify-diagnostics.go"),
		repositoryFilePath(t, ".github", "scripts", "verify-diagnostics_test.go"),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("diagnostic evidence tests: %v\n%s", err, output)
	}
}
