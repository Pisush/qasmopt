// Package parser implements a recursive-descent parser for the OpenQASM
// 2.0 subset supported by qasmopt. It produces an *ast.Program and
// reports errors as values carrying line/column positions.
//
// Constructs excluded from v1 (gate definitions, if, opaque, reset) are
// rejected with a clear "unsupported" error rather than a syntax error.
package parser

import (
	"fmt"
	"math"

	"github.com/Pisush/qasmopt/ast"
	"github.com/Pisush/qasmopt/lexer"
	"github.com/Pisush/qasmopt/token"
)

// Error is a parse error at a specific source position.
type Error struct {
	Pos token.Pos
	Msg string
}

// Error returns the message prefixed with "line:col".
func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

// errorf builds an *Error at pos.
func errorf(pos token.Pos, format string, args ...any) error {
	return &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// gateSpec records the arity of a supported standard gate.
type gateSpec struct {
	params int // number of parenthesized parameters
	qubits int // number of quantum arguments
}

// gateSpecs lists the standard gates of the supported subset (the
// relevant part of qelib1.inc plus cx). Anything else is treated as an
// unsupported custom gate.
var gateSpecs = map[string]gateSpec{
	"x":   {0, 1},
	"y":   {0, 1},
	"z":   {0, 1},
	"h":   {0, 1},
	"s":   {0, 1},
	"sdg": {0, 1},
	"t":   {0, 1},
	"tdg": {0, 1},
	"cx":  {0, 2},
	"rx":  {1, 1},
	"ry":  {1, 1},
	"rz":  {1, 1},
	"u1":  {1, 1},
	"u2":  {2, 1},
	"u3":  {3, 1},
}

// unsupported maps keywords of excluded v1 constructs to a description
// used in error messages.
var unsupported = map[token.Kind]string{
	token.GATE:   "custom gate definitions",
	token.IF:     "if statements",
	token.OPAQUE: "opaque declarations",
	token.RESET:  "reset statements",
}

// Parser holds the state of a single parse. Create one with New or use
// the Parse convenience function.
type Parser struct {
	lex *lexer.Lexer
	tok token.Token // current token
}

// New returns a Parser reading src.
func New(src string) *Parser {
	p := &Parser{lex: lexer.New(src)}
	p.next()
	return p
}

// Parse parses a complete OpenQASM 2.0 source string. It stops at the
// first error.
func Parse(src string) (*ast.Program, error) {
	return New(src).ParseProgram()
}

// next advances to the following token.
func (p *Parser) next() { p.tok = p.lex.Next() }

// expect consumes a token of the given kind or fails with a positioned
// error naming what was expected.
func (p *Parser) expect(kind token.Kind) (token.Token, error) {
	tok := p.tok
	if tok.Kind != kind {
		return tok, errorf(tok.Pos, "expected %s, found %s", kind, describe(tok))
	}
	p.next()
	return tok, nil
}

// describe renders a token for error messages.
func describe(tok token.Token) string {
	switch tok.Kind {
	case token.EOF:
		return "end of input"
	case token.ILLEGAL:
		return fmt.Sprintf("invalid token %q", tok.Lit)
	case token.IDENT, token.INT, token.REAL, token.STRING:
		return fmt.Sprintf("%s %q", tok.Kind, tok.Lit)
	default:
		return fmt.Sprintf("%q", tok.Lit)
	}
}

// ParseProgram parses the OPENQASM header followed by statements until
// EOF.
func (p *Parser) ParseProgram() (*ast.Program, error) {
	head, err := p.expect(token.OPENQASM)
	if err != nil {
		return nil, err
	}
	ver := p.tok
	if ver.Kind != token.REAL && ver.Kind != token.INT {
		return nil, errorf(ver.Pos, "expected version number after OPENQASM, found %s", describe(ver))
	}
	if ver.Lit != "2.0" {
		return nil, errorf(ver.Pos, "unsupported OpenQASM version %q (only 2.0 is supported)", ver.Lit)
	}
	p.next()
	if _, err := p.expect(token.SEMICOLON); err != nil {
		return nil, err
	}

	prog := &ast.Program{VersionPos: head.Pos, Version: ver.Lit}
	for p.tok.Kind != token.EOF {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		prog.Stmts = append(prog.Stmts, stmt)
	}
	return prog, nil
}

// parseStmt parses a single statement.
func (p *Parser) parseStmt() (ast.Stmt, error) {
	tok := p.tok
	if desc, ok := unsupported[tok.Kind]; ok {
		return nil, errorf(tok.Pos, "unsupported construct %q: %s are not supported in v1", tok.Lit, desc)
	}
	switch tok.Kind {
	case token.INCLUDE:
		return p.parseInclude()
	case token.QREG, token.CREG:
		return p.parseRegDecl()
	case token.MEASURE:
		return p.parseMeasure()
	case token.BARRIER:
		return p.parseBarrier()
	case token.IDENT:
		return p.parseGate()
	default:
		return nil, errorf(tok.Pos, "expected statement, found %s", describe(tok))
	}
}

// parseInclude parses `include "path";`. Only qelib1.inc is accepted; it
// is treated as an implicit standard library and never read from disk.
func (p *Parser) parseInclude() (ast.Stmt, error) {
	kw := p.tok
	p.next()
	path, err := p.expect(token.STRING)
	if err != nil {
		return nil, err
	}
	if path.Lit != "qelib1.inc" {
		return nil, errorf(path.Pos, "unsupported include %q: only \"qelib1.inc\" is supported in v1", path.Lit)
	}
	if _, err := p.expect(token.SEMICOLON); err != nil {
		return nil, err
	}
	return &ast.Include{IncludePos: kw.Pos, Path: path.Lit}, nil
}

// parseRegDecl parses `qreg name[size];` or `creg name[size];`.
func (p *Parser) parseRegDecl() (ast.Stmt, error) {
	kw := p.tok
	kind := ast.QReg
	if kw.Kind == token.CREG {
		kind = ast.CReg
	}
	p.next()
	name, err := p.expect(token.IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.LBRACKET); err != nil {
		return nil, err
	}
	size, err := p.parseIndex()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, errorf(kw.Pos, "register %q must have at least one element", name.Lit)
	}
	if _, err := p.expect(token.RBRACKET); err != nil {
		return nil, err
	}
	if _, err := p.expect(token.SEMICOLON); err != nil {
		return nil, err
	}
	return &ast.RegDecl{DeclPos: kw.Pos, Kind: kind, Name: name.Lit, Size: size}, nil
}

// parseIndex parses a non-negative integer literal used as a register
// size or subscript.
func (p *Parser) parseIndex() (int, error) {
	tok, err := p.expect(token.INT)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range tok.Lit {
		n = n*10 + int(r-'0')
		if n > 1<<24 { // generous cap; avoids overflow on absurd literals
			return 0, errorf(tok.Pos, "index %s is too large", tok.Lit)
		}
	}
	return n, nil
}

// parseArg parses a register reference: `name` or `name[i]`.
func (p *Parser) parseArg() (ast.Arg, error) {
	name, err := p.expect(token.IDENT)
	if err != nil {
		return ast.Arg{}, err
	}
	arg := ast.Arg{ArgPos: name.Pos, Reg: name.Lit}
	if p.tok.Kind == token.LBRACKET {
		p.next()
		idx, err := p.parseIndex()
		if err != nil {
			return ast.Arg{}, err
		}
		if _, err := p.expect(token.RBRACKET); err != nil {
			return ast.Arg{}, err
		}
		arg.Index = idx
		arg.Indexed = true
	}
	return arg, nil
}

// parseArgList parses a comma-separated list of register references.
func (p *Parser) parseArgList() ([]ast.Arg, error) {
	var args []ast.Arg
	for {
		arg, err := p.parseArg()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.tok.Kind != token.COMMA {
			return args, nil
		}
		p.next()
	}
}

// parseGate parses a standard-gate application such as `h q[0];`,
// `rz(pi/2) q[1];`, or `cx q[0], q[1];`.
func (p *Parser) parseGate() (ast.Stmt, error) {
	name := p.tok
	spec, ok := gateSpecs[name.Lit]
	if !ok {
		return nil, errorf(name.Pos, "unknown gate %q: custom gates are not supported in v1", name.Lit)
	}
	p.next()

	var params []float64
	if p.tok.Kind == token.LPAREN {
		p.next()
		for {
			exprPos := p.tok.Pos
			v, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if math.IsInf(v, 0) || math.IsNaN(v) {
				return nil, errorf(exprPos, "gate parameter does not evaluate to a finite number")
			}
			params = append(params, v)
			if p.tok.Kind != token.COMMA {
				break
			}
			p.next()
		}
		if _, err := p.expect(token.RPAREN); err != nil {
			return nil, err
		}
	}
	if len(params) != spec.params {
		return nil, errorf(name.Pos, "gate %q takes %d parameter(s), got %d", name.Lit, spec.params, len(params))
	}

	args, err := p.parseArgList()
	if err != nil {
		return nil, err
	}
	if len(args) != spec.qubits {
		return nil, errorf(name.Pos, "gate %q takes %d qubit argument(s), got %d", name.Lit, spec.qubits, len(args))
	}
	for i := 0; i < len(args); i++ {
		for j := i + 1; j < len(args); j++ {
			if args[i].Reg == args[j].Reg && args[i].Indexed == args[j].Indexed &&
				(!args[i].Indexed || args[i].Index == args[j].Index) {
				return nil, errorf(args[j].ArgPos, "duplicate qubit argument %s in gate %q", args[j], name.Lit)
			}
		}
	}
	if _, err := p.expect(token.SEMICOLON); err != nil {
		return nil, err
	}
	return &ast.GateStmt{NamePos: name.Pos, Name: name.Lit, Params: params, Args: args}, nil
}

// parseMeasure parses `measure src -> dst;`.
func (p *Parser) parseMeasure() (ast.Stmt, error) {
	kw := p.tok
	p.next()
	src, err := p.parseArg()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.ARROW); err != nil {
		return nil, err
	}
	dst, err := p.parseArg()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.SEMICOLON); err != nil {
		return nil, err
	}
	return &ast.MeasureStmt{MeasurePos: kw.Pos, Src: src, Dst: dst}, nil
}

// parseBarrier parses `barrier args...;`.
func (p *Parser) parseBarrier() (ast.Stmt, error) {
	kw := p.tok
	p.next()
	args, err := p.parseArgList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.SEMICOLON); err != nil {
		return nil, err
	}
	return &ast.BarrierStmt{BarrierPos: kw.Pos, Args: args}, nil
}
