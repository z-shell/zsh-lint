package survey

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Verdict is one file's survey outcome: whether it parsed and, when it did
// not, its first greppable diagnostic.
type Verdict struct {
	OK         bool
	Diagnostic string
}

// CompareOptions configures Compare.
type CompareOptions struct {
	// Base returns the base build's verdict for each file. BaseBinary wraps
	// a zsh-lint-survey binary for it.
	Base func(names []string) (map[string]Verdict, error)
	// Candidate, when set, returns the candidate build's verdict for each
	// file instead of this build's parser, so this build can classify two
	// other builds, such as a merged fix and its base (#545). BaseBinary
	// wraps a zsh-lint-survey binary for it too.
	Candidate func(names []string) (map[string]Verdict, error)
	// Native, when set, judges whether native Zsh accepts a file, which
	// splits verdict changes into fixes, regressions, and false accepts.
	// NativeZsh wraps `zsh -f -n` for it.
	Native func(name string) (valid bool, err error)
	// ListKnown, with Native, also prints a line for every unchanged file
	// whose verdict disagrees with native Zsh: GAP (valid Zsh that fails in
	// both builds) and ACCEPTED (invalid Zsh that parses in both builds).
	// Their counts are in the summary either way (#428).
	ListKnown bool
	// Runtime, with Native, runs each file that would be REGRESSED and
	// returns the first error Zsh reports, or "" when it runs without one.
	// A file that `zsh -f -n` accepts and Zsh rejects when it runs is
	// RUNTIME-REJECTED instead, which does not fail the comparison:
	// `zsh -n` does not check an assignment word or a command substitution
	// body, so a fix that rejects such a source in every context is not a
	// regression (#539, #545). RuntimeZsh wraps `zsh -f` for it. It runs the
	// file, so use it on generated probe rows, never on corpus sources.
	Runtime func(name string) (message string, err error)
	// Table, with Native, receives a Markdown table of the changed files for
	// a survey record: the class, the source, and the native, base and
	// candidate verdicts, plus the runtime error with Runtime (#545).
	Table io.Writer
}

// Change classes, printed as the first word of each changed file's line.
const (
	classFixed           = "FIXED"
	classRegressed       = "REGRESSED"
	classRuntimeRejected = "RUNTIME-REJECTED"
	classFalseAccept     = "FALSE-ACCEPT"
	classRejected        = "REJECTED"
	classNowOK           = "NOW-OK"
	classNowFail         = "NOW-FAIL"
	classMoved           = "MOVED"

	knownGap      = "GAP"
	knownAccepted = "ACCEPTED"
)

// tableRow is one changed file in the Markdown table.
type tableRow struct {
	class, name, runtime string
	valid                bool
	before, after        Verdict
}

// Compare surveys names with the current build, or opts.Candidate, and
// compares each verdict with the base build's (#412). It writes one line per
// changed file, the candidate's diagnostic under a newly failing file, both
// diagnostics under a MOVED file, and a closing count per class.
//
// With opts.Native a FAIL-to-OK change on a Zsh-valid file is FIXED and on a
// Zsh-invalid file FALSE-ACCEPT; an OK-to-FAIL change is REGRESSED or
// REJECTED the same way. Without it the changes are NOW-OK and NOW-FAIL.
// A file that fails in both builds with a different first diagnostic is
// MOVED. With opts.Runtime a REGRESSED file that Zsh rejects when it runs is
// RUNTIME-REJECTED, with the runtime error under it.
//
// With opts.Native the unchanged files are judged too, and the summary
// counts the known disagreements with native Zsh that the change neither
// fixed nor introduced: valid files failing in both builds (known gaps) and
// invalid files parsing in both (known false accepts).
//
// It returns 1 when a file regressed or a false accept was introduced
// (without Native: when any file went from OK to FAIL), 2 when the base,
// candidate, native or runtime verdict could not be obtained, and 0
// otherwise.
func Compare(names []string, w io.Writer, opts CompareOptions) int {
	base, err := opts.Base(names)
	if err != nil {
		_, _ = fmt.Fprintf(w, "compare: base survey: %v\n", err)
		return 2
	}
	var candidate map[string]Verdict
	if opts.Candidate != nil {
		if candidate, err = opts.Candidate(names); err != nil {
			_, _ = fmt.Fprintf(w, "compare: candidate survey: %v\n", err)
			return 2
		}
	}

	counts := map[string]int{}
	var unchanged, gaps, accepted int
	var rows []tableRow
	for _, name := range names {
		before, ok := base[name]
		if !ok {
			_, _ = fmt.Fprintf(w, "compare: base survey reported no verdict for %s\n", name)
			return 2
		}
		var after Verdict
		if candidate != nil {
			if after, ok = candidate[name]; !ok {
				_, _ = fmt.Fprintf(w, "compare: candidate survey reported no verdict for %s\n", name)
				return 2
			}
		} else {
			after = candidateVerdict(name)
		}

		var class, runtime string
		valid := true
		switch {
		case before.OK == after.OK && before.Diagnostic == after.Diagnostic:
			unchanged++
			if opts.Native == nil {
				continue
			}
			valid, err := opts.Native(name)
			if err != nil {
				_, _ = fmt.Fprintf(w, "compare: native verdict for %s: %v\n", name, err)
				return 2
			}
			known := ""
			switch {
			case valid && !after.OK:
				gaps++
				known = knownGap
			case !valid && after.OK:
				accepted++
				known = knownAccepted
			}
			if known != "" && opts.ListKnown {
				_, _ = fmt.Fprintf(w, "%s %s\n", known, name)
				if !after.OK {
					_, _ = fmt.Fprintln(w, after.Diagnostic)
				}
			}
			continue
		case !before.OK && !after.OK:
			class = classMoved
			if opts.Native != nil && opts.Table != nil {
				if valid, err = opts.Native(name); err != nil {
					_, _ = fmt.Fprintf(w, "compare: native verdict for %s: %v\n", name, err)
					return 2
				}
			}
		default:
			class = classNowOK
			if !after.OK {
				class = classNowFail
			}
			if opts.Native != nil {
				if valid, err = opts.Native(name); err != nil {
					_, _ = fmt.Fprintf(w, "compare: native verdict for %s: %v\n", name, err)
					return 2
				}
				class = nativeClass(after.OK, valid)
			}
			if class == classRegressed && opts.Runtime != nil {
				if runtime, err = opts.Runtime(name); err != nil {
					_, _ = fmt.Fprintf(w, "compare: runtime verdict for %s: %v\n", name, err)
					return 2
				}
				if runtime != "" {
					class = classRuntimeRejected
				}
			}
		}
		counts[class]++
		rows = append(rows, tableRow{class: class, name: name, runtime: runtime, valid: valid, before: before, after: after})

		_, _ = fmt.Fprintf(w, "%s %s\n", class, name)
		switch {
		case class == classMoved:
			_, _ = fmt.Fprintf(w, "  base: %s\n  now:  %s\n", before.Diagnostic, after.Diagnostic)
		case !after.OK:
			_, _ = fmt.Fprintln(w, after.Diagnostic)
		}
		if runtime != "" {
			_, _ = fmt.Fprintf(w, "  run:  %s\n", runtime)
		}
	}

	_, _ = fmt.Fprintf(w, "\n%d file(s) compared, %d unchanged", len(names), unchanged)
	order := []string{classFixed, classRegressed, classRuntimeRejected, classFalseAccept, classRejected, classNowOK, classNowFail, classMoved}
	for _, class := range order {
		if counts[class] > 0 {
			_, _ = fmt.Fprintf(w, ", %d %s", counts[class], strings.ToLower(class))
		}
	}
	if opts.Native != nil {
		_, _ = fmt.Fprintf(w, "; known: %d gap(s), %d false accept(s)", gaps, accepted)
	}
	_, _ = fmt.Fprintln(w)

	if opts.Table != nil {
		writeTable(opts.Table, rows, opts.Runtime != nil)
	}

	if counts[classRegressed] > 0 || counts[classFalseAccept] > 0 || counts[classNowFail] > 0 {
		return 1
	}
	return 0
}

// writeTable writes the changed files as a Markdown table in the shape of a
// survey record's rows (#545). A file holding one short line is shown as
// its source, any other by its name.
func writeTable(w io.Writer, rows []tableRow, runtime bool) {
	header := "| #   | Class | Source | zsh -f -n | base | candidate |"
	rule := "| --- | --- | --- | --- | --- | --- |"
	if runtime {
		header += " run |"
		rule += " --- |"
	}
	_, _ = fmt.Fprintln(w, header)
	_, _ = fmt.Fprintln(w, rule)
	verdict := func(ok bool) string {
		if ok {
			return "accept"
		}
		return "reject"
	}
	for i, row := range rows {
		line := fmt.Sprintf("| %d | %s | %s | %s | %s | %s |", i+1, row.class, tableSource(row.name),
			verdict(row.valid), verdict(row.before.OK), verdict(row.after.OK))
		if runtime {
			run := "-"
			switch {
			case row.runtime != "":
				run = strings.ReplaceAll(row.runtime, "|", "\\|")
			case row.class == classRegressed:
				run = "none"
			}
			line += " " + run + " |"
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

// tableSource renders a file for a table cell: its content as a code span
// when it is one line of at most 120 bytes, and its name otherwise. The span
// is fenced with one backquote more than the longest run inside it, and a
// `|` is escaped, as GFM table cells need.
func tableSource(name string) string {
	text := name
	if content, err := os.ReadFile(name); err == nil {
		line := strings.TrimSuffix(string(content), "\n")
		if line != "" && len(line) <= 120 && !strings.Contains(line, "\n") {
			text = line
		}
	}
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	pad := ""
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		pad = " "
	}
	return fence + pad + strings.ReplaceAll(text, "|", "\\|") + pad + fence
}

func nativeClass(nowOK, valid bool) string {
	switch {
	case nowOK && valid:
		return classFixed
	case nowOK:
		return classFalseAccept
	case valid:
		return classRegressed
	default:
		return classRejected
	}
}

func candidateVerdict(name string) Verdict {
	if err := surveyFile(name); err != nil {
		return Verdict{Diagnostic: formatErr(name, err)}
	}
	return Verdict{OK: true}
}

// BaseBinary returns a CompareOptions.Base that runs the zsh-lint-survey
// binary at path over the files and reads its report. The names are passed
// without a "--" separator, because builds from before #408 read every
// argument as a file; a name must therefore not start with "-".
func BaseBinary(path string) func([]string) (map[string]Verdict, error) {
	return func(names []string) (map[string]Verdict, error) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(path, names...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exit *exec.ExitError
		if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
			return nil, fmt.Errorf("%s: %v: %s", path, err, strings.TrimSpace(stderr.String()))
		}
		return ParseReport(&stdout)
	}
}

// ParseReport reads the per-file verdicts from a zsh-lint-survey report: an
// "OK   <path>" line, or a "FAIL <path>" line followed by its diagnostic.
func ParseReport(r io.Reader) (map[string]Verdict, error) {
	verdicts := map[string]Verdict{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "OK   "):
			verdicts[strings.TrimPrefix(line, "OK   ")] = Verdict{OK: true}
		case strings.HasPrefix(line, "FAIL "):
			name := strings.TrimPrefix(line, "FAIL ")
			if !scanner.Scan() {
				return nil, fmt.Errorf("report ends after FAIL %s without a diagnostic", name)
			}
			verdicts[name] = Verdict{Diagnostic: scanner.Text()}
		}
	}
	return verdicts, scanner.Err()
}

// NativeZsh returns a CompareOptions.Native that judges a file with
// `zsh -f -n`. A file is valid when zsh writes nothing to standard error; the
// exit status is not used, because `zsh -n` exits 1 without a diagnostic for
// a valid negated pipeline such as `! true`. It judges through
// NativeZshDiagnostic, so a file that cannot be opened, or a directory, is
// an error, not invalid Zsh.
func NativeZsh(zsh string) func(string) (bool, error) {
	diagnostic := NativeZshDiagnostic(zsh)
	return func(name string) (bool, error) {
		message, err := diagnostic(name)
		if err != nil {
			return false, err
		}
		return message == "", nil
	}
}

// runtimeLimit bounds one RuntimeZsh run; a test shortens it.
var runtimeLimit = 10 * time.Second

// RuntimeZsh returns a CompareOptions.Runtime that runs a file with
// `zsh -f FILE` in a new empty directory, with no input and a 10 second
// limit, and returns the first line Zsh writes to standard error without
// its `FILE:LINE: ` prefix. A run that times out reports no error, so the
// file stays REGRESSED rather than being excused. It executes the file:
// use it only on generated probe rows.
func RuntimeZsh(zsh string) func(string) (string, error) {
	return func(name string) (string, error) {
		path, err := filepath.Abs(name)
		if err != nil {
			return "", err
		}
		dir, err := os.MkdirTemp("", "zsh-lint-runtime-")
		if err != nil {
			return "", err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		ctx, cancel := context.WithTimeout(context.Background(), runtimeLimit)
		defer cancel()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, zsh, "-f", path)
		cmd.Dir = dir
		cmd.Stderr = &stderr
		err = cmd.Run()
		if ctx.Err() != nil {
			return "", nil
		}
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			return "", err
		}
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if rest, ok := strings.CutPrefix(line, path+":"); ok {
			if _, message, ok := strings.Cut(rest, ": "); ok {
				line = message
			}
		}
		return line, nil
	}
}
