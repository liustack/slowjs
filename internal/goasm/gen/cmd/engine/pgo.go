// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	gen "github.com/liustack/quickjs/internal/goasm/gen"
)

// src is one C file of the engine and the group it belongs to.
type src struct{ path, group string }

// pgoBuild is a generation with a profile (-profile): a first compile without
// it writes the inlining down, the second compiles with it and replays that
// inlining (gen.InlineRemarks).
type pgoBuild struct {
	text, indexed, dir string
	names              map[string]bool
	mu                 sync.Mutex
	stale, fresh       []string
	misexpect          []string
}

func newPGO(text, dir string, n int) *pgoBuild {
	must(os.RemoveAll(dir))
	must(os.MkdirAll(dir, 0o755))
	p := &pgoBuild{text: text, dir: dir, indexed: filepath.Join(dir, "profile.profdata")}
	// The checked-in profile is text (the same bytes on every machine). clang
	// reads the indexed form, which llvm-profdata makes from it.
	must1(capture(filepath.Join(*llvm, "llvm-profdata"), "merge", "-o", p.indexed, text))
	p.names = gen.ProfileNames(readFile(text))
	if len(p.names) == 0 {
		die(fmt.Errorf("%s: no function in the profile", text))
	}
	return p
}

func (p *pgoBuild) clangRemarks(i int) string {
	return filepath.Join(p.dir, fmt.Sprintf("clang-%03d.txt", i))
}
func (p *pgoBuild) optRemarks() string { return filepath.Join(p.dir, "opt.txt") }

// firstPass compiles every file and the linked module as the build without a
// profile does, and keeps only the inlining it did.
func (p *pgoBuild) firstPass(srcs []src, srcArgs func(src) []string, llName func(int, src, string) string, postArgs []string, postOn bool) {
	clang := filepath.Join(*llvm, "clang")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var cerr error
	sem := make(chan bool, *jobs)
	lls := make([]string, len(srcs))
	for i, s := range srcs {
		lls[i] = llName(i, s, ".first")
		args := append(append(srcArgs(s), gen.RemarksFlags...), "-S", "-emit-llvm", "-o", lls[i], s.path)
		wg.Add(1)
		go func(i int, args []string) {
			defer wg.Done()
			sem <- true
			defer func() { <-sem }()
			stderr, err := capture(clang, args...)
			if err == nil {
				err = os.WriteFile(p.clangRemarks(i), []byte(gen.InlineRemarks(stderr)), 0o644)
			}
			if err != nil {
				mu.Lock()
				cerr = err
				mu.Unlock()
			}
		}(i, args)
	}
	wg.Wait()
	if cerr != nil {
		die(cerr)
	}
	linked := filepath.Join(p.dir, "first.ll")
	must1(capture(filepath.Join(*llvm, "llvm-link"), append([]string{"-S", "-o", linked}, lls...)...))
	remarks := ""
	if postOn {
		out := filepath.Join(p.dir, "first.opt.ll")
		stderr := must1(capture(filepath.Join(*llvm, "opt"), append(append(append([]string{}, postArgs...), "-pass-remarks=inline"), "-S", "-o", out, linked)...))
		remarks = gen.InlineRemarks(stderr)
	}
	writeFile(p.optRemarks(), remarks)
}

// secondPassArgs are the clang flags of file i with the profile.
func (p *pgoBuild) secondPassArgs(i int) []string {
	return append(gen.ProfileFlags(p.indexed), gen.ReplayClangArgs(p.clangRemarks(i))...)
}

// optArgs are the flags the post-link optimizer adds with the profile.
func (p *pgoBuild) optArgs() []string {
	return append(append([]string{}, gen.PGOOptFlags...), gen.ReplayOptArgs(p.optRemarks())...)
}

// readFrontIR notes the functions of one file that got no profile.
func (p *pgoBuild) readFrontIR(i int, path string) {
	stale, fresh := gen.UnprofiledFunctions(readFile(path), p.names)
	p.mu.Lock()
	p.stale = append(p.stale, stale...)
	p.fresh = append(p.fresh, fresh...)
	p.mu.Unlock()
}

// noteMisexpect keeps the -Wmisexpect warnings of one compile.
func (p *pgoBuild) noteMisexpect(stderr string) {
	m := gen.MisexpectLines(stderr)
	p.mu.Lock()
	p.misexpect = append(p.misexpect, m...)
	p.mu.Unlock()
}

// reportStale prints how well the profile fits the C, and the warnings of
// gen.StaleWarnings. A stale profile never stops make gen.
func (p *pgoBuild) reportStale(max int) {
	show := func(s []string) string {
		if len(s) > 20 {
			return strings.Join(s[:20], " ") + " ..."
		}
		return strings.Join(s, " ")
	}
	fmt.Fprintf(os.Stderr, "profile %s: %d functions, %d whose C changed since (no profile used: %s), %d new (%s)\n",
		p.text, len(p.names), len(p.stale), show(p.stale), len(p.fresh), show(p.fresh))
	for _, w := range gen.StaleWarnings(p.stale, p.fresh, gen.PGORequired, max) {
		fmt.Fprintln(os.Stderr, w)
	}
	sort.Strings(p.misexpect)
	list := filepath.Join(p.dir, "misexpect.txt")
	writeFile(list, strings.Join(p.misexpect, "\n")+"\n")
	fmt.Fprintf(os.Stderr, "profile: %d likely() or unlikely() of the C the training ran the other way (%s)\n", len(p.misexpect), list)
}

// stripCGProfile takes the "CG Profile" module flag out of an IR file
// (gen.StripCGProfile).
func stripCGProfile(path string) {
	s := readFile(path)
	if t := gen.StripCGProfile(s); t != s {
		writeFile(path, t)
	}
}
