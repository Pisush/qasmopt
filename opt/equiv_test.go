package opt

import (
	"math"
	"math/rand"
	"testing"

	"github.com/Pisush/qasmopt/ir"
	"github.com/Pisush/qasmopt/sim"
)

// TestOptimizeEquivalence generates random circuits (seeded, so the test
// is deterministic), optimizes them, and simulates both versions from a
// random initial state. The final state vectors must match up to global
// phase — the strongest semantics-preservation check available.
func TestOptimizeEquivalence(t *testing.T) {
	const (
		nQubits  = 4
		nOps     = 60
		nTrials  = 50
		tightEps = 1e-9
	)
	rng := rand.New(rand.NewSource(20260706))
	angles := []float64{math.Pi, math.Pi / 2, math.Pi / 4, -math.Pi / 2, 2 * math.Pi, 0.3}

	for trial := 0; trial < nTrials; trial++ {
		ops := randomCircuit(rng, nQubits, nOps, angles)
		optimized := Optimize(ops)

		start := randomState(rng, nQubits)
		a := start.Clone()
		if err := a.RunOps(ops); err != nil {
			t.Fatalf("trial %d: original: %v", trial, err)
		}
		b := start.Clone()
		if err := b.RunOps(optimized); err != nil {
			t.Fatalf("trial %d: optimized: %v", trial, err)
		}
		if !sim.EqualUpToGlobalPhase(a, b, tightEps) {
			t.Fatalf("trial %d: optimization changed circuit semantics\noriginal (%d ops):  %+v\noptimized (%d ops): %+v",
				trial, len(ops), ops, len(optimized), optimized)
		}
	}
}

// randomCircuit builds a random op list. To make sure the optimizer has
// real work to do, it frequently emits back-to-back or nearly-adjacent
// inverse pairs and repeated rotations, plus occasional barriers.
func randomCircuit(rng *rand.Rand, nQubits, nOps int, angles []float64) []ir.Op {
	gates1 := []string{"x", "y", "z", "h", "s", "sdg", "t", "tdg"}
	rots := []string{"rx", "ry", "rz", "u1"}
	var ops []ir.Op
	for len(ops) < nOps {
		q := rng.Intn(nQubits)
		switch rng.Intn(10) {
		case 0, 1, 2: // single-qubit Clifford/T, often doubled
			name := gates1[rng.Intn(len(gates1))]
			ops = append(ops, ir.Op{Name: name, Qubits: []int{q}})
			if rng.Intn(2) == 0 {
				ops = append(ops, ir.Op{Name: name, Qubits: []int{q}})
			}
		case 3, 4: // rotation, often repeated on the same qubit
			name := rots[rng.Intn(len(rots))]
			theta := angles[rng.Intn(len(angles))]
			ops = append(ops, ir.Op{Name: name, Qubits: []int{q}, Params: []float64{theta}})
			if rng.Intn(2) == 0 {
				ops = append(ops, ir.Op{Name: name, Qubits: []int{q}, Params: []float64{-theta}})
			}
		case 5, 6: // cx, sometimes doubled
			t := (q + 1 + rng.Intn(nQubits-1)) % nQubits
			ops = append(ops, ir.Op{Name: "cx", Qubits: []int{q, t}})
			if rng.Intn(2) == 0 {
				ops = append(ops, ir.Op{Name: "cx", Qubits: []int{q, t}})
			}
		case 7: // u2/u3: never optimized, exercise "decline" paths
			if rng.Intn(2) == 0 {
				ops = append(ops, ir.Op{Name: "u2", Qubits: []int{q},
					Params: []float64{randAngle(rng), randAngle(rng)}})
			} else {
				ops = append(ops, ir.Op{Name: "u3", Qubits: []int{q},
					Params: []float64{randAngle(rng), randAngle(rng), randAngle(rng)}})
			}
		case 8: // inverse pair split by a disjoint op (window fodder)
			name := gates1[rng.Intn(len(gates1))]
			inv, _ := inverseOf(name)
			other := (q + 1) % nQubits
			ops = append(ops,
				ir.Op{Name: name, Qubits: []int{q}},
				ir.Op{Name: "h", Qubits: []int{other}},
				ir.Op{Name: inv, Qubits: []int{q}},
			)
		case 9: // occasional barrier fence
			ops = append(ops, ir.Op{Name: ir.OpBarrier, Qubits: []int{q}})
		}
	}
	return ops
}

func randAngle(rng *rand.Rand) float64 { return (rng.Float64() - 0.5) * 4 * math.Pi }

// randomState returns a random normalized state vector.
func randomState(rng *rand.Rand, nQubits int) sim.State {
	s := make(sim.State, 1<<nQubits)
	norm := 0.0
	for i := range s {
		re, im := rng.NormFloat64(), rng.NormFloat64()
		s[i] = complex(re, im)
		norm += re*re + im*im
	}
	scale := complex(1/math.Sqrt(norm), 0)
	for i := range s {
		s[i] *= scale
	}
	return s
}
