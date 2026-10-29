// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command sjdev times builds of the C engine against each other. It makes the
// code-position variants of a program, runs them in turn, reads the result by
// the rule of "Code positions" in docs/performance.md and counts instructions
// with callgrind. It uses only the standard library and is registered as a
// tool, so `go tool sjdev bench ...` runs it. Generating the engine, the checks
// and running on another machine are make targets (see docs/testing.md).
//
//	sjdev bench build   a binary per code position
//	sjdev bench run     time the binaries in turn
//	sjdev bench report  the result as a Markdown table
//	sjdev bench cg      instructions per render under callgrind
//	sjdev bench cgdiff  the same from callgrind_annotate files
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/liustack/slowjs/internal/sjdev"
	"github.com/liustack/slowjs/internal/sjdev/bench"
)

const usage = `usage: sjdev bench build|run|report|cg|cgdiff [flags]

  build   build a binary per code position of the C engine
  run     time the binaries in turn, round after round
  report  read the records of a run: a table with the verdict of each metric
  cg      count instructions per render with callgrind
  cgdiff  the same from callgrind_annotate files

sjdev bench COMMAND -h lists the flags of a command.`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if len(os.Args) < 2 || os.Args[1] != "bench" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := cmdBench(ctx, os.Args[2:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintln(os.Stderr, "sjdev:", err)
		os.Exit(1)
	}
}

// list is a flag that can be given more than once.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(s string) error { *l = append(*l, s); return nil }

func csv(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func cmdBench(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sjdev bench build|run|report|cg|cgdiff [flags]")
	}
	switch args[0] {
	case "build":
		return benchBuild(ctx, args[1:])
	case "run":
		return benchRun(ctx, args[1:])
	case "report":
		return benchReport(args[1:])
	case "cg":
		return benchCG(ctx, args[1:])
	case "cgdiff":
		return benchCGDiff(args[1:])
	}
	return fmt.Errorf("unknown bench command %q", args[0])
}

func benchBuild(ctx context.Context, args []string) error {
	fs := newFlags("bench build")
	var o bench.BuildOptions
	pos := fs.String("positions", "quick", "quick (4), all (8) or a list such as 0.0,32.16 (shift.front)")
	fs.StringVar(&o.Side, "side", "", "label of this side, such as base or new")
	fs.StringVar(&o.Tree, "tree", ".", "the slowjs checkout, with the engine generated")
	fs.StringVar(&o.App, "app", "", "build in this module (tifa) against the tree through a go.work")
	fs.StringVar(&o.Pkg, "pkg", "", "package to build, relative to -app (or -tree)")
	fs.BoolVar(&o.Test, "test", false, "build the test binary of the package (go test -c)")
	fs.StringVar(&o.Tags, "tags", "", "build tags, if any")
	fs.StringVar(&o.PGO, "pgo", "off", "the -pgo flag")
	fs.StringVar(&o.Front, "front", "JS_CallInternal", "the function whose body moves")
	fs.StringVar(&o.Out, "out", "", "folder of the binaries (side@shift.front)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	if o.Positions, err = bench.ParsePositions(*pos); err != nil {
		return err
	}
	if err := o.Check(); err != nil {
		return err
	}
	xf, err := bench.BuildXform(ctx, o.Tree, o.Out, sjdev.Exec)
	if err != nil {
		return err
	}
	return bench.Build(ctx, o, xf, sjdev.Exec, os.Stdout)
}

func benchRun(ctx context.Context, args []string) error {
	fs := newFlags("bench run")
	var o bench.RunOptions
	var envs list
	sides, pos := fs.String("sides", "", "comma list: only these sides"), fs.String("positions", "", "comma list: only these positions")
	fs.StringVar(&o.Bin, "bin", "", "folder of the binaries (side@pos)")
	fs.StringVar(&o.Out, "out", "", "file of records (appended to, so a stopped run goes on)")
	fs.StringVar(&o.Kind, "kind", "run", "label of this measurement in the file")
	fs.IntVar(&o.Rounds, "rounds", 5, "rounds")
	fs.IntVar(&o.Round0, "round0", 0, "number of the first round")
	fs.StringVar(&o.Parse, "parse", "", "gobench:UNIT (such as gobench:p50-ms) or jsrun")
	fs.StringVar(&o.Dir, "dir", "", "working folder of the processes")
	fs.Var(&envs, "env", "K=V for the processes (repeatable)")
	fs.IntVar(&o.CPU, "cpu", -1, "the timing core, to check that nothing else ran on it (-1: no check)")
	fs.IntVar(&o.Sib, "sib", -1, "its hyperthread sibling, which must stay idle (-1: no check)")
	fs.Float64Var(&o.BusyPct, "busy", 10, "percent of busy sibling or other work that makes a process run again")
	fs.IntVar(&o.Tries, "tries", 3, "tries of a disturbed process")
	fs.DurationVar(&o.Timeout, "timeout", 30*time.Minute, "limit of one process")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.Env, o.Args = envs, fs.Args()
	o.Sides, o.Pos = csv(*sides), csv(*pos)
	if o.Bin == "" || o.Out == "" || o.Parse == "" {
		return fmt.Errorf("-bin, -out and -parse are needed")
	}
	return bench.Run(ctx, o, bench.Start, bench.ProcTicks, os.Stdout)
}

func benchReport(args []string) error {
	fs := newFlags("bench report")
	in := fs.String("in", "", "file of records")
	kind := fs.String("kind", "run", "kind to read")
	base := fs.String("base", "", "side that is the base")
	nw := fs.String("new", "", "side compared with it")
	detail := fs.Bool("detail", false, "also the ratio at each position")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *base == "" || *nw == "" {
		return fmt.Errorf("-in, -base and -new are needed")
	}
	rs, err := bench.ReadRecords(*in)
	if err != nil {
		return err
	}
	rows, err := bench.Compare(rs, *kind, *base, *nw)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s against %s\n\n", *kind, *nw, *base)
	bench.WriteTable(os.Stdout, rows, *base, *nw, *detail)
	if n := bench.Disturbed(rs, *kind); n > 0 {
		fmt.Printf("\n%d processes still ran with a busy sibling core or other work after all their tries. Run again on a quiet machine.\n", n)
	}
	if mem := bench.Memory(rs, *kind); len(mem) > 0 {
		fmt.Println()
		bench.WriteMemory(os.Stdout, mem)
	}
	return nil
}

func benchCG(ctx context.Context, args []string) error {
	fs := newFlags("bench cg")
	var bins list
	var envs list
	var o bench.CGOptions
	fs.Var(&bins, "bin", "binary to count (give a second one to compare)")
	fs.IntVar(&o.Small, "small", 2, "renders of the small run")
	fs.IntVar(&o.Big, "big", 12, "renders of the big run")
	fs.StringVar(&o.Dir, "dir", "", "working folder")
	fs.Var(&envs, "env", "K=V for the processes (repeatable)")
	fs.StringVar(&o.Toggle, "toggle", "", "collect only inside this function")
	fs.StringVar(&o.ValgrindRoot, "valgrind-root", "", "folder with usr/bin/valgrind, when it is not installed")
	fs.StringVar(&o.Out, "out", "", "folder for the callgrind files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.Bins, o.Env, o.Args = bins, envs, fs.Args()
	return bench.CG(ctx, o, sjdev.Exec, os.Stdout)
}

func benchCGDiff(args []string) error {
	fs := newFlags("bench cgdiff")
	renders := fs.Float64("renders", 10, "renders the big runs did more than the small ones")
	if err := fs.Parse(args); err != nil {
		return err
	}
	f := fs.Args()
	if len(f) != 2 && len(f) != 4 {
		return fmt.Errorf("give SMALL.incl BIG.incl, and a second pair to compare")
	}
	read := func(p string) bench.Listing {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sjdev:", err)
			os.Exit(1)
		}
		return bench.ParseListing(string(b))
	}
	a := bench.Diff(*renders, read(f[0]), read(f[1]))
	if len(f) == 2 {
		bench.WriteDiff(os.Stdout, a, nil)
		return nil
	}
	b := bench.Diff(*renders, read(f[2]), read(f[3]))
	bench.WriteDiff(os.Stdout, a, &b)
	return nil
}
