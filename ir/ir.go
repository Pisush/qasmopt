// Package ir defines qasmopt's flat intermediate representation: a
// program is a list of register declarations plus a flat []Op sequence
// in which register references have been flattened to global indices
// (q[0], q[1], r[0] -> 0, 1, 2). Barriers and measurements are ordinary
// IR ops, so a single slice captures the whole circuit.
package ir

import (
	"fmt"

	"github.com/Pisush/qasmopt/ast"
	"github.com/Pisush/qasmopt/token"
)

// Names of the non-gate ops present in the IR.
const (
	// OpBarrier is an optimization fence spanning Qubits.
	OpBarrier = "barrier"
	// OpMeasure measures Qubits[0] into classical bit Cbits[0].
	OpMeasure = "measure"
)

// Op is a single flattened circuit operation: a standard gate, a
// barrier, or a measurement. Qubits and Cbits hold global indices.
type Op struct {
	Name   string
	Qubits []int
	Params []float64
	Cbits  []int // classical targets; only used by measure
}

// Reg describes a declared register and the global index range
// [Offset, Offset+Size) assigned to it during flattening. Quantum and
// classical registers occupy separate index spaces.
type Reg struct {
	Name   string
	Size   int
	Offset int
}

// Program is a flattened circuit.
type Program struct {
	QRegs []Reg // quantum registers, in declaration order
	CRegs []Reg // classical registers, in declaration order
	Ops   []Op  // ops in source order
}

// NumQubits returns the total number of flattened qubits.
func (p *Program) NumQubits() int { return regTotal(p.QRegs) }

// NumCbits returns the total number of flattened classical bits.
func (p *Program) NumCbits() int { return regTotal(p.CRegs) }

func regTotal(regs []Reg) int {
	n := 0
	for _, r := range regs {
		n += r.Size
	}
	return n
}

// Error is a lowering error at a specific source position.
type Error struct {
	Pos token.Pos
	Msg string
}

// Error returns the message prefixed with "line:col".
func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

func errorf(pos token.Pos, format string, args ...any) error {
	return &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// regKind distinguishes symbol-table entries.
type symbol struct {
	kind ast.RegKind
	reg  Reg
}

// lowerer accumulates state while flattening an AST.
type lowerer struct {
	prog *Program
	syms map[string]symbol
}

// Lower flattens a parsed program into IR, resolving register references
// to global indices. It reports (with source positions) redeclared or
// undeclared registers, out-of-range indices, kind mismatches (creg used
// as a qubit and vice versa), and broadcast forms not supported in v1.
func Lower(prog *ast.Program) (*Program, error) {
	lw := &lowerer{prog: &Program{}, syms: make(map[string]symbol)}
	for _, stmt := range prog.Stmts {
		if err := lw.stmt(stmt); err != nil {
			return nil, err
		}
	}
	return lw.prog, nil
}

func (lw *lowerer) stmt(stmt ast.Stmt) error {
	switch s := stmt.(type) {
	case *ast.Include:
		return nil // qelib1.inc is the implicit stdlib; nothing to do
	case *ast.RegDecl:
		return lw.regDecl(s)
	case *ast.GateStmt:
		return lw.gate(s)
	case *ast.MeasureStmt:
		return lw.measure(s)
	case *ast.BarrierStmt:
		return lw.barrier(s)
	default:
		// Unreachable while the parser and IR agree on statement kinds.
		panic(fmt.Sprintf("ir: unknown statement type %T", stmt))
	}
}

func (lw *lowerer) regDecl(s *ast.RegDecl) error {
	if prev, ok := lw.syms[s.Name]; ok {
		return errorf(s.Pos(), "register %q already declared as %s", s.Name, prev.kind)
	}
	regs := &lw.prog.QRegs
	if s.Kind == ast.CReg {
		regs = &lw.prog.CRegs
	}
	reg := Reg{Name: s.Name, Size: s.Size, Offset: regTotal(*regs)}
	*regs = append(*regs, reg)
	lw.syms[s.Name] = symbol{kind: s.Kind, reg: reg}
	return nil
}

// resolve looks up arg as a register of the wanted kind and returns its
// Reg. Index bounds are checked for indexed args.
func (lw *lowerer) resolve(arg ast.Arg, want ast.RegKind) (Reg, error) {
	sym, ok := lw.syms[arg.Reg]
	if !ok {
		return Reg{}, errorf(arg.Pos(), "undeclared register %q", arg.Reg)
	}
	if sym.kind != want {
		return Reg{}, errorf(arg.Pos(), "register %q is a %s, but a %s is required here", arg.Reg, sym.kind, want)
	}
	if arg.Indexed && arg.Index >= sym.reg.Size {
		return Reg{}, errorf(arg.Pos(), "index %d out of range for register %q of size %d", arg.Index, arg.Reg, sym.reg.Size)
	}
	return sym.reg, nil
}

// qubit resolves an indexed quantum argument to its global index.
func (lw *lowerer) qubit(arg ast.Arg) (int, error) {
	reg, err := lw.resolve(arg, ast.QReg)
	if err != nil {
		return 0, err
	}
	if !arg.Indexed {
		return 0, errorf(arg.Pos(), "whole-register argument %q is not allowed here", arg.Reg)
	}
	return reg.Offset + arg.Index, nil
}

func (lw *lowerer) gate(s *ast.GateStmt) error {
	// Broadcast form: a single-qubit gate applied to a whole register
	// expands to one op per element. For two-qubit gates (cx), v1
	// requires indexed arguments; QASM's mixed broadcast semantics are
	// out of scope.
	if len(s.Args) == 1 && !s.Args[0].Indexed {
		reg, err := lw.resolve(s.Args[0], ast.QReg)
		if err != nil {
			return err
		}
		for i := 0; i < reg.Size; i++ {
			lw.emit(Op{Name: s.Name, Qubits: []int{reg.Offset + i}, Params: s.Params})
		}
		return nil
	}
	qubits := make([]int, 0, len(s.Args))
	for _, arg := range s.Args {
		if !arg.Indexed {
			return errorf(arg.Pos(), "whole-register argument %q to multi-qubit gate %q is not supported in v1; index it explicitly", arg.Reg, s.Name)
		}
		q, err := lw.qubit(arg)
		if err != nil {
			return err
		}
		qubits = append(qubits, q)
	}
	lw.emit(Op{Name: s.Name, Qubits: qubits, Params: s.Params})
	return nil
}

func (lw *lowerer) measure(s *ast.MeasureStmt) error {
	src, err := lw.resolve(s.Src, ast.QReg)
	if err != nil {
		return err
	}
	dst, err := lw.resolve(s.Dst, ast.CReg)
	if err != nil {
		return err
	}
	switch {
	case s.Src.Indexed && s.Dst.Indexed:
		lw.emit(Op{
			Name:   OpMeasure,
			Qubits: []int{src.Offset + s.Src.Index},
			Cbits:  []int{dst.Offset + s.Dst.Index},
		})
	case !s.Src.Indexed && !s.Dst.Indexed:
		// Broadcast measure: registers must have equal size.
		if src.Size != dst.Size {
			return errorf(s.Pos(), "measure %s -> %s: register sizes differ (%d vs %d)", s.Src.Reg, s.Dst.Reg, src.Size, dst.Size)
		}
		for i := 0; i < src.Size; i++ {
			lw.emit(Op{
				Name:   OpMeasure,
				Qubits: []int{src.Offset + i},
				Cbits:  []int{dst.Offset + i},
			})
		}
	default:
		return errorf(s.Pos(), "measure requires both sides indexed or both whole registers")
	}
	return nil
}

func (lw *lowerer) barrier(s *ast.BarrierStmt) error {
	var qubits []int
	seen := make(map[int]bool)
	for _, arg := range s.Args {
		reg, err := lw.resolve(arg, ast.QReg)
		if err != nil {
			return err
		}
		lo, hi := 0, reg.Size
		if arg.Indexed {
			lo, hi = arg.Index, arg.Index+1
		}
		for i := lo; i < hi; i++ {
			q := reg.Offset + i
			if seen[q] {
				return errorf(arg.Pos(), "duplicate qubit %s in barrier", arg)
			}
			seen[q] = true
			qubits = append(qubits, q)
		}
	}
	lw.emit(Op{Name: OpBarrier, Qubits: qubits})
	return nil
}

func (lw *lowerer) emit(op Op) { lw.prog.Ops = append(lw.prog.Ops, op) }

// Stats counts ops by name. Barriers and measures are counted like any
// other op under their own names.
func Stats(ops []Op) map[string]int {
	counts := make(map[string]int)
	for _, op := range ops {
		counts[op.Name]++
	}
	return counts
}
