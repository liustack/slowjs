// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT

// Package enginecmp measures the current slowjs engine under the same GC stop
// setup as the generated prototype, for comparison.
package enginecmp

import (
	"fmt"
	"runtime"
	"runtime/metrics"
	"sync"
	"testing"
	"time"

	"github.com/liustack/slowjs"
)

// TestGCStopsJSLoop runs GOMAXPROCS-1 VMs, each on its own goroutine, in an
// endless JS loop of the same shape as the C spin loop, and forces 150 GCs
// 2 ms apart, exactly like the prototype's TestGCStopsSpinLoop.
func TestGCStopsJSLoop(t *testing.T) {
	n := runtime.GOMAXPROCS(0) - 1
	if n < 1 {
		n = 1
	}
	var vms []*slowjs.VM
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		m, err := slowjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		vms = append(vms, m)
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Eval(`let acc = 0; for (let i = 0; ; i++) { acc += i * i }`, slowjs.EvalGlobal)
		}()
	}
	read := func() *metrics.Float64Histogram {
		s := []metrics.Sample{{Name: "/sched/pauses/stopping/gc:seconds"}}
		metrics.Read(s)
		return s[0].Value.Float64Histogram()
	}
	time.Sleep(20 * time.Millisecond)
	before := read()
	for i := 0; i < 150; i++ {
		runtime.GC()
		time.Sleep(2 * time.Millisecond)
	}
	after := read()
	for _, m := range vms {
		m.Interrupt()
	}
	wg.Wait()
	for _, m := range vms {
		m.Close()
	}
	var total uint64
	counts := make([]uint64, len(after.Counts))
	for i := range after.Counts {
		counts[i] = after.Counts[i] - before.Counts[i]
		total += counts[i]
	}
	fmt.Printf("current slowjs JS loop: %d workers, %d stops\n", n, total)
	for i, c := range counts {
		if c > 0 {
			fmt.Printf("  [%.1f, %.1f) µs: %d\n", after.Buckets[i]*1e6, after.Buckets[i+1]*1e6, c)
		}
	}
	q := func(p float64) float64 {
		target := uint64(float64(total)*p + 0.999999)
		var cum uint64
		for i, c := range counts {
			cum += c
			if cum >= target && c > 0 {
				return after.Buckets[i+1] * 1e6
			}
		}
		return 0
	}
	fmt.Printf("  P50 <= %.1f µs, P99 <= %.1f µs, max <= %.1f µs\n", q(0.5), q(0.99), q(1))
}
