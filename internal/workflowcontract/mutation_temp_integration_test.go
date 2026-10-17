//go:build integration

package workflowcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Compile the real runner with a focused same-package test so its subprocess
// boundary can be exercised without a mutation campaign or a Git fixture.
func TestMutationOwnsTemporaryDirectories(t *testing.T) {
	dir := t.TempDir()
	body, err := os.ReadFile(repositoryFilePath(t, ".github", "scripts", "mutation.go"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"go.mod":                []byte("module example.invalid/mutationtemp\ngo 1.27.0\n"),
		"mutation.go":           body,
		"mutation_temp_test.go": []byte(mutationTempTests),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "-count=1", "-v", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runner temp regressions: %v\n%s", err, out)
	}
}

const mutationTempTests = `package main

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestRelativeTemporaryDirectory(t *testing.T) {
 root := t.TempDir()
 t.Chdir(root)
 t.Setenv("TMPDIR", ".")
 gotmp := filepath.Join(root, "caller-gotmp")
 if err := os.Mkdir(gotmp, 0700); err != nil { t.Fatal(err) }
 t.Setenv("GOTMPDIR", gotmp)
 work, err := os.MkdirTemp("", "relative-worker-")
 if err != nil { t.Fatal(err) }
 for name, body := range map[string]string{
  "go.mod": "module example.invalid/relativeprobe\ngo 1.27.0\n",
  "probe_test.go": "package probe\nimport \"testing\"\nfunc TestPass(t *testing.T) {}\n",
 } {
  if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0600); err != nil { t.Fatal(err) }
 }
 status, output := test(context.Background(), work, []testCommand{{"", []string{"."}}}, 15*time.Second, "off", false)
 if status != "LIVED" { t.Fatalf("relative worker did not pass: %s: %s", status, output) }
 entries, err := os.ReadDir(work)
 if err != nil || len(entries) != 2 { t.Fatalf("relative worker cleanup mismatch: %v %v", entries, err) }
 entries, err = os.ReadDir(gotmp)
 if err != nil || len(entries) != 0 { t.Fatalf("caller temporary directory changed: %v %v", entries, err) }
}

func TestTemporaryDirectories(t *testing.T) {
 for _, mode := range []string{"success", "timeout", "cancel"} {
  t.Run(mode, func(t *testing.T) {
   work, outside := t.TempDir(), t.TempDir()
   tmp, gotmp := filepath.Join(outside, "tmp"), filepath.Join(outside, "gotmp")
   for _, path := range []string{tmp, gotmp} {
    if err := os.Mkdir(path, 0700); err != nil { t.Fatal(err) }
   }
   sentinel := filepath.Join(outside, "keep")
   if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil { t.Fatal(err) }
   record := filepath.Join(outside, "record")
   t.Setenv("TMPDIR", tmp)
   t.Setenv("GOTMPDIR", gotmp)
   t.Setenv("TEMP_RECORD", record)
   t.Setenv("TEMP_MODE", mode)
   t.Setenv("TEMP_OUTSIDE", outside)
   source := "package probe\nimport (\"os\"; \"path/filepath\"; \"testing\"; \"time\")\nfunc TestTemp(t *testing.T) {\n" +
    "tmp, err := os.MkdirTemp(\"\", \"interp-bin-\"); if err != nil { t.Fatal(err) }; " +
    "if err := os.Symlink(os.Getenv(\"TEMP_OUTSIDE\"), filepath.Join(tmp, \"outside\")); err != nil { t.Fatal(err) }; " +
    "if err := os.WriteFile(os.Getenv(\"TEMP_RECORD\"), []byte(os.Getenv(\"TMPDIR\")+\"\\n\"+os.Getenv(\"GOTMPDIR\")+\"\\n\"+tmp), 0600); err != nil { t.Fatal(err) }; " +
    "if os.Getenv(\"TEMP_MODE\") != \"success\" { time.Sleep(time.Minute) } }\n"
   for name, body := range map[string]string{
    "go.mod": "module example.invalid/probe\ngo 1.27.0\n",
    "probe_test.go": source,
   } {
    if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0600); err != nil { t.Fatal(err) }
   }
   ctx, cancel := context.WithCancel(context.Background())
   defer cancel()
   timeout := 15 * time.Second
   done := make(chan struct{})
   if mode == "cancel" {
    go func() {
     defer close(done)
     ticker := time.NewTicker(10 * time.Millisecond)
     defer ticker.Stop()
     for {
      if _, err := os.Stat(record); err == nil { cancel(); return }
      select { case <-ctx.Done(): return; case <-ticker.C: }
     }
    }()
   } else { close(done) }
   status, output := test(ctx, work, []testCommand{{"", []string{"."}}}, timeout, "off", false)
   cancel()
   <-done
   want := "LIVED"
   if mode != "success" { want = "TIMED OUT" }
   if status != want { t.Fatalf("want %s, got %s: %s", want, status, output) }
   data, err := os.ReadFile(record)
   if err != nil { t.Fatalf("test subprocess did not start: %v: %s", err, output) }
   paths := strings.Split(string(data), "\n")
   if len(paths) != 3 { t.Fatalf("bad temp record: %q", data) }
   for _, path := range paths {
    rel, err := filepath.Rel(work, path)
    if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
     t.Errorf("subprocess temp escaped runner directory: %s", path)
    }
    if _, err := os.Stat(path); !os.IsNotExist(err) { t.Errorf("subprocess temp survived: %s (%v)", path, err) }
   }
   for _, path := range []string{tmp, gotmp} {
    entries, err := os.ReadDir(path)
    if err != nil || len(entries) != 0 { t.Errorf("caller temp changed: %s: %v %v", path, entries, err) }
   }
   if body, err := os.ReadFile(sentinel); err != nil || string(body) != "keep" { t.Fatalf("outside sentinel changed: %q %v", body, err) }
   entries, err := os.ReadDir(work)
   if err != nil || len(entries) != 2 { t.Errorf("runner left temporary files: %v %v", entries, err) }
  })
 }
}
`
