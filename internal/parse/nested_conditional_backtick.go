package parse

import (
	"fmt"
	"reflect"
	"sort"

	"mvdan.cc/sh/v3/syntax"
)

type islandSourceMap struct {
	originalOffsetByBoundary []int
	syntheticOffsets         map[int]struct{}
}

type legacyBacktickWorkProbe func()

type legacyBacktickTargetKey struct {
	openOffset  int
	closeOffset int
}

type legacyBacktickTargetIndexProbe func()

func buildLegacyBacktickIsland(
	masked, original []byte,
	island legacyBacktickIsland,
	probes ...legacyBacktickWorkProbe,
) ([]byte, islandSourceMap, error) {
	var inspect legacyBacktickWorkProbe
	if len(probes) > 0 {
		inspect = probes[0]
	}
	root := island.root
	if len(masked) != len(original) || root == nil || root.openStart < 0 ||
		root.openOffset < root.openStart || root.closeStart <= root.openOffset ||
		root.closeOffset < root.closeStart || root.closeOffset >= len(original) {
		return nil, islandSourceMap{}, fmt.Errorf("invalid legacy backtick island bounds")
	}
	if original[root.openOffset] != '`' || original[root.closeOffset] != '`' ||
		masked[root.openOffset] != '`' || masked[root.closeOffset] != '`' {
		return nil, islandSourceMap{}, fmt.Errorf("legacy backtick island delimiter mismatch")
	}

	marked := append([]*legacyBacktickSpan(nil), island.markedClosures...)
	sort.Slice(marked, func(i, j int) bool {
		return marked[i].closeStart < marked[j].closeStart
	})
	markedByCloseStart := make(map[int]*legacyBacktickSpan, len(marked))
	previousCloseStart := -1
	for _, span := range marked {
		if span == nil || span.closeStart <= root.openOffset ||
			span.closeOffset > root.closeOffset || span.closeStart <= previousCloseStart ||
			span.closeOffset < span.closeStart || original[span.closeOffset] != '`' {
			return nil, islandSourceMap{}, fmt.Errorf("invalid marked legacy backtick closure")
		}
		previousCloseStart = span.closeStart
		markedByCloseStart[span.closeStart] = span
	}

	transformed := make([]byte, 0, root.closeOffset-root.openStart+1+len(marked)+2)
	sourceMap := islandSourceMap{
		originalOffsetByBoundary: []int{root.openStart},
		syntheticOffsets:         make(map[int]struct{}, len(marked)+2),
	}
	appendSynthetic := func(b byte, originalBoundary int) {
		offset := len(transformed)
		transformed = append(transformed, b)
		sourceMap.originalOffsetByBoundary = append(sourceMap.originalOffsetByBoundary, originalBoundary)
		sourceMap.syntheticOffsets[offset] = struct{}{}
		if inspect != nil {
			inspect()
		}
	}
	appendOriginal := func(originalOffset int) error {
		if sourceMap.originalOffsetByBoundary[len(sourceMap.originalOffsetByBoundary)-1] != originalOffset {
			return fmt.Errorf("non-monotonic source traversal at byte %d", originalOffset)
		}
		transformed = append(transformed, masked[originalOffset])
		sourceMap.originalOffsetByBoundary = append(sourceMap.originalOffsetByBoundary, originalOffset+1)
		if inspect != nil {
			inspect()
		}
		return nil
	}

	if root.doubleQuotedParent {
		appendSynthetic('"', root.openStart)
	}
	for originalOffset := root.openStart; originalOffset <= root.closeOffset; originalOffset++ {
		if _, ok := markedByCloseStart[originalOffset]; ok {
			appendSynthetic('\n', originalOffset)
		}
		if err := appendOriginal(originalOffset); err != nil {
			return nil, islandSourceMap{}, err
		}
	}
	if root.doubleQuotedParent {
		appendSynthetic('"', root.closeOffset+1)
	}
	if len(sourceMap.originalOffsetByBoundary) != len(transformed)+1 {
		return nil, islandSourceMap{}, fmt.Errorf("legacy backtick source-map width mismatch")
	}
	if err := validateIslandSourceMap(sourceMap, len(transformed), root); err != nil {
		return nil, islandSourceMap{}, err
	}
	return transformed, sourceMap, nil
}

func validateIslandSourceMap(
	sourceMap islandSourceMap,
	transformedLen int,
	root *legacyBacktickSpan,
) error {
	if root == nil || transformedLen < 0 ||
		len(sourceMap.originalOffsetByBoundary) != transformedLen+1 {
		return fmt.Errorf("invalid legacy backtick source map width")
	}
	for offset := range sourceMap.syntheticOffsets {
		if offset < 0 || offset >= transformedLen {
			return fmt.Errorf("synthetic offset %d is outside transformed island", offset)
		}
	}
	for offset := 0; offset < transformedLen; offset++ {
		start := sourceMap.originalOffsetByBoundary[offset]
		end := sourceMap.originalOffsetByBoundary[offset+1]
		if start < root.openStart || start > root.closeOffset+1 ||
			end < root.openStart || end > root.closeOffset+1 || end < start {
			return fmt.Errorf("non-monotonic legacy backtick source map at byte %d", offset)
		}
		_, synthetic := sourceMap.syntheticOffsets[offset]
		if synthetic {
			if end != start {
				return fmt.Errorf("synthetic byte %d advances original source", offset)
			}
			continue
		}
		if end != start+1 {
			return fmt.Errorf("original byte %d does not advance source exactly once", offset)
		}
	}
	return nil
}

func parseAndGraftLegacyBacktickIslands(
	tree *syntax.File,
	masked, original []byte,
	name string,
	islands []legacyBacktickIsland,
	parse func([]byte, string) (*syntax.File, error),
	probes ...legacyBacktickWorkProbe,
) error {
	if tree == nil || parse == nil || len(masked) != len(original) {
		return fmt.Errorf("invalid legacy backtick graft input")
	}
	if err := validateLegacyBacktickIslandOrder(islands, len(original)); err != nil {
		return err
	}
	// Index the placeholder substitutions once. Looking them up during each
	// island graft would rescan the full retry AST and make sibling islands
	// quadratic in the number of source statements.
	graftTargets := indexLegacyBacktickGraftTargets(tree)
	lineStarts := originalLineStarts(original)
	for _, island := range islands {
		transformed, sourceMap, err := buildLegacyBacktickIsland(masked, original, island, probes...)
		if err != nil {
			return err
		}
		islandTree, err := parse(transformed, name)
		if err != nil {
			return err
		}
		root := island.root
		openOffset, ok := transformedOffsetForOriginalByte(transformed, sourceMap, original, root.openOffset)
		if !ok {
			return fmt.Errorf("legacy backtick open at byte %d is not mapped", root.openOffset)
		}
		closeOffset, ok := transformedOffsetForOriginalByte(transformed, sourceMap, original, root.closeOffset)
		if !ok {
			return fmt.Errorf("legacy backtick close at byte %d is not mapped", root.closeOffset)
		}

		var selected []*syntax.CmdSubst
		syntax.Walk(islandTree, func(node syntax.Node) bool {
			substitution, ok := node.(*syntax.CmdSubst)
			if ok && substitution.Backquotes && int(substitution.Left.Offset()) == openOffset &&
				int(substitution.Right.Offset()) == closeOffset {
				selected = append(selected, substitution)
			}
			return true
		})
		if len(selected) != 1 {
			return fmt.Errorf(
				"legacy backtick island at byte %d matched %d parsed substitutions",
				root.openOffset,
				len(selected),
			)
		}
		if err := rejectSyntheticSemicolons(selected[0], sourceMap); err != nil {
			return err
		}
		if err := rebaseLegacyBacktickBody(selected[0], sourceMap, lineStarts, root); err != nil {
			return err
		}

		targets := graftTargets[legacyBacktickTargetKey{
			openOffset:  root.openOffset,
			closeOffset: root.closeOffset,
		}]
		if len(targets) != 1 {
			return fmt.Errorf(
				"legacy backtick island at byte %d matched %d graft targets",
				root.openOffset,
				len(targets),
			)
		}
		targets[0].Stmts = selected[0].Stmts
		targets[0].Last = selected[0].Last
	}
	return nil
}

func indexLegacyBacktickGraftTargets(
	tree *syntax.File,
	probes ...legacyBacktickTargetIndexProbe,
) map[legacyBacktickTargetKey][]*syntax.CmdSubst {
	var inspect legacyBacktickTargetIndexProbe
	if len(probes) > 0 {
		inspect = probes[0]
	}
	targets := make(map[legacyBacktickTargetKey][]*syntax.CmdSubst)
	if tree == nil {
		return targets
	}
	syntax.Walk(tree, func(node syntax.Node) bool {
		if inspect != nil {
			inspect()
		}
		substitution, ok := node.(*syntax.CmdSubst)
		if !ok || !substitution.Backquotes {
			return true
		}
		key := legacyBacktickTargetKey{
			openOffset:  int(substitution.Left.Offset()),
			closeOffset: int(substitution.Right.Offset()),
		}
		targets[key] = append(targets[key], substitution)
		return true
	})
	return targets
}

func validateLegacyBacktickIslandOrder(islands []legacyBacktickIsland, sourceLen int) error {
	previousClose := -1
	for _, island := range islands {
		root := island.root
		if root == nil || root.openStart <= previousClose || root.openOffset < root.openStart ||
			root.closeStart <= root.openOffset || root.closeOffset < root.closeStart ||
			root.closeOffset >= sourceLen {
			return fmt.Errorf("legacy backtick islands are not sorted and disjoint")
		}
		previousClose = root.closeOffset
	}
	return nil
}

func transformedOffsetForOriginalByte(
	transformed []byte,
	sourceMap islandSourceMap,
	original []byte,
	originalOffset int,
) (int, bool) {
	for offset := 0; offset < len(transformed); offset++ {
		if _, synthetic := sourceMap.syntheticOffsets[offset]; synthetic {
			continue
		}
		if sourceMap.originalOffsetByBoundary[offset] == originalOffset &&
			sourceMap.originalOffsetByBoundary[offset+1] == originalOffset+1 &&
			originalOffset >= 0 && originalOffset < len(original) &&
			transformed[offset] == original[originalOffset] {
			return offset, true
		}
	}
	return 0, false
}

func rejectSyntheticSemicolons(substitution *syntax.CmdSubst, sourceMap islandSourceMap) error {
	var found error
	syntax.Walk(substitution, func(node syntax.Node) bool {
		if found != nil {
			return false
		}
		stmt, ok := node.(*syntax.Stmt)
		if !ok || !stmt.Semicolon.IsValid() {
			return true
		}
		if _, synthetic := sourceMap.syntheticOffsets[int(stmt.Semicolon.Offset())]; synthetic {
			found = fmt.Errorf("synthetic terminator became a statement semicolon at byte %d", stmt.Semicolon.Offset())
			return false
		}
		return true
	})
	return found
}

func rebaseLegacyBacktickBody(
	substitution *syntax.CmdSubst,
	sourceMap islandSourceMap,
	lineStarts []int,
	root *legacyBacktickSpan,
) error {
	for _, stmt := range substitution.Stmts {
		if err := rebaseLegacyBacktickPositions(reflect.ValueOf(stmt), sourceMap, lineStarts, root); err != nil {
			return err
		}
	}
	if err := rebaseLegacyBacktickPositions(reflect.ValueOf(&substitution.Last), sourceMap, lineStarts, root); err != nil {
		return err
	}
	return nil
}

var syntaxPosType = reflect.TypeOf(syntax.Pos{})

func rebaseLegacyBacktickPositions(
	value reflect.Value,
	sourceMap islandSourceMap,
	lineStarts []int,
	root *legacyBacktickSpan,
) error {
	if !value.IsValid() {
		return nil
	}
	if value.Type() == syntaxPosType {
		if !value.CanSet() {
			return fmt.Errorf("syntax position is not settable")
		}
		position := value.Interface().(syntax.Pos)
		if !position.IsValid() {
			return nil
		}
		transformedOffset := int(position.Offset())
		if transformedOffset < 0 || transformedOffset >= len(sourceMap.originalOffsetByBoundary) {
			return fmt.Errorf("transformed position %d is outside island source map", transformedOffset)
		}
		originalOffset := sourceMap.originalOffsetByBoundary[transformedOffset]
		if originalOffset <= root.openOffset || originalOffset > root.closeOffset {
			return fmt.Errorf("mapped position %d is outside legacy backtick body", originalOffset)
		}
		lineIndex := sort.Search(len(lineStarts), func(index int) bool {
			return lineStarts[index] > originalOffset
		}) - 1
		if lineIndex < 0 {
			return fmt.Errorf("mapped position %d has no source line", originalOffset)
		}
		value.Set(reflect.ValueOf(syntax.NewPos(
			uint(originalOffset),
			uint(lineIndex+1),
			uint(originalOffset-lineStarts[lineIndex]+1),
		)))
		return nil
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return rebaseLegacyBacktickPositions(value.Elem(), sourceMap, lineStarts, root)
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return rebaseLegacyBacktickPositions(value.Elem(), sourceMap, lineStarts, root)
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if !field.CanSet() {
				continue
			}
			if err := rebaseLegacyBacktickPositions(field, sourceMap, lineStarts, root); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		for index := 0; index < value.Len(); index++ {
			if err := rebaseLegacyBacktickPositions(value.Index(index), sourceMap, lineStarts, root); err != nil {
				return err
			}
		}
	}
	return nil
}

func originalLineStarts(src []byte) []int {
	starts := []int{0}
	for offset, b := range src {
		if b == '\n' && offset+1 < len(src) {
			starts = append(starts, offset+1)
		}
	}
	return starts
}
