package syntax

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestMainCleanup(t *testing.T) {
	for _, result := range []string{"pass", "fail"} {
		t.Run(result, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "path")
			cmd := exec.Command(os.Args[0], "-test.run=^TestTestMainCleanupProbe$")
			cmd.Env = append(os.Environ(), "TMPDIR="+dir, "TESTMAIN_CLEANUP_MARKER="+marker, "TESTMAIN_CLEANUP_RESULT="+result)
			out, err := cmd.CombinedOutput()
			want := 0
			if result == "fail" {
				want = 1
			}
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != want {
				t.Fatalf("subprocess exit: want %d, got %v: %v\n%s", want, cmd.ProcessState, err, out)
			}
			body, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			path := string(body)
			if filepath.Dir(path) != dir || !strings.HasPrefix(filepath.Base(path), "interp-bin-") {
				t.Fatalf("unexpected setup directory: %q", path)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("TestMain left setup directory %s: %v", path, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != "path" {
				t.Errorf("cleanup must preserve unrelated files: %v %v", entries, err)
			}
		})
	}
}

func TestTestMainCleanupProbe(t *testing.T) {
	marker := os.Getenv("TESTMAIN_CLEANUP_MARKER")
	if marker == "" {
		return
	}
	path := filepath.SplitList(os.Getenv("PATH"))[0]
	if _, err := os.Stat(filepath.Join(path, "foo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(path), 0o600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TESTMAIN_CLEANUP_RESULT") == "fail" {
		t.Error("intentional subprocess failure")
	}
}
