// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Results is one run of the comparison, as stored in results/<name>.json.
type Results struct {
	Name       string                                  `json:"name"`
	Started    string                                  `json:"started"`
	Finished   string                                  `json:"finished"`
	Machine    Machine                                 `json:"machine"`
	Source     buildInfo                               `json:"source"`            // the commit of this repository the binaries come from
	Drivers    []string                                `json:"drivers,omitempty"` // commits of the compare command that ran the run, when they differ from Source
	CPU        string                                  `json:"pinned_cpu,omitempty"`
	GOMAXPROCS int                                     `json:"gomaxprocs"`
	Rounds     int                                     `json:"rounds"`
	Pinned     map[string]string                       `json:"pinned_inputs"`
	Engines    []Info                                  `json:"engines"`
	Features   []FeatureRow                            `json:"features,omitempty"`
	Concurrent map[string]json.RawMessage              `json:"concurrent,omitempty"`
	Timed      map[string]map[string][]json.RawMessage `json:"timed,omitempty"` // workload -> engine -> one result per round
	Disturbed  map[string]int                          `json:"disturbed_reruns,omitempty"`
	MemCap     string                                  `json:"memory_cap"`
	Test262    *T262Summary                            `json:"test262,omitempty"`
	Added      []Added                                 `json:"added,omitempty"` // engines merged in from runs made later
}

// Added is a set of engines merged into a run from another run (-merge-engines).
type Added struct {
	Engines  []string  `json:"engines"`
	Run      string    `json:"run"`
	Source   buildInfo `json:"source"`
	Started  string    `json:"started"`
	Finished string    `json:"finished"`
}

type Machine struct {
	Desc   string `json:"desc,omitempty"`
	Host   string `json:"host"`
	CPU    string `json:"cpu"`
	NumCPU int    `json:"num_cpu"`
	Kernel string `json:"kernel"`
	Go     string `json:"go"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type Info struct {
	Engine      string   `json:"engine"`
	Module      string   `json:"module"`
	Version     string   `json:"version"`
	Sum         string   `json:"sum,omitempty"`
	Go          string   `json:"go"`
	Settings    []string `json:"settings"`
	Interrupt   bool     `json:"interrupt"`
	MemoryLimit bool     `json:"memory_limit"`
	Modules     bool     `json:"modules"`
	Shared      bool     `json:"shared_programs"`
	Notes       []string `json:"notes"`
}

type statsJ struct {
	N      int     `json:"n"`
	Median float64 `json:"median_ns"`
	Min    float64 `json:"min_ns"`
	Max    float64 `json:"max_ns"`
}

// series is one number per round, or the reason there is none.
type series struct {
	vals []float64
	why  string
}

func (s series) median() float64 {
	if len(s.vals) == 0 {
		return math.NaN()
	}
	v := append([]float64(nil), s.vals...)
	sort.Float64s(v)
	if len(v)%2 == 1 {
		return v[len(v)/2]
	}
	return (v[len(v)/2-1] + v[len(v)/2]) / 2
}

func (s series) lo() float64 { v := append([]float64(nil), s.vals...); sort.Float64s(v); return v[0] }
func (s series) hi() float64 {
	v := append([]float64(nil), s.vals...)
	sort.Float64s(v)
	return v[len(v)-1]
}

func crash(raw json.RawMessage) string {
	var c struct {
		Crash string `json:"crash"`
		Err   string `json:"err"`
		OK    *bool  `json:"ok"`
	}
	json.Unmarshal(raw, &c)
	if c.Crash != "" {
		return "crash: " + c.Crash
	}
	if c.OK != nil && !*c.OK {
		return c.Err
	}
	return ""
}

// collect reads one number per round with get.
func collect(raws []json.RawMessage, get func(json.RawMessage) (float64, string)) series {
	var s series
	for _, r := range raws {
		if why := crash(r); why != "" {
			s.why = why
			continue
		}
		v, why := get(r)
		if why != "" {
			s.why = why
			continue
		}
		s.vals = append(s.vals, v)
	}
	if len(s.vals) > 0 && len(s.vals) < len(raws) {
		s.why = fmt.Sprintf("%d of %d rounds failed: %s", len(raws)-len(s.vals), len(raws), s.why)
	} else if len(s.vals) > 0 {
		s.why = ""
	}
	return s
}

func statField(field string) func(json.RawMessage) (float64, string) {
	return func(r json.RawMessage) (float64, string) {
		var m map[string]json.RawMessage
		json.Unmarshal(r, &m)
		raw, ok := m[field]
		if !ok || string(raw) == "null" {
			return 0, "no " + field
		}
		var s statsJ
		json.Unmarshal(raw, &s)
		return s.Median, ""
	}
}

// numField reads a number field of a result.
func numField(field string) func(json.RawMessage) (float64, string) {
	return func(r json.RawMessage) (float64, string) {
		var m map[string]json.RawMessage
		json.Unmarshal(r, &m)
		raw, ok := m[field]
		if !ok {
			return 0, "no " + field
		}
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil || v == 0 {
			return 0, "no " + field
		}
		return v, ""
	}
}

// scaled multiplies every value of s by k.
func scaled(s series, k float64) series {
	for i := range s.vals {
		s.vals[i] *= k
	}
	return s
}

func ms(ns float64) string {
	switch {
	case ns >= 100e6:
		return fmt.Sprintf("%.0f", ns/1e6)
	case ns >= 10e6:
		return fmt.Sprintf("%.1f", ns/1e6)
	case ns >= 1e6:
		return fmt.Sprintf("%.2f", ns/1e6)
	default:
		return fmt.Sprintf("%.3f", ns/1e6)
	}
}

func cellMS(s series) string {
	if len(s.vals) == 0 {
		return "no¹"
	}
	if len(s.vals) == 1 {
		return ms(s.median())
	}
	return fmt.Sprintf("%s (%s–%s)", ms(s.median()), ms(s.lo()), ms(s.hi()))
}

func num(v float64) string {
	if v >= 100 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func cellNum(s series) string {
	if len(s.vals) == 0 {
		return "no¹"
	}
	if len(s.vals) == 1 {
		return num(s.median())
	}
	return fmt.Sprintf("%s (%s–%s)", num(s.median()), num(s.lo()), num(s.hi()))
}

func mib(b float64) string { return fmt.Sprintf("%.1f", b/(1<<20)) }

func (res *Results) engineNames() []string {
	var n []string
	for _, e := range res.Engines {
		n = append(n, e.Engine)
	}
	return n
}

type notes struct {
	list []string
}

func (n *notes) add(engine, what, why string) {
	if why == "" {
		return
	}
	if len(why) > 220 {
		why = why[:220] + "..."
	}
	n.list = append(n.list, fmt.Sprintf("- %s, %s: %s", engine, what, strings.ReplaceAll(why, "|", "\\|")))
}

func (n *notes) write(b *strings.Builder) {
	if len(n.list) == 0 {
		return
	}
	b.WriteString("\n¹ Did not run:\n\n")
	for _, l := range n.list {
		b.WriteString(l + "\n")
	}
}

func renderMarkdown(res *Results) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	p("# Pure Go JavaScript engines: run %s\n\n", res.Name)
	m := res.Machine
	p("- Started %s, finished %s\n", res.Started, res.Finished)
	p("- liustack/slowjs and this comparison at `%s` (%s)\n", res.Source.Describe, res.Source.Commit)
	if len(res.Drivers) > 0 {
		p("- Driver (`cmd/compare`) at %s, the engine binaries are from the commit above\n", strings.Join(res.Drivers, ", "))
	}
	for _, a := range res.Added {
		p("- %s added from run %s (%s to %s), binaries at `%s` (%s)\n", strings.Join(a.Engines, ", "), a.Run, a.Started, a.Finished, a.Source.Describe, a.Source.Commit)
	}
	p("- Machine: %s, %s, %d CPUs, %s", m.CPU, m.Kernel, m.NumCPU, m.GOOS+"/"+m.GOARCH)
	if m.Desc != "" {
		p(" (%s)", m.Desc)
	}
	p("\n- Go: %s, binaries built with `CGO_ENABLED=0 -pgo=off -trimpath`\n", m.Go)
	if res.MemCap != "" {
		p("- Memory cap per engine process: %s\n", res.MemCap)
	}
	if res.Timed != nil {
		pin := "not pinned"
		if res.CPU != "" {
			pin = "pinned to CPU " + res.CPU
		}
		lo, hi := -1, 0
		for _, byEngine := range res.Timed {
			for _, rounds := range byEngine {
				if lo < 0 || len(rounds) < lo {
					lo = len(rounds)
				}
				hi = max(hi, len(rounds))
			}
		}
		n := strconv.Itoa(hi)
		if lo != hi {
			n = fmt.Sprintf("%d to %d", lo, hi)
		}
		p("- Timed workloads: %s rounds, engines in a different order each round, GOMAXPROCS=%d, %s\n", n, res.GOMAXPROCS, pin)
	}
	if len(res.Disturbed) > 0 {
		var ks []string
		for k, v := range res.Disturbed {
			ks = append(ks, fmt.Sprintf("%s ×%d", k, v))
		}
		sort.Strings(ks)
		p("- Runs repeated because the sibling hyperthread was busy or the process waited for its CPU (a run still disturbed on its third try is kept): %s\n", strings.Join(ks, ", "))
	}
	var pins []string
	for k, v := range res.Pinned {
		pins = append(pins, k+" "+v)
	}
	sort.Strings(pins)
	p("- Pinned inputs: %s\n\n", strings.Join(pins, ", "))

	p("## Engines\n\n| Engine | Module | Version | How it is used |\n|---|---|---|---|\n")
	for _, e := range res.Engines {
		p("| %s | `%s` | `%s` | %s |\n", e.Engine, e.Module, e.Version, strings.Join(e.Notes, " "))
	}
	names := res.engineNames()

	if res.Timed != nil {
		renderSpeed(&b, res, names)
	}
	if res.Features != nil {
		renderFeatures(&b, res, names)
	}
	if res.Test262 != nil {
		renderTest262(&b, res)
	}
	return strings.TrimRight(b.String(), " \t\n") + "\n"
}

func renderSpeed(b *strings.Builder, res *Results, names []string) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	var n notes
	p("\n## Speed\n\nMedian of the per-round medians, with the range of the round medians in brackets. Times in milliseconds, lower is better. V8 suite scores: higher is better.\n\n")
	p("| Engine | React SSR render | V8 suite score | JSON.parse | JSON.stringify | String building | RegExp |\n|---|---|---|---|---|---|---|\n")
	for _, e := range names {
		react := collect(res.Timed["react"][e], statField("render"))
		n.add(e, "React SSR", react.why)
		v8 := collect(res.Timed["v8"][e], v8Score)
		n.add(e, "V8 suite", v8.why)
		row := []string{e, cellMS(react), cellNum(v8)}
		for _, w := range []string{"json-parse", "json-stringify", "string-concat", "regexp"} {
			s := collect(res.Timed["micro/"+w][e], statField("call"))
			n.add(e, w, s.why)
			row = append(row, cellMS(s))
		}
		p("| %s |\n", strings.Join(row, " | "))
	}
	n.write(b)

	p("\n### CPU time\n\nProcess CPU time (user and system, all threads) per operation, in milliseconds, median of the rounds. It counts work that wall time can hide when it runs on other threads, such as a concurrent garbage collector. When the run is pinned to one CPU, those threads run on that CPU too and so also add to wall time. V8 suite: CPU time of the whole suite over its wall time.\n\n")
	p("| Engine | React SSR render | JSON.parse | JSON.stringify | String building | RegExp | V8 suite CPU / wall |\n|---|---|---|---|---|---|---|\n")
	for _, e := range names {
		row := []string{e, cellMS(scaled(collect(res.Timed["react"][e], numField("cpu_ms_per_op")), 1e6))}
		for _, w := range []string{"json-parse", "json-stringify", "string-concat", "regexp"} {
			row = append(row, cellMS(scaled(collect(res.Timed["micro/"+w][e], numField("cpu_ms_per_op")), 1e6)))
		}
		ratio := collect(res.Timed["v8"][e], func(r json.RawMessage) (float64, string) {
			var v struct {
				WallS float64 `json:"wall_s"`
				CPUS  float64 `json:"cpu_s"`
			}
			json.Unmarshal(r, &v)
			if v.WallS == 0 || v.CPUS == 0 {
				return 0, "no cpu_s"
			}
			return v.CPUS / v.WallS, ""
		})
		cell := "n/a"
		if len(ratio.vals) > 0 {
			cell = fmt.Sprintf("%.2f", ratio.median())
		}
		p("| %s | %s |\n", strings.Join(row, " | "), cell)
	}

	if _, ok := res.Timed["v8"]; ok {
		p("\n### V8 benchmark suite, version 7\n\nScore per benchmark (higher is better), median of the rounds.\n\n")
		benches := []string{"Richards", "DeltaBlue", "Crypto", "RayTrace", "EarleyBoyer", "RegExp", "Splay", "NavierStokes"}
		p("| Engine | %s | Score |\n|---|%s---|\n", strings.Join(benches, " | "), strings.Repeat("---|", len(benches)))
		for _, e := range names {
			row := []string{e}
			for _, bn := range benches {
				s := collect(res.Timed["v8"][e], v8Bench(bn))
				if len(s.vals) == 0 {
					why := s.why
					if len(why) > 60 {
						why = why[:60] + "..."
					}
					row = append(row, "fails: "+strings.ReplaceAll(why, "|", "\\|"))
				} else {
					row = append(row, num(s.median()))
				}
			}
			row = append(row, cellNum(collect(res.Timed["v8"][e], v8Score)))
			p("| %s |\n", strings.Join(row, " | "))
		}
	}

	if _, ok := res.Timed["cold"]; ok {
		var n notes
		p("\n### Cold start and memory\n\nCold start: a new VM, then the React bundle (117 KB) loaded into it. \"From program\" runs a program compiled once beforehand, for engines whose compiled programs work on any VM. Memory: what the process grows by per VM, with 100 VMs alive, each with the bundle loaded and one page rendered. RSS counts memory outside the Go heap too (the QuickJS ports built by ccgo allocate there), and can be lower than the Go heap growth when the heap reuses pages it already had.\n\n")
		p("| Engine | New VM | New VM + load source | New VM + load program | First render | RSS per VM (MiB) | Go heap per VM (MiB) |\n|---|---|---|---|---|---|---|\n")
		for _, e := range names {
			raws := res.Timed["cold"][e]
			nv := collect(raws, statField("new_vm"))
			src := collect(raws, statField("new_vm_and_load_source"))
			n.add(e, "cold start", src.why)
			prog := collect(raws, statField("new_vm_and_load_program"))
			first := collect(raws, statField("first_render"))
			progCell := cellMS(prog)
			if len(prog.vals) == 0 {
				progCell = "n/a"
			}
			mem := res.Timed["mem"][e]
			rssS := collect(mem, func(r json.RawMessage) (float64, string) {
				var m struct {
					RSS float64 `json:"rss_per_vm_bytes"`
				}
				json.Unmarshal(r, &m)
				return m.RSS, ""
			})
			heapS := collect(mem, func(r json.RawMessage) (float64, string) {
				var m struct {
					H float64 `json:"go_heap_per_vm_bytes"`
				}
				json.Unmarshal(r, &m)
				return m.H, ""
			})
			n.add(e, "memory", rssS.why)
			rssCell, heapCell := "no¹", "no¹"
			if len(rssS.vals) > 0 {
				rssCell, heapCell = mib(rssS.median()), mib(heapS.median())
			}
			p("| %s | %s | %s | %s | %s | %s | %s |\n", e, cellMS(nv), cellMS(src), progCell, cellMS(first), rssCell, heapCell)
		}
		n.write(b)
	}

	if _, ok := res.Timed["upstream"]; ok {
		p("\n### QuickJS micro benchmarks (`compare/testdata`)\n\nTime per iteration against slowjs, geometric mean over the tests both engines run (below 1 is faster than slowjs).\n\n")
		p("| Engine | Tests run | Geomean time vs slowjs | Faster than slowjs on |\n|---|---|---|---|\n")
		base := upstreamMedians(res.Timed["upstream"][selfRow(res.Timed["upstream"])])
		for _, e := range names {
			mine := upstreamMedians(res.Timed["upstream"][e])
			logSum, k, faster := 0.0, 0, 0
			for t, v := range mine {
				if bv, ok := base[t]; ok && bv > 0 && v > 0 {
					logSum += math.Log(v / bv)
					k++
					if v < bv {
						faster++
					}
				}
			}
			g := "n/a"
			if k > 0 {
				g = fmt.Sprintf("%.2f", math.Exp(logSum/float64(k)))
			}
			p("| %s | %d | %s | %d of %d |\n", e, len(mine), g, faster, k)
		}
	}

	if res.Concurrent != nil {
		p("\n### Several VMs on several goroutines\n\nFour goroutines, one VM each, 20 renders each at the same time. Every page is checked.\n\n| Engine | Result |\n|---|---|\n")
		for _, e := range names {
			var c struct {
				OK     bool    `json:"ok"`
				Err    string  `json:"err"`
				Crash  string  `json:"crash"`
				WallMS float64 `json:"wall_ms"`
			}
			json.Unmarshal(res.Concurrent[e], &c)
			cell := "works"
			if !c.OK {
				cell = "fails: " + c.Err + c.Crash
				if len(cell) > 150 {
					cell = cell[:150] + "..."
				}
			}
			p("| %s | %s |\n", e, strings.ReplaceAll(cell, "|", "\\|"))
		}
	}
}

func v8Score(r json.RawMessage) (float64, string) {
	var v struct {
		Scores map[string]string `json:"scores"`
		Load   map[string]string `json:"load_errors"`
	}
	json.Unmarshal(r, &v)
	// The suite's own score is the geometric mean of the benchmarks that
	// ran; it is only valid when all eight ran.
	benches := []string{"Richards", "DeltaBlue", "Crypto", "RayTrace", "EarleyBoyer", "RegExp", "Splay", "NavierStokes"}
	logSum := 0.0
	for _, bn := range benches {
		f, err := strconv.ParseFloat(v.Scores[bn], 64)
		if err != nil || f <= 0 {
			why := v.Scores[bn]
			for k, e := range v.Load {
				why += " " + k + ": " + e
			}
			return 0, bn + " did not run: " + strings.TrimSpace(why)
		}
		logSum += math.Log(f)
	}
	return math.Exp(logSum / float64(len(benches))), ""
}

func v8Bench(name string) func(json.RawMessage) (float64, string) {
	return func(r json.RawMessage) (float64, string) {
		var v struct {
			Scores map[string]string `json:"scores"`
			Load   map[string]string `json:"load_errors"`
		}
		json.Unmarshal(r, &v)
		f, err := strconv.ParseFloat(v.Scores[name], 64)
		if err != nil {
			why := v.Scores[name]
			if why == "" {
				for k, e := range v.Load {
					why += k + ": " + e + " "
				}
			}
			return 0, strings.TrimSpace(why)
		}
		return f, ""
	}
}

// selfRow is the name of slowjs's row in a results file. Files made before
// 2026-10-24 name it liustack-quickjs, the module's name until v0.28.0.
func selfRow(byEngine map[string][]json.RawMessage) string {
	if _, ok := byEngine["slowjs"]; !ok {
		if _, old := byEngine["liustack-quickjs"]; old {
			return "liustack-quickjs"
		}
	}
	return "slowjs"
}

func upstreamMedians(raws []json.RawMessage) map[string]float64 {
	per := map[string][]float64{}
	for _, r := range raws {
		var u struct {
			Tests map[string]struct {
				Ns  float64 `json:"ns_per_n"`
				Err string  `json:"err"`
			} `json:"tests"`
		}
		if json.Unmarshal(r, &u) != nil {
			continue
		}
		for t, v := range u.Tests {
			if v.Err == "" && v.Ns > 0 {
				per[t] = append(per[t], v.Ns)
			}
		}
	}
	out := map[string]float64{}
	for t, v := range per {
		out[t] = series{vals: v}.median()
	}
	return out
}

func renderFeatures(b *strings.Builder, res *Results, names []string) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("\n## Language and library support\n\nyes: the probe gives the expected answer (for Intl, the same string as Node %s). no: it throws, gives another answer or crashes the process. n/a: the engine's API has no way to run it. Host APIs are not part of ECMAScript: yes means the global exists without the host adding it.\n\n", "24")
	p("| Probe | Edition | %s |\n|---|---|%s\n", strings.Join(names, " | "), strings.Repeat("---|", len(names)))
	var n notes
	totals := map[string][2]int{}
	for _, f := range res.Features {
		row := []string{"`" + f.ID + "`", f.Edition}
		for _, e := range names {
			var r struct {
				Status string `json:"status"`
				Err    string `json:"err"`
				Got    string `json:"got"`
				Crash  string `json:"crash"`
			}
			json.Unmarshal(f.Results[e], &r)
			cell := ""
			switch {
			case r.Crash != "":
				cell = "crash"
				n.add(e, f.ID, r.Crash)
			case r.Status == "pass" && f.ID == "recursion-depth":
				cell = r.Got
			case r.Status == "pass":
				cell = "yes"
			case r.Status == "n/a":
				cell = "n/a"
			default:
				cell = "no"
				why := r.Err
				if r.Got != "" {
					if why != "" {
						why = ", " + why
					}
					why = "got " + strconv.Quote(r.Got) + why
				}
				n.add(e, f.ID, why)
			}
			if f.Edition != "Host" {
				t := totals[e]
				t[1]++
				if r.Status == "pass" {
					t[0]++
				}
				totals[e] = t
			}
			row = append(row, cell)
		}
		p("| %s |\n", strings.Join(row, " | "))
	}
	row := []string{"**Total (without host APIs)**", ""}
	for _, e := range names {
		row = append(row, fmt.Sprintf("%d/%d", totals[e][0], totals[e][1]))
	}
	p("| %s |\n", strings.Join(row, " | "))
	if len(n.list) > 0 {
		b.WriteString("\nWhat went wrong:\n\n")
		for _, l := range n.list {
			b.WriteString(l + "\n")
		}
	}
}

func pct(a, t int) string {
	if t == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(a)/float64(t))
}

func renderTest262(b *strings.Builder, res *Results) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	t := res.Test262
	var ex []string
	for k, v := range t.Excluded {
		ex = append(ex, fmt.Sprintf("%d %s", v, k))
	}
	sort.Strings(ex)
	p("\n## test262\n\ntest262 at `%s`, directories %s: %d files. Left out for every engine: %s (module tests need a loader each engine wires differently, the others need host hooks such as `$262.createRealm` or `$262.detachArrayBuffer`). The rest run once sloppy and once strict unless their flags say otherwise: %d runs per engine. Each run gets a fresh VM and a 10 s limit, and each test262 process a memory cap. Out of memory: tests that went over the cap when run alone in a new process. Memory restarts: times a process went over the cap while the test it was on, run alone, did not; the memory came from earlier tests. \"ES2023 and older\" counts the runs in language and built-ins of tests whose feature tags are all from ES2023 or earlier (the list of newer tags is `t262.Newer`).\n\n",
		t.Commit, strings.Join(t.Dirs, ", "), t.Files, strings.Join(ex, ", "), t.Runs)
	p("| Engine | All | language | built-ins | intl402 | ES2023 and older | ES2023 language | ES2023 built-ins | Crashes | Timeouts | Out of memory | Memory restarts |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	var names []string
	for n := range t.Engines {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return t.Engines[names[i]].Pass > t.Engines[names[j]].Pass })
	for _, n := range names {
		e := t.Engines[n]
		var cnt = map[string][2]int{}
		for a, c := range e.Areas {
			top := strings.SplitN(a, "/", 2)[0]
			x := cnt[top]
			x[0] += c[0]
			x[1] += c[1]
			cnt[top] = x
		}
		es := e.ES2023
		p("| %s | %s (%d/%d) | %s | %s | %s | %s (%d/%d) | %s | %s | %d | %d | %d | %d |\n", n, pct(e.Pass, e.Runs), e.Pass, e.Runs,
			pct(cnt["language"][0], cnt["language"][1]), pct(cnt["built-ins"][0], cnt["built-ins"][1]), pct(cnt["intl402"][0], cnt["intl402"][1]),
			pct(es["all"][0]-es["intl402"][0], es["all"][1]-es["intl402"][1]), es["all"][0]-es["intl402"][0], es["all"][1]-es["intl402"][1],
			pct(es["language"][0], es["language"][1]), pct(es["built-ins"][0], es["built-ins"][1]), e.Crashes, e.Timeout, e.OOM, e.MemRestarts)
	}
	tags := []string{"class", "generators", "async-functions", "async-iteration", "Proxy", "Reflect", "BigInt", "regexp-named-groups", "regexp-lookbehind", "regexp-unicode-property-escapes", "regexp-v-flag", "optional-chaining", "coalesce-expression", "class-fields-private", "WeakRef", "Array.prototype.at", "Temporal", "iterator-helpers", "set-methods"}
	p("\n### test262 by feature tag\n\nPass rate of the runs whose `features` include the tag.\n\n| Tag | %s |\n|---|%s\n", strings.Join(names, " | "), strings.Repeat("---|", len(names)))
	for _, tag := range tags {
		row := []string{"`" + tag + "`"}
		for _, n := range names {
			c := t.Engines[n].Feature[tag]
			row = append(row, pct(c[0], c[1]))
		}
		p("| %s |\n", strings.Join(row, " | "))
	}
}
