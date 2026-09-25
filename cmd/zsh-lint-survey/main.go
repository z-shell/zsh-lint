// Command zsh-lint-survey runs the parser front end across Zsh files and
// reports parser gaps without evaluating static-analysis rules.
//
// With -trace-parses it also writes, on standard error, how many whole-source
// parses each file cost and how deep the adapter retries nested (#408).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/z-shell/zsh-lint/internal/survey"
)

func main() {
	flags := flag.NewFlagSet("zsh-lint-survey", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint-survey [-trace-parses] <file.zsh> [file.zsh ...]")
		flags.PrintDefaults()
	}
	traceParses := flags.Bool("trace-parses", false,
		"write per-file base parse counts and adapter retry depth to standard error")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() < 1 {
		flags.Usage()
		os.Exit(2)
	}
	var opts survey.Options
	if *traceParses {
		opts.Trace = os.Stderr
	}
	os.Exit(survey.RunWithOptions(flags.Args(), os.Stdout, opts))
}
