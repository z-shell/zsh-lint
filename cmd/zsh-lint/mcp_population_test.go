package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The committed population fixes membership; external source roots are supplied
// explicitly so this integration check never fetches or scans arbitrary trees.
func TestMCPCommittedInventoryParity(t *testing.T) {
	rootsFile := os.Getenv("ZSH_LINT_MCP_POPULATION_ROOTS")
	if rootsFile == "" {
		t.Skip("set ZSH_LINT_MCP_POPULATION_ROOTS to a JSON root-alias map for committed-population parity")
	}
	data, err := os.ReadFile(rootsFile)
	if err != nil {
		t.Fatal(err)
	}
	var roots map[string]string
	if err := json.Unmarshal(data, &roots); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"inventory", "organization", "configured"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../docs/project/diagnostic-populations", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct {
				Mode  string   `json:"mode"`
				Files []string `json:"files"`
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			var files []string
			seen := make(map[string]int)
			for _, path := range manifest.Files {
				alias, relative, ok := strings.Cut(path, ":")
				if !ok || roots[alias] == "" {
					t.Fatalf("missing alias for %s", path)
				}
				// Sample each inventory root, rather than selecting only one
				// organization's files; configured checks preserve all siblings.
				if manifest.Mode == "no-config" && seen[alias] >= 8 {
					continue
				}
				seen[alias]++
				files = append(files, filepath.Join(roots[alias], filepath.FromSlash(relative)))
			}
			arguments := map[string]any{"files": files}
			args := []string{"--format=json"}
			if manifest.Mode == "no-config" {
				args = append(args, "--no-config")
				arguments["no_config"] = true
			}
			args = append(args, "--")
			args = append(args, files...)
			var stdout, stderr bytes.Buffer
			if exit := run(args, &stdout, &stderr); exit == 2 || stderr.Len() != 0 {
				t.Fatalf("CLI: %d %s", exit, stderr.String())
			}
			client := startMCP(t)
			result := toolResult(t, client, "zsh_lint", arguments)
			var expected any
			if err := json.Unmarshal(stdout.Bytes(), &expected); err != nil {
				t.Fatal(err)
			}
			if result["isError"] == true || !reflect.DeepEqual(result["structuredContent"], expected) {
				t.Fatalf("MCP/CLI mismatch for %s", name)
			}
			t.Logf("%s: %d committed inputs match CLI", name, len(files))
		})
	}
}

func TestMCPNativeCancellation(t *testing.T) {
	binary := requireNativeZsh(t)
	file := filepath.Join(t.TempDir(), "test.zsh")
	writeFile(t, file, "print ok\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := checkNativeSyntax(ctx, binary, []string{file}); err == nil {
		t.Fatal("cancelled check returned a syntax verdict")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if _, err := checkNativeSyntax(ctx, binary, []string{file}); err == nil {
		t.Fatal("expired deadline returned a syntax verdict")
	}
}
