// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The flags of the program. Run it with -test.run '^$' -test.bench Pages -test.benchtime 1x: each
// page is one sub-benchmark that does its own renders, whatever b.N is. docs/testing.md has the
// commands of a count and of a timing.
var (
	flagMode    = flag.String("mode", string(Tifa), "how the VM is set up: tifa (as tifa does) or standin (as the program that measured v0.31.1 and before did)")
	flagBare    = flag.Bool("bare", false, "register no tifa hook, so the bundle runs React's javascript in their place")
	flagKind    = flag.String("kind", "", "the pages of the repository: "+strings.Join(Kinds(), ", "))
	flagPages   = flag.String("pages", "", "with -kind, only these pages, comma-separated (the names of their inputs without .json)")
	flagBundle  = flag.String("bundle", "", "a bundle of your own, in place of -kind")
	flagInputs  = flag.String("inputs", "", "with -bundle, the inputs of its pages, comma-separated")
	flagPrelude = flag.String("prelude", "", "scripts evaluated before the bundle, comma-separated (default: none in the tifa mode, the stand-ins in the stand-in mode)")
	flagReact   = flag.Bool("react", false, "with -bundle, the bundle defines render(), which returns the HTML")
	flagWarm    = flag.Int("warm", 8, "renders of each page before the timed ones (React's page: 20)")
	flagTimed   = flag.Int("timed", 20, "timed renders of each page, whose median is reported (React's page: 40)")
	flagLimit   = flag.Int64("limit", defaultMemory, "the memory limit of the VM in bytes, 0 for none (React's page in the stand-in mode: none)")
	flagNoPool  = flag.Bool("nopool", false, "no collection between renders, only those the engine starts itself")
	flagTimeout = flag.Duration("evaltimeout", defaultTimeout, "the eval timeout of the tifa mode, tifa's ssr.timeout. Any value above 0 does the same work: raise it under callgrind")
	flagHash    = flag.Bool("hash", false, "print the SHA-256 of the HTML of the first render of each page (html NAME HASH)")
	flagWindow  = flag.Bool("cg", false, "mark the window of callgrind (bench cg -dumps -small 0, amd64 only): render every page -warm times, zero the counts, render every page -timed times, then dump the counts")
	flagRepo    = flag.String("repo", "", "the liustack/slowjs checkout that holds the pages (default: the one the working directory is in)")
)

var errNothing = errors.New("nothing to render")

// run is a run of the program as its flags say.
type run struct {
	o           Options
	inputs      []string
	warm, timed int
	prefix      string
}

func set(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) { found = found || f.Name == name })
	return found
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// fromFlags turns the flags into a run.
func fromFlags() (run, error) {
	s := Setup{Mode: Mode(*flagMode), Bare: *flagBare}
	if s.Mode != Tifa && s.Mode != StandIn {
		return run{}, fmt.Errorf("-mode %q: tifa or standin", *flagMode)
	}
	repo, err := Repo(*flagRepo)
	if err != nil {
		return run{}, err
	}
	var k Kind
	switch {
	case *flagKind != "" && *flagBundle != "":
		return run{}, errors.New("give -kind or -bundle, not both")
	case *flagKind != "":
		if k, err = FindKind(repo, *flagKind, split(*flagPages)); err != nil {
			return run{}, err
		}
	case *flagBundle != "":
		k = Kind{Bundle: *flagBundle, Inputs: split(*flagInputs), React: *flagReact}
	default:
		return run{}, errNothing
	}
	if !k.React && len(k.Inputs) == 0 {
		return run{}, errors.New("no page to render: give -inputs")
	}
	r := run{o: Options{Setup: s, Limit: uintptr(*flagLimit), Timeout: *flagTimeout, Bundle: k.Bundle, React: k.React, NoPool: *flagNoPool, Log: os.Stderr},
		inputs: k.Inputs, warm: *flagWarm, timed: *flagTimed, prefix: s.Prefix()}
	if *flagLimit < 0 {
		return run{}, fmt.Errorf("-limit %d", *flagLimit)
	}
	if k.React {
		// as the program the stand-in mode reproduces timed React's page
		if !set("warm") {
			r.warm = 20
		}
		if !set("timed") {
			r.timed = 40
		}
		if s.Mode == StandIn && !set("limit") {
			r.o.Limit = 0
		}
	}
	switch {
	case set("prelude"):
		r.o.Preludes = split(*flagPrelude)
	case s.Mode == StandIn && !k.React:
		r.o.Preludes = []string{filepath.Join(repo, "internal", "measure", "pages", "hostjs.js"), filepath.Join(repo, "engine", "pgo", "pages", "fakeintl.js")}
	}
	if r.warm < 0 || r.timed < 1 {
		return run{}, fmt.Errorf("-warm %d -timed %d: at least one timed render", r.warm, r.timed)
	}
	if *flagWindow && !canMarkWindow {
		return run{}, errors.New("-cg: only amd64 has the client requests of valgrind")
	}
	return r, nil
}

// BenchmarkPages loads the bundle in one VM, then renders each page -warm times and -timed times
// more, and reports the median of the timed renders as p50-ms, on a line named
// Pages/MODE/PAGE (Pages/PAGE in the stand-in mode). The collections between renders are not
// timed.
func BenchmarkPages(b *testing.B) {
	r, err := fromFlags()
	if errors.Is(err, errNothing) {
		b.Skip("give -kind, or -bundle with -inputs or -react")
	}
	if err != nil {
		b.Fatal(err)
	}
	runner, err := Open(r.o)
	if err != nil {
		b.Fatal(err)
	}
	defer runner.Close()
	if spinShare > 0 {
		fmt.Fprintf(os.Stderr, "pages: each timed render also waits %g of the median warm render of its page (spin.go)\n", float64(spinShare))
	}
	pages := r.inputs
	if r.o.React {
		pages = []string{""}
	}
	page := func(b *testing.B, in string) (input, name string) {
		if in == "" {
			return "", "react-ssr"
		}
		js, err := os.ReadFile(in)
		if err != nil {
			b.Fatal(err)
		}
		return string(js), PageName(in)
	}
	if *flagWindow {
		b.Run(r.prefix+"window", func(b *testing.B) { window(b, runner, r, pages, page) })
		return
	}
	render := func(b *testing.B, in string) {
		input, name := page(b, in)
		b.Run(name, func(b *testing.B) {
			ds := make([]time.Duration, 0, r.timed)
			var warm []time.Duration // for spinShare (spin.go)
			for i := 0; i < r.warm+r.timed; i++ {
				html, d, err := runner.Render(input)
				if err != nil {
					b.Fatalf("%s, render %d: %v", name, i, err)
				}
				if spinShare > 0 {
					if i < r.warm {
						warm = append(warm, d)
					} else {
						d += spinFor(spinTarget(warm, spinShare))
					}
				}
				if i == 0 && *flagHash {
					fmt.Fprintf(os.Stderr, "html %s %x\n", name, sha256.Sum256([]byte(html)))
				}
				if err := runner.After(); err != nil {
					b.Fatal(err)
				}
				if i >= r.warm {
					ds = append(ds, d)
				}
			}
			slices.Sort(ds)
			b.ReportMetric(float64(ds[len(ds)/2])/1e6, "p50-ms")
		})
	}
	if r.prefix == "" {
		for _, in := range pages {
			render(b, in)
		}
		return
	}
	b.Run(strings.TrimSuffix(r.prefix, "/"), func(b *testing.B) {
		for _, in := range pages {
			render(b, in)
		}
	})
}

// The requests of callgrind, VG_USERREQ_TOOL_BASE('C','T') and the next ones, in valgrind's
// callgrind.h.
const (
	cgDumpStats = 0x43540000
	cgZeroStats = 0x43540001
)

// window renders every page r.warm times, zeroes the counts of callgrind, renders every page
// r.timed times, each followed by what follows a render in tifa (After), and dumps the counts:
// bench cg -dumps -small 0 reads that one dump and divides it by r.timed, the counts of the pages
// rendered once each.
func window(b *testing.B, runner *Runner, r run, pages []string, page func(*testing.B, string) (string, string)) {
	// read before the window, which holds the renders only
	inputs, names := make([]string, len(pages)), make([]string, len(pages))
	for i, in := range pages {
		inputs[i], names[i] = page(b, in)
	}
	do := func(n int) {
		for p, input := range inputs {
			for i := 0; i < n; i++ {
				if _, _, err := runner.Render(input); err != nil {
					b.Fatalf("%s, render %d: %v", names[p], i, err)
				}
				if err := runner.After(); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	do(r.warm)
	clientRequest(&[6]uint64{cgZeroStats})
	do(r.timed)
	clientRequest(&[6]uint64{cgDumpStats})
	b.ReportMetric(float64(r.timed*len(pages)), "renders")
}
