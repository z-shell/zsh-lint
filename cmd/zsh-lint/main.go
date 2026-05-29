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
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint <file.zsh> [file.zsh ...]")
		os.Exit(2)
	}

	an := analyzer.New(rules.Default()...)
	var exitNonZero bool

	for _, name := range os.Args[1:] {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			exitNonZero = true
			continue
		}

		file, err := parse.Parse(f, name)
		f.Close()

		if err != nil {
			fmt.Println(formatErr(name, err))
			exitNonZero = true
			continue
		}

		diags := an.Analyze(file, name)
		for _, d := range diags {
			// Errors and warnings cause a non-zero exit; Info/Hint do not.
			if d.Severity <= diag.Warning {
				exitNonZero = true
			}

			// Format similar to gcc/clang
			if d.Range.IsValid() {
				fmt.Printf("%s:%d:%d: [%s] %s\n", d.File, d.Range.Start.Line, d.Range.Start.Column, d.RuleID, d.Message)
			} else {
				fmt.Printf("%s: [%s] %s\n", d.File, d.RuleID, d.Message)
			}
		}
	}

	if exitNonZero {
		os.Exit(1)
	}
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
