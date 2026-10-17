package rules

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Explicit splitting uses its separator rather than inherited IFS:
// https://zsh.sourceforge.io/Doc/Release/Expansion.html#Parameter-Expansion-Flags.
// This runtime evidence does not make dynamic arguments statically accepted.
func TestFunctionScopedOptionsNativeSafeInitializers(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for native safe-initializer controls")
	}
	for _, init := range []string{
		"builtin emulate -L zsh",
		"builtin emulate -L zsh ${(s: :)${options[xtrace]:#off}:+-o xtrace}",
	} {
		for _, ifs := range []string{"", ":", "x", " \\t\\n"} {
			for _, trace := range []string{"on", "off"} {
				t.Run(init+ifs+trace, func(t *testing.T) {
					dir := t.TempDir()
					body := init + "\nprint -r -- inside:${options[localoptions]}:${options[shwordsplit]}:${options[xtrace]}\nsetopt extended_glob\n"
					if err := os.WriteFile(filepath.Join(dir, "handler"), []byte(body), 0o600); err != nil {
						t.Fatal(err)
					}
					driver := "fpath=( \"$1\" )\nautoload -Uz handler\nunsetopt local_options extended_glob\nsetopt sh_word_split\nIFS=$'" + ifs + "'\n"
					if trace == "on" {
						driver += "setopt xtrace\n"
					} else {
						driver += "unsetopt xtrace\n"
					}
					// Exercise both initial autoload and already-loaded execution.
					driver += "handler\nhandler\nprint -r -- after:${options[localoptions]}:${options[shwordsplit]}:${options[extendedglob]}:${options[xtrace]}\n"
					path := filepath.Join(dir, "driver.zsh")
					if err := os.WriteFile(path, []byte(driver), 0o600); err != nil {
						t.Fatal(err)
					}
					cmd := exec.Command(zsh, "-f", path, dir)
					cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir)
					var stderr bytes.Buffer
					cmd.Stderr = &stderr
					output, err := cmd.Output()
					inside := "inside:on:off:" + trace + "\n"
					want := inside + inside + "after:off:on:off:" + trace + "\n"
					if err != nil || string(output) != want || strings.Contains(stderr.String(), "no such option:") {
						t.Fatalf("native = %q (%v), stderr %q; want %q", output, err, stderr.String(), want)
					}
				})
			}
		}
	}
}
