// Command wikidoc is a CI/dev tool (not product code) that reads a
// gomarkdoc-generated Markdown file, sanitizes it for MDX compatibility, and
// injects the result into a marked region of a Docusaurus .mdx page in place.
//
// Usage:
//
//	wikidoc -in <generated.md> -mdx <page.mdx> [-start <marker>] [-end <marker>]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/z-shell/zsh-lint/internal/wikidoc"
)

func main() {
	inPath := flag.String("in", "", "path to gomarkdoc-generated Markdown file (required)")
	mdxPath := flag.String("mdx", "", "path to target .mdx file, edited in place (required)")
	startMarker := flag.String("start", "{/* zsh-lint:generated:start */}", "start marker in the .mdx file")
	endMarker := flag.String("end", "{/* zsh-lint:generated:end */}", "end marker in the .mdx file")
	flag.Parse()

	if *inPath == "" || *mdxPath == "" {
		fmt.Fprintln(os.Stderr, "usage: wikidoc -in <generated.md> -mdx <page.mdx> [-start <marker>] [-end <marker>]")
		os.Exit(2)
	}

	rawMD, err := os.ReadFile(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wikidoc: reading %s: %v\n", *inPath, err)
		os.Exit(1)
	}

	info, err := os.Stat(*mdxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wikidoc: stat %s: %v\n", *mdxPath, err)
		os.Exit(1)
	}

	page, err := os.ReadFile(*mdxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wikidoc: reading %s: %v\n", *mdxPath, err)
		os.Exit(1)
	}

	sanitized := wikidoc.Sanitize(string(rawMD))

	result, err := wikidoc.Inject(string(page), sanitized, *startMarker, *endMarker)
	if err != nil {
		// Inject already prefixes its errors with "wikidoc:"; print as-is to
		// avoid a doubled "wikidoc: wikidoc: ..." prefix.
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// Preserve the target file's existing permissions rather than forcing 0644.
	if err := os.WriteFile(*mdxPath, []byte(result), info.Mode().Perm()); err != nil {
		fmt.Fprintf(os.Stderr, "wikidoc: writing %s: %v\n", *mdxPath, err)
		os.Exit(1)
	}
}
