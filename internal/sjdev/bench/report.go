// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strings"
)

// The rule of "Code positions" in docs/performance.md.
const (
	// MinChange is the smallest change that can count: 2%.
	MinChange = 0.02
	// CallgrindBelow is the change under which timings do not argue: 3%.
	CallgrindBelow = 0.03
)

// Median returns the middle value (the mean of the two middle ones for an even count).
func Median(x []float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	s := slices.Clone(x)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// Row is the comparison of one metric.
type Row struct {
	Metric    string
	Base, New float64            // median over positions of the median over rounds
	Ratio     float64            // New / Base, the median over positions of the median over rounds of the paired ratios
	Range     float64            // the spread of Base over its own positions: max / min - 1
	PerPos    map[string]float64 // the ratio at each position
	Agree     int                // positions on the side of 1 that Ratio is on
	Positions int
	Rounds    int // pairs behind the thinnest position
	Verdict   string
	Callgrind bool // the change is too small to argue from timings
}

// Verdict applies the rule: a change is real only when its size passes 2% and
// half of the range of the base's own positions, and most positions agree on
// the sign.
func Verdict(ratio, rng float64, agree, positions int) string {
	d := ratio - 1
	// A change of exactly 2%, or exactly half the range, is not past it: the tolerance keeps
	// the rounding of 1.02 - 1 from deciding.
	const eps = 1e-9
	if math.Abs(d) > MinChange+eps && math.Abs(d) > rng/2+eps && agree*2 > positions {
		if d < 0 {
			return "faster"
		}
		return "slower"
	}
	return "no change"
}

// Compare reads the records of one kind and compares side nw with side base
// for every metric, and for the sum of the metrics when there are several.
func Compare(rs []Record, kind, base, nw string) ([]Row, error) {
	// by[pos][round][side] is the metrics of the good process.
	by := map[string]map[int]map[string]map[string]float64{}
	names := map[string]bool{}
	for _, r := range rs {
		if r.Kind != kind || r.Err != "" || r.Metrics == nil {
			continue
		}
		if by[r.Pos] == nil {
			by[r.Pos] = map[int]map[string]map[string]float64{}
		}
		if by[r.Pos][r.Round] == nil {
			by[r.Pos][r.Round] = map[string]map[string]float64{}
		}
		// A later record of the same process (a rerun after a disturbed one) replaces the earlier.
		by[r.Pos][r.Round][r.Side] = r.Metrics
		for n := range r.Metrics {
			names[n] = true
		}
	}
	if len(by) == 0 {
		return nil, fmt.Errorf("no records of kind %q", kind)
	}
	var metrics []string
	for n := range names {
		metrics = append(metrics, n)
	}
	sort.Strings(metrics)
	if len(metrics) > 1 {
		metrics = append(metrics, "sum")
	}
	var positions []string
	for p := range by {
		positions = append(positions, p)
	}
	sort.Strings(positions)
	val := func(m map[string]float64, name string) (float64, bool) {
		if name != "sum" {
			v, ok := m[name]
			return v, ok
		}
		s := 0.0
		for _, v := range m {
			s += v
		}
		return s, true
	}
	var rows []Row
	for _, name := range metrics {
		row := Row{Metric: name, PerPos: map[string]float64{}, Rounds: math.MaxInt}
		var ratios, bases, news []float64
		for _, p := range positions {
			var rr, bs, ns []float64
			for _, sides := range by[p] {
				b, okb := sides[base]
				x, okn := sides[nw]
				if !okb || !okn {
					continue
				}
				bv, ok1 := val(b, name)
				nv, ok2 := val(x, name)
				if !ok1 || !ok2 {
					continue
				}
				rr = append(rr, nv/bv)
				bs = append(bs, bv)
				ns = append(ns, nv)
			}
			if len(rr) == 0 {
				continue
			}
			row.PerPos[p] = Median(rr)
			ratios = append(ratios, row.PerPos[p])
			bases = append(bases, Median(bs))
			news = append(news, Median(ns))
			row.Rounds = min(row.Rounds, len(rr))
		}
		if len(ratios) == 0 {
			continue
		}
		row.Positions = len(ratios)
		row.Ratio = Median(ratios)
		row.Base, row.New = Median(bases), Median(news)
		row.Range = slices.Max(bases)/slices.Min(bases) - 1
		for _, r := range ratios {
			if r != 1 && (r < 1) == (row.Ratio < 1) {
				row.Agree++
			}
		}
		row.Verdict = Verdict(row.Ratio, row.Range, row.Agree, row.Positions)
		row.Callgrind = math.Abs(row.Ratio-1) < CallgrindBelow
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sides %q and %q share no process of kind %q", base, nw, kind)
	}
	return rows, nil
}

// WriteTable writes the rows as a Markdown table.
func WriteTable(w io.Writer, rows []Row, base, nw string, detail bool) {
	fmt.Fprintf(w, "| metric | %s | %s | ratio | range | agree | rounds | verdict |\n", base, nw)
	fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |")
	for _, r := range rows {
		verdict := r.Verdict
		if r.Callgrind {
			verdict += ", under 3% so see callgrind"
		}
		fmt.Fprintf(w, "| %s | %.4g | %.4g | %.3f | %.1f%% | %d/%d | %d | %s |\n", r.Metric, r.Base, r.New, r.Ratio, r.Range*100, r.Agree, r.Positions, r.Rounds, verdict)
	}
	if !detail {
		return
	}
	fmt.Fprintln(w)
	var ps []string
	for p := range rows[0].PerPos {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	fmt.Fprintf(w, "| metric | %s |\n| --- |%s\n", strings.Join(ps, " | "), strings.Repeat(" ---: |", len(ps)))
	for _, r := range rows {
		cells := make([]string, len(ps))
		for i, p := range ps {
			if v, ok := r.PerPos[p]; ok {
				cells[i] = fmt.Sprintf("%.3f", v)
			}
		}
		fmt.Fprintf(w, "| %s | %s |\n", r.Metric, strings.Join(cells, " | "))
	}
}

// MemRow is the peak memory of one side.
type MemRow struct {
	Side             string
	Median, Min, Max float64 // MiB
	N                int
}

// Final returns the record that counts for each process of the kind: the last
// try of a side at a position in a round.
func Final(rs []Record, kind string) []Record {
	type k struct {
		side, pos string
		round     int
	}
	idx := map[k]int{}
	var out []Record
	for _, r := range rs {
		if r.Kind != kind || r.Err != "" {
			continue
		}
		key := k{r.Side, r.Pos, r.Round}
		if i, ok := idx[key]; ok {
			out[i] = r
			continue
		}
		idx[key] = len(out)
		out = append(out, r)
	}
	return out
}

// Disturbed counts the processes whose last try still ran with the sibling core
// or other work busy.
func Disturbed(rs []Record, kind string) int {
	n := 0
	for _, r := range Final(rs, kind) {
		if r.Disturbed {
			n++
		}
	}
	return n
}

// Memory returns the peak memory (the VmHWM each process reported) per side.
func Memory(rs []Record, kind string) []MemRow {
	by := map[string][]float64{}
	for _, r := range Final(rs, kind) {
		if r.HWMKB > 0 {
			by[r.Side] = append(by[r.Side], float64(r.HWMKB)/1024)
		}
	}
	var rows []MemRow
	for s, v := range by {
		rows = append(rows, MemRow{s, Median(v), slices.Min(v), slices.Max(v), len(v)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Side < rows[j].Side })
	return rows
}

// WriteMemory writes the memory table.
func WriteMemory(w io.Writer, rows []MemRow) {
	fmt.Fprintln(w, "| side | peak MiB (median) | min | max | processes |")
	fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: |")
	for _, r := range rows {
		fmt.Fprintf(w, "| %s | %.1f | %.1f | %.1f | %d |\n", r.Side, r.Median, r.Min, r.Max, r.N)
	}
}
