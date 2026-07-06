package sim

import (
	"math"
	"math/cmplx"
	"testing"

	"github.com/Pisush/qasmopt/ir"
)

const eps = 1e-9

func op(name string, qubits ...int) ir.Op { return ir.Op{Name: name, Qubits: qubits} }

func rot(name string, theta float64, q int) ir.Op {
	return ir.Op{Name: name, Qubits: []int{q}, Params: []float64{theta}}
}

func run(t *testing.T, n int, ops []ir.Op) State {
	t.Helper()
	s := NewState(n)
	if err := s.RunOps(ops); err != nil {
		t.Fatalf("RunOps: %v", err)
	}
	return s
}

func TestHTwiceIsIdentity(t *testing.T) {
	got := run(t, 1, []ir.Op{op("h", 0), op("h", 0)})
	want := NewState(1)
	if !EqualUpToGlobalPhase(got, want, eps) {
		t.Errorf("HH|0> = %v, want |0>", got)
	}
}

func TestHGivesEqualSuperposition(t *testing.T) {
	got := run(t, 1, []ir.Op{op("h", 0)})
	for i, amp := range got {
		if math.Abs(cmplx.Abs(amp)-1/math.Sqrt2) > eps {
			t.Errorf("amplitude %d = %v, want magnitude 1/sqrt2", i, amp)
		}
	}
}

func TestBellState(t *testing.T) {
	got := run(t, 2, []ir.Op{op("h", 0), op("cx", 0, 1)})
	invSqrt2 := complex(1/math.Sqrt2, 0)
	want := State{invSqrt2, 0, 0, invSqrt2} // (|00> + |11>)/sqrt2
	if !EqualUpToGlobalPhase(got, want, eps) {
		t.Errorf("Bell state = %v, want %v", got, want)
	}
}

func TestCXOnUpperQubit(t *testing.T) {
	// X on qubit 1, then cx with control 1, target 0: |10> -> |11>.
	got := run(t, 2, []ir.Op{op("x", 1), op("cx", 1, 0)})
	want := State{0, 0, 0, 1}
	if !EqualUpToGlobalPhase(got, want, eps) {
		t.Errorf("state = %v, want |11>", got)
	}
}

func TestRz2PiIsGlobalPhase(t *testing.T) {
	plus := run(t, 1, []ir.Op{op("h", 0)})
	rotated := plus.Clone()
	if err := rotated.RunOps([]ir.Op{rot("rz", 2*math.Pi, 0)}); err != nil {
		t.Fatalf("RunOps: %v", err)
	}
	// rz(2pi) = -I: equal only up to global phase.
	if !EqualUpToGlobalPhase(plus, rotated, eps) {
		t.Errorf("rz(2pi) changed the physical state: %v vs %v", plus, rotated)
	}
	if cmplx.Abs(rotated[0]-plus[0]) < eps {
		t.Errorf("rz(2pi) should flip the sign of amplitudes, got %v", rotated)
	}
}

func TestGateAlgebra(t *testing.T) {
	tests := []struct {
		name string
		a, b []ir.Op // two op sequences that must agree up to global phase
	}{
		{"s sdg = I", []ir.Op{op("s", 0), op("sdg", 0)}, nil},
		{"t t = s", []ir.Op{op("t", 0), op("t", 0)}, []ir.Op{op("s", 0)}},
		{"x = h z h", []ir.Op{op("x", 0)}, []ir.Op{op("h", 0), op("z", 0), op("h", 0)}},
		{"u1(pi) = z", []ir.Op{rot("u1", math.Pi, 0)}, []ir.Op{op("z", 0)}},
		{"rz(pi) ~ z", []ir.Op{rot("rz", math.Pi, 0)}, []ir.Op{op("z", 0)}},
		{"rx(pi) ~ x", []ir.Op{rot("rx", math.Pi, 0)}, []ir.Op{op("x", 0)}},
		{"ry(pi) ~ y", []ir.Op{rot("ry", math.Pi, 0)}, []ir.Op{op("y", 0)}},
		{
			"u3(pi/2,0,pi) = h",
			[]ir.Op{{Name: "u3", Qubits: []int{0}, Params: []float64{math.Pi / 2, 0, math.Pi}}},
			[]ir.Op{op("h", 0)},
		},
		{
			"u2(0,pi) = h",
			[]ir.Op{{Name: "u2", Qubits: []int{0}, Params: []float64{0, math.Pi}}},
			[]ir.Op{op("h", 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start from a superposition so phases on |1> matter.
			pre := []ir.Op{op("h", 0)}
			sa := run(t, 1, append(append([]ir.Op{}, pre...), tt.a...))
			sb := run(t, 1, append(append([]ir.Op{}, pre...), tt.b...))
			if !EqualUpToGlobalPhase(sa, sb, eps) {
				t.Errorf("states differ: %v vs %v", sa, sb)
			}
		})
	}
}

func TestBarrierIsNoOpAndMeasureRejected(t *testing.T) {
	s := run(t, 2, []ir.Op{op("h", 0), {Name: ir.OpBarrier, Qubits: []int{0, 1}}})
	want := run(t, 2, []ir.Op{op("h", 0)})
	if !EqualUpToGlobalPhase(s, want, eps) {
		t.Errorf("barrier changed the state")
	}
	err := s.RunOps([]ir.Op{{Name: ir.OpMeasure, Qubits: []int{0}, Cbits: []int{0}}})
	if err == nil {
		t.Error("RunOps(measure) succeeded, want error")
	}
}

func TestEqualUpToGlobalPhaseRejectsDifferentStates(t *testing.T) {
	a := run(t, 1, []ir.Op{op("h", 0)})
	b := run(t, 1, []ir.Op{op("x", 0)})
	if EqualUpToGlobalPhase(a, b, eps) {
		t.Error("distinct states reported equal")
	}
}
