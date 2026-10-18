package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func TestMCPTransportInvalidRequestIDs(t *testing.T) {
	for _, frame := range []string{
		`{"jsonrpc":"1.0","id":"bad-version","method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":"bad-version","method":22}`,
	} {
		var output bytes.Buffer
		transport := &recoveringStdio{reader: io.NopCloser(strings.NewReader(frame + "\n")), writer: &output}
		connection, err := transport.Connect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := connection.Read(context.Background()); !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		var response map[string]any
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response["id"] != "bad-version" {
			t.Errorf("invalid request must preserve identifiable ID: %v", response)
		}
	}
	var output bytes.Buffer
	transport := &recoveringStdio{reader: io.NopCloser(strings.NewReader("{bad\n")), writer: &output}
	connection, err := transport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = connection.Read(context.Background())
	var response map[string]any
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if value, exists := response["id"]; !exists || value != nil {
		t.Errorf("parse error must carry null id: %s", output.String())
	}
}

func TestMCPTransportBoundsAndCancellation(t *testing.T) {
	var output bytes.Buffer
	transport := &recoveringStdio{reader: io.NopCloser(strings.NewReader(strings.Repeat("x", 16*1024*1024+1) + "\n")), writer: &output}
	connection, err := transport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Read(context.Background()); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("oversized frame must fail: %v", err)
	}
	if output.Len() != 0 {
		t.Fatal("oversized transport wrote a partial response")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := connection.Write(ctx, &jsonrpc.Response{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled write: %v", err)
	}
}
