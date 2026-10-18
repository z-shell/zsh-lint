package workflowcontract

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
	return readRepositoryFile(t, ".github", "workflows", "go-ci.yml")
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

	step := workflowJobStep(t, workflow, "build-test", `⚡ zsh -n`)
	return workflowRunScript(t, step)
}

func zshCompileScript(t *testing.T, workflow string) string {
	t.Helper()

	step := workflowJobStep(t, workflow, "build-test", `⚡ zcompile`)
	return workflowRunScript(t, step)
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
	for _, subdir := range []string{"legacy/functions", "examples/plugin", "doc"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0o700); err != nil {
			t.Fatalf("create fixture directory %q: %v", subdir, err)
		}
	}
	for _, name := range []string{
		"line\nbreak.zsh",
		"-leading.zsh",
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
	if err := os.WriteFile(filepath.Join(dir, "doc", "excluded.zsh"), []byte("if true; then\n"), 0o600); err != nil {
		t.Fatalf("write excluded documentation fixture: %v", err)
	}

	workflow := zshSyntaxWorkflow(t)
	for _, check := range []struct {
		name   string
		script string
		want   string
	}{
		{"syntax", zshSyntaxScript(t, workflow), "Checked 3 file(s) with zsh -n, 0 failed."},
		{"compile", zshCompileScript(t, workflow), "Compiled 3 file(s), 0 failed."},
	} {
		t.Run(check.name, func(t *testing.T) {
			command := exec.Command(bash, "--noprofile", "--norc", "-c", check.script)
			command.Dir = dir
			output, runErr := command.CombinedOutput()
			if runErr != nil {
				t.Fatalf("workflow command failed: %v\n%s", runErr, output)
			}
			// The three .zsh files are visited once, including the newline
			// and leading-dash names; extensionless decoys stay excluded.
			if !strings.Contains(string(output), check.want) {
				t.Fatalf("step must visit each .zsh file exactly once; want %q in:\n%s", check.want, output)
			}
			if check.name == "compile" {
				for _, name := range []string{"line\nbreak.zsh", "-leading.zsh", "ordinary.zsh"} {
					if _, err := os.Stat(filepath.Join(dir, name+".zwc")); !os.IsNotExist(err) {
						t.Errorf("compilation must not leave output beside source %q: %v", name, err)
					}
				}
			}
		})
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
			t.Run("partial discovery failure", func(t *testing.T) {
				dir := t.TempDir()
				bin := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "good.zsh"), []byte("true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				stub := "#!/bin/sh\nprintf './good.zsh\\0'\nprintf 'find: traversal failed\\n' >&2\nexit 1\n"
				if err := os.WriteFile(filepath.Join(bin, "find"), []byte(stub), 0o700); err != nil {
					t.Fatal(err)
				}
				command := exec.Command(bash, "--noprofile", "--norc", "-c", script.text)
				command.Dir = dir
				command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				output, runErr := command.CombinedOutput()
				if runErr == nil || !strings.Contains(string(output), "Zsh source discovery failed") {
					t.Fatalf("partial inventory must fail before validation: %v\n%s", runErr, output)
				}
			})
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

	workflow := zshSyntaxWorkflow(t)
	for _, check := range []struct {
		name   string
		script string
		want   string
	}{
		{"syntax", zshSyntaxScript(t, workflow), "Checked 3 file(s) with zsh -n, 2 failed."},
		{"compile", zshCompileScript(t, workflow), "Compiled 3 file(s), 2 failed."},
	} {
		t.Run(check.name, func(t *testing.T) {
			command := exec.Command(bash, "--noprofile", "--norc", "-c", check.script)
			command.Dir = dir
			output, runErr := command.CombinedOutput()
			if runErr == nil {
				t.Fatalf("native step must fail when a file has a syntax error:\n%s", output)
			}
			for _, name := range []string{"broken-a.zsh", "broken-b.zsh"} {
				if !strings.Contains(string(output), "::error file="+name) {
					t.Errorf("native step must annotate %s; got:\n%s", name, output)
				}
			}
			if !strings.Contains(string(output), check.want) {
				t.Errorf("native step must report the full tally; want %q in:\n%s", check.want, output)
			}
		})
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
	if _, err := os.Stat(filepath.Join(dir, filename+".zwc")); !os.IsNotExist(err) {
		t.Errorf("compilation must not leave output beside source: %v", err)
	}
	if !strings.Contains(string(output), "Compiled 1 file(s), 0 failed.") {
		t.Errorf("zcompile did not verify output for the intended filename:\n%s", output)
	}
	if runErr != nil {
		t.Errorf("zcompile workflow command failed: %v\n%s", runErr, output)
	}
	t.Run("successful command without compiled output fails", func(t *testing.T) {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "zsh"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		command := exec.Command(bash, "--noprofile", "--norc", "-c", zshCompileScript(t, zshSyntaxWorkflow(t)))
		command.Dir = dir
		command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		output, runErr := command.CombinedOutput()
		if runErr == nil || !strings.Contains(string(output), "zcompile produced no .zwc output") {
			t.Fatalf("missing compiled output must fail despite command success: %v\n%s", runErr, output)
		}
	})
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

func TestGoCIMutationIntegrationLane(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "go-ci.yml")
	vet := workflowJobStep(t, workflow, "build-test", "🔎 Vet")
	if violations := exactWorkflowMappingViolations(
		"integration-aware vet step", vet, 8,
		[]workflowMappingField{{name: "run", value: "go vet -tags=integration ./..."}},
	); len(violations) != 0 {
		t.Fatalf("vet must retain integration test coverage: %s", strings.Join(violations, "; "))
	}
	step := workflowJobStep(t, workflow, "build-test", "Test mutation-runner integration")
	if violations := exactWorkflowMappingViolations(
		"mutation-runner integration step", step, 8,
		[]workflowMappingField{{name: "run", value: "go test -tags=integration ./internal/workflowcontract/"}},
	); len(violations) != 0 {
		t.Fatalf("integration tests must run without condition or failure bypasses: %s", strings.Join(violations, "; "))
	}
	job := directWorkflowMapping(workflowBlock(t, workflow, "build-test:", 2), 4)
	for _, field := range []string{"if", "continue-on-error"} {
		if values := job[field]; len(values) != 0 {
			t.Errorf("build-test must not make integration tests optional with %s: %q", field, values)
		}
	}
	on := directWorkflowMapping(workflowBlock(t, workflow, "on:", 0), 2)
	if values := on["pull_request"]; len(values) != 1 || values[0] != "{}" {
		t.Errorf("Go CI must run integration tests for every pull request without filters; got %q", values)
	}
}

func TestGoCIForkRequiresBash53(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "go-ci.yml")
	install := workflowJobStep(t, workflow, "build-test", "Install Bash 5.3 for fork confirmation")
	fork := workflowJobStep(t, workflow, "build-test", "🧪 Test the parser fork")
	if strings.Index(workflow, install) > strings.Index(workflow, fork) {
		t.Fatal("Bash 5.3 must be prepared before required-shell tests")
	}
	script := workflowRunScript(t, install)
	const releaseDigest = "0d5cd86965f869a26cf64f4b71be7b96f90a3ba8b3d74e27e8e9d9d5550f31ba"
	if !strings.Contains(script, "https://ftp.gnu.org/gnu/bash/bash-5.3.tar.gz") || !strings.Contains(script, releaseDigest) {
		t.Fatal("the required Bash release must retain its verified download and checksum")
	}
	dir := t.TempDir()
	release := filepath.Join(dir, "release", "bash-5.3")
	bin := filepath.Join(dir, "bin")
	for _, path := range []string{release, bin} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configure := filepath.Join(release, "configure")
	if err := os.WriteFile(configure, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "release.tar.gz")
	pack := func() {
		t.Helper()
		cmd := exec.Command("tar", "-czf", archive, "-C", filepath.Dir(release), "bash-5.3")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("prepare Bash release fixture: %v\n%s", err, out)
		}
	}
	pack()
	contents, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	script = strings.Replace(script, releaseDigest, fmt.Sprintf("%x", sha256.Sum256(contents)), 1)
	shell := filepath.Join(dir, "bash53")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf 'GNU bash, version 5.3.0(1)-release\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"curl": "#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then cp \"$TEST_RELEASE\" \"$2\"; exit; fi; shift; done\nexit 1\n",
		"make": "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_MAKE_CALLS\"\nif [ \"$1\" = install ]; then mkdir -p \"$RUNNER_TEMP/fork-bash/bin\"; cp \"$TEST_BASH\" \"$RUNNER_TEMP/fork-bash/bin/bash\"; fi\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(text), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths, calls := filepath.Join(dir, "paths"), filepath.Join(dir, "make-calls")
	run := func() error {
		cmd := exec.Command("bash", "--noprofile", "--norc", "-euo", "pipefail", "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "RUNNER_TEMP="+dir, "GITHUB_PATH="+paths, "TEST_RELEASE="+archive, "TEST_BASH="+shell, "TEST_MAKE_CALLS="+calls)
		return cmd.Run()
	}
	if err := run(); err != nil {
		t.Fatalf("matching release and required Bash version must pass: %v", err)
	}
	pathText, err := os.ReadFile(paths)
	if err != nil || !strings.Contains(string(pathText), filepath.Join(dir, "fork-bash", "bin")) {
		t.Fatalf("the prepared Bash must reach subsequent steps: %v\n%s", err, pathText)
	}
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf 'GNU bash, version 5.2.0(1)-release\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := run(); err == nil {
		t.Fatal("a build producing the wrong Bash series must be refused")
	}
	before, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configure, []byte("#!/bin/sh\n# altered but valid release archive\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	pack()
	if err := run(); err == nil {
		t.Fatal("a changed release archive must fail its checksum")
	}
	after, err := os.ReadFile(calls)
	if err != nil || string(before) != string(after) {
		t.Fatalf("an unverified release must not reach a build: %v", err)
	}
}

func TestGoCIForkRequiresExternalShells(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "go-ci.yml")
	fork := workflowJobStep(t, workflow, "build-test", "🧪 Test the parser fork")
	env := workflowBlock(t, fork, "env:", 8)
	if values := directWorkflowMapping(env, 10)["REQUIRE_SHELLS"]; len(values) != 1 || values[0] != `"1"` {
		t.Fatalf("fork tests must require native shells, not silently skip them:\n%s", fork)
	}
	install := workflowJobStep(t, workflow, "build-test", "Install fork confirmation shells")
	if strings.Index(workflow, install) > strings.Index(workflow, fork) {
		t.Fatal("shell installation must precede required-shell tests")
	}
	script := workflowRunScript(t, install)
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"apt-get":  "#!/bin/sh\n[ \"$*\" = 'download dash mksh' ] || exit 1\n: > dash.deb\n: > mksh.deb\n",
		"dpkg-deb": "#!/bin/sh\n[ \"$1\" = --extract ] || exit 1\nprintf '%s\\n' \"$2\" >> \"$TEST_EXTRACTED\"\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	pathFile, extracted := filepath.Join(dir, "paths"), filepath.Join(dir, "extracted")
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "RUNNER_TEMP="+dir, "GITHUB_PATH="+pathFile, "TEST_EXTRACTED="+extracted)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell preparation failed: %v\n%s", err, out)
	}
	packages, err := os.ReadFile(extracted)
	if err != nil || !strings.Contains(string(packages), "dash.deb") || !strings.Contains(string(packages), "mksh.deb") {
		t.Fatalf("both native shells must be extracted: %v\n%s", err, packages)
	}
	paths, err := os.ReadFile(pathFile)
	if err != nil || !strings.Contains(string(paths), filepath.Join(dir, "fork-shells", "root", "bin")) {
		t.Fatalf("shell binaries must be on following steps' PATH: %v\n%s", err, paths)
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
	if values := directWorkflowMapping(onBlock, 2)["workflow_dispatch"]; len(values) != 1 || values[0] != "{}" {
		t.Fatalf("Go CI must retain manual validation; got %q", values)
	}
	push := workflowBlock(t, onBlock, "push:", 2)
	fields := directWorkflowMapping(push, 4)
	if values := fields["branches"]; len(values) != 1 || values[0] != "[main]" {
		t.Fatalf("Go CI must run on main pushes; got %q", values)
	}
	if values := fields["tags"]; len(values) != 1 || values[0] != `["v*.*.*"]` {
		t.Fatalf("Go CI must preserve native validation on release-tag pushes; got %q", values)
	}
	paths := workflowBlock(t, push, "paths:", 4)
	if got := len(exactWorkflowLineSpans(paths, `      - "**/*.zsh"`)); got != 1 {
		t.Fatalf("Go CI must run for Zsh-only main pushes; got %d matching paths", got)
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
	run := directWorkflowMapping(workflowBlock(t, config, "run:", 0), 2)
	if values := run["build-tags"]; len(values) != 1 || values[0] != "[integration]" {
		t.Fatalf("lint must retain integration test coverage; got build-tags %q", values)
	}
	for _, setting := range []string{"max-issues-per-linter", "max-same-issues"} {
		if got := len(exactWorkflowLineSpans(config, "  "+setting+": 0")); got != 1 {
			t.Fatalf(".golangci.yml must set %s to 0 so no finding is hidden (#380); got %d matching lines", setting, got)
		}
	}
}

// Native checks must fail the existing required build-test context, not report
// a separate optional success or inherit a conditional/failure bypass.
func TestGoCINativeChecksRunWithoutBypasses(t *testing.T) {
	workflow := zshSyntaxWorkflow(t)
	setup := workflowJobStep(t, workflow, "build-test", "⚡ Set up Zsh")
	previous := strings.Index(workflow, setup)
	for _, name := range []string{"⚡ zsh -n", "⚡ zcompile"} {
		step := workflowJobStep(t, workflow, "build-test", name)
		if violations := exactWorkflowMappingViolations(name, step, 8,
			[]workflowMappingField{{name: "run", value: "|"}, {name: "shell", value: "bash"}}); len(violations) != 0 {
			t.Fatalf("native checks must run without condition or failure bypasses: %s", strings.Join(violations, "; "))
		}
		position := strings.Index(workflow, step)
		if position <= previous {
			t.Fatalf("%s must follow Zsh setup and the preceding native check", name)
		}
		previous = position
	}
	fields := directWorkflowMapping(workflowBlock(t, workflow, "build-test:", 2), 4)
	for _, field := range []string{"name", "if", "continue-on-error", "strategy"} {
		if values := fields[field]; len(values) != 0 {
			t.Errorf("native checks must retain unconditional build-test context without %s: %q", field, values)
		}
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
		[]workflowMappingField{{name: "go-version", value: `"1.27"`}},
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
		[]workflowMappingField{{name: "run", value: "go test -tags=integration -count=1 ./..."}},
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
          go-version: "1.27"`,
		`        if: false
        with:
          go-version: "1.24"
          # go-version: "1.27"`,
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
				"        run: go test -tags=integration -count=1 ./...",
				"        continue-on-error: true\n        run: go test -tags=integration -count=1 ./...",
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
