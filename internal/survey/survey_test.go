package survey

import (
	"bytes"
	"strings"
	"testing"
)

func TestSurveyOK(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"testdata/ok.zsh"}, &out)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "OK   testdata/ok.zsh") {
		t.Fatalf("missing OK line; got:\n%s", got)
	}
	if !strings.Contains(got, "1 file") || !strings.Contains(got, "0 failed") {
		t.Fatalf("missing/incorrect summary; got:\n%s", got)
	}
}

func TestSurveyParseGap(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"testdata/gap.zsh"}, &out)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	got := out.String()
	// Greppable diagnostic: path:line:col: message
	if !strings.Contains(got, "testdata/gap.zsh:3:") {
		t.Fatalf("expected path:line:col diagnostic; got:\n%s", got)
	}
	// The path must appear exactly once per diagnostic (no doubled prefix).
	if strings.Contains(got, "testdata/gap.zsh: testdata/gap.zsh") {
		t.Fatalf("diagnostic has doubled path prefix; got:\n%s", got)
	}
	// The diagnostic must begin at column 0 (no indent) so it is greppable
	// and usable by editor problem matchers.
	var sawDiag bool
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "testdata/gap.zsh:") {
			sawDiag = true
		}
	}
	if !sawDiag {
		t.Fatalf("expected a diagnostic line starting at column 0 with the path; got:\n%s", got)
	}
	if !strings.Contains(got, "1 failed") {
		t.Fatalf("expected summary to report 1 failed; got:\n%s", got)
	}
}

func TestSurveyMissingFile(t *testing.T) {
	var out bytes.Buffer
	code := Run([]string{"testdata/does-not-exist.zsh"}, &out)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "does-not-exist.zsh") {
		t.Fatalf("expected error line referencing the file; got:\n%s", out.String())
	}
}

func TestSurveyTraceParses(t *testing.T) {
	names := []string{"testdata/ok.zsh", "testdata/gap.zsh"}

	var plain bytes.Buffer
	plainCode := Run(names, &plain)

	var out, trace bytes.Buffer
	code := RunWithOptions(names, &out, Options{Trace: &trace})
	if code != plainCode {
		t.Fatalf("exit code with trace = %d, without = %d", code, plainCode)
	}
	if out.String() != plain.String() {
		t.Fatalf("standard output changed with trace:\n%s\nwant:\n%s", out.String(), plain.String())
	}

	lines := strings.Split(strings.TrimSpace(trace.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("trace has %d lines, want one per file plus a total:\n%s", len(lines), trace.String())
	}
	for i, name := range names {
		if !strings.HasPrefix(lines[i], "TRACE "+name+" parses=") || !strings.Contains(lines[i], " adapter-depth=") {
			t.Fatalf("trace line %d = %q, want TRACE %s parses=<n> adapter-depth=<d>", i, lines[i], name)
		}
		if strings.HasPrefix(lines[i], "TRACE "+name+" parses=0 ") {
			t.Fatalf("trace line %d = %q, want at least one parse", i, lines[i])
		}
	}
	if !strings.HasPrefix(lines[2], "TRACE total files=2 parses=") {
		t.Fatalf("trace total = %q, want TRACE total files=2 parses=<n> max-adapter-depth=<d>", lines[2])
	}
}
