package main

import (
	"strings"
	"testing"
)

const wantBell = `OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
barrier q[0], q[1];
measure q[0] -> c[0];
measure q[1] -> c[1];
`

// The Bell circuit has no cancellable/mergeable gates (barrier fences it), so
// before and after op counts are identical.
const wantBellStats = `before:
  barrier  1
  cx       1
  h        1
  measure  2
  total    5
after:
  barrier  1
  cx       1
  h        1
  measure  2
  total    5
`

func TestRunGolden(t *testing.T) {
	var out, errOut strings.Builder
	if err := run("../../examples/bell.qasm", true, true, nil, &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != wantBell {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), wantBell)
	}
	if errOut.String() != wantBellStats {
		t.Errorf("stderr:\n%s\nwant:\n%s", errOut.String(), wantBellStats)
	}
}

func TestRunStdin(t *testing.T) {
	var out, errOut strings.Builder
	in := strings.NewReader("OPENQASM 2.0;\nqreg q[1];\nx q[0];\n")
	if err := run("-", false, true, in, &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "x q[0];\n") {
		t.Errorf("stdout missing gate line:\n%s", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr without -stats: %q", errOut.String())
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
		in   string
		want string
	}{
		{"parse error position", "-", "OPENQASM 2.0;\nqreg q[1];\nfoo q[0];\n", `<stdin>:3:1: unknown gate "foo"`},
		{"lowering error position", "-", "OPENQASM 2.0;\nqreg q[1];\nx q[3];\n", "<stdin>:3:3: index 3 out of range"},
		{"missing file", "no/such/file.qasm", "", "no/such/file.qasm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut strings.Builder
			err := run(tt.path, false, true, strings.NewReader(tt.in), &out, &errOut)
			if err == nil {
				t.Fatalf("run succeeded, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}
