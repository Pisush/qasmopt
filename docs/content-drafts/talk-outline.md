> DRAFT — conference talk outline for qasmopt (github.com/Pisush/qasmopt)

# Talk outline: Writing a semantics-preserving quantum circuit optimizer in Go

**Format:** 25-30 min conference talk
**Audience:** Go engineers who've written or read a lexer/parser/IR
pipeline before (or want to); no quantum computing background assumed.

## CFP abstract (150-200 words)

Strip away the physics and a quantum circuit is just a program: a
sequence of typed operations over a small register. Which means the
classic compiler pipeline — lex, parse, lower to a flat IR, run peephole
passes to a fixpoint — applies almost unchanged, and building it is a
great excuse to revisit compiler fundamentals with a genuinely novel
payload.

This talk builds qasmopt, an OpenQASM 2.0 parser and optimizer in pure
Go with zero third-party dependencies, stage by stage: a hand-written
rune lexer, a recursive-descent parser with a four-function expression
evaluator for gate parameters, a flat op-list IR, and three peephole
passes (inverse cancellation, rotation merging, a commutation-aware
sliding window) that run to a fixpoint.

Then we answer the question that matters for any optimizer: how do you
know it didn't change what the program computes? We'll generate random
circuits, run them through a tiny vendored simulator before and after
optimization, and check the results match up to global phase — live, on
stage, watching gate counts drop while a state vector proves nothing
broke.

## Section breakdown with timings

**1. Cold open — a quantum circuit is just a program (3 min)**
- OpenQASM source on one slide next to a small imperative program on the
  other — same shape: declarations, then a sequence of operations.
- The pitch: every stage of a classic compiler pipeline has a quantum
  analogue, and none of it requires understanding quantum mechanics to
  build correctly.

**2. Lexing without a library (4 min)**
- Live: hand-lex `rz(pi/2) q[0];` on a slide, rune by rune, tracking
  line/col.
- Show the `advance()` function's line/col bookkeeping and the
  never-fail design: unrecognized input becomes `token.ILLEGAL` instead
  of an error, deferring error reporting to the parser.
- One beat on why: a single place (the parser) owns error messages and
  position reporting, instead of splitting that responsibility across
  two hand-written components.

**3. Recursive descent + a 4-function expression evaluator (6 min)**
- Show the grammar for gate-parameter expressions:
  `expr := term {+/-} ; term := unary {*/} ; unary := "-" unary | primary`.
- Trace `u3(pi/4, -pi/2, pi)` through the four functions live, ending in
  three `float64`s — no symbolic parameter representation ever exists in
  v1.
- Show the "reject clearly, immediately" design for out-of-scope
  constructs (custom gates, `if`, `opaque`, `reset`) — a clear
  "unsupported construct" error rather than a confusing downstream
  syntax error.

**4. Flattening to a flat op list (4 min)**
- The `ir.Op` struct on a slide: `Name`, `Qubits []int`, `Params
  []float64`, `Cbits []int`.
- Show register flattening: `q[0]`, `q[1]`, `r[0]` becoming global
  integers 0, 1, 2 — and why that one decision means every later pass
  only ever deals with bare qubit numbers.
- Note barriers and measurements are ops too, not special tree nodes —
  why that uniformity is what makes the optimizer passes simple.

**5. Three passes to a fixpoint — with a live demo (7 min — see demo
plan below)**
- `CancelInverses`: adjacent inverse pairs, cascading (`x h h x` fully
  collapses in one sweep).
- `MergeRotations`: `rz(a) rz(b) -> rz(a+b)`, dropped if ≡ 0 mod 2π
  (epsilon-checked, never `==`).
- `CancelAcrossWindow`: the commutation-aware version — a partner up to
  16 ops away across qubit-disjoint ops, blocked by any barrier.
- The one rule tying all three together: when a rewrite's safety isn't
  locally obvious, decline. Never reorder, only delete or merge in place.

**6. Proving it: equivalence testing against a vendored simulator (5 min)**
- The trust question: how do you know an optimizer pass didn't change
  semantics?
- Introduce the tiny vendored `sim` package — built for one job only,
  checking equivalence, never general simulation.
- Show `TestOptimizeEquivalence`: randomized circuits seeded for
  reproducibility, run original and optimized through the simulator from
  the same random state, compare up to global phase.
- Mention golden-file tests as the complementary check: exact
  before/after QASM text, not just semantics, pinned per fixture.

**7. Close / Q&A (1-2 min)**
- What's deliberately out of scope today (`u2`/`u3` decomposition,
  two-qubit commutation rules) and why "decline when unsure" made that a
  safe thing to defer rather than a bug to fix urgently.
- Repo link, one-line pitch: "an optimizer you can prove is correct is
  worth more than one that's merely fast."

## Live-demo plan

**Setup (before the talk):** repo built (`go build ./cmd/qasmopt`);
`examples/bell.qasm` and a hand-typed pipe-in example ready in shell
history; terminal font large; `opt/opt.go` and `opt/equiv_test.go` open
in editor tabs for the code walkthrough in section 5-6.

**Demo 1 — the optimizer collapsing a circuit live (section 5, ~3 min):**
```sh
printf 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[1];\nh q[0];\nh q[0];\nx q[0];\nrz(0.5) q[0];\nrz(0.5) q[0];\n' \
  | go run ./cmd/qasmopt -stats -
```
- Narrate the `-stats` output as it prints: 5 gates before (`h,h,x,rz,rz`)
  collapsing to 2 after (`x, rz(1)`) — point at exactly which pass fired
  where: `h h` cancels immediately, `rz(.5) rz(.5)` merges into `rz(1)`
  once adjacent.
- Re-run with `-no-opt` to show the same circuit re-emitted unoptimized,
  as a visual "before" anchor.

**Demo 2 — proving equivalence live (section 6, ~3 min):**
```sh
go test ./opt/... -run TestOptimizeEquivalence -v
```
- Let the 50 randomized trials scroll, all passing; then deliberately
  break something small in a scratch copy of `opt.go` (e.g. comment out
  the `isZeroMod2Pi` drop in `MergeRotations`) and re-run to show a
  failing trial with a concrete counterexample circuit printed — makes
  the equivalence check feel real, not just decorative.
- Revert the change before moving on.
- Fallback if live-breaking risks running long: skip the deliberate
  break, keep the passing run, and narrate what a failure would look
  like instead.

**Time budget safety valve:** if running short, drop the deliberate
breakage in Demo 2 and keep only the passing run — Demo 1 (the collapsing
gate-count example) is the concrete, memorable payoff and should never be
cut.
