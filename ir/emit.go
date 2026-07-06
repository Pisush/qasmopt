package ir

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// WriteQASM writes the program back out as OpenQASM 2.0 source. The
// output is normalized: one statement per line, every register reference
// fully indexed (broadcast sugar from the input is expanded), and gate
// parameters printed with the shortest float64 representation that
// round-trips exactly.
func (p *Program) WriteQASM(w io.Writer) error {
	bw := &errWriter{w: w}
	bw.printf("OPENQASM 2.0;\n")
	bw.printf("include \"qelib1.inc\";\n")
	for _, r := range p.QRegs {
		bw.printf("qreg %s[%d];\n", r.Name, r.Size)
	}
	for _, r := range p.CRegs {
		bw.printf("creg %s[%d];\n", r.Name, r.Size)
	}
	qnames := indexNames(p.QRegs)
	cnames := indexNames(p.CRegs)
	for _, op := range p.Ops {
		bw.printf("%s\n", formatOp(op, qnames, cnames))
	}
	return bw.err
}

// String renders the program as OpenQASM 2.0 source (see WriteQASM).
func (p *Program) String() string {
	var sb strings.Builder
	_ = p.WriteQASM(&sb) // strings.Builder never fails
	return sb.String()
}

// indexNames builds the reverse mapping from global index to "name[i]".
func indexNames(regs []Reg) []string {
	names := make([]string, regTotal(regs))
	for _, r := range regs {
		for i := 0; i < r.Size; i++ {
			names[r.Offset+i] = fmt.Sprintf("%s[%d]", r.Name, i)
		}
	}
	return names
}

// formatOp renders one op as a QASM statement (without newline).
func formatOp(op Op, qnames, cnames []string) string {
	var sb strings.Builder
	if op.Name == OpMeasure {
		fmt.Fprintf(&sb, "measure %s -> %s;", qnames[op.Qubits[0]], cnames[op.Cbits[0]])
		return sb.String()
	}
	sb.WriteString(op.Name)
	if len(op.Params) > 0 {
		sb.WriteByte('(')
		for i, v := range op.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(formatParam(v))
		}
		sb.WriteByte(')')
	}
	for i, q := range op.Qubits {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte(' ')
		sb.WriteString(qnames[q])
	}
	sb.WriteByte(';')
	return sb.String()
}

// formatParam prints a gate parameter with the shortest representation
// that re-parses to the identical float64. Integral values print without
// a decimal point (e.g. "3"); the parser reads them back as the same
// float64, which is what matters.
func formatParam(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// errWriter wraps an io.Writer, remembering the first error so the
// emitter can check once at the end.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...any) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}
