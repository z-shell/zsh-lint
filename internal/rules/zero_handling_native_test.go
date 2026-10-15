package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestZeroHandlingNativeSourceName(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the native source-name control")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "example.plugin.zsh")
	if err := os.WriteFile(path, []byte("print -r -- \"$0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("ZDOTDIR", dir)
	// FUNCTION_ARGZERO is sampled on entry, not when options change within
	// the file. Both outcomes are valid native executions of the same source.
	for _, test := range []struct {
		name   string
		option string
		want   string
	}{
		{name: "native default", want: path},
		{name: "caller disables function argzero", option: "--no-functionargzero", want: "caller"},
		{name: "caller enables posix argzero", option: "--posixargzero", want: "caller"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"-f"}
			if test.option != "" {
				args = append(args, test.option)
			}
			args = append(args, "-c", "source \"$1\"", "caller", path)
			output, err := exec.Command(zsh, args...).CombinedOutput()
			if err != nil || string(output) != test.want+"\n" {
				t.Fatalf("native source name = %q, err %v; want %q", output, err, test.want+"\n")
			}
		})
	}
}
