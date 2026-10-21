package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// A directive after the invocation arguments remains a real comment and
// suppresses a finding on that same line, not the next line (#279).
func TestAnonymousInvocationTailSuppression(t *testing.T) {
	for _, prefix := range []string{"", "repeat 3 ", "for i in a; ", "if [[ x ]] ", "while [[ x ]] "} {
		for _, directive := range []string{"", " # zsh-lint disable=security/eval"} {
			file := filepath.Join(t.TempDir(), "tail.zsh")
			writeFile(t, file, prefix+"() { eval $code; } argument"+directive+"\neval $next\n")
			var stdout, stderr bytes.Buffer
			if exit := run([]string{"--no-config", file}, &stdout, &stderr); exit != 0 || stderr.Len() != 0 {
				t.Fatalf("%q %q: exit %d, stderr %s, stdout %s", prefix, directive, exit, stderr.String(), stdout.String())
			}
			want := 2
			if directive != "" {
				want = 1
			}
			if got := strings.Count(stdout.String(), "[security/eval]"); got != want {
				t.Fatalf("%q %q: got %d eval findings, want %d: %s", prefix, directive, got, want, stdout.String())
			}
			if !strings.Contains(stdout.String(), file+":2:1:") {
				t.Fatalf("next line finding lost or moved: %s", stdout.String())
			}
		}
	}
}
