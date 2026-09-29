package survey

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// Zsh reports a file it cannot open on standard error, which NativeZsh must
// not read as a rejection: a missing file is an error, not invalid Zsh.
func TestNativeZshMissingFileIsAnError(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required to judge a file")
	}
	native := NativeZsh(zsh)
	if valid, err := native(okFile); err != nil || !valid {
		t.Fatalf("%s judged valid=%v, %v; want valid", okFile, valid, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.zsh")
	if valid, err := native(missing); err == nil {
		t.Fatalf("missing file judged valid=%v with no error", valid)
	}
	if valid, err := native(t.TempDir()); err == nil {
		t.Fatalf("directory judged valid=%v with no error", valid)
	}
	// A file that went from OK to FAIL is judged natively; the missing file's
	// open error must end the comparison, not classify it.
	var out bytes.Buffer
	code := Compare([]string{missing}, &out, CompareOptions{
		Base:   fixedBase(map[string]Verdict{missing: {OK: true}}),
		Native: native,
	})
	if code != 2 {
		t.Fatalf("missing file: code = %d, want 2; output:\n%s", code, out.String())
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

// -runtime turns a REGRESSED file that Zsh rejects when run into
// RUNTIME-REJECTED, which does not fail the comparison; one that runs clean
// stays REGRESSED (#545).
func TestCompareRuntimeRejected(t *testing.T) {
	runtime := func(message string) func(string) (string, error) {
		return func(string) (string, error) { return message, nil }
	}
	tests := []struct {
		name     string
		runtime  func(string) (string, error)
		wantLine string
		wantCode int
	}{
		{"runtime error", runtime("invalid subscript"), "RUNTIME-REJECTED " + gapFile, 0},
		{"runs clean", runtime(""), "REGRESSED " + gapFile, 1},
		{"no runtime", nil, "REGRESSED " + gapFile, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := Compare([]string{gapFile}, &out, CompareOptions{
				Base:    fixedBase(map[string]Verdict{gapFile: {OK: true}}),
				Native:  fixedNative(true),
				Runtime: tt.runtime,
			})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; output:\n%s", code, tt.wantCode, out.String())
			}
			if lines := strings.Split(out.String(), "\n"); lines[0] != tt.wantLine {
				t.Fatalf("first line = %q, want %q; output:\n%s", lines[0], tt.wantLine, out.String())
			}
		})
	}

	var out bytes.Buffer
	Compare([]string{gapFile}, &out, CompareOptions{
		Base:    fixedBase(map[string]Verdict{gapFile: {OK: true}}),
		Native:  fixedNative(true),
		Runtime: runtime("invalid subscript"),
	})
	if !strings.Contains(out.String(), "  run:  invalid subscript\n") ||
		!strings.Contains(out.String(), "1 file(s) compared, 0 unchanged, 1 runtime-rejected") {
		t.Fatalf("output lacks the runtime error or count:\n%s", out.String())
	}

	// Only a REGRESSED file is run: a false accept, a fix or a rejection
	// keeps its class.
	ran := false
	spy := func(string) (string, error) { ran = true; return "x", nil }
	out.Reset()
	Compare([]string{okFile}, &out, CompareOptions{
		Base:    fixedBase(map[string]Verdict{okFile: {Diagnostic: okFile + ":1:1: old"}}),
		Native:  fixedNative(false),
		Runtime: spy,
	})
	if ran || !strings.HasPrefix(out.String(), "FALSE-ACCEPT "+okFile) {
		t.Fatalf("ran = %v; output:\n%s", ran, out.String())
	}
	out.Reset()
	Compare([]string{gapFile}, &out, CompareOptions{
		Base:    fixedBase(map[string]Verdict{gapFile: {OK: true}}),
		Native:  fixedNative(false),
		Runtime: spy,
	})
	if ran || !strings.HasPrefix(out.String(), "REJECTED "+gapFile) {
		t.Fatalf("ran = %v; output:\n%s", ran, out.String())
	}

	out.Reset()
	broken := func(string) (string, error) { return "", errors.New("no zsh") }
	if code := Compare([]string{gapFile}, &out, CompareOptions{
		Base:    fixedBase(map[string]Verdict{gapFile: {OK: true}}),
		Native:  fixedNative(true),
		Runtime: broken,
	}); code != 2 {
		t.Fatalf("runtime error: code = %d, want 2", code)
	}
}

// -candidate takes the candidate verdicts from another build, so this
// build can classify a merged fix against its base (#545).
func TestCompareCandidateBuild(t *testing.T) {
	var out bytes.Buffer
	code := Compare([]string{okFile}, &out, CompareOptions{
		Base:      fixedBase(map[string]Verdict{okFile: {OK: true}}),
		Candidate: fixedBase(map[string]Verdict{okFile: {Diagnostic: okFile + ":1:1: new"}}),
		Native:    fixedNative(false),
	})
	if want := "REJECTED " + okFile + "\n" + okFile + ":1:1: new\n"; code != 0 || !strings.HasPrefix(out.String(), want) {
		t.Fatalf("code = %d, output = %q, want prefix %q", code, out.String(), want)
	}

	out.Reset()
	if code := Compare([]string{okFile}, &out, CompareOptions{
		Base:      fixedBase(map[string]Verdict{okFile: {OK: true}}),
		Candidate: fixedBase(map[string]Verdict{}),
	}); code != 2 || !strings.Contains(out.String(), "candidate survey reported no verdict for "+okFile) {
		t.Fatalf("missing candidate verdict: code = %d, output = %q", code, out.String())
	}
	out.Reset()
	failing := func([]string) (map[string]Verdict, error) { return nil, errors.New("boom") }
	if code := Compare([]string{okFile}, &out, CompareOptions{
		Base:      fixedBase(map[string]Verdict{okFile: {OK: true}}),
		Candidate: failing,
	}); code != 2 {
		t.Fatalf("candidate error: code = %d, want 2", code)
	}
}

// -table writes the changed files, and only those, as the rows of a survey
// record, with each one-line source as a code span (#545).
func TestCompareTable(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	fixed := write("fixed.zsh", "print a|b `x` \n")
	regressed := write("regressed.zsh", "x=$x[(r)a[]\n")
	clean := write("clean.zsh", "print $x[(r)a[]]\n")
	moved := write("moved.zsh", "print a\nprint b\n")
	same := write("same.zsh", "print c\n")
	// The longest line still shown as source, and one byte more.
	longest := write("longest.zsh", "print "+strings.Repeat("a", 114)+"\n")
	longer := write("longer.zsh", "print "+strings.Repeat("a", 115)+"\n")
	names := []string{fixed, regressed, clean, moved, same, longest, longer}
	candidate := map[string]Verdict{
		fixed:     {OK: true},
		regressed: {Diagnostic: regressed + ":1:1: new"},
		clean:     {Diagnostic: clean + ":1:1: new"},
		moved:     {Diagnostic: moved + ":1:1: new"},
		same:      {OK: true},
		longest:   {OK: true},
		longer:    {OK: true},
	}
	base := map[string]Verdict{
		fixed:     {Diagnostic: fixed + ":1:1: old"},
		regressed: {OK: true},
		clean:     {OK: true},
		moved:     {Diagnostic: moved + ":1:1: old"},
		same:      {OK: true},
		longest:   {Diagnostic: longest + ":1:1: old"},
		longer:    {Diagnostic: longer + ":1:1: old"},
	}
	var out, table bytes.Buffer
	code := Compare(names, &out, CompareOptions{
		Base:      fixedBase(base),
		Candidate: fixedBase(candidate),
		Native:    func(name string) (bool, error) { return name != moved, nil },
		Runtime: func(name string) (string, error) {
			if name == clean {
				return "", nil
			}
			return "invalid | subscript", nil
		},
		Table: &table,
	})
	if code != 1 {
		t.Fatalf("code = %d; output:\n%s", code, out.String())
	}
	want := "| #   | Class | Source | zsh -f -n | base | candidate | run |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | FIXED | ``print a\\|b `x` `` | accept | reject | accept | - |\n" +
		"| 2 | RUNTIME-REJECTED | `x=$x[(r)a[]` | accept | accept | reject | invalid \\| subscript |\n" +
		"| 3 | REGRESSED | `print $x[(r)a[]]` | accept | accept | reject | none |\n" +
		"| 4 | MOVED | `" + moved + "` | reject | reject | reject | - |\n" +
		"| 5 | FIXED | `print " + strings.Repeat("a", 114) + "` | accept | reject | accept | - |\n" +
		"| 6 | FIXED | `" + longer + "` | accept | reject | accept | - |\n"
	if table.String() != want {
		t.Fatalf("table:\n%s\nwant:\n%s", table.String(), want)
	}

	// Without Runtime there is no run column.
	table.Reset()
	Compare([]string{fixed}, &out, CompareOptions{
		Base:      fixedBase(base),
		Candidate: fixedBase(candidate),
		Native:    fixedNative(true),
		Table:     &table,
	})
	if want := "| #   | Class | Source | zsh -f -n | base | candidate |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | FIXED | ``print a\\|b `x` `` | accept | reject | accept |\n"; table.String() != want {
		t.Fatalf("table:\n%s\nwant:\n%s", table.String(), want)
	}
}

// RuntimeZsh runs a file in an empty directory and reports Zsh's first
// error without its location; a clean run reports nothing (#545).
func TestRuntimeZsh(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required to run a file")
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	run := RuntimeZsh(zsh)
	// `zsh -f -n` accepts both; only running the first reports the
	// subscript (#539).
	for _, tt := range []struct{ content, want string }{
		{"x=$x[(r)a[]\n", "invalid subscript"},
		{"x=(a b)\nprint -r -- $x[(r)a]\n", ""},
		{"print ok >created\n", ""},
		// Whatever Zsh writes to standard error counts, whatever the status,
		// as it does for the native verdict.
		{"print -u2 complaint\n", "complaint"},
	} {
		got, err := run(write("row.zsh", tt.content))
		if err != nil || got != tt.want {
			t.Errorf("RuntimeZsh(%q) = %q, %v; want %q", tt.content, got, err, tt.want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "created")); err == nil {
		t.Error("RuntimeZsh ran the file in the file's own directory, not an empty one")
	}

	// A row that does not finish reports no error, so it stays REGRESSED
	// rather than being excused.
	defer func(limit time.Duration) { runtimeLimit = limit }(runtimeLimit)
	runtimeLimit = 200 * time.Millisecond
	if got, err := run(write("hang.zsh", "print -u2 started\nwhile :; do :; done\n")); err != nil || got != "" {
		t.Errorf("RuntimeZsh(hanging row) = %q, %v; want no error", got, err)
	}
}

// Without -table a MOVED file is not judged natively, so -compare spends
// no `zsh -n` run on it, as before #545.
func TestCompareMovedSkipsNativeWithoutTable(t *testing.T) {
	calls := 0
	native := func(string) (bool, error) { calls++; return true, nil }
	var out bytes.Buffer
	Compare([]string{gapFile}, &out, CompareOptions{
		Base:   fixedBase(map[string]Verdict{gapFile: {Diagnostic: gapFile + ":1:1: old"}}),
		Native: native,
	})
	if calls != 0 || !strings.HasPrefix(out.String(), "MOVED "+gapFile) {
		t.Fatalf("native calls = %d; output:\n%s", calls, out.String())
	}
	var table bytes.Buffer
	Compare([]string{gapFile}, &out, CompareOptions{
		Base:   fixedBase(map[string]Verdict{gapFile: {Diagnostic: gapFile + ":1:1: old"}}),
		Native: native,
		Table:  &table,
	})
	if calls != 1 {
		t.Fatalf("native calls with a table = %d, want 1", calls)
	}
}
