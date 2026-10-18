package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The SDK's default stdio transport terminates on malformed JSON. Keep one
// bounded line as the recovery unit so invalid requests receive JSON-RPC errors
// without losing an otherwise healthy session. MCP lifecycle and dispatch stay
// in the SDK; only newline framing and error recovery live here.
type recoveringStdio struct {
	reader io.ReadCloser
	writer io.Writer
}

func (t *recoveringStdio) Connect(context.Context) (mcp.Connection, error) {
	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, 4096), mcp.DefaultMaxLineLength)
	return &stdioConnection{reader: t.reader, scanner: scanner, writer: t.writer}, nil
}

type stdioConnection struct {
	reader  io.ReadCloser
	scanner *bufio.Scanner
	writer  io.Writer
	writeMu sync.Mutex
}

func (c *stdioConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	for c.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := c.scanner.Bytes()
		var envelope map[string]json.RawMessage
		code := jsonrpc.CodeInvalidRequest
		var message jsonrpc.Message
		var err error
		if !json.Valid(line) {
			code = jsonrpc.CodeParseError
			err = fmt.Errorf("invalid JSON")
		} else if err = json.Unmarshal(line, &envelope); err == nil && envelope != nil {
			if err = validateMCPEnvelope(envelope); err == nil {
				message, err = jsonrpc.DecodeMessage(line)
			}
		} else {
			err = fmt.Errorf("expected a JSON-RPC 2.0 object")
		}
		if err == nil {
			return message, nil
		}
		// A valid notification envelope never receives a response, even when
		// its parameters are rejected before SDK dispatch.
		var version string
		var method *string
		_, hasID := envelope["id"]
		if !hasID && json.Unmarshal(envelope["jsonrpc"], &version) == nil && version == "2.0" && json.Unmarshal(envelope["method"], &method) == nil && method != nil {
			continue
		}
		// Keep even an unsupported numeric ID exact in the rejection. Passing it
		// through the SDK's float64 decoder would silently round correlation IDs.
		id := envelope["id"]
		var value any
		decoder := json.NewDecoder(bytes.NewReader(id))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			id = json.RawMessage("null")
		} else if _, text := value.(string); !text {
			if _, numeric := value.(json.Number); !numeric {
				id = json.RawMessage("null")
			}
		}
		data, encodeErr := json.Marshal(struct {
			Version string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *jsonrpc.Error  `json:"error"`
		}{"2.0", id, &jsonrpc.Error{Code: int64(code), Message: err.Error()}})
		if encodeErr != nil {
			return nil, encodeErr
		}
		if err := c.write(ctx, data); err != nil {
			return nil, err
		}
	}
	if err := c.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// The SDK represents numeric IDs through float64. Accept only the exact-integer
// range; clients needing larger correlation values can use string IDs instead.
func validMCPID(raw json.RawMessage) bool {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return false
	}
	switch value := value.(type) {
	case string:
		return true
	case json.Number:
		number, err := value.Int64()
		return err == nil && number >= -9007199254740991 && number <= 9007199254740991
	default:
		return false
	}
}

func validateMCPEnvelope(envelope map[string]json.RawMessage) error {
	var version string
	if json.Unmarshal(envelope["jsonrpc"], &version) != nil || version != "2.0" {
		return fmt.Errorf("expected a JSON-RPC 2.0 object")
	}
	id, hasID := envelope["id"]
	if hasID && !validMCPID(id) {
		return fmt.Errorf("id must be a string or an integer between -9007199254740991 and 9007199254740991")
	}
	method, request := envelope["method"]
	_, result := envelope["result"]
	_, failure := envelope["error"]
	if request {
		var name string
		if string(method) == "null" || json.Unmarshal(method, &name) != nil || result || failure {
			return fmt.Errorf("request must have a string method and no result or error")
		}
		if name == "notifications/cancelled" {
			var params map[string]json.RawMessage
			if json.Unmarshal(envelope["params"], &params) != nil || !validMCPID(params["requestId"]) {
				return fmt.Errorf("cancellation requestId must be a supported request id")
			}
		}
	} else if !hasID || result == failure {
		return fmt.Errorf("response must have an id and exactly one of result or error")
	}
	return nil
}

func (c *stdioConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := jsonrpc.EncodeMessage(message)
	if err != nil {
		return err
	}
	if response, ok := message.(*jsonrpc.Response); ok && !response.ID.IsValid() {
		// The SDK omits a zero ID, but JSON-RPC errors without a recoverable
		// request ID must explicitly carry null rather than look like a notification.
		data = append([]byte(`{"id":null,`), data[1:]...)
	}
	return c.write(ctx, data)
}

func (c *stdioConnection) write(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.writer.Write(append(data, '\n'))
	return err
}

func (c *stdioConnection) Close() error      { return c.reader.Close() }
func (c *stdioConnection) SessionID() string { return "" }
