// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package enginecmp

import (
	"fmt"
	"testing"
	"time"

	"github.com/liustack/slowjs"
)

// TestLoopPeriod times one iteration of the JS loop used by TestGCStopsJSLoop,
// so the time between two polls (10000 back edges) can be read off.
func TestLoopPeriod(t *testing.T) {
	m, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	const n = 20_000_000
	best := time.Duration(1 << 62)
	for r := 0; r < 5; r++ {
		t0 := time.Now()
		if _, err := m.Eval(fmt.Sprintf(`{ let acc = 0; for (let i = 0; i < %d; i++) { acc += i * i } }`, n), slowjs.EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(t0); d < best {
			best = d
		}
	}
	fmt.Printf("JS loop: %.2f ns per iteration, %.1f µs per 10000 iterations\n",
		float64(best)/n, float64(best)/n*10000/1000)
}
