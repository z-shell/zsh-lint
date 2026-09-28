// Command zsh-lint-survey runs the parser front end across Zsh files and
// reports parser gaps without evaluating static-analysis rules.
//
// With -trace-parses it also writes, on standard error, how many whole-source
// parses each file cost and how deep the adapter retries nested (#408).
//
// With -compare <base-binary> it instead reports how each file's verdict
// changed from the base build to this one; -native adds the `zsh -f -n`
// verdict so the changes split into fixes, regressions, and false accepts
// (#412). With -candidate <binary> the candidate verdicts come from that
// build instead of this one; -runtime runs each regressed file with `zsh -f`
// and reports the ones Zsh rejects when run as RUNTIME-REJECTED; -table
// <file> writes the changed files as a Markdown table for a survey record
// (#545).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/z-shell/zsh-lint/internal/survey"
)

func main() {
	flags := flag.NewFlagSet("zsh-lint-survey", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint-survey [-trace-parses] <file.zsh> [file.zsh ...]")
		fmt.Fprintln(os.Stderr, "       zsh-lint-survey -compare <base-binary> [-candidate <binary>] [-native [-known] [-runtime] [-table <file>]] <file.zsh> [file.zsh ...]")
		flags.PrintDefaults()
	}
	traceParses := flags.Bool("trace-parses", false,
		"write per-file base parse counts and adapter retry depth to standard error")
	compare := flags.String("compare", "",
		"report verdict changes against the zsh-lint-survey `binary` of a base build")
	native := flags.Bool("native", false,
		"with -compare, classify changes by the zsh -f -n verdict (needs zsh on PATH)")
	known := flags.Bool("known", false,
		"with -compare -native, also list unchanged files that disagree with zsh -f -n")
	candidate := flags.String("candidate", "",
		"with -compare, take the candidate verdicts from this zsh-lint-survey `binary` instead of this build")
	runtime := flags.Bool("runtime", false,
		"with -compare -native, run each regressed file with zsh -f and count the ones Zsh rejects when run as runtime-rejected; runs the files, so use it on probe rows only")
	table := flags.String("table", "",
		"with -compare -native, write the changed files as a Markdown table to this `file`")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() < 1 {
		flags.Usage()
		os.Exit(2)
	}

	if *compare == "" {
		if *native || *known || *candidate != "" || *runtime || *table != "" {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -native, -known, -candidate, -runtime and -table need -compare")
			os.Exit(2)
		}
		var opts survey.Options
		if *traceParses {
			opts.Trace = os.Stderr
		}
		os.Exit(survey.RunWithOptions(flags.Args(), os.Stdout, opts))
	}

	if *traceParses {
		fmt.Fprintln(os.Stderr, "zsh-lint-survey: -trace-parses does not combine with -compare")
		os.Exit(2)
	}
	if (*known || *runtime || *table != "") && !*native {
		fmt.Fprintln(os.Stderr, "zsh-lint-survey: -known, -runtime and -table need -native")
		os.Exit(2)
	}
	opts := survey.CompareOptions{Base: survey.BaseBinary(*compare), ListKnown: *known}
	if *candidate != "" {
		opts.Candidate = survey.BaseBinary(*candidate)
	}
	if *native {
		zsh, err := exec.LookPath("zsh")
		if err != nil {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -native needs zsh on PATH")
			os.Exit(2)
		}
		opts.Native = survey.NativeZsh(zsh)
		if *runtime {
			opts.Runtime = survey.RuntimeZsh(zsh)
		}
	}
	if *table != "" {
		out, err := os.Create(*table)
		if err != nil {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey:", err)
			os.Exit(2)
		}
		opts.Table = out
		code := survey.Compare(flags.Args(), os.Stdout, opts)
		if err := out.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey:", err)
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(survey.Compare(flags.Args(), os.Stdout, opts))
}
