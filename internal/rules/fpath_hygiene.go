package rules

import (
	"strings"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// FpathHygiene checks `fpath` modifications in plugin scripts for safety and best practices.
//
// ID: `plugin/fpath-hygiene`
//
// Name: fpath manipulation hygiene in plugin entrypoints
//
// Summary: Checks array `fpath` and scalar `FPATH` assignments, declarations,
// and `set -A fpath` for destructive literal replacements, non-function
// directories (`bin/`, `tests/`), and user-specific paths including `~/`.
// Configured analysis checks executed plugin and Zi annex sourced-library
// entrypoints; unconfigured analysis also visits named functions, but skips
// test directories. Function-local search paths are not caller mutations.
//
// Why: Sourced Zsh plugins must not destructively replace `fpath` (which removes
// standard system and user function directories), add hardcoded user machine paths,
// or add non-function directories (`bin/`, `tests/`) that can trigger completion
// security audit warnings (`compaudit`) or namespace collisions. Plugins should
// append or prepend relative `functions/` or `completions/` directories.
// See https://wiki.zshell.dev/community/zsh_plugin_standard#completions-and-compinit-ownership.
// The tied array/scalar pair and `set -A` semantics are documented at
// https://zsh.sourceforge.io/Doc/Release/Parameters.html#index-fpath and
// https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html#index-set.
//
// Bad:
//
//	# Destructively overwrites existing autoload paths
//	fpath=( "${0:h}/functions" )
//
//	# Adds binary directory to fpath
//	fpath+=( "${0:h}/bin" )
//
// Good:
//
//	fpath+=( "${0:h}/functions" "${0:h}/completions" )
//
//	# Prepending while preserving existing paths
//	fpath=( "${0:h}/functions" $fpath )
//
// Severity: Warning for destructive `fpath` overwrites or additions of `bin`/`tests`
// directories to `fpath`.
//
// False positives: Standalone shell bootstrap scripts can deliberately replace
// the search path; suppress with a reason. Whole parameter expansions may hold
// saved or filtered paths, including copies prepared in another file. The rule
// does not infer destructive replacement from those unknown values, but still
// checks literal directory hazards. This can miss unsafe dynamic replacements.
//
// Suppression: Use
// `# zsh-lint disable=plugin/fpath-hygiene -- <reason>` on the finding line or
// immediately before the next non-comment, non-blank source line.
//
// Corpus evidence: `z-a-meta-plugins/z-a-meta-plugins.plugin.zsh:12` uses
// compliant `fpath+=( "${0:h}/functions" )`.
type FpathHygiene struct{}

func (FpathHygiene) ID() diag.RuleID {
	return "plugin/fpath-hygiene"
}

func (FpathHygiene) Name() string {
	return "fpath manipulation hygiene in plugin entrypoints"
}

func (rule FpathHygiene) Analyze(ctx *analyzer.Context, node syntax.Node) {
	file, ok := node.(*syntax.File)
	if !ok || (ctx.Source.Configured() && !sourcedPluginRuleApplies(ctx)) {
		return
	}
	if !ctx.Source.Configured() {
		for _, segment := range strings.Split(strings.ReplaceAll(ctx.FilePath, `\`, "/"), "/") {
			if segment == "tests" || segment == "test" {
				return
			}
		}
	}
	invoked := make(map[*syntax.FuncDecl]bool)
	for _, invocation := range ctx.File.AnonymousInvocations() {
		invoked[invocation.Function] = true
	}
	rule.walkScope(ctx, file, invoked, false, false)
}

// Each function owns its local fpath/FPATH pair. Return whether that pair is
// definitely localized; branches share entry state, not sibling declarations.
func (rule FpathHygiene) walkScope(ctx *analyzer.Context, node syntax.Node, invoked map[*syntax.FuncDecl]bool, inFunction, localized bool) bool {
	walkList := func(stmts []*syntax.Stmt, local bool) bool {
		for _, stmt := range stmts {
			local = rule.walkScope(ctx, stmt, invoked, inFunction, local)
		}
		return local
	}
	syntax.Walk(node, func(current syntax.Node) bool {
		switch value := current.(type) {
		case *syntax.Stmt:
			if value.Background {
				rule.walkScope(ctx, value.Cmd, invoked, inFunction, localized)
				for _, redir := range value.Redirs {
					rule.walkScope(ctx, redir, invoked, inFunction, localized)
				}
				return false
			}
		case *syntax.FuncDecl:
			if value.Body != nil && (!ctx.Source.Configured() || invoked[value]) {
				rule.walkScope(ctx, value.Body, invoked, true, localized)
			}
			return false
		case *syntax.IfClause:
			entry := walkList(value.Cond, localized)
			thenLocal := walkList(value.Then, entry)
			elseLocal := entry
			if value.Else != nil {
				elseLocal = rule.walkScope(ctx, value.Else, invoked, inFunction, entry)
			}
			localized = thenLocal && (len(value.Cond) == 0 || elseLocal)
			return false
		case *syntax.BinaryCmd:
			left := rule.walkScope(ctx, value.X, invoked, inFunction, localized)
			if value.Op == syntax.AndStmt || value.Op == syntax.OrStmt {
				rule.walkScope(ctx, value.Y, invoked, inFunction, left)
				localized = left // The right-hand side may not execute.
			} else {
				// Zsh runs the final pipeline command in the current shell.
				localized = rule.walkScope(ctx, value.Y, invoked, inFunction, localized)
			}
			return false
		case *syntax.WhileClause:
			localized = walkList(value.Cond, localized)
			walkList(value.Do, localized)
			return false
		case *syntax.ForClause:
			if value.Loop != nil {
				rule.walkScope(ctx, value.Loop, invoked, inFunction, localized)
			}
			walkList(value.Do, localized)
			return false
		case *syntax.RepeatClause:
			if value.Count != nil {
				rule.walkScope(ctx, value.Count, invoked, inFunction, localized)
			}
			walkList(value.Do, localized)
			return false
		case *syntax.CaseClause:
			if value.Word != nil {
				rule.walkScope(ctx, value.Word, invoked, inFunction, localized)
			}
			allLocal, hasDefault := true, false
			for _, item := range value.Items {
				itemLocal := rule.walkScope(ctx, item, invoked, inFunction, localized)
				allLocal = allLocal && itemLocal
				for _, pattern := range item.Patterns {
					if len(pattern.Parts) == 1 {
						if lit, ok := pattern.Parts[0].(*syntax.Lit); ok && lit.Value == "*" {
							hasDefault = true
						}
					}
				}
			}
			localized = localized || (allLocal && hasDefault)
			return false
		case *syntax.Subshell:
			walkList(value.Stmts, localized)
			return false
		case *syntax.CmdSubst:
			walkList(value.Stmts, localized)
			return false
		case *syntax.ProcSubst:
			walkList(value.Stmts, localized)
			return false
		case *syntax.CallExpr:
			if !localized {
				rule.analyzeCall(ctx, value)
			}
		case *syntax.DeclClause:
			if value.Variant == nil {
				return true
			}
			scan := scanDeclarationOptions(value.Variant.Value, value.Args)
			if scan.mode == declarationModeNonDeclaration || (inFunction && scan.mode == declarationModeUnknown) {
				return true
			}
			for _, assign := range value.Args[scan.firstOperand:] {
				name, _, _, valid := declaredParameter(assign)
				if valid && (name == "fpath" || name == "FPATH") && inFunction && scan.mode == declarationModeLocal {
					localized = true
				}
				if !localized {
					rule.analyzeAssignment(ctx, assign, true)
				}
			}
		}
		return true
	})
	return localized
}

func (rule FpathHygiene) analyzeCall(ctx *analyzer.Context, call *syntax.CallExpr) {
	for _, assign := range call.Assigns {
		// A command prefix is temporary in native Zsh, not a persistent
		// overwrite of the caller's paths. Directory hazards still apply.
		rule.analyzeAssignment(ctx, assign, len(call.Args) == 0)
	}
	name, args := effectiveCommand(call)
	if name != "set" {
		return
	}
	values, pos, preserves, ok := setFpathArguments(args)
	if !ok {
		return
	}
	for _, word := range values {
		if wordMayContainSearchPaths(word) {
			preserves = true
		}
		checkFpathElement(ctx, word, rule.ID(), false)
	}
	if !preserves {
		rule.reportOverwrite(ctx, pos, call.End(), "fpath")
	}
}

// set consumes the argument to -o and the array name after A, including an
// attached name. Once A has consumed its name, later words are array data in
// native Zsh (KSH_ARRAYS off), even when they look like options.
func setFpathArguments(args []*syntax.Word) ([]*syntax.Word, syntax.Pos, bool, bool) {
	for len(args) > 0 {
		word := args[0]
		option, static := staticDeclarationWord(word)
		args = args[1:]
		if !static || len(option) < 2 || !strings.ContainsRune("-+", rune(option[0])) || option == "--" {
			break
		}
		for i := 1; i < len(option); i++ {
			switch option[i] {
			case 'A':
				name, pos := option[i+1:], word.Pos()
				if name == "" && len(args) > 0 {
					name, static = staticDeclarationWord(args[0])
					pos = args[0].Pos()
					args = args[1:]
				}
				return args, pos, option[0] == '+', static && name == "fpath"
			case 'o':
				if i+1 == len(option) {
					if len(args) == 0 {
						return nil, syntax.Pos{}, false, false
					}
					if _, static := staticDeclarationWord(args[0]); !static {
						return nil, syntax.Pos{}, false, false
					}
					args = args[1:]
				}
				i = len(option)
			default:
				// The released manual's native single-letter options, plus
				// set's sorting flag s. Invalid flags never reach an assignment.
				if !strings.ContainsRune("0123456789BCDEFGHIJKLMNOPQRSTUVWXYZaefghiklmnprstuvwxy", rune(option[i])) {
					return nil, syntax.Pos{}, false, false
				}
			}
		}
	}
	return nil, syntax.Pos{}, false, false
}

func (rule FpathHygiene) analyzeAssignment(ctx *analyzer.Context, assign *syntax.Assign, overwrite bool) {
	if assign == nil || assign.Name == nil {
		return
	}
	name := assign.Name.Value
	if name != "fpath" && name != "FPATH" {
		return
	}
	if assign.Array != nil {
		if overwrite && assign.Index == nil && !assign.Append && !arrayMayContainSearchPaths(assign.Array) {
			rule.reportOverwrite(ctx, assign.Pos(), assign.End(), name)
		}
		for _, elem := range assign.Array.Elems {
			if elem != nil {
				checkFpathElement(ctx, elem.Value, rule.ID(), false)
			}
		}
	} else if assign.Value != nil {
		if _, static := staticDeclarationWord(assign.Value); overwrite && static && assign.Index == nil && !assign.Append {
			rule.reportOverwrite(ctx, assign.Pos(), assign.End(), name)
		}
		checkFpathElement(ctx, assign.Value, rule.ID(), name == "FPATH")
	}
}

func (rule FpathHygiene) reportOverwrite(ctx *analyzer.Context, pos, end syntax.Pos, name string) {
	ctx.Report(pos, end, rule.ID(), diag.Warning,
		"Destructive assignment to '"+name+"' removes existing autoload paths; append paths or include '$"+name+"'")
}

func arrayMayContainSearchPaths(array *syntax.ArrayExpr) bool {
	if array == nil {
		return false
	}
	for _, elem := range array.Elems {
		if elem != nil && wordMayContainSearchPaths(elem.Value) {
			return true
		}
	}
	return false
}

// A whole expanded value can be the prior paths, a saved copy from another
// file, or an unload filter. Without value-flow proof, calling it destructive
// is speculative. Literal directory lists remain actionable. Do not descend
// into substitutions or parameter operands, whose references are not values
// included in this array.
func wordMayContainSearchPaths(word *syntax.Word) bool {
	if word == nil || len(word.Parts) != 1 {
		return false
	}
	part := word.Parts[0]
	if quoted, ok := part.(*syntax.DblQuoted); ok {
		if len(quoted.Parts) != 1 {
			return false
		}
		part = quoted.Parts[0]
	}
	pe, ok := part.(*syntax.ParamExp)
	return ok && !pe.Length && !pe.IsSet && !pe.Width && !pe.Excl
}

func checkFpathElement(ctx *analyzer.Context, word *syntax.Word, ruleID diag.RuleID, scalar bool) {
	if word == nil {
		return
	}
	if len(word.Parts) > 0 {
		if lit, ok := word.Parts[0].(*syntax.Lit); ok && strings.HasPrefix(lit.Value, "~/") {
			ctx.Report(word.Pos(), word.End(), ruleID, diag.Warning,
				"Do not add hardcoded user machine paths to 'fpath'")
			return
		}
	}
	var sb strings.Builder
	syntax.Walk(word, func(n syntax.Node) bool {
		switch part := n.(type) {
		case *syntax.Lit:
			sb.WriteString(part.Value)
		case *syntax.SglQuoted:
			sb.WriteString(part.Value)
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst:
			sb.WriteByte(0) // Unknown text must not join unrelated literal fragments.
			return false
		}
		return true
	})
	pathStr := sb.String()
	paths := []string{pathStr}
	if scalar {
		paths = strings.Split(pathStr, ":")
	}
	for _, path := range paths {
		checkFpathDirectory(ctx, word, ruleID, path)
	}
}

func checkFpathDirectory(ctx *analyzer.Context, word *syntax.Word, ruleID diag.RuleID, pathStr string) {
	if strings.HasSuffix(pathStr, "/bin") || strings.HasSuffix(pathStr, "/tests") || strings.HasSuffix(pathStr, "/test") {
		ctx.Report(
			word.Pos(),
			word.End(),
			ruleID,
			diag.Warning,
			"Do not add non-function directories ('bin', 'tests') to 'fpath'",
		)
		return
	}

	if strings.HasPrefix(pathStr, "/home/") || strings.HasPrefix(pathStr, "/Users/") {
		ctx.Report(
			word.Pos(),
			word.End(),
			ruleID,
			diag.Warning,
			"Do not add hardcoded user machine paths to 'fpath'",
		)
		return
	}
}
