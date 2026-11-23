// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Command stress renders tifa's pinned pages for a set time on pools of VMs
// kept as tifa's pool keeps them, with memory limits, eval timeouts,
// Interrupts and kills landing at random points of the renders, and checks
// that every VM stays correct and bounded: each render gives the page's HTML
// or the error its stop documents, a VM goes on after a stop that was not a
// kill, the memory of the engine, of the host and of Go and the goroutines
// stay bounded with no drift after the warm-up, and closing every VM gives
// all its memory back. See "Long runs" in docs/internals/testing.md.
//
//	go run ./internal/stress -duration 10m -series series.csv
//
// It exits 1 when a check failed, and prints each failure as it happens.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liustack/beastjs/internal/gohost"
	"github.com/liustack/beastjs/internal/hostmem"
)

// config is what a run does.
type config struct {
	repo          string
	duration      time.Duration
	warmup        time.Duration // the part of the run the drift checks leave out
	every         time.Duration // how often a row of the series is taken
	workers       int
	churn         int // churn VMs of each bundle in each pool, besides its steady VM
	seed          uint64
	kinds         []string
	renderTimeout time.Duration // the eval timeout of a render that is not stopped on purpose
	measureEvery  int           // how many renders of a steady VM between two measures
	// percent of the steps that are each op, the rest normal renders
	oom, timeout, interrupt, kill, abort int
	// refuse is how many of every thousand maps of the OS are refused at random
	// (-tags testhooks), 0 for none
	refuse int
	// the drift a measure may show from the middle third of the run after
	// the warm-up to its last third: a fraction and bytes
	tolerance  float64
	slack      int64
	series     string // the file of the series, none when empty
	cpuprofile string // the file of a CPU profile of the run, none when empty
	maxLog     int    // failures printed in full
	stopOnFail bool
	log        io.Writer
}

// stress is a run.
type stress struct {
	cfg     config
	bundles []*bundle
	workers []*worker
	counts  counts
	steady  steadyVMs

	ids                                        atomic.Int64
	loaded, closed, retired, kills, interrupts atomic.Int64
	finalizeFails                              atomic.Int64
	// aborts of the engine, and the renders that gave the right HTML after the
	// first of them
	aborts, htmlAfterAbort atomic.Int64
	refused, loadsRefused  atomic.Int64
	// refusing is set while the OS refuses maps at random: any render may then
	// run out of memory, and a load may fail
	refusing bool
	failures atomic.Int64
	logMu    sync.Mutex
	failed   []string
}

func (s *stress) nextID() int { return int(s.ids.Add(1)) }

func (s *stress) stopNow() bool { return s.cfg.stopOnFail && s.failures.Load() > 0 }

// unexpected records a failure of a check: what a render or a step gave that
// it may not.
func (s *stress) unexpected(worker, vm int, what, page, detail string) {
	n := s.failures.Add(1)
	line := fmt.Sprintf("FAIL worker %d vm %d %s %s: %s", worker, vm, what, page, detail)
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if len(s.failed) < 200 {
		s.failed = append(s.failed, line)
	}
	if int(n) <= s.cfg.maxLog {
		fmt.Fprintln(s.cfg.log, line)
	}
}

// steadyVMs keeps the last measure of each steady VM.
type steadyVMs struct {
	mu   sync.Mutex
	last map[string][3]int64 // kept, size, renders
}

func (st *steadyVMs) record(worker int, bundle string, kept, size int64, renders int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.last == nil {
		st.last = map[string][3]int64{}
	}
	st.last[fmt.Sprintf("%d/%s", worker, bundle)] = [3]int64{kept, size, int64(renders)}
}

// sums returns the sum of what the steady VMs keep, their largest, and the
// sum of their MemorySize, at their last measures.
func (st *steadyVMs) sums() (kept, maxKept, size int64, n int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, m := range st.last {
		kept += m[0]
		maxKept = max(maxKept, m[0])
		size += m[1]
		n++
	}
	return
}

func main() {
	var c config
	var kinds string
	flag.StringVar(&c.repo, "repo", "", "the root of the liustack/beastjs checkout (default: the first one above the working directory)")
	flag.DurationVar(&c.duration, "duration", time.Minute, "how long the workers render")
	flag.DurationVar(&c.warmup, "warmup", 0, "the start of the run the drift checks leave out (default: a fifth of -duration)")
	flag.DurationVar(&c.every, "every", 0, "how often a row of the series is taken (default: -duration/120, at least a second)")
	flag.IntVar(&c.workers, "workers", 4, "worker goroutines, each with a pool of its own")
	flag.IntVar(&c.churn, "churn", 1, "churn VMs of each bundle in each pool, besides the steady VM")
	flag.Uint64Var(&c.seed, "seed", 0, "the seed of the random choices (default: from the clock, printed)")
	flag.StringVar(&kinds, "kinds", "store,wb,mui", "the kinds of pages, of store, wb and mui")
	flag.DurationVar(&c.renderTimeout, "render-timeout", 10*time.Second, "the eval timeout of a render that is not stopped on purpose")
	flag.IntVar(&c.measureEvery, "measure-every", 64, "renders of a steady VM between two measures of what it keeps")
	flag.IntVar(&c.oom, "oom", 12, "percent of the renders under a memory limit drawn at random")
	flag.IntVar(&c.timeout, "timeout", 12, "percent of the renders under an eval timeout drawn at random")
	flag.IntVar(&c.interrupt, "interrupt", 12, "percent of the renders with an Interrupt at a random moment")
	flag.IntVar(&c.kill, "kill", 3, "percent of the renders of churn VMs that end in a kill")
	flag.IntVar(&c.abort, "abort", 0, "percent of the renders of churn VMs during which the engine aborts (needs -tags testhooks)")
	flag.IntVar(&c.refuse, "refuse", 0, "of every thousand maps of chunks and blocks, how many the OS refuses at random (needs -tags testhooks)")
	flag.Float64Var(&c.tolerance, "tolerance", 0.05, "the growth a memory measure may show from the middle third of the run after the warm-up to the last third, as a fraction")
	flag.Int64Var(&c.slack, "slack", 1<<20, "and in bytes, besides the fraction")
	flag.StringVar(&c.series, "series", "", "write the series, one row per -every, as CSV to this file")
	flag.IntVar(&c.maxLog, "max-log", 50, "failures printed in full")
	flag.BoolVar(&c.stopOnFail, "stop-on-fail", false, "end the run at the first failure")
	flag.StringVar(&c.cpuprofile, "cpuprofile", "", "write a CPU profile of the run to this file, each render labeled with its worker, vm, op and page")
	flag.Parse()
	c.kinds = strings.Split(kinds, ",")
	c.log = os.Stderr
	if err := run(c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// findRepo returns the first folder from the working directory up whose
// go.mod is the module of beastjs.
func findRepo() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		b, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil && bytes.HasPrefix(b, []byte("module github.com/liustack/beastjs\n")) {
			return d, nil
		}
		up := filepath.Dir(d)
		if up == d {
			return "", errors.New("stress: no liustack/beastjs checkout above the working directory: pass -repo")
		}
		d = up
	}
}

// run runs the stress of c, and returns an error when a check failed.
func run(c config) error {
	if c.repo == "" {
		r, err := findRepo()
		if err != nil {
			return err
		}
		c.repo = r
	}
	if c.warmup == 0 {
		c.warmup = c.duration / 5
	}
	if c.every == 0 {
		c.every = max(time.Second, c.duration/120)
	}
	if c.seed == 0 {
		c.seed = uint64(time.Now().UnixNano())
	}
	if c.workers < 1 || c.churn < 0 || c.measureEvery < 1 || c.oom+c.timeout+c.interrupt+c.kill+c.abort > 100 {
		return errors.New("stress: -workers at least 1, -churn at least 0, -measure-every at least 1, and the percents of the ops at most 100 together")
	}
	if c.abort > 0 && !canAbort {
		return errors.New("stress: -abort needs a build with -tags testhooks")
	}
	if c.cpuprofile != "" {
		f, err := os.Create(c.cpuprofile)
		if err != nil {
			return err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return err
		}
		defer func() {
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				fmt.Fprintf(c.log, "stress: the CPU profile: %v\n", err)
			}
		}()
	}
	s := &stress{cfg: c}
	fmt.Fprintf(c.log, "stress: seed %d, %d workers, %d churn VMs a bundle, %v, warm-up %v, kinds %s, ops oom %d%% timeout %d%% interrupt %d%% kill %d%% abort %d%%, GOMAXPROCS %d\n",
		c.seed, c.workers, c.churn, c.duration, c.warmup, strings.Join(c.kinds, ","), c.oom, c.timeout, c.interrupt, c.kill, c.abort, runtime.GOMAXPROCS(0))

	base := snapshot()
	var err error
	if s.bundles, err = openBundles(c.repo, c.kinds); err != nil {
		return err
	}
	for _, b := range s.bundles {
		if err := reference(b, c.renderTimeout); err != nil {
			closeBundles(s.bundles)
			return fmt.Errorf("%s: %w", b.name, err)
		}
		for _, p := range b.pages {
			fmt.Fprintf(c.log, "stress: %s %s: %x, %v, %d KiB a render, %d pushes\n", b.name, p.name, p.hash[:8], p.took.Round(10*time.Microsecond), p.grows>>10, p.push)
		}
	}

	// the pools
	vmsPerPool := len(s.bundles) * (1 + c.churn)
	for w := range c.workers {
		wk := &worker{s: s, id: w, rng: rand.New(rand.NewPCG(c.seed, uint64(w)))}
		for _, b := range s.bundles {
			for k := range 1 + c.churn {
				v, err := load(b, s.nextID(), c.renderTimeout)
				if err != nil {
					s.closeAll()
					return err
				}
				v.steady = k == 0
				wk.pool = append(wk.pool, v)
				s.loaded.Add(1)
			}
		}
		wk.updateEngine()
		s.workers = append(s.workers, wk)
	}
	maxVMs := int64(c.workers * vmsPerPool)

	// the series, sampled while the workers run
	ser := &series{s: s, base: base, maxVMs: maxVMs, maxGoroutines: base.goroutines + 2*c.workers + 8}
	if c.series != "" {
		f, err := os.Create(c.series)
		if err != nil {
			s.closeAll()
			return err
		}
		defer f.Close()
		ser.out = f
	}
	undo := func() {}
	if c.refuse > 0 {
		var err error
		undo, err = refuse(c.refuse, c.seed, func() { s.refused.Add(1) })
		if err != nil {
			s.closeAll()
			return err
		}
		s.refusing = true
	}
	start := time.Now()
	end := start.Add(c.duration)
	ser.start = start
	done := make(chan struct{})
	var sampler sync.WaitGroup
	sampler.Go(func() { ser.run(c.every, done) })
	var wg sync.WaitGroup
	for _, w := range s.workers {
		wg.Go(func() { w.run(end) })
	}
	wg.Wait()
	undo()
	close(done)
	sampler.Wait()

	// everything closed: the totals of the host are back where they were
	errs := []error{closeBundles(s.bundles)}
	errs = append(errs, s.checkFreed(base))
	errs = append(errs, ser.drift(c))
	s.report(time.Since(start), ser)
	if n := s.failures.Load(); n > 0 {
		errs = append(errs, fmt.Errorf("stress: %d failures, the first ones:\n%s", n, strings.Join(s.firstFailures(20), "\n")))
	}
	return errors.Join(errs...)
}

func (s *stress) firstFailures(n int) []string {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return slices.Clone(s.failed[:min(n, len(s.failed))])
}

// closeAll closes every VM of the pools and the bundles, after a setup that
// failed.
func (s *stress) closeAll() {
	for _, w := range s.workers {
		for _, v := range w.pool {
			w.closeVM(v, "after the setup failed")
		}
	}
	closeBundles(s.bundles)
}

// checkFreed checks that every VM and copy of bytecode, once closed, gave
// back all it held: the totals of the host, the blocks of Go functions, and
// the goroutines.
func (s *stress) checkFreed(base sample) error {
	var errs []error
	if t := hostmem.ProcessTotals(); t != base.totals {
		errs = append(errs, fmt.Errorf("stress: after every VM closed the host holds %+v, before the first %+v", t, base.totals))
	}
	if n := hostmem.Blocks(); n != 0 {
		errs = append(errs, fmt.Errorf("stress: %d blocks of Go functions are still mapped", n))
	}
	if n := hostmem.Kept(); n != 0 {
		errs = append(errs, fmt.Errorf("stress: %d bytes are still kept for Go functions", n))
	}
	if l := gohost.Leftovers(); len(l) != 0 {
		errs = append(errs, fmt.Errorf("stress: %d closed VMs still hold memory: %+v", len(l), l))
	}
	if n := runtime.NumGoroutine(); n > base.goroutines {
		errs = append(errs, fmt.Errorf("stress: %d goroutines after the run, %d before it", n, base.goroutines))
	}
	if closed, loaded := s.closed.Load(), s.loaded.Load(); closed != loaded {
		errs = append(errs, fmt.Errorf("stress: %d VMs loaded and %d closed", loaded, closed))
	}
	return errors.Join(errs...)
}

// report prints the renders by op and outcome and what the run did.
func (s *stress) report(took time.Duration, ser *series) {
	w := s.cfg.log
	total := s.counts.total()
	fmt.Fprintf(w, "stress: %d renders in %v (%.1f a second), %d VMs loaded, %d closed after a stop, %d killed, %d Interrupts sent, %d failed jobs after a stop\n",
		total, took.Round(time.Second), float64(total)/took.Seconds(), s.loaded.Load(), s.retired.Load(), s.kills.Load(), s.interrupts.Load(), s.finalizeFails.Load())
	if s.refusing {
		fmt.Fprintf(w, "stress: the OS refused %d maps, and %d loads failed for it\n", s.refused.Load(), s.loadsRefused.Load())
	}
	if n := s.aborts.Load(); n > 0 {
		fmt.Fprintf(w, "stress: the engine aborted %d times, and %d renders after the first abort gave the right HTML\n", n, s.htmlAfterAbort.Load())
	}
	for r, name := range []string{"VMs that never stopped", "VMs that went on after a stop"} {
		fmt.Fprintf(w, "stress: %s\n", name)
		for o := range nOps {
			var parts []string
			for x := range nOutcomes {
				if n := s.counts.n[r][o][x].Load(); n > 0 {
					parts = append(parts, fmt.Sprintf("%s %d", outcomeNames[x], n))
				}
			}
			if len(parts) > 0 {
				fmt.Fprintf(w, "stress:   %-9s %s\n", opNames[o], strings.Join(parts, ", "))
			}
		}
	}
	ser.summary(w)
	fmt.Fprintf(w, "stress: %d failures\n", s.failures.Load())
}
