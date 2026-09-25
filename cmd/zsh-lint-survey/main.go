// Command zsh-lint-survey runs the parser front end across Zsh files and
// reports parser gaps without evaluating static-analysis rules.
//
// With -trace-parses it also writes, on standard error, how many whole-source
// parses each file cost and how deep the adapter retries nested (#408).
//
// With -compare <base-binary> it instead reports how each file's verdict
// changed from the base build to this one; -native adds the `zsh -f -n`
// verdict so the changes split into fixes, regressions, and false accepts
// (#412).
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
		fmt.Fprintln(os.Stderr, "       zsh-lint-survey -compare <base-binary> [-native [-known]] <file.zsh> [file.zsh ...]")
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
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() < 1 {
		flags.Usage()
		os.Exit(2)
	}

	if *compare == "" {
		if *native || *known {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -native and -known need -compare")
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
	if *known && !*native {
		fmt.Fprintln(os.Stderr, "zsh-lint-survey: -known needs -native")
		os.Exit(2)
	}
	opts := survey.CompareOptions{Base: survey.BaseBinary(*compare), ListKnown: *known}
	if *native {
		zsh, err := exec.LookPath("zsh")
		if err != nil {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -native needs zsh on PATH")
			os.Exit(2)
		}
		opts.Native = survey.NativeZsh(zsh)
	}
	os.Exit(survey.Compare(flags.Args(), os.Stdout, opts))
}
