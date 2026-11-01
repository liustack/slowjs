// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	stat := "cpu  10 0 5 100 0 0 0 0 0 0\ncpu0 1 0 1 50 2 0 0 0 0 0\ncpu1 7 1 2 3 4 5 0 6 9 9\nintr 5\n"
	m := ParseTicks(stat)
	// cpu1: busy is user, nice, system, irq and softirq (7+1+2+5+0), steal is apart, guest is already in user.
	if len(m) != 2 || m[0] != (CPUTicks{2, 0}) || m[1] != (CPUTicks{15, 6}) {
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
	o1 := Order(vs, "k", 3, 1)
	o2 := Order(vs, "k", 3, 1)
	if fmt.Sprint(o1) != fmt.Sprint(o2) {
		t.Error("the order of a round is not repeatable")
	}
	if len(o1) != 4 {
		t.Fatalf("%d blocks", len(o1))
	}
	for _, b := range o1 {
		if len(b.Sides) != 2 || b.Sides[0].Side == b.Sides[1].Side || b.Sides[0].Pos != b.Pos || b.Sides[1].Pos != b.Pos {
			t.Errorf("block %+v", b)
		}
	}
	different := false
	for r := 0; r < 8 && !different; r++ {
		different = fmt.Sprint(Order(vs, "k", r, 1)) != fmt.Sprint(o1)
	}
	if !different {
		t.Error("every round has the same order")
	}
	if fmt.Sprint(Order(vs, "other experiment", 3, 1)) == fmt.Sprint(o1) {
		t.Error("another experiment repeats the order")
	}
}

func TestOrderBalancesEveryPairOfSides(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0", "c@0.0", "d@0.0", "a@0.8", "b@0.8", "c@0.8", "d@0.8")
	vs, _ := Discover(dir, nil, nil)
	// before[x+y] counts the blocks where x ran before y.
	before := map[string]int{}
	first := map[string]int{}
	for r := range 40 {
		for _, b := range Order(vs, "k", r, 1) {
			first[b.Sides[0].Side]++
			for i := range b.Sides {
				for j := i + 1; j < len(b.Sides); j++ {
					before[b.Sides[i].Side+b.Sides[j].Side]++
				}
			}
		}
	}
	for _, x := range "abcd" {
		for _, y := range "abcd" {
			if x < y && before[string(x)+string(y)] != before[string(y)+string(x)] {
				t.Errorf("%c before %c %d times, the other way %d", x, y, before[string(x)+string(y)], before[string(y)+string(x)])
			}
		}
	}
	if len(first) < 3 {
		t.Errorf("first sides: %v", first)
	}
}

func TestOrderRepeatsTheFormalLayout(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0", "a@0.8", "b@0.8")
	vs, _ := Discover(dir, nil, nil)
	n := map[string]int{}
	reps := map[int]bool{}
	for _, b := range Order(vs, "k", 0, 3) {
		n[b.Pos]++
		if b.Pos == "0.0" {
			reps[b.Rep] = true
		}
	}
	if n["0.0"] != 3 || n["0.8"] != 1 || len(reps) != 3 {
		t.Errorf("blocks %v, reps %v", n, reps)
	}
}

func TestDiscoverWantsEveryPositionOfEverySide(t *testing.T) {
	dir := variants(t, "a@0.0", "a@0.8", "b@0.0")
	if _, err := Discover(dir, nil, nil); err == nil {
		t.Error("a side without a position accepted")
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
	if f.dirs[0] != "/work" || !slices.Contains(f.envs[0], "A=1") {
		t.Errorf("dir %q env %v", f.dirs[0], f.envs[0])
	}
	// A second call with 4 rounds runs only the last two.
	f2 := &fakeStart{out: f.out}
	o.Rounds = 4
	if err := Run(context.Background(), o, f2.start, nil, &log); err != nil {
		t.Fatal(err)
	}
	if f2.calls != 8 {
		t.Errorf("resumed run made %d processes, want 8", f2.calls)
	}
}

func TestRunRunsDisturbedBlocksAgainWhole(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "BenchmarkX 1 1 ns/op 3 ms\n"}
	// The sibling core (1) is busy for 50 ticks during the second process of
	// the first try only; each process lasts one second.
	n := 0
	busy := int64(0)
	ticks := func() map[int]CPUTicks {
		n++
		if n == 4 {
			busy += 50
		}
		return map[int]CPUTicks{0: {}, 1: {Busy: busy}, 2: {Busy: int64(n) * 30}}
	}
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "gobench:ms", CPU: 0, Sib: 1, Tries: 3}
	if err := Run(context.Background(), o, f.start, ticks, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	rs, _ := ReadRecords(out)
	// round 0: a disturbed try of both sides, then a good one; round 1: one try.
	if len(rs) != 6 {
		t.Fatalf("%d records, want 6", len(rs))
	}
	if rs[0].Attempt != 0 || rs[1].Attempt != 0 || !rs[1].Disturbed || rs[0].Disturbed {
		t.Errorf("first try: %+v %+v", rs[0], rs[1])
	}
	if rs[2].Attempt != 1 || rs[3].Attempt != 1 || rs[2].Side != rs[0].Side || rs[2].Disturbed || rs[3].Disturbed {
		t.Errorf("second try: %+v %+v", rs[2], rs[3])
	}
	if rs[0].Order != rs[0].Side+","+rs[1].Side || rs[0].Slot != 0 || rs[1].Slot != 1 || rs[0].Start == "" || rs[0].Session == "" {
		t.Errorf("order, slot, start, session: %+v", rs[0])
	}
	if rs[0].MachinePct != 30 {
		t.Errorf("machine busy %v, want 30", rs[0].MachinePct)
	}
	// The sides run in the other order in round 1.
	if rs[4].Side != rs[1].Side {
		t.Errorf("round 1 starts with %s, round 0 with %s", rs[4].Side, rs[0].Side)
	}
	// A resumed run with nothing left to do runs nothing.
	f2 := &fakeStart{out: f.out}
	if err := Run(context.Background(), o, f2.start, ticks, &bytes.Buffer{}); err != nil || f2.calls != 0 {
		t.Errorf("resumed: %d processes, %v", f2.calls, err)
	}
}

func TestRunKeepsTheLastTryOfABlockThatStaysDisturbed(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "BenchmarkX 1 1 ns/op 3 ms\n"}
	steal := int64(0)
	ticks := func() map[int]CPUTicks {
		steal += 10
		return map[int]CPUTicks{0: {Steal: steal}, 1: {}}
	}
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "gobench:ms", CPU: 0, Sib: 1, Tries: 2}
	if err := Run(context.Background(), o, f.start, ticks, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	rs, _ := ReadRecords(out)
	if len(rs) != 8 {
		t.Fatalf("%d records, want 8 (2 tries of 2 sides in 2 rounds)", len(rs))
	}
	for _, r := range rs {
		if !r.Disturbed || r.StealPct != 10 {
			t.Errorf("%+v", r)
		}
	}
}

func TestRunWantsEvenRounds(t *testing.T) {
	dir := variants(t, "a@0.0")
	o := RunOptions{Bin: dir, Out: filepath.Join(t.TempDir(), "r"), Rounds: 3, Parse: "jsrun", CPU: -1, Sib: -1}
	if err := Run(context.Background(), o, (&fakeStart{}).start, nil, &bytes.Buffer{}); err == nil {
		t.Error("odd rounds accepted")
	}
}

func TestRunStopsOnAFailedProcess(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "boom", err: errors.New("exit status 2")}
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "jsrun", CPU: -1, Sib: -1}
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

// A run writes a manifest, a run that goes on in the file must be the same
// experiment (same binaries, same settings) and adds its rounds, and the
// records carry its id.
func TestRunKeepsAManifest(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "BenchmarkX 1 1 ns/op 3 ms\n"}
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "gobench:ms", CPU: -1, Sib: -1}
	if err := Run(context.Background(), o, f.start, nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m, ok, err := ReadManifest(out)
	if err != nil || !ok || m.Experiment == "" || fmt.Sprint(m.Plan.Rounds) != "[0 1]" || len(m.Binaries) != 2 || m.Binaries["a@0.0"] == "" {
		t.Fatalf("%+v %v %v", m, ok, err)
	}
	rs, _ := ReadRecords(out)
	for _, r := range rs {
		if r.Experiment != m.Experiment {
			t.Fatalf("record of experiment %q", r.Experiment)
		}
	}
	// More rounds: the same experiment, with them added.
	o.Round0, o.Rounds = 2, 2
	if err := Run(context.Background(), o, f.start, nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m2, _, _ := ReadManifest(out)
	if m2.Experiment != m.Experiment || fmt.Sprint(m2.Plan.Rounds) != "[0 1 2 3]" {
		t.Errorf("%+v", m2)
	}
	// The judge reads them as one complete plan.
	rs, _ = ReadRecords(out)
	if _, v := Judge(rs, m2.Plan, m2.Experiment, "a", "b", false, 0.05, 0.01); !v.Complete || v.Units != 2 {
		t.Errorf("%+v", v)
	}
	// A binary replaced under the same name.
	if err := os.WriteFile(filepath.Join(dir, "b@0.0"), []byte("y"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), o, f.start, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "binaries") {
		t.Errorf("a replaced binary: %v", err)
	}
	// Other settings.
	os.WriteFile(filepath.Join(dir, "b@0.0"), []byte("x"), 0o755)
	o.Env = []string{"X=1"}
	if err := Run(context.Background(), o, f.start, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "env") {
		t.Errorf("other settings: %v", err)
	}
}

func TestRunRefusesRecordsWithoutAManifest(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	os.WriteFile(out, []byte(`{"side":"a","pos":"0.0","kind":"run","round":0,"attempt":0,"slot":0,"wall_ms":1,"cpu_ms":1,"sib_pct":0,"other_pct":0,"steal_pct":0,"machine_pct":0}`+"\n"), 0o644)
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "gobench:ms", CPU: -1, Sib: -1}
	if err := Run(context.Background(), o, (&fakeStart{}).start, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no manifest") {
		t.Errorf("%v", err)
	}
}

func TestDiscoverWantsEverythingAskedFor(t *testing.T) {
	// No side has 0.16: it must not drop out of the plan.
	dir := variants(t, "a@0.0", "a@0.8", "b@0.0", "b@0.8")
	if _, err := Discover(dir, nil, []string{"0.0", "0.8", "0.16"}); err == nil || !strings.Contains(err.Error(), "0.16") {
		t.Errorf("a position asked for and missing: %v", err)
	}
	if _, err := Discover(dir, []string{"a", "b", "c"}, nil); err == nil || !strings.Contains(err.Error(), "side c") {
		t.Errorf("a side asked for and missing: %v", err)
	}
	if vs, err := Discover(dir, []string{"a", "b"}, []string{"0.0", "0.8"}); err != nil || len(vs) != 4 {
		t.Errorf("%v, %v", vs, err)
	}
}

func TestEffectiveEnv(t *testing.T) {
	got, err := EffectiveEnv([]string{"PATH=/bin", "GOMAXPROCS=4", "HOME=/h", "SSH_TTY=/dev/pts/1"}, []string{"GOGC=50", "PATH=/usr/bin", "GOGC=100"})
	if err != nil || !slices.Equal(got, []string{"GOGC=100", "HOME=/h", "PATH=/usr/bin"}) {
		t.Errorf("%v, %v", got, err)
	}
	if _, err := EffectiveEnv(nil, []string{"NOVALUE"}); err == nil {
		t.Error("an -env without = accepted")
	}
}

func TestRunRecordsTheEnvironmentAndFolderItUses(t *testing.T) {
	dir := variants(t, "a@0.0", "b@0.0")
	out := filepath.Join(t.TempDir(), "r.jsonl")
	f := &fakeStart{out: "BenchmarkX 1 1 ns/op 3 ms\n"}
	work := t.TempDir()
	t.Chdir(work)
	os.Mkdir("ssr", 0o755)
	t.Setenv("TMPDIR", "/tmp/one")
	t.Setenv("GOMAXPROCS", "3")
	t.Setenv("TIFA_WARM", "5")
	o := RunOptions{Bin: dir, Out: out, Rounds: 2, Parse: "gobench:ms", Dir: "ssr", Env: []string{"GOMAXPROCS=1", "TIFA_WARM=8"}, CPU: -1, Sib: -1}
	var log bytes.Buffer
	if err := Run(context.Background(), o, f.start, nil, &log); err != nil {
		t.Fatal(err)
	}
	m, _, _ := ReadManifest(out)
	real, _ := filepath.EvalSymlinks(work)
	if m.Dir != filepath.Join(real, "ssr") && m.Dir != filepath.Join(work, "ssr") {
		t.Errorf("manifest dir %q", m.Dir)
	}
	// The process gets what the manifest says, nothing more.
	for i := range f.envs {
		if !slices.Equal(f.envs[i], m.Env) || f.dirs[i] != m.Dir {
			t.Fatalf("process %d: dir %q env %v, manifest %q %v", i, f.dirs[i], f.envs[i], m.Dir, m.Env)
		}
	}
	if !slices.Contains(m.Env, "GOMAXPROCS=1") || !slices.Contains(m.Env, "TIFA_WARM=8") || !slices.Contains(m.Env, "TMPDIR=/tmp/one") || slices.Contains(m.Env, "GOMAXPROCS=3") {
		t.Errorf("manifest env %v", m.Env)
	}
	// Going on with another value of what the processes get is refused.
	for name, env := range map[string][]string{
		"GOMAXPROCS": {"GOMAXPROCS=2", "TIFA_WARM=8"},
		"GOGC":       {"GOMAXPROCS=1", "TIFA_WARM=8", "GOGC=50"},
		"TIFA_WARM":  {"GOMAXPROCS=1", "TIFA_WARM=4"},
	} {
		o2 := o
		o2.Env = env
		if err := Run(context.Background(), o2, f.start, nil, &log); err == nil || !strings.Contains(err.Error(), "env") {
			t.Errorf("%s changed: %v", name, err)
		}
	}
	t.Setenv("TMPDIR", "/tmp/two")
	if err := Run(context.Background(), o, f.start, nil, &log); err == nil || !strings.Contains(err.Error(), "env") {
		t.Errorf("an inherited TMPDIR changed: %v", err)
	}
	t.Setenv("TMPDIR", "/tmp/one")
	// The same relative folder from another folder is another folder.
	other := t.TempDir()
	os.Mkdir(filepath.Join(other, "ssr"), 0o755)
	t.Chdir(other)
	if err := Run(context.Background(), o, f.start, nil, &log); err == nil || !strings.Contains(err.Error(), "dir") {
		t.Errorf("another folder: %v", err)
	}
	// What the shell sets and -env does not is not passed, and said so.
	t.Chdir(work)
	t.Setenv("GOGC", "10")
	log.Reset()
	if err := Run(context.Background(), o, f.start, nil, &log); err != nil {
		t.Fatalf("an inherited GOGC the processes do not get: %v", err)
	}
	if !strings.Contains(log.String(), "GOGC") || slices.Contains(f.envs[len(f.envs)-1], "GOGC=10") {
		t.Errorf("log: %s", log.String())
	}
}
