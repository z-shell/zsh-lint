---
description: "Guidelines for writing semantic analysis rules and AST traversals in Go using mvdan/sh"
applyTo: "internal/analyzer/**, internal/rules/**"
---

# Go AST Linting & Semantic Analysis

These instructions dictate how to build the semantic analyzer engine and lint rules for `zsh-lint` using the `mvdan/sh` parser.

## 1. The Rule Interface

All linting rules must conform to a standard, context-aware interface. The engine will drive the traversal and pass the AST nodes to the rules.

```go
// Example structural pattern (subject to ADR 0011 implementation)
type Diagnostic struct {
    Pos     syntax.Pos
    Message string
    Code    string
    Rule    string
}

type Rule interface {
    // Name returns the unique identifier for the rule (e.g., "SC2086")
    Name() string
    // Analyze evaluates a node and reports diagnostics to the Context
    Analyze(ctx *Context, node syntax.Node)
}
```

## 2. AST Traversal (The Visitor Pattern)

Do not write custom recursive descent walkers unless absolutely necessary. Rely on `syntax.Walk` from `mvdan/sh/syntax`.

```go
// Good: Using the standard walker
syntax.Walk(file, func(node syntax.Node) bool {
    switch x := node.(type) {
    case *syntax.CallExpr:
        // Handle command calls
    case *syntax.Assign:
        // Handle assignments
    }
    return true // continue traversal
})
```

## 3. Extracting Text from Nodes

Shell grammar wraps text in multiple layers (e.g., `Word` -> `Lit`). Never cast blindly. Use the parser's printer or explicit type checks to extract text cleanly.

```go
// Extracting literal text safely
if word, ok := node.(*syntax.Word); ok {
    if len(word.Parts) == 1 {
        if lit, ok := word.Parts[0].(*syntax.Lit); ok {
            return lit.Value
        }
    }
}
```

## 4. Testing Rules

Linter TDD requires table-driven tests that parse a Zsh string, run the rule, and assert against expected diagnostics.

```go
func TestMyRule(t *testing.T) {
    tests := []struct{
        name     string
        code     string
        expected int // number of diagnostics
    }{
        {"valid", "echo 'hello'", 0},
        {"invalid", "bad_command 'hello'", 1},
    }
    
    // ... setup parser, feed syntax.File to rule, assert slice length
}
```

## 5. Defensive Programming

AST nodes from `mvdan/sh` can have `nil` pointers depending on the syntax parsed (e.g., a command with no arguments, or an assignment without a value). **Always perform nil checks** before dereferencing node properties.