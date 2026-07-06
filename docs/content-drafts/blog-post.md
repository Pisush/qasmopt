> DRAFT — technical blog post for qasmopt (github.com/Pisush/qasmopt)

# Writing a semantics-preserving quantum circuit optimizer in Go

Quantum circuits, once you strip away the physics, are just programs:
sequences of operations over a small register of typed variables. Which
means the classic compiler pipeline — lex, parse, lower to IR, run
peephole passes to a fixpoint — applies to them almost unchanged. qasmopt
is that pipeline built for OpenQASM 2.0, in pure Go with zero third-party
dependencies, and this post walks through each stage plus the part that
makes an optimizer trustworthy: proving it didn't change what the
circuit computes.

## The pipeline

qasmopt has four stages, each its own package: `lexer` turns source text
into tokens, `parser` turns tokens into an AST (with a small expression
evaluator for gate parameters), `ir` flattens the AST into a single flat
op list, and `opt` rewrites that op list to a fixpoint. A `cmd/qasmopt`
CLI wires all four together and can print before/after gate-count stats.

## A hand-written lexer

The lexer (`lexer/lexer.go`) is rune-based and hand-written — no
`regexp`, no generated state machine. It tracks byte offset plus 1-based
line and column as it advances, so every token carries a source position
for error messages:

```go
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
```

It never fails outright: unrecognized input becomes a `token.ILLEGAL`
token and the *parser* is the one that turns that into a reported error.
That split matters for a hand-rolled lexer — it keeps error recovery and
position reporting in one place (the parser, which already has to report
errors for other reasons) instead of duplicating it in the lexer too.
Numbers get real/int classification inline (a `.` or an exponent makes it
`REAL`), and identifiers are looked up against a keyword table so `qreg`,
`measure`, and friends come out as their own token kinds rather than
generic identifiers.

## Recursive descent, plus a tiny evaluator

The parser (`parser/parser.go`) is straightforward recursive descent:
one function per grammar production, each returning an AST node or a
positioned `*parser.Error`. What's more interesting is `parser/expr.go`,
which handles gate-parameter expressions like `rz(pi/2)` or
`u3(pi/4, -pi/2, pi)`. OpenQASM 2.0 parameters are constant expressions,
not runtime values, so qasmopt evaluates them to `float64` right there in
the parser — no symbolic representation ever exists:

```
expr    := term  { ("+" | "-") term }
term    := unary { ("*" | "/") unary }
unary   := "-" unary | primary
primary := INT | REAL | "pi" | "(" expr ")"
```

Four mutually recursive functions implement exactly that grammar,
handling `pi`, the four arithmetic operators, unary minus, and
parentheses — enough for every gate-parameter expression that shows up
in practice, without pulling in a general math-expression library for a
handful of lines of arithmetic.

The parser also draws a hard, explicit line around v1's scope: custom
gate definitions, `if`, `opaque`, and `reset` are recognized (so the
lexer/keyword table knows about them) but rejected with a clear
"unsupported construct" error rather than a confusing syntax error two
tokens later. Knowing what you don't support, and saying so immediately,
is underrated parser design.

## Flattening to a list of ops

`ir.Lower` (`ir/ir.go`) turns the AST into a `Program`: a list of
register descriptors plus a flat `[]Op`. The key move is flattening
register-relative references to global indices — `q[0]`, `q[1]`, `r[0]`
become plain integers 0, 1, 2 — so every later stage works with bare
qubit numbers instead of re-resolving register names each time:

```go
type Op struct {
    Name   string
    Qubits []int
    Params []float64
    Cbits  []int // classical targets; only used by measure
}
```

Barriers and measurements are just ops with their own `Name` (`"barrier"`,
`"measure"`), so the optimizer's passes only ever have to reason about one
sequence type, not a tree of statement kinds. The lowering step is also
where broadcast forms get expanded — `h q;` over a whole register becomes
one `h` op per qubit — and where a handful of static errors get caught:
undeclared registers, out-of-range indices, using a `creg` where a `qreg`
is required.

## The optimizer: three passes to a fixpoint

`opt.Optimize` (`opt/opt.go`) runs three passes repeatedly until a full
round leaves the op list unchanged:

- **`CancelInverses`** deletes adjacent inverse pairs — `h h`, `x x`,
  `cx cx` (matching control and target), `s`/`sdg`, `t`/`tdg` — and does
  it in one pass over the output-so-far, so cancellation cascades:
  in `x h h x`, removing the inner `h h` makes the outer `x x` adjacent
  and it's removed in the same sweep.
- **`MergeRotations`** fuses adjacent same-qubit rotations of the same
  kind: `rz(a) rz(b)` becomes `rz(a+b)`, and if the summed angle is
  congruent to 0 mod 2π (checked with an epsilon, never `==`) the op is
  dropped entirely.
- **`CancelAcrossWindow`** is the commutation-aware version of both: ops
  on disjoint qubits commute, so a cancelling or mergeable partner can be
  up to 16 ops away as long as everything in between shares no qubit with
  the candidate. A barrier anywhere in that window blocks the search
  outright — barriers are optimization fences, full stop, regardless of
  which qubits they name.

Run the CLI's demo circuit and you can watch all three interact:

```sh
$ echo 'OPENQASM 2.0; include "qelib1.inc"; qreg q[1];
h q[0]; h q[0]; x q[0]; rz(0.5) q[0]; rz(0.5) q[0];' | go run ./cmd/qasmopt -stats -
before:
  h        2
  rz       2
  x        1
  total    5
after:
  rz       1
  x        1
  total    2
OPENQASM 2.0;
include "qelib1.inc";
qreg q[1];
x q[0];
rz(1) q[0];
```

`h h` cancels on the first pass, `rz(.5) rz(.5)` merges into `rz(1)` once
they become adjacent, and five gates become two — five characters of
input semantics preserved by four lines of pass logic.

Every pass follows one rule religiously: when a rewrite's safety isn't
locally obvious, decline. `CancelAcrossWindow` never reorders ops, only
deletes or merges in place, so declining to act is always safe — it just
means a shorter circuit was left on the table, never a wrong one.

## Proving it, not just hoping

The real trust question for any circuit optimizer is: how do you know it
didn't change what the circuit computes? qasmopt answers this with a
tiny vendored state-vector simulator (`sim/sim.go`) that exists for
exactly one purpose — checking optimizer output against the original,
never for general simulation. `TestOptimizeEquivalence` (in
`opt/equiv_test.go`) generates randomized circuits seeded for
reproducibility, deliberately packed with adjacent and near-adjacent
inverse pairs, repeated rotations, and occasional barriers so the
optimizer has real work to do; runs both the original and optimized
op lists from the same random initial state; and checks that the final
state vectors match up to a global phase — the strongest equivalence
check available, since global phase is physically unobservable and
rotations dropped for summing to a multiple of 2π are only equivalent up
to exactly that phase.

```go
if !sim.EqualUpToGlobalPhase(a, b, tightEps) {
    t.Fatalf("trial %d: optimization changed circuit semantics ...", trial)
}
```

Alongside the randomized equivalence test, golden-file tests
(`opt/golden_test.go`) pin down exact before/after QASM for a fixed set
of `testdata/*.qasm` inputs, so a regression in the *shape* of the
output — not just its semantics — gets caught too.

## What's next

The `opt` package deliberately declines to touch `u2`/`u3` gates (the
general single-qubit rotation forms) — they're neither in the
cancellation table nor the rotation-merge set, so they pass through
untouched, which the equivalence test exercises directly as a "decline"
path. Extending the pass set to recognize when a `u3` decomposes into
something cancellable, or adding a second commutation rule for
two-qubit gates, are the natural next steps — each one bounded by the
same rule that's kept this optimizer honest so far: when in doubt, leave
it alone.
