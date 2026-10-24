package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Precommand Modifiers and the typeset builtin distinguish ordinary arguments
// from reserved-word assignments. These probes execute only minimized controls
// in an empty environment, never consumer source or inherited parameter data.
// https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Precommand-Modifiers
// https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-typeset
func TestDeclarationRuntimeCardinalityNative(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	for _, test := range []struct{ name, source, want string }{
		{"optional argument exposes global option", "typeset -a empty=(); Plugins=outer; f() { builtin typeset unused=${^empty} -g Plugins >/dev/null; Plugins=changed; }; f; print -r -- OBS:$Plugins\n", "OBS:changed\n"},
		{"expanding argument changes tie roles", "typeset -a values=(one array); Plugins=outer; f() { builtin typeset -T -- scalar=${values[@]} Plugins >/dev/null; Plugins=changed; }; f; print -r -- OBS:$Plugins\n", "OBS:changed\n"},
		{"expanding argument does not localize fpath", "typeset -a values=(one array); FPATH=/original; f() { builtin typeset -T -- scalar=${values[@]} FPATH >/dev/null; FPATH=/changed; }; f; print -r -- OBS:$FPATH\n", "OBS:/changed\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "probe.zsh")
			if err := os.WriteFile(file, []byte(test.source), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(zsh, "-f", file)
			command.Dir = dir
			command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "ZDOTDIR=" + dir, "TMPDIR=" + dir, "LC_ALL=C"}
			output, err := command.CombinedOutput()
			if err != nil || string(output) != test.want {
				t.Fatalf("output %q, error %v, want %q", output, err, test.want)
			}
		})
	}
}

func TestDeclarationCallsNative(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	for _, prefix := range []string{"", "builtin ", "noglob ", "- ", "nocorrect "} {
		t.Run(prefix, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "probe.zsh")
			src := "value=outer; example_f() { " + prefix + "local value=inner; print -r -- $value; }; example_f; print -r -- $value\n"
			if err := os.WriteFile(file, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(zsh, "-f", file)
			command.Dir = dir
			command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "ZDOTDIR=" + dir, "TMPDIR=" + dir, "LC_ALL=C"}
			output, err := command.CombinedOutput()
			if err != nil || string(output) != "inner\nouter\n" {
				t.Fatalf("local behavior: output %q, error %v", output, err)
			}
			src = "example_f() { " + prefix + "local -a items=(a b); print -r -- count:$#items; }; example_f\n"
			if err := os.WriteFile(file, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			command = exec.Command(zsh, "-f", file)
			command.Dir = dir
			command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "ZDOTDIR=" + dir, "TMPDIR=" + dir, "LC_ALL=C"}
			output, err = command.CombinedOutput()
			reserved := prefix == "" || prefix == "nocorrect "
			if reserved {
				if err != nil || string(output) != "count:2\n" {
					t.Fatalf("reserved array: output %q, error %v", output, err)
				}
			} else if err == nil || (!strings.Contains(string(output), "number expected") && !strings.Contains(string(output), "inconsistent type")) {
				t.Fatalf("ordinary array must fail: output %q, error %v", output, err)
			}
		})
	}
}
