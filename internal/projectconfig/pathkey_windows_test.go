//go:build windows

package projectconfig

import "testing"

func TestPathKeyFoldsWindowsPathCase(t *testing.T) {
	if got, want := PathKey(`C:\\Proj\\zsh-lint.json`), PathKey(`c:\\proj\\zsh-lint.json`); got != want {
		t.Fatalf("PathKey() = %q and %q, want equal keys", got, want)
	}
}
