// Command qasmopt parses an OpenQASM 2.0 file and prints it back as
// normalized QASM (registers flattened to indexed form) on stdout.
//
// Usage:
//
//	qasmopt [-stats] file.qasm
//	qasmopt [-stats] -        # read from stdin
//
// With -stats, per-gate op counts are printed to stderr so stdout stays
// valid QASM.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/Pisush/qasmopt/ir"
	"github.com/Pisush/qasmopt/opt"
	"github.com/Pisush/qasmopt/parser"
)

func main() {
	stats := flag.Bool("stats", false, "print per-gate op counts (before/after optimization) to stderr")
	noOpt := flag.Bool("no-opt", false, "disable optimization; just parse and re-emit normalized QASM")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: qasmopt [-stats] [-no-opt] <file.qasm | ->\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *stats, !*noOpt, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "qasmopt: %v\n", err)
		os.Exit(1)
	}
}

// run does the actual work so tests can exercise it without os.Exit.
// stdin is read only when path is "-". When optimize is true the peephole
// passes run to fixpoint before emitting; with stats, before/after op counts
// are printed to stderr.
func run(path string, stats, optimize bool, stdin io.Reader, out, errOut io.Writer) error {
	var src []byte
	var err error
	if path == "-" {
		src, err = io.ReadAll(stdin)
	} else {
		src, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}

	astProg, err := parser.Parse(string(src))
	if err != nil {
		return fmt.Errorf("%s:%w", displayName(path), err)
	}
	prog, err := ir.Lower(astProg)
	if err != nil {
		return fmt.Errorf("%s:%w", displayName(path), err)
	}

	if stats {
		printStats(errOut, "before", ir.Stats(prog.Ops))
	}
	if optimize {
		prog.Ops = opt.Optimize(prog.Ops)
	}
	if stats {
		printStats(errOut, "after", ir.Stats(prog.Ops))
	}
	return prog.WriteQASM(out)
}

// displayName is the file name used in error prefixes.
func displayName(path string) string {
	if path == "-" {
		return "<stdin>"
	}
	return path
}

// printStats writes op counts sorted by name, followed by a total.
func printStats(w io.Writer, label string, counts map[string]int) {
	names := make([]string, 0, len(counts))
	total := 0
	for name, n := range counts {
		names = append(names, name)
		total += n
	}
	sort.Strings(names)
	fmt.Fprintf(w, "%s:\n", label)
	for _, name := range names {
		fmt.Fprintf(w, "  %-8s %d\n", name, counts[name])
	}
	fmt.Fprintf(w, "  %-8s %d\n", "total", total)
}
