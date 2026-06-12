package diag

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSONEnvelope(t *testing.T) {
	diags := Diagnostics{
		{
			RuleID:   "quoting/unquoted-var",
			Severity: Warning,
			Message:  "Unquoted variable expansion",
			File:     "a.zsh",
			Range: Range{
				Start: Position{Line: 3, Column: 7, Offset: 42},
				End:   Position{Line: 3, Column: 12, Offset: 47},
			},
		},
		{
			RuleID:   "parse/error",
			Severity: Error,
			Message:  "parameter expansion flags are a zsh feature",
			File:     "b.zsh",
		},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, 2, diags); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Version     int `json:"version"`
		Diagnostics []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			File     string `json:"file"`
			Range    *struct {
				Start struct {
					Line   int `json:"line"`
					Column int `json:"column"`
					Offset int `json:"offset"`
				} `json:"start"`
				End struct {
					Line   int `json:"line"`
					Column int `json:"column"`
					Offset int `json:"offset"`
				} `json:"end"`
			} `json:"range"`
		} `json:"diagnostics"`
		Summary struct {
			Files       int `json:"files"`
			Diagnostics int `json:"diagnostics"`
			Errors      int `json:"errors"`
			Warnings    int `json:"warnings"`
			Infos       int `json:"infos"`
			Hints       int `json:"hints"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if len(got.Diagnostics) != 2 {
		t.Fatalf("diagnostics len = %d, want 2", len(got.Diagnostics))
	}

	d0 := got.Diagnostics[0]
	if d0.Rule != "quoting/unquoted-var" || d0.Severity != "warning" || d0.File != "a.zsh" {
		t.Errorf("diagnostic[0] = %+v", d0)
	}
	if d0.Range == nil {
		t.Fatal("diagnostic[0] should have a range")
	}
	if d0.Range.Start.Line != 3 || d0.Range.Start.Column != 7 || d0.Range.Start.Offset != 42 {
		t.Errorf("diagnostic[0].range.start = %+v", d0.Range.Start)
	}
	if d0.Range.End.Line != 3 || d0.Range.End.Column != 12 || d0.Range.End.Offset != 47 {
		t.Errorf("diagnostic[0].range.end = %+v", d0.Range.End)
	}

	d1 := got.Diagnostics[1]
	if d1.Rule != "parse/error" || d1.Severity != "error" {
		t.Errorf("diagnostic[1] = %+v", d1)
	}
	if d1.Range != nil {
		t.Errorf("diagnostic[1] is unpositioned; range must be omitted, got %+v", d1.Range)
	}

	s := got.Summary
	if s.Files != 2 || s.Diagnostics != 2 || s.Errors != 1 || s.Warnings != 1 || s.Infos != 0 || s.Hints != 0 {
		t.Errorf("summary = %+v", s)
	}

	// The envelope ends with a single trailing newline so it is line-friendly
	// in CI logs.
	if !strings.HasSuffix(buf.String(), "}\n") {
		t.Errorf("output must end with a single newline; got tail %q", buf.String()[len(buf.String())-3:])
	}
}

func TestWriteJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, 0, nil); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// diagnostics must be an empty array, not null, so consumers can iterate
	// without a nil check.
	if diags, ok := got["diagnostics"].([]any); !ok || len(diags) != 0 {
		t.Errorf("diagnostics = %#v, want empty array", got["diagnostics"])
	}
}
