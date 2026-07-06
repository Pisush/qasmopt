DRAFT — podcast script for qasmopt (github.com/Pisush/qasmopt)

# Episode: qasmopt — a compiler for quantum circuits, proven correct

**HOST A** — curious generalist
**HOST B** — the builder, wrote qasmopt

---

**HOST A:** So today we're talking about a project called qasmopt. And
before we even get into what it does, I have to ask — why does a
quantum computing project sound like it's going to be a compilers
episode?

**HOST B:** Because it is one. That's kind of the whole pitch. If you
strip away the physics, a quantum circuit is just a program — a small
set of typed registers and a sequence of operations over them. OpenQASM
2.0, which is the input format qasmopt works with, looks almost exactly
like a tiny imperative language. You declare a `qreg` — a quantum
register — and a `creg` for classical bits, and then you write a flat
list of gate applications, like `h q[0];` or `cx q[0], q[1];`.

**HOST A:** So it really is just declarations plus a straight-line
instruction stream.

**HOST B:** Exactly. And once you see it that way, the classic compiler
pipeline just falls out: lex the source into tokens, parse the tokens
into an AST with recursive descent, lower the AST into a flat
intermediate representation, and then run optimization passes over that
IR until you hit a fixpoint. qasmopt does literally that, in four Go
packages with zero third-party dependencies — `lexer`, `parser`, `ir`,
and `opt`.

**HOST A:** Zero dependencies is a flex.

**HOST B:** It's also just honest about what the problem needs. The
lexer is maybe 200 lines — it's rune-based, hand-written, tracks
1-based line and column as it advances so every token can carry a
position for error messages. It never fails outright, actually — if it
hits something it doesn't recognize, it emits a `token.ILLEGAL` token
instead of erroring immediately, and lets the parser be the one place
that reports errors.

**HOST A:** Why push that decision to the parser instead of just
failing at the lexer?

**HOST B:** Because the parser already has to own error reporting for
everything else — unexpected tokens, wrong arity, all of that. If the
lexer also independently decided how to report bad input, you'd have
two places doing error UX instead of one. Keeping it in one place made
the whole thing simpler.

**HOST A:** Okay, and then parsing?

**HOST B:** Recursive descent, one function per grammar rule — pretty
textbook. The one genuinely fun bit is the expression evaluator for
gate parameters. Something like `rz(pi/2)` or `u3(pi/4, -pi/2, pi)` —
those parenthesized values are constant expressions in OpenQASM 2.0,
not runtime values, so qasmopt just evaluates them to a `float64` right
there during parsing. Four mutually recursive functions: `expr` handles
plus and minus, `term` handles times and divide, `unary` handles a
leading minus, and `primary` handles a literal, `pi`, or a parenthesized
sub-expression. By the time parsing is done, there's no symbolic
representation of a parameter anywhere — it's just a float.

**HOST A:** Does the parser support the full OpenQASM 2.0 language, or
did you scope it down?

**HOST B:** Scoped down, deliberately. Custom gate definitions, `if`
statements, `opaque` declarations, `reset` — those are all real parts of
the spec, and qasmopt doesn't support any of them in this version. But
here's the detail I actually care about: the lexer still recognizes
those keywords, so when the parser hits one it can say "unsupported
construct: gate definitions are not supported in v1" instead of some
generic syntax error two tokens later that leaves you guessing. Knowing
what you don't support, and saying so immediately and specifically, is
underrated parser design. Same instinct shows up in how gates get
validated — every standard gate has a known arity, so `rz` with zero
parameters or `cx` with three qubit arguments fails immediately with a
message naming exactly what was expected.

**HOST A:** And then the IR.

**HOST B:** Right, this is the part I actually think is the cleanest
decision in the whole codebase. `ir.Lower` takes the AST and flattens
every register reference to a global integer index. So `q[0]`, `q[1]`,
and a second register `r[0]` become plain integers — 0, 1, 2. Every op
after that point is just a `Name` string, a slice of qubit indices, a
slice of params, and classical bit indices for measurement. Barriers
and measurements are ops too, with their own names — they're not some
separate tree node type. So the entire program becomes one flat `[]Op`
slice, and every later pass only has to reason about one data
structure.

**HOST A:** Does lowering do anything besides renumbering, or is it
purely mechanical?

**HOST B:** A bit more than renumbering. Broadcast forms get expanded
there too — if you write `h q;` over a whole register instead of
indexing a specific qubit, lowering expands that into one `h` op per
qubit in the register. And it's where a handful of static checks live:
undeclared registers, an index past the end of a register, using a
`creg` somewhere a `qreg` is required, that kind of thing. All of that
gets caught once, at lowering time, with a source position attached —
so by the time an `[]Op` slice exists, the optimizer never has to worry
about any of it. It only ever sees valid ops on valid qubit indices.

**HOST A:** Let's get to the actual optimizing. What does "optimize" even
mean for a quantum circuit — you're not folding constants, there's no
dead code in the traditional sense.

**HOST B:** It's peephole optimization, same idea as in a classical
compiler, just with quantum-specific rewrite rules. And here's the
constraint that matters more than any individual rule: every pass has to
be semantics-preserving, and — this is the important part — when a pass
isn't sure a rewrite is safe, it has to decline. Not "probably fine,"
not "usually correct." Decline outright. Because these are real quantum
states — if you silently change what the circuit computes, you don't
get a crash, you get a circuit that quietly returns the wrong physics,
which is so much worse than an error.

**HOST A:** So how do you actually know a pass got that right? Like,
how do you build confidence that "decline when unsure" was applied
correctly everywhere?

**HOST B:** This is my favorite part of the whole project, honestly.
There's a tiny vendored state-vector simulator in the `sim` package,
and it exists for exactly one job: not general simulation, just
checking that an optimized circuit is equivalent to the original. There's
a test that generates randomized circuits — seeded, so it's
reproducible — deliberately stuffed with adjacent inverse pairs and
repeated rotations so the optimizer has real work to do. It runs both
the original and the optimized op list from the same random initial
state through the simulator, and then checks that the two resulting
state vectors are equal up to global phase.

**HOST A:** Up to global phase — meaning?

**HOST B:** Meaning the two state vectors can differ by an overall
complex phase factor, `e^{i·phi}`, and still be physically the same
state — global phase isn't observable in quantum mechanics, so it's not
a real difference. That's actually the exact right equivalence notion
here too, not just a technicality, because one of the rewrite rules
literally produces states that differ by global phase on purpose.

**HOST A:** Which rule does that?

**HOST B:** Rotation merging. So — three passes total. First,
`CancelInverses`: adjacent inverse pairs cancel. `h h` cancels because
Hadamard is self-inverse. Same for `x x`, `y y`, `z z`, and `cx cx` with
matching control and target. `s` and `sdg` cancel each other, `t` and
`tdg` cancel each other. And it cascades — if you have `x h h x`,
removing the inner `h h` makes the outer `x x` adjacent, and that gets
swept away in the same pass.

**HOST A:** And the second pass is the rotation merging you mentioned.

**HOST B:** Right — `MergeRotations`. Adjacent rotations of the same
kind on the same qubit fuse by adding their angles: `rz(a)` followed by
`rz(b)` becomes `rz(a+b)`. Same for `rx`, `ry`, and `u1`. And if the
summed angle comes out congruent to zero mod 2π — checked with an
epsilon, never exact floating-point equality — the op gets dropped
entirely. And that's exactly the case where global-phase equivalence
matters: a full 2π rotation on `rx`, `ry`, or `rz` is actually
negative-identity, not identity, so dropping it is only correct up to
that unobservable global phase. `u1` is different — it's a pure phase
gate, so a full 2π rotation there really is the identity, no phase
subtlety at all.

**HOST A:** And the third pass?

**HOST B:** `CancelAcrossWindow` — the commutation-aware one. The first
two passes only catch pairs that are literally adjacent in the op list.
But if two gates act on completely disjoint qubits, they commute — order
between them doesn't matter physically — so a cancelling or mergeable
partner might be sitting a few gates further down the list, separated
by ops that don't touch the same qubits. This pass slides a window
forward, up to sixteen ops, and as long as everything it steps over is
qubit-disjoint from the candidate gate, it can still find and apply the
cancellation or merge. The moment it hits an op that shares a qubit and
isn't the partner — or hits a barrier — it stops looking. Barriers are
a hard fence in all three passes, by the way. Nothing cancels or merges
across a barrier, full stop, regardless of which qubits it names.

**HOST A:** And all three passes just run in a loop?

**HOST B:** Repeatedly, in that order, until one full round leaves the
op list completely unchanged — that's the fixpoint.

**HOST A:** Should we just run it?

**HOST B:** Yeah, let's do it live. Let's take five gates: `h q[0]; h
q[0]; x q[0]; rz(0.5) q[0]; rz(0.5) q[0];` on a single qubit.

**HOST A:** Walk me through what should happen before you run it.

**HOST B:** So — `h h` are adjacent, Hadamard's self-inverse, that pair
cancels. Then `rz(.5) rz(.5)` are adjacent same-qubit rotations, so they
merge into `rz(1)`. The `x` in between doesn't interact with either
pair, it's just along for the ride. Let's actually run the CLI with
`-stats` so we get before-and-after gate counts.

**HOST A:** Okay, running it.

**HOST B:** `qasmopt -stats -`, piping that circuit in. And — there it
is. Before: `h` two, `rz` two, `x` one, total five. After: `rz` one,
`x` one, total two. And the actual output circuit is `x q[0]; rz(1)
q[0];` — which is exactly what we predicted by hand a second ago.

**HOST A:** Five gates down to two, and you didn't have to trust that
by eyeballing it — the simulator backs it up separately.

**HOST B:** Right, that's the whole point. The gate-count drop is the
satisfying part you can see, but the equivalence test running
underneath is what makes it trustworthy rather than just a fun demo.

**HOST A:** That feels like a good place to land it. Where should
people look if they want to dig into the actual code?

**HOST B:** `opt/opt.go` for the three passes, `sim/sim.go` for the
equivalence simulator, and there are golden-file tests under
`opt/testdata` that pin down exact before/after QASM for a handful of
fixed circuits, on top of the randomized equivalence testing. Between
the two, you get both kinds of regression coverage: golden files catch
a change in the exact *shape* of the output, and the randomized
simulator test catches a change in what the circuit actually computes,
which golden files alone can't see.

**HOST A:** Is there anything the optimizer deliberately doesn't touch
yet?

**HOST B:** Yeah — `u2` and `u3`, the general single-qubit rotation
gates. They're not in the cancellation table and they're not in the
rotation-merge set, so right now they just pass straight through
untouched. That's not an oversight so much as the same "decline when
unsure" rule playing out at the level of what gates get rules at all —
nobody's worked out a safe rewrite for them yet, so nothing happens to
them, which is exactly the correct behavior until someone does. Same
story for two-qubit commutation — right now the window pass only
reasons about qubit-disjointness to let things commute, it doesn't have
a rule for, say, reordering two `cx` gates that share a qubit in a
provably-safe way. Both are natural next steps, and both are bounded by
that same rule.

**HOST A:** Great place to leave it. Thanks for walking through this.

**HOST B:** Thanks for having me.

---
