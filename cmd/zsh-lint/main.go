// Command zsh-lint performs static analysis of Zsh shell scripts.
package main

import (
	"errors"
	"fmt"
	"os"

	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/rules"
)

func main() {
	args := os.Args[1:]
	jsonOut := false
	if len(args) > 0 && args[0] == "--format=json" {
		jsonOut = true
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint [--format=json] <file.zsh> [file.zsh ...]")
		os.Exit(2)
	}

	an := analyzer.New(rules.Default()...)
	var all diag.Diagnostics
	var exitNonZero bool

	for _, name := range args {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			exitNonZero = true
			continue
		}

		file, err := parse.Parse(f, name)
		f.Close()

		if err != nil {
			exitNonZero = true
			if jsonOut {
				// Parser failures share the diagnostics model under the
				// reserved rule ID parse/error
				// (docs/project/output-contract.md).
				all = append(all, parseErrDiag(name, err))
			} else {
				fmt.Println(formatErr(name, err))
			}
			continue
		}

		diags := an.Analyze(file, name)
		for _, d := range diags {
			// Errors and warnings cause a non-zero exit; Info/Hint do not.
			if d.Severity <= diag.Warning {
				exitNonZero = true
			}

			if jsonOut {
				continue
			}
			// Format similar to gcc/clang
			if d.Range.IsValid() {
				fmt.Printf("%s:%d:%d: [%s] %s\n", d.File, d.Range.Start.Line, d.Range.Start.Column, d.RuleID, d.Message)
			} else {
				fmt.Printf("%s: [%s] %s\n", d.File, d.RuleID, d.Message)
			}
		}
		if jsonOut {
			all = append(all, diags...)
		}
	}

	if jsonOut {
		all.Sort()
		if err := diag.WriteJSON(os.Stdout, len(args), all); err != nil {
			fmt.Fprintf(os.Stderr, "zsh-lint: encoding JSON: %v\n", err)
			os.Exit(2)
		}
	}

	if exitNonZero {
		os.Exit(1)
	}
}

// parseErrDiag converts a parser/IO error into a parse/error diagnostic,
// extracting the source position when the front end provides one.
func parseErrDiag(name string, err error) diag.Diagnostic {
	d := diag.Diagnostic{
		RuleID:   "parse/error",
		Severity: diag.Error,
		File:     name,
	}
	var perr syntax.ParseError
	if errors.As(err, &perr) {
		d.Message = perr.Text
		d.Range = posRange(perr.Pos)
		return d
	}
	var lerr syntax.LangError
	if errors.As(err, &lerr) {
		// LangError.Error() embeds the position; the range carries it in the
		// contract, so strip the prefix to keep the message position-free.
		lerr.Filename = ""
		msg := lerr.Error()
		if lerr.Pos.IsValid() {
			prefix := fmt.Sprintf("%d:%d: ", lerr.Pos.Line(), lerr.Pos.Col())
			if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
				msg = msg[len(prefix):]
			}
		}
		d.Message = msg
		d.Range = posRange(lerr.Pos)
		return d
	}
	d.Message = err.Error()
	return d
}

// posRange converts a parser position into a zero-width diagnostic range.
func posRange(p syntax.Pos) diag.Range {
	if !p.IsValid() {
		return diag.Range{}
	}
	pos := diag.Position{Line: int(p.Line()), Column: int(p.Col()), Offset: int(p.Offset())}
	return diag.Range{Start: pos, End: pos}
}

// formatErr renders a parser/IO error as a greppable `path:line:col: message`
func formatErr(name string, err error) string {
	var perr syntax.ParseError
	if errors.As(err, &perr) {
		perr.Filename = ""
		return name + ":" + perr.Error()
	}
	var lerr syntax.LangError
	if errors.As(err, &lerr) {
		lerr.Filename = ""
		return name + ":" + lerr.Error()
	}
	return fmt.Sprintf("%s: %v", name, err)
}
