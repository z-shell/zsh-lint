// Package wikidoc transforms gomarkdoc-generated Markdown into MDX-safe content
// and injects it into a marked region of a Docusaurus .mdx page. It is dev/CI
// tooling, not product code.
//
// # Known limitations
//
// Step 4 (bare-character escaping) operates line-by-line and does NOT exempt
// inline code spans (single-backtick runs) on prose lines. Characters inside
// `backtick spans` on prose lines will be escaped along with the surrounding
// text. This is acceptable for the current gomarkdoc output, which uses only
// indented blocks and fenced blocks for code samples, not inline spans
// containing angle brackets or braces.
package wikidoc

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHTMLAnchor  = regexp.MustCompile(`</?a\b[^>]*>`)
	reAngleLink   = regexp.MustCompile(`\]\(<([^>\s]*)>\)`)
)

// Sanitize transforms a gomarkdoc Markdown string into MDX-safe content by
// applying the following transformations in order:
//
//  1. Remove HTML comments (gomarkdoc header, etc.).
//  2. Remove HTML anchor tags (<a name="..."></a>).
//  3. Unwrap angle-bracketed link destinations (](<#Run>) → ](#Run)).
//  4. Escape bare <, >, {, } on prose lines (not inside fenced or indented code).
func Sanitize(md string) string {
	// Step 1: remove HTML comments.
	out := reHTMLComment.ReplaceAllString(md, "")

	// Step 2: remove HTML anchor tags.
	out = reHTMLAnchor.ReplaceAllString(out, "")

	// Step 3: unwrap angle-bracketed link destinations.
	out = reAngleLink.ReplaceAllString(out, "]($1)")

	// Step 4: escape bare MDX special chars on prose lines only.
	out = escapeProse(out)

	return out
}

// escapeProse escapes bare <, >, {, } on non-code lines.
// Code lines are:
//   - lines inside a fenced block (toggled by lines whose trimmed text starts
//     with three backticks), or
//   - indented lines (start with a tab or 4+ spaces).
func escapeProse(s string) string {
	lines := strings.Split(s, "\n")
	inFenced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			// Toggle fenced state; the fence line itself is not code content
			// that needs escaping — it's a delimiter. Leave it unchanged.
			inFenced = !inFenced
			continue
		}
		if inFenced {
			continue
		}
		// Indented code line: starts with a tab or 4+ spaces.
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			continue
		}
		// Prose line — escape MDX special characters.
		lines[i] = escapeLine(line)
	}
	return strings.Join(lines, "\n")
}

// escapeLine replaces bare MDX-hazardous characters on a single prose line.
func escapeLine(line string) string {
	line = strings.ReplaceAll(line, "<", "&lt;")
	line = strings.ReplaceAll(line, ">", "&gt;")
	line = strings.ReplaceAll(line, "{", "&#123;")
	line = strings.ReplaceAll(line, "}", "&#125;")
	return line
}

// Inject replaces the content between startMarker and endMarker in mdx with
// block, surrounded by blank lines, and returns the result. The markers
// themselves are preserved. Returns an error (prefixed "wikidoc:") if either
// marker is missing or if endMarker appears before startMarker.
func Inject(mdx, block, startMarker, endMarker string) (string, error) {
	startIdx := strings.Index(mdx, startMarker)
	if startIdx < 0 {
		return "", fmt.Errorf("wikidoc: start marker %q not found", startMarker)
	}
	afterStart := startIdx + len(startMarker)
	// Search for the end marker only AFTER the start marker so an unrelated
	// occurrence of the end-marker token earlier in the document (e.g. quoted
	// in narrative prose) does not cause a false "missing end marker" error.
	relEnd := strings.Index(mdx[afterStart:], endMarker)
	if relEnd < 0 {
		return "", fmt.Errorf("wikidoc: end marker %q not found after start marker", endMarker)
	}
	endIdx := afterStart + relEnd
	result := mdx[:afterStart] +
		"\n\n" +
		strings.TrimRight(block, "\n") +
		"\n\n" +
		mdx[endIdx:]
	return result, nil
}
