// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command behind lists the items where slowjs needs at least 2 times the time
// of the median of the other pure Go engines (for the V8 suite: half the
// score). It reads the results file of a run of cmd/compare.
//
//	go run ./cmd/behind results/<run>.json [limit]
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// results is the part of a results file that the list reads. A round of a
// workload stays raw until the workload is known.
type results struct {
	Engines []struct {
		Engine string `json:"engine"`
	} `json:"engines"`
	Timed map[string]map[string][]json.RawMessage `json:"timed"`
}

type row struct {
	kind, name string
	ratio      float64
	ours       float64
	median     float64
	best       string
	bestValue  float64
	n          int
}

func median(xs []float64) float64 {
	s := slices.Clone(xs)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// rounds returns the rounds of engine e in workload w. The workload must be in
// the file.
func (r *results) rounds(w, e string) ([]json.RawMessage, error) {
	t, ok := r.Timed[w]
	if !ok {
		return nil, fmt.Errorf("the results have no workload %q", w)
	}
	return t[e], nil
}

// perTest returns, for engine e, the median over its rounds of the number
// each round gives per test: ns per iteration of a micro benchmark, or the
// score of a V8 benchmark.
func (r *results) perTest(w, e string) (map[string]float64, error) {
	rs, err := r.rounds(w, e)
	if err != nil {
		return nil, err
	}
	all := map[string][]float64{}
	for _, raw := range rs {
		switch w {
		case "upstream":
			var rd struct {
				Tests map[string]struct {
					NsPerN *float64 `json:"ns_per_n"`
				} `json:"tests"`
			}
			if err := json.Unmarshal(raw, &rd); err != nil {
				return nil, err
			}
			for k, v := range rd.Tests {
				if v.NsPerN != nil {
					all[k] = append(all[k], *v.NsPerN)
				}
			}
		case "v8":
			var rd struct {
				Scores map[string]any `json:"scores"`
			}
			if err := json.Unmarshal(raw, &rd); err != nil {
				return nil, err
			}
			for k, v := range rd.Scores {
				switch x := v.(type) {
				case float64:
					all[k] = append(all[k], x)
				case string:
					if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
						all[k] = append(all[k], f)
					}
				default:
					return nil, fmt.Errorf("score %s of %s is %v", k, e, v)
				}
			}
		}
	}
	out := map[string]float64{}
	for k, v := range all {
		out[k] = median(v)
	}
	return out, nil
}

// cpu returns the median CPU per operation of the rounds that worked, and
// whether there is one.
func (r *results) cpu(w, e string) (float64, bool, error) {
	rs, err := r.rounds(w, e)
	if err != nil {
		return 0, false, err
	}
	var v []float64
	for _, raw := range rs {
		var x struct {
			OK  bool     `json:"ok"`
			CPU *float64 `json:"cpu_ms_per_op"`
		}
		if err := json.Unmarshal(raw, &x); err != nil {
			return 0, false, err
		}
		if !x.OK {
			continue
		}
		if x.CPU == nil {
			return 0, false, fmt.Errorf("a round of %s in %s has no cpu_ms_per_op", e, w)
		}
		v = append(v, *x.CPU)
	}
	if len(v) == 0 {
		return 0, false, nil
	}
	return median(v), true, nil
}

// behind returns the rows with a ratio of at least limit, the worst first.
func behind(data []byte, limit float64) ([]row, error) {
	var r results
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	var names []string
	for _, e := range r.Engines {
		names = append(names, e.Engine)
	}
	// Results files made before 2026-10-24 name the row liustack-quickjs, the
	// module's name until v0.28.0.
	me := "liustack-quickjs"
	if slices.Contains(names, "slowjs") || !slices.Contains(names, "liustack-quickjs") {
		me = "slowjs"
	}
	var others []string
	for _, e := range names {
		if e != me {
			others = append(others, e)
		}
	}
	var rows []row
	for _, w := range []string{"upstream", "v8"} {
		mine, err := r.perTest(w, me)
		if err != nil {
			return nil, err
		}
		per := map[string]map[string]float64{}
		for _, e := range others {
			if per[e], err = r.perTest(w, e); err != nil {
				return nil, err
			}
		}
		keys := make([]string, 0, len(mine))
		for k := range mine {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			var have []string
			var vals []float64
			for _, e := range others {
				if v, ok := per[e][k]; ok {
					have = append(have, e)
					vals = append(vals, v)
				}
			}
			if w == "upstream" {
				if len(have) < 2 {
					continue
				}
				m := median(vals)
				rows = append(rows, newRow("micro", k, mine[k]/m, mine[k], m, have, vals, false))
			} else {
				if len(have) == 0 {
					return nil, fmt.Errorf("no other engine has a V8 score for %s", k)
				}
				m := median(vals)
				rows = append(rows, newRow("v8", k, m/mine[k], mine[k], m, have, vals, true))
			}
		}
	}
	for _, w := range []string{"react", "micro/json-parse", "micro/json-stringify", "micro/regexp", "micro/string-concat"} {
		mine, ok, err := r.cpu(w, me)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s has no CPU time in workload %s", me, w)
		}
		var have []string
		var vals []float64
		for _, e := range others {
			v, ok, err := r.cpu(w, e)
			if err != nil {
				return nil, err
			}
			if ok && v != 0 {
				have = append(have, e)
				vals = append(vals, v)
			}
		}
		if len(have) == 0 {
			return nil, fmt.Errorf("no other engine has a CPU time in workload %s", w)
		}
		m := median(vals)
		rows = append(rows, newRow("workload", w, mine/m, mine, m, have, vals, false))
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ratio > rows[j].ratio })
	var out []row
	for _, x := range rows {
		if x.ratio >= limit {
			out = append(out, x)
		}
	}
	return out, nil
}

// newRow picks the best of the other engines: the lowest value, or for a
// score the highest. The first of equals wins.
func newRow(kind, name string, ratio, ours, median float64, have []string, vals []float64, higher bool) row {
	best := 0
	for i, v := range vals {
		if (higher && v > vals[best]) || (!higher && v < vals[best]) {
			best = i
		}
	}
	return row{kind, name, ratio, ours, median, have[best], vals[best], len(have)}
}

// g4 is Python's "%.4g".
func g4(x float64) string {
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return strings.ToLower(strconv.FormatFloat(x, 'g', -1, 64))
	}
	return strconv.FormatFloat(x, 'g', 4, 64)
}

func (x row) String() string {
	return fmt.Sprintf("%-8s %-28s %5.2f  ours %s  median %s  best %s %s  n=%d",
		x.kind, x.name, x.ratio, g4(x.ours), g4(x.median), x.best, g4(x.bestValue), x.n)
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: behind results/<run>.json [limit]")
		os.Exit(2)
	}
	limit := 2.0
	if len(os.Args) == 3 {
		var err error
		if limit, err = strconv.ParseFloat(os.Args[2], 64); err != nil {
			fmt.Fprintln(os.Stderr, "behind:", err)
			os.Exit(2)
		}
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "behind:", err)
		os.Exit(1)
	}
	rows, err := behind(data, limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "behind:", err)
		os.Exit(1)
	}
	for _, x := range rows {
		fmt.Println(x)
	}
}
