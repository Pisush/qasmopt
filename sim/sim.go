// Package sim is a deliberately tiny state-vector simulator, vendored
// inside qasmopt for one purpose: checking that optimized circuits are
// equivalent to their originals (up to global phase) in tests. It is not
// a general-purpose simulator — it supports exactly the gate set of the
// qasmopt IR, ignores barriers, and does not implement measurement.
package sim

import (
	"fmt"
	"math"
	"math/cmplx"

	"github.com/Pisush/qasmopt/ir"
)

// maxQubits caps state allocation; equivalence tests only ever need a
// handful of qubits.
const maxQubits = 16

// State is the amplitude vector of an n-qubit register, length 2^n,
// indexed so that qubit q corresponds to bit q of the index
// (little-endian).
type State []complex128

// NewState returns the all-zeros basis state |0...0> on n qubits. It
// panics if n is out of range — a programmer error in tests.
func NewState(n int) State {
	if n < 1 || n > maxQubits {
		panic(fmt.Sprintf("sim: qubit count %d out of range [1,%d]", n, maxQubits))
	}
	s := make(State, 1<<n)
	s[0] = 1
	return s
}

// Clone returns an independent copy of the state.
func (s State) Clone() State {
	out := make(State, len(s))
	copy(out, s)
	return out
}

// apply1 applies the 2x2 matrix m to qubit q by mixing amplitude pairs
// whose indices differ only in bit q.
func (s State) apply1(m [2][2]complex128, q int) {
	bit := 1 << q
	for i := range s {
		if i&bit != 0 {
			continue
		}
		a, b := s[i], s[i|bit]
		s[i] = m[0][0]*a + m[0][1]*b
		s[i|bit] = m[1][0]*a + m[1][1]*b
	}
}

// applyCX applies a controlled-X with the given control and target.
func (s State) applyCX(control, target int) {
	cbit, tbit := 1<<control, 1<<target
	for i := range s {
		// Visit each swapped pair once, from its target=0 member.
		if i&cbit != 0 && i&tbit == 0 {
			s[i], s[i|tbit] = s[i|tbit], s[i]
		}
	}
}

// gateMatrix returns the 2x2 unitary for a single-qubit IR op.
func gateMatrix(name string, params []float64) ([2][2]complex128, error) {
	i := complex(0, 1)
	invSqrt2 := complex(1/math.Sqrt2, 0)
	switch name {
	case "x":
		return [2][2]complex128{{0, 1}, {1, 0}}, nil
	case "y":
		return [2][2]complex128{{0, -i}, {i, 0}}, nil
	case "z":
		return [2][2]complex128{{1, 0}, {0, -1}}, nil
	case "h":
		return [2][2]complex128{{invSqrt2, invSqrt2}, {invSqrt2, -invSqrt2}}, nil
	case "s":
		return [2][2]complex128{{1, 0}, {0, i}}, nil
	case "sdg":
		return [2][2]complex128{{1, 0}, {0, -i}}, nil
	case "t":
		return [2][2]complex128{{1, 0}, {0, cmplx.Exp(i * math.Pi / 4)}}, nil
	case "tdg":
		return [2][2]complex128{{1, 0}, {0, cmplx.Exp(-i * math.Pi / 4)}}, nil
	case "rx":
		c, s := complex(math.Cos(params[0]/2), 0), complex(math.Sin(params[0]/2), 0)
		return [2][2]complex128{{c, -i * s}, {-i * s, c}}, nil
	case "ry":
		c, s := complex(math.Cos(params[0]/2), 0), complex(math.Sin(params[0]/2), 0)
		return [2][2]complex128{{c, -s}, {s, c}}, nil
	case "rz":
		e := cmplx.Exp(i * complex(params[0]/2, 0))
		return [2][2]complex128{{1 / e, 0}, {0, e}}, nil
	case "u1":
		return [2][2]complex128{{1, 0}, {0, cmplx.Exp(i * complex(params[0], 0))}}, nil
	case "u2":
		phi, lam := params[0], params[1]
		return [2][2]complex128{
			{invSqrt2, -invSqrt2 * cmplx.Exp(i*complex(lam, 0))},
			{invSqrt2 * cmplx.Exp(i*complex(phi, 0)), invSqrt2 * cmplx.Exp(i*complex(phi+lam, 0))},
		}, nil
	case "u3":
		theta, phi, lam := params[0], params[1], params[2]
		c := complex(math.Cos(theta/2), 0)
		s := complex(math.Sin(theta/2), 0)
		return [2][2]complex128{
			{c, -s * cmplx.Exp(i*complex(lam, 0))},
			{s * cmplx.Exp(i*complex(phi, 0)), c * cmplx.Exp(i*complex(phi+lam, 0))},
		}, nil
	}
	return [2][2]complex128{}, fmt.Errorf("sim: unsupported gate %q", name)
}

// RunOps applies a flattened op sequence to the state. Barriers are
// no-ops. Measurement is not supported and returns an error: the
// simulator exists to compare pure states.
func (s State) RunOps(ops []ir.Op) error {
	for _, op := range ops {
		switch op.Name {
		case ir.OpBarrier:
			// no-op
		case ir.OpMeasure:
			return fmt.Errorf("sim: measurement is not supported")
		case "cx":
			s.applyCX(op.Qubits[0], op.Qubits[1])
		default:
			m, err := gateMatrix(op.Name, op.Params)
			if err != nil {
				return err
			}
			s.apply1(m, op.Qubits[0])
		}
	}
	return nil
}

// EqualUpToGlobalPhase reports whether a and b describe the same physical
// state: b == e^{i*phi} * a for some real phi, within eps per amplitude.
func EqualUpToGlobalPhase(a, b State, eps float64) bool {
	if len(a) != len(b) {
		return false
	}
	// Find the largest amplitude of a to anchor the phase estimate.
	ref, best := -1, eps
	for i := range a {
		if m := cmplx.Abs(a[i]); m > best {
			ref, best = i, m
		}
	}
	if ref < 0 {
		// a is (numerically) the zero vector; require b to be too.
		for i := range b {
			if cmplx.Abs(b[i]) > eps {
				return false
			}
		}
		return true
	}
	phase := b[ref] / a[ref]
	if math.Abs(cmplx.Abs(phase)-1) > eps {
		return false
	}
	for i := range a {
		if cmplx.Abs(b[i]-phase*a[i]) > eps {
			return false
		}
	}
	return true
}
