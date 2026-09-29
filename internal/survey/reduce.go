package survey

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/z-shell/zsh-lint/internal/reduce"
)

// ReduceOptions configures ReduceFile.
type ReduceOptions struct {
	// Native judges a file with native Zsh; NativeZshDiagnostic wraps
	// `zsh -f -n` for it.
	Native func(string) (string, error)
	// Lint takes zsh-lint's verdict on a file; nil means this build, and
	// BinaryVerdict takes it from another build.
	Lint func(string) (Verdict, error)
	// Fixed, when set, is a build that fixes the gap or false accept, and
	// a candidate is kept only while that build accepts it (a gap) or
	// rejects it (a false accept). It keeps the reduction on the defect a
	// fix addressed, to replay a merged fix or to minimize a fixture.
	Fixed func(string) (Verdict, error)
	// MaxTests caps the oracle calls (reduce.Options.MaxTests).
	MaxTests int
	// Out receives the reduced source.
	Out io.Writer
}

// ReduceFile shrinks the parser gap or false accept in the file name to a
// smaller source that keeps its family and site (#562), and writes the
// reduced source to opts.Out and a report to w: the family, the sizes before
// and after, and the oracle calls it took.
//
// A candidate is kept only while its Judgement.Family is the original's and
// the error stays where it was. For a gap, Zsh accepts the candidate and
// zsh-lint fails with the same first message at the same byte of the
// original source. For a false accept, Zsh rejects it with the same message
// on a line holding a byte of the original's error line, and zsh-lint
// accepts it. The same message at another byte is usually a different
// defect, so a reducer keyed on the message alone walks onto whichever of
// them is smallest.
//
// Each candidate is written to a file with the original's base name in a new
// temporary directory and judged with `zsh -f -n`; no candidate is run.
//
// It returns 0 when the file reduced, 1 when it is neither a gap nor a false
// accept (or opts.Fixed does not fix it), and 2 when a verdict could not be
// obtained.
func ReduceFile(name string, w io.Writer, opts ReduceOptions) int {
	lint := opts.Lint
	if lint == nil {
		lint = BuildVerdict
	}
	src, err := os.ReadFile(name)
	if err != nil {
		_, _ = fmt.Fprintf(w, "reduce: %v\n", err)
		return 2
	}
	dir, err := os.MkdirTemp("", "zsh-lint-reduce-")
	if err != nil {
		_, _ = fmt.Fprintf(w, "reduce: %v\n", err)
		return 2
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, filepath.Base(name))

	var oracleErr error
	var panics int
	var firstPanic string
	// judge returns the candidate's judgement, or ok false when it could
	// not be judged or made the parser panic.
	judge := func(text string) (judgement Judgement, ok bool) {
		if oracleErr != nil {
			return Judgement{}, false
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			oracleErr = err
			return Judgement{}, false
		}
		judgement, err := Judge(path, opts.Native, lint)
		var panicked *PanicError
		if errors.As(err, &panicked) {
			// A candidate that makes the parser panic is not the
			// original defect; keep reducing and report it.
			if panics == 0 {
				firstPanic = text
			}
			panics++
			return Judgement{}, false
		}
		if err != nil {
			oracleErr = err
			return Judgement{}, false
		}
		return judgement, true
	}
	fixes := func(class string) bool {
		if opts.Fixed == nil {
			return true
		}
		verdict, err := opts.Fixed(path)
		if err != nil {
			oracleErr = err
			return false
		}
		return verdict.OK == (class == ClassGap)
	}

	original, ok := judge(string(src))
	switch {
	case oracleErr != nil:
		_, _ = fmt.Fprintf(w, "reduce: %v\n", oracleErr)
		return 2
	case !ok:
		_, _ = fmt.Fprintf(w, "reduce: %s: the parser panics on it; file the panic instead of reducing it\n", name)
		return 2
	case original.Class == ClassAgree:
		_, _ = fmt.Fprintf(w, "reduce: %s: native Zsh and zsh-lint agree; nothing to reduce\n", name)
		return 1
	}
	want := original.Family(path)
	site, ok := errorSite(original, path, string(src))
	if !ok {
		_, _ = fmt.Fprintf(w, "reduce: %s: cannot read the error position from %q\n", name, want)
		return 2
	}
	if !fixes(original.Class) {
		if oracleErr != nil {
			_, _ = fmt.Fprintf(w, "reduce: %v\n", oracleErr)
			return 2
		}
		_, _ = fmt.Fprintf(w, "reduce: %s: the fixed build does not fix it\n", name)
		return 1
	}

	reduced, stats, _ := reduce.Reduce(string(src), func(c reduce.Candidate) bool {
		judgement, ok := judge(c.Text)
		if !ok || judgement.Family(path) != want {
			return false
		}
		candidateSite, ok := errorSite(judgement, path, c.Text)
		if !ok || !site.matches(candidateSite, c, len(src)) {
			return false
		}
		return fixes(judgement.Class)
	}, reduce.Options{MaxTests: opts.MaxTests})
	if oracleErr != nil {
		_, _ = fmt.Fprintf(w, "reduce: %v\n", oracleErr)
		return 2
	}
	if _, err := io.WriteString(opts.Out, reduced); err != nil {
		_, _ = fmt.Fprintf(w, "reduce: %v\n", err)
		return 2
	}
	_, _ = fmt.Fprintf(w, "%s\n%s: %d -> %d byte(s), %d -> %d line(s), %d oracle call(s)\n",
		want, name, len(src), len(reduced), len(reduce.Lines(string(src))), len(reduce.Lines(reduced)), stats.Tests)
	if stats.Capped {
		_, _ = fmt.Fprintf(w, "stopped at the %d-call cap; the result may shrink further with a higher -max-tests\n", stats.Tests)
	}
	if panics > 0 {
		_, _ = fmt.Fprintf(w, "%d candidate(s) made the parser panic; the first:\n%s\n", panics, firstPanic)
	}
	return 0
}

// site is where a gap's or false accept's error is: for a gap, the byte
// offset of zsh-lint's first error; for a false accept, the byte range of
// the line Zsh reports.
type site struct {
	start, end int
}

// matches reports whether the candidate's site c holds the original site
// s, mapped through the candidate's byte origins.
func (s site) matches(c site, candidate reduce.Candidate, originalLength int) bool {
	if s.end == s.start {
		return candidate.OriginAt(c.start, originalLength) == s.start
	}
	for offset := c.start; offset < c.end; offset++ {
		if origin := candidate.OriginAt(offset, originalLength); origin >= s.start && origin < s.end {
			return true
		}
	}
	return false
}

// errorSite finds a judgement's site in text, which the file path held.
func errorSite(j Judgement, path, text string) (site, bool) {
	switch j.Class {
	case ClassGap:
		// "<path>:LINE:COL: message"; COL counts bytes from 1.
		line, col, ok := lineCol(strings.TrimPrefix(j.Lint.Diagnostic, path+":"), true)
		if !ok {
			return site{}, false
		}
		start, _, ok := lineRange(text, line)
		if !ok || start+col-1 > len(text) {
			return site{}, false
		}
		return site{start: start + col - 1, end: start + col - 1}, true
	case ClassFalseAccept:
		// "<path>:LINE: message"
		line, _, ok := lineCol(strings.TrimPrefix(j.Native, path+":"), false)
		if !ok {
			return site{}, false
		}
		start, end, ok := lineRange(text, line)
		return site{start: start, end: end}, ok
	default:
		return site{}, false
	}
}

// lineCol reads a leading "LINE:" or "LINE:COL:".
func lineCol(s string, withCol bool) (line, col int, ok bool) {
	fields := strings.SplitN(s, ":", 3)
	if len(fields) < 2 {
		return 0, 0, false
	}
	line, err := strconv.Atoi(fields[0])
	if err != nil || line < 1 {
		return 0, 0, false
	}
	if !withCol {
		return line, 0, true
	}
	col, err = strconv.Atoi(fields[1])
	if err != nil || col < 1 || len(fields) < 3 {
		return 0, 0, false
	}
	return line, col, true
}

// lineRange returns the byte range of line (from 1) in text, its newline
// included. A line just past the last one is the empty range at the end.
func lineRange(text string, line int) (start, end int, ok bool) {
	for n := 1; n < line; n++ {
		i := strings.IndexByte(text[start:], '\n')
		if i < 0 {
			return 0, 0, false
		}
		start += i + 1
	}
	end = len(text)
	if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
		end = start + i + 1
	}
	return start, end, true
}
