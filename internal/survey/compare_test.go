package survey

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const (
	okFile  = "testdata/ok.zsh"
	gapFile = "testdata/gap.zsh"
)

func fixedBase(verdicts map[string]Verdict) func([]string) (map[string]Verdict, error) {
	return func([]string) (map[string]Verdict, error) { return verdicts, nil }
}

func fixedNative(valid bool) func(string) (bool, error) {
	return func(string) (bool, error) { return valid, nil }
}

func TestCompareClassifiesVerdictChanges(t *testing.T) {
	gapNow := candidateVerdict(gapFile)
	if gapNow.OK {
		t.Fatalf("%s parses; the test needs a failing fixture", gapFile)
	}

	tests := []struct {
		name     string
		file     string
		base     Verdict
		native   func(string) (bool, error)
		wantLine string
		wantCode int
	}{
		{"fixed", okFile, Verdict{Diagnostic: okFile + ":1:1: old"}, fixedNative(true), "FIXED " + okFile, 0},
		{"false accept", okFile, Verdict{Diagnostic: okFile + ":1:1: old"}, fixedNative(false), "FALSE-ACCEPT " + okFile, 1},
		{"now ok without native", okFile, Verdict{Diagnostic: okFile + ":1:1: old"}, nil, "NOW-OK " + okFile, 0},
		{"regressed", gapFile, Verdict{OK: true}, fixedNative(true), "REGRESSED " + gapFile, 1},
		{"rejected", gapFile, Verdict{OK: true}, fixedNative(false), "REJECTED " + gapFile, 0},
		{"now fail without native", gapFile, Verdict{OK: true}, nil, "NOW-FAIL " + gapFile, 1},
		{"moved", gapFile, Verdict{Diagnostic: gapFile + ":1:1: old"}, fixedNative(true), "MOVED " + gapFile, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := Compare([]string{tt.file}, &out, CompareOptions{
				Base:   fixedBase(map[string]Verdict{tt.file: tt.base}),
				Native: tt.native,
			})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; output:\n%s", code, tt.wantCode, out.String())
			}
			lines := strings.Split(out.String(), "\n")
			if lines[0] != tt.wantLine {
				t.Fatalf("first line = %q, want %q; output:\n%s", lines[0], tt.wantLine, out.String())
			}
			if !strings.Contains(out.String(), "1 file(s) compared, 0 unchanged, 1 "+strings.ToLower(strings.Fields(tt.wantLine)[0])) {
				t.Fatalf("summary missing the class count; output:\n%s", out.String())
			}
		})
	}
}

func TestCompareShowsDiagnostics(t *testing.T) {
	gapNow := candidateVerdict(gapFile)

	var out bytes.Buffer
	Compare([]string{gapFile}, &out, CompareOptions{
		Base: fixedBase(map[string]Verdict{gapFile: {Diagnostic: gapFile + ":1:1: old"}}),
	})
	want := "MOVED " + gapFile + "\n  base: " + gapFile + ":1:1: old\n  now:  " + gapNow.Diagnostic + "\n"
	if !strings.HasPrefix(out.String(), want) {
		t.Fatalf("output = %q, want prefix %q", out.String(), want)
	}

	out.Reset()
	Compare([]string{gapFile}, &out, CompareOptions{Base: fixedBase(map[string]Verdict{gapFile: {OK: true}})})
	if want := "NOW-FAIL " + gapFile + "\n" + gapNow.Diagnostic + "\n"; !strings.HasPrefix(out.String(), want) {
		t.Fatalf("output = %q, want prefix %q", out.String(), want)
	}
}

func TestCompareUnchanged(t *testing.T) {
	names := []string{okFile, gapFile}
	base := map[string]Verdict{okFile: candidateVerdict(okFile), gapFile: candidateVerdict(gapFile)}

	var out bytes.Buffer
	code := Compare(names, &out, CompareOptions{Base: fixedBase(base)})
	if code != 0 || out.String() != "\n2 file(s) compared, 2 unchanged\n" {
		t.Fatalf("code = %d, output = %q", code, out.String())
	}
}

// With a native verdict, unchanged files that still disagree with Zsh are
// counted as known gaps and false accepts, listed only on request, and never
// change the exit status (#428).
func TestCompareCountsKnownDisagreements(t *testing.T) {
	names := []string{okFile, gapFile}
	base := map[string]Verdict{okFile: candidateVerdict(okFile), gapFile: candidateVerdict(gapFile)}
	// okFile parses but Zsh calls it invalid; gapFile fails but Zsh calls it
	// valid: one known false accept and one known gap.
	native := func(name string) (bool, error) { return name == gapFile, nil }

	var out bytes.Buffer
	code := Compare(names, &out, CompareOptions{Base: fixedBase(base), Native: native})
	if want := "\n2 file(s) compared, 2 unchanged; known: 1 gap(s), 1 false accept(s)\n"; code != 0 || out.String() != want {
		t.Fatalf("code = %d, output = %q, want %q", code, out.String(), want)
	}

	out.Reset()
	Compare(names, &out, CompareOptions{Base: fixedBase(base), Native: native, ListKnown: true})
	got := out.String()
	if !strings.Contains(got, "ACCEPTED "+okFile+"\n") || !strings.Contains(got, "GAP "+gapFile+"\n"+candidateVerdict(gapFile).Diagnostic+"\n") {
		t.Fatalf("listing lacks the known lines:\n%s", got)
	}
}

func TestCompareReportsMissingVerdicts(t *testing.T) {
	var out bytes.Buffer
	if code := Compare([]string{okFile}, &out, CompareOptions{Base: fixedBase(map[string]Verdict{})}); code != 2 {
		t.Fatalf("missing base verdict: code = %d, want 2; output:\n%s", code, out.String())
	}

	out.Reset()
	failing := func([]string) (map[string]Verdict, error) { return nil, errors.New("boom") }
	if code := Compare([]string{okFile}, &out, CompareOptions{Base: failing}); code != 2 {
		t.Fatalf("base error: code = %d, want 2", code)
	}

	out.Reset()
	brokenNative := func(string) (bool, error) { return false, errors.New("no zsh") }
	code := Compare([]string{okFile}, &out, CompareOptions{
		Base:   fixedBase(map[string]Verdict{okFile: {Diagnostic: "x"}}),
		Native: brokenNative,
	})
	if code != 2 {
		t.Fatalf("native error: code = %d, want 2", code)
	}
}

// ParseReport must read back exactly what Run writes, since the base build
// is an older copy of this command.
func TestParseReportReadsRunOutput(t *testing.T) {
	names := []string{okFile, gapFile}
	var out bytes.Buffer
	Run(names, &out)

	got, err := ParseReport(&out)
	if err != nil {
		t.Fatalf("ParseReport: %v", err)
	}
	for _, name := range names {
		if want := candidateVerdict(name); got[name] != want {
			t.Fatalf("%s: ParseReport = %+v, candidate = %+v", name, got[name], want)
		}
	}
	if len(got) != len(names) {
		t.Fatalf("ParseReport returned %d verdicts, want %d: %+v", len(got), len(names), got)
	}

	if _, err := ParseReport(strings.NewReader("FAIL x.zsh\n")); err == nil {
		t.Fatal("ParseReport accepted a FAIL line without a diagnostic")
	}
}
