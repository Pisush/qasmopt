package opt

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pisush/qasmopt/ir"
	"github.com/Pisush/qasmopt/parser"
)

// update rewrites the golden files instead of comparing against them.
var update = flag.Bool("update", false, "rewrite golden files with current output")

// TestGolden optimizes each testdata/*.qasm file and compares the
// emitted QASM against the matching *.golden file. Regenerate goldens
// with: go test ./opt -run TestGolden -update
func TestGolden(t *testing.T) {
	inputs, err := filepath.Glob("testdata/*.qasm")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no golden inputs found in testdata/")
	}
	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".qasm")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			astProg, err := parser.Parse(string(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			prog, err := ir.Lower(astProg)
			if err != nil {
				t.Fatalf("Lower: %v", err)
			}
			prog.Ops = Optimize(prog.Ops)
			got := prog.String()

			goldenPath := strings.TrimSuffix(input, ".qasm") + ".golden"
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden file (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("optimized output mismatch for %s:\n--- got ---\n%s--- want ---\n%s", input, got, want)
			}
		})
	}
}
