// Package survey runs the parser front end across a set of Zsh files and
// reports, per file, whether parsing succeeded. It produces greppable
// `path:line:col: message` diagnostics for failures and a one-line summary.
//
// This is the reboot's parser-evaluation surface (issues #5, #8): it reports
// parser outcomes only and intentionally implements no lint rules yet.
package survey

import (
	"fmt"
	"io"
	"os"

	"github.com/z-shell/zsh-lint/internal/parse"
	"mvdan.cc/sh/v3/syntax"
)

// Run parses each file in names, writing per-file status and a summary to w.
// It returns a process exit code: 0 if every file parsed, 1 otherwise.
func Run(names []string, w io.Writer) int {
	var failed int
	for _, name := range names {
		if err := surveyFile(name); err != nil {
			failed++
			fmt.Fprintf(w, "FAIL %s\n  %s\n", name, formatErr(name, err))
			continue
		}
		fmt.Fprintf(w, "OK   %s\n", name)
	}
	total := len(names)
	fmt.Fprintf(w, "\n%d file(s) surveyed, %d ok, %d failed\n", total, total-failed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func surveyFile(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = parse.Parse(f, name)
	return err
}

// formatErr renders a parser/IO error as a greppable single line. When the
// error carries a source position (syntax.ParseError), it emits
// `path:line:col: message`; otherwise it falls back to `path: message`.
func formatErr(name string, err error) string {
	if perr, ok := err.(syntax.ParseError); ok {
		return fmt.Sprintf("%s:%d:%d: %s", name, perr.Pos.Line(), perr.Pos.Col(), perr.Text)
	}
	return fmt.Sprintf("%s: %v", name, err)
}
