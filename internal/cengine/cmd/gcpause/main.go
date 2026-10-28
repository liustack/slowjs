// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command gcpause measures how long the Go garbage collector waits to stop
// the world while VMs run a script that stays in the engine. Built with
// -tags slowjs_cengine it runs the C engine, which Go can stop only where it
// polls. Built without, it runs the engine of lib, which Go stops by signal
// anywhere, so that one run gives both:
//
//	go run -tags slowjs_cengine ./internal/cengine/cmd/gcpause -secs 10
//	go run ./internal/cengine/cmd/gcpause -secs 10
//
// For each scenario it starts -vms VMs (default GOMAXPROCS-1, so that every
// P but the one that asks for the GC runs a VM), each in a goroutine that
// runs one script which never returns to Go, forces a GC every -gap, and
// reads runtime/metrics: how long a stop took to stop the other Ps
// (/sched/pauses/stopping/gc:seconds) and how long the world stood still in
// all (/sched/pauses/total/gc:seconds). The bounds of those histograms are
// about 6% apart, so the numbers are upper bounds of that size. At the end
// it interrupts the VMs and reports how long the slowest took to return.
// One line of tab separated fields per scenario goes to stdout.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync"
	"time"

	"github.com/liustack/slowjs"
)

// scenarios run one script each, which never ends by itself. Every one keeps
// the engine inside one long operation for as long as the operation takes.
var scenarios = []struct{ name, js string }{
	{"loop", `let a = 0; for (let i = 0; ; i++) { a += i * i }`},
	{"fib", `function f(n) { return n < 2 ? n : f(n - 1) + f(n - 2) } for (;;) f(27)`},
	{"regexp_backtrack", `const s = 'x'.repeat(24); for (;;) { /(x+x+)+y/.test(s) }`},
	{"regexp_scan", `const s = 'a'.repeat(1 << 26); for (;;) { /a*b/.test(s) }`},
	{"regexp_replace", `const s = 'ab'.repeat(1 << 24); for (;;) { s.replace(/a/g, 'cc') }`},
	{"json_parse", `const s = JSON.stringify(Array.from({ length: 300000 }, (_, i) => ({ id: i, name: 'item ' + i, tags: ['a', 'b'], v: i / 3 })));
		for (;;) { JSON.parse(s) }`},
	{"json_stringify", `const o = Array.from({ length: 300000 }, (_, i) => ({ id: i, name: 'item ' + i, tags: ['a', 'b'], v: i / 3 }));
		for (;;) { JSON.stringify(o) }`},
	{"string_search", `const s = 'ab'.repeat(1 << 26); for (;;) { s.indexOf('c'); s.lastIndexOf('c'); s.includes('ba' + 'c') }`},
	{"string_repeat_split", `for (;;) { 'ab'.repeat(1 << 25).split('b').length }`},
	{"array_join", `const a = Array.from({ length: 2000000 }, (_, i) => 'item' + i); for (;;) { a.join(',') }`},
	{"sort_typed", `const a = new Float64Array(1 << 22).map(() => Math.random()); for (;;) { a.slice().sort() }`},
	{"sort_default", `const a = Array.from({ length: 500000 }, () => Math.random()); for (;;) { a.slice().sort() }`},
	{"bigint_mul", `let x = 7n ** 300000n; for (;;) { x * x }`},
	{"array_fill", `for (;;) { new Array(1 << 24).fill(0).reverse().indexOf(1) }`},
	{"normalize", `const s = 'éå'.repeat(1 << 22); for (;;) { s.normalize('NFC') }`},
}

type histo struct {
	counts []uint64
	bounds []float64
}

func read(name string) histo {
	s := []metrics.Sample{{Name: name}}
	metrics.Read(s)
	h := s[0].Value.Float64Histogram()
	return histo{append([]uint64(nil), h.Counts...), h.Buckets}
}

// since gives the number of pauses since before and the P50, P99 and highest
// upper bound among them, in microseconds.
func (h histo) since(before histo) (n uint64, p50, p99, hi float64) {
	c := make([]uint64, len(h.counts))
	for i := range c {
		c[i] = h.counts[i] - before.counts[i]
		n += c[i]
	}
	q := func(p float64) float64 {
		target := uint64(math.Ceil(float64(n) * p))
		var cum uint64
		for i, k := range c {
			cum += k
			if k > 0 && cum >= target {
				return h.bounds[i+1] * 1e6
			}
		}
		return 0
	}
	for i := len(c) - 1; i >= 0; i-- {
		if c[i] > 0 {
			hi = h.bounds[i+1] * 1e6
			break
		}
	}
	return n, q(.5), q(.99), hi
}

func main() {
	secs := flag.Float64("secs", 10, "seconds of each scenario")
	gap := flag.Duration("gap", 5*time.Millisecond, "time between two forced GCs")
	vms := flag.Int("vms", runtime.GOMAXPROCS(0)-1, "VMs running the script (at least 1)")
	only := flag.String("only", "", "comma separated scenario names (default all)")
	label := flag.String("label", "", "first field of each line")
	flag.Parse()
	*vms = max(*vms, 1)
	want := map[string]bool{}
	for _, n := range strings.Split(*only, ",") {
		if n != "" {
			want[n] = true
		}
	}
	fmt.Println("label\tscenario\tvms\tgomaxprocs\tgcs\tstop_p50_us\tstop_p99_us\tstop_max_us\ttotal_p50_us\ttotal_p99_us\ttotal_max_us\tinterrupt_ms")
	for _, sc := range scenarios {
		if len(want) > 0 && !want[sc.name] {
			continue
		}
		if err := run(*label, sc.name, sc.js, *vms, *secs, *gap); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", sc.name, err)
			os.Exit(1)
		}
	}
}

func run(label, name, js string, n int, secs float64, gap time.Duration) error {
	vms := make([]*slowjs.VM, n)
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := range vms {
		m, err := slowjs.NewVM()
		if err != nil {
			return err
		}
		vms[i] = m
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = m.Eval(js, slowjs.EvalGlobal)
		}()
	}
	// Let the scripts get to their long operation.
	time.Sleep(2 * time.Second)
	stop0, total0 := read("/sched/pauses/stopping/gc:seconds"), read("/sched/pauses/total/gc:seconds")
	end := time.Now().Add(time.Duration(secs * float64(time.Second)))
	var gcs int
	for time.Now().Before(end) {
		runtime.GC()
		gcs++
		time.Sleep(gap)
	}
	stop1, total1 := read("/sched/pauses/stopping/gc:seconds"), read("/sched/pauses/total/gc:seconds")
	begin := time.Now()
	done := make(chan struct{})
	for _, m := range vms {
		m.Interrupt()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Minute):
		return fmt.Errorf("the scripts did not stop a minute after Interrupt")
	}
	interrupt := time.Since(begin)
	for i, err := range results {
		if err == nil {
			return fmt.Errorf("VM %d: the script ended without an error: it was meant to run until interrupted", i)
		}
	}
	for _, m := range vms {
		if err := m.Close(); err != nil {
			return err
		}
	}
	_, s50, s99, smax := stop1.since(stop0)
	_, t50, t99, tmax := total1.since(total0)
	fmt.Printf("%s\t%s\t%d\t%d\t%d\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\t%.2f\n", label, name, n, runtime.GOMAXPROCS(0), gcs, s50, s99, smax, t50, t99, tmax,
		float64(interrupt)/1e6)
	return nil
}
