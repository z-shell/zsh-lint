package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Native callback spellings are interfaces, not arbitrary plugin namespaces:
// https://zsh.sourceforge.io/Doc/Release/Functions.html#Special-Functions,
// https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html#Special-Widgets,
// https://zsh.sourceforge.io/Doc/Release/User-Contributions.html#Writing-Themes.
func TestProjectInterfacesNative(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for native project interface controls")
	}
	dir := t.TempDir()
	body := "builtin emulate -L zsh\nprint -r -- unload-called\nunfunction example_plugin_unload\n"
	if err := os.WriteFile(filepath.Join(dir, "example_plugin_unload"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt_demo_setup"), []byte("print -r -- prompt-called\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const driver = `builtin emulate -R zsh
chpwd() { print -r -- hook-called; }
cd -- "$1"
unfunction chpwd
TRAPUSR1() { print -r -- trap-called; }
kill -s USR1 $$
unfunction TRAPUSR1
zmodload zsh/parameter
zle-line-init() { :; }
zle -N zle-line-init
[[ $widgets[zle-line-init] == user:zle-line-init ]] || exit 41
print -r -- widget-registered
zle -D zle-line-init
unfunction zle-line-init
fpath=( "$1" $fpath )
autoload -Uz example_plugin_unload
example_plugin_unload
(( ! ${+functions[example_plugin_unload]} )) || exit 42
typeset -gA Plugins=( item original )
inspect() { typeset -p Plugins >/dev/null; Plugins[item]=changed; }
inspect
[[ $Plugins[item] == changed ]] || exit 43
print -r -- print-does-not-localize
inspect_global() { typeset -gp Plugins >/dev/null; }
inspect_global
[[ $Plugins[item] == changed ]] || exit 44
print -r -- print-does-not-mutate
autoload -Uz promptinit
promptinit
prompt demo
`
	path := filepath.Join(dir, "driver.zsh")
	if err := os.WriteFile(path, []byte(driver), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zsh, "-f", path, dir)
	cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir)
	output, err := cmd.CombinedOutput()
	const want = "hook-called\ntrap-called\nwidget-registered\nunload-called\nprint-does-not-localize\nprint-does-not-mutate\nprompt-called\n"
	if err != nil || string(output) != want {
		t.Fatalf("native project interfaces = %q (%v), want %q", output, err, want)
	}
}
