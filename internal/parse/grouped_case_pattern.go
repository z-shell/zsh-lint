package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

const invalidGroupedCasePatternCloser = "`)` can only be used to close a subshell"

// parseGroupedCasePattern retries only the documented Zsh case-arm grouping
// boundary. The scanner masks structural grouping bytes, never the final
// case-arm terminator, then restores the original AST literals byte for byte.
func parseGroupedCasePattern(src []byte, name string, firstErr error) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidGroupedCasePatternCloser {
		return nil, firstErr
	}

	scan, ok := scanConditionalPatterns(src, int(parseErr.Pos.Offset()), nil)
	if !ok {
		return nil, firstErr
	}
	var edits []patternEdit
	for _, candidate := range scan.groupedCasePatterns {
		if candidate.seed {
			edits = append(edits, candidate.edits...)
		}
	}
	if len(edits) == 0 {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	for _, edit := range edits {
		if edit.offset < 0 || edit.offset >= len(masked) || masked[edit.offset] != edit.original {
			return nil, firstErr
		}
		masked[edit.offset] = edit.replacement
	}
	tree, err := retryExcluding(parseGroupedCasePattern)(masked, name)
	if err != nil {
		return nil, err
	}
	if err := restorePatternEdits(tree, src, edits); err != nil {
		return nil, err
	}
	return tree, nil
}
