package parser

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Pisush/qasmopt/ast"
)

const eps = 1e-9

const header = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\n"

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	return prog
}

func TestParseBellCircuit(t *testing.T) {
	src := header + `qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
barrier q;
measure q[0] -> c[0];
measure q[1] -> c[1];
`
	prog := mustParse(t, src)
	if prog.Version != "2.0" {
		t.Errorf("Version = %q, want 2.0", prog.Version)
	}
	if len(prog.Stmts) != 8 {
		t.Fatalf("got %d statements, want 8", len(prog.Stmts))
	}
	if inc, ok := prog.Stmts[0].(*ast.Include); !ok || inc.Path != "qelib1.inc" {
		t.Errorf("stmt 0: got %#v, want include qelib1.inc", prog.Stmts[0])
	}
	qreg, ok := prog.Stmts[1].(*ast.RegDecl)
	if !ok || qreg.Kind != ast.QReg || qreg.Name != "q" || qreg.Size != 2 {
		t.Errorf("stmt 1: got %#v, want qreg q[2]", prog.Stmts[1])
	}
	cx, ok := prog.Stmts[4].(*ast.GateStmt)
	if !ok || cx.Name != "cx" || len(cx.Args) != 2 {
		t.Fatalf("stmt 4: got %#v, want cx with 2 args", prog.Stmts[4])
	}
	if cx.Args[0] != (ast.Arg{ArgPos: cx.Args[0].ArgPos, Reg: "q", Index: 0, Indexed: true}) ||
		cx.Args[1] != (ast.Arg{ArgPos: cx.Args[1].ArgPos, Reg: "q", Index: 1, Indexed: true}) {
		t.Errorf("cx args = %v, %v; want q[0], q[1]", cx.Args[0], cx.Args[1])
	}
	bar, ok := prog.Stmts[5].(*ast.BarrierStmt)
	if !ok || len(bar.Args) != 1 || bar.Args[0].Indexed {
		t.Errorf("stmt 5: got %#v, want barrier on whole register q", prog.Stmts[5])
	}
	meas, ok := prog.Stmts[6].(*ast.MeasureStmt)
	if !ok || meas.Src.Reg != "q" || meas.Dst.Reg != "c" {
		t.Errorf("stmt 6: got %#v, want measure q[0] -> c[0]", prog.Stmts[6])
	}
}

func TestParamExpressions(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"0", 0},
		{"pi", math.Pi},
		{"-pi", -math.Pi},
		{"pi/2", math.Pi / 2},
		{"-pi/4", -math.Pi / 4},
		{"2*pi", 2 * math.Pi},
		{"pi/2 + pi/4", 3 * math.Pi / 4},
		{"1 - 2 - 3", -4},         // left associativity
		{"12/3/2", 2},             // left associativity
		{"1 + 2*3", 7},            // precedence
		{"(1 + 2)*3", 9},          // parentheses
		{"-(pi/2)", -math.Pi / 2}, // unary on parens
		{"--1", 1},                // double negation
		{"1.5e2", 150},
		{".5", 0.5},
		{"3.", 3},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			src := header + "qreg q[1];\nrz(" + tt.expr + ") q[0];\n"
			prog := mustParse(t, src)
			gate := prog.Stmts[len(prog.Stmts)-1].(*ast.GateStmt)
			if len(gate.Params) != 1 {
				t.Fatalf("got %d params, want 1", len(gate.Params))
			}
			if math.Abs(gate.Params[0]-tt.want) > eps {
				t.Errorf("rz(%s) = %v, want %v", tt.expr, gate.Params[0], tt.want)
			}
		})
	}
}

func TestMultiParamGates(t *testing.T) {
	src := header + "qreg q[1];\nu2(0, pi) q[0];\nu3(pi/2, 0, pi) q[0];\n"
	prog := mustParse(t, src)
	u2 := prog.Stmts[2].(*ast.GateStmt)
	if len(u2.Params) != 2 || math.Abs(u2.Params[1]-math.Pi) > eps {
		t.Errorf("u2 params = %v, want [0 pi]", u2.Params)
	}
	u3 := prog.Stmts[3].(*ast.GateStmt)
	if len(u3.Params) != 3 || math.Abs(u3.Params[0]-math.Pi/2) > eps {
		t.Errorf("u3 params = %v, want [pi/2 0 pi]", u3.Params)
	}
}

func TestAllSupportedGates(t *testing.T) {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("qreg q[2];\n")
	for name, spec := range gateSpecs {
		b.WriteString(name)
		if spec.params > 0 {
			b.WriteString("(")
			for i := 0; i < spec.params; i++ {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("pi/2")
			}
			b.WriteString(")")
		}
		b.WriteString(" q[0]")
		if spec.qubits == 2 {
			b.WriteString(", q[1]")
		}
		b.WriteString(";\n")
	}
	mustParse(t, b.String())
}

// errorAt asserts that parsing src fails with a *parser.Error at the
// given position whose message contains want.
func errorAt(t *testing.T, src string, line, col int, want string) {
	t.Helper()
	_, err := Parse(src)
	if err == nil {
		t.Fatalf("Parse succeeded, want error containing %q", want)
	}
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("error is %T, want *parser.Error (%v)", err, err)
	}
	if perr.Pos.Line != line || perr.Pos.Col != col {
		t.Errorf("error at %v, want %d:%d (%v)", perr.Pos, line, col, err)
	}
	if !strings.Contains(perr.Msg, want) {
		t.Errorf("error %q does not contain %q", perr.Msg, want)
	}
}

func TestSyntaxErrors(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		line, col int
		want      string
	}{
		{"missing header", "qreg q[1];", 1, 1, "expected OPENQASM"},
		{"wrong version", "OPENQASM 3.0;", 1, 10, "unsupported OpenQASM version"},
		{"missing semicolon after header", "OPENQASM 2.0\nqreg q[1];", 2, 1, "expected ;"},
		{"missing semicolon", header + "qreg q[2]\nh q[0];", 4, 1, "expected ;"},
		{"missing bracket", header + "qreg q 2];", 3, 8, "expected ["},
		{"missing register size", header + "qreg q[];", 3, 8, "expected integer"},
		{"empty register", header + "qreg q[0];", 3, 1, "at least one element"},
		{"missing arrow", header + "qreg q[1]; creg c[1];\nmeasure q[0] c[0];", 4, 14, "expected ->"},
		{"unknown gate", header + "qreg q[1];\nfoo q[0];", 4, 1, "unknown gate \"foo\""},
		{"too few qubit args", header + "qreg q[2];\ncx q[0];", 4, 1, "takes 2 qubit argument(s), got 1"},
		{"missing params", header + "qreg q[1];\nrz q[0];", 4, 1, "takes 1 parameter(s), got 0"},
		{"unexpected params", header + "qreg q[1];\nh(pi) q[0];", 4, 1, "takes 0 parameter(s), got 1"},
		{"duplicate qubit", header + "qreg q[2];\ncx q[1], q[1];", 4, 10, "duplicate qubit argument"},
		{"bad expression", header + "qreg q[1];\nrz(pi +) q[0];", 4, 8, "expected expression"},
		{"unbalanced paren", header + "qreg q[1];\nrz((pi/2 q[0];", 4, 10, "expected )"},
		{"division by zero", header + "qreg q[1];\nrz(pi/0) q[0];", 4, 6, "division by zero"},
		{"illegal rune", header + "qreg q[1];\nh @;", 4, 3, "invalid token"},
		{"truncated input", header + "qreg q[1];\nh q[", 4, 5, "expected integer, found end of input"},
		{"bad include", header + "include \"other.inc\";", 3, 9, "only \"qelib1.inc\""},
		{"expression identifier", header + "qreg q[1];\nrz(theta) q[0];", 4, 4, "expected expression"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorAt(t, tt.src, tt.line, tt.col, tt.want)
		})
	}
}

func TestUnsupportedConstructs(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		line, col int
		want      string
	}{
		{"gate definition", header + "gate mygate a, b { cx a, b; }", 3, 1, "custom gate definitions are not supported"},
		{"if statement", header + "qreg q[1]; creg c[1];\nif (c == 1) x q[0];", 4, 1, "if statements are not supported"},
		{"opaque", header + "opaque magic q;", 3, 1, "opaque declarations are not supported"},
		{"reset", header + "qreg q[1];\nreset q[0];", 4, 1, "reset statements are not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorAt(t, tt.src, tt.line, tt.col, tt.want)
		})
	}
}

func TestErrorStringFormat(t *testing.T) {
	_, err := Parse("OPENQASM 2.0;\nqreg q[2]\nh q[0];")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.HasPrefix(err.Error(), "3:1: ") {
		t.Errorf("error %q does not start with \"3:1: \"", err.Error())
	}
}
