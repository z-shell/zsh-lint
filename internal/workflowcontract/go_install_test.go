package workflowcontract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Versioned go install ignores the caller's main module. Local replacements
// therefore break the documented installation path even when local builds pass.
func TestVersionedInstallModule(t *testing.T) {
	root := repositoryFilePath(t)
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var module struct {
		Replace []json.RawMessage
		Exclude []json.RawMessage
	}
	if err := json.Unmarshal(out, &module); err != nil {
		t.Fatal(err)
	}
	if len(module.Replace) != 0 || len(module.Exclude) != 0 {
		t.Errorf("versioned go install requires a module without replace or exclude directives")
	}
	// A nested module is omitted from the root module's downloadable zip.
	if _, err := os.Stat(filepath.Join(root, "third_party", "mvdan-sh", "go.mod")); !os.IsNotExist(err) {
		t.Errorf("parser fork must ship inside the root module, not as a nested module: %v", err)
	}
}

func TestCLIUsesInModuleParserFork(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/zsh-lint")
	cmd.Dir = repositoryFilePath(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range strings.Fields(string(out)) {
		if name == "mvdan.cc/sh/v3/syntax" {
			t.Error("CLI must use the in-module parser import path")
		}
		if name == "github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax" {
			found = true
		}
	}
	if !found {
		t.Error("CLI dependency graph does not contain the in-module parser fork")
	}
}
