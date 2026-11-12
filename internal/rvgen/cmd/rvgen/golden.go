// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// The goldens of the fuzz seeds, recorded from the old loop at the last
// commit that had it (engine/test/regvm/golden/seeds, see golden.sh):
//
//	rvgen check -rvdiff BIN [-j J] [-t T] FILE
//		makes the programs of the seeds of FILE with the frozen generator
//		(internal/rvgen/v1), checks their hash, runs each once (rvdiff
//		-golden into an empty folder, which writes the run) and compares
//		each kind with FILE. It exits with 1 when any differs
//	rvgen record -rvdiff BIN [-core|-wide] [-seed S] [-n N] [-j J] [-t T] FILE
//		writes FILE from a run of each of N seeds: a line per seed with
//		the hash of its program, the status and the hash and line count
//		of each kind of its transcript. The run takes the place of the old
//		loop as the witness, so it is only for a change of output that is
//		meant: read the diff of FILE before it goes into a commit
//
// Only seeds and hashes are kept: a difference names its kinds and shows
// the lines of the run, not the lines of the golden.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	v1 "github.com/liustack/quickjs/internal/rvgen/v1"
)

// the kinds of a transcript (rvdiff.c), in order
var goldenKinds = []string{"out", "result", "exception", "stack", "job", "job-stack", "mem-end", "mem-gc", "expect"}

func hash16(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// summary is the golden line of a transcript file written by rvdiff
// (-golden -update): the status, then KIND=HASH/LINES for each
// kind it has
func summary(file string) (string, map[string][]string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", nil, err
	}
	lines := map[string][]string{}
	// a kind rvdiff keeps as "hash KIND COUNT FNV" (see rvdiff.c)
	hashed := map[string]string{}
	status := "?"
	for _, l := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(l, "--- status "); ok {
			status = rest
			break
		}
		if l == "" {
			continue
		}
		k, rest, _ := strings.Cut(l, " ")
		if k == "hash" {
			if f := strings.Fields(rest); len(f) == 3 {
				hashed[f[0]] = "fnv" + f[2] + "/" + f[1]
				continue
			}
		}
		lines[k] = append(lines[k], l)
	}
	parts := []string{"status=" + strings.ReplaceAll(status, " ", "_")}
	for _, k := range goldenKinds {
		if h, ok := hashed[k]; ok {
			parts = append(parts, k+"="+h)
		} else if ls := lines[k]; len(ls) > 0 {
			parts = append(parts, fmt.Sprintf("%s=%s/%d", k, hash16(strings.Join(ls, "\n")), len(ls)))
		}
	}
	return strings.Join(parts, " "), lines, nil
}

type goldenHead struct {
	o    v1.Options
	mode string
}

func (h goldenHead) String() string {
	return fmt.Sprintf("# rvgen v1 goldens: mode=%s funcs=%d stmts=%d depth=%d", h.mode, h.o.Funcs, h.o.Stmts, h.o.Depth)
}

var headLine = regexp.MustCompile(`^# rvgen v1 goldens: mode=(\w+) funcs=(\d+) stmts=(\d+) depth=(\d+)`)

func parseHead(l string) (goldenHead, error) {
	m := headLine.FindStringSubmatch(l)
	if m == nil {
		return goldenHead{}, fmt.Errorf("not a golden file of rvgen v1: %q", l)
	}
	var h goldenHead
	h.mode = m[1]
	h.o.Funcs, _ = strconv.Atoi(m[2])
	h.o.Stmts, _ = strconv.Atoi(m[3])
	h.o.Depth, _ = strconv.Atoi(m[4])
	h.o.Core = h.mode == "core"
	h.o.Wide = h.mode == "wide"
	return h, nil
}

var (
	updLine    = regexp.MustCompile(`^UPDATE (\S+)$`)
	noupdLine  = regexp.MustCompile(`^NOUPDATE (\S+): (.*)$`)
	seedOfFile = regexp.MustCompile(`p-(\d+)\.js`)
)

// runBatch writes the programs of seeds to a new folder and runs rvdiff on
// it with args (and -root, the folder); it returns the folder of the
// transcripts and rvdiff's output
func runBatch(bin string, jobs int, seeds []uint64, gen func(uint64) string, args ...string) (string, string, string, error) {
	bd, err := os.MkdirTemp("", "rvgold")
	if err != nil {
		return "", "", "", err
	}
	td := filepath.Join(bd, "t")
	pd := filepath.Join(bd, "p")
	must(os.MkdirAll(pd, 0o777))
	for _, s := range seeds {
		must(os.WriteFile(filepath.Join(pd, fmt.Sprintf("p-%d.js", s)), []byte(gen(s)), 0o666))
	}
	a := append([]string{"-q", "-t", fmt.Sprint(limit), "-j", fmt.Sprint(jobs), "-root", pd}, args...)
	for i, x := range a {
		if x == "DIR" {
			a[i] = td
		}
	}
	a = append(a, pd)
	cmd := exec.Command(bin, a...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		err = nil
	}
	return bd, td, out.String(), err
}

func goldenOpts(fs *flag.FlagSet) (*v1.Options, *bool, *bool) {
	o := &v1.Options{}
	core := fs.Bool("core", false, "the core mode")
	wide := fs.Bool("wide", false, "the wide mode")
	fs.IntVar(&o.Funcs, "funcs", 8, "functions in a program")
	fs.IntVar(&o.Stmts, "stmts", 6, "statements in a function, at most")
	fs.IntVar(&o.Depth, "depth", 3, "nesting, at most")
	return o, core, wide
}

func recordCmd(args []string) {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	bin := fs.String("rvdiff", "", "the rvdiff program")
	seed := fs.Uint64("seed", 1, "first seed")
	n := fs.Int("n", 1000, "programs")
	batch := fs.Int("batch", 100, "programs a run of rvdiff")
	jobs := fs.Int("j", 4, "programs at a time")
	fs.IntVar(&limit, "t", 60, "seconds a run may take")
	o, core, wide := goldenOpts(fs)
	fs.Parse(args)
	if fs.NArg() != 1 || *bin == "" {
		usage()
	}
	o.Core, o.Wide = *core, *wide
	h := goldenHead{o: *o, mode: map[bool]string{true: "core", false: map[bool]string{true: "wide", false: "full"}[*wide]}[*core]}
	gen := func(s uint64) string { return v1.Generate(s, h.o) }
	f, err := os.Create(fs.Arg(0))
	must(err)
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, h)
	var recorded, not int
	for start := 0; start < *n; start += *batch {
		var seeds []uint64
		for i := start; i < *n && i < start+*batch; i++ {
			seeds = append(seeds, *seed+uint64(i))
		}
		bd, td, out, err := runBatch(*bin, *jobs, seeds, gen, "-golden", "DIR", "-update")
		if err != nil {
			fmt.Fprintf(os.Stderr, "rvgen: rvdiff: %v\n%s", err, out)
			os.Exit(2)
		}
		why := map[uint64]string{}
		for _, l := range strings.Split(out, "\n") {
			if m := noupdLine.FindStringSubmatch(l); m != nil {
				if sm := seedOfFile.FindStringSubmatch(m[1]); sm != nil {
					s, _ := strconv.ParseUint(sm[1], 10, 64)
					why[s] = m[2]
				}
			}
		}
		for _, s := range seeds {
			ph := hash16(gen(s))
			if r, ok := why[s]; ok {
				not++
				fmt.Fprintf(w, "%d %s norecord %s\n", s, ph, strings.ReplaceAll(r, " ", "_"))
				continue
			}
			sum, _, err := summary(filepath.Join(td, fmt.Sprintf("p-%d.js.txt", s)))
			if err != nil {
				not++
				fmt.Fprintf(w, "%d %s norecord no_transcript\n", s, ph)
				continue
			}
			recorded++
			fmt.Fprintf(w, "%d %s %s\n", s, ph, sum)
		}
		os.RemoveAll(bd)
		fmt.Printf("rvgen record: %d of %d seeds recorded, %d not\n", recorded, start+len(seeds), not)
	}
	must(w.Flush())
	must(f.Close())
}

func checkCmd(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	bin := fs.String("rvdiff", "", "the rvdiff program")
	batch := fs.Int("batch", 100, "programs a run of rvdiff")
	jobs := fs.Int("j", 4, "programs at a time")
	fs.IntVar(&limit, "t", 60, "seconds a run may take")
	fs.Parse(args)
	if fs.NArg() != 1 || *bin == "" {
		usage()
	}
	b, err := os.ReadFile(fs.Arg(0))
	must(err)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	h, err := parseHead(lines[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "rvgen:", err)
		os.Exit(2)
	}
	gen := func(s uint64) string { return v1.Generate(s, h.o) }
	type want struct{ prog, sum string }
	wants := map[uint64]want{}
	var seeds []uint64
	skipped := 0
	for _, l := range lines[1:] {
		f := strings.SplitN(l, " ", 3)
		if len(f) < 3 {
			continue
		}
		s, _ := strconv.ParseUint(f[0], 10, 64)
		if strings.HasPrefix(f[2], "norecord ") {
			skipped++
			continue
		}
		wants[s] = want{f[1], f[2]}
		seeds = append(seeds, s)
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i] < seeds[j] })
	var checked, differ, changed int
	for start := 0; start < len(seeds); start += *batch {
		end := min(start+*batch, len(seeds))
		var run []uint64
		for _, s := range seeds[start:end] {
			if hash16(gen(s)) != wants[s].prog {
				changed++
				fmt.Printf("CHANGED p-%d.js: the generator makes another program for this seed (internal/rvgen/v1 must not change)\n", s)
				continue
			}
			run = append(run, s)
		}
		bd, td, out, err := runBatch(*bin, *jobs, run, gen, "-golden", "DIR", "-update")
		if err != nil {
			fmt.Fprintf(os.Stderr, "rvgen: rvdiff: %v\n%s", err, out)
			os.Exit(2)
		}
		noupd := map[uint64]string{}
		for _, l := range strings.Split(out, "\n") {
			if m := noupdLine.FindStringSubmatch(l); m != nil {
				if sm := seedOfFile.FindStringSubmatch(m[1]); sm != nil {
					s, _ := strconv.ParseUint(sm[1], 10, 64)
					noupd[s] = m[2]
				}
			}
		}
		for _, s := range run {
			checked++
			if r, ok := noupd[s]; ok {
				differ++
				fmt.Printf("DIFF p-%d.js: status %s (golden %s)\n", s, r, strings.Fields(wants[s].sum)[0])
				continue
			}
			sum, got, err := summary(filepath.Join(td, fmt.Sprintf("p-%d.js.txt", s)))
			if err != nil {
				differ++
				fmt.Printf("DIFF p-%d.js: no transcript of the run\n", s)
				continue
			}
			if sum == wants[s].sum {
				continue
			}
			differ++
			fmt.Printf("DIFF p-%d.js: %s\n", s, kindsDiffer(wants[s].sum, sum))
			for _, k := range goldenKinds {
				if partOf(wants[s].sum, k) != partOf(sum, k) && len(got[k]) > 0 {
					fmt.Printf("    run | %.300s\n", got[k][0])
				}
			}
		}
		os.RemoveAll(bd)
		fmt.Printf("rvgen check: %d seeds run, %d differ, %d programs changed, %d not recorded\n", checked, differ, changed, skipped)
	}
	if differ > 0 || changed > 0 {
		os.Exit(1)
	}
}

// partOf is the KIND=... part of a golden line, or ""
func partOf(sum, kind string) string {
	for _, p := range strings.Fields(sum) {
		if strings.HasPrefix(p, kind+"=") {
			return p
		}
	}
	return ""
}

func kindsDiffer(want, got string) string {
	var ks []string
	for _, k := range append([]string{"status"}, goldenKinds...) {
		if partOf(want, k) != partOf(got, k) {
			ks = append(ks, k)
		}
	}
	return strings.Join(ks, ",")
}
