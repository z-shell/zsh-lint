package diag

import (
	"encoding/json"
	"io"
)

// JSONVersion is the version of the machine-readable output contract emitted
// by WriteJSON. The contract is documented in docs/project/output-contract.md
// (issue #20); any breaking change to the envelope or field semantics must
// increment it.
const JSONVersion = 1

type jsonPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

type jsonRange struct {
	Start jsonPosition `json:"start"`
	End   jsonPosition `json:"end"`
}

type jsonDiagnostic struct {
	Rule     string     `json:"rule"`
	Severity string     `json:"severity"`
	Message  string     `json:"message"`
	File     string     `json:"file,omitempty"`
	Range    *jsonRange `json:"range,omitempty"`
}

type jsonSummary struct {
	Files       int `json:"files"`
	Diagnostics int `json:"diagnostics"`
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
	Infos       int `json:"infos"`
	Hints       int `json:"hints"`
}

type jsonEnvelope struct {
	Version     int              `json:"version"`
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
	Summary     jsonSummary      `json:"summary"`
}

// WriteJSON renders diagnostics as the versioned machine-readable envelope
// defined by the output contract (issue #20) and writes it to w followed by a
// single newline. files is the number of files inspected (reported in the
// summary; it is independent of how many files produced findings).
//
// Diagnostics are emitted in the given order; callers that need deterministic
// output must call Diagnostics.Sort first. An unpositioned diagnostic (zero
// Range) omits the "range" member entirely.
func WriteJSON(w io.Writer, files int, diags Diagnostics) error {
	env := jsonEnvelope{
		Version:     JSONVersion,
		Diagnostics: make([]jsonDiagnostic, 0, len(diags)),
		Summary:     jsonSummary{Files: files, Diagnostics: len(diags)},
	}
	for _, d := range diags {
		jd := jsonDiagnostic{
			Rule:     string(d.RuleID),
			Severity: d.Severity.String(),
			Message:  d.Message,
			File:     d.File,
		}
		if d.Range.IsValid() {
			jd.Range = &jsonRange{
				Start: jsonPosition(d.Range.Start),
				End:   jsonPosition(d.Range.End),
			}
		}
		env.Diagnostics = append(env.Diagnostics, jd)
		switch d.Severity {
		case Error:
			env.Summary.Errors++
		case Warning:
			env.Summary.Warnings++
		case Info:
			env.Summary.Infos++
		case Hint:
			env.Summary.Hints++
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}
