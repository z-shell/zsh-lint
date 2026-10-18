package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"

	"io"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

// Exercise the actual entry point and stdio transport without rebuilding a
// binary for every test. The parent owns the subprocess and its deadline.
func TestMCPProcess(t *testing.T) {
	if os.Getenv("ZSH_LINT_MCP_TEST_PROCESS") != "1" {
		return
	}
	os.Args = append([]string{"zsh-lint"}, os.Args[3:]...)
	main()
}

type scriptedMCP struct {
	input  io.WriteCloser
	output *bufio.Reader
	id     int
}

func startMCP(t *testing.T, args ...string) *scriptedMCP {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, append([]string{"-test.run=^TestMCPProcess$", "--", "mcp"}, args...)...)
	command.Env = append(os.Environ(), "ZSH_LINT_MCP_TEST_PROCESS=1")
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = input.Close()
		if err := command.Wait(); err != nil {
			t.Errorf("MCP process: %v; stderr: %s", err, stderr.String())
		}
	})
	client := &scriptedMCP{input: input, output: bufio.NewReader(output)}
	response := client.call(t, "initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "scripted-test", "version": "1"}})
	if response["error"] != nil {
		t.Fatalf("initialize: %v", response)
	}
	result := response["result"].(map[string]any)
	if result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("protocolVersion: %v", result)
	}
	client.send(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return client
}

func (c *scriptedMCP) send(t *testing.T, message any) {
	t.Helper()
	if err := json.NewEncoder(c.input).Encode(message); err != nil {
		t.Fatal(err)
	}
}

func (c *scriptedMCP) call(t *testing.T, method string, params any) map[string]any {
	t.Helper()
	c.id++
	c.send(t, map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	return c.response(t, c.id)
}

func (c *scriptedMCP) response(t *testing.T, id any) map[string]any {
	t.Helper()
	for {
		line, err := c.output.ReadBytes('\n')
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		var response map[string]any
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("non-protocol stdout %q: %v", line, err)
		}
		if id == nil && response["error"] != nil || response["id"] == float64(c.id) {
			return response
		}
		if response["method"] == nil {
			t.Fatalf("unexpected response: %v", response)
		}
	}
}

func TestMCPListsOnlyAnalysisTools(t *testing.T) {
	client := startMCP(t)
	response := client.call(t, "tools/list", map[string]any{})
	if response["error"] != nil {
		t.Fatal(response)
	}
	tools := response["result"].(map[string]any)["tools"].([]any)
	var names []string
	for _, value := range tools {
		tool := value.(map[string]any)
		names = append(names, tool["name"].(string))
		if tool["inputSchema"] == nil {
			t.Errorf("missing schema: %v", tool)
		}
	}
	if !reflect.DeepEqual(names, []string{"zsh_lint", "zsh_syntax"}) {
		t.Fatalf("tools = %v", names)
	}
}

func TestMCPMalformedFramesRecover(t *testing.T) {
	client := startMCP(t)
	for _, frame := range []string{"{invalid\n", "null\n", "[]\n", "{\"jsonrpc\":\"1.0\",\"id\":123,\"method\":\"tools/list\"}\n"} {
		if _, err := io.WriteString(client.input, frame); err != nil {
			t.Fatal(err)
		}
		response := client.response(t, nil)
		failure, ok := response["error"].(map[string]any)
		if !ok || (failure["code"] != float64(-32700) && failure["code"] != float64(-32600)) {
			t.Fatalf("malformed frame returned %v", response)
		}
		if result := client.call(t, "tools/list", map[string]any{}); result["error"] != nil {
			t.Fatal(result)
		}
	}
}
