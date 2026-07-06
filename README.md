# qasmopt

An OpenQASM 2.0 parser and peephole circuit optimizer written in pure Go —
zero third-party dependencies.

Status: M1–M3 complete. See the roadmap below.

## Roadmap

- [x] M1 — Hand-written lexer (line/column positions) + recursive-descent parser → AST, with a constant-folding expression evaluator for gate parameters.
- [x] M2 — Flat IR (`[]Op`) with register flattening, plus a `cmd/qasmopt` CLI that round-trips QASM and prints gate-count stats.
- [x] M3 — Semantics-preserving optimizer passes (inverse cancellation, rotation merging, commutation-aware window) run to fixpoint, with golden-file tests. The CLI optimizes by default (`-no-opt` to disable); `-stats` prints before/after gate counts.
- [x] Stretch — Equivalence testing against a tiny vendored state-vector simulator (`sim/`): original vs optimized circuits match up to global phase.
