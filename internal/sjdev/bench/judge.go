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

// Unit is what the analysis counts: the two blocks of one position and rep in
// the rounds 2k and 2k+1, where every two sides ran in opposite orders. A unit
// is used whole or not at all, so the order of the sides cancels in its mean.
type Unit struct {
	Pos   string
	Rep   int
	Round int // 2k, the first round of the pair
	// The metrics of the base and the new side in the block of round 2k (0)
	// and of round 2k+1 (1).
	Base, New [2]map[string]float64
	NewFirst  [2]bool // the new side ran before the base in that block
}

// Y returns ln(new/base) of the metric in the two blocks ("sum" adds every
// metric of the process). ok is false when a block lacks the metric.
func (u Unit) Y(metric string) (y [2]float64, ok bool) {
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
	for i := range 2 {
		b, n := val(u.Base[i]), val(u.New[i])
		if !(b > 0) || !(n > 0) {
			return y, false
		}
		y[i] = math.Log(n / b)
	}
	return y, true
}

// Validation is what reading the records against the plan found.
type Validation struct {
	// Problems make the records unusable: a failed process, a record twice, a
	// record of another experiment or outside the plan, a block whose sides ran
	// in different sessions or with other page settings. With any, nothing is
	// judged.
	Problems []string
	Planned  int      // units in the plan
	Units    int      // of those, whole and usable
	Lost     []string // the units left out, and why
	Reruns   int      // attempts run again for disturbance
	// Complete is true when every position of the plan has a usable unit and the
	// formal one has 2 (or all it planned, when fewer). Only a complete run
	// gets a verdict.
	Complete   bool
	Incomplete []string // why not
	Manifest   bool     // the plan came from the manifest of the run
}

// UnitsOf reads the records of the plan's kind and returns the usable units of
// base and nw. A block is usable when its last attempt has a record of every
// side of the plan, none of them failed or disturbed, all from one session and
// with the plan's page setting. A unit is usable when both its blocks are.
// experiment, when not empty, is the id every record must carry. verified asks
// that every record with fresh pages also had them checked.
func UnitsOf(rs []Record, p Plan, experiment, base, nw string, verified bool) ([]Unit, Validation) {
	var v Validation
	if err := p.Check(); err != nil {
		v.Problems = append(v.Problems, err.Error())
		return nil, v
	}
	for _, s := range []string{base, nw} {
		if !slices.Contains(p.Sides, s) {
			v.Problems = append(v.Problems, fmt.Sprintf("side %s is not in the plan %v", s, p.Sides))
		}
	}
	if len(v.Problems) > 0 {
		return nil, v
	}
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
	for _, r := range rs {
		if r.Kind != p.Kind {
			continue
		}
		where := fmt.Sprintf("%s@%s round %d rep %d attempt %d", r.Side, r.Pos, r.Round, r.Rep, r.Attempt)
		switch {
		case experiment != "" && r.Experiment != experiment:
			v.Problems = append(v.Problems, fmt.Sprintf("%s belongs to experiment %q, the manifest is %q", where, r.Experiment, experiment))
			continue
		case !slices.Contains(p.Sides, r.Side), !slices.Contains(p.Positions, r.Pos), !slices.Contains(p.Rounds, r.Round), r.Rep < 0 || r.Rep >= p.Reps(r.Pos):
			v.Problems = append(v.Problems, fmt.Sprintf("%s is outside the plan", where))
			continue
		}
		b := bk{r.Pos, r.Round, r.Rep}
		k := pk{b, r.Attempt, r.Side}
		if o, dup := seen[k]; dup {
			v.Problems = append(v.Problems, fmt.Sprintf("%s is in the records twice (sessions %q and %q)", where, o.Session, r.Session))
			continue
		}
		seen[k] = r
		if a, ok := last[b]; !ok || r.Attempt > a {
			last[b] = r.Attempt
		}
	}
	var names []string
	// block returns the base and new records of a usable block, or why it is not.
	block := func(b bk) (rb, rn Record, why string) {
		a, ok := last[b]
		if !ok {
			return rb, rn, "not run"
		}
		v.Reruns += a
		session := ""
		for _, s := range p.Sides {
			r, ok := seen[pk{b, a, s}]
			if !ok {
				return rb, rn, fmt.Sprintf("attempt %d has no record of %s", a, s)
			}
			if r.Err != "" {
				v.Problems = append(v.Problems, fmt.Sprintf("%s@%s round %d attempt %d failed: %s", r.Side, r.Pos, r.Round, a, trunc(r.Err, 200)))
				return rb, rn, "failed"
			}
			if session == "" {
				session = r.Session
			} else if r.Session != session {
				v.Problems = append(v.Problems, fmt.Sprintf("block %s round %d rep %d attempt %d ran in two sessions (%q, %q)", b.pos, b.round, b.rep, a, session, r.Session))
				return rb, rn, "two sessions"
			}
			if r.FreshPages != p.FreshPages || (verified && r.FreshPages && !r.FreshVerified) {
				v.Problems = append(v.Problems, fmt.Sprintf("%s@%s round %d attempt %d: fresh pages %v (checked %v), the plan says %v", r.Side, r.Pos, r.Round, a, r.FreshPages, r.FreshVerified, p.FreshPages))
				return rb, rn, "page setting"
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
				return rb, rn, "metrics"
			}
		}
		for _, s := range p.Sides {
			if seen[pk{b, a, s}].Disturbed {
				return rb, rn, fmt.Sprintf("still disturbed after %d tries (%s)", a+1, s)
			}
		}
		return seen[pk{b, a, base}], seen[pk{b, a, nw}], ""
	}
	var us []Unit
	positions := slices.Clone(p.Positions)
	slices.Sort(positions)
	rounds := slices.Clone(p.Rounds)
	slices.Sort(rounds)
	for _, rd := range rounds {
		if rd%2 != 0 {
			continue
		}
		for _, pos := range positions {
			for rep := range p.Reps(pos) {
				v.Planned++
				u := Unit{Pos: pos, Rep: rep, Round: rd}
				var whys []string
				for i := range 2 {
					rb, rn, why := block(bk{pos, rd + i, rep})
					if why != "" {
						whys = append(whys, fmt.Sprintf("round %d: %s", rd+i, why))
						continue
					}
					u.Base[i], u.New[i], u.NewFirst[i] = rb.Metrics, rn.Metrics, rn.Slot < rb.Slot
				}
				if len(whys) > 0 {
					v.Lost = append(v.Lost, fmt.Sprintf("%s rep %d rounds %d and %d: %s", pos, rep, rd, rd+1, strings.Join(whys, ", ")))
					continue
				}
				if u.NewFirst[0] == u.NewFirst[1] {
					v.Problems = append(v.Problems, fmt.Sprintf("%s rep %d: %s and %s ran in the same order in rounds %d and %d", pos, rep, base, nw, rd, rd+1))
					continue
				}
				us = append(us, u)
			}
		}
	}
	v.Units = len(us)
	have := map[string]int{}
	for _, u := range us {
		have[u.Pos]++
	}
	for _, pos := range positions {
		need := 1
		if pos == Formal.String() {
			need = min(2, p.Reps(pos)*len(rounds)/2)
		}
		if have[pos] < need {
			v.Incomplete = append(v.Incomplete, fmt.Sprintf("%s has %d usable units of the %d it needs", pos, have[pos], need))
		}
	}
	v.Complete = len(v.Incomplete) == 0 && len(v.Problems) == 0
	return us, v
}

// Analysis is the result of a comparison on one metric. Every value is on the
// log scale: exp(x)-1 is the change.
type Analysis struct {
	Metric  string
	Units   int
	Layouts int
	// Formal is the formal layout, the binary that ships: the mean of its units
	// with the run error pooled over every unit (df: units - 1).
	Formal Interval
	// Summary is the mean over the layouts of the plan, each with the same
	// weight, with the interval of the spread of the layout means (df: layouts
	// - 1). It treats this fixed set of layouts as if it were a sample of the
	// layouts a change can meet, which it is not: it was chosen, not drawn.
	Summary Interval
	// Fixed is the same mean with the interval of the run error only: how well
	// these very layouts were measured.
	Fixed Interval
	// RunSD is the run error of one block's log ratio, from the differences of
	// the two blocks of each unit after taking out their mean (the order
	// effect). It assumes the errors are independent with one variance.
	RunSD float64
	// LayoutSD is the spread of the layout means past what RunSD explains, under
	// the same assumption. 0 does not show that the change is the same at every
	// layout.
	LayoutSD float64
	// Order is mean(y | new ran first) - mean(y | base ran first) over the
	// units, with its SE. The mean of a unit does not hold it.
	Order, OrderSE float64
	PerLayout      map[string][]float64 // the unit means of each layout
	Verdict        string               // of Summary
	FormalVerdict  string
	// FormalRegression is true when the formal layout is slower by more than
	// delta (its whole interval past it), FormalSlower when it is slower at all.
	FormalRegression, FormalSlower bool
}

// Analyze computes the analysis of the units at 1-alpha and the regression
// limit delta (a fraction, such as 0.01). complete says whether the run may
// have a verdict.
func Analyze(metric string, us []Unit, alpha, delta float64, complete bool) Analysis {
	a := Analysis{Metric: metric, PerLayout: map[string][]float64{}}
	var d, o []float64
	formal := Formal.String()
	for _, u := range us {
		y, ok := u.Y(metric)
		if !ok {
			continue
		}
		m := (y[0] + y[1]) / 2
		a.PerLayout[u.Pos] = append(a.PerLayout[u.Pos], m)
		d = append(d, m)
		if u.NewFirst[0] {
			o = append(o, y[0]-y[1])
		} else {
			o = append(o, y[1]-y[0])
		}
	}
	a.Units = len(d)
	var layouts []string
	for l := range a.PerLayout {
		layouts = append(layouts, l)
	}
	sort.Strings(layouts)
	a.Layouts = len(layouts)
	U := float64(len(d))
	nan := TInterval(math.NaN(), math.NaN(), 0, alpha)
	a.Formal, a.Summary, a.Fixed = nan, nan, nan
	a.RunSD, a.LayoutSD, a.Order, a.OrderSE = math.NaN(), math.NaN(), math.NaN(), math.NaN()
	if len(o) > 1 {
		a.Order = Mean(o)
		a.OrderSE = math.Sqrt(Var(o) / U)
		// y_newfirst - y_basefirst = 2c + e1 - e2: its variance about the mean
		// is twice that of one block.
		a.RunSD = math.Sqrt(Var(o) / 2)
	}
	unitVar := a.RunSD * a.RunSD / 2 // of the mean of a unit
	var means []float64
	invn := 0.0
	for _, l := range layouts {
		means = append(means, Mean(a.PerLayout[l]))
		invn += 1 / float64(len(a.PerLayout[l]))
	}
	if L := float64(len(means)); L > 0 {
		m := Mean(means)
		invn /= L
		a.Summary = TInterval(m, math.Sqrt(Var(means)/L), L-1, alpha)
		a.Fixed = TInterval(m, math.Sqrt(unitVar*invn/L), U-1, alpha)
		if len(means) > 1 && !math.IsNaN(unitVar) {
			a.LayoutSD = math.Sqrt(max(0, Var(means)-unitVar*invn))
		}
	}
	if f := a.PerLayout[formal]; len(f) > 0 {
		a.Formal = TInterval(Mean(f), math.Sqrt(unitVar/float64(len(f))), U-1, alpha)
	}
	lim := math.Log1p(delta)
	a.FormalSlower = a.Formal.Lo > 0
	a.FormalRegression = a.Formal.Lo > lim
	if complete {
		a.Verdict = Decide(a.Summary, delta)
		a.FormalVerdict = Decide(a.Formal, delta)
	} else {
		a.Verdict, a.FormalVerdict = VerdictIncomplete, VerdictIncomplete
	}
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
	VerdictIncomplete   = "incomplete"
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

// WriteJudgement writes the analyses as Markdown, the first row (the sum of
// the metrics) being the one that decides.
func WriteJudgement(w io.Writer, as []Analysis, v Validation, delta, alpha float64, detail bool) {
	if v.Complete {
		fmt.Fprintf(w, "Complete: %d of %d planned units.\n\n", v.Units, v.Planned)
	} else {
		fmt.Fprintf(w, "INCOMPLETE, no verdict: %d of %d planned units. %s. Run the lost units again (a higher -tries goes on in the same file) before reading the numbers below, which only describe what was measured.\n\n", v.Units, v.Planned, strings.Join(v.Incomplete, ", "))
	}
	if !v.Manifest {
		fmt.Fprintln(w, "No manifest: the plan comes from the flags, and the identity of the binaries was not checked.")
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%.0f%% two-sided t intervals of the change, delta %.2f%%. A unit is the two blocks of a layout in rounds 2k and 2k+1, where the sides ran in opposite orders.\n\n", 100*(1-alpha), 100*delta)
	fmt.Fprintln(w, "| metric | units | formal layout | formal verdict | mean of the layouts | verdict | these layouts only | run SD (est.) | layout SD (est.) | order effect |")
	fmt.Fprintln(w, "| --- | ---: | --- | --- | --- | --- | --- | ---: | ---: | --- |")
	for _, a := range as {
		order := "-"
		if !math.IsNaN(a.Order) {
			order = fmt.Sprintf("%s ± %s", pct(a.Order), sdPct(a.OrderSE))
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n", a.Metric, a.Units, pctIv(a.Formal), a.FormalVerdict, pctIv(a.Summary), a.Verdict,
			pctIv(a.Fixed), sdPct(a.RunSD), sdPct(a.LayoutSD), order)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- formal layout: the binary that ships, with the run error pooled over every unit.")
	fmt.Fprintln(w, "- mean of the layouts: each layout of the plan with the same weight. Its interval treats this fixed, chosen set of layouts as if it were a random sample of the places a change can land, which nothing has shown.")
	fmt.Fprintln(w, "- these layouts only: the same mean with the run error alone, how well these very layouts were measured.")
	fmt.Fprintln(w, "- run SD and layout SD are estimates that assume independent run errors of one variance, after taking out the order effect. A layout SD of 0 does not show that the change is the same at every layout.")
	fmt.Fprintf(w, "\n%d attempts ran again for disturbance.\n", v.Reruns)
	if len(as) > 0 {
		a := as[0]
		switch {
		case a.FormalRegression:
			fmt.Fprintf(w, "\n**The formal layout is slower by more than delta** (%s). The mean of the layouts does not cancel this: the binary as it ships would regress.\n", pctIv(a.Formal))
		case a.FormalSlower:
			fmt.Fprintf(w, "\n**The formal layout is slower** (%s), not shown to be past delta.\n", pctIv(a.Formal))
		}
		if v.Complete && conflict(a.Verdict, a.FormalVerdict) {
			fmt.Fprintln(w, "\nThe formal layout and the mean of the layouts point in opposite directions: the change depends on where the code sits.")
		}
	}
	if len(v.Lost) > 0 {
		fmt.Fprintln(w, "\nUnits left out:")
		fmt.Fprintln(w)
		for _, l := range v.Lost {
			fmt.Fprintf(w, "- %s\n", l)
		}
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
	fmt.Fprintf(w, "\n| layout | units | mean change of %s | each unit |\n| --- | ---: | ---: | --- |\n", a.Metric)
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
// each metric alone, against the plan. When the validation has problems, there
// are no analyses.
func Judge(rs []Record, p Plan, experiment, base, nw string, verified bool, alpha, delta float64) ([]Analysis, Validation) {
	us, v := UnitsOf(rs, p, experiment, base, nw, verified)
	v.Manifest = experiment != ""
	if len(v.Problems) > 0 {
		return nil, v
	}
	as := []Analysis{Analyze("sum", us, alpha, delta, v.Complete)}
	names := map[string]bool{}
	for _, u := range us {
		for k := range u.Base[0] {
			names[k] = true
		}
	}
	var ks []string
	for k := range names {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	if len(ks) > 1 {
		for _, k := range ks {
			as = append(as, Analyze(k, us, alpha, delta, v.Complete))
		}
	}
	return as, v
}

// PlanFromRecords is the plan of a file of records that has no manifest: the
// sides found in the records of the kind, the positions and rounds given, and
// the page setting of the records (which must be one).
func PlanFromRecords(rs []Record, kind string, positions []string, rounds []int, formalReps int) (Plan, error) {
	p := Plan{Kind: kind, Positions: positions, Rounds: rounds, FormalReps: formalReps}
	fresh := map[bool]bool{}
	for _, r := range rs {
		if r.Kind != kind {
			continue
		}
		if !slices.Contains(p.Sides, r.Side) {
			p.Sides = append(p.Sides, r.Side)
		}
		fresh[r.FreshPages] = true
	}
	slices.Sort(p.Sides)
	if len(fresh) > 1 {
		return p, fmt.Errorf("the records of %s mix runs with and without fresh pages", kind)
	}
	p.FreshPages = fresh[true]
	return p, p.Check()
}
