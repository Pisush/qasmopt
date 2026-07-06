---
marp: true
theme: default
paginate: true
---

# qasmopt

A hand-rolled compiler pipeline for OpenQASM 2.0 circuits — in pure Go,
zero third-party dependencies.

<!-- notes: Opening slide. Introduce yourself and the one-line pitch: this is a real compiler pipeline (lexer, parser, IR, optimizer) built for quantum circuits instead of a general-purpose language. -->

---

# A quantum circuit is just a program

- `qreg` / `creg` declarations ≈ variable declarations
- A flat sequence of gate applications ≈ a flat instruction stream
- `h q[0];` `cx q[0], q[1];` `rz(pi/2) q[1];` — no control flow, no functions
- Strip away the physics and the classic compiler pipeline applies
  almost unchanged

<!-- notes: The core reframe: OpenQASM 2.0's structure is declarations followed by a straight-line list of operations. That's exactly the shape a lexer/parser/IR pipeline expects. -->

---

# The core idea

Peephole optimization for quantum circuits, with a **verified-correct**
guarantee:

- Every pass is semantics-preserving
- When a rewrite's safety isn't locally obvious, the pass **declines**
- A vendored simulator checks equivalence up to global phase after
  every optimization — not just trusted, checked

<!-- notes: State the thesis before the architecture: this isn't just "fewer gates," it's fewer gates with a machine-checked correctness argument attached. -->

---

# Architecture

```
source text
   │  lexer/    (hand-written, rune-based, tracks line/col)
   ▼
tokens
   │  parser/   (recursive descent → AST, evaluates gate params)
   ▼
AST
   │  ir/       (flattens registers to global qubit indices)
   ▼
[]Op  (flat IR)
   │  opt/      (3 passes, run to fixpoint)
   ▼
optimized []Op
   │  sim/      (verify equivalence up to global phase)
   ▼
cmd/qasmopt     (CLI: parse, optimize, emit QASM, -stats)
```

<!-- notes: One slide, one picture. Every arrow is a real Go package in the repo. Point out this is the textbook pipeline, package for stage. -->

---

# The lexer

- Hand-written, rune-based — no `regexp`, no generated state machine
- Tracks 1-based line/column on every token, for positioned errors
- Never fails outright: unrecognized input becomes `token.ILLEGAL`
- The *parser* is the single place that turns that into a reported error

```go
func (l *Lexer) advance() rune {
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

<!-- notes: Real code from lexer/lexer.go. The never-fail design is worth a beat: it keeps error reporting in one place (the parser) rather than splitting the responsibility. -->

---

# Recursive descent + a 4-function evaluator

Gate parameters like `rz(pi/2)` or `u3(pi/4, -pi/2, pi)` are constant
expressions — evaluated to `float64` right in the parser:

```
expr    := term  { ("+" | "-") term }
term    := unary { ("*" | "/") unary }
unary   := "-" unary | primary
primary := INT | REAL | "pi" | "(" expr ")"
```

- Four mutually recursive functions, no symbolic parameter ever exists
- Constructs out of v1 scope (custom gates, `if`, `opaque`, `reset`) are
  rejected immediately with a clear "unsupported construct" error

<!-- notes: From parser/expr.go. Emphasize "reject clearly, immediately" as a parser design principle worth stealing for other hand-rolled parsers. -->

---

# Flattening to a flat IR

```go
type Op struct {
    Name   string
    Qubits []int
    Params []float64
    Cbits  []int // classical targets; only used by measure
}
```

- `ir.Lower` flattens register-relative refs to global indices:
  `q[0]`, `q[1]`, `r[0]` → `0`, `1`, `2`
- Barriers and measurements are ops too — one flat `[]Op`, not a tree
- Broadcast forms (`h q;` over a whole register) expand here

<!-- notes: From ir/ir.go. The key move is that every later stage — especially the optimizer — only ever deals with bare integers and one flat slice type. -->

---

# Three passes, run to a fixpoint

- **`CancelInverses`** — adjacent inverse pairs cancel: `h h`, `x x`,
  `cx cx` (same control/target), `s`/`sdg`, `t`/`tdg`. Cascades:
  `x h h x` fully collapses in one sweep.
- **`MergeRotations`** — `rz(a) rz(b)` → `rz(a+b)` (same for `rx`, `ry`,
  `u1`); dropped entirely if the sum ≡ 0 mod 2π (epsilon-checked, never
  `==`).
- **`CancelAcrossWindow`** — commutation-aware: a partner up to 16 ops
  away, across qubit-disjoint ops. Any barrier blocks the search.

<!-- notes: From opt/opt.go. All three share the same rule: never reorder, only delete or merge in place, so declining to act is always safe. -->

---

# The one rule

> When a rewrite's safety isn't locally obvious, decline.

- Barriers are a hard fence in all three passes — nothing cancels or
  merges across one, regardless of which qubits it names
- `CancelAcrossWindow` never reorders ops, only deletes/merges in place
- `u2` / `u3` (general single-qubit rotations) aren't in any rule yet —
  they pass through untouched, on purpose

<!-- notes: This is the thesis slide. A wrong optimization on a real quantum state doesn't crash — it silently returns wrong physics. That asymmetry is why "decline" beats "guess." -->

---

# Proving it: the vendored simulator

- `sim/` is a tiny state-vector simulator — built for **one job only**:
  checking optimizer output against the original
- Not general-purpose: no measurement support, ignores barriers, exists
  only to compare circuits
- `TestOptimizeEquivalence`: randomized circuits (seeded, reproducible),
  run original + optimized from the same initial state
- Compares final state vectors **up to global phase**

<!-- notes: From sim/sim.go and opt/equiv_test.go. Global phase equivalence is the physically correct notion here — an unobservable phase difference is not a real difference, and rotation-drop relies on exactly that. -->

---

# Equivalence check, in code

```go
if !sim.EqualUpToGlobalPhase(a, b, tightEps) {
    t.Fatalf("trial %d: optimization changed circuit semantics ...",
        trial)
}
```

- Golden-file tests (`opt/testdata/*.qasm` + `.golden`) pin exact
  before/after QASM text for fixed fixtures
- Two complementary checks: golden files catch a shape regression,
  the simulator catches a semantics regression

<!-- notes: From opt/equiv_test.go and opt/golden_test.go. Neither test alone is sufficient — walk through why briefly if time allows. -->

---

# Demo: the running example

```
h q[0]; h q[0]; x q[0]; rz(0.5) q[0]; rz(0.5) q[0];
```

- `h h` — adjacent, Hadamard is self-inverse → cancels
- `rz(.5) rz(.5)` — adjacent same-qubit rotations → merge to `rz(1)`
- `x q[0]` — untouched, shares no cancellable/mergeable partner

Predicted result: **5 gates → 2 gates** — `x q[0]; rz(1) q[0];`

<!-- notes: Set up the prediction before running the CLI live, so the audience can verify the before/after themselves as it prints. -->

---

# Demo: running the CLI

```sh
$ echo 'OPENQASM 2.0; include "qelib1.inc"; qreg q[1];
h q[0]; h q[0]; x q[0]; rz(0.5) q[0]; rz(0.5) q[0];' \
  | qasmopt -stats -
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

<!-- notes: Verified CLI output from cmd/qasmopt. -stats prints before/after per-gate counts to stderr; the optimized QASM prints to stdout. -no-opt disables optimization for comparison. -->

---

# The CLI

```
qasmopt [-stats] [-no-opt] <file.qasm | ->
```

- Optimizes by default; `-no-opt` disables optimization (parse + re-emit
  normalized QASM only)
- `-stats` prints before/after per-gate op counts to stderr, so stdout
  stays valid QASM
- Output QASM is normalized: fully indexed args, shortest round-tripping
  float formatting

<!-- notes: From cmd/qasmopt/main.go. Worth noting the design choice of keeping stats on stderr specifically so stdout is always pipeable QASM. -->

---

# Status

- M1 — lexer + recursive-descent parser + constant-folding expression
  evaluator
- M2 — flat IR + CLI that round-trips QASM with gate-count stats
- M3 — semantics-preserving optimizer passes to fixpoint, golden-file
  tests
- Stretch — equivalence testing against the vendored simulator

<!-- notes: Pulled directly from the README roadmap. All four are complete (checked off) as of this deck. -->

---

# What's next

- `u2` / `u3` decomposition into cancellable/mergeable forms
- A second commutation rule for two-qubit gates
- Both bounded by the same rule that's kept the optimizer honest:
  **when in doubt, leave it alone**

<!-- notes: Frame these as natural, safe extensions rather than gaps — the "decline when unsure" design is exactly what makes deferring them a non-issue. -->

---

# Thanks

qasmopt — an optimizer you can prove is correct is worth more than one
that's merely fast.

`github.com/Pisush/qasmopt`

<!-- notes: Closing slide. Leave the repo link up during Q&A. -->
