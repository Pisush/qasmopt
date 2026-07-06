package parser

import (
	"math"
	"strconv"

	"github.com/Pisush/qasmopt/token"
)

// Gate-parameter expressions are evaluated to float64 at parse time — no
// symbolic parameters in v1. The grammar is:
//
//	expr    := term  { ("+" | "-") term }
//	term    := unary { ("*" | "/") unary }
//	unary   := "-" unary | primary
//	primary := INT | REAL | "pi" | "(" expr ")"

// parseExpr parses and evaluates an additive expression.
func (p *Parser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.tok.Kind == token.PLUS || p.tok.Kind == token.MINUS {
		op := p.tok.Kind
		p.next()
		rhs, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == token.PLUS {
			v += rhs
		} else {
			v -= rhs
		}
	}
	return v, nil
}

// parseTerm parses and evaluates a multiplicative expression.
func (p *Parser) parseTerm() (float64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for p.tok.Kind == token.STAR || p.tok.Kind == token.SLASH {
		op := p.tok
		p.next()
		rhs, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		if op.Kind == token.STAR {
			v *= rhs
		} else {
			if rhs == 0 {
				return 0, errorf(op.Pos, "division by zero in gate parameter")
			}
			v /= rhs
		}
	}
	return v, nil
}

// parseUnary parses an optional chain of unary minuses before a primary.
func (p *Parser) parseUnary() (float64, error) {
	if p.tok.Kind == token.MINUS {
		p.next()
		v, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -v, nil
	}
	return p.parsePrimary()
}

// parsePrimary parses a numeric literal, pi, or a parenthesized
// expression.
func (p *Parser) parsePrimary() (float64, error) {
	tok := p.tok
	switch tok.Kind {
	case token.INT, token.REAL:
		v, err := strconv.ParseFloat(tok.Lit, 64)
		if err != nil {
			return 0, errorf(tok.Pos, "invalid numeric literal %q", tok.Lit)
		}
		p.next()
		return v, nil
	case token.PI:
		p.next()
		return math.Pi, nil
	case token.LPAREN:
		p.next()
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if _, err := p.expect(token.RPAREN); err != nil {
			return 0, err
		}
		return v, nil
	default:
		return 0, errorf(tok.Pos, "expected expression, found %s", describe(tok))
	}
}
