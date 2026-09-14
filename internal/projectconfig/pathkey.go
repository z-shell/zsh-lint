package projectconfig

import (
	"path/filepath"
	"strings"
)

// PathKey returns a stable key for grouping filesystem paths. Windows path
// spellings are compared case-insensitively so equivalent spellings do not
// split one configured project into separate analysis buckets.
func PathKey(path string) string {
	key := filepath.Clean(path)
	if filepath.Separator == '\\' {
		key = strings.ToLower(key)
	}
	return key
}
