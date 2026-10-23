// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command jsbench runs one workload on the engine linked into it (see
// package engine) and prints the result as JSON. The compare command builds
// one jsbench per engine and runs them in turn.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/liustack/slowjs/compare/engines/engine"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: jsbench info|feature|t262|react|v8|micro|upstream|cold|mem [flags]")
		os.Exit(2)
	}
	eng, err := engine.Get()
	if err != nil {
		fatal(err)
	}
	cmd, args := os.Args[1], os.Args[2:]
	// JSBENCH_CPUPROFILE=<file> writes a Go CPU profile of the whole command,
	// to see where an engine spends its time on a workload.
	if p := os.Getenv("JSBENCH_CPUPROFILE"); p != "" {
		f, err := os.Create(p)
		if err != nil {
			fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fatal(err)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	var out any
	switch cmd {
	case "info":
		out = info(eng)
	case "feature":
		id := fs.String("id", "", "feature id")
		file := fs.String("file", "features/features.json", "feature table")
		fs.Parse(args)
		out = runFeature(eng, *file, *id)
	case "t262":
		root := fs.String("root", "", "test262 checkout")
		list := fs.String("list", "", "file of tests to run, one per line: <mode> <path>")
		timeout := fs.Duration("timeout", 10*time.Second, "time limit per test")
		fs.Parse(args)
		runTest262(eng, *root, *list, *timeout)
		return
	case "react":
		bundle := fs.String("bundle", "react/react-ssr.js", "bundle")
		expected := fs.String("expected", "react/react-ssr.expected.html", "reference output")
		n := fs.Int("n", 200, "measured renders")
		warm := fs.Int("warm", 50, "renders before measuring")
		fs.Parse(args)
		out = runReact(eng, *bundle, *expected, *warm, *n)
	case "concurrent":
		bundle := fs.String("bundle", "react/react-ssr.js", "bundle")
		expected := fs.String("expected", "react/react-ssr.expected.html", "reference output")
		g := fs.Int("g", 4, "goroutines, one VM each")
		n := fs.Int("n", 20, "renders per goroutine")
		fs.Parse(args)
		out = runConcurrent(eng, *bundle, *expected, *g, *n)
	case "v8":
		dir := fs.String("dir", "../../internal/arewefastyet/v8-v7", "suite directory (the copy this repository already has)")
		only := fs.String("only", "", "run one benchmark: its file name without .js, such as richards")
		fs.Parse(args)
		out = runV8(eng, *dir, *only)
	case "micro":
		name := fs.String("name", "", "workload")
		n := fs.Int("n", 30, "measured calls")
		warm := fs.Int("warm", 5, "calls before measuring")
		fs.Parse(args)
		out = runMicro(eng, *name, *warm, *n)
	case "upstream":
		dir := fs.String("dir", "../testdata", "QuickJS micro benchmarks")
		budget := fs.Duration("budget", 200*time.Millisecond, "measured time per test")
		only := fs.String("only", "", "run the tests whose name matches this glob")
		fs.Parse(args)
		out = runUpstream(eng, *dir, *budget, *only)
	case "tifa-ssr":
		bundle := fs.String("bundle", "", "a tifa SSR bundle (.tifa/dist/ssr.js)")
		inputs := fs.String("inputs", "", "comma-separated files with the JSON of render inputs")
		outDir := fs.String("html", "", "write the first HTML of each page here")
		warm := fs.Int("warm", 50, "renders per page before measuring")
		n := fs.Int("n", 200, "measured renders per page")
		intl := fs.Bool("intl", true, "register the engine's own Intl where it is opt-in")
		prelude := fs.String("prelude", "", "comma-separated scripts to run before the bundle")
		probe := fs.String("probe", "", "an expression recorded after the first and the timed renders")
		tag := fs.String("tag", "", "added to the names of the HTML files")
		mem := fs.Int("mem", 0, "VMs kept alive to measure memory per VM (0: skip)")
		fs.Parse(args)
		o := tifaOpts{intl: *intl, probe: *probe, tag: *tag, mem: *mem}
		if *prelude != "" {
			o.preludes = strings.Split(*prelude, ",")
		}
		out = runTifaSSR(eng, *bundle, strings.Split(*inputs, ","), *outDir, *warm, *n, o)
	case "cold":
		bundle := fs.String("bundle", "react/react-ssr.js", "bundle")
		n := fs.Int("n", 30, "VMs per kind")
		fs.Parse(args)
		out = runCold(eng, *bundle, *n)
	case "mem":
		bundle := fs.String("bundle", "react/react-ssr.js", "bundle")
		k := fs.Int("k", 20, "VMs kept alive")
		fs.Parse(args)
		out = runMem(eng, *bundle, *k)
	default:
		fatal(fmt.Errorf("unknown command %q", cmd))
	}
	pprof.StopCPUProfile()
	emit(out)
}

func emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fatal(err)
	}
	os.Stdout.Write(append(b, '\n'))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "jsbench:", err)
	os.Exit(1)
}

type infoResult struct {
	Engine      string   `json:"engine"`
	Module      string   `json:"module"`
	Version     string   `json:"version"`
	Sum         string   `json:"sum,omitempty"`
	Go          string   `json:"go"`
	GOOS        string   `json:"goos"`
	GOARCH      string   `json:"goarch"`
	Settings    []string `json:"settings"`
	Interrupt   bool     `json:"interrupt"`
	MemoryLimit bool     `json:"memory_limit"`
	Modules     bool     `json:"modules"`
	Shared      bool     `json:"shared_programs"`
	Notes       []string `json:"notes"`
}

func info(e *engine.Engine) infoResult {
	r := infoResult{Engine: e.Name, Module: e.Module, Go: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Interrupt: e.Interrupt, MemoryLimit: e.MemoryLimit, Modules: e.Modules, Shared: e.SharedPrograms, Notes: e.Notes}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == e.Module {
				r.Version, r.Sum = d.Version, d.Sum
				if d.Replace != nil {
					r.Version = d.Version + " => " + d.Replace.Path
				}
			}
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "-tags", "-pgo", "CGO_ENABLED", "GOAMD64", "GOARM64", "-trimpath":
				r.Settings = append(r.Settings, s.Key+"="+s.Value)
			}
		}
	}
	return r
}

// stats summarizes durations in nanoseconds.
type stats struct {
	N      int     `json:"n"`
	Median float64 `json:"median_ns"`
	Min    float64 `json:"min_ns"`
	Max    float64 `json:"max_ns"`
	Mean   float64 `json:"mean_ns"`
}

func summarize(d []time.Duration) stats {
	if len(d) == 0 {
		return stats{}
	}
	s := make([]float64, len(d))
	sum := 0.0
	for i, v := range d {
		s[i] = float64(v)
		sum += s[i]
	}
	sort.Float64s(s)
	med := s[len(s)/2]
	if len(s)%2 == 0 {
		med = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return stats{N: len(s), Median: med, Min: s[0], Max: s[len(s)-1], Mean: sum / float64(len(s))}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if os.Getenv("JSBENCH_FULL_ERRORS") != "" {
		// For reports to engine authors: the whole message, with any stack.
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

// cpuTime is the CPU time this process has used, user and system, on all
// its threads.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		fatal(err)
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
