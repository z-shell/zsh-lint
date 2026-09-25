// Command zsh-lint-probe writes a probe grid for a parser change (#428): every
// body from the body file placed in every scanner context, one file each.
// Judge the grid with `zsh-lint-survey -compare <base-binary> -native`.
//
//	zsh-lint-probe -bodies bodies.txt -out grid/
//	zsh-lint-probe -contexts   # list the contexts
//
// The body file holds the construct's variants, valid and invalid, separated
// by lines holding only "---".
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
		fmt.Fprintln(os.Stderr, "       zsh-lint-probe -contexts")
		flags.PrintDefaults()
	}
	bodiesPath := flags.String("bodies", "", "file of bodies separated by lines holding only ---")
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
	if *bodiesPath == "" || *out == "" || flags.NArg() != 0 {
		flags.Usage()
		os.Exit(2)
	}

	source, err := os.Open(*bodiesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
		os.Exit(1)
	}
	bodies, err := probe.ReadBodies(source)
	_ = source.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
		os.Exit(1)
	}
	if len(bodies) == 0 {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe: no bodies in", *bodiesPath)
		os.Exit(1)
	}
	files, err := probe.Generate(bodies, probe.Contexts)
	if err == nil {
		err = probe.Write(*out, files)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "zsh-lint-probe:", err)
		os.Exit(1)
	}
	fmt.Printf("%d bodies x %d contexts = %d files in %s\n", len(bodies), len(probe.Contexts), len(files), *out)
}
