package wikidoc_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/wikidoc"
)

// ---- Inject tests ----

// TestInject_ReplacesContent verifies that Inject replaces the region between
// markers, preserves content outside the markers, and keeps the markers themselves.
func TestInject_ReplacesContent(t *testing.T) {
	const start = "{/* zsh-lint:generated:start */}"
	const end = "{/* zsh-lint:generated:end */}"
	mdx := "# Title\n\n" + start + "\nOLD\n" + end + "\n\nFooter"
	got, err := wikidoc.Inject(mdx, "NEW CONTENT", start, end)
	if err != nil {
		t.Fatalf("Inject returned unexpected error: %v", err)
	}
	if !strings.Contains(got, "# Title") {
		t.Errorf("Inject should preserve title; got:\n%s", got)
	}
	if !strings.Contains(got, "Footer") {
		t.Errorf("Inject should preserve footer; got:\n%s", got)
	}
	if strings.Contains(got, "OLD") {
		t.Errorf("Inject should replace OLD content; got:\n%s", got)
	}
	if !strings.Contains(got, "NEW CONTENT") {
		t.Errorf("Inject should insert new block; got:\n%s", got)
	}
	if !strings.Contains(got, start) {
		t.Errorf("Inject should preserve start marker; got:\n%s", got)
	}
	if !strings.Contains(got, end) {
		t.Errorf("Inject should preserve end marker; got:\n%s", got)
	}
}

// TestInject_MissingStartMarker verifies an error is returned when start marker is absent.
func TestInject_MissingStartMarker(t *testing.T) {
	const end = "{/* zsh-lint:generated:end */}"
	_, err := wikidoc.Inject("# Title\n"+end, "NEW", "MISSING_START", end)
	if err == nil {
		t.Fatal("Inject should return error when start marker is missing")
	}
	if !strings.HasPrefix(err.Error(), "wikidoc:") {
		t.Errorf("error should be prefixed 'wikidoc:'; got: %v", err)
	}
}

// TestInject_MissingEndMarker verifies an error is returned when end marker is absent.
func TestInject_MissingEndMarker(t *testing.T) {
	const start = "{/* zsh-lint:generated:start */}"
	_, err := wikidoc.Inject("# Title\n"+start, "NEW", start, "MISSING_END")
	if err == nil {
		t.Fatal("Inject should return error when end marker is missing")
	}
	if !strings.HasPrefix(err.Error(), "wikidoc:") {
		t.Errorf("error should be prefixed 'wikidoc:'; got: %v", err)
	}
}

// TestInject_EndBeforeStart verifies an error when endMarker appears only before
// startMarker (and not after) — there is no valid region to inject into.
func TestInject_EndBeforeStart(t *testing.T) {
	const start = "{/* zsh-lint:generated:start */}"
	const end = "{/* zsh-lint:generated:end */}"
	_, err := wikidoc.Inject(end+"\n"+start, "NEW", start, end)
	if err == nil {
		t.Fatal("Inject should return error when end marker precedes start marker")
	}
	if !strings.HasPrefix(err.Error(), "wikidoc:") {
		t.Errorf("error should be prefixed 'wikidoc:'; got: %v", err)
	}
}

// TestInject_EndMarkerTokenAlsoBeforeStart verifies that an occurrence of the
// end-marker token in prose before the real start marker does NOT confuse the
// search; the function must still find the real end marker after the start and
// inject correctly. Regression test for the bug Copilot flagged on PR #28.
func TestInject_EndMarkerTokenAlsoBeforeStart(t *testing.T) {
	const start = "{/* zsh-lint:generated:start */}"
	const end = "{/* zsh-lint:generated:end */}"
	// The end-marker token appears in narrative prose first (e.g. a doc page
	// describing the markers), then the real region follows.
	mdx := "Narrative mentions " + end + " as an example.\n\n" +
		"# Reference\n\n" + start + "\nOLD\n" + end + "\n\nfooter\n"
	out, err := wikidoc.Inject(mdx, "NEW", start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "OLD") {
		t.Fatalf("OLD must be replaced; got:\n%s", out)
	}
	if !strings.Contains(out, "NEW") {
		t.Fatalf("NEW must be present; got:\n%s", out)
	}
	// The earlier narrative occurrence of the end-marker token must be preserved.
	if !strings.Contains(out, "Narrative mentions "+end+" as an example.") {
		t.Fatalf("prose mention of the end-marker token must be preserved; got:\n%s", out)
	}
	if !strings.Contains(out, "footer") {
		t.Fatalf("content after the region must be preserved; got:\n%s", out)
	}
}

// TestInject_EmptyMarkers verifies that empty start/end markers are rejected
// rather than silently anchoring at index 0 (strings.Index(s, "") == 0).
func TestInject_EmptyMarkers(t *testing.T) {
	const start = "{/* zsh-lint:generated:start */}"
	const end = "{/* zsh-lint:generated:end */}"
	mdx := "# Title\n\n" + start + "\nOLD\n" + end + "\n"
	cases := []struct {
		name       string
		start, end string
	}{
		{"empty start", "", end},
		{"empty end", start, ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := wikidoc.Inject(mdx, "NEW", tc.start, tc.end)
			if err == nil {
				t.Fatal("Inject should return error for empty marker")
			}
			if !strings.HasPrefix(err.Error(), "wikidoc:") {
				t.Errorf("error should be prefixed 'wikidoc:'; got: %v", err)
			}
		})
	}
}

// TestSanitize_RemoveHTMLComment verifies that the gomarkdoc-generated header comment
// is stripped and surrounding content is preserved.
func TestSanitize_RemoveHTMLComment(t *testing.T) {
	input := "<!-- Code generated by gomarkdoc. DO NOT EDIT -->\n# Title"
	got := wikidoc.Sanitize(input)
	if strings.Contains(got, "<!--") {
		t.Errorf("Sanitize should remove HTML comments; got: %q", got)
	}
	if !strings.Contains(got, "# Title") {
		t.Errorf("Sanitize should preserve content after comment; got: %q", got)
	}
}

// TestSanitize_RemoveHTMLAnchor verifies that gomarkdoc anchor tags are removed.
func TestSanitize_RemoveHTMLAnchor(t *testing.T) {
	input := `<a name="Run"></a>

## func Run`
	got := wikidoc.Sanitize(input)
	if strings.Contains(got, "<a") {
		t.Errorf("Sanitize should remove HTML anchor tags; got: %q", got)
	}
	if !strings.Contains(got, "## func Run") {
		t.Errorf("Sanitize should preserve heading after anchor; got: %q", got)
	}
}

// TestSanitize_UnwrapAngleBracketLinks verifies angle-bracketed link destinations are unwrapped.
func TestSanitize_UnwrapAngleBracketLinks(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "fragment link",
			input: "[Run](<#Run>)",
			want:  "[Run](#Run)",
		},
		{
			name:  "full URL link",
			input: "[g](<https://h.example>)",
			want:  "[g](https://h.example)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wikidoc.Sanitize(tc.input)
			if got != tc.want {
				t.Errorf("Sanitize(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestSanitize_RewriteDocusaurusHeadingID verifies gomarkdoc fragment links still
// resolve after Docusaurus derives heading slugs from API declaration headings.
func TestSanitize_RewriteDocusaurusHeadingID(t *testing.T) {
	input := "[func Run](#Run)\n\n## func Run(names []string, w io.Writer) int"
	got := wikidoc.Sanitize(input)
	want := "[func Run](#func-run)\n\n##### func Run(names []string, w io.Writer) int"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_RewriteDocusaurusMethodHeadingID verifies receiver-method links
// target the slug Docusaurus derives from the corresponding generated heading.
func TestSanitize_RewriteDocusaurusMethodHeadingID(t *testing.T) {
	input := "[func \\(r Backquotes\\) Analyze\\(ctx \\*analyzer.Context\\)](#Backquotes.Analyze)\n\n" +
		"### func \\(Backquotes\\) Analyze"
	got := wikidoc.Sanitize(input)
	want := "[func \\(r Backquotes\\) Analyze\\(ctx \\*analyzer.Context\\)](#func-backquotes-analyze)\n\n" +
		"###### func \\(Backquotes\\) Analyze"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_EscapedInlineCode verifies gomarkdoc's plain-format escapes are
// removed only inside complete inline-code spans.
func TestSanitize_EscapedInlineCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "punctuation and entities",
			input: "ID: \\`style/backquotes\\`\n" +
				"Suppression: \\`\\# zsh\\-lint disable=style/backquotes \\-\\- \\&lt;reason\\&gt;\\`",
			want: "ID: `style/backquotes`\n" +
				"Suppression: `# zsh-lint disable=style/backquotes -- <reason>`",
		},
		{
			name:  "unmatched delimiter",
			input: "Prefix \\`unterminated",
			want:  "Prefix \\`unterminated",
		},
		{
			name:  "non punctuation backslashes",
			input: "Path: \\`C:\\temp\\name\\`",
			want:  "Path: `C:\\temp\\name`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wikidoc.Sanitize(tc.input)
			if got != tc.want {
				t.Errorf("Sanitize(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestSanitize_DemoteHeadings verifies generated headings nest beneath the
// wiki page's h3 Reference section without producing invalid h7 headings.
func TestSanitize_DemoteHeadings(t *testing.T) {
	input := "# package\n\n## Index\n\n#### Deep"
	got := wikidoc.Sanitize(input)
	want := "#### package\n\n##### Index\n\n###### Deep"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_FenceIndentedCode verifies gomarkdoc's tab-indented code is
// converted to fenced code that MDX both parses and renders as code.
func TestSanitize_FenceIndentedCode(t *testing.T) {
	input := "\tfunc render() { print ok; }\n\t  print done\n\t\n\nProse"
	got := wikidoc.Sanitize(input)
	want := "```go\nfunc render() { print ok; }\n  print done\n```\n\nProse"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_FenceIndentedCodeLanguages verifies generated Go declarations
// and Zsh rule examples receive the explicit languages required by the wiki.
func TestSanitize_FenceIndentedCodeLanguages(t *testing.T) {
	input := "## func Run\n\n\tfunc Run() int\n\n" +
		"Bad:\n\n\tprint -r -- $value\n\n" +
		"Good:\n\n\tprint -r -- \"$value\"\n\n" +
		"Good \\(configured sourced entrypoint\\):\n\n\t() { print -r -- \"$1\" } \"$value\"\n\n" +
		"Bad (unconfigured script):\n\n\t0=$PWD/script.zsh"
	got := wikidoc.Sanitize(input)
	want := "##### func Run\n\n```go\nfunc Run() int\n```\n\n" +
		"Bad:\n\n```zsh\nprint -r -- $value\n```\n\n" +
		"Good:\n\n```zsh\nprint -r -- \"$value\"\n```\n\n" +
		"Good \\(configured sourced entrypoint\\):\n\n```zsh\n" +
		"() { print -r -- \"$1\" } \"$value\"\n```\n\n" +
		"Bad (unconfigured script):\n\n```zsh\n0=$PWD/script.zsh\n```"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_WhitespaceOnlyIndent verifies a tab-only separator is cleaned
// without producing an empty fenced block.
func TestSanitize_WhitespaceOnlyIndent(t *testing.T) {
	input := "\t\n\nProse"
	got := wikidoc.Sanitize(input)
	want := "\n\nProse"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_EscapeProseChars verifies bare < > { } are escaped on prose lines.
func TestSanitize_EscapeProseChars(t *testing.T) {
	input := "usage <file.zsh> {x}"
	got := wikidoc.Sanitize(input)
	want := "usage &lt;file.zsh&gt; &#123;x&#125;"
	if got != want {
		t.Errorf("Sanitize(%q) = %q; want %q", input, got, want)
	}
}

// TestSanitize_IndentedCodePreservesContent verifies fenced conversion leaves
// code contents untouched.
func TestSanitize_IndentedCodePreservesContent(t *testing.T) {
	input := "\tfunc F(a <T>) {}"
	got := wikidoc.Sanitize(input)
	want := "```go\nfunc F(a <T>) {}\n```"
	if got != want {
		t.Errorf("Sanitize should fence tab-indented code without escaping it; got: %q, want: %q", got, want)
	}
}

// TestSanitize_FencedCodeNotEscaped verifies content inside fenced blocks is left verbatim.
func TestSanitize_FencedCodeNotEscaped(t *testing.T) {
	input := "```\n<y> {z}\n```"
	got := wikidoc.Sanitize(input)
	if got != input {
		t.Errorf("Sanitize should not escape fenced code block contents; got: %q, want: %q", got, input)
	}
}

// TestSanitize_FourSpaceIndentFenced verifies existing Markdown-indented code
// is converted to MDX-compatible fenced code.
func TestSanitize_FourSpaceIndentFenced(t *testing.T) {
	input := "    func F(a <T>) {}"
	got := wikidoc.Sanitize(input)
	want := "```go\nfunc F(a <T>) {}\n```"
	if got != want {
		t.Errorf("Sanitize should fence 4-space-indented code; got: %q, want: %q", got, want)
	}
}

func TestRuleReferenceProducesUserFacingRules(t *testing.T) {
	const raw = "<!-- Code generated by gomarkdoc. DO NOT EDIT -->\n" +
		"# rules\n\n" +
		"\timport \"github.com/z-shell/zsh-lint/internal/rules\"\n\n" +
		"## Index\n\n" +
		"- [type Example](<#Example>)\n\n" +
		"## type Example\n\n" +
		"Example is an internal Go type.\n\n" +
		"ID: \\`style/example\\`\n\n" +
		"Name: Prefer the example\\-form\n\n" +
		"Summary: Reports the \\[old\\] spelling.\n\n" +
		"Bad:\n\n" +
		"\tprint old\n\n" +
		"Good:\n\n" +
		"\tprint new\n\n" +
		"Severity: Hint.\n\n" +
		"Suppression: Use \\`\\# zsh\\-lint disable=style/example \\-\\- \\<reason\\>\\`.\n\n" +
		"\ttype Example struct{}\n\n" +
		"### func \\(Example\\) Analyze\n\n" +
		"\tfunc (Example) Analyze()\n\n" +
		"## type Profile\n\n" +
		"Profile is not a lint rule.\n"
	const commit = "0123456789abcdef0123456789abcdef01234567"
	got, err := wikidoc.RuleReference(raw, "v1.2.3", commit)
	if err != nil {
		t.Fatalf("RuleReference returned unexpected error: %v", err)
	}
	for _, want := range []string{
		"generated from the published `v1.2.3` release",
		commit,
		"[`style/example`](#rule-styleexample)",
		"## Rule: `style/example`",
		"**Name:** Prefer the example-form",
		"**Summary:** Reports the \\[old\\] spelling.",
		"```zsh\nprint old\n```",
		"`# zsh-lint disable=style/example -- <reason>`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RuleReference output is missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"internal Go type",
		"import \"github.com",
		"type Example struct",
		"func (Example) Analyze",
		"Profile is not a lint rule",
		`example\-form`,
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("RuleReference output contains internal API text %q:\n%s", unwanted, got)
		}
	}
}

func TestRuleReferenceRejectsInvalidMetadataAndEmptyRules(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name    string
		raw     string
		version string
		commit  string
	}{
		{name: "invalid version", raw: "## type Example\n\nID: \\`style/example\\`\n", version: "v1.2.3 unsafe", commit: commit},
		{name: "invalid commit", raw: "## type Example\n\nID: \\`style/example\\`\n", version: "v1.2.3", commit: "abc"},
		{name: "no documented rules", raw: "# rules\n\n## type Profile\n\nNo ID.\n", version: "v1.2.3", commit: commit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := wikidoc.RuleReference(test.raw, test.version, test.commit); err == nil {
				t.Fatal("RuleReference succeeded, want error")
			}
		})
	}
}
