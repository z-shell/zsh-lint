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
//
// Each file produces either an "OK   <path>" line or a "FAIL <path>" line
// followed by a greppable "<path>:<line>:<col>: <message>" diagnostic. A final
// summary line reports the total number of files surveyed, ok, and failed.
//
// It returns a process exit code: 0 if every file parsed, 1 otherwise.
func Run(names []string, w io.Writer) int {
	return RunWithOptions(names, w, Options{})
}

// Options adjusts a survey run.
type Options struct {
	// Trace, when set, receives one "TRACE <path> parses=<n>
	// adapter-depth=<d>" line per file and a closing "TRACE total" line
	// (#408). The counts come from parse.ReadStats: whole-source parses
	// through the base parser, and the deepest nesting of adapter retries.
	// Standard output is unchanged.
	Trace io.Writer
}

// RunWithOptions is Run with options.
func RunWithOptions(names []string, w io.Writer, opts Options) int {
	var failed int
	var totalParses, maxDepth int64
	for _, name := range names {
		parse.ResetStats()
		err := surveyFile(name)
		if opts.Trace != nil {
			stats := parse.ReadStats()
			totalParses += stats.TreeParses
			maxDepth = max(maxDepth, stats.MaxAdapterDepth)
			_, _ = fmt.Fprintf(opts.Trace, "TRACE %s parses=%d adapter-depth=%d\n",
				name, stats.TreeParses, stats.MaxAdapterDepth)
		}
		if err != nil {
			failed++
			// The diagnostic is emitted on its own line starting at column 0
			// (no indent) so it begins with `path:line:col:` and stays
			// greppable / consumable by editor problem matchers.
			_, _ = fmt.Fprintf(w, "FAIL %s\n%s\n", name, formatErr(name, err))
			continue
		}
		_, _ = fmt.Fprintf(w, "OK   %s\n", name)
	}
	total := len(names)
	_, _ = fmt.Fprintf(w, "\n%d file(s) surveyed, %d ok, %d failed\n", total, total-failed, failed)
	if opts.Trace != nil {
		_, _ = fmt.Fprintf(opts.Trace, "TRACE total files=%d parses=%d max-adapter-depth=%d\n",
			total, totalParses, maxDepth)
	}
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
	defer func() { _ = f.Close() }()
	_, err = parse.Parse(f, name)
	return err
}

// formatErr renders a parser/IO error as a greppable `path:line:col: message`
// line. The parser uses mvdan/sh's LangZsh variant and can return either
// syntax.ParseError or syntax.LangError. Both embed their own filename in
// Error(); errors.As yields a copy, so the filename is blanked and the caller
// controls the path. Other errors (for example, IO failures) fall back to
// `path: message`.
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
