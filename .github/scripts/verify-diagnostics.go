// Diagnostic comparison runner using the project's configuration resolver.
package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

// EvidenceError indicates the comparison cannot establish trustworthy evidence.
type EvidenceError string

func (e EvidenceError) Error() string {
	return string(e)
}

// Finding represents a single diagnostic entry.
type Finding map[string]any

// DiagnosticSummary matches the summary structure of zsh-lint JSON output.
type DiagnosticSummary struct {
	Files       int `json:"files"`
	Diagnostics int `json:"diagnostics"`
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
	Infos       int `json:"infos"`
	Hints       int `json:"hints"`
}

// DiagnosticOutput represents the envelope produced by zsh-lint --format=json.
type DiagnosticOutput struct {
	Version     int               `json:"version"`
	Diagnostics []Finding         `json:"diagnostics"`
	Summary     DiagnosticSummary `json:"summary"`
}

// InputsResult tracks hashes for input files and effective configurations.
type InputsResult struct {
	Files                  map[string]string          `json:"files"`
	Configurations         map[string]string          `json:"configurations"`
	EffectiveConfiguration map[string]string          `json:"effective_configuration"`
	Resolutions            map[string]InputResolution `json:"resolutions"`
}

// InputResolution distinguishes discovered configuration from active metadata.
type InputResolution struct {
	Configured              bool                      `json:"configured"`
	DiscoveredConfiguration string                    `json:"discovered_configuration,omitempty"`
	Profile                 projectconfig.Profile     `json:"profile,omitempty"`
	Role                    projectconfig.SourceRole  `json:"role,omitempty"`
	ConfigVersion           int                       `json:"config_version,omitempty"`
	ProjectKind             projectconfig.ProjectKind `json:"project_kind,omitempty"`
	ProjectIdentifier       string                    `json:"project_identifier,omitempty"`
	MinimumZsh              string                    `json:"minimum_zsh,omitempty"`
	SourceRoot              string                    `json:"source_root,omitempty"`
	FallbackReason          string                    `json:"fallback_reason,omitempty"`
}

// RuleDelta tracks added and removed findings for a single rule.
type RuleDelta struct {
	Removed []Finding `json:"removed"`
	Added   []Finding `json:"added"`
}

// CompareResult stores the multiset comparison outcome across rules.
type CompareResult struct {
	PerRule            map[string]*RuleDelta `json:"per_rule"`
	NonTargetIdentical *bool                 `json:"non_target_identical"`
}

// CommandRow records a single executed subprocess and its artifacts.
type CommandRow struct {
	Args       []string `json:"args"`
	Cwd        string   `json:"cwd"`
	TimedOut   bool     `json:"timed_out,omitempty"`
	Returncode int      `json:"returncode"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
}

// Commands executes subprocesses with process-group timeouts and captures logs.
type Commands struct {
	output  string
	timeout time.Duration
	rows    []CommandRow
}

func newCommands(output string, timeout time.Duration) *Commands {
	return &Commands{
		output:  output,
		timeout: timeout,
		rows:    []CommandRow{},
	}
}

func (c *Commands) run(args []string, cwd string, allowed []int) ([]byte, []byte, error) {
	number := len(c.rows) + 1
	row := CommandRow{
		Args: args,
		Cwd:  cwd,
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdoutBuf, stderrBuf bytes.Buffer
	// Own the pipes so process exit and output completion can be bounded
	// separately. Cmd.WaitDelay can hide a drain error behind an allowed exit 1.
	stdoutRead, stdoutWrite, startErr := os.Pipe()
	var stderrRead, stderrWrite *os.File
	if startErr == nil {
		defer stdoutRead.Close()
		defer stdoutWrite.Close()
		stderrRead, stderrWrite, startErr = os.Pipe()
		if startErr == nil {
			defer stderrRead.Close()
			defer stderrWrite.Close()
		}
	}
	cmd.Stdout, cmd.Stderr = stdoutWrite, stderrWrite

	var timedOut bool
	var waitErr, streamErr error
	if startErr == nil {
		startErr = cmd.Start()
	}
	if startErr != nil {
		waitErr = startErr
		fmt.Fprintln(&stderrBuf, startErr)
	} else {
		streamErr = errors.Join(stdoutWrite.Close(), stderrWrite.Close())
		drained := make(chan error, 2)
		go func() {
			_, err := io.Copy(&stdoutBuf, stdoutRead)
			drained <- err
		}()
		go func() {
			_, err := io.Copy(&stderrBuf, stderrRead)
			drained <- err
		}()
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		if c.timeout > 0 {
			select {
			case <-time.After(c.timeout):
				timedOut = true
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				waitErr = <-done
			case waitErr = <-done:
			}
		} else {
			waitErr = <-done
		}
		var drainTimeout <-chan time.Time
		if c.timeout > 0 {
			timer := time.NewTimer(min(c.timeout, time.Second))
			defer timer.Stop()
			drainTimeout = timer.C
		}
		for remaining := 2; remaining > 0; {
			select {
			case err := <-drained:
				streamErr = errors.Join(streamErr, err)
				remaining--
			case <-drainTimeout:
				timedOut = true
				_ = stdoutRead.Close()
				_ = stderrRead.Close()
				drainTimeout = nil
			}
		}
		if streamErr != nil {
			fmt.Fprintln(&stderrBuf, "capture command output:", streamErr)
		}
	}

	if timedOut {
		row.TimedOut = true
	}

	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() {
					row.Returncode = -int(status.Signal())
				} else {
					row.Returncode = status.ExitStatus()
				}
			} else {
				row.Returncode = exitErr.ExitCode()
			}
		} else {
			row.Returncode = 1
		}
	} else {
		row.Returncode = 0
	}

	stdoutName := fmt.Sprintf("command-%d.stdout", number)
	stderrName := fmt.Sprintf("command-%d.stderr", number)

	stdoutErr := os.WriteFile(filepath.Join(c.output, stdoutName), stdoutBuf.Bytes(), 0644)
	stderrErr := os.WriteFile(filepath.Join(c.output, stderrName), stderrBuf.Bytes(), 0644)

	row.Stdout = stdoutName
	row.Stderr = stderrName

	c.rows = append(c.rows, row)

	cmdData, marshalErr := json.MarshalIndent(c.rows, "", "  ")
	cmdData = append(cmdData, '\n')
	logErr := os.WriteFile(filepath.Join(c.output, "commands.json"), cmdData, 0644)
	if err := errors.Join(stdoutErr, stderrErr, marshalErr, logErr); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("write command evidence: %w", err)
	}

	isAllowed := false
	for _, code := range allowed {
		if row.Returncode == code {
			isAllowed = true
			break
		}
	}

	if startErr != nil || streamErr != nil || row.TimedOut || !isAllowed {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), EvidenceError(
			fmt.Sprintf("command %d failed: see %s", number, filepath.Join(c.output, "commands.json")),
		)
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

func sha256Bytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Bytes(data), nil
}

func loadPopulation(manifestPath string, roots map[string]string) (string, []string, map[string]string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", nil, nil, err
	}

	var rawMap map[string]any
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return "", nil, nil, EvidenceError(fmt.Sprintf("invalid population manifest: %s", manifestPath))
	}

	versionVal, hasVersion := rawMap["version"]
	modeVal, hasMode := rawMap["mode"]
	filesVal, hasFiles := rawMap["files"]

	if !hasVersion || !hasMode || !hasFiles {
		return "", nil, nil, EvidenceError(fmt.Sprintf("invalid population manifest: %s", manifestPath))
	}

	verFloat, ok := versionVal.(float64)
	if !ok || verFloat != 1 {
		return "", nil, nil, EvidenceError(fmt.Sprintf("invalid population manifest: %s", manifestPath))
	}

	modeStr, ok := modeVal.(string)
	if !ok || (modeStr != "no-config" && modeStr != "configured" && modeStr != "configured-only") {
		return "", nil, nil, EvidenceError(fmt.Sprintf("invalid population manifest: %s", manifestPath))
	}

	filesSlice, ok := filesVal.([]any)
	if !ok || len(filesSlice) == 0 {
		return "", nil, nil, EvidenceError(fmt.Sprintf("empty or duplicate population: %s", manifestPath))
	}

	seenEntries := make(map[string]bool)
	var rawNames []string
	for _, f := range filesSlice {
		s, ok := f.(string)
		if !ok || seenEntries[s] {
			return "", nil, nil, EvidenceError(fmt.Sprintf("empty or duplicate population: %s", manifestPath))
		}
		seenEntries[s] = true
		rawNames = append(rawNames, s)
	}

	resolved := make(map[string]string)
	var resolvedOrder []string
	seenResolved := make(map[string]bool)

	for _, name := range rawNames {
		alias, relative, hasSep := strings.Cut(name, ":")
		if !hasSep || alias == "" || relative == "" {
			return "", nil, nil, EvidenceError(fmt.Sprintf("invalid or unmapped input: %s", name))
		}
		rootPath, exists := roots[alias]
		if !exists {
			return "", nil, nil, EvidenceError(fmt.Sprintf("invalid or unmapped input: %s", name))
		}
		if filepath.IsAbs(relative) {
			return "", nil, nil, EvidenceError(fmt.Sprintf("invalid or unmapped input: %s", name))
		}
		for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
			if part == ".." {
				return "", nil, nil, EvidenceError(fmt.Sprintf("invalid or unmapped input: %s", name))
			}
		}

		target := filepath.Join(rootPath, relative)
		fi, err := os.Stat(target)
		if err != nil {
			return "", nil, nil, err
		}
		if !fi.Mode().IsRegular() {
			return "", nil, nil, EvidenceError(fmt.Sprintf("input escapes root or is not a file: %s", name))
		}

		resPath, err := filepath.EvalSymlinks(target)
		if err != nil {
			return "", nil, nil, err
		}
		resPath, err = filepath.Abs(resPath)
		if err != nil {
			return "", nil, nil, err
		}

		resRoot, err := filepath.EvalSymlinks(rootPath)
		if err != nil {
			return "", nil, nil, err
		}
		resRoot, err = filepath.Abs(resRoot)
		if err != nil {
			return "", nil, nil, err
		}

		relCheck, err := filepath.Rel(resRoot, resPath)
		if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
			return "", nil, nil, EvidenceError(fmt.Sprintf("input escapes root or is not a file: %s", name))
		}

		fiRes, err := os.Stat(resPath)
		if err != nil || !fiRes.Mode().IsRegular() {
			return "", nil, nil, EvidenceError(fmt.Sprintf("input escapes root or is not a file: %s", name))
		}

		if seenResolved[resPath] {
			return "", nil, nil, EvidenceError(fmt.Sprintf("duplicate resolved input: %s", name))
		}
		seenResolved[resPath] = true
		resolved[name] = resPath
		resolvedOrder = append(resolvedOrder, name)
	}

	return modeStr, resolvedOrder, resolved, nil
}

func computeInputs(files map[string]string, configured bool) (*InputsResult, error) {
	result := &InputsResult{
		Files:                  make(map[string]string),
		Configurations:         make(map[string]string),
		EffectiveConfiguration: make(map[string]string),
		Resolutions:            make(map[string]InputResolution),
	}
	configs := make(map[string]*projectconfig.Config)

	for name, path := range files {
		h, err := sha256File(path)
		if err != nil {
			return nil, err
		}
		result.Files[name] = h

		resolution := InputResolution{FallbackReason: "configuration-disabled"}
		if !configured {
			result.Resolutions[name] = resolution
			continue
		}
		foundConfig, err := projectconfig.Discover(path)
		if err != nil {
			return nil, fmt.Errorf("discover %s: %w", name, err)
		}
		resolution = InputResolution{DiscoveredConfiguration: foundConfig, FallbackReason: "no-configuration"}
		if foundConfig == "" {
			result.Resolutions[name] = resolution
			continue
		}
		config, ok := configs[foundConfig]
		if !ok {
			hCfg, err := sha256File(foundConfig)
			if err != nil {
				return nil, err
			}
			result.Configurations[foundConfig] = hCfg
			config, err = projectconfig.Load(foundConfig)
			if err != nil {
				return nil, fmt.Errorf("configuration for %s: %w", name, err)
			}
			configs[foundConfig] = config
		}
		context, err := config.Resolve(path)
		if err != nil {
			if !projectconfig.IsUnmatchedSource(err) {
				return nil, fmt.Errorf("configuration for %s: %w", name, err)
			}
			resolution.FallbackReason = "unmatched-source-root"
		} else {
			resolution.Configured = true
			resolution.FallbackReason = ""
			resolution.Profile = context.Profile
			resolution.Role = context.Role
			resolution.ConfigVersion = context.ConfigVersion
			resolution.ProjectKind = context.ProjectKind
			resolution.ProjectIdentifier = context.ProjectIdentifier
			resolution.MinimumZsh = context.MinimumZsh
			resolution.SourceRoot = context.SourceRoot
			result.EffectiveConfiguration[name] = foundConfig
		}
		result.Resolutions[name] = resolution
	}

	return result, nil
}

func populationInputs(files map[string]string, mode string) (*InputsResult, error) {
	inputs, err := computeInputs(files, mode != "no-config")
	if err != nil {
		return nil, err
	}
	if mode == "configured-only" {
		for name, resolution := range inputs.Resolutions {
			if !resolution.Configured {
				return nil, EvidenceError(fmt.Sprintf("configured-only input %s uses fallback: %s", name, resolution.FallbackReason))
			}
		}
	}
	return inputs, nil
}

func equalInputs(a, b *InputsResult) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(a.Files, b.Files) &&
		reflect.DeepEqual(a.Configurations, b.Configurations) &&
		reflect.DeepEqual(a.EffectiveConfiguration, b.EffectiveConfiguration) &&
		reflect.DeepEqual(a.Resolutions, b.Resolutions)
}

func validateOutput(raw []byte, expectedFiles map[string]bool) (*DiagnosticOutput, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()

	var rawMap map[string]any
	if err := d.Decode(&rawMap); err != nil {
		return nil, EvidenceError("invalid diagnostic envelope")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, EvidenceError("trailing diagnostic output")
	}

	verNum, ok := rawMap["version"].(json.Number)
	if !ok || verNum.String() != "1" {
		return nil, EvidenceError("invalid diagnostic envelope")
	}

	rawDiag, ok := rawMap["diagnostics"].([]any)
	if !ok {
		return nil, EvidenceError("invalid diagnostic envelope")
	}

	diagnostics := []Finding{}
	counts := map[string]int{
		"errors":   0,
		"warnings": 0,
		"infos":    0,
		"hints":    0,
	}

	for _, item := range rawDiag {
		f, ok := item.(map[string]any)
		if !ok {
			return nil, EvidenceError("invalid diagnostic fields")
		}
		for _, k := range []string{"file", "rule", "severity", "message"} {
			v, ok := f[k].(string)
			if !ok || v == "" {
				return nil, EvidenceError("invalid diagnostic fields")
			}
		}
		file := f["file"].(string)
		severity := f["severity"].(string)

		if !expectedFiles[file] {
			return nil, EvidenceError("unknown diagnostic file or severity")
		}
		if severity != "error" && severity != "warning" && severity != "info" && severity != "hint" {
			return nil, EvidenceError("unknown diagnostic file or severity")
		}

		if rawRange, hasRange := f["range"]; hasRange {
			bounds, ok := rawRange.(map[string]any)
			if !ok || len(bounds) != 2 || bounds["start"] == nil || bounds["end"] == nil {
				return nil, EvidenceError("invalid diagnostic range")
			}
			for _, posKey := range []string{"start", "end"} {
				pos, ok := bounds[posKey].(map[string]any)
				if !ok || len(pos) != 3 {
					return nil, EvidenceError("invalid diagnostic position")
				}
				for _, coordKey := range []string{"line", "column", "offset"} {
					num, ok := pos[coordKey].(json.Number)
					if !ok {
						return nil, EvidenceError("invalid diagnostic position")
					}
					val, err := strconv.ParseInt(string(num), 10, 64)
					if err != nil || val < 0 || strings.Contains(string(num), ".") {
						return nil, EvidenceError("invalid diagnostic position")
					}
				}
			}
		}
		counts[severity+"s"]++
		diagnostics = append(diagnostics, Finding(f))
	}

	summaryMap, ok := rawMap["summary"].(map[string]any)
	if !ok || len(summaryMap) != 6 {
		return nil, EvidenceError("diagnostic summary does not match inspected inputs and findings")
	}

	expectedSummary := map[string]int{
		"files":       len(expectedFiles),
		"diagnostics": len(diagnostics),
		"errors":      counts["errors"],
		"warnings":    counts["warnings"],
		"infos":       counts["infos"],
		"hints":       counts["hints"],
	}

	for k, exp := range expectedSummary {
		num, ok := summaryMap[k].(json.Number)
		if !ok {
			return nil, EvidenceError("diagnostic summary does not match inspected inputs and findings")
		}
		val, err := strconv.ParseInt(string(num), 10, 64)
		if err != nil || int(val) != exp || strings.Contains(string(num), ".") {
			return nil, EvidenceError("diagnostic summary does not match inspected inputs and findings")
		}
	}

	summary := DiagnosticSummary{
		Files:       expectedSummary["files"],
		Diagnostics: expectedSummary["diagnostics"],
		Errors:      expectedSummary["errors"],
		Warnings:    expectedSummary["warnings"],
		Infos:       expectedSummary["infos"],
		Hints:       expectedSummary["hints"],
	}

	return &DiagnosticOutput{
		Version:     1,
		Diagnostics: diagnostics,
		Summary:     summary,
	}, nil
}

func sourceFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(out), "\x00")
	var result []string
	seen := make(map[string]bool)
	for _, p := range parts {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		fi, err := os.Stat(filepath.Join(root, p))
		if err == nil && fi.Mode().IsRegular() {
			result = append(result, p)
		}
	}
	sort.Strings(result)
	return result, nil
}

func fingerprint(root string) (map[string]string, error) {
	files, err := sourceFiles(root)
	if err != nil {
		return nil, err
	}
	res := make(map[string]string)
	for _, p := range files {
		h, err := sha256File(filepath.Join(root, p))
		if err != nil {
			return nil, err
		}
		res[p] = h
	}
	return res, nil
}

func build(commands *Commands, root, revision, destination string, working bool) (string, error) {
	if err := os.Mkdir(destination, 0755); err != nil {
		return "", err
	}
	if working {
		files, err := sourceFiles(root)
		if err != nil {
			return "", err
		}
		for _, name := range files {
			src := filepath.Join(root, name)
			dst := filepath.Join(destination, name)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return "", err
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return "", err
			}
			fi, err := os.Stat(src)
			mode := os.FileMode(0644)
			if err == nil {
				mode = fi.Mode().Perm()
			}
			if err := os.WriteFile(dst, data, mode); err != nil {
				return "", err
			}
		}
	} else {
		archive := destination + ".tar"
		if _, _, err := commands.run([]string{"git", "archive", "--format=tar", "--output", archive, revision}, root, []int{0}); err != nil {
			return "", err
		}
		tarFile, err := os.Open(archive)
		if err != nil {
			return "", err
		}
		defer tarFile.Close()

		tr := tar.NewReader(tarFile)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			cleaned := filepath.Clean(header.Name)
			if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
				return "", EvidenceError("archive entry escapes destination: " + header.Name)
			}
			target := filepath.Join(destination, cleaned)
			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.MkdirAll(target, 0755); err != nil {
					return "", err
				}
			case tar.TypeReg, tar.TypeRegA:
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					return "", err
				}
				outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode().Perm())
				if err != nil {
					return "", err
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					outFile.Close()
					return "", err
				}
				outFile.Close()
			}
		}
	}

	binary := filepath.Join(filepath.Dir(destination), filepath.Base(destination)+"-cli")
	if _, _, err := commands.run([]string{"go", "build", "-buildvcs=false", "-o", binary, "./cmd/zsh-lint"}, destination, []int{0}); err != nil {
		return "", err
	}
	return binary, nil
}

var buildCLI = build

func compare(base, candidate []Finding, target string) CompareResult {
	before := make(map[string]int)
	after := make(map[string]int)
	findingMap := make(map[string]Finding)

	for _, d := range base {
		encoded, err := json.Marshal(d)
		if err != nil {
			continue
		}
		s := string(encoded)
		before[s]++
		findingMap[s] = d
	}
	for _, d := range candidate {
		encoded, err := json.Marshal(d)
		if err != nil {
			continue
		}
		s := string(encoded)
		after[s]++
		findingMap[s] = d
	}

	removedDelta := make(map[string]int)
	for k, count := range before {
		if count > after[k] {
			removedDelta[k] = count - after[k]
		}
	}
	addedDelta := make(map[string]int)
	for k, count := range after {
		if count > before[k] {
			addedDelta[k] = count - before[k]
		}
	}

	perRule := make(map[string]*RuleDelta)

	var sortedRemoved []string
	for k := range removedDelta {
		sortedRemoved = append(sortedRemoved, k)
	}
	sort.Strings(sortedRemoved)
	for _, encoded := range sortedRemoved {
		d := findingMap[encoded]
		rule, _ := d["rule"].(string)
		if perRule[rule] == nil {
			perRule[rule] = &RuleDelta{Removed: []Finding{}, Added: []Finding{}}
		}
		for i := 0; i < removedDelta[encoded]; i++ {
			perRule[rule].Removed = append(perRule[rule].Removed, d)
		}
	}

	var sortedAdded []string
	for k := range addedDelta {
		sortedAdded = append(sortedAdded, k)
	}
	sort.Strings(sortedAdded)
	for _, encoded := range sortedAdded {
		d := findingMap[encoded]
		rule, _ := d["rule"].(string)
		if perRule[rule] == nil {
			perRule[rule] = &RuleDelta{Removed: []Finding{}, Added: []Finding{}}
		}
		for i := 0; i < addedDelta[encoded]; i++ {
			perRule[rule].Added = append(perRule[rule].Added, d)
		}
	}

	var nonTargetIdentical *bool
	if target != "" {
		identical := true
		for rule := range perRule {
			if rule != target {
				identical = false
				break
			}
		}
		nonTargetIdentical = &identical
	}

	return CompareResult{
		PerRule:            perRule,
		NonTargetIdentical: nonTargetIdentical,
	}
}

func writeManifest(argv []string) int {
	var listPath, relativeTo, mode, outputPath string
	var rootFlags []string

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--list" && i+1 < len(argv) {
			listPath = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--list=") {
			listPath = strings.TrimPrefix(arg, "--list=")
		} else if arg == "--root" && i+1 < len(argv) {
			rootFlags = append(rootFlags, argv[i+1])
			i++
		} else if strings.HasPrefix(arg, "--root=") {
			rootFlags = append(rootFlags, strings.TrimPrefix(arg, "--root="))
		} else if arg == "--relative-to" && i+1 < len(argv) {
			relativeTo = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--relative-to=") {
			relativeTo = strings.TrimPrefix(arg, "--relative-to=")
		} else if arg == "--mode" && i+1 < len(argv) {
			mode = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--mode=") {
			mode = strings.TrimPrefix(arg, "--mode=")
		} else if arg == "--output" && i+1 < len(argv) {
			outputPath = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--output=") {
			outputPath = strings.TrimPrefix(arg, "--output=")
		} else {
			fmt.Fprintf(os.Stderr, "unknown argument: %s\n", arg)
			return 2
		}
	}

	if listPath == "" || len(rootFlags) == 0 || relativeTo == "" || mode == "" || outputPath == "" {
		fmt.Fprintln(os.Stderr, "missing required manifest generation arguments")
		return 2
	}
	if mode != "no-config" && mode != "configured" && mode != "configured-only" {
		fmt.Fprintln(os.Stderr, "mode must be no-config, configured or configured-only")
		return 2
	}

	roots := make(map[string]string)
	for _, item := range rootFlags {
		alias, path, ok := strings.Cut(item, "=")
		if !ok || alias == "" || path == "" || roots[alias] != "" {
			fmt.Fprintln(os.Stderr, "roots must be unique ALIAS=DIR pairs")
			return 2
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		roots[alias] = resolved
	}

	relativeRoot, ok := roots[relativeTo]
	if !ok {
		fmt.Fprintln(os.Stderr, "relative-to must name a mapped root")
		return 2
	}

	rawBytes, err := os.ReadFile(listPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	raw := string(rawBytes)
	var names []string
	if strings.Contains(raw, "\x00") {
		names = strings.Split(raw, "\x00")
	} else {
		names = strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	}

	var files []string
	seenFiles := make(map[string]bool)

	for _, name := range names {
		if name == "" {
			continue
		}
		var targetPath string
		if filepath.IsAbs(name) {
			targetPath = name
		} else {
			targetPath = filepath.Join(relativeRoot, name)
		}
		resolvedPath, err := filepath.EvalSymlinks(targetPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "not a file: %s\n", name)
			return 2
		}
		resolvedPath, err = filepath.Abs(resolvedPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "not a file: %s\n", name)
			return 2
		}
		fi, err := os.Stat(resolvedPath)
		if err != nil || !fi.Mode().IsRegular() {
			fmt.Fprintf(os.Stderr, "not a file: %s\n", name)
			return 2
		}

		type rootMatch struct {
			alias string
			root  string
			parts int
		}
		var matches []rootMatch
		for alias, root := range roots {
			rel, err := filepath.Rel(root, resolvedPath)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				parts := len(strings.Split(filepath.Clean(root), string(filepath.Separator)))
				matches = append(matches, rootMatch{alias: alias, root: root, parts: parts})
			}
		}
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "unmapped input: %s\n", name)
			return 2
		}
		best := matches[0]
		for _, m := range matches[1:] {
			if m.parts > best.parts {
				best = m
			}
		}
		relPath, _ := filepath.Rel(best.root, resolvedPath)
		entry := fmt.Sprintf("%s:%s", best.alias, filepath.ToSlash(relPath))
		if seenFiles[entry] {
			fmt.Fprintln(os.Stderr, "empty or duplicate population")
			return 2
		}
		seenFiles[entry] = true
		files = append(files, entry)
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "empty or duplicate population")
		return 2
	}

	outF, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer outF.Close()

	type manifestData struct {
		Version int      `json:"version"`
		Mode    string   `json:"mode"`
		Files   []string `json:"files"`
	}
	m := manifestData{
		Version: 1,
		Mode:    mode,
		Files:   files,
	}
	enc := json.NewEncoder(outF)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func runMain(argv []string) int {
	if len(argv) > 0 && argv[0] == "--write-manifest" {
		return writeManifest(argv[1:])
	}

	var base, candidate, targetRule, output string
	timeout := 600
	var populationFlags, rootFlags []string

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--candidate" && i+1 < len(argv) {
			candidate = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--candidate=") {
			candidate = strings.TrimPrefix(arg, "--candidate=")
		} else if arg == "--target-rule" && i+1 < len(argv) {
			targetRule = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--target-rule=") {
			targetRule = strings.TrimPrefix(arg, "--target-rule=")
		} else if arg == "--output" && i+1 < len(argv) {
			output = argv[i+1]
			i++
		} else if strings.HasPrefix(arg, "--output=") {
			output = strings.TrimPrefix(arg, "--output=")
		} else if arg == "--timeout" && i+1 < len(argv) {
			t, err := strconv.Atoi(argv[i+1])
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid timeout")
				return 2
			}
			timeout = t
			i++
		} else if strings.HasPrefix(arg, "--timeout=") {
			t, err := strconv.Atoi(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid timeout")
				return 2
			}
			timeout = t
		} else if arg == "--population" && i+1 < len(argv) {
			populationFlags = append(populationFlags, argv[i+1])
			i++
		} else if strings.HasPrefix(arg, "--population=") {
			populationFlags = append(populationFlags, strings.TrimPrefix(arg, "--population="))
		} else if arg == "--root" && i+1 < len(argv) {
			rootFlags = append(rootFlags, argv[i+1])
			i++
		} else if strings.HasPrefix(arg, "--root=") {
			rootFlags = append(rootFlags, strings.TrimPrefix(arg, "--root="))
		} else if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
			return 2
		} else {
			if base == "" {
				base = arg
			} else {
				fmt.Fprintf(os.Stderr, "multiple base arguments: %s and %s\n", base, arg)
				return 2
			}
		}
	}

	report := map[string]any{
		"version":     1,
		"status":      "error",
		"populations": map[string]any{},
	}

	if output == "" {
		fmt.Fprintln(os.Stderr, "flag --output is required")
		return 2
	}
	if timeout <= 0 {
		fmt.Fprintln(os.Stderr, "timeout must be positive")
		return 2
	}

	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOut, err := rootCmd.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to find git root")
		return 2
	}
	root := strings.TrimSpace(string(rootOut))
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	root, err = filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	outputAbs, err := filepath.Abs(output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	outputParent, err := filepath.EvalSymlinks(filepath.Dir(outputAbs))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	outputAbs = filepath.Join(outputParent, filepath.Base(outputAbs))

	relCheck, err := filepath.Rel(root, outputAbs)
	if err == nil && relCheck != ".." && !strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		fmt.Fprintln(os.Stderr, "output must be outside the checkout")
		return 2
	}

	if err := os.Mkdir(outputAbs, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	created := true

	var populations []struct {
		name         string
		mode         string
		fileNames    []string
		files        map[string]string
		initial      *InputsResult
		manifestPath string
		manifestHash string
	}
	reportPopulations := make(map[string]map[string]any)

	finish := func() int {
		if created {
			baseVal, _ := report["base"].(string)
			if baseVal == "" {
				baseVal = "unknown"
			}
			candVal, _ := report["candidate"].(string)
			if candVal == "" {
				candVal = "unknown"
			}

			lines := []string{
				fmt.Sprintf("Diagnostic comparison: %s", report["status"]),
				fmt.Sprintf("Base: %s", baseVal),
				fmt.Sprintf("Candidate: %s", candVal),
			}
			for _, pop := range populations {
				row, ok := reportPopulations[pop.name]
				if !ok {
					continue
				}
				perRule, ok := row["per_rule"].(map[string]*RuleDelta)
				if !ok {
					continue
				}
				baseRow, _ := row["base"].(map[string]any)
				candRow, _ := row["candidate"].(map[string]any)
				baseSummary, _ := baseRow["summary"].(DiagnosticSummary)
				candSummary, _ := candRow["summary"].(DiagnosticSummary)
				basePE, _ := baseRow["parse_errors"].(int)
				candPE, _ := candRow["parse_errors"].(int)

				var ntiStr string
				if nti, ok := row["non_target_identical"].(*bool); ok && nti != nil {
					if *nti {
						ntiStr = "True"
					} else {
						ntiStr = "False"
					}
				} else {
					ntiStr = "None"
				}

				lines = append(lines, fmt.Sprintf(
					"%s: %d -> %d diagnostics; parse errors %d -> %d; non-target-identical=%s",
					pop.name, baseSummary.Diagnostics, candSummary.Diagnostics, basePE, candPE, ntiStr,
				))
				configuredCount := 0
				for _, resolution := range pop.initial.Resolutions {
					if resolution.Configured {
						configuredCount++
					}
				}
				lines = append(lines, fmt.Sprintf("  mode=%s; configured=%d; unconfigured=%d", pop.mode, configuredCount, len(pop.initial.Resolutions)-configuredCount))
				if pop.mode == "configured" {
					lines = append(lines, "  Historical mixed-mode compatibility evidence, not a configured-only population.")
				}
				for _, lane := range []struct {
					name     string
					warnings int
				}{{"base", baseSummary.Warnings}, {"candidate", candSummary.Warnings}} {
					if lane.warnings == 0 {
						lines = append(lines, fmt.Sprintf("  %s Warning precision: undefined (zero findings); the 80%% goal is not established.", lane.name))
					} else {
						lines = append(lines, fmt.Sprintf("  %s: %d Warning findings require judgments; diagnostic counts do not establish precision.", lane.name, lane.warnings))
					}
				}

				var sortedRules []string
				for r := range perRule {
					sortedRules = append(sortedRules, r)
				}
				sort.Strings(sortedRules)
				for _, r := range sortedRules {
					delta := perRule[r]
					lines = append(lines, fmt.Sprintf("  %s: %d removed, %d added", r, len(delta.Removed), len(delta.Added)))
				}
			}
			if errStr, ok := report["error"].(string); ok && errStr != "" {
				lines = append(lines, errStr)
			}
			summaryText := strings.Join(lines, "\n") + "\n"
			summaryErr := os.WriteFile(filepath.Join(outputAbs, "summary.md"), []byte(summaryText), 0644)
			if summaryErr != nil {
				report["status"] = "error"
				report["error"] = fmt.Sprintf("write diagnostic summary: %v", summaryErr)
			}
			diagBytes, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "encode diagnostic evidence: %v\n", err)
				return 2
			}
			diagBytes = append(diagBytes, '\n')
			if err := os.WriteFile(filepath.Join(outputAbs, "diagnostics.json"), diagBytes, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "write diagnostic evidence: %v\n", err)
				return 2
			}
			if summaryErr != nil {
				fmt.Fprintln(os.Stderr, report["error"])
				return 2
			}

			fmt.Print(summaryText)
			fmt.Printf("Evidence: %s\n", filepath.Join(outputAbs, "diagnostics.json"))
		} else {
			if errStr, ok := report["error"].(string); ok && errStr != "" {
				fmt.Fprintln(os.Stderr, errStr)
			}
		}

		statusStr, _ := report["status"].(string)
		switch statusStr {
		case "pass":
			return 0
		case "fail":
			return 1
		default:
			return 2
		}
	}

	commands := newCommands(outputAbs, time.Duration(timeout)*time.Second)

	verifyScript := filepath.Join(root, ".github", "scripts", "verify-parser-change.sh")
	guard := []string{"bash", verifyScript, "--check-base"}
	if candidate != "" {
		guard = append(guard, "--candidate", candidate)
	}
	if base != "" {
		guard = append(guard, base)
	}
	rawGuard, _, err := commands.run(guard, root, []int{0})
	if err != nil {
		report["error"] = err.Error()
		return finish()
	}

	var ids []string
	for _, line := range strings.Split(string(rawGuard), "\n") {
		if strings.HasPrefix(line, "verify: base ") || strings.HasPrefix(line, "verify: candidate ") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				ids = append(ids, fields[2])
			}
		}
	}
	if len(ids) != 2 {
		report["error"] = "base guard did not identify both revisions"
		return finish()
	}

	report["base"] = ids[0]
	report["candidate"] = ids[1]
	report["working_tree"] = (candidate == "")

	roots := make(map[string]string)
	for _, item := range rootFlags {
		alias, path, ok := strings.Cut(item, "=")
		if !ok || alias == "" || path == "" || roots[alias] != "" {
			report["error"] = "roots must be unique ALIAS=DIR pairs"
			return finish()
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			report["error"] = err.Error()
			return finish()
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			report["error"] = err.Error()
			return finish()
		}
		roots[alias] = resolved
	}

	popMap := make(map[string]bool)
	for _, item := range populationFlags {
		name, manifest, ok := strings.Cut(item, "=")
		if !ok || name == "" || manifest == "" || popMap[name] {
			report["error"] = "populations must be unique NAME=MANIFEST pairs"
			return finish()
		}
		popMap[name] = true
		mode, fileNames, files, err := loadPopulation(manifest, roots)
		if err != nil {
			report["error"] = err.Error()
			return finish()
		}
		mHash, err := sha256File(manifest)
		if err != nil {
			report["error"] = err.Error()
			return finish()
		}
		initInputs, err := populationInputs(files, mode)
		if err != nil {
			report["error"] = err.Error()
			return finish()
		}
		populations = append(populations, struct {
			name         string
			mode         string
			fileNames    []string
			files        map[string]string
			initial      *InputsResult
			manifestPath string
			manifestHash string
		}{
			name:         name,
			mode:         mode,
			fileNames:    fileNames,
			files:        files,
			initial:      initInputs,
			manifestPath: manifest,
			manifestHash: mHash,
		})
	}

	if len(populations) == 0 {
		report["error"] = "at least one population is required"
		return finish()
	}

	frozen, err := fingerprint(root)
	if err != nil {
		report["error"] = err.Error()
		return finish()
	}
	frozenJSON, _ := json.Marshal(frozen)
	report["source_sha256"] = sha256Bytes(frozenJSON)
	report["roots"] = roots

	baseBinary, err := buildCLI(commands, root, ids[0], filepath.Join(outputAbs, "base-source"), false)
	if err != nil {
		report["error"] = err.Error()
		return finish()
	}
	candidateBinary, err := buildCLI(commands, root, ids[1], filepath.Join(outputAbs, "candidate-source"), candidate == "")
	if err != nil {
		report["error"] = err.Error()
		return finish()
	}

	for popIdx, pop := range populations {
		row := map[string]any{
			"inputs":          pop.initial,
			"mode":            pop.mode,
			"manifest_sha256": pop.manifestHash,
		}
		reportPopulations[pop.name] = row
		report["populations"] = reportPopulations

		outputs := make(map[string][]Finding)

		lanes := []struct {
			lane   string
			binary string
		}{
			{"base", baseBinary},
			{"candidate", candidateBinary},
		}

		var fileArgList []string
		fileArgSet := make(map[string]bool)
		for _, n := range pop.fileNames {
			p := pop.files[n]
			fileArgList = append(fileArgList, p)
			fileArgSet[p] = true
		}

		for _, l := range lanes {
			checkInputs, err := populationInputs(pop.files, pop.mode)
			if err != nil || !equalInputs(checkInputs, pop.initial) {
				report["error"] = fmt.Sprintf("inputs changed before %s/%s", pop.name, l.lane)
				return finish()
			}

			cliArgs := []string{l.binary, "--format=json"}
			if pop.mode == "no-config" {
				cliArgs = append(cliArgs, "--no-config")
			}
			cliArgs = append(cliArgs, fileArgList...)

			rawOut, stderr, err := commands.run(cliArgs, root, []int{0, 1})
			if err != nil {
				report["error"] = err.Error()
				return finish()
			}
			if len(stderr) > 0 {
				report["error"] = fmt.Sprintf("unexpected CLI stderr for %s/%s", pop.name, l.lane)
				return finish()
			}

			data, err := validateOutput(rawOut, fileArgSet)
			if err != nil {
				report["error"] = err.Error()
				return finish()
			}

			expectedExit := 0
			if data.Summary.Errors+data.Summary.Warnings > 0 {
				expectedExit = 1
			}
			lastCode := commands.rows[len(commands.rows)-1].Returncode
			if lastCode != expectedExit {
				report["error"] = fmt.Sprintf("CLI exit disagrees with diagnostics for %s/%s", pop.name, l.lane)
				return finish()
			}

			namesMap := make(map[string]string)
			for n, p := range pop.files {
				namesMap[p] = n
			}
			for _, f := range data.Diagnostics {
				if filePath, ok := f["file"].(string); ok {
					if aliasName, ok := namesMap[filePath]; ok {
						f["file"] = aliasName
					}
				}
			}
			outputs[l.lane] = data.Diagnostics

			perRuleCounts := make(map[string]int)
			parseErrors := 0
			for _, f := range data.Diagnostics {
				rule, _ := f["rule"].(string)
				perRuleCounts[rule]++
				if rule == "parse/error" {
					parseErrors++
				}
			}

			row[l.lane] = map[string]any{
				"summary":      data.Summary,
				"per_rule":     perRuleCounts,
				"parse_errors": parseErrors,
			}

			laneJSONData, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				report["error"] = err.Error()
				return finish()
			}
			laneJSONData = append(laneJSONData, '\n')
			if err := os.WriteFile(filepath.Join(outputAbs, fmt.Sprintf("%d-%s.json", popIdx+1, l.lane)), laneJSONData, 0644); err != nil {
				report["error"] = err.Error()
				return finish()
			}

			checkInputs, err = populationInputs(pop.files, pop.mode)
			if err != nil || !equalInputs(checkInputs, pop.initial) {
				report["error"] = fmt.Sprintf("inputs changed during %s/%s", pop.name, l.lane)
				return finish()
			}
		}

		comp := compare(outputs["base"], outputs["candidate"], targetRule)
		row["per_rule"] = comp.PerRule
		row["non_target_identical"] = comp.NonTargetIdentical
		row["input_hashes_identical"] = true
	}

	currentFp, err := fingerprint(root)
	if err != nil || !reflect.DeepEqual(currentFp, frozen) {
		report["error"] = "source changed during verification"
		return finish()
	}
	for _, pop := range populations {
		checkInputs, err := populationInputs(pop.files, pop.mode)
		if err != nil || !equalInputs(checkInputs, pop.initial) {
			report["error"] = fmt.Sprintf("inputs changed during verification: %s", pop.name)
			return finish()
		}
		mHash, err := sha256File(pop.manifestPath)
		if err != nil || mHash != pop.manifestHash {
			report["error"] = fmt.Sprintf("inputs changed during verification: %s", pop.name)
			return finish()
		}
	}

	if targetRule != "" {
		report["target_rule"] = targetRule
	} else {
		report["target_rule"] = nil
	}

	allPass := true
	for _, pop := range populations {
		row := reportPopulations[pop.name]
		if nti, ok := row["non_target_identical"].(*bool); ok && nti != nil && !*nti {
			allPass = false
			break
		}
	}
	if allPass {
		report["status"] = "pass"
	} else {
		report["status"] = "fail"
	}

	return finish()
}

func main() {
	os.Exit(runMain(os.Args[1:]))
}
