// The contributor mutation command's standard-library line runner.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type mutant struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Old            string `json:"old"`
	New            string `json:"new"`
	offset, column int
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	status, err := run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutation:", err)
		status = 2
	}
	os.Exit(status)
}

func git(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.Output()
}

var hunk = regexp.MustCompile(`(?m)^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

func run(ctx context.Context) (int, error) {
	flags := flag.NewFlagSet("mutation", flag.ContinueOnError)
	candidate := flags.String("candidate", "", "replay a committed candidate")
	spec := flags.String("spec", "", "extend mutants with JSON file, line, old, new entries")
	limit := flags.Int("max-mutants", 0, "cap a diagnostic run, reporting incomplete evidence")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return 2, err
	}
	if flags.NArg() > 1 || *limit < 0 {
		return 2, errors.New("options must precede the optional base revision")
	}
	base := "origin/main"
	if flags.NArg() == 1 {
		base = flags.Arg(0)
	}
	rootBytes, err := git("", "rev-parse", "--show-toplevel")
	if err != nil {
		return 2, errors.New("not inside a Git checkout")
	}
	root := strings.TrimSpace(string(rootBytes))
	baseBytes, err := git(root, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return 2, fmt.Errorf("unknown base revision: %s", base)
	}
	baseID := strings.TrimSpace(string(baseBytes))
	if *candidate != "" {
		value, e := git(root, "rev-parse", "--verify", "--end-of-options", *candidate+"^{commit}")
		if e != nil {
			return 2, fmt.Errorf("unknown candidate revision: %s", *candidate)
		}
		*candidate = strings.TrimSpace(string(value))
	}
	work, err := os.MkdirTemp("", "zsh-lint-mutation-")
	if err != nil {
		return 2, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := snapshot(root, work, *candidate); err != nil {
		return 2, err
	}
	diffArgs := []string{"diff", "--no-ext-diff", "--no-textconv", "--name-only", "-z", "--diff-filter=ACM", baseID}
	if *candidate != "" {
		diffArgs = append(diffArgs, *candidate)
	}
	names, err := git(root, append(diffArgs, "--", "*.go")...)
	if err != nil {
		return 2, err
	}
	untracked := map[string]bool{}
	if *candidate == "" {
		newNames, e := git(root, "ls-files", "--others", "--exclude-standard", "-z", "--", "*.go")
		if e != nil {
			return 2, e
		}
		names = append(names, newNames...)
		for _, name := range strings.Split(string(newNames), "\x00") {
			untracked[name] = true
		}
	}
	var mutants []mutant
	changed := map[string]map[int]bool{}
	unsupported := map[string]bool{}
	for _, name := range strings.Split(string(names), "\x00") {
		if name == "" || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, ".github/") {
			continue
		}
		args := []string{"diff", "--no-ext-diff", "--no-textconv", "--unified=0", "--no-color", baseID}
		if *candidate != "" {
			args = append(args, *candidate)
		}
		diff, e := git(root, append(args, "--", name)...)
		if e != nil {
			return 2, e
		}
		lines := map[int]bool{}
		for _, match := range hunk.FindAllSubmatch(diff, -1) {
			start, _ := strconv.Atoi(string(match[1]))
			count := 1
			if len(match[2]) > 0 {
				count, _ = strconv.Atoi(string(match[2]))
			}
			for n := start; n < start+count; n++ {
				lines[n] = true
			}
		}
		source, e := os.ReadFile(filepath.Join(work, name))
		if e != nil {
			return 2, e
		}
		if untracked[name] {
			for line := 1; line <= bytes.Count(source, []byte("\n"))+1; line++ {
				lines[line] = true
			}
		}
		changed[name] = lines
		generated, e := generate(name, source, lines)
		if e != nil {
			return 2, e
		}
		if len(generated) == 0 {
			unsupported[name] = true
		}
		mutants = append(mutants, generated...)
	}
	if *spec != "" {
		body, e := os.ReadFile(*spec)
		if e != nil {
			return 2, e
		}
		var entries []mutant
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(&entries); e != nil {
			return 2, e
		}
		if e := decoder.Decode(new(any)); e != io.EOF {
			return 2, errors.New("spec must contain one JSON array")
		}
		for _, entry := range entries {
			if !changed[entry.File][entry.Line] || entry.Old == "" || entry.New == entry.Old {
				return 2, fmt.Errorf("spec must replace text on a changed source line: %s:%d", entry.File, entry.Line)
			}
			source, e := os.ReadFile(filepath.Join(work, entry.File))
			if e != nil {
				return 2, e
			}
			lines := bytes.Split(source, []byte("\n"))
			if entry.Line > len(lines) {
				return 2, errors.New("spec line outside source")
			}
			line := lines[entry.Line-1]
			if bytes.Count(line, []byte(entry.Old)) != 1 {
				return 2, fmt.Errorf("spec old text must match once: %s:%d", entry.File, entry.Line)
			}
			for _, previous := range lines[:entry.Line-1] {
				entry.offset += len(previous) + 1
			}
			entry.column = bytes.Index(line, []byte(entry.Old)) + 1
			entry.offset += entry.column - 1
			mutants = append(mutants, entry)
			delete(unsupported, entry.File)
		}
	}
	for name := range unsupported {
		fmt.Printf("UNSUPPORTED %s: no supported automatic or explicit mutations on changed lines\n", name)
	}
	slices.SortFunc(mutants, func(a, b mutant) int {
		if order := strings.Compare(a.File, b.File); order != 0 {
			return order
		}
		if a.offset != b.offset {
			return a.offset - b.offset
		}
		return strings.Compare(a.Old+"\x00"+a.New, b.Old+"\x00"+b.New)
	})
	mutants = slices.CompactFunc(mutants, func(a, b mutant) bool {
		return a.File == b.File && a.offset == b.offset && a.Old == b.Old && a.New == b.New
	})
	if len(mutants) == 0 {
		fmt.Println("mutation: no supported mutations on changed Go source lines")
		if len(unsupported) > 0 {
			return 3, nil
		}
		return 0, nil
	}
	coefficient := 30.0
	if value := os.Getenv("MUTATION_TIMEOUT_COEFFICIENT"); value != "" {
		coefficient, err = strconv.ParseFloat(value, 64)
		if err != nil || coefficient <= 0 || math.IsNaN(coefficient) || math.IsInf(coefficient, 0) || coefficient > 1e6 {
			return 2, errors.New("MUTATION_TIMEOUT_COEFFICIENT must be positive")
		}
	}
	var explicit time.Duration
	if value := os.Getenv("MUTATION_TIMEOUT"); value != "" {
		seconds, e := strconv.ParseFloat(value, 64)
		if e != nil || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds > 86400 {
			return 2, errors.New("MUTATION_TIMEOUT must be positive seconds")
		}
		explicit = time.Duration(seconds * float64(time.Second))
	}
	killed, lived, timedOut, invalid := 0, 0, 0, 0
	baselines := map[string]time.Duration{}
	truncated := *limit > 0 && len(mutants) > *limit
	if truncated {
		fmt.Printf("mutation: truncated: testing %d of %d mutants\n", *limit, len(mutants))
		mutants = mutants[:*limit]
	}
	for _, m := range mutants {
		commands := testCommands(m.File)
		key := fmt.Sprint(commands)
		if baselines[key] == 0 {
			fmt.Printf("mutation: baseline %s\n", key)
			start := time.Now()
			status, output := test(ctx, work, commands, 120*time.Second)
			if status != "LIVED" {
				return 2, fmt.Errorf("baseline did not pass (%s):\n%s", status, output)
			}
			baselines[key] = time.Since(start)
		}
		timeout := max(30*time.Second, time.Duration(float64(baselines[key])*coefficient))
		if explicit > 0 {
			timeout = explicit
		}
		path := filepath.Join(work, m.File)
		original, e := os.ReadFile(path)
		if e != nil {
			return 2, e
		}
		value := append([]byte{}, original[:m.offset]...)
		value = append(value, m.New...)
		value = append(value, original[m.offset+len(m.Old):]...)
		if e := os.WriteFile(path, value, 0o600); e != nil {
			return 2, e
		}
		status, _ := test(ctx, work, commands, timeout)
		if e := os.WriteFile(path, original, 0o600); e != nil {
			return 2, e
		}
		if ctx.Err() != nil {
			return 2, ctx.Err()
		}
		switch status {
		case "KILLED":
			killed++
		case "LIVED":
			lived++
		case "TIMED OUT":
			timedOut++
		default:
			invalid++
		}
		fmt.Printf("%s %s:%d:%d %q -> %q\n", status, m.File, m.Line, m.column, m.Old, m.New)
	}
	fmt.Printf("mutation: against %s, %d killed, %d lived, %d timed out, %d invalid\n", base, killed, lived, timedOut, invalid)
	if lived > 0 {
		return 1, nil
	}
	if truncated || invalid > 0 || len(unsupported) > 0 || timedOut > killed+lived {
		fmt.Println("mutation: inconclusive: incomplete, invalid, unsupported or timeout-dominated run")
		return 3, nil
	}
	return 0, nil
}

func snapshot(root, target, candidate string) error {
	var names []byte
	var err error
	if candidate != "" {
		names, err = git(root, "ls-tree", "-r", "--name-only", "-z", candidate)
	} else {
		names, err = git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	}
	if err != nil {
		return err
	}
	for _, name := range strings.Split(string(names), "\x00") {
		if name == "" {
			continue
		}
		var body []byte
		mode := os.FileMode(0o600)
		if candidate != "" {
			body, err = git(root, "show", candidate+":"+name)
		} else {
			from := filepath.Join(root, name)
			info, e := os.Lstat(from)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("snapshot requires regular files: %s", name)
			}
			mode = info.Mode().Perm()
			body, err = os.ReadFile(from)
		}
		if err != nil {
			return err
		}
		to := filepath.Join(target, name)
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(to, body, mode); err != nil {
			return err
		}
	}
	return nil
}

func generate(name string, source []byte, lines map[int]bool) ([]mutant, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, 0)
	if err != nil {
		return nil, err
	}
	var result []mutant
	add := func(pos, end token.Pos, replacement string) {
		start, finish := fset.PositionFor(pos, false), fset.PositionFor(end, false)
		if lines[start.Line] {
			result = append(result, mutant{File: name, Line: start.Line, Old: string(source[start.Offset:finish.Offset]), New: replacement, offset: start.Offset, column: start.Column})
		}
	}
	opposites := map[token.Token]string{token.EQL: "!=", token.NEQ: "==", token.LSS: ">=", token.LEQ: ">", token.GTR: "<=", token.GEQ: "<", token.LAND: "||", token.LOR: "&&"}
	boundaries := map[token.Token]string{token.LSS: "<=", token.LEQ: "<", token.GTR: ">=", token.GEQ: ">"}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if value, ok := opposites[n.Op]; ok {
				add(n.OpPos, n.OpPos+token.Pos(len(n.Op.String())), value)
			}
			if value, ok := boundaries[n.Op]; ok {
				add(n.OpPos, n.OpPos+token.Pos(len(n.Op.String())), value)
			}
		case *ast.IncDecStmt:
			value := "++"
			if n.Tok == token.INC {
				value = "--"
			}
			add(n.TokPos, n.TokPos+2, value)
		case *ast.BasicLit:
			if n.Kind == token.INT {
				add(n.Pos(), n.End(), "("+n.Value+" + 1)")
				add(n.Pos(), n.End(), "("+n.Value+" - 1)")
			}
		case *ast.Ident:
			if n.Name == "true" {
				add(n.Pos(), n.End(), "false")
			}
			if n.Name == "false" {
				add(n.Pos(), n.End(), "true")
			}
		case *ast.IfStmt:
			start, end := fset.PositionFor(n.Cond.Pos(), false), fset.PositionFor(n.Cond.End(), false)
			add(n.Cond.Pos(), n.Cond.End(), "!("+string(source[start.Offset:end.Offset])+")")
		case *ast.SwitchStmt:
			if n.Tag == nil {
				for _, statement := range n.Body.List {
					clause := statement.(*ast.CaseClause)
					for _, guard := range clause.List {
						start, end := fset.PositionFor(guard.Pos(), false), fset.PositionFor(guard.End(), false)
						add(guard.Pos(), guard.End(), "!("+string(source[start.Offset:end.Offset])+")")
					}
				}
			}
		}
		return true
	})
	return result, nil
}

type testCommand struct {
	dir  string
	args []string
}

func testCommands(name string) []testCommand {
	if strings.HasPrefix(name, "third_party/mvdan-sh/") {
		return []testCommand{{"third_party/mvdan-sh", []string{"./syntax/"}}, {"", []string{"./internal/parse/", "./internal/survey/"}}}
	}
	return []testCommand{{"", []string{"./" + filepath.ToSlash(filepath.Dir(name)) + "/"}}}
}

func test(parent context.Context, work string, commands []testCommand, timeout time.Duration) (string, string) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var output strings.Builder
	for _, command := range commands {
		log, err := os.CreateTemp(work, "test-output-")
		if err != nil {
			return "INVALID", err.Error()
		}
		args := append([]string{"test", "-json", "-count=1", "-timeout=0"}, command.args...)
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = filepath.Join(work, command.dir)
		cmd.Env = append(os.Environ(), "GOWORK=off")
		cmd.Stdout, cmd.Stderr = log, log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		err = cmd.Run()
		// Kill descendants even after a wrapper exits normally. File output
		// avoids an inherited pipe making Wait hang after its parent exits.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		_, _ = log.Seek(0, io.SeekStart)
		body, _ := io.ReadAll(log)
		_ = log.Close()
		_ = os.Remove(log.Name())
		output.Write(body)
		if ctx.Err() != nil {
			return "TIMED OUT", output.String()
		}
		if err != nil {
			if bytes.Contains(body, []byte("[build failed]")) || bytes.Contains(body, []byte(`"Action":"build-fail"`)) {
				return "INVALID", output.String()
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				return "INVALID", output.String()
			}
			return "KILLED", output.String()
		}
	}
	return "LIVED", output.String()
}
