package projectconfig

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Resolve classifies filename using the most-specific matching source root.
// Both the configuration root and input path use lexical absolute paths;
// symlinks are not resolved or executed.
func (c *Config) Resolve(filename string) (SourceContext, error) {
	if c == nil || c.root == "" {
		return SourceContext{}, fmt.Errorf("resolve %q: configuration has no root", filename)
	}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return SourceContext{}, fmt.Errorf("resolve %q: %w", filename, err)
	}
	abs = filepath.Clean(abs)
	if !pathContains(c.root, abs) {
		return SourceContext{}, fmt.Errorf("resolve %q: input is outside configuration root %q", filename, c.root)
	}

	best := -1
	bestDepth := -1
	for index, source := range c.Sources {
		root := filepath.Join(c.root, filepath.FromSlash(source.Root))
		if !pathContains(root, abs) {
			continue
		}
		depth := sourceDepth(source.Root)
		if depth > bestDepth {
			best = index
			bestDepth = depth
		}
	}
	if best < 0 {
		return SourceContext{}, fmt.Errorf("resolve %q: input matches no configured source root", filename)
	}

	source := c.Sources[best]
	return SourceContext{
		ConfigVersion:      c.Version,
		ProjectKind:        c.Project.Kind,
		MinimumZsh:         c.Project.MinimumZsh,
		FunctionNamespaces: append([]string(nil), c.Project.FunctionNamespaces...),
		Profile:            source.Profile,
		Role:               source.Role,
		ConfigRoot:         c.root,
		SourceRoot:         source.Root,
	}, nil
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func sourceDepth(root string) int {
	if root == "." {
		return 0
	}
	return strings.Count(root, "/") + 1
}
