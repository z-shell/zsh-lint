package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestRecallOnlyDeclarationPreservesLaterIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		rule   analyzer.Rule
		source string
		marker string
	}{
		{"valid local registry", SharedPluginsRegistry{}, "example_f() { builtin local Plugins=inner; Plugins=changed; }\n", "Plugins=changed"},
		{"underfull tie registry", SharedPluginsRegistry{}, "example_f() { builtin typeset -T Plugins; Plugins=changed; }\n", "Plugins=changed"},
		{"overfull tie registry", SharedPluginsRegistry{}, "example_f() { builtin typeset -T scalar Plugins : extra; Plugins=changed; }\n", "Plugins=changed"},
		{"invalid option registry", SharedPluginsRegistry{}, "example_f() { builtin typeset -Q Plugins; Plugins=changed; }\n", "Plugins=changed"},
		{"overfull tie path", FpathHygiene{}, "example_f() { builtin typeset -T scalar FPATH : extra; FPATH=/changed; }\n", "FPATH=/changed"},
		{"invalid option path", FpathHygiene{}, "example_f() { builtin typeset -Q FPATH; FPATH=/changed; }\n", "FPATH=/changed"},
		{"static zero declaration", ZeroHandling{}, "builtin typeset 0=changed\nprint -r -- $0\n", "$0"},
		{"existing command prefix", ZeroHandling{}, "0=changed builtin typeset value=1\n", "0=changed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), "example.zsh")
			if err != nil {
				t.Fatal(err)
			}
			context := sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example")
			if _, ok := test.rule.(FpathHygiene); ok {
				context = projectconfig.SourceContext{}
			}
			got := analyzer.New(test.rule).AnalyzeSource(file, "example.zsh", context)
			start := strings.Index(test.source, test.marker)
			found := false
			for _, d := range got {
				if d.Range.Start.Offset == start {
					found = true
				}
			}
			if !found {
				t.Fatalf("later finding at byte %d missing: %+v", start, got)
			}
		})
	}
}
