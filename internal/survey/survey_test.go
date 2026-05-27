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
	if !strings.Contains(got, "testdata/gap.zsh:1:") {
		t.Fatalf("expected path:line:col diagnostic; got:\n%s", got)
	}
	// The path must appear exactly once per diagnostic (no doubled prefix).
	if strings.Contains(got, "testdata/gap.zsh: testdata/gap.zsh") {
		t.Fatalf("diagnostic has doubled path prefix; got:\n%s", got)
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
