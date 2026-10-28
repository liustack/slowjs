// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParserGoBench(t *testing.T) {
	p, err := ParserFor("gobench:p50-ms")
	if err != nil {
		t.Fatal(err)
	}
	out := `goos: linux
BenchmarkShop/render-projects-studio	     100	  59610000 ns/op	        59.61 p50-ms	        88.10 p99-ms
BenchmarkShop/render-projects-studio-orders-SH-1042	     100	  10470000 ns/op	        10.47 p50-ms
BenchmarkShop/load-8	  50	 1000 ns/op	 12 vm-KiB
PASS
`
	m, err := p(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m["Shop/render-projects-studio"] != 59.61 || m["Shop/render-projects-studio-orders-SH-1042"] != 10.47 {
		t.Errorf("metrics = %v", m)
	}
	if _, err := p("BenchmarkX 1 5 ns/op\n"); err == nil {
		t.Error("a line without the unit parsed")
	}
	ns, _ := ParserFor("gobench:ns/op")
	m, err = ns("BenchmarkX-8 \t 10\t 1500.5 ns/op\n")
	if err != nil || m["X-8"] != 1500.5 {
		t.Errorf("ns/op: %v, %v", m, err)
	}
	if _, err := ParserFor("gobench"); err == nil {
		t.Error("gobench without a unit accepted")
	}
	if _, err := ParserFor("other"); err == nil {
		t.Error("an unknown parser accepted")
	}
}

func TestParserJsrun(t *testing.T) {
	p, _ := ParserFor("jsrun")
	m, err := p("result: 5\neval: 3 runs, min 90ms, median 101.5ms, mean 102ms, max 110ms\nvmhwm-kb: 5\n")
	if err != nil || m["eval"] != 101.5 {
		t.Fatalf("%v, %v", m, err)
	}
	if _, err := p("nothing"); err == nil {
		t.Error("no eval line accepted")
	}
}

func TestParseTicks(t *testing.T) {
	stat := "cpu  10 0 5 100 0 0 0 0 0 0\ncpu0 1 0 1 50 2 0 0 0 0 0\ncpu1 7 1 2 3 4 5 0 0 0 0\nintr 5\n"
	m := ParseTicks(stat)
	if len(m) != 2 || m[0] != 2 || m[1] != 15 {
		t.Errorf("ticks = %v", m)
	}
}

func variants(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDiscover(t *testing.T) {
	dir := variants(t, "a@0.0", "a@32.0", "b@0.0", "b@32.0", "notes.txt")
	vs, err := Discover(dir, nil, nil)
	if err != nil || len(vs) != 4 {
		t.Fatalf("%v, %v", vs, err)
	}
	vs, _ = Discover(dir, []string{"b"}, []string{"32.0"})
	if len(vs) != 1 || vs[0].Side != "b" || vs[0].Pos != "32.0" {
		t.Errorf("filtered = %v", vs)
	}
	if _, err := Discover(dir, []string{"zzz"}, nil); err == nil {
		t.Error("an empty selection accepted")
	}
}

func TestOrderIsRepeatableAndMixed(t *testing.T) {
	dir := variants(t, "a@0.0", "a@0.16", "a@32.0", "a@32.16", "b@0.0", "b@0.16", "b@32.0", "b@32.16")
	vs, _ := Discover(dir, nil, nil)
	o1 := Order(vs, "k", 3)
	o2 := Order(vs, "k", 3)
	if !slices.Equal(o1, o2) {
		t.Error("the order of a round is not repeatable")
	}
	if len(o1) != 8 {
		t.Fatalf("%d processes", len(o1))
	}
	// Both sides of a position come one after the other.
	for i := 0; i < 8; i += 2 {
		if o1[i].Pos != o1[i+1].Pos || o1[i].Side == o1[i+1].Side {
			t.Errorf("pair %d: %v %v", i/2, o1[i], o1[i+1])
		}
	}
	different := false
	for r := 0; r < 8 && !different; r++ {
		different = !slices.Equal(Order(vs, "k", r), o1)
	}
	if !different {
		t.Error("every round has the same order")
	}
	// The side that goes first changes from round to round.
	first := map[string]int{}
	for r := range 40 {
		first[Order(vs, "k", r)[0].Side]++
	}
	if first["a"] == 0 || first["b"] == 0 {
		t.Errorf("first sides: %v", first)
	}
}

type fakeStart struct {
	calls   int
	disturb func(call int) bool // the call's cpu ticks are filled by ticks()
	out     string
	err     error
	seen    []string
	dirs    []string
	envs    [][]string
}

func (f *fakeStart) start(ctx context.Context, bin string, args []string, dir string, env []string) Proc {
	f.calls++
	f.seen = append(f.seen, filepath.Base(bin))
	f.dirs = append(f.dirs, dir)
	f.envs = append(f.envs, env)
	return Proc{Out: f.out, Wall: time.Second, CPU: time.Second, Err: f.err}
}

func TestRunWritesOneRecordPerProcessAndResumes(t *testing.T) {
	dir := variants(t, "a@0.0", "a@32.0", "b@0.0", "b@32.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "BenchmarkShop/p 1 5 ns/op 7.5 p50-ms\nvmhwm-kb: 4096\n"}
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "gobench:p50-ms", Dir: "/work", Env: []string{"A=1"}, CPU: -1, Sib: -1}
	var log bytes.Buffer
	if err := Run(context.Background(), o, f.start, nil, &log); err != nil {
		t.Fatal(err)
	}
	if f.calls != 8 {
		t.Fatalf("%d processes, want 8", f.calls)
	}
	rs, err := ReadRecords(out)
	if err != nil || len(rs) != 8 {
		t.Fatalf("%d records, %v", len(rs), err)
	}
	if rs[0].Metrics["Shop/p"] != 7.5 || rs[0].HWMKB != 4096 || rs[0].Kind != "run" {
		t.Errorf("record = %+v", rs[0])
	}
	if f.dirs[0] != "/work" || !slices.Equal(f.envs[0], []string{"A=1"}) {
		t.Errorf("dir %q env %v", f.dirs[0], f.envs[0])
	}
	// A second call with 3 rounds runs only the third.
	f2 := &fakeStart{out: f.out}
	o.Rounds = 3
	if err := Run(context.Background(), o, f2.start, nil, &log); err != nil {
		t.Fatal(err)
	}
	if f2.calls != 4 {
		t.Errorf("resumed run made %d processes, want 4", f2.calls)
	}
}

func TestRunRunsDisturbedProcessesAgain(t *testing.T) {
	dir := variants(t, "a@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "BenchmarkX 1 1 ns/op 3 ms\n"}
	// The sibling core (1) is busy for 50 ticks during each process, which lasts one second.
	n := 0
	ticks := func() map[int]int64 {
		n++
		return map[int]int64{0: 0, 1: int64(50 * (n / 2))}
	}
	o := RunOptions{Bin: dir, Out: out, Rounds: 1, Parse: "gobench:ms", CPU: 0, Sib: 1, Tries: 3}
	if err := Run(context.Background(), o, f.start, ticks, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	rs, _ := ReadRecords(out)
	if len(rs) != 3 {
		t.Fatalf("%d records, want 3 (the tries)", len(rs))
	}
	for i, r := range rs {
		if r.Attempt != i || !r.Disturbed {
			t.Errorf("record %d: %+v", i, r)
		}
	}
}

func TestRunStopsOnAFailedProcess(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "boom", err: errors.New("exit status 2")}
	o := RunOptions{Bin: dir, Out: out, Rounds: 1, Parse: "jsrun", CPU: -1, Sib: -1}
	err := Run(context.Background(), o, f.start, nil, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exit status 2") {
		t.Fatalf("Run = %v", err)
	}
	if f.calls != 1 {
		t.Errorf("%d processes after the failure", f.calls)
	}
	// Output without the metric is an error too.
	f = &fakeStart{out: "no timing here"}
	os.Remove(out)
	if err := Run(context.Background(), o, f.start, nil, &bytes.Buffer{}); err == nil {
		t.Error("output without an eval line accepted")
	}
}
