package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The CLI and the MCP tool report the same diagnostics for executable code
// in later subscripts, which both now reach (#252).
func TestLaterSubscriptCLIAndMCPAgree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "later.zsh")
	src := "print -r -- ${a[1][$(eval \"$code\")]}\nprint -r -- ${a[1][${b[2][$(eval \"$x\")]}]}\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--format=json", "--no-config", "--", path}, &stdout, &stderr); exit == 2 || stderr.Len() != 0 {
		t.Fatalf("CLI: %d %s", exit, stderr.String())
	}
	var expected map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &expected); err != nil {
		t.Fatal(err)
	}
	evals := 0
	diagnostics, _ := expected["diagnostics"].([]any)
	for _, entry := range diagnostics {
		if d, ok := entry.(map[string]any); ok && d["rule"] == "security/eval" {
			evals++
		}
	}
	if evals != 2 {
		t.Fatalf("CLI security/eval findings = %d, want 2: %s", evals, stdout.String())
	}
	client := startMCP(t)
	result := toolResult(t, client, "zsh_lint", map[string]any{"files": []string{path}, "no_config": true})
	if result["isError"] == true || !reflect.DeepEqual(result["structuredContent"], any(expected)) {
		t.Fatalf("MCP result differs from CLI:\nMCP %v\nCLI %v", result["structuredContent"], expected)
	}
}
