// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command rvgen makes random programs (package rvgen) and runs them through
// rvdiff, the runner of the native test host that compares each run with its
// golden (engine/test/regvm/rvdiff.c, built by engine/test/regvm/build.sh):
//
//	rvgen gen [-seed S] [-n N] [-core] [-funcs F] OUTDIR
//		writes N programs, seeds S to S+N-1, as OUTDIR/p-SEED.js
//	rvgen check ... FILE, rvgen record ... FILE
//		the goldens of fixed seeds, recorded from the old loop while it
//		existed, and a run of the engine against them (see golden.go)
//	rvgen node -rvdiff BIN [-portable] ...
//		new programs, one run of the engine against Node 24 (see node.go)
//
// A program prints its log and a checksum. Seeds make every program again.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/liustack/quickjs/internal/rvgen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "gen":
		gen(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "usage: rvgen gen|check|record|node ... (see the comment of internal/rvgen/cmd/rvgen)")
	os.Exit(2)
}

func opts(fs *flag.FlagSet) *rvgen.Options {
	o := &rvgen.Options{}
	fs.BoolVar(&o.Core, "core", false, "only what the register pass took first")
	fs.BoolVar(&o.Wide, "wide", false, "also new, classes, ++ in expressions, unary operators, in, instanceof, element writes, delete, spread, regular expressions and getters that throw")
	fs.BoolVar(&o.Captures, "captures", false, "also closures kept and called when the run of their function is over, over variables written before and after the capture")
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

// limit is the time a run of a program may take, in seconds (rvdiff -t)
var limit = 30

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "rvgen:", err)
		os.Exit(2)
	}
}
