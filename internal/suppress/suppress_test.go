package suppress

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func mustParse(t *testing.T, src string) *parse.File {
	t.Helper()
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func TestCollectDirectives(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLen   int
		wantRules []diag.RuleID
		wantTgt   int
		wantBad   string // substring of Malformed; "" = well-formed
	}{
		{
			name:      "trailing single ID",
			src:       "eval $x # zsh-lint disable=security/eval\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"security/eval"},
			wantTgt:   1,
		},
		{
			name:      "preceding multi ID with reason",
			src:       "# zsh-lint disable=security/eval,quoting/unquoted-var -- table is static\neval $x\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"security/eval", "quoting/unquoted-var"},
			wantTgt:   2,
		},
		{
			name:      "no space after hash",
			src:       "#zsh-lint disable=style/backquotes\nfoo=`date`\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"style/backquotes"},
			wantTgt:   2,
		},
		{
			name:    "regular comment ignored",
			src:     "# regular comment about zsh\nprint ok\n",
			wantLen: 0,
		},
		{
			name:    "prose starting with product name variant ignored",
			src:     "# zsh-lint-survey output below\nprint ok\n",
			wantLen: 0,
		},
		{
			name:    "missing verb",
			src:     "# zsh-lint\nprint ok\n",
			wantLen: 1,
			wantBad: "missing",
			wantTgt: 2,
		},
		{
			name:    "unknown verb",
			src:     "# zsh-lint enable=security/eval\nprint ok\n",
			wantLen: 1,
			wantBad: `unknown verb "enable"`,
			wantTgt: 2,
		},
		{
			name:    "empty rule list",
			src:     "# zsh-lint disable=\nprint ok\n",
			wantLen: 1,
			wantBad: "empty rule list",
			wantTgt: 2,
		},
		{
			name:    "invalid rule ID",
			src:     "# zsh-lint disable=Security/Eval\nprint ok\n",
			wantLen: 1,
			wantBad: `invalid rule ID "Security/Eval"`,
			wantTgt: 2,
		},
		{
			name:    "space in rule list",
			src:     "# zsh-lint disable=security/eval, quoting/unquoted-var\nprint ok\n",
			wantLen: 1,
			wantBad: "without spaces",
			wantTgt: 2,
		},
		{
			name:    "junk after list",
			src:     "# zsh-lint disable=security/eval reason without separator\nprint ok\n",
			wantLen: 1,
			wantBad: "unexpected text",
			wantTgt: 2,
		},
		{
			name:      "preceding skips blank and comment lines",
			src:       "# zsh-lint disable=security/eval\n\n# explainer comment\neval $x\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"security/eval"},
			wantTgt:   4,
		},
		{
			name:      "comment alone inside multi-line construct is preceding",
			src:       "arr=(\n  one\n  # zsh-lint disable=quoting/unquoted-var\n  $two\n)\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"quoting/unquoted-var"},
			wantTgt:   4,
		},
		{
			name:      "directive with no following code has no target",
			src:       "print ok\n# zsh-lint disable=security/eval\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"security/eval"},
			wantTgt:   0,
		},
		{
			name:      "empty reason after separator is fine",
			src:       "eval $x # zsh-lint disable=security/eval --\n",
			wantLen:   1,
			wantRules: []diag.RuleID{"security/eval"},
			wantTgt:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := Collect(mustParse(t, tt.src))
			if len(ds) != tt.wantLen {
				t.Fatalf("Collect returned %d directives, want %d: %+v", len(ds), tt.wantLen, ds)
			}
			if tt.wantLen == 0 {
				return
			}
			d := ds[0]
			if tt.wantBad != "" {
				if d.Malformed == "" || !strings.Contains(d.Malformed, tt.wantBad) {
					t.Errorf("Malformed = %q, want substring %q", d.Malformed, tt.wantBad)
				}
				if len(d.Rules) != 0 {
					t.Errorf("malformed directive carries rules %v, want none", d.Rules)
				}
			} else {
				if d.Malformed != "" {
					t.Fatalf("unexpected Malformed: %q", d.Malformed)
				}
				if len(d.Rules) != len(tt.wantRules) {
					t.Fatalf("Rules = %v, want %v", d.Rules, tt.wantRules)
				}
				for i := range tt.wantRules {
					if d.Rules[i] != tt.wantRules[i] {
						t.Errorf("Rules[%d] = %q, want %q", i, d.Rules[i], tt.wantRules[i])
					}
				}
			}
			if d.Target != tt.wantTgt {
				t.Errorf("Target = %d, want %d", d.Target, tt.wantTgt)
			}
			if !d.Range.IsValid() {
				t.Error("directive Range is invalid, want the comment span")
			}
		})
	}
}

func finding(id diag.RuleID, line int) diag.Diagnostic {
	return diag.Diagnostic{
		RuleID:   id,
		Severity: diag.Warning,
		Message:  "x",
		File:     "test.zsh",
		Range: diag.Range{
			Start: diag.Position{Line: line, Column: 1, Offset: 0},
			End:   diag.Position{Line: line, Column: 2, Offset: 1},
		},
	}
}

func ids(ds diag.Diagnostics) []string {
	var out []string
	for _, d := range ds {
		out = append(out, string(d.RuleID))
	}
	return out
}

func TestApply(t *testing.T) {
	known := map[diag.RuleID]bool{
		"security/eval":        true,
		"quoting/unquoted-var": true,
	}
	wellFormed := func(target int, rules ...diag.RuleID) Directive {
		return Directive{
			Rules:  rules,
			Target: target,
			Range: diag.Range{
				Start: diag.Position{Line: target, Column: 10, Offset: 9},
				End:   diag.Position{Line: target, Column: 40, Offset: 39},
			},
		}
	}

	t.Run("suppresses only the listed rule on the target line", func(t *testing.T) {
		got := Apply(
			[]Directive{wellFormed(3, "security/eval")},
			diag.Diagnostics{finding("security/eval", 3), finding("quoting/unquoted-var", 3), finding("security/eval", 5)},
			known, "test.zsh",
		)
		want := []string{"quoting/unquoted-var", "security/eval"}
		if len(got) != 2 || got[0].RuleID != "quoting/unquoted-var" || got[1].RuleID != "security/eval" || got[1].Range.Start.Line != 5 {
			t.Fatalf("Apply = %v, want findings %v (other rule on line 3, eval on line 5)", ids(got), want)
		}
	})

	t.Run("per-ID staleness within one directive", func(t *testing.T) {
		got := Apply(
			[]Directive{wellFormed(3, "security/eval", "quoting/unquoted-var")},
			diag.Diagnostics{finding("security/eval", 3)},
			known, "test.zsh",
		)
		if len(got) != 1 || got[0].RuleID != "meta/unused-suppression" {
			t.Fatalf("Apply = %v, want exactly one meta/unused-suppression", ids(got))
		}
		if got[0].Severity != diag.Info {
			t.Errorf("unused severity = %v, want Info", got[0].Severity)
		}
		if !strings.Contains(got[0].Message, "quoting/unquoted-var") {
			t.Errorf("unused message %q does not name the stale ID", got[0].Message)
		}
		if strings.Contains(got[0].Message, "unknown") {
			t.Errorf("known rule ID flagged as unknown: %q", got[0].Message)
		}
	})

	t.Run("unknown rule ID is called out", func(t *testing.T) {
		got := Apply(
			[]Directive{wellFormed(3, "style/no-such-rule")},
			nil, known, "test.zsh",
		)
		if len(got) != 1 || got[0].RuleID != "meta/unused-suppression" {
			t.Fatalf("Apply = %v, want one meta/unused-suppression", ids(got))
		}
		if !strings.Contains(got[0].Message, "unknown rule ID") || !strings.Contains(got[0].Message, "style/no-such-rule") {
			t.Errorf("message %q should call out the unknown rule ID", got[0].Message)
		}
	})

	t.Run("malformed directive reports and suppresses nothing", func(t *testing.T) {
		bad := Directive{Malformed: `unknown verb "enable"`, Target: 3,
			Range: diag.Range{Start: diag.Position{Line: 2, Column: 1, Offset: 0}, End: diag.Position{Line: 2, Column: 30, Offset: 29}}}
		got := Apply([]Directive{bad}, diag.Diagnostics{finding("security/eval", 3)}, known, "test.zsh")
		if len(got) != 2 {
			t.Fatalf("Apply = %v, want the original finding plus meta/malformed-suppression", ids(got))
		}
		var meta *diag.Diagnostic
		for i := range got {
			if got[i].RuleID == "meta/malformed-suppression" {
				meta = &got[i]
			}
		}
		if meta == nil {
			t.Fatalf("no meta/malformed-suppression in %v", ids(got))
		}
		if meta.Severity != diag.Warning {
			t.Errorf("malformed severity = %v, want Warning", meta.Severity)
		}
		if !strings.Contains(meta.Message, "enable") {
			t.Errorf("malformed message %q should include the parse reason", meta.Message)
		}
	})

	t.Run("meta findings are never suppressed", func(t *testing.T) {
		got := Apply(
			[]Directive{wellFormed(3, "meta/unused-suppression")},
			diag.Diagnostics{finding("meta/unused-suppression", 3)},
			known, "test.zsh",
		)
		// The meta finding survives, and the directive that tried to silence
		// it is itself stale (its ID matched nothing suppressible).
		var kept, stale bool
		for _, d := range got {
			if d.RuleID == "meta/unused-suppression" && d.Range.Start.Line == 3 && d.Message == "x" {
				kept = true
			}
			if d.RuleID == "meta/unused-suppression" && d.Message != "x" {
				stale = true
			}
		}
		if !kept || !stale {
			t.Fatalf("Apply = %+v, want the original meta finding kept and the directive reported stale", got)
		}
	})

	t.Run("no-target directive reports all IDs unused", func(t *testing.T) {
		// A real no-target directive (nothing follows it in the file) still
		// has a valid comment range; the meta findings must carry it.
		d := Directive{
			Rules:  []diag.RuleID{"security/eval", "quoting/unquoted-var"},
			Target: 0,
			Range: diag.Range{
				Start: diag.Position{Line: 5, Column: 1, Offset: 50},
				End:   diag.Position{Line: 5, Column: 35, Offset: 84},
			},
		}
		got := Apply([]Directive{d}, nil, known, "test.zsh")
		if len(got) != 2 {
			t.Fatalf("Apply = %v, want two meta/unused-suppression findings", ids(got))
		}
		for _, m := range got {
			if m.Range.Start.Line != 5 {
				t.Errorf("meta finding positioned at line %d, want the directive's line 5", m.Range.Start.Line)
			}
		}
	})
}
