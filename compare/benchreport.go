// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Exit status 1 when the output is not the complete result of a run: a line of
// a failed benchmark or package, no result at all, a result that is not a
// positive finite number, or an engine that lacks an argument another has.
func main() {
	if err := report(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "benchreport:", err)
		os.Exit(1)
	}
}

func report(in io.Reader, out io.Writer) error {
	b := bytes.NewBuffer(nil)
	io.Copy(b, in)
	m := map[string]map[string]float64{}
	a := strings.Split(b.String(), "\n")
	var args []string
	for _, v := range a {
		if strings.HasPrefix(v, "FAIL") || strings.HasPrefix(v, "--- FAIL") || strings.HasPrefix(v, "panic:") {
			return fmt.Errorf("the run failed: %s", v)
		}
		if !strings.HasPrefix(v, "Benchmark") {
			continue
		}

		v = v[len("Benchmark"):]
		f := strings.Fields(v)
		if len(f) < 3 {
			continue
		}

		k := strings.SplitN(f[0], "/", 2)
		if len(k) != 2 {
			continue
		}

		bench, arg := k[0], k[1]
		args = append(args, arg)
		t, err := strconv.ParseFloat(f[2], 64)
		if err != nil || t <= 0 || math.IsInf(t, 0) || math.IsNaN(t) {
			return fmt.Errorf("%s: %q is not a positive time", f[0], f[2])
		}
		m2 := m[bench]
		if m2 == nil {
			m2 = map[string]float64{}
			m[bench] = m2
		}
		m2[arg] = t
	}
	var benches []string
	for bench := range m {
		benches = append(benches, bench)
	}
	slices.Sort(benches)
	slices.Sort(args)
	args = slices.Compact(args)
	if len(benches) == 0 {
		return fmt.Errorf("no benchmark result in the input")
	}
	for _, bench := range benches {
		for _, arg := range args {
			if _, ok := m[bench][arg]; !ok {
				return fmt.Errorf("%s has no result for %s", bench, arg)
			}
		}
	}
	fmt.Fprintf(out, "%35s", "arg")
	for _, v := range benches {
		fmt.Fprintf(out, "%12s", v)
	}
	hr := strings.Repeat("-", 35+12*(len(benches)))
	fmt.Fprintf(out, "\n%s\n", hr)
	nums := map[string][]float64{}
	for _, arg := range args {
		fmt.Fprintf(out, "%35s", arg)
		var t1 float64
		for i, bench := range benches {
			t := m[bench][arg]
			if i == 0 {
				t1 = t
			}
			k := t / t1
			nums[bench] = append(nums[bench], k)
			fmt.Fprintf(out, "%12.3f", k)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s\n%35s", hr, "geomean")
	for _, bench := range benches {
		fmt.Fprintf(out, "%12.3f", geoMean(nums[bench]))
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%35s", "")
	for _, v := range benches {
		fmt.Fprintf(out, "%12s", v)
	}
	fmt.Fprintln(out)
	return nil
}

func geoMean(s []float64) float64 {
	if len(s) == 0 {
		return 0.0
	}

	logSum := 0.0
	for _, num := range s {
		if num == 0 {
			return 0.0
		}

		logSum += math.Log(num)
	}

	return math.Exp(logSum / float64(len(s)))
}
