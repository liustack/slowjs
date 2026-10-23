// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command compare builds one jsbench binary per engine, runs every workload
// on each, and writes the results of the run to results/ as JSON and as
// Markdown. Old results are never overwritten.
//
//	go run ./cmd/compare                       # everything, 5 rounds
//	go run ./cmd/compare -phases react,v8      # some workloads
//	go run ./cmd/compare -cpu 1 -sibling 5     # pinned to CPU 1 (Linux), rerun when CPU 5 was busy
//	go run ./cmd/compare -report results/x.json  # Markdown again from a JSON file
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liustack/slowjs/compare/engines/t262"
)

type variant struct {
	Name string   // as in results
	Bin  string   // binary name
	Tags string   // build tags
	Env  []string // extra environment
	// Conformance: run test262 on this variant. Every variant runs it: a
	// JIT can get things wrong that its interpreter gets right.
	Conformance bool
}

var variants = []variant{
	{"liustack-quickjs", "liustack", "eng_liustack", nil, true},
	{"modernc-quickjs", "modernc", "eng_modernc", nil, true},
	{"go-quickjs-interp", "goquickjs", "eng_goquickjs", nil, true},
	{"go-quickjs-jit", "goquickjsjit", "eng_goquickjsjit quickjs_jit", nil, true},
	{"goant-interp", "goant", "eng_goant", nil, true},
	{"goant-jit", "goantjit", "eng_goantjit", nil, true},
	{"moejs", "moejs", "eng_moejs", nil, true},
}

var (
	flagPhases   = flag.String("phases", "features,concurrent,react,micro,v8,upstream,cold,mem,t262", "workloads to run")
	flagEngines  = flag.String("engines", "", "comma-separated engines (default all)")
	flagRounds   = flag.Int("rounds", 5, "rounds of each timed workload, engines take turns in a different order each round")
	flagCPU      = flag.String("cpu", "", "run every engine process under taskset -c <cpu> (Linux)")
	flagSibling  = flag.Int("sibling", -1, "the hyperthread sibling of -cpu; a run is repeated when another process kept it busy for 3 ticks or more and over 5% of the run")
	flagProcs    = flag.Int("gomaxprocs", 1, "GOMAXPROCS of timed workloads")
	flagOut      = flag.String("out", "results", "where results go")
	flagRaw      = flag.String("raw", "", "directory for raw output (default results/raw/<run>)")
	flagT262     = flag.String("test262", "", "test262 checkout (default: download the pinned commit into the cache)")
	flagJobs     = flag.Int("j", 1, "test262 processes per engine (each under -memcap-test262)")
	flagReport   = flag.String("report", "", "write Markdown for an existing results JSON and exit")
	flagMachine  = flag.String("machine", "", "machine description for the results (default: detected)")
	flagRun      = flag.String("name", "", "run name (default <date>-<liustack version>-<host>)")
	flagNoBuild  = flag.Bool("nobuild", false, "use the binaries in bin/")
	flagReactN   = flag.Int("react-n", 200, "measured renders per round")
	flagBuildOS  = flag.String("goos", "", "build the engine binaries for this GOOS (with -build-only, to run them on another machine)")
	flagBuildArc = flag.String("goarch", "", "build the engine binaries for this GOARCH")
	flagBuildOnl = flag.Bool("build-only", false, "build the engine binaries and exit")
	flagBinDir   = flag.String("bindir", "bin", "where the engine binaries are")
	flagCPU262   = flag.String("cpu-test262", "", "taskset -c for test262 processes (default -cpu); test262 needs no quiet core")
	flagNice262  = flag.Int("nice-test262", 0, "run test262 processes under nice -n <n>")
	flagMerge    = flag.String("merge", "", "copy the test262 results of this results JSON into the run named by -name, and exit")
	flagMergeEng = flag.Bool("merge-engines", false, "with -merge: copy every result of the engines in that JSON (made by other binaries too) into the run, replacing what the run has for them")
	flagAdd      = flag.Bool("add", false, "add to the existing run named by -name: timed workloads get -rounds more rounds, other phases are replaced. Splits a long run into short ones")
)

// Pinned inputs.
const (
	test262Commit = "d53ebc8b1931d9f1541e43d59650776e4b766248"
	v8Commit      = "d2ddc6ce1ce4f8d90606864615543708d1cfa3fd" // mozilla/arewefastyet benchmarks/v8-v7
)

func main() {
	flag.Parse()
	if *flagReport != "" {
		var res Results
		b, err := os.ReadFile(*flagReport)
		check(err)
		check(json.Unmarshal(b, &res))
		md := strings.TrimSuffix(*flagReport, ".json") + ".md"
		check(os.WriteFile(md, []byte(renderMarkdown(&res)), 0o644))
		fmt.Println("wrote", md)
		return
	}
	if *flagMerge != "" {
		merge(*flagMerge)
		return
	}
	vs := selected()
	if !*flagNoBuild {
		build(vs)
	}
	if *flagBuildOnl {
		return
	}
	res := &Results{Started: time.Now().UTC().Format(time.RFC3339), Machine: machine(), Rounds: *flagRounds,
		Pinned: map[string]string{"test262": test262Commit, "v8-v7 (mozilla/arewefastyet)": v8Commit}}
	res.GOMAXPROCS, res.CPU = *flagProcs, *flagCPU
	if b, err := os.ReadFile(filepath.Join(*flagBinDir, "build.json")); err == nil {
		check(json.Unmarshal(b, &res.Source))
	}
	res.Name = *flagRun
	if *flagAdd {
		if res.Name == "" {
			check(errors.New("-add needs -name"))
		}
		b, err := os.ReadFile(filepath.Join(*flagOut, res.Name+".json"))
		check(err)
		prev := &Results{}
		check(json.Unmarshal(b, prev))
		if prev.Source.Commit != res.Source.Commit || prev.CPU != res.CPU || prev.GOMAXPROCS != res.GOMAXPROCS {
			check(fmt.Errorf("-add: run %s was made with other binaries or settings", res.Name))
		}
		res = prev
	}
	if d := driverCommit(); d != "" && d != res.Source.Commit && !slices.Contains(res.Drivers, d) {
		res.Drivers = append(res.Drivers, d)
	}
	res.MemCap = capDesc()
	if res.Name == "" {
		// The version of liustack/slowjs: the tag of the commit the
		// binaries were built from.
		ver := strings.SplitN(res.Source.Describe, "-", 2)[0]
		if ver == "" || ver == "unknown" {
			ver = "unknown"
		}
		host, _ := os.Hostname()
		res.Name = time.Now().Format("2006-01-02") + "-" + ver + "-" + strings.Split(host, ".")[0]
	}
	rawDir = *flagRaw
	if rawDir == "" {
		rawDir = filepath.Join(*flagOut, "raw", res.Name)
	}
	check(os.MkdirAll(rawDir, 0o755))
	for _, v := range vs {
		var in Info
		check(json.Unmarshal(runJSON(v, false, "info"), &in))
		res.Engines = slices.DeleteFunc(res.Engines, func(e Info) bool { return e.Engine == in.Engine })
		res.Engines = append(res.Engines, in)
	}
	order := func(name string) int {
		return slices.IndexFunc(variants, func(v variant) bool { return v.Name == name })
	}
	slices.SortStableFunc(res.Engines, func(a, b Info) int { return order(a.Engine) - order(b.Engine) })
	phases := strings.Split(*flagPhases, ",")
	has := func(p string) bool { return slices.Contains(phases, p) }
	save := func() {
		res.Finished = time.Now().UTC().Format(time.RFC3339)
		b, _ := json.MarshalIndent(res, "", " ")
		p := filepath.Join(*flagOut, res.Name+".json")
		check(os.WriteFile(p, b, 0o644))
		check(os.WriteFile(strings.TrimSuffix(p, ".json")+".md", []byte(renderMarkdown(res)), 0o644))
	}
	if has("features") {
		res.Features = runFeatures(vs)
		save()
	}
	if has("concurrent") {
		res.Concurrent = map[string]json.RawMessage{}
		for _, v := range vs {
			res.Concurrent[v.Name] = runJSON(v, false, "concurrent")
		}
		save()
	}
	timed := []string{}
	for _, p := range []string{"react", "micro", "v8", "upstream", "cold", "mem"} {
		if has(p) {
			timed = append(timed, p)
		}
	}
	if len(timed) > 0 {
		if res.Timed == nil {
			res.Timed = map[string]map[string][]json.RawMessage{}
			res.Disturbed = map[string]int{}
		}
		// With -add, rounds go after the rounds a workload already has, and
		// the engine order carries on from them.
		done := -1
		for _, p := range timed {
			for _, j := range jobs(p) {
				for _, v := range vs {
					if n := len(res.Timed[j.key][v.Name]); done < 0 || n < done {
						done = n
					}
				}
			}
		}
		for r := 0; r < *flagRounds; r++ {
			order := rotate(vs, done+r)
			for _, p := range timed {
				for _, job := range jobs(p) {
					for _, v := range order {
						out := timedRun(v, job, res)
						key := job.key
						if res.Timed[key] == nil {
							res.Timed[key] = map[string][]json.RawMessage{}
						}
						res.Timed[key][v.Name] = append(res.Timed[key][v.Name], out)
					}
				}
			}
			fmt.Fprintf(os.Stderr, "round %d done\n", done+r+1)
			save()
		}
	}
	if has("t262") {
		root := *flagT262
		if root == "" {
			root = fetchTest262()
		}
		t := runTest262(vs, root)
		if res.Test262 != nil && res.Test262.Commit == t.Commit {
			// -add with some engines: keep the others.
			for n, e := range t.Engines {
				res.Test262.Engines[n] = e
			}
		} else {
			res.Test262 = t
		}
		save()
	}
	save()
	fmt.Println("wrote", filepath.Join(*flagOut, res.Name+".json"))
}

var rawDir string

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "compare:", err)
		os.Exit(1)
	}
}

func selected() []variant {
	if *flagEngines == "" {
		return variants
	}
	var out []variant
	for _, n := range strings.Split(*flagEngines, ",") {
		found := false
		for _, v := range variants {
			if v.Name == n || v.Bin == n {
				out, found = append(out, v), true
			}
		}
		if !found {
			check(fmt.Errorf("no engine %q", n))
		}
	}
	return out
}

func rotate(vs []variant, r int) []variant {
	out := make([]variant, len(vs))
	for i := range vs {
		out[i] = vs[(i+r)%len(vs)]
	}
	if r%2 == 1 {
		slices.Reverse(out)
	}
	return out
}

// buildInfo is written next to the binaries, so that a run on another
// machine knows which commit of this repository they come from.
type buildInfo struct {
	Commit   string `json:"commit"`
	Describe string `json:"describe"`
	Built    string `json:"built"`
}

func gitOut(args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", "../.."}, args...)...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func build(vs []variant) {
	check(os.MkdirAll(*flagBinDir, 0o755))
	bi := buildInfo{Commit: gitOut("rev-parse", "HEAD"), Describe: gitOut("describe", "--tags", "--always", "--dirty"), Built: time.Now().UTC().Format(time.RFC3339)}
	b, _ := json.MarshalIndent(bi, "", " ")
	check(os.WriteFile(filepath.Join(*flagBinDir, "build.json"), b, 0o644))
	for _, v := range vs {
		fmt.Fprintln(os.Stderr, "build", v.Bin)
		cmd := exec.Command("go", "build", "-pgo=off", "-trimpath", "-tags", v.Tags, "-o", filepath.Join(*flagBinDir, v.Bin), "./cmd/jsbench")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if *flagBuildOS != "" {
			cmd.Env = append(cmd.Env, "GOOS="+*flagBuildOS)
		}
		if *flagBuildArc != "" {
			cmd.Env = append(cmd.Env, "GOARCH="+*flagBuildArc)
		}
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		check(cmd.Run())
	}
}

func command(v variant, pinned bool, args ...string) *exec.Cmd {
	bin := filepath.Join(*flagBinDir, v.Bin)
	// Every engine process stays on -cpu, so that a run on a shared machine
	// uses no other core. GOMAXPROCS is set for timed workloads only.
	cpu, nice := *flagCPU, 0
	limit := parseSize(*flagMemCap)
	if len(args) > 0 && args[0] == "t262" {
		limit = parseSize(*flagMemCap262)
		nice = *flagNice262
		if *flagCPU262 != "" {
			cpu = *flagCPU262
		}
	}
	a := argv(limit, cpu, nice, bin, args)
	cmd := exec.Command(a[0], a[1:]...)
	cmd.Env = append(os.Environ(), v.Env...)
	if pinned {
		cmd.Env = append(cmd.Env, "GOMAXPROCS="+strconv.Itoa(*flagProcs))
	}
	return cmd
}

func runJSON(v variant, pinned bool, args ...string) json.RawMessage {
	return runJSONTimeout(v, pinned, 30*time.Minute, args...)
}

func runJSONTimeout(v variant, pinned bool, limit time.Duration, args ...string) json.RawMessage {
	cmd := command(v, pinned, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, why, err := outputTimeout(cmd, parseSize(*flagMemCap), limit)
	appendRaw(v.Name, args, out, stderr.String())
	if why == "memory" {
		b, _ := json.Marshal(map[string]string{"crash": outOfMemory(parseSize(*flagMemCap))})
		return b
	}
	if err != nil {
		tail := stderr.String()
		if i := strings.Index(tail, "fatal error"); i >= 0 {
			tail = tail[i:]
		} else if i := strings.Index(tail, "panic:"); i >= 0 {
			tail = tail[i:]
		}
		if len(tail) > 400 {
			tail = tail[:400]
		}
		b, _ := json.Marshal(map[string]string{"crash": strings.TrimSpace(err.Error() + ": " + tail)})
		return b
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return json.RawMessage(lines[len(lines)-1])
}

// outputTimeout runs cmd under the memory cap mem and kills it after limit.
func outputTimeout(cmd *exec.Cmd, mem int64, limit time.Duration) ([]byte, string, error) {
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	g := newGuard(cmd, mem)
	t := time.AfterFunc(limit, func() { g.kill("timeout") })
	err := cmd.Wait()
	t.Stop()
	if ps := cmd.ProcessState; ps != nil {
		childCPU.Add(int64(ps.UserTime() + ps.SystemTime()))
	}
	why := g.why()
	if why == "timeout" {
		err = fmt.Errorf("killed after %v", limit)
	}
	return []byte(out.String()), why, err
}

var rawMu sync.Mutex

func appendRaw(engine string, args []string, out []byte, stderr string) {
	rawMu.Lock()
	defer rawMu.Unlock()
	f, err := os.OpenFile(filepath.Join(rawDir, "runs.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	check(err)
	defer f.Close()
	rec := map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "engine": engine, "args": args,
		"stdout": strings.TrimSpace(string(out))}
	if s := strings.TrimSpace(stderr); s != "" {
		if len(s) > 2000 {
			s = s[:2000]
		}
		rec["stderr"] = s
	}
	b, _ := json.Marshal(rec)
	f.Write(append(b, '\n'))
}

// ---- features ----

type featureTable struct {
	Node     string `json:"node"`
	ICU      string `json:"icu"`
	Features []struct {
		ID      string `json:"id"`
		Edition string `json:"edition"`
		Kind    string `json:"kind"`
		Src     string `json:"src"`
		Expect  string `json:"expect"`
	} `json:"features"`
}

var goProbes = []struct{ ID, Edition string }{
	{"tail-call", "ES2015"},
	{"recursion-depth", "Limits"},
	{"recursion-overflow", "Limits"},
	{"interrupt", "Limits"},
	{"interrupt-uncatchable", "Limits"},
	{"memory-limit", "Limits"},
}

type FeatureRow struct {
	ID      string                     `json:"id"`
	Edition string                     `json:"edition"`
	Src     string                     `json:"src,omitempty"`
	Expect  string                     `json:"expect,omitempty"`
	Results map[string]json.RawMessage `json:"results"`
}

func runFeatures(vs []variant) []FeatureRow {
	b, err := os.ReadFile("features/features.json")
	check(err)
	var ft featureTable
	check(json.Unmarshal(b, &ft))
	var rows []FeatureRow
	for _, f := range ft.Features {
		rows = append(rows, FeatureRow{ID: f.ID, Edition: f.Edition, Src: f.Src, Expect: f.Expect})
	}
	for _, p := range goProbes {
		rows = append(rows, FeatureRow{ID: p.ID, Edition: p.Edition})
	}
	for i := range rows {
		rows[i].Results = map[string]json.RawMessage{}
		for _, v := range vs {
			rows[i].Results[v.Name] = runJSONTimeout(v, false, time.Minute, "feature", "-id", rows[i].ID)
		}
		fmt.Fprintf(os.Stderr, "feature %s\n", rows[i].ID)
	}
	return rows
}

// ---- timed workloads ----

type job struct {
	key  string
	args []string
}

func jobs(phase string) []job {
	switch phase {
	case "react":
		return []job{{"react", []string{"react", "-n", strconv.Itoa(*flagReactN), "-warm", "50"}}}
	case "micro":
		var js []job
		for _, n := range []string{"json-parse", "json-stringify", "string-concat", "regexp"} {
			js = append(js, job{"micro/" + n, []string{"micro", "-name", n, "-n", "30", "-warm", "5"}})
		}
		return js
	case "v8":
		return []job{{"v8", []string{"v8"}}}
	case "upstream":
		return []job{{"upstream", []string{"upstream", "-budget", "100ms"}}}
	case "cold":
		return []job{{"cold", []string{"cold", "-n", "30"}}}
	case "mem":
		return []job{{"mem", []string{"mem", "-k", "100"}}}
	}
	return nil
}

// timedRun runs a job pinned, and runs it again, up to 3 tries in all,
// when the sibling hyperthread was busy during the run or the process
// waited for its CPU.
func timedRun(v variant, j job, res *Results) json.RawMessage {
	var out json.RawMessage
	for try := 0; try < 3; try++ {
		before := cpuTimes(*flagSibling)
		load0 := loadavg()
		childCPU.Store(0)
		start := time.Now()
		if j.key == "upstream" {
			out = runUpstreamSplit(v, j.args)
		} else {
			out = runJSON(v, true, j.args...)
		}
		wall := time.Since(start)
		cpu := time.Duration(childCPU.Load())
		ticks, busy := siblingBusy(before, cpuTimes(*flagSibling))
		rec := map[string]any{"engine": v.Name, "job": j.key, "try": try, "loadavg_before": load0, "loadavg_after": loadavg(),
			"sibling_busy": busy, "sibling_ticks": ticks, "seconds": wall.Seconds(), "cpu_seconds": cpu.Seconds()}
		b, _ := json.Marshal(rec)
		rawMu.Lock()
		f, err := os.OpenFile(filepath.Join(rawDir, "load.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		check(err)
		f.Write(append(b, '\n'))
		f.Close()
		rawMu.Unlock()
		if !disturbed(ticks, busy) && !preempted(cpu, wall) {
			break
		}
		res.Disturbed[v.Name+" "+j.key]++
		fmt.Fprintf(os.Stderr, "rerun %s %s: sibling busy %.0f ticks, %.0f%%; process CPU %v of %v\n", v.Name, j.key, ticks, busy*100, cpu.Round(time.Millisecond), wall.Round(time.Millisecond))
		time.Sleep(10 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "%-16s %-20s done\n", v.Name, j.key)
	return out
}

// driverCommit is the commit this command was built from, as the Go
// toolchain records it.
func driverCommit() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return ""
	}
	return rev + dirty
}

// upstreamDir is where the QuickJS micro benchmarks are, as jsbench's
// default -dir sees it.
const upstreamDir = "../testdata"

// runUpstreamSplit runs each QuickJS micro benchmark in a process of its own
// and merges the results, so that a test that goes over the memory cap (goant
// needs about 5 GiB for string_build_large2) loses only that test. Each
// process gets a directory holding a link to its one test.
func runUpstreamSplit(v variant, args []string) json.RawMessage {
	files, err := filepath.Glob(filepath.Join(upstreamDir, "*.js"))
	check(err)
	tests := map[string]json.RawMessage{}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".js")
		if strings.HasPrefix(name, ".") {
			continue
		}
		dir := filepath.Join(rawDir, "upstream-one", name)
		check(os.MkdirAll(dir, 0o755))
		link := filepath.Join(dir, name+".js")
		abs, err := filepath.Abs(f)
		check(err)
		if _, err := os.Lstat(link); err != nil {
			check(os.Symlink(abs, link))
		}
		out := runJSON(v, true, append(append([]string{}, args...), "-dir", dir)...)
		var r struct {
			Tests map[string]json.RawMessage `json:"tests"`
			Crash string                     `json:"crash"`
		}
		if json.Unmarshal(out, &r) != nil || r.Tests[name] == nil {
			msg := r.Crash
			if msg == "" {
				msg = "no result: " + string(out)
			}
			b, _ := json.Marshal(map[string]string{"err": msg})
			tests[name] = b
			continue
		}
		tests[name] = r.Tests[name]
	}
	b, _ := json.Marshal(map[string]any{"tests": tests})
	return b
}

func loadavg() string {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(b))[:3], " ")
}

// cpuTimes returns busy and total jiffies of a CPU from /proc/stat.
func cpuTimes(cpu int) [2]float64 {
	if cpu < 0 {
		return [2]float64{}
	}
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return [2]float64{}
	}
	prefix := "cpu" + strconv.Itoa(cpu) + " "
	for _, ln := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(ln, prefix) {
			continue
		}
		f := strings.Fields(ln)[1:]
		var t, idle float64
		for i, x := range f {
			n, _ := strconv.ParseFloat(x, 64)
			t += n
			if i == 3 || i == 4 { // idle, iowait
				idle += n
			}
		}
		return [2]float64{t - idle, t}
	}
	return [2]float64{}
}

// siblingBusy returns how many clock ticks the sibling spent busy between
// two readings, and that as a share of all its ticks.
func siblingBusy(a, b [2]float64) (ticks, share float64) {
	if b[1] <= a[1] {
		return 0, 0
	}
	ticks = b[0] - a[0]
	return ticks, ticks / (b[1] - a[1])
}

// disturbed says whether a run must be repeated. A run counts as disturbed
// when the sibling was busy for at least 3 ticks (30 ms at the usual 100 Hz)
// and for more than 5% of the time. The tick floor keeps one stray tick from
// failing a run that lasts a fraction of a second.
func disturbed(ticks, share float64) bool {
	return ticks >= 3 && share > 0.05
}

// merge copies the test262 engines of another run into the run named by
// -name. test262 can then run on another core, as its own run, while timed
// workloads run on the quiet one.
func merge(from string) {
	read := func(p string) *Results {
		b, err := os.ReadFile(p)
		check(err)
		r := &Results{}
		check(json.Unmarshal(b, r))
		return r
	}
	if *flagRun == "" {
		check(errors.New("-merge needs -name"))
	}
	p := filepath.Join(*flagOut, *flagRun+".json")
	res, src := read(p), read(from)
	if *flagMergeEng {
		mergeEngines(res, src)
		b, _ := json.MarshalIndent(res, "", " ")
		check(os.WriteFile(p, b, 0o644))
		check(os.WriteFile(strings.TrimSuffix(p, ".json")+".md", []byte(renderMarkdown(res)), 0o644))
		fmt.Println("merged the engines of", from, "into", p)
		return
	}
	if src.Test262 == nil {
		check(fmt.Errorf("-merge: %s has no test262 results", from))
	}
	if src.Source.Commit != res.Source.Commit {
		check(fmt.Errorf("-merge: %s was made with other binaries", from))
	}
	if res.Test262 == nil || res.Test262.Commit != src.Test262.Commit {
		res.Test262 = src.Test262
	} else {
		for n, e := range src.Test262.Engines {
			res.Test262.Engines[n] = e
		}
	}
	b, _ := json.MarshalIndent(res, "", " ")
	check(os.WriteFile(p, b, 0o644))
	check(os.WriteFile(strings.TrimSuffix(p, ".json")+".md", []byte(renderMarkdown(res)), 0o644))
	fmt.Println("merged test262 of", from, "into", p)
}

// mergeEngines copies everything src has for its engines into res: their
// info, feature probes, concurrent run, timed rounds, repeats and test262. An
// engine added to a run after it was made comes from other binaries, so res
// records the run and commit it came from.
func mergeEngines(res, src *Results) {
	order := func(name string) int {
		return slices.IndexFunc(variants, func(v variant) bool { return v.Name == name })
	}
	var names []string
	for _, in := range src.Engines {
		names = append(names, in.Engine)
		res.Engines = slices.DeleteFunc(res.Engines, func(e Info) bool { return e.Engine == in.Engine })
		res.Engines = append(res.Engines, in)
	}
	slices.SortStableFunc(res.Engines, func(a, b Info) int { return order(a.Engine) - order(b.Engine) })
	for _, sf := range src.Features {
		i := slices.IndexFunc(res.Features, func(f FeatureRow) bool { return f.ID == sf.ID })
		if i < 0 {
			res.Features = append(res.Features, FeatureRow{ID: sf.ID, Edition: sf.Edition, Src: sf.Src, Expect: sf.Expect, Results: map[string]json.RawMessage{}})
			i = len(res.Features) - 1
		}
		for _, n := range names {
			if r, ok := sf.Results[n]; ok {
				res.Features[i].Results[n] = r
			}
		}
	}
	for _, n := range names {
		if c, ok := src.Concurrent[n]; ok {
			if res.Concurrent == nil {
				res.Concurrent = map[string]json.RawMessage{}
			}
			res.Concurrent[n] = c
		}
		for w, byEngine := range src.Timed {
			if rs, ok := byEngine[n]; ok {
				if res.Timed == nil {
					res.Timed = map[string]map[string][]json.RawMessage{}
				}
				if res.Timed[w] == nil {
					res.Timed[w] = map[string][]json.RawMessage{}
				}
				res.Timed[w][n] = rs
			}
		}
		for k := range res.Disturbed {
			if strings.HasPrefix(k, n+" ") {
				delete(res.Disturbed, k)
			}
		}
		for k, c := range src.Disturbed {
			if strings.HasPrefix(k, n+" ") {
				if res.Disturbed == nil {
					res.Disturbed = map[string]int{}
				}
				res.Disturbed[k] = c
			}
		}
		if src.Test262 != nil && src.Test262.Engines[n] != nil {
			if res.Test262 == nil || res.Test262.Commit != src.Test262.Commit {
				check(fmt.Errorf("-merge-engines: test262 of %s is not at the run's commit", n))
			}
			res.Test262.Engines[n] = src.Test262.Engines[n]
		}
	}
	res.Added = slices.DeleteFunc(res.Added, func(a Added) bool { return slices.Equal(a.Engines, names) })
	res.Added = append(res.Added, Added{Engines: names, Run: src.Name, Source: src.Source, Started: src.Started, Finished: src.Finished})
}

// childCPU adds up the CPU time of the engine processes a timed run starts.
var childCPU atomic.Int64

// preempted says whether a timed run's processes waited for their CPU: the
// run is single-threaded and pinned, so its CPU time is close to its wall
// time unless another process ran on the same CPU. A run counts as
// preempted when it lost at least 50 ms and more than 10% of its time.
func preempted(cpu, wall time.Duration) bool {
	if cpu <= 0 {
		return false
	}
	lost := wall - cpu
	return lost >= 50*time.Millisecond && lost > wall/10
}

// ---- test262 ----

func fetchTest262() string {
	cache, err := os.UserCacheDir()
	check(err)
	dir := filepath.Join(cache, "jsbench", "test262-"+test262Commit)
	if _, err := os.Stat(filepath.Join(dir, "harness", "assert.js")); err == nil {
		return dir
	}
	check(os.MkdirAll(filepath.Dir(dir), 0o755))
	tgz := dir + ".tar.gz"
	fmt.Fprintln(os.Stderr, "download test262", test262Commit)
	check(exec.Command("curl", "-sSfL", "-o", tgz, "https://codeload.github.com/tc39/test262/tar.gz/"+test262Commit).Run())
	check(exec.Command("tar", "xzf", tgz, "-C", filepath.Dir(dir)).Run())
	os.Remove(tgz)
	return dir
}

type T262Engine struct {
	Runs    int `json:"runs"`
	Pass    int `json:"pass"`
	Crashes int `json:"crashes"`
	Timeout int `json:"timeouts"`
	OOM     int `json:"out_of_memory"` // tests that went over -memcap-test262 when run alone
	// MemRestarts counts processes that went over the cap while the test
	// they were on passed or failed normally when run alone: memory the
	// engine kept from earlier tests.
	MemRestarts int64             `json:"memory_restarts"`
	Areas       map[string][2]int `json:"areas"` // area -> [pass, runs]
	// ES2023 counts the runs of tests that use no feature newer than
	// ES2023 (t262.Newer), by top directory: [pass, runs].
	ES2023  map[string][2]int `json:"es2023"`
	Feature map[string][2]int `json:"features"` // test262 feature tag -> [pass, runs]
}

type T262Summary struct {
	Commit   string                 `json:"commit"`
	Dirs     []string               `json:"dirs"`
	Files    int                    `json:"files"`
	Excluded map[string]int         `json:"excluded"`
	Runs     int                    `json:"runs"`
	Engines  map[string]*T262Engine `json:"engines"`
}

func runTest262(vs []variant, root string) *T262Summary {
	plans, err := t262.Collect(root)
	check(err)
	sum := &T262Summary{Commit: test262Commit, Dirs: t262.Dirs, Files: len(plans), Excluded: map[string]int{}, Engines: map[string]*T262Engine{}}
	var lines []string
	features := map[string][]string{}
	for _, p := range plans {
		if p.Exclude != "" {
			sum.Excluded[p.Exclude]++
			continue
		}
		b, _ := os.ReadFile(filepath.Join(root, p.Path))
		features[p.Path] = t262.Parse(string(b)).Features
		for _, m := range p.Modes {
			lines = append(lines, m+" "+p.Path)
		}
	}
	sum.Runs = len(lines)
	for _, v := range vs {
		if !v.Conformance {
			continue
		}
		fmt.Fprintf(os.Stderr, "test262 %s: %d runs\n", v.Name, len(lines))
		memRestarts.Store(0)
		results := supervise262(v, root, lines)
		e := &T262Engine{Areas: map[string][2]int{}, Feature: map[string][2]int{}, ES2023: map[string][2]int{}}
		f, err := os.Create(filepath.Join(rawDir, "test262-"+v.Name+".jsonl"))
		check(err)
		w := bufio.NewWriter(f)
		for _, r := range results {
			b, _ := json.Marshal(r)
			w.Write(append(b, '\n'))
			e.Runs++
			if r.Pass {
				e.Pass++
			}
			if strings.HasPrefix(r.Err, "crash") {
				e.Crashes++
			}
			if strings.HasPrefix(r.Err, "timeout") {
				e.Timeout++
			}
			if strings.HasPrefix(r.Err, "out of memory") {
				e.OOM++
			}
			a := t262.Area(r.Path)
			c := e.Areas[a]
			c[1]++
			if r.Pass {
				c[0]++
			}
			e.Areas[a] = c
			if !t262.IsNewer(features[r.Path]) {
				top := strings.SplitN(a, "/", 2)[0]
				for _, k := range []string{top, "all"} {
					c := e.ES2023[k]
					c[1]++
					if r.Pass {
						c[0]++
					}
					e.ES2023[k] = c
				}
			}
			for _, ft := range features[r.Path] {
				c := e.Feature[ft]
				c[1]++
				if r.Pass {
					c[0]++
				}
				e.Feature[ft] = c
			}
		}
		w.Flush()
		f.Close()
		e.MemRestarts = memRestarts.Load()
		sum.Engines[v.Name] = e
	}
	return sum
}

type t262Result struct {
	Path string  `json:"t"`
	Mode string  `json:"m"`
	Pass bool    `json:"ok"`
	Err  string  `json:"e,omitempty"`
	MS   float64 `json:"ms"`
}

// supervise262 runs the list in -j processes. A process that dies or stops
// reporting for 60 s is killed; the test it was on is recorded as a crash
// or a timeout, and a new process carries on after it.
func supervise262(v variant, root string, lines []string) []t262Result {
	n := *flagJobs
	chunks := make([][]string, n)
	for i, l := range lines {
		chunks[i%n] = append(chunks[i%n], l)
	}
	results := make([][]t262Result, n)
	var wg sync.WaitGroup
	for i := range chunks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = superviseChunk(v, root, chunks[i], i)
		}(i)
	}
	wg.Wait()
	var all []t262Result
	for _, r := range results {
		all = append(all, r...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Path != all[j].Path {
			return all[i].Path < all[j].Path
		}
		return all[i].Mode < all[j].Mode
	})
	return all
}

func superviseChunk(v variant, root string, lines []string, id int) []t262Result {
	var out []t262Result
	listFile := filepath.Join(rawDir, fmt.Sprintf("t262-%s-%d.list", v.Bin, id))
	defer os.Remove(listFile)
	mem := parseSize(*flagMemCap262)
	for len(lines) > 0 {
		got, why, detail := runChunk(v, root, listFile, lines)
		out = append(out, got...)
		lines = lines[len(got):]
		if len(lines) == 0 {
			break
		}
		mode, path, _ := strings.Cut(lines[0], " ")
		var r t262Result
		switch why {
		case "timeout":
			r = t262Result{Path: path, Mode: mode, Err: "timeout: no result in 60 s"}
		case "memory":
			// The process may have grown over earlier tests. Run this
			// test alone in a new process before blaming it.
			again, why2, _ := runChunk(v, root, listFile, lines[:1])
			switch {
			case len(again) == 1:
				r = again[0]
				memRestarts.Add(1)
			case why2 == "memory":
				r = t262Result{Path: path, Mode: mode, Err: outOfMemory(mem)}
			default:
				r = t262Result{Path: path, Mode: mode, Err: "crash: the test alone ended with " + why2 + " after its process went over the memory cap"}
			}
		default:
			r = t262Result{Path: path, Mode: mode, Err: "crash: " + detail}
		}
		out = append(out, r)
		lines = lines[1:]
		if len(out)%1000 == 0 {
			fmt.Fprintf(os.Stderr, "  %s/%d: %d done\n", v.Name, id, len(out))
		}
	}
	return out
}

// memRestarts counts test262 processes that went over the memory cap while
// the test they were on passed or failed normally when run alone.
var memRestarts atomic.Int64

// runChunk runs lines in one process until it ends. It returns the results
// it got, why the process ended early ("", "timeout", "memory" or "crash")
// and, for a crash, the end of its stderr.
func runChunk(v variant, root, listFile string, lines []string) ([]t262Result, string, string) {
	check(os.WriteFile(listFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	cmd := command(v, false, "t262", "-root", root, "-list", listFile, "-timeout", "10s")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	check(err)
	check(cmd.Start())
	g := newGuard(cmd, parseSize(*flagMemCap262))
	got := make(chan t262Result)
	readDone := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pipe)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var r t262Result
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				got <- r
			}
		}
		close(readDone)
	}()
	var out []t262Result
loop:
	for {
		select {
		case r := <-got:
			out = append(out, r)
		case <-readDone:
			break loop
		case <-time.After(60 * time.Second):
			g.kill("timeout")
			<-readDone
			break loop
		}
	}
	werr := cmd.Wait()
	why := g.why()
	if len(out) == len(lines) || why != "" {
		return out, why, ""
	}
	tail := stderr.String()
	if i := strings.Index(tail, "fatal error"); i >= 0 {
		tail = tail[i:]
	} else if i := strings.Index(tail, "panic:"); i >= 0 {
		tail = tail[i:]
	}
	if len(tail) > 200 {
		tail = tail[:200]
	}
	return out, "crash", fmt.Sprint(werr) + " " + strings.ReplaceAll(tail, "\n", " ")
}

var _ = errors.New

// ---- machine ----

func machine() Machine {
	m := Machine{Go: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, NumCPU: runtime.NumCPU(), Desc: *flagMachine}
	m.Host, _ = os.Hostname()
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(ln, "model name") {
				m.CPU = strings.TrimSpace(strings.SplitN(ln, ":", 2)[1])
				break
			}
		}
	} else if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		m.CPU = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("uname", "-sr").Output(); err == nil {
		m.Kernel = strings.TrimSpace(string(out))
	}
	return m
}
