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
//
// With -judge it prints, per file, whether it is a parser gap (valid Zsh that
// zsh-lint rejects), a false accept (invalid Zsh that zsh-lint accepts) or
// agrees, with both verdicts; `zsh -f -n` decides validity by empty standard
// error, and no file is run (#562). With -reduce it shrinks one gap or false
// accept to a smaller source that keeps its first message, writes the source
// to standard output and a report to standard error; -fixed <binary> also
// keeps each candidate fixed by that build. -candidate <binary> takes
// zsh-lint's verdict from that build for either mode.
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
		fmt.Fprintln(os.Stderr, "       zsh-lint-survey -judge [-candidate <binary>] <file.zsh> [file.zsh ...]")
		fmt.Fprintln(os.Stderr, "       zsh-lint-survey -reduce [-candidate <binary>] [-fixed <binary>] [-max-tests <n>] <file.zsh> > reduced.zsh")
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
		"with -compare, -judge or -reduce, take the zsh-lint verdicts from this zsh-lint-survey `binary` instead of this build")
	runtime := flags.Bool("runtime", false,
		"with -compare -native, run each regressed file with zsh -f and count the ones Zsh rejects when run as runtime-rejected; runs the files, so use it on probe rows only")
	table := flags.String("table", "",
		"with -compare -native, write the changed files as a Markdown table to this `file`")
	judge := flags.Bool("judge", false,
		"classify each file as a gap, a false accept or agreeing, by zsh -f -n and zsh-lint (needs zsh on PATH)")
	reduceFile := flags.Bool("reduce", false,
		"shrink one gap or false accept while its first message stays the same, and write the result to standard output (needs zsh on PATH)")
	fixed := flags.String("fixed", "",
		"with -reduce, keep only candidates that this zsh-lint-survey `binary` fixes")
	maxTests := flags.Int("max-tests", 0,
		"with -reduce, cap the candidates judged (default 5000)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() < 1 {
		flags.Usage()
		os.Exit(2)
	}

	if *judge || *reduceFile {
		if *judge && *reduceFile {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -judge and -reduce do not combine")
			os.Exit(2)
		}
		if *traceParses || *compare != "" || *native || *known || *runtime || *table != "" {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -judge and -reduce take only -candidate, -fixed and -max-tests")
			os.Exit(2)
		}
		if (*maxTests != 0 || *fixed != "") && !*reduceFile {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -fixed and -max-tests need -reduce")
			os.Exit(2)
		}
		zsh, err := exec.LookPath("zsh")
		if err != nil {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -judge and -reduce need zsh on PATH")
			os.Exit(2)
		}
		var lint func(string) (survey.Verdict, error)
		if *candidate != "" {
			lint = survey.BinaryVerdict(*candidate)
		}
		if *judge {
			os.Exit(survey.JudgeFiles(flags.Args(), os.Stdout, survey.NativeZshDiagnostic(zsh), lint))
		}
		if flags.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "zsh-lint-survey: -reduce takes one file")
			os.Exit(2)
		}
		opts := survey.ReduceOptions{
			Native:   survey.NativeZshDiagnostic(zsh),
			Lint:     lint,
			MaxTests: *maxTests,
			Out:      os.Stdout,
		}
		if *fixed != "" {
			opts.Fixed = survey.BinaryVerdict(*fixed)
		}
		os.Exit(survey.ReduceFile(flags.Arg(0), os.Stderr, opts))
	}
	if *maxTests != 0 || *fixed != "" {
		fmt.Fprintln(os.Stderr, "zsh-lint-survey: -fixed and -max-tests need -reduce")
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
