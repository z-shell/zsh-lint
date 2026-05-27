// Command zsh-lint surveys Zsh files with the mvdan/sh parser front end and
// reports parse success or failure per file (reboot parser-evaluation phase,
// issues #5, #8). Lint rule diagnostics (issue #18) build on this foundation.
package main

import (
	"fmt"
	"os"

	"github.com/z-shell/zsh-lint/internal/survey"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint <file.zsh> [file.zsh ...]")
		os.Exit(2)
	}
	os.Exit(survey.Run(os.Args[1:], os.Stdout))
}
