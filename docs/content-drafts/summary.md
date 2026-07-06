DRAFT

# qasmopt: a compiler pipeline for quantum circuits, proven correct by simulation

qasmopt is an OpenQASM 2.0 parser and peephole optimizer written in pure
Go, with zero third-party dependencies. It takes a `.qasm` file — a
sequence of quantum gate operations over declared qubit registers — and
runs it through a pipeline that will look immediately familiar to
anyone who's built a compiler: a hand-written lexer, a recursive-descent
parser, a flat intermediate representation, and a set of optimization
passes run to a fixpoint. Feed it a circuit, and it hands back an
equivalent circuit with fewer gates.

The one clever idea here isn't any single pass — it's applying the
*whole shape* of a classic compiler to a domain that doesn't usually get
one. Quantum circuits, once you strip away the physics, are just
programs: `qreg`/`creg` declarations are variable declarations, and gate
applications are a flat instruction stream. So qasmopt treats them
exactly that way. `lexer/` turns source text into position-tracked
tokens. `parser/` builds an AST with a small four-function expression
evaluator (`expr → term → unary → primary`) that resolves gate
parameters like `rz(pi/2)` to `float64` at parse time. `ir/` flattens
register-relative references (`q[0]`, `q[1]`, `r[0]`) into a single flat
`[]Op` list with global qubit indices. And `opt/` runs three peephole
passes — adjacent-inverse cancellation, rotation merging, and a
commutation-aware sliding window — repeatedly until a full round leaves
the circuit unchanged.

What makes it more than a toy is the second half of the clever idea:
correctness isn't just asserted, it's checked. Every optimization pass
follows one rule — when a rewrite's safety isn't locally obvious,
decline rather than risk it — and the project backs that rule with a
tiny vendored state-vector simulator (`sim/`) whose only job is
comparing a circuit against its optimized self. A randomized test
generates circuits packed with cancellable and mergeable gates, runs
both versions from the same initial state, and checks the resulting
state vectors match up to global phase — the correct notion of
equivalence for quantum states, since global phase is physically
unobservable. That's a genuinely satisfying thing to see work: run
`h q[0]; h q[0]; x q[0]; rz(.5) q[0]; rz(.5) q[0];` through the CLI with
`-stats` and watch 5 gates become 2 (`x q[0]; rz(1) q[0];`), then watch
the simulator confirm nothing about what the circuit *does* changed
along the way.

It's a small project — no third-party dependencies, a deliberately
narrow OpenQASM 2.0 subset, three passes — but it's a complete,
correctness-checked instance of a real compiler pipeline, applied
somewhere compiler pipelines don't usually show up.
