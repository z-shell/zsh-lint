package survey

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixedNativeDiagnostic(message string) func(string) (string, error) {
	return func(string) (string, error) { return message, nil }
}

func fixedLint(verdict Verdict) func(string) (Verdict, error) {
	return func(string) (Verdict, error) { return verdict, nil }
}

func TestJudgeClassifies(t *testing.T) {
	const name = "dir/f.zsh"
	reject := Verdict{Diagnostic: name + ":3:7: `;` can only immediately follow a statement"}
	tests := []struct {
		name       string
		native     string
		lint       Verdict
		wantClass  string
		wantFamily string
	}{
		{"gap", "", reject, ClassGap, "GAP: `;` can only immediately follow a statement"},
		{"false accept", name + ":2: parse error near `}'", Verdict{OK: true}, ClassFalseAccept, "FALSE-ACCEPT: parse error near `}'"},
		{"both accept", "", Verdict{OK: true}, ClassAgree, ""},
		{"both reject", name + ":1: parse error", reject, ClassAgree, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			judgement, err := Judge(name, fixedNativeDiagnostic(tt.native), fixedLint(tt.lint))
			if err != nil {
				t.Fatal(err)
			}
			if judgement.Class != tt.wantClass {
				t.Fatalf("class = %s, want %s", judgement.Class, tt.wantClass)
			}
			if got := judgement.Family(name); got != tt.wantFamily {
				t.Fatalf("family = %q, want %q", got, tt.wantFamily)
			}
		})
	}
}

// A family names the defect, not where it is: the same message at another
// position is the same family, and a position quoted inside the message is
// abstracted too. Quoted literals are kept, so two operators stay apart.
func TestFamilyAbstractsPositions(t *testing.T) {
	const name = "f.zsh"
	family := func(diagnostic string) string {
		return Judgement{Class: ClassGap, Lint: Verdict{Diagnostic: diagnostic}}.Family(name)
	}
	if a, b := family(name+":1:9: `|` must be followed by a statement"), family(name+":40:2: `|` must be followed by a statement"); a != b {
		t.Errorf("positions split one family: %q, %q", a, b)
	}
	if a, b := family(name+":2:1: reached `)` without matching `(` at 1:4"), family(name+":7:1: reached `)` without matching `(` at 6:12"); a != b {
		t.Errorf("a position inside the message split one family: %q, %q", a, b)
	}
	if a, b := family(name+":1:9: `|` must be followed by a statement"), family(name+":1:9: `&&` must be followed by a statement"); a == b {
		t.Errorf("two operators share the family %q", a)
	}
	native := func(diagnostic string) string {
		return Judgement{Class: ClassFalseAccept, Native: diagnostic, Lint: Verdict{OK: true}}.Family(name)
	}
	if a, b := native(name+":1: parse error near `}'"), native(name+":12: parse error near `}'"); a != b {
		t.Errorf("native line numbers split one family: %q, %q", a, b)
	}
}

func TestJudgeReportsOracleErrors(t *testing.T) {
	broken := errors.New("broken")
	if _, err := Judge("f.zsh", func(string) (string, error) { return "", broken }, fixedLint(Verdict{OK: true})); !errors.Is(err, broken) {
		t.Fatalf("native error = %v", err)
	}
	if _, err := Judge("f.zsh", fixedNativeDiagnostic(""), func(string) (Verdict, error) { return Verdict{}, broken }); !errors.Is(err, broken) {
		t.Fatalf("lint error = %v", err)
	}
}

func TestJudgeFilesPrintsBothVerdicts(t *testing.T) {
	var out bytes.Buffer
	lint := func(name string) (Verdict, error) {
		if name == "gap.zsh" {
			return Verdict{Diagnostic: "gap.zsh:1:1: bad"}, nil
		}
		return Verdict{OK: true}, nil
	}
	code := JudgeFiles([]string{"gap.zsh", "ok.zsh"}, &out, fixedNativeDiagnostic(""), lint)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", code, out.String())
	}
	want := "GAP gap.zsh\n  zsh -f -n: accept\n  zsh-lint:  gap.zsh:1:1: bad\n" +
		"AGREE ok.zsh\n  zsh -f -n: accept\n  zsh-lint:  accept\n" +
		"\n2 file(s) judged, 1 gap(s), 0 false accept(s), 1 agree\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}

	out.Reset()
	if code := JudgeFiles([]string{"ok.zsh"}, &out, fixedNativeDiagnostic(""), lint); code != 0 {
		t.Fatalf("exit code = %d for an agreeing file; output:\n%s", code, out.String())
	}
	out.Reset()
	if code := JudgeFiles([]string{"ok.zsh"}, &out, func(string) (string, error) { return "", errors.New("no zsh") }, lint); code != 2 {
		t.Fatalf("exit code = %d for a failed oracle; output:\n%s", code, out.String())
	}
}

// `zsh -n` exits 1 with no diagnostic for a valid negated pipeline, so the
// helper judges by standard error, never by the exit status (#562).
func TestNativeZshDiagnosticIgnoresTheExitStatus(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required to judge a file")
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	negated := write("negated.zsh", "! true\n")
	if exec.Command(zsh, "-f", "-n", negated).Run() == nil {
		t.Fatal("zsh -f -n exits 0 on `! true`; the row no longer tests the exit status")
	}
	native := NativeZshDiagnostic(zsh)
	if message, err := native(negated); err != nil || message != "" {
		t.Fatalf("`! true` judged %q, %v; want valid", message, err)
	}
	judgement, err := Judge(negated, native, BuildVerdict)
	if err != nil || judgement.Class != ClassAgree || judgement.Native != "" {
		t.Fatalf("`! true` judgement = %+v, %v; want both accepting", judgement, err)
	}

	invalid := write("invalid.zsh", "for } in a; do :; done\n")
	message, err := native(invalid)
	if err != nil || !strings.HasPrefix(message, invalid+":1: ") || !strings.Contains(message, "parse error") {
		t.Fatalf("invalid source judged %q, %v; want its first diagnostic", message, err)
	}
}

func TestBuildVerdictRecoversAPanic(t *testing.T) {
	verdict, err := BuildVerdict(okFile)
	if err != nil || !verdict.OK {
		t.Fatalf("BuildVerdict(%s) = %+v, %v", okFile, verdict, err)
	}
	_, err = recoverPanic(func(string) Verdict { panic("boom") })("f.zsh")
	var panicked *PanicError
	if !errors.As(err, &panicked) || err.Error() != "parser panic: boom" {
		t.Fatalf("panic returned %v, want a *PanicError", err)
	}
}
