package ir

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Pisush/qasmopt/parser"
	"github.com/Pisush/qasmopt/token"
)

const header = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\n"

// lower parses and lowers src, failing the test on any error.
func lower(t *testing.T, src string) *Program {
	t.Helper()
	astProg, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	prog, err := Lower(astProg)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return prog
}

func TestFlattening(t *testing.T) {
	src := header + `qreg q[2];
qreg r[3];
creg c[2];
creg d[1];
h q[1];
x r[0];
cx q[0], r[2];
measure r[1] -> d[0];
`
	prog := lower(t, src)
	if got := prog.NumQubits(); got != 5 {
		t.Errorf("NumQubits = %d, want 5", got)
	}
	if got := prog.NumCbits(); got != 3 {
		t.Errorf("NumCbits = %d, want 3", got)
	}
	wantRegs := []Reg{{Name: "q", Size: 2, Offset: 0}, {Name: "r", Size: 3, Offset: 2}}
	if !reflect.DeepEqual(prog.QRegs, wantRegs) {
		t.Errorf("QRegs = %+v, want %+v", prog.QRegs, wantRegs)
	}
	wantOps := []Op{
		{Name: "h", Qubits: []int{1}},
		{Name: "x", Qubits: []int{2}},
		{Name: "cx", Qubits: []int{0, 4}},
		{Name: OpMeasure, Qubits: []int{3}, Cbits: []int{2}},
	}
	if !opsEqual(prog.Ops, wantOps) {
		t.Errorf("Ops = %+v, want %+v", prog.Ops, wantOps)
	}
}

// opsEqual compares ops treating nil and empty slices as equal.
func opsEqual(a, b []Op) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			!intsEqual(a[i].Qubits, b[i].Qubits) ||
			!intsEqual(a[i].Cbits, b[i].Cbits) ||
			!floatsEqual(a[i].Params, b[i].Params) {
			return false
		}
	}
	return true
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] { // exact: lowering must not perturb params
			return false
		}
	}
	return true
}

func TestBroadcast(t *testing.T) {
	src := header + `qreg q[3];
creg c[3];
h q;
barrier q;
measure q -> c;
`
	prog := lower(t, src)
	wantOps := []Op{
		{Name: "h", Qubits: []int{0}},
		{Name: "h", Qubits: []int{1}},
		{Name: "h", Qubits: []int{2}},
		{Name: OpBarrier, Qubits: []int{0, 1, 2}},
		{Name: OpMeasure, Qubits: []int{0}, Cbits: []int{0}},
		{Name: OpMeasure, Qubits: []int{1}, Cbits: []int{1}},
		{Name: OpMeasure, Qubits: []int{2}, Cbits: []int{2}},
	}
	if !opsEqual(prog.Ops, wantOps) {
		t.Errorf("Ops = %+v, want %+v", prog.Ops, wantOps)
	}
}

func TestBarrierMixedArgs(t *testing.T) {
	src := header + "qreg q[2];\nqreg r[2];\nbarrier q[1], r;\n"
	prog := lower(t, src)
	want := []Op{{Name: OpBarrier, Qubits: []int{1, 2, 3}}}
	if !opsEqual(prog.Ops, want) {
		t.Errorf("Ops = %+v, want %+v", prog.Ops, want)
	}
}

func TestLowerErrors(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		line, col int
		want      string
	}{
		{"undeclared register", header + "h q[0];", 3, 3, `undeclared register "q"`},
		{"redeclared register", header + "qreg q[1];\ncreg q[1];", 4, 1, `already declared as qreg`},
		{"index out of range", header + "qreg q[2];\nx q[2];", 4, 3, "out of range"},
		{"creg as qubit", header + "qreg q[1]; creg c[1];\nh c[0];", 4, 3, `"c" is a creg`},
		{"qreg as measure target", header + "qreg q[2];\nmeasure q[0] -> q[1];", 4, 17, `"q" is a qreg`},
		{"cx broadcast", header + "qreg q[2]; qreg r[2];\ncx q, r[0];", 4, 4, "not supported in v1"},
		{"measure size mismatch", header + "qreg q[2]; creg c[3];\nmeasure q -> c;", 4, 1, "register sizes differ"},
		{"measure mixed forms", header + "qreg q[2]; creg c[2];\nmeasure q -> c[0];", 4, 1, "both sides indexed or both whole registers"},
		{"barrier duplicate qubit", header + "qreg q[2];\nbarrier q, q[0];", 4, 12, "duplicate qubit"},
		{"measure out of range", header + "qreg q[2]; creg c[2];\nmeasure q[5] -> c[0];", 4, 9, "out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			astProg, err := parser.Parse(tt.src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			_, err = Lower(astProg)
			if err == nil {
				t.Fatalf("Lower succeeded, want error containing %q", tt.want)
			}
			var lerr *Error
			if !errors.As(err, &lerr) {
				t.Fatalf("error is %T, want *ir.Error (%v)", err, err)
			}
			if lerr.Pos != (token.Pos{Line: tt.line, Col: tt.col}) {
				t.Errorf("error at %v, want %d:%d (%v)", lerr.Pos, tt.line, tt.col, err)
			}
			if !strings.Contains(lerr.Msg, tt.want) {
				t.Errorf("error %q does not contain %q", lerr.Msg, tt.want)
			}
		})
	}
}

func TestEmitRoundTrip(t *testing.T) {
	src := header + `qreg q[2];
qreg r[1];
creg c[2];
h q;
rz(pi/2) q[0];
u3(pi/2, -pi/4, 0.125) q[1];
cx q[0], r[0];
barrier q, r[0];
measure q -> c;
`
	prog := lower(t, src)
	emitted := prog.String()

	// The emitted source must parse and lower to the identical program.
	reProg := lower(t, emitted)
	if !reflect.DeepEqual(prog.QRegs, reProg.QRegs) || !reflect.DeepEqual(prog.CRegs, reProg.CRegs) {
		t.Errorf("registers changed after round trip:\n%+v\nvs\n%+v", prog, reProg)
	}
	if !opsEqual(prog.Ops, reProg.Ops) {
		t.Errorf("ops changed after round trip:\nfirst:  %+v\nsecond: %+v", prog.Ops, reProg.Ops)
	}
	// And emitting again must be a fixpoint (normalized form).
	if again := reProg.String(); again != emitted {
		t.Errorf("emission is not stable:\n%s\nvs\n%s", emitted, again)
	}
}

func TestEmitFormat(t *testing.T) {
	src := header + "qreg q[2];\ncreg c[1];\nrz(pi) q[0];\ncx q[0], q[1];\nmeasure q[1] -> c[0];\n"
	got := lower(t, src).String()
	want := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[1];
rz(3.141592653589793) q[0];
cx q[0], q[1];
measure q[1] -> c[0];
`
	if got != want {
		t.Errorf("emitted:\n%s\nwant:\n%s", got, want)
	}
}

func TestStats(t *testing.T) {
	src := header + "qreg q[2];\ncreg c[2];\nh q;\ncx q[0], q[1];\nbarrier q;\nmeasure q -> c;\n"
	got := Stats(lower(t, src).Ops)
	want := map[string]int{"h": 2, "cx": 1, OpBarrier: 1, OpMeasure: 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Stats = %v, want %v", got, want)
	}
}
