// Package ast defines the abstract syntax tree produced by the qasmopt
// parser for the supported OpenQASM 2.0 subset. Gate parameters are
// evaluated to float64 at parse time, so the AST carries no expression
// nodes.
package ast

import (
	"strconv"

	"github.com/Pisush/qasmopt/token"
)

// Node is implemented by all AST nodes.
type Node interface {
	// Pos returns the position of the first token of the node.
	Pos() token.Pos
}

// Program is a parsed OpenQASM 2.0 source file.
type Program struct {
	VersionPos token.Pos
	Version    string // version literal from the header, e.g. "2.0"
	Stmts      []Stmt // statements in source order, including includes
}

// Pos returns the position of the OPENQASM header.
func (p *Program) Pos() token.Pos { return p.VersionPos }

// Stmt is implemented by all statement nodes.
type Stmt interface {
	Node
	stmt()
}

// RegKind distinguishes quantum from classical register declarations.
type RegKind int

// Register kinds.
const (
	QReg RegKind = iota // qreg: quantum register
	CReg                // creg: classical register
)

// String returns "qreg" or "creg".
func (k RegKind) String() string {
	if k == CReg {
		return "creg"
	}
	return "qreg"
}

// Include is an `include "...";` statement. Per the spec, qelib1.inc is
// treated as an implicit standard library and never read from disk.
type Include struct {
	IncludePos token.Pos
	Path       string
}

// RegDecl is a `qreg name[size];` or `creg name[size];` declaration.
type RegDecl struct {
	DeclPos token.Pos
	Kind    RegKind
	Name    string
	Size    int
}

// Arg is a register reference in a gate, measure, or barrier statement:
// either an indexed single (qu)bit like q[2], or a whole register like q
// (broadcast form).
type Arg struct {
	ArgPos  token.Pos
	Reg     string
	Index   int  // valid only if Indexed
	Indexed bool // true for q[i], false for bare q
}

// Pos returns the position of the register name.
func (a Arg) Pos() token.Pos { return a.ArgPos }

// String renders the argument as it would appear in QASM source.
func (a Arg) String() string {
	if !a.Indexed {
		return a.Reg
	}
	return a.Reg + "[" + strconv.Itoa(a.Index) + "]"
}

// GateStmt is an application of a standard gate, e.g. `cx q[0], q[1];` or
// `rz(pi/2) q[0];`. Params are already evaluated to float64.
type GateStmt struct {
	NamePos token.Pos
	Name    string
	Params  []float64
	Args    []Arg
}

// MeasureStmt is `measure src -> dst;`.
type MeasureStmt struct {
	MeasurePos token.Pos
	Src        Arg // quantum source
	Dst        Arg // classical destination
}

// BarrierStmt is `barrier args...;`, an optimization fence.
type BarrierStmt struct {
	BarrierPos token.Pos
	Args       []Arg
}

// Pos implementations.

// Pos returns the position of the include keyword.
func (s *Include) Pos() token.Pos { return s.IncludePos }

// Pos returns the position of the qreg/creg keyword.
func (s *RegDecl) Pos() token.Pos { return s.DeclPos }

// Pos returns the position of the gate name.
func (s *GateStmt) Pos() token.Pos { return s.NamePos }

// Pos returns the position of the measure keyword.
func (s *MeasureStmt) Pos() token.Pos { return s.MeasurePos }

// Pos returns the position of the barrier keyword.
func (s *BarrierStmt) Pos() token.Pos { return s.BarrierPos }

func (*Include) stmt()     {}
func (*RegDecl) stmt()     {}
func (*GateStmt) stmt()    {}
func (*MeasureStmt) stmt() {}
func (*BarrierStmt) stmt() {}
