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

// The defaults of docs/measuring.md.
const (
	// DefaultDelta is the regression that matters: 1%, about the smallest
	// change to the engine that 2 rounds over the 16 layouts can show.
	DefaultDelta = 0.01
	// DefaultAlpha makes the intervals 95% two-sided.
	DefaultAlpha = 0.05
)

// Pair is one block's comparison of two sides: the log of new/base.
type Pair struct {
	Pos      string
	Round    int
	Rep      int
	Y        float64 // ln(new/base) of the metric
	NewFirst bool    // the new side ran before the base in the block
}

// Validation is what reading the records found wrong or left out.
type Validation struct {
	Blocks    int      // blocks of the kind with both sides in their last attempt
	Disturbed int      // of those, blocks still disturbed after all tries: left out
	Reruns    int      // attempts that were run again
	Problems  []string // what makes the records unusable
}

// Pairs reads the records of one kind and returns the pairs of base and nw for
// the metric ("sum" adds every metric of the process). It checks the records:
// a process that failed, two records of the same process, a block that lacks a
// side, and processes that report different metrics are problems. Disturbed
// blocks are left out and counted.
func Pairs(rs []Record, kind, base, nw, metric string) ([]Pair, Validation) {
	var v Validation
	type bk struct {
		pos        string
		round, rep int
	}
	type pk struct {
		b       bk
		attempt int
		side    string
	}
	seen := map[pk]Record{}
	last := map[bk]int{}
	var names []string
	for _, r := range rs {
		if r.Kind != kind || (r.Side != base && r.Side != nw) {
			continue
		}
		b := bk{r.Pos, r.Round, r.Rep}
		k := pk{b, r.Attempt, r.Side}
		if _, dup := seen[k]; dup {
			v.Problems = append(v.Problems, fmt.Sprintf("%s@%s round %d rep %d attempt %d is in the records twice (sessions %q and %q)", r.Side, r.Pos, r.Round, r.Rep, r.Attempt, seen[k].Session, r.Session))
			continue
		}
		seen[k] = r
		if a, ok := last[b]; !ok || r.Attempt > a {
			last[b] = r.Attempt
		}
	}
	var blocks []bk
	for b := range last {
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool {
		x, y := blocks[i], blocks[j]
		if x.round != y.round {
			return x.round < y.round
		}
		if x.pos != y.pos {
			return x.pos < y.pos
		}
		return x.rep < y.rep
	})
	val := func(m map[string]float64) float64 {
		if metric != "sum" {
			x, ok := m[metric]
			if !ok {
				return math.NaN()
			}
			return x
		}
		s := 0.0
		for _, x := range m {
			s += x
		}
		return s
	}
	var ps []Pair
	for _, b := range blocks {
		a := last[b]
		v.Reruns += a
		rb, okb := seen[pk{b, a, base}]
		rn, okn := seen[pk{b, a, nw}]
		if !okb || !okn {
			missing := base
			if okb {
				missing = nw
			}
			v.Problems = append(v.Problems, fmt.Sprintf("block %s round %d rep %d attempt %d has no record of %s", b.pos, b.round, b.rep, a, missing))
			continue
		}
		bad := false
		for _, r := range []Record{rb, rn} {
			if r.Err != "" {
				v.Problems = append(v.Problems, fmt.Sprintf("%s@%s round %d failed: %s", r.Side, r.Pos, r.Round, trunc(r.Err, 200)))
				bad = true
				continue
			}
			ks := make([]string, 0, len(r.Metrics))
			for k := range r.Metrics {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			if names == nil {
				names = ks
			} else if !slices.Equal(names, ks) {
				v.Problems = append(v.Problems, fmt.Sprintf("%s@%s round %d reports the metrics %v, others %v", r.Side, r.Pos, r.Round, ks, names))
				bad = true
			}
		}
		if rb.FreshPages != rn.FreshPages {
			v.Problems = append(v.Problems, fmt.Sprintf("block %s round %d: one side ran with fresh pages and the other without", b.pos, b.round))
			continue
		}
		if bad {
			continue
		}
		v.Blocks++
		if rb.Disturbed || rn.Disturbed {
			v.Disturbed++
			continue
		}
		x, y := val(rb.Metrics), val(rn.Metrics)
		if !(x > 0) || !(y > 0) {
			v.Problems = append(v.Problems, fmt.Sprintf("block %s round %d: metric %s is %v and %v", b.pos, b.round, metric, x, y))
			continue
		}
		ps = append(ps, Pair{Pos: b.pos, Round: b.round, Rep: b.rep, Y: math.Log(y / x), NewFirst: rn.Slot < rb.Slot})
	}
	if len(ps) == 0 && len(v.Problems) == 0 {
		v.Problems = append(v.Problems, fmt.Sprintf("no block of kind %q has both %s and %s", kind, base, nw))
	}
	return ps, v
}

// Analysis is the result of a comparison on one metric. Every value is on the
// log scale: exp(x)-1 is the change.
type Analysis struct {
	Metric  string
	Pairs   int
	Layouts int
	// Formal is the formal layout alone, from its own pairs.
	Formal Interval
	// Summary is the mean over the layouts, each with the same weight, with the
	// interval that treats the layouts as a sample of the layouts a change can
	// meet (the spread of the layout means). This is the one that decides.
	Summary Interval
	// Fixed is the same mean with the interval of the run-to-run error only: how
	// well these very layouts were measured.
	Fixed Interval
	// RunSD is the run-to-run error of one pair, pooled within the layouts.
	RunSD float64
	// LayoutSD is the spread of the true effect between layouts, after taking
	// out what the run-to-run error explains (0 when it explains it all).
	LayoutSD float64
	// Order is mean(y | new ran first) - mean(y | base ran first), with its SE.
	Order, OrderSE float64
	PerLayout      map[string][]float64
	Verdict        string // of Summary
	FormalVerdict  string
}

// Analyze computes the analysis of the pairs with intervals at 1-alpha and the
// regression limit delta (a fraction, such as 0.005).
func Analyze(metric string, ps []Pair, alpha, delta float64) Analysis {
	a := Analysis{Metric: metric, Pairs: len(ps), PerLayout: map[string][]float64{}}
	var first, second []float64
	for _, p := range ps {
		a.PerLayout[p.Pos] = append(a.PerLayout[p.Pos], p.Y)
		if p.NewFirst {
			first = append(first, p.Y)
		} else {
			second = append(second, p.Y)
		}
	}
	var layouts []string
	for l := range a.PerLayout {
		layouts = append(layouts, l)
	}
	sort.Strings(layouts)
	a.Layouts = len(layouts)
	var means []float64
	ssw, dfw, invn := 0.0, 0.0, 0.0
	for _, l := range layouts {
		y := a.PerLayout[l]
		means = append(means, Mean(y))
		if len(y) > 1 {
			ssw += Var(y) * float64(len(y)-1)
			dfw += float64(len(y) - 1)
		}
		invn += 1 / float64(len(y))
	}
	invn /= float64(len(layouts))
	a.RunSD = math.NaN()
	if dfw > 0 {
		a.RunSD = math.Sqrt(ssw / dfw)
	}
	if f := a.PerLayout[Formal.String()]; len(f) > 0 {
		a.Formal = TInterval(Mean(f), math.Sqrt(Var(f)/float64(len(f))), float64(len(f)-1), alpha)
	} else {
		a.Formal = TInterval(math.NaN(), math.NaN(), 0, alpha)
	}
	if len(means) > 0 {
		m := Mean(means)
		L := float64(len(means))
		a.Summary = TInterval(m, math.Sqrt(Var(means)/L), L-1, alpha)
		a.Fixed = TInterval(m, a.RunSD*math.Sqrt(invn/L), dfw, alpha)
		if len(means) > 1 && dfw > 0 {
			a.LayoutSD = math.Sqrt(max(0, Var(means)-a.RunSD*a.RunSD*invn))
		} else {
			a.LayoutSD = math.NaN()
		}
	}
	a.Order, a.OrderSE = math.NaN(), math.NaN()
	if len(first) > 1 && len(second) > 1 {
		a.Order = Mean(first) - Mean(second)
		a.OrderSE = math.Sqrt(Var(first)/float64(len(first)) + Var(second)/float64(len(second)))
	}
	a.Verdict = Decide(a.Summary, delta)
	a.FormalVerdict = Decide(a.Formal, delta)
	return a
}

// The verdicts.
const (
	VerdictFaster       = "faster"
	VerdictSlower       = "slower"
	VerdictSlowerSmall  = "slower, by less than delta"
	VerdictNoRegression = "no regression beyond delta"
	VerdictInconclusive = "inconclusive"
	VerdictNone         = "not measured"
)

// Decide reads an interval of the log ratio against the regression limit delta:
// faster when it is all below 0, slower when it is all above 0, no regression
// beyond delta when it holds 0 and stays below ln(1+delta), and inconclusive
// when it reaches past ln(1+delta).
func Decide(iv Interval, delta float64) string {
	if math.IsNaN(iv.Lo) || math.IsNaN(iv.Hi) {
		return VerdictNone
	}
	lim := math.Log1p(delta)
	switch {
	case iv.Hi < 0:
		return VerdictFaster
	case iv.Lo > 0 && iv.Hi < lim:
		return VerdictSlowerSmall
	case iv.Lo > 0:
		return VerdictSlower
	case iv.Hi < lim:
		return VerdictNoRegression
	}
	return VerdictInconclusive
}

func pct(x float64) string {
	if math.IsNaN(x) {
		return "-"
	}
	return fmt.Sprintf("%+.2f%%", 100*math.Expm1(x))
}

func pctIv(iv Interval) string {
	if math.IsNaN(iv.Lo) {
		return "-"
	}
	return fmt.Sprintf("%s [%s, %s]", pct(iv.Est), pct(iv.Lo), pct(iv.Hi))
}

func sdPct(x float64) string {
	if math.IsNaN(x) {
		return "-"
	}
	return fmt.Sprintf("%.2f%%", 100*x)
}

// WriteJudgement writes the analyses as Markdown, the first row being the one
// that decides.
func WriteJudgement(w io.Writer, as []Analysis, v Validation, delta, alpha float64, detail bool) {
	fmt.Fprintf(w, "Intervals are %.0f%% two-sided t intervals of the change, delta is %.2f%%.\n\n", 100*(1-alpha), 100*delta)
	fmt.Fprintln(w, "| metric | pairs | layouts | summary (decides) | verdict | formal layout | formal verdict | run SD | layout SD | fixed layouts only | order effect |")
	fmt.Fprintln(w, "| --- | ---: | ---: | --- | --- | --- | --- | ---: | ---: | --- | --- |")
	for _, a := range as {
		order := "-"
		if !math.IsNaN(a.Order) {
			order = fmt.Sprintf("%s ± %s", pct(a.Order), sdPct(a.OrderSE))
		}
		fmt.Fprintf(w, "| %s | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n", a.Metric, a.Pairs, a.Layouts, pctIv(a.Summary), a.Verdict,
			pctIv(a.Formal), a.FormalVerdict, sdPct(a.RunSD), sdPct(a.LayoutSD), pctIv(a.Fixed), order)
	}
	fmt.Fprintf(w, "\n%d blocks, %d attempts run again for disturbance, %d blocks still disturbed and left out.\n", v.Blocks, v.Reruns, v.Disturbed)
	if len(as) > 0 && as[0].Verdict != VerdictNone && as[0].FormalVerdict != VerdictNone && conflict(as[0].Verdict, as[0].FormalVerdict) {
		fmt.Fprintln(w, "\nThe formal layout and the summary point in opposite directions: the change depends on where the code sits.")
	}
	if !detail || len(as) == 0 {
		return
	}
	a := as[0]
	var ls []string
	for l := range a.PerLayout {
		ls = append(ls, l)
	}
	sort.Strings(ls)
	fmt.Fprintf(w, "\n| layout | pairs | mean change of %s | each pair |\n| --- | ---: | ---: | --- |\n", a.Metric)
	for _, l := range ls {
		y := a.PerLayout[l]
		cells := make([]string, len(y))
		for i, x := range y {
			cells[i] = pct(x)
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s |\n", l, len(y), pct(Mean(y)), strings.Join(cells, " "))
	}
}

func conflict(a, b string) bool {
	slow := func(s string) bool { return s == VerdictSlower || s == VerdictSlowerSmall }
	return (a == VerdictFaster && slow(b)) || (slow(a) && b == VerdictFaster)
}

// Judge compares nw with base on the sum of the metrics, which decides, and on
// each metric alone. The validation lists the problems of the records; when it
// has any, the analyses must not be used.
func Judge(rs []Record, kind, base, nw string, alpha, delta float64) ([]Analysis, Validation) {
	ps, v := Pairs(rs, kind, base, nw, "sum")
	if len(v.Problems) > 0 {
		return nil, v
	}
	as := []Analysis{Analyze("sum", ps, alpha, delta)}
	names := map[string]bool{}
	for _, r := range rs {
		if r.Kind == kind && r.Side == base {
			for k := range r.Metrics {
				names[k] = true
			}
		}
	}
	var ks []string
	for k := range names {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	if len(ks) > 1 {
		for _, k := range ks {
			p, _ := Pairs(rs, kind, base, nw, k)
			as = append(as, Analyze(k, p, alpha, delta))
		}
	}
	return as, v
}
