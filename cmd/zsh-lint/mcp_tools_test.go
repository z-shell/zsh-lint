package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func toolResult(t *testing.T, client *scriptedMCP, name string, arguments any) map[string]any {
	t.Helper()
	response := client.call(t, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if response["error"] != nil {
		t.Fatalf("tool protocol error: %v", response)
	}
	return response["result"].(map[string]any)
}

func TestMCPLintMatchesCLI(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	writeFile(t, config, cliConfig)
	entry := filepath.Join(root, "example.plugin.zsh")
	unload := filepath.Join(root, "lib", "state.zsh")
	invalid := filepath.Join(root, "invalid.txt")
	writeFile(t, entry, "add-zsh-hook precmd _example_tick\neval $value\n")
	writeFile(t, unload, "example_plugin_unload() { unfunction example_plugin_unload }\n")
	writeFile(t, invalid, "if then\n")
	client := startMCP(t)
	tests := []struct {
		name    string
		files   []string
		flags   []string
		options map[string]any
	}{
		{name: "discovered multi-file project", files: []string{entry, unload}},
		{name: "explicit project", files: []string{entry, unload}, flags: []string{"--config", config}, options: map[string]any{"config": config}},
		{name: "disabled discovery", files: []string{entry}, flags: []string{"--no-config"}, options: map[string]any{"no_config": true}},
		{name: "parser failure is diagnostic", files: []string{invalid}, flags: []string{"--no-config"}, options: map[string]any{"no_config": true}},
		{name: "relative fixture", files: []string{"../../internal/survey/testdata/gap.zsh"}, flags: []string{"--no-config"}, options: map[string]any{"no_config": true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--format=json"}, test.flags...)
			args = append(args, "--")
			args = append(args, test.files...)
			var stdout, stderr bytes.Buffer
			exit := run(args, &stdout, &stderr)
			if exit == 2 || stderr.Len() != 0 {
				t.Fatalf("CLI failed: %d %s", exit, stderr.String())
			}
			arguments := map[string]any{"files": test.files}
			for key, value := range test.options {
				arguments[key] = value
			}
			result := toolResult(t, client, "zsh_lint", arguments)
			if result["isError"] == true {
				t.Fatalf("findings are not tool errors: %v", result)
			}
			var expected any
			if err := json.Unmarshal(stdout.Bytes(), &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result["structuredContent"], expected) {
				t.Errorf("MCP = %v, CLI = %v", result, expected)
			}
			text := result["content"].([]any)[0].(map[string]any)["text"]
			if text != stdout.String() {
				t.Errorf("MCP JSON text differs: %v", text)
			}
		})
	}
}

func TestMCPLintErrorsAndRecovery(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.zsh")
	writeFile(t, good, "print ok\n")
	client := startMCP(t)
	tests := []struct {
		name string
		args any
	}{
		{"no files", map[string]any{"files": []string{}}},
		{"wrong type", map[string]any{"files": "file.zsh"}},
		{"unknown field", map[string]any{"files": []string{good}, "source": "print ok"}},
		{"empty path", map[string]any{"files": []string{""}}},
		{"directory", map[string]any{"files": []string{root}}},
		{"missing file", map[string]any{"files": []string{filepath.Join(root, "missing.zsh")}}},
		{"conflicting config", map[string]any{"files": []string{good}, "config": "anything.json", "no_config": true}},
		{"empty config", map[string]any{"files": []string{good}, "config": ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := toolResult(t, client, "zsh_lint", test.args)
			if result["isError"] != true {
				t.Fatalf("expected tool error: %v", result)
			}
			if recovered := toolResult(t, client, "zsh_lint", map[string]any{"files": []string{good}, "no_config": true}); recovered["isError"] == true {
				t.Fatal(recovered)
			}
		})
	}
	for _, test := range []struct {
		method string
		params any
		code   float64
	}{
		{"does/not/exist", map[string]any{}, -32601},
		{"tools/call", map[string]any{"name": "not_a_tool"}, -32602},
		{"tools/call", []any{1}, -32602},
	} {
		response := client.call(t, test.method, test.params)
		failure, ok := response["error"].(map[string]any)
		if !ok || failure["code"] != test.code {
			t.Errorf("protocol error = %v, want %v", response, test.code)
		}
		if recovered := client.call(t, "tools/list", map[string]any{}); recovered["error"] != nil {
			t.Fatal(recovered)
		}
	}
}

func requireNativeZsh(t *testing.T) string {
	t.Helper()
	binary, err := exec.LookPath("zsh")
	if err != nil {
		if os.Getenv("REQUIRE_SHELLS") != "" || os.Getenv("CI") != "" {
			t.Fatalf("Zsh is required in this test environment: %v", err)
		}
		t.Skipf("native Zsh is unavailable: %v", err)
	}
	return binary
}

func TestMCPNativeSyntax(t *testing.T) {
	binary := requireNativeZsh(t)
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("TMPDIR", root)
	client := startMCP(t, "--zsh", binary)
	valid := filepath.Join(root, "valid negation.zsh")
	invalid := filepath.Join(root, "invalid.txt")
	danger := filepath.Join(root, "not executed.zsh")
	marker := filepath.Join(root, "executed")
	writeFile(t, valid, "! true\n")
	writeFile(t, invalid, "if then\n")
	writeFile(t, danger, "print forbidden > '"+marker+"'\nprint \"$(print substituted > '"+marker+"')\"\n")
	result := toolResult(t, client, "zsh_syntax", map[string]any{"files": []string{valid, invalid, danger}})
	if result["isError"] == true {
		t.Fatal(result)
	}
	var output syntaxOutput
	encoded, err := json.Marshal(result["structuredContent"])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Files) != 3 {
		t.Fatalf("files = %v", output.Files)
	}
	if !output.Files[0].Valid || output.Files[0].Syntax.ExitCode != 1 || output.Files[0].Syntax.Stderr != "" || output.Files[0].Compile.Stderr != "" {
		t.Errorf("negation must be valid: %+v", output.Files[0])
	}
	if output.Files[1].Valid || output.Files[1].Syntax.Stderr == "" || output.Files[1].Compile.Stderr == "" {
		t.Errorf("invalid syntax accepted: %+v", output.Files[1])
	}
	if !output.Files[2].Valid {
		t.Errorf("no-execution source failed: %+v", output.Files[2])
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("input executed: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".zwc") {
			t.Errorf("leaked native artifact: %s", entry.Name())
		}
	}
	for _, file := range []string{valid, invalid, danger} {
		if _, err := os.Stat(file + ".zwc"); !os.IsNotExist(err) {
			t.Errorf("artifact beside input: %v", err)
		}
	}
}

func TestMCPMissingZshDoesNotDisableLint(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.zsh")
	writeFile(t, file, "print ok\n")
	client := startMCP(t, "--zsh", filepath.Join(t.TempDir(), "missing-zsh"))
	result := toolResult(t, client, "zsh_syntax", map[string]any{"files": []string{file}})
	if result["isError"] != true {
		t.Fatal(result)
	}
	if result := toolResult(t, client, "zsh_lint", map[string]any{"files": []string{file}, "no_config": true}); result["isError"] == true {
		t.Fatal(result)
	}
}

func TestMCPPartialConfigurationFailurePreservesDiagnostics(t *testing.T) {
	badRoot := t.TempDir()
	bad := filepath.Join(badRoot, "bad.zsh")
	good := filepath.Join(t.TempDir(), "good.zsh")
	writeFile(t, filepath.Join(badRoot, "zsh-lint.json"), `{}`)
	writeFile(t, bad, "print bad\n")
	writeFile(t, good, "eval $value\n")
	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--format=json", bad, good}, &stdout, &stderr); exit != 1 || stderr.Len() == 0 {
		t.Fatalf("comparison CLI = %d %s", exit, stderr.String())
	}
	result := toolResult(t, startMCP(t), "zsh_lint", map[string]any{"files": []string{bad, good}})
	var expected any
	if err := json.Unmarshal(stdout.Bytes(), &expected); err != nil {
		t.Fatal(err)
	}
	if result["isError"] != true || !reflect.DeepEqual(result["structuredContent"], expected) {
		t.Fatalf("partial result = %v, want error plus %v", result, expected)
	}
}

func TestMCPUsage(t *testing.T) {
	for _, args := range [][]string{{"--bad"}, {"file"}, {"--zsh="}, {"--zsh=one", "--zsh=two"}} {
		var stderr bytes.Buffer
		if code := runMCP(context.Background(), args, &stderr); code != 2 || !strings.Contains(stderr.String(), mcpUsage) {
			t.Errorf("args %v: code %d stderr %q", args, code, stderr.String())
		}
	}
}
