package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPUnreadableNativeInputIsToolError(t *testing.T) {
	requireNativeZsh(t)
	file := filepath.Join(t.TempDir(), "unreadable.zsh")
	writeFile(t, file, "print ok\n")
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o600) })
	if handle, err := os.Open(file); err == nil {
		_ = handle.Close()
		t.Skip("current user can read mode-zero files")
	}
	result := toolResult(t, startMCP(t), "zsh_syntax", map[string]any{"files": []string{file}})
	if result["isError"] != true {
		t.Fatalf("unreadable input must be an execution error: %v", result)
	}
}

func TestMCPRejectsMalformedEnvelopes(t *testing.T) {
	for _, frame := range []string{
		`{"jsonrpc":"2.0","method":null}`,
		`{"jsonrpc":"2.0","id":2}`,
		`{"jsonrpc":"2.0","id":2,"method":null}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping","result":{}}`,
		`{"jsonrpc":"2.0","id":2,"result":{},"error":{"code":1,"message":"bad"}}`,
	} {
		var output bytes.Buffer
		connection, err := (&recoveringStdio{reader: io.NopCloser(strings.NewReader(frame + "\n")), writer: &output}).Connect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = connection.Read(context.Background())
		var response struct {
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatalf("no protocol response for %s: %v", frame, err)
		}
		if response.Error == nil || response.Error.Code != -32600 {
			t.Fatalf("expected invalid request for %s: %s", frame, output.String())
		}
	}
}

func TestMCPRejectsUnsafeNumericIDsWithoutRounding(t *testing.T) {
	for _, id := range []string{"9007199254740993", "-9007199254740993", "1.5", "1.0000000000000001", "1e0", "1e400", "1" + strings.Repeat("0", 400)} {
		frame := `{"jsonrpc":"2.0","id":` + id + `,"method":"ping"}`
		var output bytes.Buffer
		connection, err := (&recoveringStdio{reader: io.NopCloser(strings.NewReader(frame + "\n")), writer: &output}).Connect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = connection.Read(context.Background())
		var response struct {
			ID    json.RawMessage `json:"id"`
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatalf("missing rejection for id %s: %v", id, err)
		}
		if response.Error == nil || response.Error.Code != -32600 || string(response.ID) != id {
			t.Fatalf("ID must be rejected without normalization: %s", output.String())
		}
	}
}

func TestMCPIDBoundaries(t *testing.T) {
	for _, id := range []string{`"9007199254740993"`, `""`, "0", "-9007199254740991", "9007199254740991"} {
		if !validMCPID(json.RawMessage(id)) {
			t.Errorf("supported id rejected: %s", id)
		}
	}
	for _, id := range []string{"null", "true", "{}", "[]", "9007199254740992", "-9007199254740992"} {
		if validMCPID(json.RawMessage(id)) {
			t.Errorf("unsupported id accepted: %s", id)
		}
	}
	envelope := map[string]json.RawMessage{"jsonrpc": json.RawMessage(`"2.0"`), "method": json.RawMessage(`"notifications/cancelled"`), "params": json.RawMessage(`{"requestId":9007199254740993}`)}
	if err := validateMCPEnvelope(envelope); err == nil {
		t.Fatal("unsafe cancellation id accepted")
	}
}

func TestMCPEmptyMethodRejectedNotificationIsSilent(t *testing.T) {
	client := startMCP(t)
	params := json.RawMessage(strings.Repeat("[", 1001) + "0" + strings.Repeat("]", 1001))
	client.send(t, map[string]any{"jsonrpc": "2.0", "method": "", "params": params})
	if result := client.call(t, "ping", map[string]any{}); result["error"] != nil {
		t.Fatal(result)
	}
}

func TestMCPInvalidCancellationNotificationIsSilent(t *testing.T) {
	client := startMCP(t)
	for _, params := range []any{map[string]any{}, map[string]any{"requestId": json.Number("9007199254740993")}} {
		client.send(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": params})
		// An unsolicited error would be read instead of the barrier's response.
		if result := client.call(t, "ping", map[string]any{}); result["error"] != nil {
			t.Fatal(result)
		}
	}
}
