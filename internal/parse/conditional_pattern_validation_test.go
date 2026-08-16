package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Native Zsh 5.9.2 rejects this unmatched nested pattern group. This test
// catches accepting the AST engine's more permissive success result.
func TestParseRejectsUnmatchedNestedConditionalPatternGroup(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-120-unmatched-conditional-pattern.txt")
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-120-unmatched-conditional-pattern.zsh")
	if err == nil {
		t.Fatal("Parse() accepted a native-invalid unmatched nested pattern group")
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	wantOffset := strings.Index(string(src), "((a|b)")
	if got := int(parseErr.Pos.Offset()); got != wantOffset {
		t.Errorf("error offset = %d, want %d", got, wantOffset)
	}
	if parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != 13 {
		t.Errorf("error position = %d:%d, want 2:13", parseErr.Pos.Line(), parseErr.Pos.Col())
	}
	if parseErr.Text != "unmatched `(` in conditional pattern" {
		t.Errorf("error text = %q", parseErr.Text)
	}
}

// These controls catch treating inactive or balanced parentheses as unmatched
// pattern groups. Native Zsh 5.9.2 accepts every source in this table.
func TestParseConditionalPatternValidationControls(t *testing.T) {
	tests := []string{
		"[[ $line == ((a|b)|c) ]]\n",
		"[[ $line == \"((a|b)\" ]]\n",
		"[[ $line == \\(\\(a\\|b\\) ]]\n",
		"[[ $line == $((1 + 2)) ]]\n",
		"[[ $line == $(print '((a|b)') ]]\n",
		"[[ $line == <(print '((a|b)') ]]\n",
		"[[ $line == (foo`print x`) ]]\n",
		"[[ $line == <1-9> ]]\n",
		"# [[ $line == ((a|b) ]]\nprint ok\n",
		"cat <<'EOF'\n[[ $line == ((a|b) ]]\nEOF\n",
		"print `[[ $line == ((a|b)|c) ]]`\n",
	}
	for _, src := range tests {
		if _, err := Parse(strings.NewReader(src), "valid-control.zsh"); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
}

func TestScanConditionalPatternsReportsFirstUnmatchedGroup(t *testing.T) {
	const src = "print 'λ'\n[[ $line == ((a|b) ]]\n"
	scan, ok := scanConditionalPatterns([]byte(src), -1, nil)
	if !ok || scan.finding == nil {
		t.Fatalf("scan = (%+v, %t), want finding", scan, ok)
	}
	want := strings.Index(src, "((a|b)")
	if scan.finding.offset != want {
		t.Errorf("finding offset = %d, want %d", scan.finding.offset, want)
	}
}

func TestScanConditionalPatternsKeepsBalancedCompatibilityCandidate(t *testing.T) {
	const src = "[[ $line == ((a|b)|c) ]]\n"
	_, firstErr := parseTree([]byte(src), "balanced.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		t.Fatalf("parseTree() error = %v, want syntax.ParseError", firstErr)
	}
	scan, ok := scanConditionalPatterns([]byte(src), int(parseErr.Pos.Offset()), nil)
	if !ok {
		t.Fatal("scanConditionalPatterns() rejected balanced compatibility source")
	}
	if scan.finding != nil {
		t.Fatalf("finding = %+v, want nil", scan.finding)
	}
	if len(scan.candidates) != 1 || len(scan.candidates[0].edits) == 0 {
		t.Fatalf("candidates = %+v, want one compatibility candidate", scan.candidates)
	}
}

func TestSourcePosCountsByteColumns(t *testing.T) {
	src := []byte("λ(")
	pos, ok := sourcePos(src, 2)
	if !ok {
		t.Fatal("sourcePos() rejected a valid UTF-8 byte offset")
	}
	if pos.Offset() != 2 || pos.Line() != 1 || pos.Col() != 3 {
		t.Errorf("sourcePos() = offset %d line %d column %d, want 2/1/3", pos.Offset(), pos.Line(), pos.Col())
	}
}

func FuzzValidateConditionalPatterns(f *testing.F) {
	f.Add("[[ $line == ((a|b) ]]\n")
	f.Add("[[ $line == ((a|b)|c) ]]\n")
	f.Add("cat <<'EOF'\n((\nEOF\n")
	f.Fuzz(func(t *testing.T, src string) {
		err := validateConditionalPatterns([]byte(src), "fuzz.zsh")
		if err == nil {
			return
		}
		var parseErr syntax.ParseError
		if errors.As(err, &parseErr) && int(parseErr.Pos.Offset()) > len(src) {
			t.Fatalf("error offset %d exceeds source length %d", parseErr.Pos.Offset(), len(src))
		}
	})
}
