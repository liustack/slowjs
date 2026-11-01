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

func TestHolm(t *testing.T) {
	got := Holm([]float64{0.01, 0.04, 0.03, 0.005}, 0.05)
	// sorted 0.005 (<0.0125), 0.01 (<0.0167), 0.03 (>0.025): stop.
	want := []bool{true, false, false, true}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Holm = %v, want %v", got, want)
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

// pairs makes pairs at layouts with the log ratios y[layout] (the first is the
// formal layout 0.0).
func pairsOf(y map[string][]float64) []Pair {
	var ps []Pair
	for l, ys := range y {
		for i, v := range ys {
			ps = append(ps, Pair{Pos: l, Round: i, Y: v, NewFirst: i%2 == 0})
		}
	}
	return ps
}

func TestAnalyzeSeparatesRunErrorFromLayoutSpread(t *testing.T) {
	// Two layouts with 4 pairs each. Within: {-1,1,-1,1}·e has variance 4e²/3.
	// Layout means 0 and 2a: their variance is 2a².
	const e, a = 0.003, 0.004
	ps := pairsOf(map[string][]float64{
		"0.0": {-e, e, -e, e},
		"0.8": {2*a - e, 2*a + e, 2*a - e, 2*a + e},
	})
	an := Analyze("sum", ps, 0.05, 0.005)
	near(t, "run SD", an.RunSD, math.Sqrt(4*e*e/3), 1e-12)
	// var(means) = 2a², minus RunSD²·mean(1/n) = (4e²/3)/4.
	near(t, "layout SD", an.LayoutSD, math.Sqrt(2*a*a-e*e/3), 1e-12)
	near(t, "summary", an.Summary.Est, a, 1e-12)
	near(t, "summary SE", an.Summary.SE, math.Sqrt(2*a*a/2), 1e-12)
	if an.Summary.DF != 1 || an.Fixed.DF != 6 || an.Formal.DF != 3 {
		t.Errorf("df %v %v %v", an.Summary.DF, an.Fixed.DF, an.Formal.DF)
	}
	near(t, "fixed SE", an.Fixed.SE, math.Sqrt(4*e*e/3)*math.Sqrt(0.25/2), 1e-12)
	near(t, "formal", an.Formal.Est, 0, 1e-12)
	near(t, "formal SE", an.Formal.SE, math.Sqrt(4*e*e/3/4), 1e-12)
	// NewFirst on even rounds: the first and third pair of each layout.
	near(t, "order", an.Order, -2*e, 1e-12)
	if an.Layouts != 2 || an.Pairs != 8 {
		t.Errorf("layouts %d pairs %d", an.Layouts, an.Pairs)
	}
}

func TestAnalyzeLayoutSpreadIsZeroWhenRunErrorExplainsIt(t *testing.T) {
	ps := pairsOf(map[string][]float64{"0.0": {0.01, -0.01}, "0.8": {0.011, -0.009}})
	an := Analyze("sum", ps, 0.05, 0.005)
	if an.LayoutSD != 0 {
		t.Errorf("layout SD %v", an.LayoutSD)
	}
}

func rec(side, pos string, round, attempt, slot int, v float64) Record {
	return Record{Side: side, Pos: pos, Kind: "k", Round: round, Attempt: attempt, Slot: slot, Metrics: map[string]float64{"p1": v, "p2": 2 * v}}
}

func TestPairsTakeTheLastAttemptOfEachBlock(t *testing.T) {
	rs := []Record{
		rec("a", "0.0", 0, 0, 0, 10), rec("b", "0.0", 0, 0, 1, 99),
		rec("a", "0.0", 0, 1, 1, 10), rec("b", "0.0", 0, 1, 0, 11),
		rec("a", "0.8", 0, 0, 0, 10), rec("b", "0.8", 0, 0, 1, 10),
	}
	rs[1].Disturbed = true
	ps, v := Pairs(rs, "k", "a", "b", "sum")
	if len(v.Problems) > 0 || len(ps) != 2 || v.Reruns != 1 || v.Disturbed != 0 {
		t.Fatalf("pairs %v, validation %+v", ps, v)
	}
	near(t, "y", ps[0].Y, math.Log(33.0/30), 1e-12)
	if !ps[0].NewFirst || ps[1].NewFirst {
		t.Errorf("order %v %v", ps[0].NewFirst, ps[1].NewFirst)
	}
	ps, _ = Pairs(rs, "k", "a", "b", "p2")
	near(t, "y of p2", ps[0].Y, math.Log(22.0/20), 1e-12)
}

func TestPairsFindProblems(t *testing.T) {
	for name, c := range map[string]struct {
		rs   []Record
		want string
	}{
		"missing side": {[]Record{rec("a", "0.0", 0, 0, 0, 1)}, "no record of b"},
		"failed":       {[]Record{rec("a", "0.0", 0, 0, 0, 1), {Side: "b", Pos: "0.0", Kind: "k", Err: "exit status 1"}}, "failed"},
		"twice":        {[]Record{rec("a", "0.0", 0, 0, 0, 1), rec("a", "0.0", 0, 0, 0, 1), rec("b", "0.0", 0, 0, 1, 1)}, "twice"},
		"metrics":      {[]Record{rec("a", "0.0", 0, 0, 0, 1), {Side: "b", Pos: "0.0", Kind: "k", Slot: 1, Metrics: map[string]float64{"p1": 1}}}, "reports the metrics"},
		"nothing":      {nil, "no block"},
	} {
		_, v := Pairs(c.rs, "k", "a", "b", "sum")
		if len(v.Problems) == 0 || !strings.Contains(strings.Join(v.Problems, "\n"), c.want) {
			t.Errorf("%s: problems %v", name, v.Problems)
		}
	}
	// A block still disturbed after its tries is left out and counted.
	rs := []Record{rec("a", "0.0", 0, 0, 0, 1), rec("b", "0.0", 0, 0, 1, 1), rec("a", "0.8", 0, 0, 0, 1), rec("b", "0.8", 0, 0, 1, 1)}
	rs[3].Disturbed = true
	ps, v := Pairs(rs, "k", "a", "b", "sum")
	if len(ps) != 1 || v.Disturbed != 1 || len(v.Problems) != 0 {
		t.Errorf("pairs %d, %+v", len(ps), v)
	}
}

func TestJudgeWritesAVerdict(t *testing.T) {
	var rs []Record
	for l, pos := range []string{"0.0", "0.8", "0.16"} {
		for r := range 4 {
			f := 1.02 + 0.001*float64(l) + 0.0005*float64(r%2)
			rs = append(rs, rec("a", pos, r, 0, r%2, 10), rec("b", pos, r, 0, 1-r%2, 10*f))
		}
	}
	as, v := Judge(rs, "k", "a", "b", 0.05, 0.005)
	if len(v.Problems) > 0 || len(as) != 3 || as[0].Metric != "sum" {
		t.Fatalf("%+v %+v", as, v)
	}
	if as[0].Verdict != VerdictSlower || as[0].FormalVerdict != VerdictSlower {
		t.Errorf("verdicts %s, %s", as[0].Verdict, as[0].FormalVerdict)
	}
	var b strings.Builder
	WriteJudgement(&b, as, v, 0.005, 0.05, true)
	if !strings.Contains(b.String(), "| sum | 12 | 3 |") || !strings.Contains(b.String(), "| 0.16 | 4 |") {
		t.Errorf("table:\n%s", b.String())
	}
}
