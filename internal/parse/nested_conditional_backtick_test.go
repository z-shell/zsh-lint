package parse

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseNestedConditionalAlternationPreservesBacktickStatements(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantStmts int
	}{
		{
			name:      "following ordinary statement",
			src:       "print `[[ $line == ((a|b)|c) ]]`\nprint second\n",
			wantStmts: 2,
		},
		{
			name:      "two affected statements",
			src:       "print `[[ $line == ((a|b)|c) ]]`\nprint `[[ $other == ((d|e)|f) ]]`\n",
			wantStmts: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "backtick-statements.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := len(file.AST().Stmts); got != test.wantStmts {
				t.Errorf("statement count = %d, want %d", got, test.wantStmts)
			}
		})
	}
}

func TestParseNestedConditionalAlternationPreservesBacktickWords(t *testing.T) {
	const src = "print before `[[ $line == ((a|b)|c) ]]` after\nprint second\n"
	file, err := Parse(strings.NewReader(src), "backtick-words.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := len(file.AST().Stmts); got != 2 {
		t.Fatalf("statement count = %d, want 2", got)
	}
	call, ok := file.AST().Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok {
		t.Fatalf("first statement command = %T, want *syntax.CallExpr", file.AST().Stmts[0].Cmd)
	}
	if got := len(call.Args); got != 4 {
		t.Errorf("first statement argument count = %d, want 4", got)
	}
}

func TestParseNestedConditionalAlternationSupportsBacktickBoundaries(t *testing.T) {
	tests := []string{
		"print `[[ $line == ((a|b)|c) ]]`",
		"print `[[ $line == ((a|b)|c) ]]`suffix\n",
		"print \"`[[ $line == ((a|b)|c) ]]`suffix\"\n",
	}

	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(src), "backtick-boundary.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			patterns := patternWords(t, file.AST())
			if len(patterns) != 1 || patterns[0] != "((a|b)|c)" {
				t.Fatalf("patterns = %q, want [((a|b)|c)]", patterns)
			}
			wantOpen := strings.IndexByte(src, '`')
			wantClose := strings.LastIndexByte(src, '`')
			matches := 0
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				substitution, ok := node.(*syntax.CmdSubst)
				if !ok || !substitution.Backquotes {
					return true
				}
				matches++
				if got := int(substitution.Left.Offset()); got != wantOpen {
					t.Errorf("backtick open offset = %d, want %d", got, wantOpen)
				}
				if got := int(substitution.Right.Offset()); got != wantClose {
					t.Errorf("backtick close offset = %d, want %d", got, wantClose)
				}
				return true
			})
			if matches != 1 {
				t.Errorf("backtick substitution count = %d, want 1", matches)
			}
		})
	}
}

func TestParseNestedConditionalAlternationClosesBacktickAfterEscapedBackslash(t *testing.T) {
	const src = "print `[[ $line == ((a|b)|c) ]]; print \\\\`suffix\n"
	file, err := Parse(strings.NewReader(src), "backtick-even-backslashes.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	patterns := patternWords(t, file.AST())
	if len(patterns) != 1 || patterns[0] != "((a|b)|c)" {
		t.Fatalf("patterns = %q, want [((a|b)|c)]", patterns)
	}

	wantOpen := strings.IndexByte(src, '`')
	wantClose := strings.LastIndexByte(src, '`')
	matches := 0
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		substitution, ok := node.(*syntax.CmdSubst)
		if !ok || !substitution.Backquotes {
			return true
		}
		matches++
		if got := int(substitution.Left.Offset()); got != wantOpen {
			t.Errorf("backtick open offset = %d, want %d", got, wantOpen)
		}
		if got := int(substitution.Right.Offset()); got != wantClose {
			t.Errorf("backtick close offset = %d, want %d", got, wantClose)
		}
		return true
	})
	if matches != 1 {
		t.Errorf("backtick substitution count = %d, want 1", matches)
	}
}

func TestLegacyBacktickEscapeStateMatchesNestedParserConsumption(t *testing.T) {
	tests := []struct {
		name                 string
		rawBackslashes       int
		wantEscapeDepth      int
		wantDelimiterEscapes int
	}{
		{name: "unescaped", rawBackslashes: 0, wantEscapeDepth: 0, wantDelimiterEscapes: 0},
		{name: "depth one", rawBackslashes: 1, wantEscapeDepth: 1, wantDelimiterEscapes: 1},
		{name: "escaped backslash", rawBackslashes: 2, wantEscapeDepth: 0, wantDelimiterEscapes: 0},
		{name: "depth two", rawBackslashes: 3, wantEscapeDepth: 2, wantDelimiterEscapes: 3},
		{name: "two escaped backslashes", rawBackslashes: 4, wantEscapeDepth: 0, wantDelimiterEscapes: 0},
		{name: "escaped pairs then depth one", rawBackslashes: 5, wantEscapeDepth: 1, wantDelimiterEscapes: 1},
		{name: "three escaped backslashes", rawBackslashes: 6, wantEscapeDepth: 0, wantDelimiterEscapes: 0},
		{name: "depth three", rawBackslashes: 7, wantEscapeDepth: 3, wantDelimiterEscapes: 7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(strings.Repeat("\\", test.rawBackslashes) + "`")
			gotDepth, gotDelimiterEscapes := legacyBacktickEscapeStateBefore(src, len(src)-1)
			if gotDepth != test.wantEscapeDepth || gotDelimiterEscapes != test.wantDelimiterEscapes {
				t.Errorf(
					"legacyBacktickEscapeStateBefore() = depth %d delimiter escapes %d, want %d/%d",
					gotDepth,
					gotDelimiterEscapes,
					test.wantEscapeDepth,
					test.wantDelimiterEscapes,
				)
			}
		})
	}
}

func TestParseNestedConditionalAlternationRejectsLeadingAndAfterBacktick(t *testing.T) {
	const src = "print `[[ $line == ((a|b)|c) ]]`\n&& print second\n"
	if _, err := Parse(strings.NewReader(src), "backtick-leading-and.zsh"); err == nil {
		t.Fatal("Parse() unexpectedly accepted native-invalid leading &&")
	}
}

func TestParseNestedConditionalAlternationResumesAfterBacktickSuffix(t *testing.T) {
	const src = "print `[[ $line == ((a|b)|c) ]]`#tag; [[ $other == ((d|e)|f) ]]\n"
	file, err := Parse(strings.NewReader(src), "backtick-suffix-resume.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	want := []string{"((a|b)|c)", "((d|e)|f)"}
	patterns := patternWords(t, file.AST())
	if len(patterns) != len(want) {
		t.Fatalf("pattern count = %d, want %d: %q", len(patterns), len(want), patterns)
	}
	for index := range want {
		if patterns[index] != want[index] {
			t.Errorf("pattern[%d] = %q, want %q", index, patterns[index], want[index])
		}
	}
}

func TestLegacyBacktickSpansPreserveNestedDelimiterDepth(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantSpanCount int
		wantMarked    int
	}{
		{
			name:          "depth one",
			src:           "print `[[ $line == ((a|b)|c) ]]`\n",
			wantSpanCount: 1,
			wantMarked:    1,
		},
		{
			name:          "escaped depth two",
			src:           "print `print \\`[[ $line == ((a|b)|c) ]]\\``\n",
			wantSpanCount: 2,
			wantMarked:    2,
		},
		{
			name:          "affected nested and outer",
			src:           "print `print \\`[[ $inner == ((d|e)|f) ]]\\`; [[ $outer == ((a|b)|c) ]]`\n",
			wantSpanCount: 2,
			wantMarked:    2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(test.src)
			_, firstErr := parseTree(src, "backtick-spans.zsh")
			var parseErr syntax.ParseError
			if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
				t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
			}
			batch, ok := nestedPatternBatch(src, int(parseErr.Pos.Offset()))
			if !ok {
				t.Fatal("nestedPatternBatch() rejected affected legacy backticks")
			}
			if len(batch.backtickIslands) != 1 {
				t.Fatalf("island count = %d, want 1", len(batch.backtickIslands))
			}
			island := batch.backtickIslands[0]
			if got := countLegacyBacktickSpans(island.root); got != test.wantSpanCount {
				t.Errorf("span count = %d, want %d", got, test.wantSpanCount)
			}
			if got := len(island.markedClosures); got != test.wantMarked {
				t.Errorf("marked closure count = %d, want %d", got, test.wantMarked)
			}

			wantOuterOpen := strings.IndexByte(test.src, '`')
			wantOuterClose := strings.LastIndexByte(test.src, '`')
			if island.root.openOffset != wantOuterOpen {
				t.Errorf("outer open offset = %d, want %d", island.root.openOffset, wantOuterOpen)
			}
			if island.root.closeStart != wantOuterClose || island.root.closeOffset != wantOuterClose {
				t.Errorf("outer close = start %d offset %d, want %d", island.root.closeStart, island.root.closeOffset, wantOuterClose)
			}
			if test.wantSpanCount == 2 {
				if len(island.root.children) != 1 {
					t.Fatalf("outer child count = %d, want 1", len(island.root.children))
				}
				child := island.root.children[0]
				innerOpenSpelling := strings.Index(test.src[wantOuterOpen+1:], "\\`") + wantOuterOpen + 1
				innerCloseSpelling := strings.LastIndex(test.src[:wantOuterClose], "\\`")
				if child.openOffset != innerOpenSpelling+1 {
					t.Errorf("inner open offset = %d, want %d", child.openOffset, innerOpenSpelling+1)
				}
				if child.closeStart != innerCloseSpelling || child.closeOffset != innerCloseSpelling+1 {
					t.Errorf("inner close = start %d offset %d, want start %d offset %d", child.closeStart, child.closeOffset, innerCloseSpelling, innerCloseSpelling+1)
				}
			}
		})
	}
}

func countLegacyBacktickSpans(span *legacyBacktickSpan) int {
	if span == nil {
		return 0
	}
	count := 1
	for _, child := range span.children {
		count += countLegacyBacktickSpans(child)
	}
	return count
}

func TestApplyLegacyBacktickPlaceholdersIsSameWidth(t *testing.T) {
	const src = "print before `print one\n[[ $line == ((a|b)|c) ]]` after\nprint second\n"
	original := []byte(src)
	_, firstErr := parseTree(original, "backtick-placeholder.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
	}
	batch, ok := nestedPatternBatch(original, int(parseErr.Pos.Offset()))
	if !ok || len(batch.backtickIslands) != 1 {
		t.Fatalf("nestedPatternBatch() islands = %+v, ok = %v, want one", batch.backtickIslands, ok)
	}

	masked := bytes.Clone(original)
	for _, edit := range batch.edits {
		masked[edit.offset] = edit.replacement
	}
	if err := applyLegacyBacktickPlaceholders(masked, original, batch.backtickIslands); err != nil {
		t.Fatalf("applyLegacyBacktickPlaceholders() error: %v", err)
	}
	if len(masked) != len(original) {
		t.Fatalf("masked length = %d, want %d", len(masked), len(original))
	}
	root := batch.backtickIslands[0].root
	if !bytes.Equal(masked[:root.openOffset+1], original[:root.openOffset+1]) {
		t.Errorf("bytes before island body changed: %q", masked[:root.openOffset+1])
	}
	if !bytes.Equal(masked[root.closeStart:], original[root.closeStart:]) {
		t.Errorf("bytes after island body changed: %q", masked[root.closeStart:])
	}
	for offset, b := range original {
		if b == '\n' && masked[offset] != '\n' {
			t.Errorf("newline at byte %d changed to %q", offset, masked[offset])
		}
		if b == '`' && masked[offset] != '`' {
			t.Errorf("backtick at byte %d changed to %q", offset, masked[offset])
		}
	}
	body := masked[root.openOffset+1 : root.closeStart]
	if !bytes.Contains(body, []byte(":")) {
		t.Fatalf("placeholder body %q does not contain neutral ':'", body)
	}
	placeholderTree, err := parseTree(masked, "backtick-placeholder.zsh")
	if err != nil {
		t.Fatalf("parseTree(placeholder) error: %v", err)
	}
	if len(placeholderTree.Stmts) != 2 {
		t.Errorf("placeholder statement count = %d, want 2", len(placeholderTree.Stmts))
	}
}

func legacyBatchForTest(t *testing.T, src string) ([]byte, nestedPatternCompatibilityBatch) {
	t.Helper()
	original := []byte(src)
	_, firstErr := parseTree(original, "legacy-island.zsh")
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
	}
	batch, ok := nestedPatternBatch(original, int(parseErr.Pos.Offset()))
	if !ok {
		t.Fatal("nestedPatternBatch() rejected affected legacy backticks")
	}
	if len(batch.backtickIslands) != 1 {
		t.Fatalf("island count = %d, want 1", len(batch.backtickIslands))
	}
	return original, batch
}

func maskedPatternSource(t *testing.T, original []byte, edits []patternEdit) []byte {
	t.Helper()
	masked := bytes.Clone(original)
	for _, edit := range edits {
		if edit.offset < 0 || edit.offset >= len(masked) || masked[edit.offset] != edit.original {
			t.Fatalf("pattern edit %+v does not match source", edit)
		}
		masked[edit.offset] = edit.replacement
	}
	return masked
}

func TestBuildLegacyBacktickIslandMapsNestedTerminators(t *testing.T) {
	const src = "print `print \\`[[ $line == ((a|b)|c) ]]\\``\n"
	original, batch := legacyBatchForTest(t, src)
	masked := maskedPatternSource(t, original, batch.edits)
	transformed, sourceMap, err := buildLegacyBacktickIsland(masked, original, batch.backtickIslands[0])
	if err != nil {
		t.Fatalf("buildLegacyBacktickIsland() error: %v", err)
	}
	if !bytes.Contains(transformed, []byte("\n\\`")) {
		t.Errorf("transformed island %q lacks newline before complete nested close", transformed)
	}
	if got := bytes.Count(transformed, []byte("`")); got != 4 {
		t.Errorf("transformed backtick count = %d, want 4", got)
	}
	if len(sourceMap.originalOffsetByBoundary) != len(transformed)+1 {
		t.Fatalf("source-map boundary count = %d, want %d", len(sourceMap.originalOffsetByBoundary), len(transformed)+1)
	}
	root := batch.backtickIslands[0].root
	delimiterOffsets := []int{
		root.openOffset,
		root.children[0].openOffset,
		root.children[0].closeStart,
		root.children[0].closeOffset,
		root.closeOffset,
	}
	for _, originalOffset := range delimiterOffsets {
		mapped := false
		for transformedOffset := 0; transformedOffset < len(transformed); transformedOffset++ {
			if sourceMap.originalOffsetByBoundary[transformedOffset] == originalOffset &&
				sourceMap.originalOffsetByBoundary[transformedOffset+1] == originalOffset+1 &&
				transformed[transformedOffset] == original[originalOffset] {
				mapped = true
				break
			}
		}
		if !mapped {
			t.Errorf("original delimiter byte %d was not mapped exactly", originalOffset)
		}
	}
}

func TestGraftLegacyBacktickIslandRestoresOriginalPositions(t *testing.T) {
	const src = "print pre\nprint `[[ $line == ((a|b)|c) ]]` suffix\nprint post\n"
	file, err := Parse(strings.NewReader(src), "legacy-graft.zsh")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := len(file.AST().Stmts); got != 3 {
		t.Fatalf("statement count = %d, want 3", got)
	}

	wantOpen := strings.IndexByte(src, '`')
	wantClose := strings.LastIndexByte(src, '`')
	var outer *syntax.CmdSubst
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		substitution, ok := node.(*syntax.CmdSubst)
		if ok && substitution.Backquotes && int(substitution.Left.Offset()) == wantOpen {
			outer = substitution
		}
		return true
	})
	if outer == nil {
		t.Fatal("grafted outer legacy CmdSubst not found")
	}
	if got := int(outer.Right.Offset()); got != wantClose {
		t.Errorf("outer close offset = %d, want %d", got, wantClose)
	}
	if outer.Left.Line() != 2 || outer.Left.Col() != 7 {
		t.Errorf("outer open position = line %d column %d, want line 2 column 7", outer.Left.Line(), outer.Left.Col())
	}
	if outer.Right.Line() != 2 || outer.Right.Col() != 32 {
		t.Errorf("outer close position = line %d column %d, want line 2 column 32", outer.Right.Line(), outer.Right.Col())
	}

	syntax.Walk(outer, func(node syntax.Node) bool {
		lit, ok := node.(*syntax.Lit)
		if !ok {
			return true
		}
		start := int(lit.ValuePos.Offset())
		end := int(lit.ValueEnd.Offset())
		if start < 0 || end < start || end > len(src) {
			t.Fatalf("literal span %d:%d is outside source", start, end)
		}
		if got := src[start:end]; got != lit.Value {
			t.Errorf("literal at %d:%d = %q, want source slice %q", start, end, lit.Value, got)
		}
		return true
	})

	testClause, ok := outer.Stmts[0].Cmd.(*syntax.TestClause)
	if !ok {
		t.Fatalf("grafted command = %T, want *syntax.TestClause", outer.Stmts[0].Cmd)
	}
	if got := int(testClause.Pos().Offset()); got != 17 || testClause.Pos().Line() != 2 || testClause.Pos().Col() != 8 {
		t.Errorf("test-clause position = offset %d line %d column %d, want 17/2/8", got, testClause.Pos().Line(), testClause.Pos().Col())
	}
	if got := int(file.AST().Stmts[2].Pos().Offset()); got != 50 || file.AST().Stmts[2].Pos().Line() != 3 || file.AST().Stmts[2].Pos().Col() != 1 {
		t.Errorf("final statement position = offset %d line %d column %d, want 50/3/1", got, file.AST().Stmts[2].Pos().Line(), file.AST().Stmts[2].Pos().Col())
	}
}

func TestLegacyBacktickIslandAddsNoSyntheticSemicolon(t *testing.T) {
	tests := []string{
		"print `[[ $line == ((a|b)|c) ]]`\n",
		"print `print \\`[[ $inner == ((d|e)|f) ]]\\`; [[ $outer == ((a|b)|c) ]]`\n",
	}
	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			original, batch := legacyBatchForTest(t, src)
			masked := maskedPatternSource(t, original, batch.edits)
			transformed, sourceMap, err := buildLegacyBacktickIsland(masked, original, batch.backtickIslands[0])
			if err != nil {
				t.Fatalf("buildLegacyBacktickIsland() error: %v", err)
			}
			tree, err := parseTree(transformed, "synthetic-semicolon.zsh")
			if err != nil {
				t.Fatalf("parseTree(island) error: %v", err)
			}
			syntax.Walk(tree, func(node syntax.Node) bool {
				stmt, ok := node.(*syntax.Stmt)
				if !ok || !stmt.Semicolon.IsValid() {
					return true
				}
				if _, synthetic := sourceMap.syntheticOffsets[int(stmt.Semicolon.Offset())]; synthetic {
					t.Errorf("statement semicolon is valid at synthetic byte %d", stmt.Semicolon.Offset())
				}
				return true
			})
		})
	}
}

func TestLegacyBacktickIslandPreservesDoubleQuotedParentSignature(t *testing.T) {
	const affected = "print \"prefix `[[ $line == ((a|b)|c) ]]` suffix\"\n"
	const control = "print \"prefix `[[ $line == foo ]]; :` suffix\"\n"

	affectedFile, err := Parse(strings.NewReader(affected), "affected-quoted-parent.zsh")
	if err != nil {
		t.Fatalf("Parse(affected) error: %v", err)
	}
	controlTree, err := parseTree([]byte(control), "control-quoted-parent.zsh")
	if err != nil {
		t.Fatalf("parseTree(control) error: %v", err)
	}

	signature := func(t *testing.T, tree *syntax.File) ([]string, int) {
		t.Helper()
		var quoted *syntax.DblQuoted
		syntax.Walk(tree, func(node syntax.Node) bool {
			if quoted == nil {
				quoted, _ = node.(*syntax.DblQuoted)
			}
			return true
		})
		if quoted == nil {
			t.Fatal("double-quoted parent not found")
		}
		var literals []string
		backquotes := 0
		for _, part := range quoted.Parts {
			switch part := part.(type) {
			case *syntax.Lit:
				literals = append(literals, part.Value)
			case *syntax.CmdSubst:
				if part.Backquotes {
					backquotes++
				}
			}
		}
		return literals, backquotes
	}

	wantLiterals, wantBackquotes := signature(t, controlTree)
	gotLiterals, gotBackquotes := signature(t, affectedFile.AST())
	if strings.Join(gotLiterals, "|") != strings.Join(wantLiterals, "|") {
		t.Errorf("double-quoted literal signature = %q, want %q", gotLiterals, wantLiterals)
	}
	if gotBackquotes != wantBackquotes || gotBackquotes != 1 {
		t.Errorf("nested legacy CmdSubst count = %d, want control count %d and literal 1", gotBackquotes, wantBackquotes)
	}
}

func TestLegacyBacktickIslandRejectsHiddenInvalidSyntax(t *testing.T) {
	tests := []string{
		"print `[[ $line == ((a|b)|c) ]]; then`\n",
		"print `[[ $line == ((a|b)|c) ]]`\n&& print second\n",
	}
	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), "hidden-invalid.zsh"); err == nil {
				t.Fatal("Parse() unexpectedly accepted native-invalid source")
			}
		})
	}
}

func TestLegacyBacktickIslandWorkIsLinearForSiblings(t *testing.T) {
	measure := func(t *testing.T, siblingCount int) (work, fullFileParses, islandParses int) {
		t.Helper()
		src := []byte(strings.Repeat("print `[[ $line == ((a|b)|c) ]]`\n", siblingCount))
		_, firstErr := parseTree(src, "legacy-siblings.zsh")
		var parseErr syntax.ParseError
		if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
			t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
		}
		tree, err := parseNestedConditionalAlternationWithParser(
			src,
			"legacy-siblings.zsh",
			firstErr,
			func(candidate []byte, name string) (*syntax.File, error) {
				if len(candidate) == len(src) {
					fullFileParses++
				} else {
					islandParses++
				}
				return parseTree(candidate, name)
			},
			func() { work++ },
		)
		if err != nil {
			t.Fatalf("parseNestedConditionalAlternationWithParser() error: %v", err)
		}
		if got := len(tree.Stmts); got != siblingCount {
			t.Fatalf("statement count = %d, want %d", got, siblingCount)
		}
		if fullFileParses != 1 {
			t.Errorf("full-file compatibility parse count = %d, want 1", fullFileParses)
		}
		if islandParses != siblingCount {
			t.Errorf("island parse count = %d, want %d", islandParses, siblingCount)
		}
		return work, fullFileParses, islandParses
	}

	smallWork, _, _ := measure(t, 256)
	largeWork, _, _ := measure(t, 512)
	if smallWork == 0 || largeWork == 0 {
		t.Fatalf("island work = %d/%d, want production observations", smallWork, largeWork)
	}
	const fixedAllowance = 128
	if largeWork > 2*smallWork+fixedAllowance {
		t.Errorf("doubling siblings changed island work from %d to %d, want at most %d", smallWork, largeWork, 2*smallWork+fixedAllowance)
	}
}

func TestLegacyBacktickGraftTargetIndexIsLinearForSiblings(t *testing.T) {
	measure := func(t *testing.T, siblingCount int) (inspections int) {
		t.Helper()
		src := []byte(strings.Repeat("print `[[ $line == ((a|b)|c) ]]`\n", siblingCount))
		_, firstErr := parseTree(src, "legacy-target-index.zsh")
		var parseErr syntax.ParseError
		if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
			t.Fatalf("parseTree() error = %v, want %q", firstErr, invalidAlternationOperator)
		}
		batch, ok := nestedPatternBatch(src, int(parseErr.Pos.Offset()))
		if !ok || len(batch.backtickIslands) != siblingCount {
			t.Fatalf("nestedPatternBatch() island count = %d, ok = %v, want %d", len(batch.backtickIslands), ok, siblingCount)
		}
		patternMasked := maskedPatternSource(t, src, batch.edits)
		placeholder := bytes.Clone(patternMasked)
		if err := applyLegacyBacktickPlaceholders(placeholder, src, batch.backtickIslands); err != nil {
			t.Fatalf("applyLegacyBacktickPlaceholders() error: %v", err)
		}
		tree, err := parseTree(placeholder, "legacy-target-index.zsh")
		if err != nil {
			t.Fatalf("parseTree(placeholder) error: %v", err)
		}
		index := indexLegacyBacktickGraftTargets(tree, func() { inspections++ })
		if len(index) != siblingCount {
			t.Fatalf("target index size = %d, want %d", len(index), siblingCount)
		}
		for _, island := range batch.backtickIslands {
			key := legacyBacktickTargetKey{
				openOffset:  island.root.openOffset,
				closeOffset: island.root.closeOffset,
			}
			if got := len(index[key]); got != 1 {
				t.Errorf("target count for %+v = %d, want 1", key, got)
			}
		}
		return inspections
	}

	smallInspections := measure(t, 256)
	largeInspections := measure(t, 512)
	if smallInspections == 0 || largeInspections == 0 {
		t.Fatalf("target-index inspections = %d/%d, want production observations", smallInspections, largeInspections)
	}
	const fixedAllowance = 128
	if largeInspections > 2*smallInspections+fixedAllowance {
		t.Errorf(
			"doubling siblings changed target-index inspections from %d to %d, want at most %d",
			smallInspections,
			largeInspections,
			2*smallInspections+fixedAllowance,
		)
	}
}

func placeholderTreeForLegacyTest(
	t *testing.T,
	src string,
) (*syntax.File, []byte, []byte, nestedPatternCompatibilityBatch) {
	t.Helper()
	original, batch := legacyBatchForTest(t, src)
	patternMasked := maskedPatternSource(t, original, batch.edits)
	placeholder := bytes.Clone(patternMasked)
	if err := applyLegacyBacktickPlaceholders(placeholder, original, batch.backtickIslands); err != nil {
		t.Fatalf("applyLegacyBacktickPlaceholders() error: %v", err)
	}
	tree, err := parseTree(placeholder, "legacy-integrity.zsh")
	if err != nil {
		t.Fatalf("parseTree(placeholder) error: %v", err)
	}
	return tree, patternMasked, original, batch
}

func TestLegacyBacktickIslandRejectsIntegrityFailures(t *testing.T) {
	const src = "print `[[ $line == ((a|b)|c) ]]`\n"

	t.Run("placeholder delimiter mismatch", func(t *testing.T) {
		_, masked, original, batch := placeholderTreeForLegacyTest(t, src)
		root := batch.backtickIslands[0].root
		masked[root.openOffset] = 'x'
		if err := applyLegacyBacktickPlaceholders(masked, original, batch.backtickIslands); err == nil {
			t.Fatal("applyLegacyBacktickPlaceholders() accepted a delimiter mismatch")
		}
	})

	t.Run("overlapping island", func(t *testing.T) {
		tree, masked, original, batch := placeholderTreeForLegacyTest(t, src)
		islands := []legacyBacktickIsland{batch.backtickIslands[0], batch.backtickIslands[0]}
		if err := parseAndGraftLegacyBacktickIslands(tree, masked, original, "overlap.zsh", islands, parseTree); err == nil {
			t.Fatal("parseAndGraftLegacyBacktickIslands() accepted overlapping islands")
		}
	})

	t.Run("missing graft target", func(t *testing.T) {
		_, masked, original, batch := placeholderTreeForLegacyTest(t, src)
		if err := parseAndGraftLegacyBacktickIslands(&syntax.File{}, masked, original, "missing.zsh", batch.backtickIslands, parseTree); err == nil {
			t.Fatal("parseAndGraftLegacyBacktickIslands() accepted a missing target")
		}
	})

	t.Run("two graft targets", func(t *testing.T) {
		tree, masked, original, batch := placeholderTreeForLegacyTest(t, src)
		tree.Stmts = append(tree.Stmts, tree.Stmts[0])
		if err := parseAndGraftLegacyBacktickIslands(tree, masked, original, "duplicate.zsh", batch.backtickIslands, parseTree); err == nil {
			t.Fatal("parseAndGraftLegacyBacktickIslands() accepted two targets")
		}
	})

	t.Run("non-monotonic source map", func(t *testing.T) {
		_, _, _, batch := placeholderTreeForLegacyTest(t, src)
		sourceMap := islandSourceMap{
			originalOffsetByBoundary: []int{8, 7},
			syntheticOffsets:         map[int]struct{}{},
		}
		if err := validateIslandSourceMap(sourceMap, 1, batch.backtickIslands[0].root); err == nil {
			t.Fatal("validateIslandSourceMap() accepted a non-monotonic map")
		}
	})

	t.Run("mapped offset outside original span", func(t *testing.T) {
		_, _, _, batch := placeholderTreeForLegacyTest(t, src)
		stmt := &syntax.Stmt{Position: syntax.NewPos(0, 1, 1)}
		sourceMap := islandSourceMap{
			originalOffsetByBoundary: []int{0},
			syntheticOffsets:         map[int]struct{}{},
		}
		if err := rebaseLegacyBacktickPositions(
			reflect.ValueOf(stmt),
			sourceMap,
			[]int{0},
			batch.backtickIslands[0].root,
		); err == nil {
			t.Fatal("rebaseLegacyBacktickPositions() accepted an outside mapping")
		}
	})

	t.Run("synthetic semicolon", func(t *testing.T) {
		substitution := &syntax.CmdSubst{Stmts: []*syntax.Stmt{{
			Semicolon: syntax.NewPos(1, 1, 2),
		}}}
		sourceMap := islandSourceMap{syntheticOffsets: map[int]struct{}{1: {}}}
		if err := rejectSyntheticSemicolons(substitution, sourceMap); err == nil {
			t.Fatal("rejectSyntheticSemicolons() accepted an injected synthetic semicolon")
		}
	})

	t.Run("island parser error", func(t *testing.T) {
		tree, masked, original, batch := placeholderTreeForLegacyTest(t, src)
		wantErr := errors.New("injected island parse failure")
		err := parseAndGraftLegacyBacktickIslands(
			tree,
			masked,
			original,
			"parse-error.zsh",
			batch.backtickIslands,
			func([]byte, string) (*syntax.File, error) { return nil, wantErr },
		)
		if !errors.Is(err, wantErr) {
			t.Fatalf("parseAndGraftLegacyBacktickIslands() error = %v, want %v", err, wantErr)
		}
	})
}
