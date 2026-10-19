package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpUsage = "usage: zsh-lint mcp [--zsh PATH]"

// mcpHelp is printed to standard output for zsh-lint mcp --help or -h, which
// exit 0 without starting the server.
const mcpHelp = mcpUsage + `

Serve Zsh analysis to an MCP client over stdio (newline-delimited JSON-RPC on
standard input and output). Configure the client to run zsh-lint with the
argument mcp. Relative paths resolve against the server working directory.

Tools:
  zsh_lint     static diagnostics for explicit files, as zsh-lint --format=json
  zsh_syntax   native zsh -f -n and zcompile checks for explicit files

Options:
  --zsh PATH   native Zsh executable for zsh_syntax (default: zsh on PATH)
  -h, --help   print this help
`

// lintInput deliberately mirrors the CLI rather than adding another rule or
// source-discovery interface. Paths resolve against the server's directory.
type lintInput struct {
	Files    []string `json:"files" jsonschema:"Explicit Zsh file paths, resolved relative to the server working directory"`
	Config   *string  `json:"config,omitempty" jsonschema:"Explicit project configuration path; mutually exclusive with no_config"`
	NoConfig bool     `json:"no_config,omitempty" jsonschema:"Disable project configuration discovery"`
}

type syntaxInput struct {
	Files []string `json:"files" jsonschema:"Explicit Zsh file paths to check with native Zsh"`
}

type nativeCheck struct {
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type syntaxFile struct {
	File    string      `json:"file"`
	Valid   bool        `json:"valid"`
	Syntax  nativeCheck `json:"syntax"`
	Compile nativeCheck `json:"compile"`
}

type syntaxOutput struct {
	Files []syntaxFile `json:"files"`
}

func runMCP(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var binary singleValue
	binary.name = "zsh"
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Var(&binary, "zsh", "native Zsh executable (default: PATH lookup)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = io.WriteString(stdout, mcpHelp)
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "zsh-lint mcp: %v\n%s\n", err, mcpUsage)
		return 2
	}
	if flags.NArg() != 0 || (binary.set && binary.value == "") {
		_, _ = fmt.Fprintln(stderr, mcpUsage)
		return 2
	}
	zsh := "zsh"
	if binary.set {
		zsh = binary.value
	}
	server := newMCPServer(zsh, stderr)
	if err := server.Run(ctx, &recoveringStdio{reader: os.Stdin, writer: stdout}); err != nil {
		_, _ = fmt.Fprintf(stderr, "zsh-lint mcp: %v\n", err)
		return 1
	}
	return 0
}

func newMCPServer(zsh string, stderr io.Writer) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "zsh-lint", Version: "1"}, &mcp.ServerOptions{
		Logger:       slog.New(slog.NewTextHandler(stderr, nil)),
		Instructions: "Analyze explicit Zsh file paths. Paths and configuration resolve against the server working directory. zsh_lint is static analysis; zsh_syntax invokes native Zsh without executing the input program. Neither tool is a sandbox for untrusted files or system startup configuration.",
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "zsh_lint", Description: "Return the same version 1 diagnostic JSON as zsh-lint --format=json, with the same configuration discovery and overrides",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, lintTool)
	mcp.AddTool(server, &mcp.Tool{
		Name: "zsh_syntax", Description: "Check Zsh files with native zsh -f -n and zcompile; nonempty stderr, not exit status alone, identifies syntax errors",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input syntaxInput) (*mcp.CallToolResult, syntaxOutput, error) {
		output, err := checkNativeSyntax(ctx, zsh, input.Files)
		return nil, output, err
	})
	return server
}

func validateFiles(files []string) error {
	if len(files) == 0 {
		return errors.New("files must contain at least one path")
	}
	for _, name := range files {
		if name == "" {
			return errors.New("file paths must not be empty")
		}
		info, err := os.Stat(name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: input must be a regular file", name)
		}
		file, err := os.Open(name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func lintTool(ctx context.Context, _ *mcp.CallToolRequest, input lintInput) (*mcp.CallToolResult, any, error) {
	if err := validateFiles(input.Files); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	args := []string{"--format=json"}
	if input.Config != nil {
		args = append(args, "--config", *input.Config)
	}
	if input.NoConfig {
		args = append(args, "--no-config")
	}
	// Preserve names (and diagnostic paths) while preventing a leading dash in
	// a filename from becoming a CLI flag.
	args = append(args, "--")
	args = append(args, input.Files...)
	var stdout, stderr bytes.Buffer
	exit := run(args, &stdout, &stderr)
	result := &mcp.CallToolResult{}
	var output any
	if stdout.Len() != 0 {
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			return nil, nil, fmt.Errorf("decode diagnostic output: %w", err)
		}
		result.Content = append(result.Content, &mcp.TextContent{Text: stdout.String()})
	}
	if stderr.Len() != 0 || exit == 2 {
		// Preserve any partial CLI envelope alongside actionable execution
		// errors. Without an envelope the error text is the only block.
		if stderr.Len() != 0 {
			result.Content = append(result.Content, &mcp.TextContent{Text: stderr.String()})
		}
		result.SetError(fmt.Errorf("analysis failed: %s", strings.TrimSpace(stderr.String())))
	}
	return result, output, nil
}

func checkNativeSyntax(ctx context.Context, zsh string, files []string) (syntaxOutput, error) {
	output := syntaxOutput{Files: make([]syntaxFile, 0, len(files))}
	if err := validateFiles(files); err != nil {
		return output, err
	}
	binary, err := exec.LookPath(zsh)
	if err != nil {
		return output, fmt.Errorf("native Zsh unavailable: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	directory, err := os.MkdirTemp("", "zsh-lint-syntax-")
	if err != nil {
		return output, err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	for index, name := range files {
		source, err := filepath.Abs(name)
		if err != nil {
			return output, err
		}
		syntax, err := nativeCommand(ctx, binary, "-f", "-n", "--", source)
		if err != nil {
			return output, err
		}
		// Source paths are positional data, never interpolated into shell code.
		// zcompile's explicit output path prevents writing beside input sources.
		compile, err := nativeCommand(ctx, binary, "-f", "-c", `builtin zcompile -U -R -- "$1" "$2"`, "zsh-lint-syntax", filepath.Join(directory, fmt.Sprintf("%d.zwc", index)), source)
		if err != nil {
			return output, err
		}
		output.Files = append(output.Files, syntaxFile{
			File: name, Valid: syntax.Stderr == "" && compile.Stderr == "", Syntax: syntax, Compile: compile,
		})
	}
	return output, nil
}

func nativeCommand(ctx context.Context, binary string, args ...string) (nativeCheck, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.WaitDelay = time.Second
	var stderr bytes.Buffer
	command.Stderr = &stderr
	err := command.Run()
	result := nativeCheck{Stderr: stderr.String()}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("run native Zsh: %w", err)
	}
	return result, nil
}
