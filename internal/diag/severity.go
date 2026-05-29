// Package diag defines the renderer-agnostic, parser-neutral diagnostics model
// shared by zsh-lint's analyzer: rule identity, severity, message, and source
// range. It depends on no concrete parser (preserving front-end swappability,
// issue #17) and performs no output formatting (see issue #20). Lint rules
// (#8) and inline suppressions (#19) build on these types.
package diag

import "fmt"

// Severity classifies how serious a diagnostic is. The ordering matches the LSP
// DiagnosticSeverity scale, from most to least severe.
type Severity int

const (
	Error Severity = iota
	Warning
	Info
	Hint
)

var severityNames = map[Severity]string{
	Error:   "error",
	Warning: "warning",
	Info:    "info",
	Hint:    "hint",
}

// String returns the canonical lowercase name of the severity. An out-of-range
// value renders as a stable "severity(N)" sentinel rather than panicking.
func (s Severity) String() string {
	if name, ok := severityNames[s]; ok {
		return name
	}
	return fmt.Sprintf("severity(%d)", int(s))
}

// ParseSeverity is the inverse of String. Input must be the canonical lowercase
// name; unknown input returns an error prefixed "diag:".
func ParseSeverity(s string) (Severity, error) {
	switch s {
	case "error":
		return Error, nil
	case "warning":
		return Warning, nil
	case "info":
		return Info, nil
	case "hint":
		return Hint, nil
	default:
		return 0, fmt.Errorf("diag: unknown severity %q", s)
	}
}
