package workflowcontract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func repositoryFilePath(t *testing.T, parts ...string) string {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate workflow contract test")
	}

	rootParts := []string{filepath.Dir(testFile), "..", ".."}
	return filepath.Clean(filepath.Join(append(rootParts, parts...)...))
}

func readRepositoryFile(t *testing.T, parts ...string) string {
	t.Helper()

	path := repositoryFilePath(t, parts...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func zshSyntaxWorkflow(t *testing.T) string {
	t.Helper()
	return readRepositoryFile(t, ".github", "workflows", "zsh-n.yml")
}

func workflowRunScript(t *testing.T, step string) string {
	t.Helper()

	runBlock := workflowBlock(t, step, "run: |", 8)
	const header = "        run: |\n"
	if !strings.HasPrefix(runBlock, header) {
		t.Fatalf("zcompile run block must begin with %q; got %q", header, runBlock)
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimPrefix(runBlock, header), "\n") {
		if line == "" {
			lines = append(lines, "")
			continue
		}
		if !strings.HasPrefix(line, "          ") {
			t.Fatalf("zcompile command must use YAML block indentation; got %q", line)
		}
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	return strings.Join(lines, "\n")
}

func workflowSequenceItems(block string, indent int) []string {
	prefix := strings.Repeat(" ", indent) + "- "
	var starts []workflowLineSpan
	for _, line := range workflowLines(block) {
		if strings.HasPrefix(line.text, prefix) {
			starts = append(starts, line)
		}
	}

	items := make([]string, 0, len(starts))
	for index, start := range starts {
		end := len(block)
		if index+1 < len(starts) {
			end = starts[index+1].start
		}
		items = append(items, block[start.start:end])
	}
	return items
}

func workflowJobSteps(t *testing.T, workflow, jobName string) []string {
	t.Helper()

	jobBlock := workflowBlock(t, workflow, jobName+":", 2)
	stepsBlock := workflowBlock(t, jobBlock, "steps:", 4)
	return workflowSequenceItems(stepsBlock, 6)
}

func workflowJobStep(t *testing.T, workflow, jobName, stepName string) string {
	t.Helper()

	want := "      - name: " + stepName
	var matches []string
	for _, step := range workflowJobSteps(t, workflow, jobName) {
		lines := workflowLines(step)
		if len(lines) > 0 && lines[0].text == want {
			matches = append(matches, step)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("job %q must contain step %q exactly once; got %d", jobName, stepName, len(matches))
	}
	return matches[0]
}

func zshSyntaxScript(t *testing.T, workflow string) string {
	t.Helper()

	step := workflowJobStep(t, workflow, "zsh-n", `⚡ zsh -n`)
	return workflowRunScript(t, step)
}

func zshCompileScript(t *testing.T, workflow string) string {
	t.Helper()

	step := workflowJobStep(t, workflow, "zsh-n", `⚡ zcompile`)
	return workflowRunScript(t, step)
}

// Filenames reach the check as NUL-delimited records rather than as words a
// shell splits. The job iterates the files itself instead of fanning out a
// matrix leg per file, so the NUL-safe boundary is `find -print0` feeding a
// `read -d ”` loop rather than a jq transport into a matrix; a filename
// holding a space, a newline or a glob character must still arrive as one
// record. TestZshSyntaxPreservesNewlineFilename proves the behavior.
func TestZshSyntaxUsesNULDelimitedFilenames(t *testing.T) {
	workflow := zshSyntaxWorkflow(t)
	for _, script := range []struct {
		name string
		text string
	}{
		{name: "zsh -n", text: zshSyntaxScript(t, workflow)},
		{name: "zcompile", text: zshCompileScript(t, workflow)},
	} {
		for _, required := range []string{
			"-print0",
			`read -r -d ''`,
			`IFS=`,
		} {
			if !strings.Contains(script.text, required) {
				t.Errorf("%s step is missing NUL-safe iteration fragment %q", script.name, required)
			}
		}
	}
}

// A filename holding a newline must be checked as one file. The former matrix
// carried filenames through JSON, so the risk was a transport splitting them;
// a single job iterates them directly, so the risk is word splitting in the
// loop. Either way the guarantee is the same and is asserted by running the
// real script: every `.zsh` file is visited exactly once, selected by
// extension, and a newline in a name does not become two files.
func TestZshSyntaxPreservesNewlineFilename(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is required for the workflow behavior test")
	}

	// Extensionless function files and the JSON fixture are on disk to prove
	// the check selects by the .zsh extension only.
	dir := t.TempDir()
	for _, subdir := range []string{"legacy/functions", "examples/plugin"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0o700); err != nil {
			t.Fatalf("create fixture directory %q: %v", subdir, err)
		}
	}
	for _, name := range []string{
		"line\nbreak.zsh",
		"ordinary.zsh",
		"legacy/functions/zsh-lint",
		"legacy/functions/.zsh-lint-worker",
		"legacy/functions/@zsh-lint-process-buffer",
		"examples/plugin/zsh-lint.json",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("true\n"), 0o600); err != nil {
			t.Fatalf("write fixture %q: %v", name, err)
		}
	}

	command := exec.Command(
		bash,
		"--noprofile",
		"--norc",
		"-c",
		zshSyntaxScript(t, zshSyntaxWorkflow(t)),
	)
	command.Dir = dir
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		t.Fatalf("zsh -n workflow command failed: %v\n%s", runErr, output)
	}

	// Exactly the two .zsh files, so a newline did not split one into two and
	// the extensionless function files were not picked up.
	if want := "Checked 2 file(s) with zsh -n, 0 failed."; !strings.Contains(string(output), want) {
		t.Fatalf("zsh -n step must visit each .zsh file exactly once; want %q in:\n%s", want, output)
	}
}

// A collapsed job that reports success having checked nothing is worse than a
// slow matrix: a `find` expression mistake would silently retire the gate. Both
// steps must fail when no file is discovered.
func TestZshSyntaxFailsWhenNoFilesDiscovered(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}

	workflow := zshSyntaxWorkflow(t)
	for _, script := range []struct {
		name string
		text string
	}{
		{name: "zsh -n", text: zshSyntaxScript(t, workflow)},
		{name: "zcompile", text: zshCompileScript(t, workflow)},
	} {
		t.Run(script.name, func(t *testing.T) {
			command := exec.Command(bash, "--noprofile", "--norc", "-c", script.text)
			command.Dir = t.TempDir()
			output, runErr := command.CombinedOutput()
			if runErr == nil {
				t.Fatalf("%s step must fail when no Zsh file is discovered; it succeeded:\n%s", script.name, output)
			}
			if !strings.Contains(string(output), "No Zsh files were discovered") {
				t.Errorf("%s step must say why it failed; got:\n%s", script.name, output)
			}
		})
	}
}

// Every file is checked and every failure reported, rather than stopping at the
// first one. This is what the matrix's `fail-fast: false` provided, and it is
// the behavior worth keeping: a contributor fixing syntax wants the whole list.
func TestZshSyntaxReportsEveryFailure(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is required for the workflow behavior test")
	}

	dir := t.TempDir()
	for name, contents := range map[string]string{
		"good.zsh":     "true\n",
		"broken-a.zsh": "if true; then\n",
		"broken-b.zsh": "while (( 1 )\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write fixture %q: %v", name, err)
		}
	}

	command := exec.Command(bash, "--noprofile", "--norc", "-c", zshSyntaxScript(t, zshSyntaxWorkflow(t)))
	command.Dir = dir
	output, runErr := command.CombinedOutput()
	if runErr == nil {
		t.Fatalf("zsh -n step must fail when a file has a syntax error:\n%s", output)
	}

	// Both broken files are named, not just the first one reached.
	for _, name := range []string{"broken-a.zsh", "broken-b.zsh"} {
		if !strings.Contains(string(output), "::error file="+name) {
			t.Errorf("zsh -n step must annotate %s; got:\n%s", name, output)
		}
	}
	if want := "Checked 3 file(s) with zsh -n, 2 failed."; !strings.Contains(string(output), want) {
		t.Errorf("zsh -n step must report the full tally; want %q in:\n%s", want, output)
	}
}

// The filename reaches zcompile as one positional argument, never as shell
// syntax. `zsh -fc 'zcompile -- "$1"' zsh "$file"` is the shape that holds
// regardless of what the name contains, and `--` stops a leading dash being
// read as an option. TestZshCompileStepTreatsMetacharacterFilenameAsData
// proves the behavior; this asserts the shape so it cannot drift into an
// interpolated command string.
func TestZshCompileStepUsesOpaqueFilename(t *testing.T) {
	script := zshCompileScript(t, zshSyntaxWorkflow(t))
	for _, required := range []string{
		`zsh -fc 'zcompile -- "$1"' zsh "$file"`,
		`"${file}.zwc"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("zcompile script must treat the filename as data; missing %q in:\n%s", required, script)
		}
	}
	// An interpolated filename inside the compiled expression would make the
	// name executable as code.
	if strings.Contains(script, `zcompile -- "$file"`) {
		t.Errorf("zcompile must pass the filename positionally, not interpolate it:\n%s", script)
	}
}

func TestZshCompileStepTreatsMetacharacterFilenameAsData(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is required for the workflow behavior test")
	}

	dir := t.TempDir()
	filename := "safe; : > injected; #.zsh"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("typeset -g PHASE1_OK=1\n"), 0o600); err != nil {
		t.Fatalf("write metacharacter fixture: %v", err)
	}

	command := exec.Command(
		bash,
		"--noprofile",
		"--norc",
		"-c",
		zshCompileScript(t, zshSyntaxWorkflow(t)),
	)
	command.Dir = dir
	output, runErr := command.CombinedOutput()

	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Errorf("filename content executed as shell syntax; marker stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, filename+".zwc")); err != nil {
		t.Errorf("zcompile did not compile the intended filename: %v", err)
	}
	if runErr != nil {
		t.Errorf("zcompile workflow command failed: %v\n%s", runErr, output)
	}
}

func TestZshSyntaxCheckoutsDoNotPersistCredentials(t *testing.T) {
	workflow := zshSyntaxWorkflow(t)
	for _, jobName := range []string{"zsh-n"} {
		var checkoutSteps []string
		for _, step := range workflowJobSteps(t, workflow, jobName) {
			uses := directWorkflowMapping(step, 8)["uses"]
			if len(uses) == 1 && strings.HasPrefix(uses[0], "actions/checkout@") {
				checkoutSteps = append(checkoutSteps, step)
			}
		}
		if len(checkoutSteps) != 1 {
			t.Fatalf("job %q must contain exactly one checkout step; got %d", jobName, len(checkoutSteps))
		}
		withBlock := workflowBlock(t, checkoutSteps[0], "with:", 8)
		values := directWorkflowMapping(withBlock, 10)["persist-credentials"]
		if len(values) != 1 || values[0] != "false" {
			t.Fatalf("job %q checkout must disable persisted credentials; got %q", jobName, values)
		}
	}
}

func TestPullRequestValidationCheckoutsDoNotPersistCredentials(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		jobName string
	}{
		{name: "Go CI", path: "go-ci.yml", jobName: "build-test"},
		{name: "Docs Generate Check", path: "docs-generate.yml", jobName: "generate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := readRepositoryFile(t, ".github", "workflows", tt.path)
			var checkoutSteps []string
			for _, step := range workflowJobSteps(t, workflow, tt.jobName) {
				uses := directWorkflowMapping(step, 8)["uses"]
				if len(uses) == 1 && strings.HasPrefix(uses[0], "actions/checkout@") {
					checkoutSteps = append(checkoutSteps, step)
				}
			}
			if len(checkoutSteps) != 1 {
				t.Fatalf("job %q must contain exactly one checkout step; got %d", tt.jobName, len(checkoutSteps))
			}
			withBlock := workflowBlock(t, checkoutSteps[0], "with:", 8)
			values := directWorkflowMapping(withBlock, 10)["persist-credentials"]
			if len(values) != 1 || values[0] != "false" {
				t.Fatalf("job %q checkout must disable persisted credentials; got %q", tt.jobName, values)
			}
		})
	}
}

func workflowPushBranches(t *testing.T, workflow string) []string {
	t.Helper()

	onBlock := workflowBlock(t, workflow, "on:", 0)
	pushBlock := workflowBlock(t, onBlock, "push:", 2)
	values := directWorkflowMapping(pushBlock, 4)["branches"]
	if len(values) != 1 {
		t.Fatalf("push trigger must define branches exactly once; got %q", values)
	}

	switch values[0] {
	case "[main]":
		return []string{"main"}
	case "":
		branchesBlock := workflowBlock(t, pushBlock, "branches:", 4)
		var branches []string
		for _, line := range strings.Split(branchesBlock, "\n") {
			const prefix = "      - "
			if strings.HasPrefix(line, prefix) {
				branches = append(branches, strings.TrimPrefix(line, prefix))
			}
		}
		return branches
	default:
		t.Fatalf("push branches must target only main; got %q", values[0])
		return nil
	}
}

func TestValidationPushTriggersUseMainOnly(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "Go CI", path: "go-ci.yml"},
		{name: "Docs Generate Check", path: "docs-generate.yml"},
		{name: "Corpus Gate", path: "corpus-gate.yml"},
		{name: "Trunk Code Quality", path: "trunk-check.yml"},
		{name: "Zsh Syntax Check", path: "zsh-n.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := readRepositoryFile(t, ".github", "workflows", tt.path)
			got := workflowPushBranches(t, workflow)
			sort.Strings(got)
			if joined := strings.Join(got, ","); joined != "main" {
				t.Fatalf("push validation must target exactly main; got %q", got)
			}
		})
	}
}

func TestDependencyAutomationUsesDefaultMain(t *testing.T) {
	var renovate map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readRepositoryFile(t, ".github", "renovate.json")), &renovate); err != nil {
		t.Fatalf("parse .github/renovate.json: %v", err)
	}
	if _, configured := renovate["baseBranchPatterns"]; configured {
		t.Fatal("Renovate must follow the repository default main branch without a baseBranchPatterns override")
	}

	path := repositoryFilePath(t, ".github", "dependabot.yml")
	if _, err := os.Stat(path); err == nil {
		dependabot := readRepositoryFile(t, ".github", "dependabot.yml")
		updatesBlock := workflowBlock(t, dependabot, "updates:", 0)
		githubActionsUpdater := workflowBlock(
			t,
			updatesBlock,
			`- package-ecosystem: "github-actions"`,
			2,
		)
		values := directWorkflowMapping(githubActionsUpdater, 4)["target-branch"]
		if len(values) != 0 {
			t.Fatalf("Dependabot must follow the repository default main branch without target-branch; got %q", values)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect dependabot.yml: %v", err)
	}
}

func TestTrunkModelHasNoMainBranchSourceGuard(t *testing.T) {
	path := repositoryFilePath(t, ".github", "workflows", "main-branch-guard.yml")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("trunk-on-main must not retain the next-to-main source guard workflow")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect retired main branch guard: %v", err)
	}
}

func TestGoCIBuildTestReportsOnEveryPullRequest(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "go-ci.yml")
	onBlock := workflowBlock(t, workflow, "on:", 0)
	values := directWorkflowMapping(onBlock, 2)["pull_request"]
	if len(values) != 1 || values[0] != "{}" {
		t.Fatalf("Go CI pull_request trigger must be unfiltered so build-test always reports; got %q", values)
	}
	if got := len(exactWorkflowLineSpans(workflow, "  build-test:")); got != 1 {
		t.Fatalf("Go CI must retain the stable build-test job ID exactly once; got %d", got)
	}
}

// Trunk judges only changed lines, so findings on main went unreported until a
// weekly run failed (#380). The build-test lint step is the whole-module check
// that closes that gap; dropping it, limiting it to new issues, or capping the
// reported findings would reopen it.
func TestGoCIBuildTestLintsTheWholeModule(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "go-ci.yml")
	var lintSteps []string
	for _, step := range workflowJobSteps(t, workflow, "build-test") {
		uses := directWorkflowMapping(step, 8)["uses"]
		if len(uses) == 1 && strings.HasPrefix(uses[0], "golangci/golangci-lint-action@") {
			lintSteps = append(lintSteps, step)
		}
	}
	if len(lintSteps) != 1 {
		t.Fatalf("Go CI build-test must run golangci-lint-action exactly once; got %d", len(lintSteps))
	}
	withBlock := workflowBlock(t, lintSteps[0], "with:", 8)
	if values := directWorkflowMapping(withBlock, 10)["only-new-issues"]; len(values) != 0 && values[0] != "false" {
		t.Fatalf("Go CI lint must judge the whole module, not only new issues; got only-new-issues %q", values)
	}

	config := readRepositoryFile(t, ".golangci.yml")
	for _, setting := range []string{"max-issues-per-linter", "max-same-issues"} {
		if got := len(exactWorkflowLineSpans(config, "  "+setting+": 0")); got != 1 {
			t.Fatalf(".golangci.yml must set %s to 0 so no finding is hidden (#380); got %d matching lines", setting, got)
		}
	}
}

// A matrix job reports one check run per leg, so zsh-n's contexts are named
// after the discovered file paths ("zsh-n (./path/to/file.zsh)") and change
// A ruleset's required_status_checks list cannot reference a moving name. The
// workflow once fanned out a matrix leg per file, whose contexts
// `zsh-n (<path>)` changed whenever a Zsh file was added, renamed or removed,
// so a separate aggregating job existed purely to publish one stable name.
//
// A single job publishes that name directly, so the aggregate job is gone and
// there is no leg whose failure could be swallowed; `needs` and `if: always()`
// existed to stop a skipped aggregate from silently never reporting, and with
// one job there is nothing to skip. What still must hold is that the stable
// context name is present exactly once and reports on every pull request.
func TestZshSyntaxExposesStableAggregateContext(t *testing.T) {
	workflow := zshSyntaxWorkflow(t)

	// The aggregate is only safe to require if it reports on every pull
	// request. A pull_request.paths filter would silence it on a pull request
	// touching no Zsh, leaving that pull request permanently pending against a
	// required check.
	onBlock := workflowBlock(t, workflow, "on:", 0)
	if values := directWorkflowMapping(onBlock, 2)["pull_request"]; len(values) != 1 || values[0] != "{}" {
		t.Fatalf("zsh-n pull_request trigger must be unfiltered so the aggregate always reports; got %q", values)
	}

	// Exactly one job, so no leg's result can be lost on the way to the
	// required context.
	jobsBlock := workflowBlock(t, workflow, "jobs:", 0)
	jobNames := make([]string, 0, 1)
	for _, line := range workflowLines(jobsBlock) {
		text := line.text
		if strings.HasPrefix(text, "  ") && !strings.HasPrefix(text, "   ") &&
			strings.HasSuffix(strings.TrimSpace(text), ":") && !strings.HasPrefix(strings.TrimSpace(text), "#") {
			jobNames = append(jobNames, strings.TrimSuffix(strings.TrimSpace(text), ":"))
		}
	}
	if len(jobNames) != 1 || jobNames[0] != "zsh-n" {
		t.Fatalf("zsh-n.yml must define exactly one job named zsh-n so the required context cannot be skipped; got %q", jobNames)
	}

	// The name the ruleset requires, unchanged from the matrix era.
	fields := directWorkflowMapping(workflowBlock(t, workflow, "zsh-n:", 2), 4)
	if values := fields["name"]; len(values) != 1 || values[0] != "Zsh Syntax Check Complete" {
		t.Errorf(
			"zsh-n name must be %q because that is the context the ruleset requires; got %q",
			"Zsh Syntax Check Complete", values,
		)
	}
	// A matrix would reintroduce per-leg contexts and the moving-name problem.
	if values := fields["strategy"]; len(values) != 0 {
		t.Errorf("zsh-n must not use a matrix strategy; per-leg context names cannot be required: got %q", values)
	}
}

func requirePinnedAction(t *testing.T, step, action string) {
	t.Helper()

	pattern := regexp.MustCompile(
		`(?m)^        uses: ` + regexp.QuoteMeta(action) + `@[0-9a-f]{40}(?: # v[0-9]+\.[0-9]+\.[0-9]+)?$`,
	)
	if got := pattern.FindAllString(step, -1); len(got) != 1 {
		t.Fatalf("%s must use exactly one immutable action SHA; got %q", action, got)
	}
}

func releaseSemanticTagScript(t *testing.T, workflow string) string {
	t.Helper()

	step := workflowStep(t, workflow, "Verify semantic tag", "Set up Go")
	return workflowRunScript(t, step)
}

const releaseSemanticTagVerificationScript = `tag="${GITHUB_REF_NAME}"
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::Expected a semantic version tag like v1.2.3, got '${tag}'"
  exit 1
fi
tag_ref="refs/tags/${tag}"
tag_type="$(git cat-file -t "$tag_ref" 2>/dev/null || true)"
if [[ "$tag_type" != "tag" ]]; then
  echo "::error::Expected '${tag}' to be an annotated tag."
  exit 1
fi
tag_commit="$(git rev-list -n 1 "$tag_ref")"
head_commit="$(git rev-parse HEAD)"
if [[ "$tag_commit" != "${GITHUB_SHA}" || "$head_commit" != "${GITHUB_SHA}" ]]; then
  echo "::error::Tag '${tag}' and the checked-out commit must both resolve to event commit '${GITHUB_SHA}'."
  exit 1
fi
`

func releaseSemanticTagStepViolations(t *testing.T, verifyStep string) []string {
	t.Helper()

	violations := exactWorkflowMappingViolations(
		"semantic tag verification step",
		verifyStep,
		8,
		[]workflowMappingField{{name: "run", value: "|"}},
	)
	if got := workflowRunScript(t, verifyStep); got != releaseSemanticTagVerificationScript {
		violations = append(violations, "semantic tag verification script must match the exact tag and HEAD checks")
	}
	return violations
}

func releaseCheckoutViolations(t *testing.T, checkoutStep string) []string {
	t.Helper()

	requirePinnedAction(t, checkoutStep, "actions/checkout")
	uses := directWorkflowMapping(checkoutStep, 8)["uses"]
	if len(uses) != 1 {
		return []string{"release checkout must define one structural uses field"}
	}
	var violations []string
	violations = append(violations, exactWorkflowMappingViolations(
		"release checkout step",
		checkoutStep,
		8,
		[]workflowMappingField{
			{name: "uses", value: uses[0]},
			{name: "with", value: ""},
		},
	)...)
	withBlock := workflowBlock(t, checkoutStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"release checkout inputs",
		withBlock,
		10,
		[]workflowMappingField{
			{name: "ref", value: "${{ github.sha }}"},
			{name: "fetch-depth", value: "0"},
			{name: "persist-credentials", value: "false"},
		},
	)...)
	return violations
}

func releaseSetupGoViolations(t *testing.T, setupStep string) []string {
	t.Helper()

	requirePinnedAction(t, setupStep, "actions/setup-go")
	uses := directWorkflowMapping(setupStep, 8)["uses"]
	if len(uses) != 1 {
		return []string{"release Go setup must define one structural uses field"}
	}
	var violations []string
	violations = append(violations, exactWorkflowMappingViolations(
		"release Go setup step",
		setupStep,
		8,
		[]workflowMappingField{
			{name: "uses", value: uses[0]},
			{name: "with", value: ""},
		},
	)...)
	withBlock := workflowBlock(t, setupStep, "with:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"release Go setup inputs",
		withBlock,
		10,
		[]workflowMappingField{{name: "go-version", value: `"1.26"`}},
	)...)
	return violations
}

const releasePublicationScript = `tag="${GITHUB_REF_NAME}"
if ! gh release view "$tag" >/dev/null 2>&1; then
  gh release create "$tag" --verify-tag --title "$tag" --generate-notes
fi
`

func releaseGateStepViolations(t *testing.T, testStep, publishStep string) []string {
	t.Helper()

	var violations []string
	violations = append(violations, exactWorkflowMappingViolations(
		"tagged-commit test step",
		testStep,
		8,
		[]workflowMappingField{{name: "run", value: "go test -count=1 ./..."}},
	)...)
	violations = append(violations, exactWorkflowMappingViolations(
		"release publication step",
		publishStep,
		8,
		[]workflowMappingField{
			{name: "env", value: ""},
			{name: "run", value: "|"},
		},
	)...)
	envBlock := workflowBlock(t, publishStep, "env:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"release publication environment",
		envBlock,
		10,
		[]workflowMappingField{{name: "GH_TOKEN", value: "${{ github.token }}"}},
	)...)
	if got := workflowRunScript(t, publishStep); got != releasePublicationScript {
		violations = append(violations, "release publication script must match the verified-tag command exactly")
	}
	return violations
}

func releaseWorkflowBoundaryViolations(t *testing.T, workflow string) []string {
	t.Helper()

	var violations []string
	document := strings.TrimPrefix(workflow, "---\n")
	if document == workflow {
		violations = append(violations, "release workflow must begin with a YAML document marker")
	}
	violations = append(violations, exactWorkflowMappingViolations(
		"release workflow",
		document,
		0,
		[]workflowMappingField{
			{name: "name", value: "Release"},
			{name: "on", value: ""},
			{name: "permissions", value: ""},
			{name: "concurrency", value: ""},
			{name: "jobs", value: ""},
		},
	)...)
	permissionsBlock := workflowBlock(t, workflow, "permissions:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"release workflow permissions",
		permissionsBlock,
		2,
		[]workflowMappingField{{name: "contents", value: "write"}},
	)...)
	concurrencyBlock := workflowBlock(t, workflow, "concurrency:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"release workflow concurrency",
		concurrencyBlock,
		2,
		[]workflowMappingField{
			{name: "group", value: "${{ github.workflow }}-${{ github.ref }}"},
			{name: "cancel-in-progress", value: "false"},
		},
	)...)
	jobsBlock := workflowBlock(t, workflow, "jobs:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"release workflow jobs",
		jobsBlock,
		2,
		[]workflowMappingField{{name: "release", value: ""}},
	)...)
	releaseJob := workflowBlock(t, workflow, "release:", 2)
	violations = append(violations, exactWorkflowMappingViolations(
		"release job",
		releaseJob,
		4,
		[]workflowMappingField{
			{name: "if", value: "github.repository == 'z-shell/zsh-lint'"},
			{name: "runs-on", value: "ubuntu-latest"},
			{name: "steps", value: ""},
		},
	)...)
	stepsBlock := workflowBlock(t, releaseJob, "steps:", 4)
	violations = append(violations, exactWorkflowStepSequenceViolations(
		stepsBlock,
		[]string{
			"name: Check out code",
			"name: Verify semantic tag",
			"name: Set up Go",
			"name: Test tagged commit",
			"name: Publish GitHub release",
		},
	)...)
	return violations
}

func TestReleaseTestsExactTagCommitBeforePublication(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	if violations := releaseWorkflowBoundaryViolations(t, workflow); len(violations) != 0 {
		t.Fatalf("release workflow must contain only the gated job and ordered steps: %s", strings.Join(violations, "; "))
	}

	checkoutStep := workflowStep(t, workflow, "Check out code", "Verify semantic tag")
	if violations := releaseCheckoutViolations(t, checkoutStep); len(violations) != 0 {
		t.Fatalf("release checkout must select the exact event commit: %s", strings.Join(violations, "; "))
	}

	verifyStep := workflowStep(t, workflow, "Verify semantic tag", "Set up Go")
	if violations := releaseSemanticTagStepViolations(t, verifyStep); len(violations) != 0 {
		t.Fatalf("semantic tag verification must not permit condition or failure bypasses: %s", strings.Join(violations, "; "))
	}

	setupStep := workflowStep(t, workflow, "Set up Go", "Test tagged commit")
	if violations := releaseSetupGoViolations(t, setupStep); len(violations) != 0 {
		t.Fatalf("release Go setup must use the deterministic toolchain: %s", strings.Join(violations, "; "))
	}

	testStep := workflowStep(t, workflow, "Test tagged commit", "Publish GitHub release")
	publishStep := workflowTerminalStep(t, workflow, "Publish GitHub release")
	if violations := releaseGateStepViolations(t, testStep, publishStep); len(violations) != 0 {
		t.Fatalf("release gate steps must not permit failure bypasses: %s", strings.Join(violations, "; "))
	}
}

func TestReleaseRejectsBoundaryExpansion(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	tests := []struct {
		name    string
		mutated string
	}{
		{
			name: "extra job",
			mutated: workflow + `
  bypass:
    runs-on: ubuntu-latest
    steps:
      - run: true
`,
		},
		{
			name: "top-level run defaults",
			mutated: strings.Replace(
				workflow,
				"permissions:\n",
				"defaults:\n  run:\n    shell: bash\n\npermissions:\n",
				1,
			),
		},
		{
			name: "job run defaults",
			mutated: strings.Replace(
				workflow,
				"    runs-on: ubuntu-latest\n",
				"    defaults:\n      run:\n        shell: bash\n    runs-on: ubuntu-latest\n",
				1,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mutated == workflow {
				t.Fatal("release workflow mutation anchor was not found")
			}
			if violations := releaseWorkflowBoundaryViolations(t, tt.mutated); len(violations) == 0 {
				t.Fatal("release workflow contract accepted a boundary expansion")
			}
		})
	}
}

func TestReleaseSetupGoRejectsVersionDecoy(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	setupStep := workflowStep(t, workflow, "Set up Go", "Test tagged commit")
	mutated := strings.Replace(
		setupStep,
		`        with:
          go-version: "1.26"`,
		`        if: false
        with:
          go-version: "1.24"
          # go-version: "1.26"`,
		1,
	)
	if mutated == setupStep {
		t.Fatal("release Go setup mutation anchor was not found")
	}
	if violations := releaseSetupGoViolations(t, mutated); len(violations) == 0 {
		t.Fatal("release Go setup contract accepted a skipped step with a version comment decoy")
	}
}

func TestReleaseCheckoutRejectsRefDecoy(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	checkoutStep := workflowStep(t, workflow, "Check out code", "Verify semantic tag")
	mutated := strings.Replace(
		checkoutStep,
		"          ref: ${{ github.sha }}",
		"          ref: main\n          # ref: ${{ github.sha }}",
		1,
	)
	if mutated == checkoutStep {
		t.Fatal("release checkout mutation anchor was not found")
	}
	if violations := releaseCheckoutViolations(t, mutated); len(violations) == 0 {
		t.Fatal("release checkout contract accepted a comment decoy for the exact event commit")
	}
}

func TestReleaseSemanticTagContractRejectsPostVerificationMutation(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	verifyStep := workflowStep(t, workflow, "Verify semantic tag", "Set up Go")
	mutated := strings.TrimSuffix(verifyStep, "\n") + "\n          git checkout --detach HEAD^\n"
	if violations := releaseSemanticTagStepViolations(t, mutated); len(violations) == 0 {
		t.Fatal("semantic tag contract accepted a post-verification working-tree mutation")
	}
}

func TestReleaseSemanticTagGuardRejectsInvalidNames(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the release tag behavior test")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required for the release tag behavior test")
	}
	script := releaseSemanticTagScript(
		t,
		readRepositoryFile(t, ".github", "workflows", "release.yml"),
	)
	dir := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		command := exec.Command(git, args...)
		command.Dir = dir
		output, runErr := command.CombinedOutput()
		if runErr != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), runErr, output)
		}
		return strings.TrimSpace(string(output))
	}
	runGit("init")
	runGit("config", "user.name", "Workflow Contract")
	runGit("config", "user.email", "workflow-contract@example.invalid")
	runGit("config", "commit.gpgsign", "false")
	runGit("config", "tag.gpgsign", "false")
	fixturePath := filepath.Join(dir, "fixture")
	if err := os.WriteFile(fixturePath, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write first tag fixture: %v", err)
	}
	runGit("add", "fixture")
	runGit("commit", "-m", "test: first tag fixture")
	firstCommit := runGit("rev-parse", "HEAD")
	runGit("tag", "-a", "v2.0.0", "-m", "mismatched annotated tag", firstCommit)
	if err := os.WriteFile(fixturePath, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second tag fixture: %v", err)
	}
	runGit("add", "fixture")
	runGit("commit", "-m", "test: second tag fixture")
	headCommit := runGit("rev-parse", "HEAD")
	runGit("tag", "-a", "v1.2.3", "-m", "valid annotated tag", headCommit)
	runGit("tag", "v1.2.4", headCommit)
	runGit("tag", "-a", "v1.2.3-rc1", "-m", "invalid semantic tag", headCommit)

	tests := []struct {
		name        string
		tag         string
		wantSuccess bool
	}{
		{name: "annotated semantic tag at event commit", tag: "v1.2.3", wantSuccess: true},
		{name: "lightweight semantic tag", tag: "v1.2.4"},
		{name: "annotated tag at another commit", tag: "v2.0.0"},
		{name: "prerelease tag", tag: "v1.2.3-rc1"},
		{name: "missing semantic tag", tag: "v3.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command(
				bash,
				"--noprofile",
				"--norc",
				"-e",
				"-o",
				"pipefail",
				"-c",
				script,
			)
			command.Dir = dir
			command.Env = append(
				os.Environ(),
				"GITHUB_REF_NAME="+tt.tag,
				"GITHUB_SHA="+headCommit,
			)
			output, runErr := command.CombinedOutput()
			if tt.wantSuccess && runErr != nil {
				t.Fatalf("valid semantic tag was rejected: %v\n%s", runErr, output)
			}
			if !tt.wantSuccess && runErr == nil {
				t.Fatalf("invalid tag was accepted:\n%s", output)
			}
		})
	}
}

func TestReleaseGateRejectsFailureBypasses(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	testStep := workflowStep(t, workflow, "Test tagged commit", "Publish GitHub release")
	publishStep := workflowTerminalStep(t, workflow, "Publish GitHub release")

	tests := []struct {
		name        string
		testStep    string
		publishStep string
	}{
		{
			name: "continue-on-error test",
			testStep: strings.Replace(
				testStep,
				"        run: go test -count=1 ./...",
				"        continue-on-error: true\n        run: go test -count=1 ./...",
				1,
			),
			publishStep: publishStep,
		},
		{
			name:     "always publish",
			testStep: testStep,
			publishStep: strings.Replace(
				publishStep,
				"        env:",
				"        if: always()\n        env:",
				1,
			),
		},
		{
			name:     "alternate publication API behind comment decoy",
			testStep: testStep,
			publishStep: strings.Replace(
				publishStep,
				`            gh release create "$tag" --verify-tag --title "$tag" --generate-notes`,
				"            # gh release create \"$tag\" --verify-tag --title \"$tag\" --generate-notes\n"+
					"            gh api \"repos/${GITHUB_REPOSITORY}/releases\" --method POST",
				1,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if violations := releaseGateStepViolations(t, tt.testStep, tt.publishStep); len(violations) == 0 {
				t.Fatal("release contract accepted a failure-bypass mutation")
			}
		})
	}
}

func TestReleasePublishesOnlyExistingSemanticTag(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	onBlock := workflowBlock(t, workflow, "on:", 0)
	if violations := exactWorkflowMappingViolations(
		"release triggers",
		onBlock,
		2,
		[]workflowMappingField{{name: "push", value: ""}},
	); len(violations) != 0 {
		t.Fatalf("release must use only the tag push trigger: %s", strings.Join(violations, "; "))
	}
	pushBlock := workflowBlock(t, onBlock, "push:", 2)
	if violations := exactWorkflowMappingViolations(
		"release tag push",
		pushBlock,
		4,
		[]workflowMappingField{{name: "tags", value: `["v*.*.*"]`}},
	); len(violations) != 0 {
		t.Fatalf("release must trigger only for semantic version tag pushes: %s", strings.Join(violations, "; "))
	}

	publishStep := workflowTerminalStep(t, workflow, "Publish GitHub release")
	if got := workflowRunScript(t, publishStep); got != releasePublicationScript {
		t.Fatalf("release publication script must match the verified-tag flow exactly:\nwant:\n%s\ngot:\n%s", releasePublicationScript, got)
	}
	if got := strings.Count(workflow, "gh release create "); got != 1 {
		t.Fatalf("release workflow must contain exactly one publication command; got %d", got)
	}
	for _, forbidden := range []string{"git tag ", "/git/refs", "gh api repos/{owner}/{repo}/git/refs"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release workflow must not create tags; found %q", forbidden)
		}
	}
}
