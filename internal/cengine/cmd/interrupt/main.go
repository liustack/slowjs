// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command interrupt times how long a script takes to stop after Interrupt,
// for the scripts of package scenarios, each of which stays inside one long
// operation of the engine.
//
//	go run ./internal/cengine/cmd/interrupt -n 20
//
// For each scenario it makes -n VMs one after the other. The script runs for
// -warm plus a random time up to -spread, so that the Interrupt falls at
// different places of the operation. The line of the scenario gives the
// median and the highest time from Interrupt to the return of Eval, and how
// many times the C engine did not stop within its grace and killed the VM
// (the time is then about the grace, 50 ms, and the VM is good only for
// Close). A scenario whose script ends without an error fails the run.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/cengine/cmd/scenarios"
)

func main() {
	n := flag.Int("n", 10, "VMs, so Interrupts, for each scenario")
	warm := flag.Duration("warm", 1500*time.Millisecond, "least time a script runs before the Interrupt")
	spread := flag.Duration("spread", 300*time.Millisecond, "most time added to warm at random")
	only := flag.String("only", "", "comma separated scenario names (default all)")
	label := flag.String("label", "", "first field of each line")
	profile := flag.String("cpuprofile", "", "write a CPU profile of the whole run here (pprof names the functions of the C engine)")
	stopProfile := flag.String("stopprofile", "", "directory for one CPU profile of each trial, from the Interrupt to the return of Eval, at 1000 Hz: merge them with go tool pprof to see where the engine spends the time it takes to stop")
	flag.Parse()
	if *stopProfile != "" {
		if err := os.MkdirAll(*stopProfile, 0o755); err != nil {
			fail(err)
		}
		runtime.SetCPUProfileRate(1000)
	}
	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			fail(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fail(err)
		}
		defer pprof.StopCPUProfile()
	}
	want := map[string]bool{}
	for _, name := range strings.Split(*only, ",") {
		if name != "" {
			want[name] = true
		}
	}
	fmt.Println("label\tscenario\ttrials\tmedian_ms\tmax_ms\tkilled")
	for _, sc := range scenarios.All {
		if len(want) > 0 && !want[sc.Name] {
			continue
		}
		var times []float64
		killed := 0
		for trial := range *n {
			m, err := slowjs.NewVM()
			if err != nil {
				fail(err)
			}
			done := make(chan error, 1)
			go func() {
				_, err := m.Eval(sc.JS, slowjs.EvalGlobal)
				done <- err
			}()
			time.Sleep(*warm + time.Duration(rand.Int64N(int64(*spread)+1)))
			var prof *os.File
			if *stopProfile != "" {
				var err error
				if prof, err = os.Create(filepath.Join(*stopProfile, fmt.Sprintf("%s-%03d.pprof", sc.Name, trial))); err != nil {
					fail(err)
				}
				if err := pprof.StartCPUProfile(prof); err != nil {
					fail(err)
				}
			}
			begin := time.Now()
			m.Interrupt()
			var err2 error
			select {
			case err2 = <-done:
			case <-time.After(time.Minute):
				fail(fmt.Errorf("%s: the script did not stop a minute after Interrupt", sc.Name))
			}
			times = append(times, float64(time.Since(begin))/1e6)
			if prof != nil {
				pprof.StopCPUProfile()
				prof.Close()
			}
			if err2 == nil {
				fail(fmt.Errorf("%s: the script ended without an error", sc.Name))
			}
			if errors.Is(err2, slowjs.ErrKilled) {
				killed++
			}
			if err := m.Close(); err != nil {
				fail(err)
			}
		}
		slices.Sort(times)
		fmt.Printf("%s\t%s\t%d\t%.2f\t%.2f\t%d\n", *label, sc.Name, len(times), times[len(times)/2], times[len(times)-1], killed)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
