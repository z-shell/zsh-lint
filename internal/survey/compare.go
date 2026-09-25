package survey

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
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
	// Native, when set, judges whether native Zsh accepts a file, which
	// splits verdict changes into fixes, regressions, and false accepts.
	// NativeZsh wraps `zsh -f -n` for it.
	Native func(name string) (valid bool, err error)
	// ListKnown, with Native, also prints a line for every unchanged file
	// whose verdict disagrees with native Zsh: GAP (valid Zsh that fails in
	// both builds) and ACCEPTED (invalid Zsh that parses in both builds).
	// Their counts are in the summary either way (#428).
	ListKnown bool
}

// Change classes, printed as the first word of each changed file's line.
const (
	classFixed       = "FIXED"
	classRegressed   = "REGRESSED"
	classFalseAccept = "FALSE-ACCEPT"
	classRejected    = "REJECTED"
	classNowOK       = "NOW-OK"
	classNowFail     = "NOW-FAIL"
	classMoved       = "MOVED"

	knownGap      = "GAP"
	knownAccepted = "ACCEPTED"
)

// Compare surveys names with the current build and compares each verdict
// with the base build's (#412). It writes one line per changed file, the
// candidate's diagnostic under a newly failing file, both diagnostics under
// a MOVED file, and a closing count per class.
//
// With opts.Native a FAIL-to-OK change on a Zsh-valid file is FIXED and on a
// Zsh-invalid file FALSE-ACCEPT; an OK-to-FAIL change is REGRESSED or
// REJECTED the same way. Without it the changes are NOW-OK and NOW-FAIL.
// A file that fails in both builds with a different first diagnostic is
// MOVED.
//
// With opts.Native the unchanged files are judged too, and the summary
// counts the known disagreements with native Zsh that the change neither
// fixed nor introduced: valid files failing in both builds (known gaps) and
// invalid files parsing in both (known false accepts).
//
// It returns 1 when a file regressed or a false accept was introduced
// (without Native: when any file went from OK to FAIL), 2 when the base or
// native verdict could not be obtained, and 0 otherwise.
func Compare(names []string, w io.Writer, opts CompareOptions) int {
	base, err := opts.Base(names)
	if err != nil {
		_, _ = fmt.Fprintf(w, "compare: base survey: %v\n", err)
		return 2
	}

	counts := map[string]int{}
	var unchanged, gaps, accepted int
	for _, name := range names {
		before, ok := base[name]
		if !ok {
			_, _ = fmt.Fprintf(w, "compare: base survey reported no verdict for %s\n", name)
			return 2
		}
		after := candidateVerdict(name)

		var class string
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
		default:
			class = classNowOK
			if !after.OK {
				class = classNowFail
			}
			if opts.Native != nil {
				valid, err := opts.Native(name)
				if err != nil {
					_, _ = fmt.Fprintf(w, "compare: native verdict for %s: %v\n", name, err)
					return 2
				}
				class = nativeClass(after.OK, valid)
			}
		}
		counts[class]++

		_, _ = fmt.Fprintf(w, "%s %s\n", class, name)
		switch {
		case class == classMoved:
			_, _ = fmt.Fprintf(w, "  base: %s\n  now:  %s\n", before.Diagnostic, after.Diagnostic)
		case !after.OK:
			_, _ = fmt.Fprintln(w, after.Diagnostic)
		}
	}

	_, _ = fmt.Fprintf(w, "\n%d file(s) compared, %d unchanged", len(names), unchanged)
	order := []string{classFixed, classRegressed, classFalseAccept, classRejected, classNowOK, classNowFail, classMoved}
	for _, class := range order {
		if counts[class] > 0 {
			_, _ = fmt.Fprintf(w, ", %d %s", counts[class], strings.ToLower(class))
		}
	}
	if opts.Native != nil {
		_, _ = fmt.Fprintf(w, "; known: %d gap(s), %d false accept(s)", gaps, accepted)
	}
	_, _ = fmt.Fprintln(w)

	if counts[classRegressed] > 0 || counts[classFalseAccept] > 0 || counts[classNowFail] > 0 {
		return 1
	}
	return 0
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
// a valid negated pipeline such as `! true`.
func NativeZsh(zsh string) func(string) (bool, error) {
	return func(name string) (bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, zsh, "-f", "-n", name)
		cmd.Stderr = &stderr
		err := cmd.Run()
		if ctx.Err() != nil {
			return false, fmt.Errorf("zsh -f -n did not finish: %w", ctx.Err())
		}
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			return false, err
		}
		return strings.TrimSpace(stderr.String()) == "", nil
	}
}
