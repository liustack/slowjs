// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.9g, want %.9g", name, got, want)
	}
}

func TestTQuantileKnownValues(t *testing.T) {
	// Tables of Student's t.
	for _, c := range []struct{ p, df, want float64 }{
		{0.975, 1, 12.7062047},
		{0.975, 2, 4.30265273},
		{0.975, 3, 3.18244631},
		{0.975, 7, 2.36462425},
		{0.975, 15, 2.13144955},
		{0.975, 30, 2.04227246},
		{0.95, 7, 1.89457861},
		{0.8, 15, 0.86624497}, // checked by integrating the density
		{0.975, 1e7, 1.95996437},
		{0.025, 7, -2.36462425},
	} {
		near(t, fmt.Sprintf("TQuantile(%v, %v)", c.p, c.df), TQuantile(c.p, c.df), c.want, 1e-6)
	}
	near(t, "TCDF(2.36462425, 7)", TCDF(2.36462425, 7), 0.975, 1e-8)
	near(t, "TCDF(0, 5)", TCDF(0, 5), 0.5, 1e-12)
	near(t, "TwoSidedP(2.13144955, 15)", TwoSidedP(2.13144955, 15), 0.05, 1e-7)
}

func TestIncBetaKnownValues(t *testing.T) {
	near(t, "I_0.5(2,3)", IncBeta(2, 3, 0.5), 0.6875, 1e-12) // 11/16
	near(t, "I_0.3(1,1)", IncBeta(1, 1, 0.3), 0.3, 1e-12)
	near(t, "I_0.9(0.5,0.5)", IncBeta(0.5, 0.5, 0.9), 2/math.Pi*math.Asin(math.Sqrt(0.9)), 1e-10)
}

func TestTIntervalOfKnownSample(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	near(t, "mean", Mean(x), 4.5, 1e-12)
	near(t, "var", Var(x), 6, 1e-12)
	iv := TInterval(Mean(x), math.Sqrt(Var(x)/8), 7, 0.05)
	// 4.5 ± 2.36462425·sqrt(6/8)
	near(t, "lo", iv.Lo, 4.5-2.36462425*math.Sqrt(0.75), 1e-6)
	near(t, "hi", iv.Hi, 4.5+2.36462425*math.Sqrt(0.75), 1e-6)
	if e := TInterval(1, math.NaN(), 0, 0.05); !math.IsNaN(e.Lo) {
		t.Error("an interval without degrees of freedom")
	}
}

func TestDecide(t *testing.T) {
	d := 0.005
	l := math.Log1p(d)
	for _, c := range []struct {
		lo, hi float64
		want   string
	}{
		{-0.02, -0.001, VerdictFaster},
		{0.001, 0.02, VerdictSlower},
		{0.001, l / 2, VerdictSlowerSmall},
		{-0.01, l / 2, VerdictNoRegression},
		{-0.01, l * 1.01, VerdictInconclusive},
		{math.NaN(), math.NaN(), VerdictNone},
	} {
		if got := Decide(Interval{Lo: c.lo, Hi: c.hi}, d); got != c.want {
			t.Errorf("Decide(%v, %v) = %s, want %s", c.lo, c.hi, got, c.want)
		}
	}
}

// unit makes a unit of pos whose block with the new side first has the log
// ratio nf and whose other block has bf.
func unit(pos string, nf, bf float64) Unit {
	one := func(y float64) (map[string]float64, map[string]float64) {
		return map[string]float64{"p": 1}, map[string]float64{"p": math.Exp(y)}
	}
	var u Unit
	u.Pos = pos
	u.Base[0], u.New[0] = one(nf)
	u.Base[1], u.New[1] = one(bf)
	u.NewFirst = [2]bool{true, false}
	return u
}

// With y = mu_l ± c and no noise, the order effect is 2c, the run SD 0 and the
// layout SD the spread of mu_l: the order is not taken for run error.
func TestAnalyzeTakesTheOrderEffectOut(t *testing.T) {
	const c = 0.004
	mu := []float64{0.01, 0.02, 0.03}
	var us []Unit
	for i, m := range mu {
		us = append(us, unit([]string{"0.0", "0.8", "0.16"}[i], m+c, m-c))
	}
	an := Analyze("sum", us, 0.05, 0.01, true)
	near(t, "order", an.Order, 2*c, 1e-12)
	near(t, "run SD", an.RunSD, 0, 1e-12)
	near(t, "layout SD", an.LayoutSD, 0.01, 1e-12)
	near(t, "summary", an.Summary.Est, 0.02, 1e-12)
}

func TestAnalyzeKnownAnswer(t *testing.T) {
	us := []Unit{
		unit("0.0", 0.012, 0.008), // mean 0.010, difference 0.004
		unit("0.0", 0.015, 0.009), // 0.012, 0.006
		unit("0.8", 0.031, 0.029), // 0.030, 0.002
	}
	an := Analyze("sum", us, 0.05, 0.01, true)
	// differences {0.004, 0.006, 0.002}: mean 0.004, variance 4e-6.
	near(t, "order", an.Order, 0.004, 1e-12)
	near(t, "order SE", an.OrderSE, math.Sqrt(4e-6/3), 1e-12)
	near(t, "run SD", an.RunSD, math.Sqrt(2e-6), 1e-12)
	// layout means 0.011 and 0.030
	near(t, "summary", an.Summary.Est, 0.0205, 1e-12)
	near(t, "summary SE", an.Summary.SE, math.Sqrt(0.019*0.019/2/2), 1e-12)
	// the variance of a unit mean is 1e-6, mean(1/n) is 0.75
	near(t, "fixed SE", an.Fixed.SE, math.Sqrt(1e-6*0.75/2), 1e-12)
	near(t, "layout SD", an.LayoutSD, math.Sqrt(0.019*0.019/2-0.75e-6), 1e-12)
	near(t, "formal", an.Formal.Est, 0.011, 1e-12)
	near(t, "formal SE", an.Formal.SE, math.Sqrt(1e-6/2), 1e-12)
	if an.Summary.DF != 1 || an.Fixed.DF != 2 || an.Formal.DF != 2 || an.Units != 3 || an.Layouts != 2 {
		t.Errorf("df %v %v %v, units %d, layouts %d", an.Summary.DF, an.Fixed.DF, an.Formal.DF, an.Units, an.Layouts)
	}
	if !an.FormalSlower || an.FormalRegression {
		t.Errorf("formal slower %v, regression %v (lo %v)", an.FormalSlower, an.FormalRegression, an.Formal.Lo)
	}
	if inc := Analyze("sum", us, 0.05, 0.01, false); inc.Verdict != VerdictIncomplete || inc.FormalVerdict != VerdictIncomplete {
		t.Errorf("an incomplete run got %s, %s", inc.Verdict, inc.FormalVerdict)
	}
}

// planRecords makes the records of a plan run with Order, the value of a process
// being f(side, pos, round, rep).
func planRecords(p Plan, seed, session string, f func(side, pos string, round, rep int) float64) []Record {
	var vs []Variant
	for _, s := range p.Sides {
		for _, pos := range p.Positions {
			vs = append(vs, Variant{Side: s, Pos: pos})
		}
	}
	var rs []Record
	for _, rd := range p.Rounds {
		for _, b := range Order(vs, seed, rd, p.FormalReps) {
			for slot, v := range b.Sides {
				x := f(v.Side, v.Pos, rd, b.Rep)
				rs = append(rs, Record{Side: v.Side, Pos: v.Pos, Kind: p.Kind, Round: rd, Rep: b.Rep, Slot: slot, Session: session, Experiment: seed,
					FreshPages: p.FreshPages, FreshVerified: p.FreshPages, Metrics: map[string]float64{"p1": x, "p2": 2 * x}})
			}
		}
	}
	return rs
}

func testPlan() Plan {
	return Plan{Kind: "k", Sides: []string{"a", "b", "c"}, Positions: []string{"0.0", "0.8", "0.16"}, Rounds: []int{0, 1}, FormalReps: 2}
}

func TestJudgeWritesAVerdict(t *testing.T) {
	p := testPlan()
	rs := planRecords(p, "e1", "s1", func(side, pos string, round, rep int) float64 {
		if side == "b" {
			return 10 * (1.02 + 0.001*float64(len(pos)) + 0.0005*float64(rep))
		}
		return 10
	})
	as, v := Judge(rs, p, "e1", "a", "b", false, 0.05, 0.005)
	if len(v.Problems) > 0 || !v.Complete || v.Units != 4 || v.Planned != 4 || len(as) != 3 {
		t.Fatalf("%+v", v)
	}
	if as[0].Verdict != VerdictSlower || as[0].FormalVerdict != VerdictSlower || !as[0].FormalRegression {
		t.Errorf("verdicts %s, %s, regression %v", as[0].Verdict, as[0].FormalVerdict, as[0].FormalRegression)
	}
	var b strings.Builder
	WriteJudgement(&b, as, v, 0.005, 0.05, true)
	for _, want := range []string{"Complete: 4 of 4", "| sum | 4 |", "| 0.16 | 1 |", "slower by more than delta", "fixed, chosen set of layouts"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no %q in:\n%s", want, b.String())
		}
	}
}

// A block with any side still disturbed is left out for every comparison, and
// with it the other block of its unit, so the orders stay balanced. A layout
// left with no unit makes the run incomplete.
func TestUnitsLeaveOutWholeUnits(t *testing.T) {
	p := testPlan()
	rs := planRecords(p, "e1", "s1", func(string, string, int, int) float64 { return 10 })
	for i := range rs {
		if rs[i].Side == "c" && rs[i].Pos == "0.8" && rs[i].Round == 1 {
			rs[i].Disturbed = true
		}
	}
	us, v := UnitsOf(rs, p, "e1", "a", "b", false)
	if len(v.Problems) > 0 || len(us) != 3 || v.Units != 3 || len(v.Lost) != 1 || v.Complete {
		t.Fatalf("units %d, %+v", len(us), v)
	}
	if !strings.Contains(v.Lost[0], "0.8") || !strings.Contains(v.Lost[0], "disturbed") || !strings.Contains(strings.Join(v.Incomplete, " "), "0.8") {
		t.Errorf("%+v", v)
	}
	for _, u := range us {
		if u.Pos == "0.8" {
			t.Error("the unit of 0.8 was kept")
		}
	}
	as, v := Judge(rs, p, "e1", "a", "b", false, 0.05, 0.01)
	if as[0].Verdict != VerdictIncomplete {
		t.Errorf("verdict %s", as[0].Verdict)
	}
	var b strings.Builder
	WriteJudgement(&b, as, v, 0.01, 0.05, false)
	if !strings.Contains(b.String(), "INCOMPLETE, no verdict") {
		t.Errorf("%s", b.String())
	}
	// With 3 formal units planned, losing one still leaves 2: complete. With 2
	// planned, losing one is not.
	q := p
	q.FormalReps = 3
	for _, c := range []struct {
		p        Plan
		complete bool
	}{{q, true}, {p, false}} {
		rs = planRecords(c.p, "e1", "s1", func(string, string, int, int) float64 { return 10 })
		for i := range rs {
			if rs[i].Pos == "0.0" && rs[i].Rep == 1 && rs[i].Round == 0 {
				rs[i].Disturbed = true
			}
		}
		if _, v := UnitsOf(rs, c.p, "e1", "a", "b", false); v.Complete != c.complete || v.Units != v.Planned-1 {
			t.Errorf("formal reps %d: %+v", c.p.FormalReps, v)
		}
	}
}

// The case of the review: the base from one day, the new side from another,
// round 0 only. Nothing is judged.
func TestJudgeRefusesSplicedSessions(t *testing.T) {
	p := Plan{Kind: "k", Sides: []string{"base", "new"}, Positions: []string{"0.0", "0.8"}, Rounds: []int{0, 1}, FormalReps: 1}
	day1 := planRecords(p, "e1", "day1", func(string, string, int, int) float64 { return 10 })
	day2 := planRecords(p, "e1", "day2", func(string, string, int, int) float64 { return 9.8 })
	var rs []Record
	for _, r := range day1 {
		if r.Side == "base" && r.Round == 0 {
			rs = append(rs, r)
		}
	}
	for _, r := range day2 {
		if r.Side == "new" && r.Round == 0 {
			rs = append(rs, r)
		}
	}
	as, v := Judge(rs, p, "e1", "base", "new", false, 0.05, 0.01)
	if as != nil || !strings.Contains(strings.Join(v.Problems, "\n"), "two sessions") {
		t.Errorf("analyses %v, problems %v", as, v.Problems)
	}
	// The same day, still round 0 only: incomplete, no verdict.
	rs = rs[:0]
	for _, r := range day1 {
		if r.Round == 0 {
			rs = append(rs, r)
		}
	}
	if as, v := Judge(rs, p, "e1", "base", "new", false, 0.05, 0.01); v.Complete || len(v.Problems) > 0 || as[0].Verdict != VerdictIncomplete {
		t.Errorf("%+v", v)
	}
	// Records of another experiment.
	other := planRecords(p, "e2", "day1", func(string, string, int, int) float64 { return 10 })
	if _, v := Judge(other, p, "e1", "base", "new", false, 0.05, 0.01); !strings.Contains(strings.Join(v.Problems, "\n"), "experiment") {
		t.Errorf("problems %v", v.Problems)
	}
}

func TestUnitsFindProblems(t *testing.T) {
	p := testPlan()
	good := func() []Record {
		return planRecords(p, "e1", "s1", func(string, string, int, int) float64 { return 10 })
	}
	for name, c := range map[string]struct {
		edit     func([]Record) []Record
		verified bool
		want     string
	}{
		"failed":   {func(rs []Record) []Record { rs[0].Err = "exit status 1"; return rs }, false, "failed"},
		"twice":    {func(rs []Record) []Record { return append(rs, rs[0]) }, false, "twice"},
		"outside":  {func(rs []Record) []Record { r := rs[0]; r.Round = 2; return append(rs, r) }, false, "outside the plan"},
		"rep":      {func(rs []Record) []Record { r := rs[0]; r.Pos, r.Rep = "0.8", 1; return append(rs, r) }, false, "outside the plan"},
		"metrics":  {func(rs []Record) []Record { rs[1].Metrics = map[string]float64{"p1": 1}; return rs }, false, "reports the metrics"},
		"pages":    {func(rs []Record) []Record { rs[0].FreshPages = true; return rs }, false, "fresh pages"},
		"unproved": {func(rs []Record) []Record { return rs }, true, ""},
	} {
		q := p
		rs := c.edit(good())
		if name == "unproved" {
			q.FreshPages = true
			for i := range rs {
				rs[i].FreshPages = true
			}
			c.want = "checked false"
		}
		_, v := UnitsOf(rs, q, "e1", "a", "b", c.verified)
		if !strings.Contains(strings.Join(v.Problems, "\n"), c.want) {
			t.Errorf("%s: problems %v", name, v.Problems)
		}
	}
	if _, v := UnitsOf(good(), p, "e1", "a", "z", false); len(v.Problems) == 0 {
		t.Error("a side outside the plan")
	}
}

func TestPlanFromRecords(t *testing.T) {
	p := testPlan()
	rs := planRecords(p, "", "s1", func(string, string, int, int) float64 { return 1 })
	q, err := PlanFromRecords(rs, "k", p.Positions, p.Rounds, 2)
	if err != nil || fmt.Sprint(q.Sides) != "[a b c]" || q.FreshPages {
		t.Fatalf("%+v %v", q, err)
	}
	rs[0].FreshPages = true
	if _, err := PlanFromRecords(rs, "k", p.Positions, p.Rounds, 2); err == nil {
		t.Error("mixed page settings")
	}
	if _, err := PlanFromRecords(rs, "k", p.Positions, []int{0}, 2); err == nil {
		t.Error("a round without its partner")
	}
}
