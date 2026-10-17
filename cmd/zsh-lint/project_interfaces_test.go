package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunProjectInterfaces(t *testing.T) {
	complete := t.TempDir()
	incomplete := t.TempDir()
	for _, root := range []string{complete, incomplete} {
		writeFile(t, filepath.Join(root, "zsh-lint.json"), cliConfig)
	}
	entry := filepath.Join(complete, "entry.zsh")
	unload := filepath.Join(complete, "functions", "example_plugin_unload")
	other := filepath.Join(incomplete, "entry.zsh")
	writeFile(t, entry, "precmd() { :; }\nTRAPINT() { :; }\nzle-line-init() { :; }\nprompt_demo_setup() { :; }\ntypeset -p Plugins\nadd-zsh-hook precmd _example_tick\n")
	writeFile(t, unload, "builtin emulate -L zsh\nunfunction example_plugin_unload\n")
	writeFile(t, other, "add-zsh-hook precmd _example_tick\n")
	for _, test := range []struct {
		name          string
		inputs        []string
		wantLifecycle int
	}{
		{"supplied autoload source", []string{entry, unload}, 0},
		{"omitted autoload source", []string{entry}, 1},
		{"other project remains missing", []string{entry, unload, other}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if status := run(test.inputs, &stdout, &stderr); status != 0 || stderr.Len() != 0 {
				t.Fatalf("run = %d, stderr %q, stdout %q", status, stderr.String(), stdout.String())
			}
			if got := strings.Count(stdout.String(), "[plugin/project-unload-lifecycle]"); got != test.wantLifecycle {
				t.Fatalf("lifecycle count = %d, want %d: %s", got, test.wantLifecycle, stdout.String())
			}
			for _, id := range []string{"plugin/function-namespace", "plugin/shared-plugins-registry"} {
				if strings.Contains(stdout.String(), "["+id+"]") {
					t.Fatalf("unexpected %s finding: %s", id, stdout.String())
				}
			}
			if test.name == "other project remains missing" && !strings.Contains(stdout.String(), other+":1:") {
				t.Fatalf("finding not attributed to other project: %s", stdout.String())
			}
		})
	}
}
