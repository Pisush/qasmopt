package lexer

import (
	"testing"

	"github.com/Pisush/qasmopt/token"
)

// collect drains the lexer into a slice, stopping after EOF or ILLEGAL.
func collect(src string) []token.Token {
	l := New(src)
	var toks []token.Token
	for {
		tok := l.Next()
		toks = append(toks, tok)
		if tok.Kind == token.EOF || tok.Kind == token.ILLEGAL {
			return toks
		}
	}
}

func TestTokens(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Token
	}{
		{
			name: "header",
			src:  "OPENQASM 2.0;",
			want: []token.Token{
				{Kind: token.OPENQASM, Lit: "OPENQASM", Pos: token.Pos{Line: 1, Col: 1}},
				{Kind: token.REAL, Lit: "2.0", Pos: token.Pos{Line: 1, Col: 10}},
				{Kind: token.SEMICOLON, Lit: ";", Pos: token.Pos{Line: 1, Col: 13}},
				{Kind: token.EOF, Pos: token.Pos{Line: 1, Col: 14}},
			},
		},
		{
			name: "include string",
			src:  `include "qelib1.inc";`,
			want: []token.Token{
				{Kind: token.INCLUDE, Lit: "include", Pos: token.Pos{Line: 1, Col: 1}},
				{Kind: token.STRING, Lit: "qelib1.inc", Pos: token.Pos{Line: 1, Col: 9}},
				{Kind: token.SEMICOLON, Lit: ";", Pos: token.Pos{Line: 1, Col: 21}},
				{Kind: token.EOF, Pos: token.Pos{Line: 1, Col: 22}},
			},
		},
		{
			name: "measure with arrow",
			src:  "measure q[0] -> c[0];",
			want: []token.Token{
				{Kind: token.MEASURE, Lit: "measure", Pos: token.Pos{Line: 1, Col: 1}},
				{Kind: token.IDENT, Lit: "q", Pos: token.Pos{Line: 1, Col: 9}},
				{Kind: token.LBRACKET, Lit: "[", Pos: token.Pos{Line: 1, Col: 10}},
				{Kind: token.INT, Lit: "0", Pos: token.Pos{Line: 1, Col: 11}},
				{Kind: token.RBRACKET, Lit: "]", Pos: token.Pos{Line: 1, Col: 12}},
				{Kind: token.ARROW, Lit: "->", Pos: token.Pos{Line: 1, Col: 14}},
				{Kind: token.IDENT, Lit: "c", Pos: token.Pos{Line: 1, Col: 17}},
				{Kind: token.LBRACKET, Lit: "[", Pos: token.Pos{Line: 1, Col: 18}},
				{Kind: token.INT, Lit: "0", Pos: token.Pos{Line: 1, Col: 19}},
				{Kind: token.RBRACKET, Lit: "]", Pos: token.Pos{Line: 1, Col: 20}},
				{Kind: token.SEMICOLON, Lit: ";", Pos: token.Pos{Line: 1, Col: 21}},
				{Kind: token.EOF, Pos: token.Pos{Line: 1, Col: 22}},
			},
		},
		{
			name: "expression operators",
			src:  "rz(-pi/2 + 0.5*3) q;",
			want: []token.Token{
				{Kind: token.IDENT, Lit: "rz", Pos: token.Pos{Line: 1, Col: 1}},
				{Kind: token.LPAREN, Lit: "(", Pos: token.Pos{Line: 1, Col: 3}},
				{Kind: token.MINUS, Lit: "-", Pos: token.Pos{Line: 1, Col: 4}},
				{Kind: token.PI, Lit: "pi", Pos: token.Pos{Line: 1, Col: 5}},
				{Kind: token.SLASH, Lit: "/", Pos: token.Pos{Line: 1, Col: 7}},
				{Kind: token.INT, Lit: "2", Pos: token.Pos{Line: 1, Col: 8}},
				{Kind: token.PLUS, Lit: "+", Pos: token.Pos{Line: 1, Col: 10}},
				{Kind: token.REAL, Lit: "0.5", Pos: token.Pos{Line: 1, Col: 12}},
				{Kind: token.STAR, Lit: "*", Pos: token.Pos{Line: 1, Col: 15}},
				{Kind: token.INT, Lit: "3", Pos: token.Pos{Line: 1, Col: 16}},
				{Kind: token.RPAREN, Lit: ")", Pos: token.Pos{Line: 1, Col: 17}},
				{Kind: token.IDENT, Lit: "q", Pos: token.Pos{Line: 1, Col: 19}},
				{Kind: token.SEMICOLON, Lit: ";", Pos: token.Pos{Line: 1, Col: 20}},
				{Kind: token.EOF, Pos: token.Pos{Line: 1, Col: 21}},
			},
		},
		{
			name: "comments and newlines",
			src:  "// header comment\nqreg q[2]; // trailing\ncreg c[2];",
			want: []token.Token{
				{Kind: token.QREG, Lit: "qreg", Pos: token.Pos{Line: 2, Col: 1}},
				{Kind: token.IDENT, Lit: "q", Pos: token.Pos{Line: 2, Col: 6}},
				{Kind: token.LBRACKET, Lit: "[", Pos: token.Pos{Line: 2, Col: 7}},
				{Kind: token.INT, Lit: "2", Pos: token.Pos{Line: 2, Col: 8}},
				{Kind: token.RBRACKET, Lit: "]", Pos: token.Pos{Line: 2, Col: 9}},
				{Kind: token.SEMICOLON, Lit: ";", Pos: token.Pos{Line: 2, Col: 10}},
				{Kind: token.CREG, Lit: "creg", Pos: token.Pos{Line: 3, Col: 1}},
				{Kind: token.IDENT, Lit: "c", Pos: token.Pos{Line: 3, Col: 6}},
				{Kind: token.LBRACKET, Lit: "[", Pos: token.Pos{Line: 3, Col: 7}},
				{Kind: token.INT, Lit: "2", Pos: token.Pos{Line: 3, Col: 8}},
				{Kind: token.RBRACKET, Lit: "]", Pos: token.Pos{Line: 3, Col: 9}},
				{Kind: token.SEMICOLON, Lit: ";", Pos: token.Pos{Line: 3, Col: 10}},
				{Kind: token.EOF, Pos: token.Pos{Line: 3, Col: 11}},
			},
		},
		{
			name: "excluded keywords are lexed",
			src:  "gate if opaque reset barrier",
			want: []token.Token{
				{Kind: token.GATE, Lit: "gate", Pos: token.Pos{Line: 1, Col: 1}},
				{Kind: token.IF, Lit: "if", Pos: token.Pos{Line: 1, Col: 6}},
				{Kind: token.OPAQUE, Lit: "opaque", Pos: token.Pos{Line: 1, Col: 9}},
				{Kind: token.RESET, Lit: "reset", Pos: token.Pos{Line: 1, Col: 16}},
				{Kind: token.BARRIER, Lit: "barrier", Pos: token.Pos{Line: 1, Col: 22}},
				{Kind: token.EOF, Pos: token.Pos{Line: 1, Col: 29}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collect(tt.src)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d tokens, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("token %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNumberLiterals(t *testing.T) {
	tests := []struct {
		src  string
		kind token.Kind
		lit  string
	}{
		{"42", token.INT, "42"},
		{"0", token.INT, "0"},
		{"2.0", token.REAL, "2.0"},
		{"3.", token.REAL, "3."},
		{".5", token.REAL, ".5"},
		{"1e3", token.REAL, "1e3"},
		{"1.5e-3", token.REAL, "1.5e-3"},
		{"2E+4", token.REAL, "2E+4"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			tok := New(tt.src).Next()
			if tok.Kind != tt.kind || tok.Lit != tt.lit {
				t.Errorf("lex(%q) = %v %q, want %v %q", tt.src, tok.Kind, tok.Lit, tt.kind, tt.lit)
			}
		})
	}
}

func TestIllegalInput(t *testing.T) {
	tests := []struct {
		name string
		src  string
		pos  token.Pos
	}{
		{"stray symbol", "qreg q[2]; @", token.Pos{Line: 1, Col: 12}},
		{"unterminated string", "include \"qelib1.inc", token.Pos{Line: 1, Col: 9}},
		{"bad exponent", "1e+; x q;", token.Pos{Line: 1, Col: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := collect(tt.src)
			last := toks[len(toks)-1]
			if last.Kind != token.ILLEGAL {
				t.Fatalf("expected ILLEGAL token, got %v", last)
			}
			if last.Pos != tt.pos {
				t.Errorf("ILLEGAL at %v, want %v", last.Pos, tt.pos)
			}
		})
	}
}

func TestEOFIsSticky(t *testing.T) {
	l := New("x")
	l.Next() // ident
	for i := 0; i < 3; i++ {
		if tok := l.Next(); tok.Kind != token.EOF {
			t.Fatalf("call %d after end: got %v, want EOF", i, tok)
		}
	}
}
