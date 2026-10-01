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

func main() {
	b := bytes.NewBuffer(nil)
	io.Copy(io.MultiWriter(os.Stdout, b), os.Stdin)
	m := map[string]map[string]float64{}
	a := strings.Split(b.String(), "\n")
	var args []string
	for _, v := range a {
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
		t, _ := strconv.ParseFloat(f[2], 4)
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
	fmt.Printf("%35s", "arg")
	for _, v := range benches {
		fmt.Printf("%12s", v)
	}
	hr := strings.Repeat("-", 35+12*(len(benches)))
	fmt.Printf("\n%s\n", hr)
	nums := map[string][]float64{}
	for _, arg := range args {
		fmt.Printf("%35s", arg)
		var t1 float64
		for i, bench := range benches {
			t := m[bench][arg]
			if i == 0 {
				t1 = t
			}
			k := t / t1
			nums[bench] = append(nums[bench], k)
			fmt.Printf("%12.3f", k)
		}
		fmt.Println()
	}
	fmt.Printf("%s\n%35s", hr, "geomean")
	for _, bench := range benches {
		fmt.Printf("%12.3f", geoMean(nums[bench]))
	}
	fmt.Println()
	fmt.Printf("%35s", "")
	for _, v := range benches {
		fmt.Printf("%12s", v)
	}
	fmt.Println()
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
