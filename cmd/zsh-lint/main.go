// Command zsh-lint performs static analysis of Zsh shell scripts.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"github.com/z-shell/zsh-lint/internal/rules"
)

const usage = "usage: zsh-lint [--format=json] [--config PATH | --no-config] <file.zsh> [file.zsh ...]"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var formatFlag singleValue
	formatFlag.name = "format"
	var configFlag singleValue
	configFlag.name = "config"
	var noConfig bool

	flags := flag.NewFlagSet("zsh-lint", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Var(&formatFlag, "format", "output format (json)")
	flags.Var(&configFlag, "config", "explicit project configuration path")
	flags.BoolVar(&noConfig, "no-config", false, "disable automatic project configuration discovery")
	if err := flags.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "zsh-lint: %v\n%s\n", err, usage)
		return 2
	}
	if formatFlag.set && formatFlag.value != "json" {
		_, _ = fmt.Fprintf(stderr, "zsh-lint: unsupported output format %q\n%s\n", formatFlag.value, usage)
		return 2
	}
	if configFlag.set && configFlag.value == "" {
		_, _ = fmt.Fprintf(stderr, "zsh-lint: --config requires a non-empty path\n%s\n", usage)
		return 2
	}
	if configFlag.set && noConfig {
		_, _ = fmt.Fprintf(stderr, "zsh-lint: --config and --no-config are mutually exclusive\n%s\n", usage)
		return 2
	}
	names := flags.Args()
	if len(names) == 0 {
		_, _ = fmt.Fprintln(stderr, usage)
		return 2
	}
	jsonOut := formatFlag.value == "json"

	sourceContexts, err := resolveSourceContexts(names, configFlag, noConfig)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zsh-lint: configuration: %v\n", err)
		return 2
	}
	defaultAnalyzer := analyzer.New(rules.Default()...)
	configuredAnalyzer := defaultAnalyzer
	if hasConfiguredInputs(sourceContexts) {
		activeRules, err := rules.ForProfile(rules.CurrentProjectProfile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "zsh-lint: rule profile: %v\n", err)
			return 2
		}
		configuredAnalyzer = analyzer.New(activeRules...)
	}
	var all diag.Diagnostics
	var exitNonZero bool
	inspectedFiles := 0
	type configuredProject struct {
		root   string
		inputs []analyzer.ProjectInput
	}
	configuredProjects := make([]configuredProject, 0, len(names))
	configuredProjectIndex := make(map[string]int, len(names))

	for index, name := range names {
		if len(sourceContexts) != 0 && sourceContexts[index].err != nil {
			_, _ = fmt.Fprintf(stderr, "zsh-lint: configuration: %v\n", sourceContexts[index].err)
			exitNonZero = true
			continue
		}
		f, err := os.Open(name)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
			exitNonZero = true
			continue
		}

		file, err := parse.Parse(f, name)
		_ = f.Close()
		inspectedFiles++

		if err != nil {
			exitNonZero = true
			if jsonOut {
				// Parser failures share the diagnostics model under the
				// reserved rule ID parse/error
				// (docs/project/output-contract.md).
				all = append(all, parseErrDiag(name, err))
			} else {
				_, _ = fmt.Fprintln(stdout, formatErr(name, err))
			}
			continue
		}

		if len(sourceContexts) != 0 && sourceContexts[index].context.Configured() {
			root := projectconfig.PathKey(sourceContexts[index].context.ConfigRoot)
			projectIndex, ok := configuredProjectIndex[root]
			if !ok {
				configuredProjects = append(configuredProjects, configuredProject{root: root})
				projectIndex = len(configuredProjects) - 1
				configuredProjectIndex[root] = projectIndex
			}
			configuredProjects[projectIndex].inputs = append(configuredProjects[projectIndex].inputs, analyzer.ProjectInput{
				File:   file,
				Path:   name,
				Source: sourceContexts[index].context,
			})
			continue
		}
		diags := defaultAnalyzer.Analyze(file, name)
		for _, d := range diags {
			// Errors and warnings cause a non-zero exit; Info/Hint do not.
			if d.Severity <= diag.Warning {
				exitNonZero = true
			}

			if jsonOut {
				continue
			}
			// Format similar to gcc/clang
			if d.Range.IsValid() {
				_, _ = fmt.Fprintf(stdout, "%s:%d:%d: [%s] %s\n", d.File, d.Range.Start.Line, d.Range.Start.Column, d.RuleID, d.Message)
			} else {
				_, _ = fmt.Fprintf(stdout, "%s: [%s] %s\n", d.File, d.RuleID, d.Message)
			}
		}
		if jsonOut {
			all = append(all, diags...)
		}
	}
	for _, project := range configuredProjects {
		diagnostics := configuredAnalyzer.AnalyzeProject(project.inputs)
		for _, diagnostic := range diagnostics {
			if diagnostic.Severity <= diag.Warning {
				exitNonZero = true
			}
			if jsonOut {
				continue
			}
			if diagnostic.Range.IsValid() {
				_, _ = fmt.Fprintf(stdout, "%s:%d:%d: [%s] %s\n", diagnostic.File, diagnostic.Range.Start.Line, diagnostic.Range.Start.Column, diagnostic.RuleID, diagnostic.Message)
			} else {
				_, _ = fmt.Fprintf(stdout, "%s: [%s] %s\n", diagnostic.File, diagnostic.RuleID, diagnostic.Message)
			}
		}
		if jsonOut {
			all = append(all, diagnostics...)
		}
	}

	if jsonOut {
		all.Sort()
		if err := diag.WriteJSON(stdout, inspectedFiles, all); err != nil {
			_, _ = fmt.Fprintf(stderr, "zsh-lint: encoding JSON: %v\n", err)
			return 2
		}
	}

	if exitNonZero {
		return 1
	}
	return 0
}

type sourceResolution struct {
	context projectconfig.SourceContext
	err     error
}

func resolveSourceContexts(names []string, configFlag singleValue, noConfig bool) ([]sourceResolution, error) {
	if noConfig {
		return nil, nil
	}
	if configFlag.set {
		config, err := projectconfig.Load(configFlag.value)
		if err != nil {
			return nil, err
		}
		contexts := make([]sourceResolution, len(names))
		for index, name := range names {
			context, err := config.Resolve(name)
			if err != nil {
				return nil, err
			}
			contexts[index].context = context
		}
		return contexts, nil
	}

	contexts := make([]sourceResolution, len(names))
	type loadedConfig struct {
		config *projectconfig.Config
		err    error
	}
	configs := make(map[string]loadedConfig, len(names))
	found := false
	failed := false
	for index, name := range names {
		if strings.HasSuffix(name, "/") || strings.HasSuffix(name, string(os.PathSeparator)) {
			continue
		}
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			continue
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			contexts[index].err = fmt.Errorf("discover %q: %w", name, err)
			failed = true
			continue
		}
		filename, err := projectconfig.Discover(name)
		if err != nil {
			contexts[index].err = fmt.Errorf("discover %q: %w", name, err)
			failed = true
			continue
		}
		if filename == "" {
			continue
		}
		found = true
		key := projectconfig.PathKey(filename)
		cached, ok := configs[key]
		if !ok {
			config, err := projectconfig.Load(filename)
			cached = loadedConfig{config: config, err: err}
			configs[key] = cached
		}
		if cached.err != nil {
			contexts[index].err = fmt.Errorf("discover %q using %q: %w", name, filename, cached.err)
			failed = true
			continue
		}
		context, err := cached.config.Resolve(name)
		if err != nil {
			if projectconfig.IsUnmatchedSource(err) {
				continue
			}
			contexts[index].err = fmt.Errorf("discover %q using %q: %w", name, filename, err)
			failed = true
			continue
		}
		contexts[index].context = context
	}
	if !found && !failed {
		return nil, nil
	}
	return contexts, nil
}

func hasConfiguredInputs(contexts []sourceResolution) bool {
	for _, context := range contexts {
		if context.context.Configured() {
			return true
		}
	}
	return false
}

type singleValue struct {
	name  string
	value string
	set   bool
}

func (value *singleValue) String() string { return value.value }

func (value *singleValue) Set(text string) error {
	if value.set {
		return fmt.Errorf("--%s may be specified only once", value.name)
	}
	value.value = text
	value.set = true
	return nil
}

// parseErrDiag converts a parser/IO error into a parse/error diagnostic,
// extracting the source position when the front end provides one.
func parseErrDiag(name string, err error) diag.Diagnostic {
	d := diag.Diagnostic{
		RuleID:   "parse/error",
		Severity: diag.Error,
		File:     name,
	}
	var perr syntax.ParseError
	if errors.As(err, &perr) {
		d.Message = perr.Text
		d.Range = posRange(perr.Pos)
		return d
	}
	var lerr syntax.LangError
	if errors.As(err, &lerr) {
		// LangError.Error() embeds the position; the range carries it in the
		// contract, so strip the prefix to keep the message position-free.
		lerr.Filename = ""
		msg := lerr.Error()
		if lerr.Pos.IsValid() {
			prefix := fmt.Sprintf("%d:%d: ", lerr.Pos.Line(), lerr.Pos.Col())
			if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
				msg = msg[len(prefix):]
			}
		}
		d.Message = msg
		d.Range = posRange(lerr.Pos)
		return d
	}
	d.Message = err.Error()
	return d
}

// posRange converts a parser position into a zero-width diagnostic range.
func posRange(p syntax.Pos) diag.Range {
	if !p.IsValid() {
		return diag.Range{}
	}
	pos := diag.Position{Line: int(p.Line()), Column: int(p.Col()), Offset: int(p.Offset())}
	return diag.Range{Start: pos, End: pos}
}

// formatErr renders a parser/IO error as a greppable `path:line:col: message`
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
