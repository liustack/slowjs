// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command jsrun evaluates a JavaScript file and prints its result, how long
// the evaluation took and what memory the VM held. Built with -tags cengine
// it runs the C engine generated into internal/cengine. Built without, it
// runs the engine of package slowjs, so that one file gives the same
// measurement on both:
//
//	go run -tags cengine ./internal/cengine/cmd/jsrun -n 20 fib.js
//	go run ./internal/cengine/cmd/jsrun -n 20 fib.js
//
// Each run makes a new VM, evaluates the file in it (the global script and
// the jobs it queued) and closes the VM. Only the evaluation is timed: the
// call that copies the source into the VM, runs the script and its jobs and
// turns the result into a string, which is all one call of an engine here.
// The source is a string before the first run, so reading the file and
// converting its bytes are outside the time, as are making, collecting,
// releasing and closing the VM. The output says so in its "timed:" line. The
// result is String() of the script's completion value, and every run must
// give the same one.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"slices"
	"time"
)

// vm is what the runner needs of an engine.
type vm interface {
	eval(src, file string) (string, error)
	gc()
	// release gives free memory back to the OS and describes what it gave.
	release(keep uintptr) (string, error)
	// memory describes what the VM holds.
	memory() string
	// close frees the VM and describes what it left, with an error when it
	// leaked.
	close() (string, error)
}

type options struct {
	seed  uint64
	clock time.Time // zero: the real clock
	stack uintptr
}

func main() {
	n := flag.Int("n", 1, "runs, each in a new VM")
	warm := flag.Int("warm", 0, "runs before the measured ones")
	seed := flag.Uint64("seed", 1, "Math.random seed")
	clock := flag.String("clock", "", "fixed wall clock, RFC 3339 (empty: the real clock)")
	stack := flag.Uint64("stack", 0, "shadow stack bytes of the C engine (0: its default)")
	gc := flag.Bool("gc", false, "run a full GC after each evaluation")
	release := flag.Int64("release", -1, "after each evaluation give free memory back, keeping this many bytes (-1: do not)")
	mem := flag.Bool("mem", false, "print the memory of the VM after each run")
	goheap := flag.Bool("goheap", false, "print the live Go heap after each run")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: jsrun [flags] file.js\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 || *n < 1 || *warm < 0 {
		flag.Usage()
		os.Exit(2)
	}
	file := flag.Arg(0)
	bytes, err := os.ReadFile(file)
	if err != nil {
		fatal(err)
	}
	// The conversion is not part of any run.
	src := string(bytes)
	o := options{seed: *seed, stack: uintptr(*stack)}
	if *clock != "" {
		if o.clock, err = time.Parse(time.RFC3339, *clock); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("engine: %s\n", engineName)
	fmt.Printf("timed: %s, not timed: reading the file, making, collecting, releasing and closing the VM\n", timedWhat)
	var times []time.Duration
	var result string
	for i := range *warm + *n {
		measured := i >= *warm
		v, err := newVM(o)
		if err != nil {
			fatal(err)
		}
		t0 := time.Now()
		r, err := v.eval(src, file)
		d := time.Since(t0)
		if err != nil {
			fatal(fmt.Errorf("run %d: %w", i+1, err))
		}
		if i == 0 {
			result = r
			fmt.Printf("result: %s\n", r)
		} else if r != result {
			fatal(fmt.Errorf("run %d gave %q, the first gave %q", i+1, r, result))
		}
		if measured {
			times = append(times, d)
		}
		if *mem {
			fmt.Printf("run %d after eval: %s\n", i+1, v.memory())
		}
		if *gc {
			v.gc()
			if *mem {
				fmt.Printf("run %d after gc: %s\n", i+1, v.memory())
			}
		}
		if *release >= 0 {
			s, err := v.release(uintptr(*release))
			if err != nil {
				fatal(err)
			}
			if *mem {
				fmt.Printf("run %d release: %s\n", i+1, s)
				fmt.Printf("run %d after release: %s\n", i+1, v.memory())
			}
		}
		s, err := v.close()
		if err != nil {
			fatal(fmt.Errorf("run %d: %w", i+1, err))
		}
		if *mem {
			fmt.Printf("run %d close: %s\n", i+1, s)
		}
		if *goheap {
			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			fmt.Printf("run %d go heap: %d bytes live in %d objects\n", i+1, ms.HeapAlloc, ms.HeapObjects)
		}
	}
	slices.Sort(times)
	var sum time.Duration
	for _, d := range times {
		sum += d
	}
	fmt.Printf("eval: %d runs, min %v, median %v, mean %v, max %v\n", len(times),
		times[0], times[len(times)/2], sum/time.Duration(len(times)), times[len(times)-1])
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "jsrun:", err)
	os.Exit(1)
}
