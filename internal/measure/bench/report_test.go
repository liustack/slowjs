// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package bench

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestMedian(t *testing.T) {
	if Median([]float64{3, 1, 2}) != 2 || Median([]float64{4, 1, 2, 3}) != 2.5 || Median([]float64{5}) != 5 {
		t.Error("Median")
	}
	if !math.IsNaN(Median(nil)) {
		t.Error("Median of nothing")
	}
}

func TestVerdictRule(t *testing.T) {
	for _, c := range []struct {
		ratio, rng float64
		agree, n   int
		want       string
	}{
		{0.95, 0.02, 4, 4, "faster"},
		{1.05, 0.02, 4, 4, "slower"},
		{0.985, 0.01, 4, 4, "no change"}, // under 2%
		{0.95, 0.12, 4, 4, "no change"},  // within the range of the base's own positions
		{0.95, 0.08, 4, 4, "faster"},     // 5% is past half of 8%
		{0.95, 0.10, 4, 4, "no change"},  // 5% is exactly half of 10%, not past it
		{0.95, 0.02, 2, 4, "no change"},  // half of the positions agree: not most
		{0.95, 0.02, 3, 4, "faster"},
		{1.02, 0.0, 4, 4, "no change"}, // exactly 2% is not past 2%
	} {
		if got := Verdict(c.ratio, c.rng, c.agree, c.n); got != c.want {
			t.Errorf("Verdict(%v, %v, %d/%d) = %s, want %s", c.ratio, c.rng, c.agree, c.n, got, c.want)
		}
	}
}

// records makes rounds of records for sides base and new at the positions. baseAt gives the
// time of the base at a position, scale the ratio new / base at a position.
func records(positions []string, rounds int, baseAt func(p string) float64, scale func(p string) float64) []Record {
	var rs []Record
	for r := range rounds {
		for _, p := range positions {
			// A little noise that keeps the median of the pairs where the test says.
			noise := 1 + 0.001*float64(r%3-1)
			b := baseAt(p) * noise
			rs = append(rs, Record{Side: "base", Pos: p, Kind: "k", Round: r, Metrics: map[string]float64{"page": b}})
			rs = append(rs, Record{Side: "new", Pos: p, Kind: "k", Round: r, Metrics: map[string]float64{"page": b * scale(p)}})
		}
	}
	return rs
}

var four = []string{"0.0", "0.16", "32.0", "32.16"}

func TestAAIsNoChange(t *testing.T) {
	rs := records(four, 5, func(p string) float64 { return 100 + float64(len(p)) }, func(string) float64 { return 1 })
	rows, err := Compare(rs, "k", "base", "new")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Verdict != "no change" || math.Abs(rows[0].Ratio-1) > 1e-9 || !rows[0].Callgrind {
		t.Errorf("A/A row = %+v", rows[0])
	}
	if rows[0].Positions != 4 || rows[0].Rounds != 5 {
		t.Errorf("positions %d rounds %d", rows[0].Positions, rows[0].Rounds)
	}
}

func TestRealChangeIsFound(t *testing.T) {
	rs := records(four, 5, func(p string) float64 { return 100 }, func(string) float64 { return 0.93 })
	rows, _ := Compare(rs, "k", "base", "new")
	r := rows[0]
	if r.Verdict != "faster" || math.Abs(r.Ratio-0.93) > 1e-6 || r.Agree != 4 || r.Callgrind {
		t.Errorf("row = %+v", r)
	}
}

func TestChangeInsideThePositionRangeIsNot(t *testing.T) {
	// The base itself moves by 12% between positions, so a 5% change is a layout effect.
	base := map[string]float64{"0.0": 100, "0.16": 112, "32.0": 100, "32.16": 110}
	rs := records(four, 5, func(p string) float64 { return base[p] }, func(string) float64 { return 0.95 })
	rows, _ := Compare(rs, "k", "base", "new")
	if rows[0].Verdict != "no change" || math.Abs(rows[0].Range-0.12) > 0.005 {
		t.Errorf("row = %+v", rows[0])
	}
}

func TestMixedSignsAreNotAChange(t *testing.T) {
	sc := map[string]float64{"0.0": 0.95, "0.16": 0.95, "32.0": 1.06, "32.16": 1.06}
	rs := records(four, 5, func(string) float64 { return 100 }, func(p string) float64 { return sc[p] })
	rows, _ := Compare(rs, "k", "base", "new")
	if rows[0].Verdict != "no change" {
		t.Errorf("row = %+v", rows[0])
	}
}

func TestSmallRealChangeAsksForCallgrind(t *testing.T) {
	rs := records(four, 5, func(string) float64 { return 100 }, func(string) float64 { return 0.975 })
	rows, _ := Compare(rs, "k", "base", "new")
	if rows[0].Verdict != "faster" || !rows[0].Callgrind {
		t.Errorf("row = %+v", rows[0])
	}
	var b bytes.Buffer
	WriteTable(&b, rows, "base", "new", false)
	if !strings.Contains(b.String(), "faster, under 3% so see callgrind") {
		t.Errorf("table:\n%s", b.String())
	}
}

func TestSeveralMetricsGetASum(t *testing.T) {
	var rs []Record
	for r := range 3 {
		for _, p := range four {
			rs = append(rs,
				Record{Side: "base", Pos: p, Kind: "k", Round: r, Metrics: map[string]float64{"a": 10, "b": 30}},
				Record{Side: "new", Pos: p, Kind: "k", Round: r, Metrics: map[string]float64{"a": 10, "b": 15}})
		}
	}
	rows, err := Compare(rs, "k", "base", "new")
	if err != nil || len(rows) != 3 {
		t.Fatalf("%v, %v", rows, err)
	}
	if rows[0].Metric != "a" || rows[2].Metric != "sum" || math.Abs(rows[2].Ratio-25.0/40) > 1e-9 {
		t.Errorf("rows = %+v", rows)
	}
	var out bytes.Buffer
	WriteTable(&out, rows, "base", "new", true)
	for _, want := range []string{"| a |", "| sum |", "0.0 | 0.16", "0.625"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("table lacks %q:\n%s", want, out.String())
		}
	}
}

func TestComparedProcessesNeedBothSides(t *testing.T) {
	rs := records(four, 3, func(string) float64 { return 100 }, func(string) float64 { return 1 })
	// Round 2 of position 0.0 has lost its new side.
	var kept []Record
	for _, r := range rs {
		if r.Round == 2 && r.Pos == "0.0" && r.Side == "new" {
			continue
		}
		kept = append(kept, r)
	}
	rows, err := Compare(kept, "k", "base", "new")
	if err != nil || rows[0].Rounds != 2 {
		t.Fatalf("%+v, %v", rows, err)
	}
	if _, err := Compare(kept, "other", "base", "new"); err == nil {
		t.Error("an unknown kind accepted")
	}
	if _, err := Compare(kept, "k", "base", "ghost"); err == nil {
		t.Error("an unknown side accepted")
	}
}

func TestRerunReplacesTheDisturbedRecord(t *testing.T) {
	rs := []Record{
		{Side: "base", Pos: "0.0", Kind: "k", Round: 0, Metrics: map[string]float64{"m": 100}},
		{Side: "new", Pos: "0.0", Kind: "k", Round: 0, Attempt: 0, Disturbed: true, Metrics: map[string]float64{"m": 150}},
		{Side: "new", Pos: "0.0", Kind: "k", Round: 0, Attempt: 1, Metrics: map[string]float64{"m": 100}},
	}
	rows, err := Compare(rs, "k", "base", "new")
	if err != nil || rows[0].Ratio != 1 {
		t.Fatalf("%+v, %v", rows, err)
	}
}

func TestMemory(t *testing.T) {
	rs := []Record{
		{Side: "a", Kind: "k", Round: 0, HWMKB: 10240}, {Side: "a", Kind: "k", Round: 1, HWMKB: 20480}, {Side: "a", Kind: "k", Round: 2, HWMKB: 30720},
		{Side: "b", Kind: "k", Round: 0, HWMKB: 1024}, {Side: "b", Kind: "other", HWMKB: 9999}, {Side: "b", Kind: "k", Round: 1},
	}
	rows := Memory(rs, "k")
	if len(rows) != 2 || rows[0].Median != 20 || rows[0].Min != 10 || rows[0].Max != 30 || rows[1].N != 1 {
		t.Errorf("rows = %+v", rows)
	}
	var b bytes.Buffer
	WriteMemory(&b, rows)
	if !strings.Contains(b.String(), fmt.Sprintf("| a | %.1f |", 20.0)) {
		t.Errorf("table:\n%s", b.String())
	}
}

func TestFinalKeepsTheLastTry(t *testing.T) {
	rs := []Record{
		{Side: "a", Pos: "0.0", Kind: "k", Round: 0, Attempt: 0, Disturbed: true, HWMKB: 1024},
		{Side: "a", Pos: "0.0", Kind: "k", Round: 0, Attempt: 1, HWMKB: 2048},
		{Side: "a", Pos: "0.0", Kind: "k", Round: 1, Attempt: 0, Disturbed: true, HWMKB: 3072},
		{Side: "a", Pos: "0.0", Kind: "other", Round: 0},
	}
	f := Final(rs, "k")
	if len(f) != 2 || f[0].HWMKB != 2048 || f[1].HWMKB != 3072 {
		t.Errorf("Final = %+v", f)
	}
	if Disturbed(rs, "k") != 1 {
		t.Errorf("Disturbed = %d", Disturbed(rs, "k"))
	}
	if m := Memory(rs, "k"); m[0].N != 2 {
		t.Errorf("Memory counted %d processes", m[0].N)
	}
}

// TestPlacesListsEachSideAtEachPlacement: with the placement paired with the
// layout, the table has a row for each k, in the order of k, with the median
// of each side there, so that two levels of time show at a glance.
func TestPlacesListsEachSideAtEachPlacement(t *testing.T) {
	ps := Protocol()
	rec := func(side string, k, round int, ms float64) Record {
		return Record{Side: side, Pos: ps[k].String(), Kind: "m", Round: round, Place: &k, Metrics: map[string]float64{"eval": ms}}
	}
	rs := []Record{
		rec("a", 7, 0, 100), rec("b", 7, 0, 115), rec("a", 7, 1, 102), rec("b", 7, 1, 117),
		rec("a", 0, 0, 114), rec("b", 0, 0, 104), rec("a", 0, 1, 114), rec("b", 0, 1, 104),
		// another kind is not read
		{Side: "a", Pos: ps[1].String(), Kind: "other", Metrics: map[string]float64{"eval": 1}},
	}
	rows := Places(rs, "m", "a", "b")
	if len(rows) != 2 || rows[0].K != 0 || rows[1].K != 7 {
		t.Fatalf("rows %+v", rows)
	}
	if r := rows[1]; r.Pos != ps[7].String() || r.Base != 101 || r.New != 116 {
		t.Errorf("k 7: %+v", r)
	}
	var b bytes.Buffer
	WritePlaces(&b, rows, "a", "b")
	out := b.String()
	for _, want := range []string{"| 0 | " + ps[0].String() + " | eval | 114.00 | 104.00 | -8.77% |", "| 7 | " + ps[7].String() + " | eval | 101.00 | 116.00 | +14.85% |"} {
		if !strings.Contains(out, want) {
			t.Errorf("no row %q in:\n%s", want, out)
		}
	}
	// Records without a placement give no table.
	for i := range rs {
		rs[i].Place = nil
	}
	if rows := Places(rs, "m", "a", "b"); len(rows) != 0 {
		t.Errorf("rows without a placement: %+v", rows)
	}
}
