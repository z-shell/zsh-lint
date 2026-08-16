package parse

import (
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func parsedPatternWords(t *testing.T, src string) []string {
	t.Helper()
	f, err := Parse(strings.NewReader(src), "nested-pattern.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	return patternWords(t, f.AST())
}

func patternWords(t *testing.T, tree *syntax.File) []string {
	t.Helper()
	var patterns []string
	syntax.Walk(tree, func(node syntax.Node) bool {
		binary, ok := node.(*syntax.BinaryTest)
		if !ok {
			return true
		}
		switch binary.Op {
		case syntax.TsMatchShort, syntax.TsMatch, syntax.TsNoMatch:
		default:
			return true
		}
		word, ok := binary.Y.(*syntax.Word)
		if !ok {
			t.Fatalf("pattern operand type = %T, want *syntax.Word", binary.Y)
		}
		patterns = append(patterns, word.Lit())
		return true
	})
	return patterns
}

func renderedPatternWords(t *testing.T, tree *syntax.File) []string {
	t.Helper()
	var patterns []string
	syntax.Walk(tree, func(node syntax.Node) bool {
		binary, ok := node.(*syntax.BinaryTest)
		if !ok {
			return true
		}
		switch binary.Op {
		case syntax.TsMatchShort, syntax.TsMatch, syntax.TsNoMatch:
		default:
			return true
		}
		word, ok := binary.Y.(*syntax.Word)
		if !ok {
			t.Fatalf("pattern operand type = %T, want *syntax.Word", binary.Y)
		}
		var rendered strings.Builder
		if err := syntax.NewPrinter().Print(&rendered, word); err != nil {
			t.Fatalf("print pattern operand: %v", err)
		}
		patterns = append(patterns, rendered.String())
		return true
	})
	return patterns
}

func TestParseNestedConditionalAlternationBatchesCompatibilityParse(t *testing.T) {
	const comparisonCount = 32
	const pattern = "((a|b)|c)"
	src := []byte(strings.Repeat(
		"print -r -- unrelated\n[[ $line == "+pattern+" ]]\n",
		comparisonCount,
	))

	_, firstErr := parseTree(src, "many-nested-patterns.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the original source")
	}

	fullFileParses := 0
	islandParses := 0
	tree, err := parseNestedConditionalAlternationWithParser(
		src,
		"many-nested-patterns.zsh",
		firstErr,
		func(masked []byte, name string) (*syntax.File, error) {
			if len(masked) == len(src) {
				fullFileParses++
			} else {
				islandParses++
			}
			return parseTree(masked, name)
		},
	)
	if err != nil {
		t.Fatalf("parseNestedConditionalAlternationWithParser() error: %v", err)
	}
	if fullFileParses != 1 {
		t.Errorf("full-file compatibility parse count = %d, want 1", fullFileParses)
	}
	if islandParses != 0 {
		t.Errorf("ordinary non-backtick island parse count = %d, want 0", islandParses)
	}

	patterns := patternWords(t, tree)
	if len(patterns) != comparisonCount {
		t.Fatalf("pattern count = %d, want %d", len(patterns), comparisonCount)
	}
	for i, got := range patterns {
		if got != pattern {
			t.Errorf("pattern[%d] = %q, want %q", i, got, pattern)
		}
	}
}

func TestNestedPatternBatchEditsUsesLinearCandidateInspections(t *testing.T) {
	collect := func(t *testing.T, pipelineCount int) (sourceBytes, inspections int) {
		t.Helper()
		src := []byte(
			"[[ $line == ((a|b)|c) ]]\n" +
				strings.Repeat("print left | print right\n", pipelineCount),
		)
		_, firstErr := parseTree(src, "batch-work-bound.zsh")
		var parseErr syntax.ParseError
		if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
			t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
		}
		edits, ok := nestedPatternBatchEdits(src, int(parseErr.Pos.Offset()), func() {
			inspections++
		})
		if !ok {
			t.Fatal("nestedPatternBatchEdits() rejected the recognized seed")
		}
		if len(edits) != 2 {
			t.Fatalf("edit count = %d, want 2: %+v", len(edits), edits)
		}
		return len(src), inspections
	}

	smallBytes, smallInspections := collect(t, 256)
	largeBytes, largeInspections := collect(t, 512)
	for _, result := range []struct {
		name        string
		sourceBytes int
		inspections int
	}{
		{name: "small", sourceBytes: smallBytes, inspections: smallInspections},
		{name: "large", sourceBytes: largeBytes, inspections: largeInspections},
	} {
		t.Run(result.name, func(t *testing.T) {
			// The production collector charges one observation per forward-scan
			// iteration and one per emitted candidate. Skipped token bytes only
			// reduce this upper bound.
			maxInspections := result.sourceBytes + 1
			if result.inspections == 0 {
				t.Fatal("candidate inspections = 0, want production observations")
			}
			if result.inspections > maxInspections {
				t.Errorf(
					"candidate inspections = %d, want at most %d for %d source bytes",
					result.inspections,
					maxInspections,
					result.sourceBytes,
				)
			}
		})
	}
	if largeInspections > 2*smallInspections {
		t.Errorf(
			"doubling pipeline input changed inspections from %d to %d, want at most %d",
			smallInspections,
			largeInspections,
			2*smallInspections,
		)
	}
}

func TestNestedPatternBatchUsesConstantLegacyDelimiterLookups(t *testing.T) {
	collect := func(t *testing.T, scale int) int {
		t.Helper()
		var src strings.Builder
		for range scale {
			src.WriteString("print $(")
		}
		for range scale {
			src.WriteString(" `[[ x == ((a|b)|c) ]]`")
		}
		for range scale {
			src.WriteByte(')')
		}
		src.WriteByte('\n')

		original := []byte(src.String())
		_, firstErr := parseTree(original, "legacy-delimiter-work.zsh")
		var parseErr syntax.ParseError
		if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
			t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
		}
		lookups := 0
		scan, ok := scanConditionalPatterns(
			original,
			int(parseErr.Pos.Offset()),
			nil,
			func() { lookups++ },
		)
		if !ok {
			t.Fatal("scanConditionalPatterns() rejected native-valid legacy substitutions")
		}
		if len(scan.candidates) != scale {
			t.Fatalf("candidate count = %d, want %d", len(scan.candidates), scale)
		}
		if len(scan.backtickIslands) != scale {
			t.Fatalf("legacy island count = %d, want %d", len(scan.backtickIslands), scale)
		}
		if lookups == 0 {
			t.Fatal("legacy delimiter lookups = 0, want production observations")
		}
		maxLookups := 2 * scale
		if lookups > maxLookups {
			t.Errorf(
				"legacy delimiter lookups = %d, want at most %d for depth and sibling count %d",
				lookups,
				maxLookups,
				scale,
			)
		}
		return lookups
	}

	smallLookups := collect(t, 256)
	largeLookups := collect(t, 512)
	t.Logf("legacy delimiter lookups at scale 256/512: %d/%d", smallLookups, largeLookups)
	if largeLookups > 2*smallLookups {
		t.Errorf(
			"doubling depth and sibling count changed legacy delimiter lookups from %d to %d, want at most %d",
			smallLookups,
			largeLookups,
			2*smallLookups,
		)
	}
}

func TestParseNestedConditionalAlternationRejectsActivePatternOperators(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "ampersand", src: "[[ $line == ((a&b)|c) ]]\n"},
		{name: "double ampersand", src: "[[ $line == ((a&&b)|c) ]]\n"},
		{name: "semicolon", src: "[[ $line == ((a;b)|c) ]]\n"},
		{name: "less than", src: "[[ $line == ((a<b)|c) ]]\n"},
		{name: "greater than", src: "[[ $line == ((a>b)|c) ]]\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(test.src), "active-pattern-operator.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			if _, err := Parse(strings.NewReader(test.src), "active-pattern-operator.zsh"); err == nil {
				t.Fatal("Parse() unexpectedly accepted native-invalid active pattern operator")
			}
		})
	}
}

func TestParseNestedConditionalAlternationAllowsNumericRangeAtoms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "bounded range",
			src:  "[[ x == ((a|<1-3>)|b) ]]\n",
			want: "((a|<1-3>)|b)",
		},
		{
			name: "omitted lower bound",
			src:  "[[ x == ((a|<-3>)|b) ]]\n",
			want: "((a|<-3>)|b)",
		},
		{
			name: "omitted upper bound",
			src:  "[[ x == ((a|<1->)|b) ]]\n",
			want: "((a|<1->)|b)",
		},
		{
			name: "both bounds omitted",
			src:  "[[ x == ((a|<->)|b) ]]\n",
			want: "((a|<->)|b)",
		},
		{
			name: "leading zero bounds",
			src:  "[[ x == ((a|<0001-0003>)|b) ]]\n",
			want: "((a|<0001-0003>)|b)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(test.src), "numeric-range-pattern.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			file, err := Parse(strings.NewReader(test.src), "numeric-range-pattern.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			patterns := patternWords(t, file.AST())
			if len(patterns) != 1 {
				t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
			}
			if patterns[0] != test.want {
				t.Errorf("pattern = %q, want %q", patterns[0], test.want)
			}
		})
	}
}

func TestParseNestedConditionalAlternationRejectsMalformedNumericRangeAtoms(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "missing closing angle", src: "[[ x == ((a|<1-3)|b) ]]\n"},
		{name: "missing opening angle", src: "[[ x == ((a|1-3>)|b) ]]\n"},
		{name: "missing separator", src: "[[ x == ((a|<1>)|b) ]]\n"},
		{name: "non-decimal lower bound", src: "[[ x == ((a|<a-3>)|b) ]]\n"},
		{name: "non-decimal upper bound", src: "[[ x == ((a|<1-b>)|b) ]]\n"},
		{name: "doubled separator", src: "[[ x == ((a|<1--3>)|b) ]]\n"},
		{name: "empty angle pair", src: "[[ x == ((a|<>)|b) ]]\n"},
		{name: "signed lower bound", src: "[[ x == ((a|<+1-3>)|b) ]]\n"},
		{name: "signed upper bound", src: "[[ x == ((a|<1-+3>)|b) ]]\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(test.src), "malformed-numeric-range-pattern.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			if _, err := Parse(strings.NewReader(test.src), "malformed-numeric-range-pattern.zsh"); err == nil {
				t.Fatal("Parse() unexpectedly accepted a native-invalid numeric range")
			}
		})
	}
}

func TestParseNestedConditionalAlternationAllowsQuotedOrEscapedOperatorBytes(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "escaped ampersand", src: "[[ $line == ((a\\&b)|c) ]]\n"},
		{name: "escaped less than", src: "[[ $line == ((a\\<b)|c) ]]\n"},
		{name: "quoted ampersand", src: "[[ $line == (('a&b'|d)|c) ]]\n"},
		{name: "quoted semicolon", src: "[[ $line == (('a;b'|d)|c) ]]\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(test.src), "quoted-pattern-operator.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			if _, err := Parse(strings.NewReader(test.src), "quoted-pattern-operator.zsh"); err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
		})
	}
}

func TestParseNestedConditionalAlternationUsesLexicalBoundaries(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "quoted conditional start lookalike",
			src:  "[[ \"[[ = fake\" == ((a|b)|c) ]]\n",
			want: "((a|b)|c)",
		},
		{
			name: "raw operator lookalike inside group",
			src:  "[[ $line == ((a = b)|c) ]]\n",
			want: "((a = b)|c)",
		},
		{
			name: "quoted operator lookalike inside pattern",
			src:  "[[ $line == (' = '((a|b)|c)) ]]\n",
			want: "(' = '((a|b)|c))",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(test.src), "lexical-boundary.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			file, err := Parse(strings.NewReader(test.src), "lexical-boundary.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			patterns := renderedPatternWords(t, file.AST())
			if len(patterns) != 1 {
				t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
			}
			if patterns[0] != test.want {
				t.Errorf("pattern = %q, want %q", patterns[0], test.want)
			}
		})
	}
}

func TestParseNestedConditionalAlternationInsideBacktickFrame(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "unquoted parent",
			src:  "print `[[ $line == ((a|b)|c) ]]`\n",
		},
		{
			name: "double quoted parent resumes",
			src:  "print \"`[[ $line == ((a|b)|c) ]]`\"\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, firstErr := parseTree([]byte(test.src), "backtick-frame.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			file, err := Parse(strings.NewReader(test.src), "backtick-frame.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			patterns := patternWords(t, file.AST())
			if len(patterns) != 1 {
				t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
			}
			if patterns[0] != "((a|b)|c)" {
				t.Errorf("pattern = %q, want %q", patterns[0], "((a|b)|c)")
			}
			wantBacktick := strings.LastIndexByte(test.src, '`')
			wantOpeningBacktick := strings.IndexByte(test.src, '`')
			backtickSubstitutions := 0
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				substitution, ok := node.(*syntax.CmdSubst)
				if !ok || !substitution.Backquotes {
					return true
				}
				backtickSubstitutions++
				if got := int(substitution.Left.Offset()); got != wantOpeningBacktick {
					t.Errorf("backtick open offset = %d, want %d", got, wantOpeningBacktick)
				}
				if got := int(substitution.Right.Offset()); got != wantBacktick {
					t.Errorf("backtick close offset = %d, want %d", got, wantBacktick)
				}
				return true
			})
			if backtickSubstitutions != 1 {
				t.Errorf("backtick substitution count = %d, want 1", backtickSubstitutions)
			}
			if test.name == "double quoted parent resumes" {
				wantOpeningQuote := strings.IndexByte(test.src, '"')
				wantQuote := strings.LastIndexByte(test.src, '"')
				doubleQuotes := 0
				syntax.Walk(file.AST(), func(node syntax.Node) bool {
					quoted, ok := node.(*syntax.DblQuoted)
					if !ok {
						return true
					}
					doubleQuotes++
					if got := int(quoted.Left.Offset()); got != wantOpeningQuote {
						t.Errorf("double-quote open offset = %d, want %d", got, wantOpeningQuote)
					}
					if got := int(quoted.Right.Offset()); got != wantQuote {
						t.Errorf("double-quote close offset = %d, want %d", got, wantQuote)
					}
					return true
				})
				if doubleQuotes != 1 {
					t.Errorf("double-quoted node count = %d, want 1", doubleQuotes)
				}
			}
		})
	}
}

func TestLowerBoundPatternEdit(t *testing.T) {
	const editCount = 65_536
	const maxProbes = 17
	edits := make([]indexedPatternEdit, editCount)
	for i := range edits {
		edits[i] = indexedPatternEdit{
			originalIndex: i,
			edit:          patternEdit{offset: i * 2},
		}
	}

	tests := []struct {
		name      string
		offset    int
		wantIndex int
	}{
		{name: "beginning", offset: 0, wantIndex: 0},
		{name: "middle", offset: editCount, wantIndex: editCount / 2},
		{name: "end", offset: (editCount - 1) * 2, wantIndex: editCount - 1},
		{name: "beyond final", offset: editCount * 2, wantIndex: editCount},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index, probes := lowerBoundPatternEdit(edits, test.offset)
			if index != test.wantIndex {
				t.Errorf("index = %d, want %d", index, test.wantIndex)
			}
			if probes > maxProbes {
				t.Errorf("probes = %d, want at most %d", probes, maxProbes)
			}
		})
	}
}

func TestRestorePatternEditsUsesBoundedCandidateInspections(t *testing.T) {
	const comparisonCount = 32
	const pattern = "((a|b)|c)"
	original := []byte(strings.Repeat(
		"print -r -- unrelated\n[[ $line == "+pattern+" ]]\n",
		comparisonCount,
	))
	_, firstErr := parseTree(original, "restore-probe.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
	}
	edits, ok := nestedPatternBatchEdits(original, int(parseErr.Pos.Offset()))
	if !ok {
		t.Fatal("nestedPatternBatchEdits() rejected the restoration probe source")
	}

	masked := append([]byte(nil), original...)
	for _, edit := range edits {
		masked[edit.offset] = edit.replacement
	}
	tree, err := parseTree(masked, "restore-probe.zsh")
	if err != nil {
		t.Fatalf("parseTree(masked) error: %v", err)
	}

	literalCount := 0
	syntax.Walk(tree, func(node syntax.Node) bool {
		if _, ok := node.(*syntax.Lit); ok {
			literalCount++
		}
		return true
	})
	candidateInspections := 0
	err = restorePatternEdits(tree, original, edits, func() {
		candidateInspections++
	})
	if err != nil {
		t.Fatalf("restorePatternEdits() error: %v", err)
	}

	maxSearchProbes := 0
	for width := len(edits); width > 0; width /= 2 {
		maxSearchProbes++
	}
	maxInspections := literalCount*maxSearchProbes + len(edits)
	if candidateInspections == 0 {
		t.Fatal("candidate inspections = 0, want production restoration observations")
	}
	if candidateInspections > maxInspections {
		t.Errorf(
			"candidate inspections = %d, want at most %d for %d literals and %d edits",
			candidateInspections,
			maxInspections,
			literalCount,
			len(edits),
		)
	}

	patterns := patternWords(t, tree)
	if len(patterns) != comparisonCount {
		t.Fatalf("pattern count = %d, want %d", len(patterns), comparisonCount)
	}
	for i, got := range patterns {
		if got != pattern {
			t.Errorf("pattern[%d] = %q, want %q", i, got, pattern)
		}
	}
}

func TestParseNestedConditionalAlternationWithShellDelimiters(t *testing.T) {
	const pattern = "((a|b)|c)"
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "here string before affected comparison",
			src:  "cat <<< \"$line\"\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "arithmetic shift before affected comparison",
			src:  "(( shifted = 8 << 1 ))\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "ANSI-C escaped apostrophe before affected comparison",
			src:  "print -r -- $'can\\'t'\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "quoted heredoc body lookalike before affected comparison",
			src:  "cat <<'DATA'\n[[ body == ((x|y)|z) ]]\nDATA\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "tab stripping heredoc body lookalike before affected comparison",
			src:  "cat <<-DATA\n\t[[ body == ((x|y)|z) ]]\n\tDATA\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "queued heredoc bodies before affected comparison",
			src:  "cat <<ONE <<'TWO'\n[[ first == ((x|y)|z) ]]\nONE\n[[ second == ((q|r)|s) ]]\nTWO\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "arithmetic expansion shift before affected comparison",
			src:  "print -r -- $(( 8 << 1 ))\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "ordinary single quote keeps backslash literal",
			src:  "print -r -- 'can\\'\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "double quoted heredoc delimiter",
			src:  "cat <<\"DATA\"\n[[ body == ((x|y)|z) ]]\nDATA\n[[ $line == ((a|b)|c) ]]\n",
		},
		{
			name: "backslash quoted heredoc delimiter",
			src:  "cat <<D\\ATA\n[[ body == ((x|y)|z) ]]\nDATA\n[[ $line == ((a|b)|c) ]]\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patterns := parsedPatternWords(t, test.src)
			if len(patterns) != 1 {
				t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
			}
			if patterns[0] != pattern {
				t.Errorf("pattern = %q, want %q", patterns[0], pattern)
			}
		})
	}
}

func TestParseNestedConditionalAlternationBeforeOuterHeredocBody(t *testing.T) {
	const pattern = "((a|b)|c)"
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "command substitution argument",
			src:  "cat <<EOF $(\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
		{
			name: "process substitution argument",
			src:  "cat <<EOF <(\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patterns := parsedPatternWords(t, test.src)
			if len(patterns) != 1 {
				t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
			}
			if patterns[0] != pattern {
				t.Errorf("pattern = %q, want %q", patterns[0], pattern)
			}
		})
	}
}

func TestParseNestedConditionalAlternationBeforeOuterHeredocBodyAcrossCaseArm(t *testing.T) {
	const pattern = "((a|b)|c)"
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "command substitution",
			src:  "cat <<EOF $(\ncase x in\n  x) : ;;\nesac\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
		{
			name: "process substitution",
			src:  "cat <<EOF <(\ncase x in\n  x) : ;;\nesac\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
		{
			name: "alternating case pattern in command substitution",
			src:  "cat <<EOF $(\ncase x in\n  x|y) : ;;\nesac\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
		{
			name: "nested command substitution in case pattern",
			src:  "cat <<EOF $(\ncase x in\n  $(print x)) : ;;\nesac\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
		{
			name: "nested process substitution in case pattern",
			src:  "cat <<EOF $(\ncase x in\n  <(print x)) : ;;\nesac\n[[ $line == ((a|b)|c) ]]\n)\nbody\nEOF\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patterns := parsedPatternWords(t, test.src)
			if len(patterns) != 1 {
				t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
			}
			if patterns[0] != pattern {
				t.Errorf("pattern = %q, want %q", patterns[0], pattern)
			}
		})
	}
}

func TestParseNestedConditionalAlternationWithDepthScopedFrames(t *testing.T) {
	const pattern = "((a|b)|c)"
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "case command in conditional child substitution",
			src: "cat <<EOF $(\n" +
				"[[ foo == $(case x in\n" +
				"  x) print ok ;;\n" +
				"esac\n" +
				") ]]\n" +
				"[[ $line == ((a|b)|c) ]]\n" +
				")\nbody\nEOF\n",
		},
		{
			name: "affected conditional in double quoted child substitution",
			src: "cat <<EOF $(\n" +
				"print \"$(\n" +
				"[[ $line == ((a|b)|c) ]]\n" +
				")\"\n" +
				")\nbody\nEOF\n",
		},
		{
			name: "inner heredoc resumes before outer heredoc body",
			src: "cat <<OUTER $(\n" +
				"cat <<INNER\n" +
				"[[ body == ((x|y)|z) ]]\n" +
				"INNER\n" +
				"[[ $line == ((a|b)|c) ]]\n" +
				")\nbody\nOUTER\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patterns := parsedPatternWords(t, test.src)
			restored := 0
			for _, got := range patterns {
				if got == pattern {
					restored++
				}
				if got == "((x|y)|z)" {
					t.Errorf("heredoc body lookalike became an active pattern: %q", patterns)
				}
			}
			if restored != 1 {
				t.Errorf("restored pattern count = %d, want 1: %q", restored, patterns)
			}
		})
	}
}

func TestParseNestedConditionalAlternationClosesRootCaseAtEsac(t *testing.T) {
	const pattern = "((a|b)|c)"
	const src = "case x in\n" +
		"  x) :\n" +
		"esac\n" +
		"[[ $line == ((a|b)|c) ]]\n"

	patterns := parsedPatternWords(t, src)
	if len(patterns) != 1 {
		t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
	}
	if patterns[0] != pattern {
		t.Errorf("pattern = %q, want %q", patterns[0], pattern)
	}
}

func TestParseNestedConditionalAlternationFindsCommandSubstitutionInsideArithmetic(t *testing.T) {
	const pattern = "((a|b)|c)"
	const src = "cat <<EOF $(\n" +
		"(( value = $(\n" +
		"  [[ $line == ((a|b)|c) ]]\n" +
		"  print 1\n" +
		") ))\n" +
		")\n" +
		"body\n" +
		"EOF\n"

	patterns := parsedPatternWords(t, src)
	if len(patterns) != 1 {
		t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
	}
	if patterns[0] != pattern {
		t.Errorf("pattern = %q, want %q", patterns[0], pattern)
	}
}

func TestParseNestedConditionalAlternationDoesNotTreatCaseOperandAsCommand(t *testing.T) {
	const src = "print $(\n[[ foo == bar || case == case ]]\n)\n[[ $line == ((a|b)|c) ]]\n"
	want := []string{"bar", "case", "((a|b)|c)"}
	got := parsedPatternWords(t, src)
	if len(got) != len(want) {
		t.Fatalf("pattern count = %d, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pattern[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseNestedConditionalAlternationWithANSICHeredocDelimiter(t *testing.T) {
	const pattern = "((a|b)|c)"
	src := "[[ $seed == ((a|b)|c) ]]\ncat <<$'EOF'\nbody\nEOF\n"
	patterns := parsedPatternWords(t, src)
	if len(patterns) != 1 {
		t.Fatalf("pattern count = %d, want 1: %q", len(patterns), patterns)
	}
	if patterns[0] != pattern {
		t.Errorf("pattern = %q, want %q", patterns[0], pattern)
	}
}

func TestNestedPatternBatchEditsSkipsANSICHeredocBodyLookalike(t *testing.T) {
	src := []byte(
		"[[ $seed == ((a|b)|c) ]]\n" +
			"cat <<$'E\\x4fF'\n" +
			"$E\\x4fF\n" +
			"[[ $fake == ((d|e)|f) ]]\n" +
			"EOF\n",
	)
	_, firstErr := parseTree(src, "ansi-c-heredoc.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
	}

	edits, ok := nestedPatternBatchEdits(src, int(parseErr.Pos.Offset()))
	if !ok {
		t.Fatal("nestedPatternBatchEdits() rejected a classified ANSI-C heredoc")
	}
	if len(edits) != 2 {
		t.Fatalf("edit count = %d, want 2: %+v", len(edits), edits)
	}
}

func TestNestedPatternBatchEditsSkipsProactiveLookalikes(t *testing.T) {
	const seed = "[[ $line == ((a|b)|c) ]]\n"
	tests := []struct {
		name   string
		suffix string
	}{
		{name: "single-quoted command word", suffix: "print '[[ $fake == ((d|e)|f) ]]'\n"},
		{name: "double-quoted command word", suffix: "print \"[[ $fake == ((d|e)|f) ]]\"\n"},
		{name: "comment", suffix: "# [[ $fake == ((d|e)|f) ]]\n"},
		{name: "quoted RHS", suffix: "[[ $fake == \"((d|e)|f)\" ]]\n"},
		{name: "escaped RHS", suffix: "[[ $fake == \\(\\(d\\|e\\)\\|f\\) ]]\n"},
		{name: "bracket expression", suffix: "[[ $fake == [d|e] ]]\n"},
		{name: "command substitution", suffix: "[[ $fake == $(print '((d|e)|f)') ]]\n"},
		{name: "process substitution", suffix: "[[ $fake == <(print '((d|e)|f)') ]]\n"},
		{name: "different operator", suffix: "[[ $fake =~ ((d|e)|f) ]]\n"},
		{name: "unbalanced pattern", suffix: "[[ $fake == ((d|e)|f ]]\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(seed + test.suffix)
			_, firstErr := parseTree(src, "proactive-lookalike.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}

			edits, ok := nestedPatternBatchEdits(src, int(parseErr.Pos.Offset()))
			if !ok {
				t.Fatal("nestedPatternBatchEdits() rejected the recognized seed")
			}
			if len(edits) != 2 {
				t.Fatalf("edit count = %d, want 2: %+v", len(edits), edits)
			}
			for _, edit := range edits {
				if edit.offset >= len(seed) {
					t.Errorf("proactive edit escaped the seed at byte %d", edit.offset)
				}
			}
		})
	}
}

func TestNestedPatternBatchEditsRequiresRecognizedSeed(t *testing.T) {
	src := []byte("[[ a | b ]]\n[[ $line == ((d|e)|f) ]]\n")
	_, firstErr := parseTree(src, "unrecognized-seed.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
	}
	if edits, ok := nestedPatternBatchEdits(src, int(parseErr.Pos.Offset())); ok {
		t.Fatalf("nestedPatternBatchEdits() = %+v, want rejected seed", edits)
	}
}

func TestNestedPatternBatchEditsSkipsHeredocLookalike(t *testing.T) {
	src := []byte("[[ $line == ((a|b)|c) ]]\ncat <<'EOF'\n[[ $fake == ((d|e)|f) ]]\nEOF\n")
	_, firstErr := parseTree(src, "heredoc-lookalike.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
	}
	edits, ok := nestedPatternBatchEdits(src, int(parseErr.Pos.Offset()))
	if !ok {
		t.Fatal("nestedPatternBatchEdits() rejected a classified heredoc")
	}
	if len(edits) != 2 {
		t.Fatalf("edit count = %d, want 2: %+v", len(edits), edits)
	}
}

func TestNestedConditionalAlternationBatchReturnsLaterParserError(t *testing.T) {
	src := []byte("[[ $line == ((a|b)|c) ]]\n[[ $other == ((d|e)|f) ]\n")
	_, firstErr := parseTree(src, "later-error.zsh")
	if firstErr == nil {
		t.Fatal("parseTree() unexpectedly accepted the original source")
	}

	compatibilityParses := 0
	tree, err := parseNestedConditionalAlternationWithParser(
		src,
		"later-error.zsh",
		firstErr,
		func(masked []byte, name string) (*syntax.File, error) {
			compatibilityParses++
			return parseTree(masked, name)
		},
	)
	if err == nil {
		t.Fatalf("parseNestedConditionalAlternationWithParser() = %#v, want parser error", tree)
	}
	if compatibilityParses != 1 {
		t.Errorf("compatibility parse count = %d, want 1", compatibilityParses)
	}
}

func TestNestedConditionalAlternationRetryReturnsUnmatchedRootCloseError(t *testing.T) {
	src := []byte("[[ $line == ((a|b)|c) ]]\n)\n")
	_, firstErr := parseTree(src, "unmatched-root-close.zsh")
	var firstParseErr syntax.ParseError
	if !errors.As(firstErr, &firstParseErr) || firstParseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want line-1 %q", firstErr, invalidAlternationOperator)
	}

	compatibilityParses := 0
	tree, err := parseNestedConditionalAlternationWithParser(
		src,
		"unmatched-root-close.zsh",
		firstErr,
		func(masked []byte, name string) (*syntax.File, error) {
			compatibilityParses++
			return parseTree(masked, name)
		},
	)
	if err == nil {
		t.Fatalf("parseNestedConditionalAlternationWithParser() = %#v, want parser error", tree)
	}
	if compatibilityParses != 1 {
		t.Errorf("compatibility parse count = %d, want 1", compatibilityParses)
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
	}
	if line, column := parseErr.Pos.Line(), parseErr.Pos.Col(); line != 2 || column != 1 {
		t.Errorf("error position = %d:%d, want 2:1", line, column)
	}
	const wantText = "`)` can only be used to close a subshell"
	if parseErr.Text != wantText {
		t.Errorf("error text = %q, want %q", parseErr.Text, wantText)
	}
}

func TestParseNestedConditionalAlternation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "double equals",
			src:  "line=foo\n[[ $line == ((a|b)|c) ]]\n",
			want: []string{"((a|b)|c)"},
		},
		{
			name: "single equals",
			src:  "[[ $line = ((a|b)|c) ]]\n",
			want: []string{"((a|b)|c)"},
		},
		{
			name: "not equals",
			src:  "[[ $line != ((a|b)|c) ]]\n",
			want: []string{"((a|b)|c)"},
		},
		{
			name: "work buffer reproduction",
			src:  "while [[ $___workbuf = (#b)...((...)|...)(*) ]]; do :; done\n",
			want: []string{"(#b)...((...)|...)(*)"},
		},
		{
			name: "secret token reproduction",
			src:  "[[ $line == (#i)*((token|password|secret|api_key|apikey|private_key)=|Bearer )* ]] && return 2\n",
			want: []string{"(#i)*((token|password|secret|api_key|apikey|private_key)=|Bearer )*"},
		},
		{
			name: "triple nesting",
			src:  "[[ $line == (((a|b)|c)|d) ]]\n",
			want: []string{"(((a|b)|c)|d)"},
		},
		{
			name: "two affected comparisons",
			src:  "[[ $a == ((a|b)|c) ]]\n[[ $b != ((d|e)|f) ]]\n",
			want: []string{"((a|b)|c)", "((d|e)|f)"},
		},
		{
			name: "short parameter expansion",
			src:  "[[ $line == (($prefix|a)|b) ]]\n",
			want: []string{"(($prefix|a)|b)"},
		},
		{
			name: "multibyte prefix",
			src:  "print 'λ'\n[[ $line == ((a|b)|c) ]]\nprint after\n",
			want: []string{"((a|b)|c)"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parsedPatternWords(t, test.src)
			if len(got) != len(test.want) {
				t.Fatalf("pattern count = %d, want %d: %q", len(got), len(test.want), got)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Errorf("pattern[%d] = %q, want %q", i, got[i], test.want[i])
				}
			}
		})
	}
}

func TestNestedConditionalAlternationFailsClosed(t *testing.T) {
	tests := []string{
		"[[ a | b ]]\n",
		"[[ $line == ((a|b)|c) ]\n",
		"[[ $line == [abc|] ]]\n",
	}
	for _, src := range tests {
		if _, err := Parse(strings.NewReader(src), "invalid.zsh"); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", src)
		}
	}
}

func TestNestedConditionalAlternationBypassesExistingValidSyntax(t *testing.T) {
	tests := []string{
		"[[ $line == $((1 | 2)) ]]\n",
		"[[ $line == $(print '(a|b)') ]]\n",
		"[[ $line == <(print '(a|b)') ]]\n",
		"[[ $line == \"((a|b)|c)\" ]]\n",
		"[[ $line == \\(\\(a\\|b\\)\\|c\\) ]]\n",
		"[[ ( $line == (a|b) ) || $line == c ]]\n",
	}
	for _, src := range tests {
		if _, err := Parse(strings.NewReader(src), "valid-control.zsh"); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
}

func TestRestorePatternEditsRejectsMaskMismatch(t *testing.T) {
	original := []byte("[[ $line == ((a|b)|c) ]]\n")
	masked := []byte("[[ $line == (xa|bx|c) ]]\n")
	tree, err := parseTree(masked, "masked.zsh")
	if err != nil {
		t.Fatalf("parseTree() error: %v", err)
	}

	offset := strings.IndexByte(string(masked), 'x')
	edit := patternEdit{offset: offset, original: '(', replacement: 'y'}
	if err := restorePatternEdits(tree, original, []patternEdit{edit}); err == nil {
		t.Fatal("restorePatternEdits() unexpectedly succeeded")
	}
}
