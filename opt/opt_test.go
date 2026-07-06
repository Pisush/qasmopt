package opt

import (
	"math"
	"testing"

	"github.com/Pisush/qasmopt/ir"
)

func op(name string, qubits ...int) ir.Op { return ir.Op{Name: name, Qubits: qubits} }

func rot(name string, theta float64, q int) ir.Op {
	return ir.Op{Name: name, Qubits: []int{q}, Params: []float64{theta}}
}

func barrier(qubits ...int) ir.Op { return ir.Op{Name: ir.OpBarrier, Qubits: qubits} }

func measure(q, c int) ir.Op {
	return ir.Op{Name: ir.OpMeasure, Qubits: []int{q}, Cbits: []int{c}}
}

// deepCopy clones an op list including backing arrays, for purity checks.
func deepCopy(ops []ir.Op) []ir.Op {
	out := make([]ir.Op, len(ops))
	for i, o := range ops {
		out[i] = ir.Op{
			Name:   o.Name,
			Qubits: append([]int(nil), o.Qubits...),
			Params: append([]float64(nil), o.Params...),
			Cbits:  append([]int(nil), o.Cbits...),
		}
	}
	return out
}

// checkPass runs pass on in, asserts the result, and verifies the pass
// did not mutate its input.
func checkPass(t *testing.T, pass Pass, in, want []ir.Op) {
	t.Helper()
	orig := deepCopy(in)
	got := pass(in)
	if !opsEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if !opsEqual(in, orig) {
		t.Errorf("pass mutated its input: %+v (was %+v)", in, orig)
	}
}

func TestCancelInverses(t *testing.T) {
	tests := []struct {
		name     string
		in, want []ir.Op
	}{
		{"empty", nil, nil},
		{"h h", []ir.Op{op("h", 0), op("h", 0)}, nil},
		{"x x", []ir.Op{op("x", 0), op("x", 0)}, nil},
		{"y y and z z", []ir.Op{op("y", 1), op("y", 1), op("z", 2), op("z", 2)}, nil},
		{"s sdg", []ir.Op{op("s", 0), op("sdg", 0)}, nil},
		{"sdg s", []ir.Op{op("sdg", 0), op("s", 0)}, nil},
		{"t tdg", []ir.Op{op("t", 0), op("tdg", 0)}, nil},
		{"tdg t", []ir.Op{op("tdg", 0), op("t", 0)}, nil},
		{"cx cx same qubits", []ir.Op{op("cx", 0, 1), op("cx", 0, 1)}, nil},
		{
			"cx cx swapped control/target survives",
			[]ir.Op{op("cx", 0, 1), op("cx", 1, 0)},
			[]ir.Op{op("cx", 0, 1), op("cx", 1, 0)},
		},
		{
			"s s does not cancel",
			[]ir.Op{op("s", 0), op("s", 0)},
			[]ir.Op{op("s", 0), op("s", 0)},
		},
		{
			"different qubits survive",
			[]ir.Op{op("h", 0), op("h", 1)},
			[]ir.Op{op("h", 0), op("h", 1)},
		},
		{
			"intervening op on same qubit blocks",
			[]ir.Op{op("h", 0), op("z", 0), op("h", 0)},
			[]ir.Op{op("h", 0), op("z", 0), op("h", 0)},
		},
		{
			"intervening op on other qubit still blocks adjacency",
			[]ir.Op{op("h", 0), op("z", 1), op("h", 0)},
			[]ir.Op{op("h", 0), op("z", 1), op("h", 0)},
		},
		{
			"barrier blocks",
			[]ir.Op{op("h", 0), barrier(0), op("h", 0)},
			[]ir.Op{op("h", 0), barrier(0), op("h", 0)},
		},
		{
			"measure blocks",
			[]ir.Op{op("x", 0), measure(0, 0), op("x", 0)},
			[]ir.Op{op("x", 0), measure(0, 0), op("x", 0)},
		},
		{
			"cascading x h h x",
			[]ir.Op{op("x", 0), op("h", 0), op("h", 0), op("x", 0)},
			nil,
		},
		{
			"surrounding ops survive",
			[]ir.Op{op("t", 1), op("h", 0), op("h", 0), op("cx", 0, 1)},
			[]ir.Op{op("t", 1), op("cx", 0, 1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkPass(t, CancelInverses, tt.in, tt.want)
		})
	}
}

func TestMergeRotations(t *testing.T) {
	pi := math.Pi
	// Computed at runtime so the expected value matches the pass's own
	// float64 addition (Go would fold a 0.5-0.4 literal exactly).
	a, b := 0.5, -0.4
	tests := []struct {
		name     string
		in, want []ir.Op
	}{
		{
			"rz merge",
			[]ir.Op{rot("rz", pi/4, 0), rot("rz", pi/4, 0)},
			[]ir.Op{rot("rz", pi/2, 0)},
		},
		{
			"rx cancel to zero",
			[]ir.Op{rot("rx", pi/3, 0), rot("rx", -pi/3, 0)},
			nil,
		},
		{
			"ry sums to 2pi drops",
			[]ir.Op{rot("ry", pi, 0), rot("ry", pi, 0)},
			nil,
		},
		{
			"u1 merge",
			[]ir.Op{rot("u1", 0.25, 2), rot("u1", 0.5, 2)},
			[]ir.Op{rot("u1", 0.75, 2)},
		},
		{
			"chain of three merges",
			[]ir.Op{rot("rz", 0.25, 0), rot("rz", 0.25, 0), rot("rz", 0.25, 0)},
			[]ir.Op{rot("rz", 0.75, 0)},
		},
		{
			"cascade after zero drop",
			// rz(a) [rx(b) rx(-b)] rz(-a): dropping the rx pair makes
			// the rz pair adjacent within the same sweep.
			[]ir.Op{rot("rz", 0.5, 0), rot("rx", 0.7, 0), rot("rx", -0.7, 0), rot("rz", -0.5, 0)},
			nil,
		},
		{
			"different axes do not merge",
			[]ir.Op{rot("rz", 0.5, 0), rot("rx", 0.5, 0)},
			[]ir.Op{rot("rz", 0.5, 0), rot("rx", 0.5, 0)},
		},
		{
			"different qubits do not merge",
			[]ir.Op{rot("rz", 0.5, 0), rot("rz", 0.5, 1)},
			[]ir.Op{rot("rz", 0.5, 0), rot("rz", 0.5, 1)},
		},
		{
			"barrier blocks",
			[]ir.Op{rot("rz", 0.5, 0), barrier(0), rot("rz", -0.5, 0)},
			[]ir.Op{rot("rz", 0.5, 0), barrier(0), rot("rz", -0.5, 0)},
		},
		{
			"near-zero sum within eps drops",
			[]ir.Op{rot("rz", 0.5, 0), rot("rz", -0.5+1e-12, 0)},
			nil,
		},
		{
			"sum near 2pi within eps drops",
			[]ir.Op{rot("rx", math.Pi, 0), rot("rx", math.Pi+1e-12, 0)},
			nil,
		},
		{
			"nonzero sum kept",
			[]ir.Op{rot("rz", a, 0), rot("rz", b, 0)},
			[]ir.Op{rot("rz", a+b, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkPass(t, MergeRotations, tt.in, tt.want)
		})
	}
}

func TestCancelAcrossWindow(t *testing.T) {
	pi := math.Pi
	tests := []struct {
		name     string
		in, want []ir.Op
	}{
		{
			"cancel across disjoint op",
			[]ir.Op{op("h", 0), op("x", 1), op("h", 0)},
			[]ir.Op{op("x", 1)},
		},
		{
			"cancel across several disjoint ops",
			[]ir.Op{op("cx", 0, 1), op("z", 2), op("t", 3), op("cx", 0, 1)},
			[]ir.Op{op("z", 2), op("t", 3)},
		},
		{
			"blocked by shared-qubit op",
			[]ir.Op{op("h", 0), op("z", 0), op("h", 0)},
			[]ir.Op{op("h", 0), op("z", 0), op("h", 0)},
		},
		{
			"blocked by overlapping cx",
			[]ir.Op{op("h", 0), op("cx", 0, 1), op("h", 0)},
			[]ir.Op{op("h", 0), op("cx", 0, 1), op("h", 0)},
		},
		{
			"blocked by barrier even on disjoint qubits",
			[]ir.Op{op("h", 0), barrier(1), op("h", 0)},
			[]ir.Op{op("h", 0), barrier(1), op("h", 0)},
		},
		{
			"blocked by measure on shared qubit",
			[]ir.Op{op("x", 0), measure(0, 0), op("x", 0)},
			[]ir.Op{op("x", 0), measure(0, 0), op("x", 0)},
		},
		{
			"measure on disjoint qubit commutes",
			[]ir.Op{op("x", 0), measure(1, 0), op("x", 0)},
			[]ir.Op{measure(1, 0)},
		},
		{
			"rotation merge across disjoint op",
			[]ir.Op{rot("rz", pi/4, 0), op("h", 1), rot("rz", pi/4, 0)},
			[]ir.Op{rot("rz", pi/2, 0), op("h", 1)},
		},
		{
			"rotation cancel across disjoint op",
			[]ir.Op{rot("ry", 0.3, 0), op("cx", 1, 2), rot("ry", -0.3, 0)},
			[]ir.Op{op("cx", 1, 2)},
		},
		{
			"chained cancellations",
			// After h0...h0 cancels, the two x1 become candidates in the
			// same sweep (i advances past them only after processing).
			[]ir.Op{op("h", 0), op("x", 1), op("h", 0), op("x", 1)},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkPass(t, CancelAcrossWindow, tt.in, tt.want)
		})
	}
}

func TestWindowLimit(t *testing.T) {
	// windowSize disjoint ops between a pair: still found. One more:
	// declined.
	within := []ir.Op{op("h", 0)}
	for i := 0; i < windowSize-1; i++ {
		within = append(within, op("t", 1+i%4))
	}
	within = append(within, op("h", 0))
	got := CancelAcrossWindow(deepCopy(within))
	if len(got) != len(within)-2 {
		t.Errorf("pair inside window not cancelled: %d ops left, want %d", len(got), len(within)-2)
	}

	beyond := []ir.Op{op("h", 0)}
	for i := 0; i < windowSize; i++ {
		beyond = append(beyond, op("t", 1+i%4))
	}
	beyond = append(beyond, op("h", 0))
	got = CancelAcrossWindow(deepCopy(beyond))
	if len(got) != len(beyond) {
		t.Errorf("pair beyond window was transformed: %d ops left, want %d", len(got), len(beyond))
	}
}

func TestOptimize(t *testing.T) {
	pi := math.Pi
	tests := []struct {
		name     string
		in, want []ir.Op
	}{
		{
			"passes cooperate",
			// cx cx cancels (adjacent), exposing rz(pi/4) rz(-pi/4)
			// which then cancels to nothing.
			[]ir.Op{rot("rz", pi/4, 0), op("cx", 0, 1), op("cx", 0, 1), rot("rz", -pi/4, 0)},
			nil,
		},
		{
			"multi-round fixpoint",
			// Round 1: the window pass cancels the x pair around t (h
			// blocks nothing there), leaving h0 t1 h0. Round 2: the
			// window pass cancels the h pair across the disjoint t.
			[]ir.Op{op("h", 0), op("x", 0), op("t", 1), op("x", 0), op("h", 0)},
			[]ir.Op{op("t", 1)},
		},
		{
			"barrier preserved as fence",
			[]ir.Op{op("h", 0), barrier(0, 1), op("h", 0), op("h", 1)},
			[]ir.Op{op("h", 0), barrier(0, 1), op("h", 0), op("h", 1)},
		},
		{
			"bell circuit untouched",
			[]ir.Op{op("h", 0), op("cx", 0, 1), measure(0, 0), measure(1, 1)},
			[]ir.Op{op("h", 0), op("cx", 0, 1), measure(0, 0), measure(1, 1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := deepCopy(tt.in)
			got := Optimize(tt.in)
			if !opsEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
			if !opsEqual(tt.in, orig) {
				t.Errorf("Optimize mutated its input")
			}
			// Optimize must be idempotent: a fixpoint stays fixed.
			if again := Optimize(got); !opsEqual(again, got) {
				t.Errorf("not a fixpoint: %+v -> %+v", got, again)
			}
		})
	}
}
