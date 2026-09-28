// Command zsh-lint-probe writes a probe grid for a parser change (#428): every
// body from the body file placed in every scanner context, one file each.
// Judge the grid with `zsh-lint-survey -compare <base-binary> -native`.
//
//	zsh-lint-probe -bodies bodies.txt -out grid/
//	zsh-lint-probe -contexts   # list the contexts
//
// The body file holds the construct's variants, valid and invalid, separated
// by lines holding only "---".
//
//	zsh-lint-probe -rows rows.txt -out grid/
//
// A row file holds one complete source per line and writes one file per
// row, for grids generated outside the context list, such as words in a
// subscript (#545). Blank lines and lines starting with "#" are skipped.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/z-shell/zsh-lint/internal/probe"
)

func main() {
	flags := flag.NewFlagSet("zsh-lint-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zsh-lint-probe -bodies <file> -out <new-directory>")
		fmt.Fprintln(os.Stderr, "       zsh-lint-probe -rows <file> -out <new-directory>")
		fmt.Fprintln(os.Stderr, "       zsh-lint-probe -contexts")
		flags.PrintDefaults()
	}
	bodiesPath := flags.String("bodies", "", "file of bodies separated by lines holding only ---")
	rowsPath := flags.String("rows", "", "file of complete sources, one per line")
	out := flags.String("out", "", "directory to create for the grid")
	list := flags.Bool("contexts", false, "list the contexts and exit")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	if *list {
		for _, context := range probe.Contexts {
			fmt.Printf("%s\t%s\n", context.Name, strings.ReplaceAll(context.Template, "\n", `\n`))
		}
		return
	}
	if (*bodiesPath == "") == (*rowsPath == "") || *out == "" || flags.NArg() != 0 {
		flags.Usage()
		os.Exit(2)
	}

	path := *bodiesPath
	if *rowsPath != "" {
		path = *rowsPath
	}
	source, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
		os.Exit(1)
	}
	var entries []string
	if *rowsPath != "" {
		entries, err = probe.ReadRows(source)
	} else {
		entries, err = probe.ReadBodies(source)
	}
	_ = source.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe: nothing to generate in", path)
		os.Exit(1)
	}
	if *rowsPath != "" {
		if err := probe.Write(*out, probe.RowFiles(entries)); err != nil {
			fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
			os.Exit(1)
		}
		fmt.Printf("%d rows = %d files in %s\n", len(entries), len(entries), *out)
		return
	}
	files, err := probe.Generate(entries, probe.Contexts)
	if err == nil {
		err = probe.Write(*out, files)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
		os.Exit(1)
	}
	fmt.Printf("%d bodies x %d contexts = %d files in %s\n", len(entries), len(probe.Contexts), len(files), *out)
}
