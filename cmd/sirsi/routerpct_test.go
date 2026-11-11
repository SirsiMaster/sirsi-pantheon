package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestPctNearestRank(t *testing.T) {
	hundred := make([]float64, 100)
	for i := range hundred {
		hundred[i] = float64(i + 1) // 1..100
	}
	four := []float64{12, 37, 95, 300}
	tests := []struct {
		name string
		in   []float64
		p    float64
		want float64
	}{
		{"empty", nil, 95, 0},
		{"one p50", []float64{7}, 50, 7},
		{"one p99", []float64{7}, 99, 7},
		{"four p50", four, 50, 37},
		{"four p95 is the max, not the third", four, 95, 300},
		{"four p99", four, 99, 300},
		{"four p100", four, 100, 300},
		{"hundred p50", hundred, 50, 50},
		{"hundred p95", hundred, 95, 95},
		{"hundred p99", hundred, 99, 99},
		{"hundred p100", hundred, 100, 100},
		{"p0 clamps to first", four, 0, 12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pct(tc.in, tc.p); got != tc.want {
				t.Fatalf("pct(%v, %v) = %v, want %v", tc.in, tc.p, got, tc.want)
			}
		})
	}
}

// pct requires ascending input and must not mutate it; callers sort first.
func TestPctDoesNotMutateInput(t *testing.T) {
	in := []float64{300, 12, 95, 37} // unsorted on purpose
	cp := append([]float64(nil), in...)
	_ = pct(in, 95)
	if !reflect.DeepEqual(in, cp) {
		t.Fatalf("pct mutated its input: %v -> %v", cp, in)
	}
	sort.Float64s(in)
	if got := pct(in, 95); got != 300 {
		t.Fatalf("after caller sort, p95 = %v, want 300", got)
	}
}
