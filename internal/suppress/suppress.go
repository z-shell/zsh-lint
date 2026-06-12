// Package suppress implements the inline suppression contract defined in
// docs/project/suppression.md (#19, implemented under #65).
//
// A suppression is a comment directive of the form
//
//	# zsh-lint disable=<rule-id>[,<rule-id>...] [-- reason]
//
// with two line-based scopes: trailing (same line as code) and preceding
// (next non-comment, non-blank source line). Malformed directives are
// reported as meta/malformed-suppression and suppress nothing; listed rule
// IDs that match no finding in scope are reported per ID as
// meta/unused-suppression. meta/* findings are never suppressible, so
// machine output always carries them (docs/project/output-contract.md).
package suppress

import (
	"fmt"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

// Reserved meta diagnostic IDs (docs/project/suppression.md).
const (
	RuleMalformed diag.RuleID = "meta/malformed-suppression"
	RuleUnused    diag.RuleID = "meta/unused-suppression"
)

// keyword is the directive marker, matched case-sensitively as a whole
// token so prose like "zsh-lint-survey" never parses as a directive.
const keyword = "zsh-lint"

// slugRe is the rule-policy ID shape: category/rule-name kebab slugs.
var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*/[a-z0-9]+(-[a-z0-9]+)*$`)

// Directive is one parsed zsh-lint comment directive.
type Directive struct {
	// Rules lists the rule IDs the directive disables; empty when Malformed.
	Rules []diag.RuleID
	// Target is the 1-based line findings must start on to be suppressed;
	// 0 means the directive has no scope (nothing follows it).
	Target int
	// Range spans the source comment, for positioning meta findings.
	Range diag.Range
	// Malformed holds the parse-failure reason; empty means well-formed.
	Malformed string
}

// Collect walks the parsed file's comments and returns every zsh-lint
// directive, well-formed or not. Non-directive comments are ignored.
func Collect(f *parse.File) []Directive {
	lines := f.Lines()
	var ds []Directive
	syntax.Walk(f.AST(), func(n syntax.Node) bool {
		c, ok := n.(*syntax.Comment)
		if !ok {
			return true
		}
		if d, ok := parseDirective(c, lines); ok {
			ds = append(ds, d)
		}
		return true
	})
	return ds
}

// Apply filters findings through the directives: a finding is dropped when a
// well-formed directive lists its rule ID and targets the line its range
// starts on. meta/* findings are never dropped. Apply appends one
// meta/malformed-suppression finding per malformed directive and one
// meta/unused-suppression finding per listed rule ID that suppressed
// nothing; known holds the active rule set so unknown IDs are called out.
// The result is unsorted; callers own ordering.
func Apply(ds []Directive, findings diag.Diagnostics, known map[diag.RuleID]bool, path string) diag.Diagnostics {
	type key struct {
		line int
		id   diag.RuleID
	}
	type slot struct{ d, r int }
	active := make(map[key][]slot)
	for di, d := range ds {
		if d.Malformed != "" || d.Target == 0 {
			continue
		}
		for ri, id := range d.Rules {
			k := key{d.Target, id}
			active[k] = append(active[k], slot{di, ri})
		}
	}

	used := make(map[slot]bool)
	kept := make(diag.Diagnostics, 0, len(findings))
	for _, f := range findings {
		if strings.HasPrefix(string(f.RuleID), "meta/") {
			kept = append(kept, f)
			continue
		}
		if slots, ok := active[key{f.Range.Start.Line, f.RuleID}]; ok {
			for _, s := range slots {
				used[s] = true
			}
			continue
		}
		kept = append(kept, f)
	}

	for di, d := range ds {
		if d.Malformed != "" {
			kept = append(kept, diag.Diagnostic{
				RuleID:   RuleMalformed,
				Severity: diag.Warning,
				Message:  fmt.Sprintf("malformed zsh-lint directive: %s; the directive suppresses nothing", d.Malformed),
				File:     path,
				Range:    d.Range,
			})
			continue
		}
		for ri, id := range d.Rules {
			if used[slot{di, ri}] {
				continue
			}
			msg := fmt.Sprintf("suppression for %s matched no finding in its scope", id)
			if !known[id] {
				msg = fmt.Sprintf("suppression for unknown rule ID %s (not in the current rule set) matched no finding in its scope", id)
			}
			kept = append(kept, diag.Diagnostic{
				RuleID:   RuleUnused,
				Severity: diag.Info,
				Message:  msg,
				File:     path,
				Range:    d.Range,
			})
		}
	}
	return kept
}

// parseDirective reports whether the comment is a zsh-lint directive and, if
// so, parses it. The keyword must be the comment's first whitespace token.
func parseDirective(c *syntax.Comment, lines []string) (Directive, bool) {
	fields := strings.Fields(c.Text)
	if len(fields) == 0 || fields[0] != keyword {
		return Directive{}, false
	}
	d := Directive{
		Range:  commentRange(c),
		Target: targetLine(int(c.Hash.Line()), int(c.Hash.Col()), lines),
	}
	rest := fields[1:]
	if len(rest) == 0 {
		d.Malformed = `missing "disable=" verb`
		return d, true
	}
	verb := rest[0]
	if !strings.HasPrefix(verb, "disable=") {
		name := verb
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		d.Malformed = fmt.Sprintf("unknown verb %q", name)
		return d, true
	}
	if len(rest) > 1 && rest[1] != "--" {
		if strings.HasSuffix(verb, ",") {
			d.Malformed = "rule list must be comma-separated without spaces"
		} else {
			d.Malformed = `unexpected text after the rule list (use " -- " to add a reason)`
		}
		return d, true
	}
	list := strings.TrimPrefix(verb, "disable=")
	if list == "" {
		d.Malformed = "empty rule list"
		return d, true
	}
	for _, id := range strings.Split(list, ",") {
		if !slugRe.MatchString(id) {
			d.Malformed = fmt.Sprintf("invalid rule ID %q", id)
			d.Rules = nil
			return d, true
		}
		d.Rules = append(d.Rules, diag.RuleID(id))
	}
	return d, true
}

// targetLine resolves the directive's scope per the contract: trailing when
// code precedes the comment on its own line, otherwise the next line that is
// neither blank nor comment-only. Returns 0 when no such line exists.
// col is the comment's 1-based start column; for the trailing check only the
// text before it matters, so any multi-byte drift upstream of the comment is
// harmless (the prefix still contains the same non-whitespace content).
func targetLine(line, col int, lines []string) int {
	if line >= 1 && line <= len(lines) {
		prefix := lines[line-1]
		if col-1 >= 0 && col-1 <= len(prefix) {
			prefix = prefix[:col-1]
		}
		if strings.TrimSpace(prefix) != "" {
			return line
		}
	}
	for i := line; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return i + 1
	}
	return 0
}

// commentRange converts the comment's parser span to a diagnostic range.
func commentRange(c *syntax.Comment) diag.Range {
	start, end := c.Hash, c.End()
	return diag.Range{
		Start: diag.Position{Line: int(start.Line()), Column: int(start.Col()), Offset: int(start.Offset())},
		End:   diag.Position{Line: int(end.Line()), Column: int(end.Col()), Offset: int(end.Offset())},
	}
}
