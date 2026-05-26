// Command zsh-lint is the entry point for the rebooted Zsh semantic analyzer.
//
// In the reboot's parser-evaluation phase (issues #5, #8) it parses each given
// file with the mvdan/sh front end and reports parse success or the first error,
// so real-world Zsh corpora can be surveyed for parser gaps. Rule diagnostics
// (issue #18) build on this foundation.
package main

import (
	"fmt"
	"os"

	"github.com/z-shell/zsh-lint/internal/parse"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint <file.zsh> [file.zsh ...]")
		os.Exit(2)
	}

	exitCode := 0
	for _, name := range os.Args[1:] {
		if err := parseFile(name); err != nil {
			fmt.Printf("FAIL %s: %v\n", name, err)
			exitCode = 1
			continue
		}
		fmt.Printf("OK   %s\n", name)
	}
	os.Exit(exitCode)
}

func parseFile(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = parse.Parse(f, name)
	return err
}
