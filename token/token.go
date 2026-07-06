// Package token defines the lexical tokens of the OpenQASM 2.0 subset
// understood by qasmopt, along with source positions.
package token

import "fmt"

// Pos is a source position. Both Line and Col are 1-based; Col counts
// runes, not bytes.
type Pos struct {
	Line int
	Col  int
}

// String returns the position in "line:col" form.
func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Kind identifies the lexical class of a token.
type Kind int

// Token kinds. Keywords that name constructs excluded from v1 (GATE, IF,
// OPAQUE, RESET) are still lexed so the parser can report a precise
// "unsupported construct" error instead of a generic syntax error.
const (
	ILLEGAL Kind = iota // unrecognized rune or malformed literal
	EOF                 // end of input

	IDENT  // identifiers: gate and register names
	INT    // integer literal, e.g. 3
	REAL   // floating-point literal, e.g. 2.0, .5, 1e-3
	STRING // quoted string literal, e.g. "qelib1.inc"

	// Keywords.
	OPENQASM
	INCLUDE
	QREG
	CREG
	MEASURE
	BARRIER
	PI
	GATE
	IF
	OPAQUE
	RESET

	// Punctuation and operators.
	SEMICOLON // ;
	COMMA     // ,
	LPAREN    // (
	RPAREN    // )
	LBRACKET  // [
	RBRACKET  // ]
	LBRACE    // {
	RBRACE    // }
	ARROW     // ->
	PLUS      // +
	MINUS     // -
	STAR      // *
	SLASH     // /
)

var kindNames = [...]string{
	ILLEGAL:   "ILLEGAL",
	EOF:       "EOF",
	IDENT:     "identifier",
	INT:       "integer",
	REAL:      "real",
	STRING:    "string",
	OPENQASM:  "OPENQASM",
	INCLUDE:   "include",
	QREG:      "qreg",
	CREG:      "creg",
	MEASURE:   "measure",
	BARRIER:   "barrier",
	PI:        "pi",
	GATE:      "gate",
	IF:        "if",
	OPAQUE:    "opaque",
	RESET:     "reset",
	SEMICOLON: ";",
	COMMA:     ",",
	LPAREN:    "(",
	RPAREN:    ")",
	LBRACKET:  "[",
	RBRACKET:  "]",
	LBRACE:    "{",
	RBRACE:    "}",
	ARROW:     "->",
	PLUS:      "+",
	MINUS:     "-",
	STAR:      "*",
	SLASH:     "/",
}

// String returns a human-readable name for the kind, suitable for error
// messages ("identifier", ";", "OPENQASM", ...).
func (k Kind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return fmt.Sprintf("Kind(%d)", int(k))
	}
	return kindNames[k]
}

// Token is a single lexical token with its literal text and position.
type Token struct {
	Kind Kind
	Lit  string // literal text as it appeared in the source
	Pos  Pos    // position of the first rune of the token
}

var keywords = map[string]Kind{
	"OPENQASM": OPENQASM,
	"include":  INCLUDE,
	"qreg":     QREG,
	"creg":     CREG,
	"measure":  MEASURE,
	"barrier":  BARRIER,
	"pi":       PI,
	"gate":     GATE,
	"if":       IF,
	"opaque":   OPAQUE,
	"reset":    RESET,
}

// Lookup maps an identifier to its keyword kind, or IDENT if it is not a
// keyword.
func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return IDENT
}
