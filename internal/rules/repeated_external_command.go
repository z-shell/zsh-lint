package rules

import (
	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"mvdan.cc/sh/v3/syntax"
)

// RepeatedExternalCommand reports known external commands in completion loops.
//
// ID: `performance/repeated-external-command`
//
// Name: Repeated external command in completion loop
//
// Summary: Reports a conservative allowlist of commands that necessarily
// create a process when they occur in a loop in a configured completion source.
//
// Why: A loop with N iterations creates N external processes for each reported
// call. Process creation is the dominant fixed cost even when the command does
// little work. Hoist invariant work, use Zsh-native parameter operations, or
// measure and retain the call when its result genuinely varies per iteration.
//
// Severity: Info. The cost model is certain, but whether it matters depends on
// iteration count, command work, and the interactive completion workload.
//
// False positives: A command whose result must be recomputed for each item may
// be intentional. Measure the completion path before changing behavior.
type RepeatedExternalCommand struct{}

func (RepeatedExternalCommand) ID() diag.RuleID {
	return "performance/repeated-external-command"
}

func (RepeatedExternalCommand) Name() string {
	return "Repeated external command in completion loop"
}

func (rule RepeatedExternalCommand) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || !ctx.Source.Configured() || ctx.Source.Profile != projectconfig.ProfileAutoloadFunction ||
		ctx.Source.Role != projectconfig.RoleCompletion {
		return
	}
	seen := make(map[uint]bool)
	syntax.Walk(file, func(current syntax.Node) bool {
		switch loop := current.(type) {
		case *syntax.ForClause:
			rule.reportLoopCalls(ctx, loop.Do, seen)
		case *syntax.WhileClause:
			rule.reportLoopCalls(ctx, loop.Do, seen)
		}
		return true
	})
}

func (rule RepeatedExternalCommand) reportLoopCalls(ctx *analyzer.Context, statements []*syntax.Stmt, seen map[uint]bool) {
	for _, statement := range statements {
		syntax.Walk(statement, func(current syntax.Node) bool {
			call, ok := current.(*syntax.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			name := getWordLiteral(call.Args[0])
			if !knownExternalCommand(name) || seen[call.Pos().Offset()] {
				return true
			}
			seen[call.Pos().Offset()] = true
			ctx.Report(call.Pos(), call.End(), rule.ID(), diag.Info,
				"External command '"+name+"' runs once per completion-loop iteration; hoist invariant work or measure the interactive cost")
			return true
		})
	}
}

func knownExternalCommand(name string) bool {
	switch name {
	case "awk", "cut", "find", "git", "grep", "sed", "sort", "tr":
		return true
	default:
		return false
	}
}
