// Package opt implements peephole optimization passes over qasmopt's
// flat IR. Every pass is a pure func([]ir.Op) []ir.Op that never mutates
// its input, and every pass is semantics-preserving: when a rewrite's
// safety is not locally obvious, the pass declines to transform.
//
// Barriers are optimization fences: no pass cancels, merges, or matches
// ops across a barrier, regardless of which qubits the barrier names.
package opt

import (
	"math"

	"github.com/Pisush/qasmopt/ir"
)

// eps is the tolerance for treating a rotation angle as zero (mod 2*pi).
// Angles are never compared with ==.
const eps = 1e-9

// windowSize bounds how far CancelAcrossWindow looks ahead past
// commuting (qubit-disjoint) ops for a matching partner.
const windowSize = 16

// A Pass rewrites an op list into an equivalent, hopefully shorter one.
// Passes must be pure: no mutation of the input slice or its ops.
type Pass func([]ir.Op) []ir.Op

// Passes returns the standard pass pipeline in application order.
func Passes() []Pass {
	return []Pass{CancelInverses, MergeRotations, CancelAcrossWindow}
}

// Optimize runs the standard passes repeatedly until a full round leaves
// the op list unchanged (a fixpoint). The input slice is not modified.
func Optimize(ops []ir.Op) []ir.Op {
	passes := Passes()
	// Each effective iteration removes at least one op (rotation merges
	// that change nothing else still shrink the list by one), so the
	// loop terminates after at most len(ops) rounds; the bound below is
	// pure belt-and-braces against a future buggy pass.
	for round := 0; round <= len(ops)+1; round++ {
		before := ops
		for _, pass := range passes {
			ops = pass(ops)
		}
		if opsEqual(before, ops) {
			return ops
		}
	}
	return ops
}

// inverseOf returns the gate that cancels name when applied immediately
// after it on the same qubits, and whether such a gate exists in the
// supported set.
func inverseOf(name string) (string, bool) {
	switch name {
	case "h", "x", "y", "z", "cx":
		return name, true // self-inverse
	case "s":
		return "sdg", true
	case "sdg":
		return "s", true
	case "t":
		return "tdg", true
	case "tdg":
		return "t", true
	}
	return "", false
}

// isRotation reports whether name is a mergeable rotation gate. rx, ry,
// rz merge by angle addition; u1 (a pure phase) does too.
func isRotation(name string) bool {
	switch name {
	case "rx", "ry", "rz", "u1":
		return true
	}
	return false
}

// cancels reports whether b undoes a: b is a's inverse on the identical
// qubit list (order matters — cx control/target must match exactly).
func cancels(a, b ir.Op) bool {
	inv, ok := inverseOf(a.Name)
	if !ok || b.Name != inv {
		return false
	}
	return sameQubits(a.Qubits, b.Qubits)
}

// sameQubits reports whether two qubit lists are identical including
// order.
func sameQubits(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sharesQubit reports whether the two ops touch at least one common
// qubit. A barrier "touches" every qubit for fencing purposes; callers
// handle barriers before calling this.
func sharesQubit(a, b ir.Op) bool {
	for _, qa := range a.Qubits {
		for _, qb := range b.Qubits {
			if qa == qb {
				return true
			}
		}
	}
	return false
}

// isZeroMod2Pi reports whether the angle is congruent to 0 (mod 2*pi)
// within eps.
func isZeroMod2Pi(theta float64) bool {
	r := math.Mod(theta, 2*math.Pi)
	return math.Abs(r) < eps || math.Abs(r-2*math.Pi) < eps || math.Abs(r+2*math.Pi) < eps
}

// CancelInverses removes adjacent inverse pairs: h h, x x, y y, z z,
// cx cx (identical control and target), s sdg, sdg s, t tdg, tdg t.
// "Adjacent" is literal: the ops must be consecutive in the list, so no
// op — and in particular no barrier — can intervene. Removal cascades:
// in x h h x, deleting the inner pair makes the outer pair adjacent and
// it is removed in the same sweep.
func CancelInverses(ops []ir.Op) []ir.Op {
	out := make([]ir.Op, 0, len(ops))
	for _, op := range ops {
		if n := len(out); n > 0 && cancels(out[n-1], op) {
			out = out[:n-1]
			continue
		}
		out = append(out, op)
	}
	return out
}

// MergeRotations fuses adjacent rotations of the same kind on the same
// qubit: rx(a) rx(b) -> rx(a+b), likewise ry, rz, and u1. If the summed
// angle is congruent to 0 (mod 2*pi) within eps the op is dropped
// entirely. For rx/ry/rz a 2*pi rotation is -identity, so dropping it
// preserves the state only up to an unobservable global phase (u1 is
// exactly periodic). Merging cascades the same way CancelInverses does.
func MergeRotations(ops []ir.Op) []ir.Op {
	out := make([]ir.Op, 0, len(ops))
	for _, op := range ops {
		n := len(out)
		if n > 0 && isRotation(op.Name) && out[n-1].Name == op.Name &&
			sameQubits(out[n-1].Qubits, op.Qubits) {
			sum := out[n-1].Params[0] + op.Params[0]
			if isZeroMod2Pi(sum) {
				out = out[:n-1]
				continue
			}
			merged := out[n-1]
			merged.Params = []float64{sum}
			out[n-1] = merged
			continue
		}
		out = append(out, op)
	}
	return out
}

// CancelAcrossWindow is the commutation-aware variant of CancelInverses
// and MergeRotations: ops acting on disjoint qubit sets commute, so a
// cancelling or mergeable partner may sit up to windowSize ops ahead as
// long as every op in between is qubit-disjoint from the candidate. Any
// barrier inside the window blocks the search (optimization fence), as
// does the first intervening op that touches one of the candidate's
// qubits without being its partner. This is a sliding-window search, not
// a DAG scheduler; ops are never reordered, only deleted or merged in
// place, so declining is always safe.
func CancelAcrossWindow(ops []ir.Op) []ir.Op {
	// Work on a copy: merges rewrite Params, and the input must stay
	// untouched.
	work := make([]ir.Op, len(ops))
	copy(work, ops)
	removed := make([]bool, len(work))

	for i := 0; i < len(work); i++ {
		if removed[i] || work[i].Name == ir.OpBarrier || work[i].Name == ir.OpMeasure {
			continue
		}
		_, invertible := inverseOf(work[i].Name)
		rotation := isRotation(work[i].Name)
		if !invertible && !rotation {
			continue
		}
		seen := 0
		for j := i + 1; j < len(work) && seen < windowSize; j++ {
			if removed[j] {
				continue
			}
			seen++
			if work[j].Name == ir.OpBarrier {
				break // fence: never optimize across a barrier
			}
			if !sharesQubit(work[i], work[j]) {
				continue // disjoint qubits commute; keep scanning
			}
			// First op sharing a qubit: either it is the partner, or
			// the search for work[i] is over.
			if invertible && cancels(work[i], work[j]) {
				removed[i], removed[j] = true, true
			} else if rotation && work[j].Name == work[i].Name &&
				sameQubits(work[i].Qubits, work[j].Qubits) {
				sum := work[i].Params[0] + work[j].Params[0]
				removed[j] = true
				if isZeroMod2Pi(sum) {
					removed[i] = true
				} else {
					merged := work[i]
					merged.Params = []float64{sum}
					work[i] = merged
				}
			}
			break
		}
	}

	out := make([]ir.Op, 0, len(work))
	for i, op := range work {
		if !removed[i] {
			out = append(out, op)
		}
	}
	return out
}

// opsEqual reports whether two op lists are identical (names, qubits,
// cbits, and bit-for-bit identical params).
func opsEqual(a, b []ir.Op) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			!sameQubits(a[i].Qubits, b[i].Qubits) ||
			!sameQubits(a[i].Cbits, b[i].Cbits) {
			return false
		}
		if len(a[i].Params) != len(b[i].Params) {
			return false
		}
		for k := range a[i].Params {
			if a[i].Params[k] != b[i].Params[k] {
				return false
			}
		}
	}
	return true
}
