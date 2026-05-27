// Package survey runs the parser front end across a set of Zsh files and
// reports, per file, whether parsing succeeded. It produces greppable
// `path:line:col: message` diagnostics for failures and a one-line summary.
//
// This is the reboot's parser-evaluation surface (issues #5, #8): it reports
// parser outcomes only and intentionally implements no lint rules yet.
package survey

import (
	"errors"
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

// formatErr renders a parser/IO error as a greppable `path:line:col: message`
// line. The mvdan/sh front end reports syntax errors as syntax.ParseError and
// zsh-only constructs (the parser runs in bash mode) as syntax.LangError. Both
// embed their own filename in Error(); since errors.As yields a copy, the
// filename is blanked so the path can be controlled by the caller. Anything
// else (e.g. an IO error) falls back to `path: message`.
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
