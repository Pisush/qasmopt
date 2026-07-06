// Package lexer implements a hand-written, rune-based lexer for the
// OpenQASM 2.0 subset supported by qasmopt. It emits tokens carrying
// 1-based line/column positions and never fails: unrecognized input is
// returned as a token.ILLEGAL token for the parser to report.
package lexer

import (
	"unicode"
	"unicode/utf8"

	"github.com/Pisush/qasmopt/token"
)

const eof = rune(-1)

// Lexer tokenizes OpenQASM 2.0 source held in memory.
type Lexer struct {
	src  string
	off  int // byte offset of the next rune
	line int // 1-based line of the next rune
	col  int // 1-based rune column of the next rune
}

// New returns a Lexer reading from src.
func New(src string) *Lexer {
	return &Lexer{src: src, line: 1, col: 1}
}

// peek returns the next rune without consuming it, or eof.
func (l *Lexer) peek() rune {
	if l.off >= len(l.src) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.off:])
	return r
}

// peek2 returns the rune after the next one without consuming, or eof.
func (l *Lexer) peek2() rune {
	if l.off >= len(l.src) {
		return eof
	}
	_, w := utf8.DecodeRuneInString(l.src[l.off:])
	if l.off+w >= len(l.src) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.off+w:])
	return r
}

// advance consumes and returns the next rune, updating line/col.
func (l *Lexer) advance() rune {
	if l.off >= len(l.src) {
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.src[l.off:])
	l.off += w
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

// pos returns the position of the next rune.
func (l *Lexer) pos() token.Pos { return token.Pos{Line: l.line, Col: l.col} }

// skipSpaceAndComments consumes whitespace and //-to-end-of-line comments.
func (l *Lexer) skipSpaceAndComments() {
	for {
		r := l.peek()
		switch {
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			l.advance()
		case r == '/' && l.peek2() == '/':
			for l.peek() != '\n' && l.peek() != eof {
				l.advance()
			}
		default:
			return
		}
	}
}

func isLetter(r rune) bool { return unicode.IsLetter(r) || r == '_' }
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }

// Next returns the next token. At end of input it returns an EOF token
// (and keeps returning it on subsequent calls).
func (l *Lexer) Next() token.Token {
	l.skipSpaceAndComments()
	pos := l.pos()
	r := l.peek()

	switch {
	case r == eof:
		return token.Token{Kind: token.EOF, Pos: pos}
	case isLetter(r):
		return l.lexIdent(pos)
	case isDigit(r) || (r == '.' && isDigit(l.peek2())):
		return l.lexNumber(pos)
	case r == '"':
		return l.lexString(pos)
	}

	l.advance()
	var kind token.Kind
	switch r {
	case ';':
		kind = token.SEMICOLON
	case ',':
		kind = token.COMMA
	case '(':
		kind = token.LPAREN
	case ')':
		kind = token.RPAREN
	case '[':
		kind = token.LBRACKET
	case ']':
		kind = token.RBRACKET
	case '{':
		kind = token.LBRACE
	case '}':
		kind = token.RBRACE
	case '+':
		kind = token.PLUS
	case '*':
		kind = token.STAR
	case '/':
		kind = token.SLASH
	case '-':
		if l.peek() == '>' {
			l.advance()
			return token.Token{Kind: token.ARROW, Lit: "->", Pos: pos}
		}
		kind = token.MINUS
	default:
		return token.Token{Kind: token.ILLEGAL, Lit: string(r), Pos: pos}
	}
	return token.Token{Kind: kind, Lit: kind.String(), Pos: pos}
}

// lexIdent lexes an identifier or keyword starting at the next rune.
func (l *Lexer) lexIdent(pos token.Pos) token.Token {
	start := l.off
	for isLetter(l.peek()) || isDigit(l.peek()) {
		l.advance()
	}
	lit := l.src[start:l.off]
	return token.Token{Kind: token.Lookup(lit), Lit: lit, Pos: pos}
}

// lexNumber lexes an integer or real literal: digits, an optional
// fractional part, and an optional exponent. A literal is REAL if it has
// a '.' or an exponent, INT otherwise.
func (l *Lexer) lexNumber(pos token.Pos) token.Token {
	start := l.off
	kind := token.INT
	for isDigit(l.peek()) {
		l.advance()
	}
	if l.peek() == '.' {
		kind = token.REAL
		l.advance()
		for isDigit(l.peek()) {
			l.advance()
		}
	}
	if r := l.peek(); r == 'e' || r == 'E' {
		next := l.peek2()
		if isDigit(next) || next == '+' || next == '-' {
			kind = token.REAL
			l.advance() // e
			if r := l.peek(); r == '+' || r == '-' {
				l.advance()
			}
			if !isDigit(l.peek()) {
				return token.Token{Kind: token.ILLEGAL, Lit: l.src[start:l.off], Pos: pos}
			}
			for isDigit(l.peek()) {
				l.advance()
			}
		}
	}
	return token.Token{Kind: kind, Lit: l.src[start:l.off], Pos: pos}
}

// lexString lexes a double-quoted string with no escape sequences, per the
// OpenQASM 2.0 grammar. The token literal excludes the quotes. An
// unterminated string yields an ILLEGAL token.
func (l *Lexer) lexString(pos token.Pos) token.Token {
	l.advance() // opening quote
	start := l.off
	for {
		r := l.peek()
		if r == '"' {
			lit := l.src[start:l.off]
			l.advance()
			return token.Token{Kind: token.STRING, Lit: lit, Pos: pos}
		}
		if r == eof || r == '\n' {
			return token.Token{Kind: token.ILLEGAL, Lit: l.src[start:l.off], Pos: pos}
		}
		l.advance()
	}
}
