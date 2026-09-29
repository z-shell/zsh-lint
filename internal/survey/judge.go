package survey

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Judgement classes: how zsh-lint's verdict on a file relates to native
// Zsh's (#562).
const (
	// ClassGap is valid Zsh that zsh-lint rejects.
	ClassGap = "GAP"
	// ClassFalseAccept is invalid Zsh that zsh-lint accepts.
	ClassFalseAccept = "FALSE-ACCEPT"
	// ClassAgree is a file both accept or both reject.
	ClassAgree = "AGREE"
)

// Judgement is one file judged by native Zsh and by zsh-lint.
type Judgement struct {
	// Class is ClassGap, ClassFalseAccept or ClassAgree.
	Class string
	// Native is the first line `zsh -f -n` wrote to standard error, or ""
	// when Zsh accepts the file.
	Native string
	// Lint is zsh-lint's verdict.
	Lint Verdict
}

// Judge judges the file name with native and lint. native returns what
// NativeZshDiagnostic returns: "" for valid Zsh, and otherwise its first
// diagnostic line.
func Judge(name string, native func(string) (string, error), lint func(string) (Verdict, error)) (Judgement, error) {
	verdict, err := lint(name)
	if err != nil {
		return Judgement{}, fmt.Errorf("zsh-lint verdict for %s: %w", name, err)
	}
	message, err := native(name)
	if err != nil {
		return Judgement{}, fmt.Errorf("native verdict for %s: %w", name, err)
	}
	return Judgement{Class: classify(message == "", verdict.OK), Native: message, Lint: verdict}, nil
}

func classify(valid, lintOK bool) string {
	switch {
	case valid && !lintOK:
		return ClassGap
	case !valid && lintOK:
		return ClassFalseAccept
	default:
		return ClassAgree
	}
}

// Family returns the key a reducer keeps fixed while it shrinks the file name
// (#562): the class and, for a gap, zsh-lint's first diagnostic, or, for a
// false accept, Zsh's, each without the file name and its position. Any
// other `LINE:COL` inside the message reads as `N:N`. Quoted literals are
// kept, so `|` and `&&` after a separator stay two families. It is "" for
// ClassAgree.
func (j Judgement) Family(name string) string {
	switch j.Class {
	case ClassGap:
		return ClassGap + ": " + lintMessage(name, j.Lint.Diagnostic)
	case ClassFalseAccept:
		return ClassFalseAccept + ": " + nativeMessage(name, j.Native)
	default:
		return ""
	}
}

var (
	leadingLineCol = regexp.MustCompile(`^\d+:\d+: `)
	leadingLine    = regexp.MustCompile(`^\d+: `)
	innerLineCol   = regexp.MustCompile(`\b\d+:\d+\b`)
)

// lintMessage strips "<name>:LINE:COL: " from a survey diagnostic.
func lintMessage(name, diagnostic string) string {
	message := strings.TrimPrefix(diagnostic, name+":")
	message = leadingLineCol.ReplaceAllString(message, "")
	return innerLineCol.ReplaceAllString(message, "N:N")
}

// nativeMessage strips "<name>:LINE: " from a Zsh diagnostic.
func nativeMessage(name, diagnostic string) string {
	message := strings.TrimPrefix(diagnostic, name+":")
	return leadingLine.ReplaceAllString(message, "")
}

// NativeZshDiagnostic returns a function that judges a file with
// `zsh -f -n` and returns the first line Zsh writes to standard error, or ""
// when it writes nothing. Like NativeZsh it ignores the exit status, because
// `zsh -n` exits 1 without a diagnostic for a valid negated pipeline such as
// `! true`. The file is judged as a file, not a `-c` string, since an
// unterminated loop header at end of input is valid only in a file; it is
// never run. A file that cannot be opened is an error, not a diagnostic,
// so a missing file is not judged invalid Zsh.
func NativeZshDiagnostic(zsh string) func(string) (string, error) {
	return func(name string) (string, error) {
		f, err := os.Open(name)
		if err != nil {
			return "", err
		}
		_ = f.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, zsh, "-f", "-n", name)
		cmd.Stderr = &stderr
		err = cmd.Run()
		if ctx.Err() != nil {
			return "", fmt.Errorf("zsh -f -n did not finish: %w", ctx.Err())
		}
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			return "", err
		}
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		return line, nil
	}
}

// BuildVerdict returns this build's verdict on the file name. A parser panic
// is returned as a *PanicError instead of ending the process, so a reducer
// can treat the candidate that caused it as a finding of its own.
func BuildVerdict(name string) (Verdict, error) {
	return recoverPanic(candidateVerdict)(name)
}

func recoverPanic(verdict func(string) Verdict) func(string) (Verdict, error) {
	return func(name string) (v Verdict, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = &PanicError{Value: r}
			}
		}()
		return verdict(name), nil
	}
}

// PanicError is a parser panic that BuildVerdict recovered.
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("parser panic: %v", e.Value)
}

// BinaryVerdict returns a function that takes one file's verdict from the
// zsh-lint-survey binary at path.
func BinaryVerdict(path string) func(string) (Verdict, error) {
	survey := BaseBinary(path)
	return func(name string) (Verdict, error) {
		verdicts, err := survey([]string{name})
		if err != nil {
			return Verdict{}, err
		}
		verdict, ok := verdicts[name]
		if !ok {
			return Verdict{}, fmt.Errorf("%s reported no verdict for %s", path, name)
		}
		return verdict, nil
	}
}

// JudgeFiles judges each file in names and writes, per file, its class, the
// Zsh verdict and the zsh-lint verdict with its first diagnostic, then a
// count per class (#562). lint defaults to this build.
//
// It returns 1 when a file is a gap or a false accept, 2 when a verdict
// could not be obtained, and 0 otherwise.
func JudgeFiles(names []string, w io.Writer, native func(string) (string, error), lint func(string) (Verdict, error)) int {
	if lint == nil {
		lint = BuildVerdict
	}
	counts := map[string]int{}
	for _, name := range names {
		judgement, err := Judge(name, native, lint)
		if err != nil {
			_, _ = fmt.Fprintf(w, "judge: %v\n", err)
			return 2
		}
		counts[judgement.Class]++
		zsh, zshLint := "accept", "accept"
		if judgement.Native != "" {
			zsh = judgement.Native
		}
		if !judgement.Lint.OK {
			zshLint = judgement.Lint.Diagnostic
		}
		_, _ = fmt.Fprintf(w, "%s %s\n  zsh -f -n: %s\n  zsh-lint:  %s\n", judgement.Class, name, zsh, zshLint)
	}
	_, _ = fmt.Fprintf(w, "\n%d file(s) judged, %d gap(s), %d false accept(s), %d agree\n",
		len(names), counts[ClassGap], counts[ClassFalseAccept], counts[ClassAgree])
	if counts[ClassGap] > 0 || counts[ClassFalseAccept] > 0 {
		return 1
	}
	return 0
}
