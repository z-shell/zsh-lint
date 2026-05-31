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

// TestSanitize_NormalizeIndent verifies generated Markdown code indentation
// uses spaces so wiki whitespace checks accept newly generated lines.
func TestSanitize_NormalizeIndent(t *testing.T) {
	input := "\timport \"example.test/pkg\"\n\t\tdeep"
	got := wikidoc.Sanitize(input)
	want := "    import \"example.test/pkg\"\n        deep"
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

// TestSanitize_IndentedCodeNotEscaped verifies tab-indented code lines are
// normalized to spaces without escaping their code contents.
func TestSanitize_IndentedCodeNotEscaped(t *testing.T) {
	input := "\tfunc F(a <T>) {}"
	got := wikidoc.Sanitize(input)
	want := "    func F(a <T>) {}"
	if got != want {
		t.Errorf("Sanitize should normalize but not escape tab-indented code; got: %q, want: %q", got, want)
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

// TestSanitize_FourSpaceIndentNotEscaped verifies 4-space-indented code lines are left verbatim.
func TestSanitize_FourSpaceIndentNotEscaped(t *testing.T) {
	input := "    func F(a <T>) {}"
	got := wikidoc.Sanitize(input)
	if got != input {
		t.Errorf("Sanitize should not escape 4-space-indented code; got: %q, want: %q", got, input)
	}
}
