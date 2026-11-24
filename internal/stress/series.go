// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/metrics"
	"strconv"
	"time"

	"github.com/liustack/beastjs/internal/hostmem"
)

// sample is a row of the series.
type sample struct {
	t          time.Duration
	renders    int64
	failures   int64
	goroutines int
	// Go: the heap the last collection marked live, the heap objects, and
	// all the memory the Go runtime holds
	goLive, goObjects, goTotal int64
	totals                     hostmem.Totals
	blocks, kept               int64
	// engine is the sum of MemorySize of the VMs of the pools, as each worker
	// last read it
	engine int64
	// what the steady VMs keep after a full collection, at their last
	// measures: the sum, the largest, the sum of MemorySize, and how many
	steadyKept, steadyMax, steadySize int64
	steadyN                           int
	rss                               int64 // resident memory of the process, 0 where it is not read
}

var goMetrics = []string{"/gc/heap/live:bytes", "/memory/classes/heap/objects:bytes", "/memory/classes/total:bytes"}

// snapshot reads what does not need the run: Go, the host and the process.
func snapshot() sample {
	ms := make([]metrics.Sample, len(goMetrics))
	for i, n := range goMetrics {
		ms[i].Name = n
	}
	metrics.Read(ms)
	val := func(i int) int64 {
		if ms[i].Value.Kind() != metrics.KindUint64 {
			panic("stress: no metric " + goMetrics[i])
		}
		return int64(ms[i].Value.Uint64())
	}
	return sample{
		goroutines: runtime.NumGoroutine(),
		goLive:     val(0), goObjects: val(1), goTotal: val(2),
		totals: hostmem.ProcessTotals(),
		blocks: hostmem.Blocks(), kept: hostmem.Kept(),
		rss: rss(),
	}
}

// rss returns the resident memory of the process on linux, from
// /proc/self/statm, and 0 elsewhere.
func rss() int64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		panic(err)
	}
	f := bytes.Fields(b)
	pages, err := strconv.ParseInt(string(f[1]), 10, 64)
	if err != nil {
		panic(err)
	}
	return pages * int64(os.Getpagesize())
}

// series takes a sample every so often while the workers run, checks the
// bounds that hold at every instant, and keeps the rows for the checks of
// drift at the end.
type series struct {
	s             *stress
	base          sample
	start         time.Time
	maxVMs        int64
	maxGoroutines int
	out           io.Writer
	rows          []sample
}

const header = "t_s,renders,failures,goroutines,go_live,go_objects,go_total,host_vms,host_stacks,host_chunks,host_heap,host_readonly,blocks,kept,engine,steady_kept,steady_max,steady_size,steady_n,rss"

func (ser *series) run(every time.Duration, done <-chan struct{}) {
	if ser.out != nil {
		fmt.Fprintln(ser.out, header)
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			ser.take()
		}
	}
}

func (ser *series) take() {
	s := ser.s
	r := snapshot()
	r.t = time.Since(ser.start)
	r.renders = s.counts.total()
	r.failures = s.failures.Load()
	for _, w := range s.workers {
		r.engine += w.engine.Load()
	}
	r.steadyKept, r.steadyMax, r.steadySize, r.steadyN = s.steady.sums()
	ser.rows = append(ser.rows, r)
	if ser.out != nil {
		fmt.Fprintf(ser.out, "%.1f,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
			r.t.Seconds(), r.renders, r.failures, r.goroutines, r.goLive, r.goObjects, r.goTotal,
			r.totals.Memories, r.totals.Stacks, r.totals.Chunks, r.totals.Heap, r.totals.ReadOnly,
			r.blocks, r.kept, r.engine, r.steadyKept, r.steadyMax, r.steadySize, r.steadyN, r.rss)
	}
	fmt.Fprintf(s.cfg.log, "stress: %v %d renders, %d failures, %d goroutines, Go live %d MiB, host heap %d MiB in %d VMs, engine %d MiB, steady VMs keep %d MiB (largest %d KiB)\n",
		r.t.Round(time.Second), r.renders, r.failures, r.goroutines, r.goLive>>20, r.totals.Heap>>20, r.totals.Memories, r.engine>>20, r.steadyKept>>20, r.steadyMax>>10)
	// The bounds that hold at every instant: the VMs of the pools, which a
	// replacement closes before it loads, and the goroutines of the
	// workers, the sampler and the interrupters, one at most for each
	// worker.
	if vms := r.totals.Memories - ser.base.totals.Memories; vms > ser.maxVMs {
		s.unexpected(-1, -1, "series", "", fmt.Sprintf("%d VMs hold memory, the pools have %d", vms, ser.maxVMs))
	}
	if r.goroutines > ser.maxGoroutines {
		s.unexpected(-1, -1, "series", "", fmt.Sprintf("%d goroutines, at most %d", r.goroutines, ser.maxGoroutines))
	}
	if heap := r.totals.Heap - ser.base.totals.Heap; heap > ser.maxVMs*2*vmMemory {
		s.unexpected(-1, -1, "series", "", fmt.Sprintf("the heaps of the VMs hold %d bytes, twice the memory limit of %d VMs is %d", heap, ser.maxVMs, ser.maxVMs*2*vmMemory))
	}
}

// measures are the series the checks of drift look at, with the factor of
// -tolerance each may grow by: what the steady VMs keep, which only bounded
// caches grow once they have rendered every page, is held to it, and the
// sums that move with the renders in flight to four times it.
var measures = []struct {
	name   string
	factor float64
	get    func(sample) int64
}{
	{"steady_kept", 1, func(r sample) int64 { return r.steadyKept }},
	{"steady_size", 4, func(r sample) int64 { return r.steadySize }},
	{"engine", 4, func(r sample) int64 { return r.engine }},
	{"host_heap", 4, func(r sample) int64 { return r.totals.Heap }},
	{"go_live", 4, func(r sample) int64 { return r.goLive }},
}

// drift compares the mean of each measure over the last third of the rows
// after the warm-up with its mean over the middle third: a growth past the
// tolerance is a failure.
func (ser *series) drift(c config) error {
	var rows []sample
	for _, r := range ser.rows {
		if r.t >= c.warmup && r.steadyN > 0 {
			rows = append(rows, r)
		}
	}
	if len(rows) < 6 {
		fmt.Fprintf(c.log, "stress: %d rows after the warm-up, too few for the checks of drift\n", len(rows))
		return nil
	}
	third := len(rows) / 3
	mid, last := rows[len(rows)-2*third:len(rows)-third], rows[len(rows)-third:]
	mean := func(rs []sample, get func(sample) int64) float64 {
		var sum float64
		for _, r := range rs {
			sum += float64(get(r))
		}
		return sum / float64(len(rs))
	}
	var errs []error
	for _, m := range measures {
		a, b := mean(mid, m.get), mean(last, m.get)
		bound := a*(1+m.factor*c.tolerance) + float64(c.slack)
		verdict := "ok"
		if b > bound {
			verdict = "DRIFT"
			errs = append(errs, fmt.Errorf("stress: %s grew from a mean of %.0f to %.0f, past %.0f", m.name, a, b, bound))
		}
		fmt.Fprintf(c.log, "stress: drift %-11s middle third %8.2f MiB, last third %8.2f MiB (%+.2f%%), bound %8.2f MiB: %s\n",
			m.name, a/(1<<20), b/(1<<20), 100*(b-a)/a, bound/(1<<20), verdict)
	}
	return errors.Join(errs...)
}

// summary prints the highest value of each measure over the run.
func (ser *series) summary(w io.Writer) {
	var hi sample
	for _, r := range ser.rows {
		hi.goroutines = max(hi.goroutines, r.goroutines)
		hi.goLive = max(hi.goLive, r.goLive)
		hi.goTotal = max(hi.goTotal, r.goTotal)
		hi.totals.Heap = max(hi.totals.Heap, r.totals.Heap)
		hi.engine = max(hi.engine, r.engine)
		hi.steadyMax = max(hi.steadyMax, r.steadyMax)
		hi.rss = max(hi.rss, r.rss)
	}
	fmt.Fprintf(w, "stress: highest over %d rows: %d goroutines, Go live %d MiB, Go total %d MiB, host heap %d MiB, engine %d MiB, a steady VM keeps %d KiB, RSS %d MiB\n",
		len(ser.rows), hi.goroutines, hi.goLive>>20, hi.goTotal>>20, hi.totals.Heap>>20, hi.engine>>20, hi.steadyMax>>10, hi.rss>>20)
}
