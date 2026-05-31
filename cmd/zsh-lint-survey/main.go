// Command zsh-lint-survey runs the parser front end across Zsh files and
// reports parser gaps without evaluating static-analysis rules.
package main

import (
	"fmt"
	"os"

	"github.com/z-shell/zsh-lint/internal/survey"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint-survey <file.zsh> [file.zsh ...]")
		os.Exit(2)
	}
	os.Exit(survey.Run(os.Args[1:], os.Stdout))
}
