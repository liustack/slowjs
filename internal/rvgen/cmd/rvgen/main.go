// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command rvgen makes random programs (package rvgen) and runs them with the
// two interpreter loops through rvdiff, the differential runner of the native
// test host (engine/test/regvm/rvdiff.c, built by engine/test/regvm/build.sh):
//
//	rvgen gen [-seed S] [-n N] [-core] [-funcs F] OUTDIR
//		writes N programs, seeds S to S+N-1, as OUTDIR/p-SEED.js
//	rvgen fuzz -rvdiff BIN [-seed S] [-n N] [-batch B] [-j J] [-core] [-for DURATION] OUTDIR
//		makes programs in batches, runs rvdiff over each batch, and for
//		every program that differs writes the program and a shrunk copy
//		(OUTDIR/found/p-SEED.js and p-SEED.min.js) and a line to
//		OUTDIR/found.txt. It exits with 1 when any differs.
//	rvgen shrink -rvdiff BIN [-o OUT] FILE
//		takes lines out of FILE while rvdiff still finds the same kinds of
//		difference, and writes the result to OUT (default FILE.min.js)
//	rvgen referee -rvdiff BIN [-node NODE] [-t SECONDS] [-strict] FILE...
//		runs each FILE under Node too and says which loop Node agrees
//		with where the two differ (see referee.go). fuzz -node NODE does
//		it for every program that differs.
//	rvgen record ... FILE, rvgen check ... FILE
//		the goldens of fixed seeds, recorded from the old loop, and a run
//		of one engine against them (see golden.go)
//	rvgen node -rvdiff BIN [-portable] ...
//		new programs, one run of the engine against Node 24 (see referee.go)
//
// A program prints its log and a checksum. Seeds make every program again.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/liustack/quickjs/internal/rvgen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "gen":
		gen(os.Args[2:])
	case "fuzz":
		fuzz(os.Args[2:])
	case "shrink":
		shrinkCmd(os.Args[2:])
	case "referee":
		refereeCmd(os.Args[2:])
	case "record":
		recordCmd(os.Args[2:])
	case "check":
		checkCmd(os.Args[2:])
	case "node":
		nodeCmd(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rvgen gen|fuzz|shrink|referee|record|check|node ... (see the comment of internal/rvgen/cmd/rvgen)")
	os.Exit(2)
}

func opts(fs *flag.FlagSet) *rvgen.Options {
	o := &rvgen.Options{}
	fs.BoolVar(&o.Core, "core", false, "only what the register pass took first")
	fs.BoolVar(&o.Wide, "wide", false, "also new, classes, ++ in expressions, unary operators, in, instanceof, element writes, delete, spread, regular expressions and getters that throw")
	fs.BoolVar(&o.Portable, "portable", false, "write the name of a caught error, not its message, so that the output is the same under Node")
	fs.IntVar(&o.Funcs, "funcs", 8, "functions in a program")
	fs.IntVar(&o.Stmts, "stmts", 6, "statements in a function, at most")
	fs.IntVar(&o.Depth, "depth", 3, "nesting, at most")
	return o
}

func gen(args []string) {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	seed := fs.Uint64("seed", 1, "first seed")
	n := fs.Int("n", 10, "programs")
	o := opts(fs)
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	dir := fs.Arg(0)
	must(os.MkdirAll(dir, 0o777))
	for i := range *n {
		s := *seed + uint64(i)
		must(os.WriteFile(filepath.Join(dir, fmt.Sprintf("p-%d.js", s)), []byte(rvgen.Generate(s, *o)), 0o666))
	}
}

var diffLine = regexp.MustCompile(`^DIFF (\S+)(?: [^:]*)?: (.*)$`)

// a program the time limit ended, which rvdiff does not compare
var slowLine = regexp.MustCompile(`^SLOW (\S+?):? `)

// limit is the time a run of a program may take, in seconds (rvdiff -t)
var limit = 30

// rvdiff runs bin -q over the paths and returns, for each program that
// differs, the kinds that differ.
func rvdiff(bin string, jobs int, paths ...string) (map[string]string, string, error) {
	args := append([]string{"-q", "-t", fmt.Sprint(limit), "-j", fmt.Sprint(jobs)}, paths...)
	cmd := exec.Command(bin, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		err = nil
	}
	found := map[string]string{}
	for _, l := range strings.Split(out.String(), "\n") {
		if m := diffLine.FindStringSubmatch(l); m != nil {
			found[m[1]] = m[2]
		}
	}
	return found, out.String(), err
}

func fuzz(args []string) {
	fs := flag.NewFlagSet("fuzz", flag.ExitOnError)
	bin := fs.String("rvdiff", "", "the rvdiff program")
	seed := fs.Uint64("seed", 1, "first seed")
	n := fs.Int("n", 1000, "programs (0: until -for runs out)")
	batch := fs.Int("batch", 100, "programs a run of rvdiff")
	jobs := fs.Int("j", 4, "programs at a time")
	fs.IntVar(&limit, "t", 30, "seconds a run may take (rvdiff -t): more under the sanitizers")
	dur := fs.Duration("for", 0, "stop after this long")
	node := fs.String("node", "", "also run each program that differs under this node, to say which loop is right")
	o := opts(fs)
	fs.Parse(args)
	if fs.NArg() != 1 || *bin == "" {
		usage()
	}
	dir := fs.Arg(0)
	must(os.MkdirAll(filepath.Join(dir, "found"), 0o777))
	start := time.Now()
	var total, differ, slow int
	report, err := os.OpenFile(filepath.Join(dir, "found.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
	must(err)
	defer report.Close()
	for s := *seed; ; s += uint64(*batch) {
		if *n > 0 && total >= *n {
			break
		}
		if *dur > 0 && time.Since(start) > *dur {
			break
		}
		bd := filepath.Join(dir, "batch")
		os.RemoveAll(bd)
		must(os.MkdirAll(bd, 0o777))
		k := *batch
		if *n > 0 && total+k > *n {
			k = *n - total
		}
		for i := range k {
			ps := s + uint64(i)
			must(os.WriteFile(filepath.Join(bd, fmt.Sprintf("p-%d.js", ps)), []byte(rvgen.Generate(ps, *o)), 0o666))
		}
		found, out, err := rvdiff(*bin, *jobs, bd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rvgen: rvdiff: %v\n%s", err, out)
			os.Exit(2)
		}
		total += k
		for _, l := range strings.Split(out, "\n") {
			if m := slowLine.FindStringSubmatch(l); m != nil {
				slow++
				fmt.Fprintf(report, "%s: too slow to compare\n", filepath.Base(m[1]))
			}
		}
		names := make([]string, 0, len(found))
		for p := range found {
			names = append(names, p)
		}
		sort.Strings(names)
		for _, p := range names {
			differ++
			base := filepath.Base(p)
			src, err := os.ReadFile(p)
			must(err)
			keep := filepath.Join(dir, "found", base)
			must(os.WriteFile(keep, src, 0o666))
			min := strings.TrimSuffix(keep, ".js") + ".min.js"
			lines := shrink(*bin, keep, found[p], min)
			fmt.Fprintf(report, "%s: %s (shrunk to %d lines: %s)\n", base, found[p], lines, min)
			fmt.Printf("DIFF %s: %s, shrunk to %d lines\n", base, found[p], lines)
			if *node != "" {
				for _, f := range []string{keep, min} {
					v := referee(*bin, *node, f, false)
					fmt.Fprintf(report, "  referee %s: %s\n", filepath.Base(f), v)
					fmt.Printf("  referee %s: %s\n", filepath.Base(f), v)
				}
			}
		}
		fmt.Printf("rvgen: %d programs, %d differ, %d too slow (%s)\n", total, differ, slow, time.Since(start).Round(time.Second))
	}
	if differ > 0 {
		os.Exit(1)
	}
}

// shrink takes lines out of file while rvdiff finds the kinds of
// difference want, and writes the smallest program to out. It returns its
// lines.
func shrink(bin, file, want, out string) int {
	src, err := os.ReadFile(file)
	must(err)
	lines := rvgen.Lines(string(src))
	tmp := out + ".try.js"
	defer os.Remove(tmp)
	still := func(ls []string) bool {
		must(os.WriteFile(tmp, []byte(strings.Join(ls, "\n")+"\n"), 0o666))
		found, _, err := rvdiff(bin, 1, tmp)
		return err == nil && found[tmp] == want
	}
	for changed := true; changed; {
		changed = false
		// whole blocks first, then single lines, from the end
		blocks := rvgen.Blocks(lines)
		starts := make([]int, 0, len(blocks))
		for s := range blocks {
			starts = append(starts, s)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(starts)))
		for _, s := range starts {
			e := blocks[s]
			if s == 1 || e >= len(lines) {
				continue // the function around the program
			}
			try := append(append([]string{}, lines[:s]...), lines[e+1:]...)
			if still(try) {
				lines = try
				changed = true
				break
			}
		}
		if changed {
			continue
		}
		for i := len(lines) - 1; i >= 0; i-- {
			try := append(append([]string{}, lines[:i]...), lines[i+1:]...)
			if still(try) {
				lines = try
				changed = true
			}
		}
	}
	must(os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o666))
	return len(lines)
}

func shrinkCmd(args []string) {
	fs := flag.NewFlagSet("shrink", flag.ExitOnError)
	bin := fs.String("rvdiff", "", "the rvdiff program")
	outp := fs.String("o", "", "where to write the shrunk program")
	fs.Parse(args)
	if fs.NArg() != 1 || *bin == "" {
		usage()
	}
	file := fs.Arg(0)
	found, out, err := rvdiff(*bin, 1, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvgen: rvdiff: %v\n%s", err, out)
		os.Exit(2)
	}
	want, ok := found[file]
	if !ok {
		fmt.Fprintf(os.Stderr, "rvgen: %s does not differ\n", file)
		os.Exit(1)
	}
	o := *outp
	if o == "" {
		o = strings.TrimSuffix(file, ".js") + ".min.js"
	}
	n := shrink(*bin, file, want, o)
	fmt.Printf("%s: %s, %d lines in %s\n", file, want, n, o)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "rvgen:", err)
		os.Exit(2)
	}
}
