// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build ignore

// Benchreport reads the output of go test -bench in compare and writes, for
// each script, the time of each engine as a multiple of the first engine's,
// and their geometric mean.
//
//	go run benchreport.go < bench.out
//
// It refuses the output of a run that failed or is missing results, so that
// a table is never made of half a run.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

func main() {
	if err := report(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "benchreport:", err)
		os.Exit(1)
	}
}

// report writes the table of the benchmark output in.
func report(in io.Reader, out io.Writer) error {
	// times[engine][script] is the time of one run, in ns.
	times := map[string]map[string]float64{}
	sc := bufio.NewScanner(in)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "FAIL") || strings.HasPrefix(line, "--- FAIL") || strings.HasPrefix(line, "panic:") {
			return fmt.Errorf("the run failed: %s", line)
		}

		rest, ok := strings.CutPrefix(line, "Benchmark")
		if !ok {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) < 3 {
			continue
		}

		engine, script, ok := strings.Cut(fields[0], "/")
		if !ok {
			continue
		}

		t, err := strconv.ParseFloat(fields[2], 64)
		if err != nil || !(t > 0) || math.IsInf(t, 0) {
			return fmt.Errorf("%s: %q is not a positive time", fields[0], fields[2])
		}

		if times[engine] == nil {
			times[engine] = map[string]float64{}
		}
		times[engine][script] = t
	}
	if err := sc.Err(); err != nil {
		return err
	}

	if len(times) == 0 {
		return errors.New("no benchmark result in the input")
	}

	var engines, scripts []string
	for e, m := range times {
		engines = append(engines, e)
		for s := range m {
			scripts = append(scripts, s)
		}
	}
	slices.Sort(engines)
	slices.Sort(scripts)
	scripts = slices.Compact(scripts)
	for _, e := range engines {
		for _, s := range scripts {
			if _, ok := times[e][s]; !ok {
				return fmt.Errorf("%s has no result for %s", e, s)
			}
		}
	}

	w := bufio.NewWriter(out)
	fmt.Fprintf(w, "%35s", "arg")
	for _, e := range engines {
		fmt.Fprintf(w, "%12s", e)
	}
	fmt.Fprintln(w)
	rule := strings.Repeat("-", 35+12*len(engines))
	fmt.Fprintln(w, rule)

	ratios := make([][]float64, len(engines))
	base := times[engines[0]]
	for _, s := range scripts {
		fmt.Fprintf(w, "%35s", s)
		for i, e := range engines {
			r := times[e][s] / base[s]
			ratios[i] = append(ratios[i], r)
			fmt.Fprintf(w, "%12.3f", r)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, rule)
	fmt.Fprintf(w, "%35s", "geomean")
	for i := range engines {
		fmt.Fprintf(w, "%12.3f", geomean(ratios[i]))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%35s", "")
	for _, e := range engines {
		fmt.Fprintf(w, "%12s", e)
	}
	fmt.Fprintln(w)
	return w.Flush()
}

// geomean is the geometric mean of l, 0 for an empty list or one with a 0.
func geomean(l []float64) float64 {
	if len(l) == 0 {
		return 0
	}

	sum := 0.0
	for _, x := range l {
		if x == 0 {
			return 0
		}
		sum += math.Log(x)
	}
	return math.Exp(sum / float64(len(l)))
}
