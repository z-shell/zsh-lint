// Package wikidoc transforms gomarkdoc-generated Markdown into MDX-safe content
// and injects it into a marked region of a Docusaurus .mdx page. It is dev/CI
// tooling, not product code.
package wikidoc

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

var (
	reHTMLComment      = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHTMLAnchor       = regexp.MustCompile(`</?a\b[^>]*>`)
	reAngleLink        = regexp.MustCompile(`\]\(<([^>\s]*)>\)`)
	reHeading          = regexp.MustCompile(`(?m)^(#{1,6})([ \t]+)`)
	reAPIHeading       = regexp.MustCompile(`(?m)^#{2,} (func|type|var|const) ([A-Za-z_][A-Za-z0-9_]*)\b.*$`)
	reAPIMethodHeading = regexp.MustCompile(
		`(?m)^#{2,} func \\\(([A-Za-z_][A-Za-z0-9_]*)\\\) ([A-Za-z_][A-Za-z0-9_]*)\b.*$`,
	)
	reRuleTypeHeading = regexp.MustCompile(`(?m)^## type ([A-Za-z_][A-Za-z0-9_]*)[^\n]*$`)
	reSourceVersion   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	reSourceCommit    = regexp.MustCompile(`^[0-9A-Fa-f]{40}$`)
)

type ruleReferenceEntry struct {
	id   string
	name string
	body string
}

// RuleReference reduces gomarkdoc's internal Go API reference to a user-facing
// list of documented lint rules. sourceVersion and sourceCommit identify the
// published source snapshot used to generate raw.
func RuleReference(raw, sourceVersion, sourceCommit string) (string, error) {
	if !reSourceVersion.MatchString(sourceVersion) {
		return "", fmt.Errorf("wikidoc: invalid source version %q", sourceVersion)
	}
	if !reSourceCommit.MatchString(sourceCommit) {
		return "", fmt.Errorf("wikidoc: invalid source commit %q", sourceCommit)
	}

	matches := reRuleTypeHeading.FindAllStringSubmatchIndex(raw, -1)
	entries := make([]ruleReferenceEntry, 0, len(matches))
	seenIDs := make(map[string]struct{}, len(matches))
	seenAnchors := make(map[string]struct{}, len(matches))
	for i, match := range matches {
		sectionEnd := len(raw)
		if i+1 < len(matches) {
			sectionEnd = matches[i+1][0]
		}
		typeName := raw[match[2]:match[3]]
		section := raw[match[1]:sectionEnd]
		entry, ok := parseRuleReferenceEntry(section, typeName)
		if !ok {
			continue
		}
		if _, exists := seenIDs[entry.id]; exists {
			return "", fmt.Errorf("wikidoc: duplicate rule ID %q", entry.id)
		}
		anchor := ruleAnchor(entry.id)
		if _, exists := seenAnchors[anchor]; exists {
			return "", fmt.Errorf("wikidoc: rule IDs produce duplicate anchor %q", anchor)
		}
		seenIDs[entry.id] = struct{}{}
		seenAnchors[anchor] = struct{}{}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("wikidoc: no documented lint rules found")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })

	var out strings.Builder
	fmt.Fprintf(&out, ":::info Published rule set\n")
	fmt.Fprintf(
		&out,
		"This reference was generated from the published `%s` release at commit [`%s`](https://github.com/z-shell/zsh-lint/commit/%s). It matches the rules users receive from `go install github.com/z-shell/zsh-lint/cmd/zsh-lint@%s`.\n",
		sourceVersion,
		sourceCommit,
		sourceCommit,
		sourceVersion,
	)
	out.WriteString(":::\n\n## Rules\n\n")
	for _, entry := range entries {
		fmt.Fprintf(&out, "- [`%s`](#%s)", entry.id, ruleAnchor(entry.id))
		if entry.name != "" {
			fmt.Fprintf(&out, ": %s", entry.name)
		}
		out.WriteByte('\n')
	}
	for _, entry := range entries {
		fmt.Fprintf(&out, "\n## Rule: `%s`\n\n%s\n", entry.id, entry.body)
	}

	return strings.TrimSpace(out.String()), nil
}

func parseRuleReferenceEntry(section, typeName string) (ruleReferenceEntry, bool) {
	section = cutRuleTypeDeclaration(section, typeName)
	lines := strings.Split(section, "\n")
	idLine := -1
	id := ""
	for i, line := range lines {
		const prefix = "ID: \\`"
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, "\\`") {
			continue
		}
		id = unescapeMarkdownPunctuation(strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), "\\`"))
		idLine = i
		break
	}
	if idLine < 0 || id == "" {
		return ruleReferenceEntry{}, false
	}

	body := strings.TrimSpace(Sanitize(strings.Join(lines[idLine+1:], "\n")))
	body = normalizeRuleProseEscapes(body)
	body = emphasizeRuleMetadata(body)
	return ruleReferenceEntry{
		id:   id,
		name: ruleMetadataValue(body, "Name"),
		body: body,
	}, true
}

func normalizeRuleProseEscapes(body string) string {
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			lines[i] = unescapeRulePunctuation(line)
		}
	}
	return strings.Join(lines, "\n")
}

func unescapeRulePunctuation(line string) string {
	var out strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) && isASCIIPunctuation(line[i+1]) &&
			line[i+1] != '[' && line[i+1] != ']' {
			i++
		}
		out.WriteByte(line[i])
	}
	return out.String()
}

func cutRuleTypeDeclaration(section, typeName string) string {
	cut := len(section)
	for _, declaration := range []string{
		"\n\ttype " + typeName,
		"\n    type " + typeName,
	} {
		if index := strings.Index(section, declaration); index >= 0 && index < cut {
			cut = index
		}
	}
	return section[:cut]
}

func emphasizeRuleMetadata(body string) string {
	labels := []string{"Name", "Summary", "Bad", "Good", "Severity", "Suppression"}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		for _, label := range labels {
			prefix := label + ":"
			if strings.HasPrefix(line, prefix) {
				lines[i] = "**" + prefix + "**" + strings.TrimPrefix(line, prefix)
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func ruleMetadataValue(body, label string) string {
	prefix := "**" + label + ":** "
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func ruleAnchor(id string) string {
	var out strings.Builder
	out.WriteString("rule-")
	for _, r := range strings.ToLower(id) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// Sanitize transforms a gomarkdoc Markdown string into MDX-safe content by
// applying the following transformations in order:
//
//  1. Remove HTML comments (gomarkdoc header, etc.).
//  2. Remove HTML anchor tags. gomarkdoc emits named anchors like
//     <a name="..."></a>; any raw anchor tag is MDX-hostile, so all opening
//     and closing <a> tags are stripped (inner text, if any, is preserved).
//  3. Unwrap angle-bracketed link destinations (](<#Run>) → ](#Run)).
//  4. Normalize leading tabs to four spaces for Markdown indentation.
//  5. Convert indented code blocks to language-tagged fenced blocks that MDX
//     parses as code and the wiki accepts.
//  6. Escape bare <, >, {, } on prose lines (not inside fenced code).
//  7. Restore gomarkdoc's escaped inline-code spans, removing Markdown
//     punctuation escapes and decoding entities only inside paired spans.
//  8. Demote generated headings by three levels so they nest under the wiki
//     page's "### Reference" section.
//  9. Rewrite Go declaration fragment links to Docusaurus slugs, so gomarkdoc
//     fragments like #Run and #Backquotes.Analyze resolve to their headings.
func Sanitize(md string) string {
	// Step 1: remove HTML comments.
	out := reHTMLComment.ReplaceAllString(md, "")

	// Step 2: remove HTML anchor tags.
	out = reHTMLAnchor.ReplaceAllString(out, "")

	// Step 3: unwrap angle-bracketed link destinations.
	out = reAngleLink.ReplaceAllString(out, "]($1)")

	// Step 4: use spaces for Markdown indentation and repository whitespace.
	out = normalizeIndent(out)

	// Step 5: MDX does not recognize Markdown-indented code blocks. Fence them
	// so braces in generated examples are not parsed as JSX expressions.
	out = fenceIndentedCodeBlocks(out)

	// Step 6: escape bare MDX special chars on prose lines only.
	out = escapeProse(out)

	// Step 7: restore inline code after prose escaping so code contents remain
	// literal, including angle brackets and braces.
	out = normalizeEscapedInlineCode(out)

	// Step 8: nest generated headings under the page's Reference section.
	out = demoteHeadings(out)

	// Step 9: target the slugs Docusaurus derives from declaration headings.
	out = rewriteAPIFragmentLinks(out)

	return out
}

// normalizeIndent converts leading tabs to four spaces so generated Markdown
// stays compatible with the wiki repository's whitespace policy.
func normalizeIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		tabs := len(line) - len(strings.TrimLeft(line, "\t"))
		if tabs > 0 {
			lines[i] = strings.Repeat("    ", tabs) + line[tabs:]
		}
	}
	return strings.Join(lines, "\n")
}

// fenceIndentedCodeBlocks converts Markdown-indented code into fenced blocks.
// MDX v3 treats four-space indentation as ordinary text, so raw braces in such
// blocks are otherwise parsed as expressions instead of rendered as code.
func fenceIndentedCodeBlocks(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inFenced := false

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFenced = !inFenced
			out = append(out, lines[i])
			i++
			continue
		}
		if inFenced || !strings.HasPrefix(lines[i], "    ") {
			out = append(out, lines[i])
			i++
			continue
		}

		blockStart := i
		block := make([]string, 0, 1)
		for i < len(lines) && strings.HasPrefix(lines[i], "    ") {
			block = append(block, strings.TrimPrefix(lines[i], "    "))
			i++
		}
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}
		if len(block) == 0 {
			out = append(out, "")
			continue
		}

		out = append(out, "```"+indentedCodeLanguage(lines, blockStart))
		out = append(out, block...)
		out = append(out, "```")
	}

	return strings.Join(out, "\n")
}

// indentedCodeLanguage classifies gomarkdoc's generated blocks. Rule examples
// immediately follow Bad:/Good: labels, optionally with a parenthetical
// qualifier, and contain Zsh. Package imports and Go declarations use Go,
// which is also the safe default for new gomarkdoc sections that do not use the
// rule-example schema.
func indentedCodeLanguage(lines []string, blockStart int) string {
	for i := blockStart - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			continue
		case isRuleExampleLabel(line):
			return "zsh"
		default:
			return "go"
		}
	}
	return "go"
}

func isRuleExampleLabel(line string) bool {
	for _, label := range []string{"Bad", "Good"} {
		if line == label+":" {
			return true
		}

		detail, found := strings.CutPrefix(line, label+" ")
		if !found || !strings.HasSuffix(detail, ":") {
			continue
		}
		detail = strings.TrimSuffix(detail, ":")
		if strings.HasPrefix(detail, "(") && strings.HasSuffix(detail, ")") {
			return true
		}
		if strings.HasPrefix(detail, `\(`) && strings.HasSuffix(detail, `\)`) {
			return true
		}
	}
	return false
}

// demoteHeadings shifts generated Markdown headings down three levels. Markdown
// has only six heading levels, so deeper input is clamped at h6.
func demoteHeadings(s string) string {
	return reHeading.ReplaceAllStringFunc(s, func(heading string) string {
		hashes := strings.IndexAny(heading, " \t")
		level := min(hashes+3, 6)
		return strings.Repeat("#", level) + heading[hashes:]
	})
}

// rewriteAPIFragmentLinks rewrites gomarkdoc declaration links to the slugs
// Docusaurus derives from headings such as "## func Run" and
// "### func \(Backquotes\) Analyze".
func rewriteAPIFragmentLinks(s string) string {
	for _, match := range reAPIHeading.FindAllStringSubmatch(s, -1) {
		name := match[2]
		slug := strings.ToLower(match[1] + "-" + name)
		s = strings.ReplaceAll(s, "](#"+name+")", "](#"+slug+")")
	}
	for _, match := range reAPIMethodHeading.FindAllStringSubmatch(s, -1) {
		receiver := match[1]
		method := match[2]
		slug := strings.ToLower("func-" + receiver + "-" + method)
		s = strings.ReplaceAll(s, "](#"+receiver+"."+method+")", "](#"+slug+")")
	}
	return s
}

// normalizeEscapedInlineCode restores paired inline-code spans emitted by
// gomarkdoc's plain formatter. Unmatched delimiters are left unchanged.
func normalizeEscapedInlineCode(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = normalizeEscapedInlineCodeLine(line)
	}
	return strings.Join(lines, "\n")
}

func normalizeEscapedInlineCodeLine(line string) string {
	const delimiter = "\\`"

	var out strings.Builder
	for {
		start := strings.Index(line, delimiter)
		if start < 0 {
			out.WriteString(line)
			break
		}
		afterStart := start + len(delimiter)
		endOffset := strings.Index(line[afterStart:], delimiter)
		if endOffset < 0 {
			out.WriteString(line)
			break
		}
		end := afterStart + endOffset

		out.WriteString(line[:start])
		out.WriteByte('`')
		out.WriteString(html.UnescapeString(unescapeMarkdownPunctuation(line[afterStart:end])))
		out.WriteByte('`')
		line = line[end+len(delimiter):]
	}
	return out.String()
}

func unescapeMarkdownPunctuation(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isASCIIPunctuation(s[i+1]) {
			i++
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func isASCIIPunctuation(b byte) bool {
	return (b >= '!' && b <= '/') ||
		(b >= ':' && b <= '@') ||
		(b >= '[' && b <= '`') ||
		(b >= '{' && b <= '~')
}

// escapeProse escapes bare <, >, {, } on non-code lines. Fenced code content
// is left verbatim.
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
// themselves are preserved. The end marker is searched for only after the start
// marker, so an unrelated occurrence of the end-marker token earlier in the
// document is ignored. Returns an error (prefixed "wikidoc:") if either marker
// is empty, if the start marker is missing, or if no end marker is found after
// the start marker.
//
// Empty markers are rejected explicitly: strings.Index(s, "") returns 0, so an
// empty marker would otherwise silently anchor at the start of the document and
// corrupt the target file rather than failing loudly.
func Inject(mdx, block, startMarker, endMarker string) (string, error) {
	if startMarker == "" || endMarker == "" {
		return "", fmt.Errorf("wikidoc: start and end markers must be non-empty")
	}
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
