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

func zshMatrixScript(t *testing.T, workflow string) string {
	t.Helper()

	step := workflowJobStep(t, workflow, "zsh-matrix", `"Set matrix output"`)
	return workflowRunScript(t, step)
}

func zshCompileScript(t *testing.T, workflow string) string {
	t.Helper()

	step := workflowJobStep(t, workflow, "zsh-n", `"⚡ zcompile ${{ matrix.file }}"`)
	return workflowRunScript(t, step)
}

func TestZshMatrixUsesNULDelimitedFilenames(t *testing.T) {
	script := zshMatrixScript(t, zshSyntaxWorkflow(t))
	for _, required := range []string{
		"-print0",
		"jq -Rsc",
		`split("\u0000")`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("matrix generation is missing NUL-safe transport fragment %q", required)
		}
	}
}

func TestZshMatrixPreservesNewlineFilename(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required for the workflow behavior test")
	}

	dir := t.TempDir()
	for _, name := range []string{"line\nbreak.zsh", "ordinary.zsh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("true\n"), 0o600); err != nil {
			t.Fatalf("write matrix fixture %q: %v", name, err)
		}
	}
	outputPath := filepath.Join(dir, "github-output")
	command := exec.Command(
		bash,
		"--noprofile",
		"--norc",
		"-e",
		"-o",
		"pipefail",
		"-c",
		zshMatrixScript(t, zshSyntaxWorkflow(t)),
	)
	command.Dir = dir
	command.Env = append(os.Environ(), "GITHUB_OUTPUT="+outputPath)
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		t.Fatalf("matrix workflow command failed: %v\n%s", runErr, output)
	}

	githubOutput, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read matrix workflow output: %v", err)
	}
	matrixJSON, found := strings.CutPrefix(strings.TrimSpace(string(githubOutput)), "matrix=")
	if !found {
		t.Fatalf("matrix workflow output is missing matrix= prefix: %q", githubOutput)
	}
	var matrix struct {
		Include []struct {
			File string `json:"file"`
		} `json:"include"`
	}
	if err := json.Unmarshal([]byte(matrixJSON), &matrix); err != nil {
		t.Fatalf("parse matrix workflow output %q: %v", matrixJSON, err)
	}
	var got []string
	for _, item := range matrix.Include {
		got = append(got, item.File)
	}
	sort.Strings(got)
	want := []string{"./line\nbreak.zsh", "./ordinary.zsh"}
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("matrix must preserve each filename as one JSON entry:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestZshCompileStepUsesOpaqueFilename(t *testing.T) {
	const want = `zsh -fc 'zcompile -- "$1"' zsh "$ZSH_FILE"; rc=$?
ls -al -- "${ZSH_FILE}.zwc"; exit "$rc"
`

	if got := zshCompileScript(t, zshSyntaxWorkflow(t)); got != want {
		t.Fatalf("zcompile script must treat the filename as one positional argument:\nwant:\n%s\ngot:\n%s", want, got)
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
	sourcePath := filepath.Join(dir, filename)
	if err := os.WriteFile(sourcePath, []byte("typeset -g PHASE1_OK=1\n"), 0o600); err != nil {
		t.Fatalf("write metacharacter fixture: %v", err)
	}

	command := exec.Command(
		bash,
		"--noprofile",
		"--norc",
		"-e",
		"-o",
		"pipefail",
		"-c",
		zshCompileScript(t, zshSyntaxWorkflow(t)),
	)
	command.Dir = dir
	command.Env = append(os.Environ(), "ZSH_FILE="+sourcePath)
	output, runErr := command.CombinedOutput()

	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Errorf("filename content executed as shell syntax; marker stat error: %v", err)
	}
	if _, err := os.Stat(sourcePath + ".zwc"); err != nil {
		t.Errorf("zcompile did not compile the intended filename: %v", err)
	}
	if runErr != nil {
		t.Errorf("zcompile workflow command failed: %v\n%s", runErr, output)
	}
}

func TestZshSyntaxCheckoutsDoNotPersistCredentials(t *testing.T) {
	workflow := zshSyntaxWorkflow(t)
	for _, jobName := range []string{"zsh-matrix", "zsh-n"} {
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

func workflowPushBranches(t *testing.T, workflow string) []string {
	t.Helper()

	onBlock := workflowBlock(t, workflow, "on:", 0)
	pushBlock := workflowBlock(t, onBlock, "push:", 2)
	values := directWorkflowMapping(pushBlock, 4)["branches"]
	if len(values) != 1 {
		t.Fatalf("push trigger must define branches exactly once; got %q", values)
	}

	switch values[0] {
	case "[main, next]":
		return []string{"main", "next"}
	case "[next, main]":
		return []string{"next", "main"}
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
		t.Fatalf("push branches must use the canonical main/next form; got %q", values[0])
		return nil
	}
}

func TestValidationPushTriggersIncludeMainAndNext(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "Go CI", path: "go-ci.yml"},
		{name: "Docs Generate Check", path: "docs-generate.yml"},
		{name: "Trunk Code Quality", path: "trunk-check.yml"},
		{name: "Zsh Syntax Check", path: "zsh-n.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := readRepositoryFile(t, ".github", "workflows", tt.path)
			got := workflowPushBranches(t, workflow)
			sort.Strings(got)
			if joined := strings.Join(got, ","); joined != "main,next" {
				t.Fatalf("push validation must target exactly main and next; got %q", got)
			}
		})
	}
}

func TestDependencyAutomationTargetsNext(t *testing.T) {
	var renovate struct {
		BaseBranchPatterns []string `json:"baseBranchPatterns"`
	}
	if err := json.Unmarshal([]byte(readRepositoryFile(t, "renovate.json")), &renovate); err != nil {
		t.Fatalf("parse renovate.json: %v", err)
	}
	if got := strings.Join(renovate.BaseBranchPatterns, ","); got != "next" {
		t.Fatalf("Renovate must target only next; got %q", got)
	}

	dependabot := readRepositoryFile(t, ".github", "dependabot.yml")
	updatesBlock := workflowBlock(t, dependabot, "updates:", 0)
	githubActionsUpdater := workflowBlock(
		t,
		updatesBlock,
		`- package-ecosystem: "github-actions"`,
		2,
	)
	values := directWorkflowMapping(githubActionsUpdater, 4)["target-branch"]
	if len(values) != 1 || values[0] != `"next"` {
		t.Fatalf("retained GitHub Actions Dependabot updater must target next; got %q", values)
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

const mainBranchGuardScript = `if [[ "${HEAD_REPOSITORY}" != "${REPOSITORY}" ]]; then
  echo "::error::Pull requests into main must come from this repository (got '${HEAD_REPOSITORY}')."
  exit 1
fi
if [[ "${HEAD_REF}" == "next" || "${HEAD_REF}" == hotfix-* ]]; then
  echo "Head branch '${HEAD_REF}' is allowed to target main."
  exit 0
fi
if [[ "${PULL_REQUEST_AUTHOR}" == "dependabot[bot]" && "${HEAD_REF}" == dependabot/* ]]; then
  echo "Dependabot security update branch '${HEAD_REF}' is allowed to target main."
  exit 0
fi
echo "::error::Pull requests into main must come from 'next', a 'hotfix-*' branch, or an authenticated Dependabot security-update branch (got '${HEAD_REF}' by '${PULL_REQUEST_AUTHOR}'). See ADR-0008 and ADR-0012 (z-shell/.github)."
exit 1
`

func mainBranchGuardMappingViolations(t *testing.T, workflow string) []string {
	t.Helper()

	var violations []string
	document := strings.TrimPrefix(workflow, "---\n")
	if document == workflow {
		violations = append(violations, "main branch guard must begin with a YAML document marker")
	}
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard workflow",
		document,
		0,
		[]workflowMappingField{
			{name: "name", value: "Main Branch Source Guard"},
			{name: "on", value: ""},
			{name: "permissions", value: ""},
			{name: "concurrency", value: ""},
			{name: "jobs", value: ""},
		},
	)...)
	permissionsBlock := workflowBlock(t, workflow, "permissions:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard permissions",
		permissionsBlock,
		2,
		[]workflowMappingField{{name: "contents", value: "read"}},
	)...)
	concurrencyBlock := workflowBlock(t, workflow, "concurrency:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard concurrency",
		concurrencyBlock,
		2,
		[]workflowMappingField{
			{name: "group", value: "${{ github.workflow }}-${{ github.event.pull_request.number }}"},
			{name: "cancel-in-progress", value: "true"},
		},
	)...)
	jobsBlock := workflowBlock(t, workflow, "jobs:", 0)
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard jobs",
		jobsBlock,
		2,
		[]workflowMappingField{{name: "guard", value: ""}},
	)...)
	jobBlock := workflowBlock(t, workflow, "guard:", 2)
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard job",
		jobBlock,
		4,
		[]workflowMappingField{
			{name: "name", value: "Guard main branch source"},
			{name: "runs-on", value: "ubuntu-latest"},
			{name: "steps", value: ""},
		},
	)...)
	stepsBlock := workflowBlock(t, jobBlock, "steps:", 4)
	violations = append(violations, exactWorkflowStepSequenceViolations(
		stepsBlock,
		[]string{"name: Verify pull request source branch"},
	)...)

	step := workflowTerminalStep(t, workflow, "Verify pull request source branch")
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard step",
		step,
		8,
		[]workflowMappingField{
			{name: "env", value: ""},
			{name: "run", value: "|"},
		},
	)...)
	if got := workflowRunScript(t, step); got != mainBranchGuardScript {
		violations = append(violations, "main branch guard script must match the authenticated source policy exactly")
	}
	envBlock := workflowBlock(t, step, "env:", 8)
	violations = append(violations, exactWorkflowMappingViolations(
		"main branch guard environment",
		envBlock,
		10,
		[]workflowMappingField{
			{name: "HEAD_REF", value: "${{ github.head_ref }}"},
			{name: "HEAD_REPOSITORY", value: "${{ github.event.pull_request.head.repo.full_name }}"},
			{name: "PULL_REQUEST_AUTHOR", value: "${{ github.event.pull_request.user.login }}"},
			{name: "REPOSITORY", value: "${{ github.repository }}"},
		},
	)...)
	return violations
}

func TestMainBranchSourceGuardEnforcesPromotionSources(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "main-branch-guard.yml")
	onBlock := workflowBlock(t, workflow, "on:", 0)
	if violations := exactWorkflowMappingViolations(
		"main branch guard triggers",
		onBlock,
		2,
		[]workflowMappingField{{name: "pull_request", value: ""}},
	); len(violations) != 0 {
		t.Fatalf("main branch guard must use only pull_request: %s", strings.Join(violations, "; "))
	}
	pullRequestBlock := workflowBlock(t, onBlock, "pull_request:", 2)
	if violations := exactWorkflowMappingViolations(
		"main branch guard pull request trigger",
		pullRequestBlock,
		4,
		[]workflowMappingField{
			{name: "branches", value: "[main]"},
			{name: "types", value: "[opened, reopened, synchronize, edited]"},
		},
	); len(violations) != 0 {
		t.Fatalf("main branch guard pull_request trigger is invalid: %s", strings.Join(violations, "; "))
	}

	if got := len(exactWorkflowLineSpans(workflow, "    name: Guard main branch source")); got != 1 {
		t.Fatalf("main branch guard must expose the stable required-check name exactly once; got %d", got)
	}
	if violations := mainBranchGuardMappingViolations(t, workflow); len(violations) != 0 {
		t.Fatalf("main branch guard must not permit condition or failure bypasses: %s", strings.Join(violations, "; "))
	}
	step := workflowTerminalStep(t, workflow, "Verify pull request source branch")

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the branch guard behavior test")
	}
	tests := []struct {
		name           string
		headRef        string
		headRepository string
		author         string
		wantSuccess    bool
	}{
		{
			name:           "next promotion",
			headRef:        "next",
			headRepository: "z-shell/zsh-lint",
			author:         "maintainer",
			wantSuccess:    true,
		},
		{
			name:           "hotfix promotion",
			headRef:        "hotfix-90",
			headRepository: "z-shell/zsh-lint",
			author:         "maintainer",
			wantSuccess:    true,
		},
		{
			name:           "Dependabot security update",
			headRef:        "dependabot/github_actions/actions-checkout-7",
			headRepository: "z-shell/zsh-lint",
			author:         "dependabot[bot]",
			wantSuccess:    true,
		},
		{
			name:           "spoofed Dependabot branch",
			headRef:        "dependabot/github_actions/actions-checkout-7",
			headRepository: "z-shell/zsh-lint",
			author:         "maintainer",
		},
		{
			name:           "feature bypass",
			headRef:        "feature-90",
			headRepository: "z-shell/zsh-lint",
			author:         "maintainer",
		},
		{
			name:           "forked next",
			headRef:        "next",
			headRepository: "contributor/zsh-lint",
			author:         "contributor",
		},
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
				workflowRunScript(t, step),
			)
			command.Env = append(
				os.Environ(),
				"HEAD_REF="+tt.headRef,
				"HEAD_REPOSITORY="+tt.headRepository,
				"PULL_REQUEST_AUTHOR="+tt.author,
				"REPOSITORY=z-shell/zsh-lint",
			)
			output, runErr := command.CombinedOutput()
			if tt.wantSuccess && runErr != nil {
				t.Fatalf("allowed source was rejected: %v\n%s", runErr, output)
			}
			if !tt.wantSuccess && runErr == nil {
				t.Fatalf("disallowed source was accepted:\n%s", output)
			}
		})
	}
}

func TestMainBranchSourceGuardRejectsBypassFields(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "main-branch-guard.yml")
	tests := []struct {
		name        string
		old         string
		replacement string
	}{
		{
			name:        "job condition",
			old:         "    name: Guard main branch source",
			replacement: "    if: false\n    name: Guard main branch source",
		},
		{
			name:        "step condition",
			old:         "        env:\n          HEAD_REF:",
			replacement: "        if: false\n        env:\n          HEAD_REF:",
		},
		{
			name:        "continue on error",
			old:         "        env:\n          HEAD_REF:",
			replacement: "        continue-on-error: true\n        env:\n          HEAD_REF:",
		},
		{
			name:        "unnamed prep step",
			old:         "    steps:\n      - name: Verify pull request source branch",
			replacement: "    steps:\n      - run: true\n      - name: Verify pull request source branch",
		},
		{
			name: "extra job",
			old:  "  guard:\n",
			replacement: "  bypass:\n" +
				"    runs-on: ubuntu-latest\n" +
				"    steps:\n" +
				"      - run: true\n" +
				"  guard:\n",
		},
		{
			name:        "top-level run defaults",
			old:         "permissions:\n",
			replacement: "defaults:\n  run:\n    shell: bash\n\npermissions:\n",
		},
		{
			name:        "job run defaults",
			old:         "    runs-on: ubuntu-latest\n",
			replacement: "    defaults:\n      run:\n        shell: bash\n    runs-on: ubuntu-latest\n",
		},
		{
			name: "extra guard command",
			old: "        run: |\n" +
				"          if [[ \"${HEAD_REPOSITORY}\" != \"${REPOSITORY}\" ]]; then",
			replacement: "        run: |\n" +
				"          true\n" +
				"          if [[ \"${HEAD_REPOSITORY}\" != \"${REPOSITORY}\" ]]; then",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := strings.Replace(workflow, tt.old, tt.replacement, 1)
			if mutated == workflow {
				t.Fatalf("mutation anchor %q was not found", tt.old)
			}
			if violations := mainBranchGuardMappingViolations(t, mutated); len(violations) == 0 {
				t.Fatal("main branch guard contract accepted a bypass mutation")
			}
		})
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
		[]workflowMappingField{{name: "go-version", value: `"1.25"`}},
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
          go-version: "1.25"`,
		`        if: false
        with:
          go-version: "1.24"
          # go-version: "1.25"`,
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
