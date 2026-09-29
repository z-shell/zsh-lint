package survey

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// paddingFixture is valid Zsh with functions, loops, conditionals and
// substitutions spanning lines, to bury a construct in.
const paddingFixture = "testdata/corpus/ok-alternate-if-condition-brace.zsh"

// markerLint is a zsh-lint stand-in that rejects the first `MARK` word at its
// position and accepts everything else, so a test controls where the gap is
// without depending on which gaps today's parser still has.
func markerLint(name string) (Verdict, error) {
	src, err := os.ReadFile(name)
	if err != nil {
		return Verdict{}, err
	}
	i := bytes.Index(src, []byte("MARK"))
	if i < 0 {
		return Verdict{OK: true}, nil
	}
	line := bytes.Count(src[:i], []byte("\n")) + 1
	col := i - (bytes.LastIndexByte(src[:i], '\n') + 1) + 1
	return Verdict{Diagnostic: fmt.Sprintf("%s:%d:%d: `MARK` is not supported", name, line, col)}, nil
}

func requireZsh(t *testing.T) string {
	t.Helper()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required to judge candidates")
	}
	return zsh
}

// buried writes padding with extra inserted after line at, and returns the
// file's path.
func buried(t *testing.T, at int, extra string) string {
	t.Helper()
	padding, err := os.ReadFile(paddingFixture)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(padding), "\n")
	src := strings.Join(lines[:at], "") + extra + strings.Join(lines[at:], "")
	path := filepath.Join(t.TempDir(), "buried.zsh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A gap buried in unrelated valid code reduces to the construct alone, and
// every candidate the reducer keeps is valid Zsh (#542, #562).
func TestReduceFileShrinksABuriedGap(t *testing.T) {
	zsh := requireZsh(t)
	name := buried(t, 30, "  print before MARK after\n")
	var out, report bytes.Buffer
	native := NativeZshDiagnostic(zsh)
	var invalid []string
	code := ReduceFile(name, &report, ReduceOptions{
		Native: native,
		Lint: func(path string) (Verdict, error) {
			verdict, err := markerLint(path)
			if message, _ := native(path); message != "" && !verdict.OK {
				src, _ := os.ReadFile(path)
				invalid = append(invalid, string(src))
			}
			return verdict, err
		},
		Out: &out,
	})
	if code != 0 {
		t.Fatalf("exit code = %d; report:\n%s", code, report.String())
	}
	if out.String() != "MARK\n" {
		t.Fatalf("reduced = %q, want %q; report:\n%s", out.String(), "MARK\n", report.String())
	}
	if len(invalid) == 0 {
		t.Fatal("no candidate was invalid Zsh, so the test does not show that the reducer refuses them")
	}
	if !strings.Contains(report.String(), "GAP: `MARK` is not supported\n") {
		t.Fatalf("report does not name the family:\n%s", report.String())
	}
}

// With a second occurrence of the same construct, the reducer keeps the one
// the original error was at, not whichever is easier to keep.
func TestReduceFileKeepsTheOriginalSite(t *testing.T) {
	zsh := requireZsh(t)
	// The second MARK sits in a line of its own and would reduce just as
	// far, so only the site check decides which one stays. It is spelled
	// MARKS, which markerLint reports with the same message, so the result
	// shows which of the two was kept.
	name := buried(t, 10, "print MARK first\n")
	src, _ := os.ReadFile(name)
	src = append(src, []byte("print MARKS second\n")...)
	if err := os.WriteFile(name, src, 0o600); err != nil {
		t.Fatal(err)
	}
	var sawSecond bool
	var out, report bytes.Buffer
	code := ReduceFile(name, &report, ReduceOptions{
		Native: NativeZshDiagnostic(zsh),
		Lint: func(path string) (Verdict, error) {
			candidate, _ := os.ReadFile(path)
			if bytes.Count(candidate, []byte("MARK")) == 1 && bytes.Contains(candidate, []byte("second")) {
				sawSecond = true
			}
			return markerLint(path)
		},
		Out: &out,
	})
	if code != 0 || out.String() != "MARK\n" {
		t.Fatalf("exit code %d, reduced %q; report:\n%s", code, out.String(), report.String())
	}
	if !sawSecond {
		t.Fatal("no candidate kept only the second MARK, so the site check is untested")
	}
}

// A candidate with the same class and site but another first message is a
// different defect, so the reducer refuses it.
func TestReduceFileKeepsTheOriginalMessage(t *testing.T) {
	zsh := requireZsh(t)
	name := buried(t, 30, "  print keep MARK\n")
	var sawOther bool
	var out, report bytes.Buffer
	code := ReduceFile(name, &report, ReduceOptions{
		Native: NativeZshDiagnostic(zsh),
		Lint: func(path string) (Verdict, error) {
			verdict, err := markerLint(path)
			src, _ := os.ReadFile(path)
			if !verdict.OK && !bytes.Contains(src, []byte("keep")) {
				sawOther = true
				verdict.Diagnostic = strings.Replace(verdict.Diagnostic, "is not supported", "is another defect", 1)
			}
			return verdict, err
		},
		Out: &out,
	})
	if code != 0 || out.String() != "keep MARK\n" {
		t.Fatalf("exit code %d, reduced %q; report:\n%s", code, out.String(), report.String())
	}
	if !sawOther {
		t.Fatal("no candidate changed the message, so the family check is untested")
	}
}

// A false accept reduces while Zsh keeps rejecting it with the same message
// on the original line.
func TestReduceFileShrinksABuriedFalseAccept(t *testing.T) {
	zsh := requireZsh(t)
	const construct = "for } in a; do :; done\n"
	name := buried(t, 30, construct)
	var out, report bytes.Buffer
	code := ReduceFile(name, &report, ReduceOptions{
		Native: NativeZshDiagnostic(zsh),
		Lint:   fixedLint(Verdict{OK: true}),
		Out:    &out,
	})
	if code != 0 {
		t.Fatalf("exit code = %d; report:\n%s", code, report.String())
	}
	if len(out.String()) >= len(construct) || !strings.Contains(out.String(), "}") {
		t.Fatalf("reduced = %q, want a part of %q; report:\n%s", out.String(), construct, report.String())
	}
	reduced := filepath.Join(t.TempDir(), "reduced.zsh")
	if err := os.WriteFile(reduced, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if message, err := NativeZshDiagnostic(zsh)(reduced); err != nil || !strings.HasSuffix(message, "parse error near `}'") {
		t.Fatalf("the reduced source is judged %q, %v; want the original's error", message, err)
	}
}

// With a second false accept on a later line, the reducer keeps the line Zsh
// reported, not the other one with the same message.
func TestReduceFileKeepsTheFalseAcceptLine(t *testing.T) {
	zsh := requireZsh(t)
	// Simple commands between the two, so removing the first line leaves
	// the second as a candidate Zsh rejects with the same message.
	src := "for } in LEFT; do :; done\n" + strings.Repeat(": pad\n", 6) + "for } in RIGHT; do :; done\n"
	name := filepath.Join(t.TempDir(), "two.zsh")
	if err := os.WriteFile(name, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	onlySecond := func(path string) bool {
		candidate, _ := os.ReadFile(path)
		return bytes.Contains(candidate, []byte("RIGHT")) && !bytes.Contains(candidate, []byte("LEFT"))
	}
	var judged, kept bool
	var out, report bytes.Buffer
	code := ReduceFile(name, &report, ReduceOptions{
		Native: NativeZshDiagnostic(zsh),
		Lint: func(path string) (Verdict, error) {
			judged = judged || onlySecond(path)
			return Verdict{OK: true}, nil
		},
		// Fixed is asked only about candidates that kept family and site.
		Fixed: func(path string) (Verdict, error) {
			kept = kept || onlySecond(path)
			return Verdict{Diagnostic: path + ":1:1: rejected"}, nil
		},
		Out: &out,
	})
	if code != 0 {
		t.Fatalf("exit code = %d; report:\n%s", code, report.String())
	}
	if !judged {
		t.Fatal("no candidate kept only the second false accept, so the line check is untested")
	}
	if kept {
		t.Fatalf("a candidate with only the second false accept was kept; reduced %q", out.String())
	}
}

// With a fixed build, a candidate is kept only while that build accepts it,
// so the reduction stays on the defect the fix addressed.
func TestReduceFileWithAFixedBuild(t *testing.T) {
	zsh := requireZsh(t)
	name := buried(t, 30, "  print keep MARK\n")
	fixedBuild := func(path string) (Verdict, error) {
		src, err := os.ReadFile(path)
		if err != nil {
			return Verdict{}, err
		}
		if bytes.Contains(src, []byte("keep")) {
			return Verdict{OK: true}, nil
		}
		return Verdict{Diagnostic: path + ":1:1: still broken"}, nil
	}
	var out, report bytes.Buffer
	code := ReduceFile(name, &report, ReduceOptions{
		Native: NativeZshDiagnostic(zsh),
		Lint:   markerLint,
		Fixed:  fixedBuild,
		Out:    &out,
	})
	if code != 0 || out.String() != "keep MARK\n" {
		t.Fatalf("exit code %d, reduced %q; report:\n%s", code, out.String(), report.String())
	}

	out.Reset()
	report.Reset()
	code = ReduceFile(name, &report, ReduceOptions{
		Native: NativeZshDiagnostic(zsh),
		Lint:   markerLint,
		Fixed:  fixedLint(Verdict{Diagnostic: "x:1:1: still broken"}),
		Out:    &out,
	})
	if code != 1 || !strings.Contains(report.String(), "the fixed build does not fix it") {
		t.Fatalf("exit code %d for a build that fixes nothing; report:\n%s", code, report.String())
	}
}

func TestReduceFileRefusesAnAgreeingFile(t *testing.T) {
	var out, report bytes.Buffer
	code := ReduceFile(okFile, &report, ReduceOptions{
		Native: fixedNativeDiagnostic(""),
		Lint:   fixedLint(Verdict{OK: true}),
		Out:    &out,
	})
	if code != 1 || out.Len() != 0 || !strings.Contains(report.String(), "agree; nothing to reduce") {
		t.Fatalf("exit code %d, output %q; report:\n%s", code, out.String(), report.String())
	}
}

func TestReduceFileReportsAPanic(t *testing.T) {
	var out, report bytes.Buffer
	code := ReduceFile(okFile, &report, ReduceOptions{
		Native: fixedNativeDiagnostic(""),
		Lint:   func(string) (Verdict, error) { return Verdict{}, &PanicError{Value: "boom"} },
		Out:    &out,
	})
	if code != 2 || !strings.Contains(report.String(), "the parser panics on it") {
		t.Fatalf("exit code %d; report:\n%s", code, report.String())
	}
}

func TestErrorSite(t *testing.T) {
	const path = "/d/f.zsh"
	text := "ab\ncdef\n"
	gap := Judgement{Class: ClassGap, Lint: Verdict{Diagnostic: path + ":2:3: x"}}
	if s, ok := errorSite(gap, path, text); !ok || s != (site{start: 5, end: 5}) {
		t.Errorf("gap site = %+v, %v; want offset 5", s, ok)
	}
	accept := Judgement{Class: ClassFalseAccept, Native: path + ":2: parse error near `x'"}
	if s, ok := errorSite(accept, path, text); !ok || s != (site{start: 3, end: 8}) {
		t.Errorf("false-accept site = %+v, %v; want line 2 at 3..8", s, ok)
	}
	for _, bad := range []Judgement{
		{Class: ClassGap, Lint: Verdict{Diagnostic: path + ": no position"}},
		{Class: ClassGap, Lint: Verdict{Diagnostic: path + ":9:1: past the end"}},
		{Class: ClassFalseAccept, Native: "zsh: no file position"},
		{Class: ClassAgree},
	} {
		if s, ok := errorSite(bad, path, text); ok {
			t.Errorf("errorSite(%+v) = %+v, want no site", bad, s)
		}
	}
}
