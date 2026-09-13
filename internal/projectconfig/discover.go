package projectconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfigName = "zsh-lint.json"

// Discover returns the nearest ancestor project configuration for filename.
// Discovery starts in the input file's directory and walks toward the
// filesystem root. When no configuration is present, Discover returns an empty
// path and a nil error.
func Discover(filename string) (string, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	dir := filepath.Clean(abs)
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			dir = filepath.Dir(dir)
		}
	} else {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	for {
		candidate := filepath.Join(dir, defaultConfigName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil:
			if !info.IsDir() {
				return candidate, nil
			}
		case !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("inspect configuration %q: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
