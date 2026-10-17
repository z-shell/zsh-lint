package rules

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Autoload file bodies and same-name definitions both execute as functions:
// https://zsh.sourceforge.io/Doc/Release/Functions.html#Autoloading-Functions.
// In particular, sourcing these fixtures would test a different scope.
func TestFunctionScopedOptionsNativeAutoload(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for native autoload controls")
	}
	for _, test := range []struct {
		name, init     string
		wrapper, trace bool
		want           string
	}{
		{"bare emulate", "builtin emulate -L zsh", false, false, "on:off:on"},
		{"bare xtrace off", "builtin emulate -L zsh ${=${options[xtrace]:#off}:+-o xtrace}", false, false, "on:off:on"},
		{"bare xtrace on", "builtin emulate -L zsh ${=${options[xtrace]:#off}:+-o xtrace}", false, true, "on:off:on"},
		{"local flag overrides disabling", "builtin emulate -L zsh +o local_options", false, false, "on:off:on"},
		{"local flag overrides inverse option", "builtin emulate -L zsh -o no_local_options", false, false, "on:off:on"},
		{"wrapper emulate", "builtin emulate -L zsh", true, false, "on:off:on"},
		{"wrapper xtrace", "builtin emulate -LR zsh ${=${options[xtrace]:#off}:+-o xtrace}", true, true, "on:off:on"},
		{"bare unscoped", ":", false, false, "off:on:on"},
		{"wrapper unscoped", ":", true, false, "off:on:on"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			body := test.init + "\nsetopt extended_glob\nprint -r -- $options[localoptions]:$options[shwordsplit]:$options[extendedglob]:$options[xtrace]\n"
			if test.wrapper {
				body = "handler() {\n" + body + "}\n"
			}
			path := filepath.Join(dir, "handler")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			check := exec.Command(zsh, "-f", "-n", path)
			if output, err := check.CombinedOutput(); err != nil || len(output) != 0 {
				t.Fatalf("syntax: %s (%v)", output, err)
			}
			driver := "fpath=( \"$1\" )\nunsetopt local_options extended_glob\nsetopt sh_word_split\nautoload -Uz handler\n"
			if test.trace {
				driver += "setopt xtrace\n"
			}
			driver += "handler\nhandler\nprint -r -- after:$options[localoptions]:$options[shwordsplit]:$options[extendedglob]:$options[xtrace]\n"
			driverPath := filepath.Join(dir, "driver.zsh")
			if err := os.WriteFile(driverPath, []byte(driver), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(zsh, "-f", driverPath, dir)
			cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			after := "off:on:off"
			if test.init == ":" {
				after = "off:on:on"
			}
			trace := ":off"
			if test.trace {
				trace = ":on"
			}
			want := test.want + trace + "\n" + test.want + trace + "\nafter:" + after + trace + "\n"
			if err != nil || string(output) != want {
				t.Fatalf("autoload = %q (%v), stderr %q; want %q", output, err, stderr.String(), want)
			}
		})
	}
}

// The installed completion entry-point setup scopes the callback's inherited
// options. This exercises registration and that setup without requiring ZLE.
func TestFunctionScopedOptionsNativeCompletion(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for native completion controls")
	}
	dir := t.TempDir()
	callback := "#compdef scope-probe\nprint -r -- callback:$options[localoptions]:$options[shwordsplit]\nsetopt sh_word_split\n"
	if err := os.WriteFile(filepath.Join(dir, "_scope_probe"), []byte(callback), 0o600); err != nil {
		t.Fatal(err)
	}
	// With FPATH absent from the environment, native Zsh supplies its own
	// installation paths. Prepend the subject without trusting inherited paths.
	driver := `fpath=( "$1" $fpath )
autoload -Uz compinit
compinit -D -i
[[ $_comps[scope-probe] == _scope_probe ]] || exit 41
unsetopt sh_word_split local_options
entry() {
  eval "$_comp_setup"
  _scope_probe
}
entry
print -r -- after:$options[localoptions]:$options[shwordsplit]
`
	path := filepath.Join(dir, "driver.zsh")
	if err := os.WriteFile(path, []byte(driver), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zsh, "-f", path, dir)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "FPATH=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+dir, "ZDOTDIR="+dir)
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "callback:on:off\nafter:off:off\n" {
		t.Fatalf("completion setup = %q (%v)", output, err)
	}
}

// ${=spec} uses inherited IFS before emulate can reset option state. A
// successful default-IFS call is not proof of safe caller-independent scoping.
func TestFunctionScopedOptionsNativeInheritedIFS(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for native inherited-IFS controls")
	}
	const dynamic = "builtin emulate -L zsh ${=${options[xtrace]:#off}:+-o xtrace}"
	for _, test := range []struct {
		name, setup, init, want string
		findings                int
	}{
		{"default IFS", "", dynamic, "inside:on:off\nafter:off:on:off\n", 1},
		{"colon IFS", "IFS=':'\n", dynamic, "inside:off:on\nafter:off:on:on\n", 1},
		{"empty IFS", "IFS=''\n", dynamic, "inside:off:on\nafter:off:on:on\n", 1},
		{"static with colon IFS", "IFS=':'\n", "builtin emulate -L zsh", "inside:on:off\nafter:off:on:off\n", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			body := test.init + "\nprint -r -- inside:${options[localoptions]}:${options[shwordsplit]}\nsetopt extended_glob\n"
			path := filepath.Join(dir, "handler")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			check := exec.Command(zsh, "-f", "-n", path)
			if output, err := check.CombinedOutput(); err != nil || len(output) != 0 {
				t.Fatalf("syntax: %s (%v)", output, err)
			}
			driver := "fpath=( \"$1\" )\nautoload -Uz handler\nunsetopt local_options extended_glob\nsetopt sh_word_split xtrace\n" + test.setup + "handler\nprint -r -- after:${options[localoptions]}:${options[shwordsplit]}:${options[extendedglob]}\n"
			driverPath := filepath.Join(dir, "driver.zsh")
			if err := os.WriteFile(driverPath, []byte(driver), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(zsh, "-f", driverPath, dir)
			cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			if err != nil || string(output) != test.want {
				t.Fatalf("autoload = %q (%v), stderr %q; want %q", output, err, stderr.String(), test.want)
			}
			if strings.HasPrefix(test.want, "inside:off:") && !strings.Contains(stderr.String(), "no such option:") {
				t.Fatalf("missing native initialization failure: %q", stderr.String())
			}
			if got := analyzeFunctionScopedOptions(t, "functions/handler", body); len(got) != test.findings {
				t.Fatalf("lines = %v, want %d findings", got, test.findings)
			}
		})
	}
}
