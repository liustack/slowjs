// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// The referee: where the two loops of the engine give a script different
// output, a third run under Node says which side the specification is on.
//
//	rvgen referee -rvdiff BIN [-node NODE] [-t SECONDS] [-strict] FILE...
//
// It runs rvdiff -v -o on each FILE for the two transcripts, and the script
// under Node with a small host that gives it print and console.log and
// writes the same lines rvdiff does (out, result, exception). Only those
// kinds are judged: stacks, memory counts and the status are not something
// Node can say anything about. The text of an error differs between the two
// engines, so an exception is compared by its name, and a line is also
// compared with everything from the first error name on cut off ("loose").
// It prints one line per FILE:
//
//	REFEREE FILE: node agrees with off|on|neither|both, ...
//
// and the first line where the loops differ with Node's line beside it.
// fuzz -node NODE runs it on every program that differs.

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/liustack/quickjs/internal/rvgen"
)

// the host of the Node run, given the script, the options of its
// "// jsrun:" line, and whether to run it strict
const nodeHost = `'use strict';
const fs = require('fs'), vm = require('vm');
const [file, flagsArg] = process.argv.slice(2);
const flags = flagsArg ? flagsArg.split(' ') : [];
const w = (s) => process.stdout.write(s);
function lines(kind, s) {
  if (s.length === 0) { w(kind + ' \n'); return; }
  let i = 0;
  while (i < s.length) {
    let j = s.indexOf('\n', i);
    if (j < 0) j = s.length;
    w(kind + ' ' + s.slice(i, j) + '\n');
    i = j + 1;
  }
}
function toStr(v) {
  if (typeof v === 'symbol') throw new TypeError('cannot convert symbol to string');
  return String(v);
}
function value(kind, v) {
  let s;
  try { s = toStr(v); } catch (e) { w(kind + ' (no string)\n'); return; }
  lines(kind, s);
}
function exception(e) {
  value('exception', e);
}
function print(...args) { lines('out', args.map(toStr).join(' ')); }
globalThis.print = print;
globalThis.console = { log: print };
globalThis.gc = function () {};
process.on('unhandledRejection', () => {});
const drain = () => new Promise((r) => setImmediate(r));
const formatRaw = (v) => typeof v === 'function' ? String(v()) : String(v);
const format = (v) => {
  const s = typeof v === 'function' ? String(v()) : String(v);
  if (s.isWellFormed()) return s;
  let r = '';
  for (const c of s) {
    const u = c.charCodeAt(0);
    r += c.length === 1 && u >= 0xd800 && u <= 0xdfff ? '\\u{' + u.toString(16) + '}' : c;
  }
  return r;
};
(async () => {
  let src = fs.readFileSync(file, 'utf8');
  if (flags.includes('-strict')) src = '"use strict";' + src;
  if (flags.includes('-call')) src = 'function run() {\n' + src + '\n}';
  let v;
  try {
    v = vm.runInThisContext(src, { filename: file });
    if (flags.includes('-call')) v = globalThis.run();
  } catch (e) { exception(e); return; }
  await drain();
  if (typeof v === 'function') {
    try { v = v(); } catch (e) { exception(e); return; }
    await drain();
  }
  if (flags.includes('-result') || flags.includes('-call')) v = globalThis.result;
  let s;
  try { s = (flags.includes('-raw') ? formatRaw : format)(v); } catch (e) { exception(e); return; }
  value('result', s);
})();
`

// the kinds of a transcript Node can judge
var judged = map[string]bool{"out": true, "result": true, "exception": true, "job": true}

// transcriptLines keeps the lines of the kinds Node can judge, up to the
// info or the status of the run
func transcriptLines(tr string) []string {
	var r []string
	for _, l := range strings.Split(tr, "\n") {
		if strings.HasPrefix(l, "--- ") {
			break
		}
		k, _, _ := strings.Cut(l, " ")
		if judged[k] {
			r = append(r, l)
		}
	}
	return r
}

var (
	errName = regexp.MustCompile(`([A-Za-z]*Error)\b.*$`)
	sumLine = regexp.MustCompile(`^result sum -?[0-9]+$`)
)

// exact makes the lines of an exception its name: the texts of errors
// differ between the engines
func exact(ls []string) []string {
	r := make([]string, len(ls))
	for i, l := range ls {
		if strings.HasPrefix(l, "exception ") || strings.HasPrefix(l, "job ") {
			l = errName.ReplaceAllString(l, "$1")
		}
		r[i] = l
	}
	return r
}

// loose also cuts every line at its first error name, and the checksum of
// a generated program, which the texts of the errors go into
func loose(ls []string) []string {
	r := make([]string, len(ls))
	for i, l := range exact(ls) {
		l = errName.ReplaceAllString(l, "$1")
		if sumLine.MatchString(l) {
			l = "result sum"
		}
		r[i] = l
	}
	return r
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func at(ls []string, i int) string {
	if i < len(ls) {
		return ls[i]
	}
	return "(none)"
}

// side names which of off and on the lines of Node equal
func side(n, off, on []string) string {
	a, b := same(n, off), same(n, on)
	switch {
	case a && b:
		return "both"
	case a:
		return "off"
	case b:
		return "on"
	}
	return "neither"
}

// scriptFlags reads the options of the "// jsrun:" line of a script
func scriptFlags(src string) []string {
	for _, l := range strings.Split(src, "\n") {
		if rest, ok := strings.CutPrefix(l, "// jsrun:"); ok {
			return strings.Fields(rest)
		}
	}
	return nil
}

// referee runs file both ways and under node and says which side Node is on
func referee(bin, node, file string, strict bool) string {
	src, err := os.ReadFile(file)
	if err != nil {
		return "cannot read: " + err.Error()
	}
	if strings.HasSuffix(file, ".mjs") {
		return "a module: not judged"
	}
	dir, err := os.MkdirTemp("", "rvref")
	must(err)
	defer os.RemoveAll(dir)
	args := []string{"-q", "-v", "-o", dir, "-t", fmt.Sprint(limit)}
	if strict {
		args = append(args, "-strict")
	}
	cmd := exec.Command(bin, append(args, file)...)
	var o bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &o
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
			return fmt.Sprintf("rvdiff: %v: %s", err, strings.TrimSpace(o.String()))
		}
	}
	offs, _ := filepath.Glob(filepath.Join(dir, "*.off"))
	if len(offs) != 1 {
		return "no transcripts (too slow to compare?)"
	}
	offT, err := os.ReadFile(offs[0])
	must(err)
	onT, err := os.ReadFile(strings.TrimSuffix(offs[0], ".off") + ".on")
	must(err)
	off, on := transcriptLines(string(offT)), transcriptLines(string(onT))
	if same(off, on) {
		return "the loops give the same out, result and exception: nothing for Node to judge"
	}

	host := filepath.Join(dir, "node-host.cjs")
	must(os.WriteFile(host, []byte(nodeHost), 0o666))
	flags := scriptFlags(string(src))
	if strict {
		flags = append(flags, "-strict")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(limit)*time.Second)
	defer cancel()
	nc := exec.CommandContext(ctx, node, host, file, strings.Join(flags, " "))
	var no, ne bytes.Buffer
	nc.Stdout, nc.Stderr = &no, &ne
	if err := nc.Run(); err != nil && ctx.Err() != nil {
		return "node ran out of time"
	}
	n := transcriptLines(no.String())
	if len(n) == 0 {
		return "node wrote nothing: " + strings.TrimSpace(ne.String())
	}

	verdict := side(exact(n), exact(off), exact(on))
	how := "exact"
	if verdict == "neither" {
		verdict, how = side(loose(n), loose(off), loose(on)), "loose"
	}
	// the first line where the loops differ, and Node's line there
	first := 0
	for first < len(off) && first < len(on) && off[first] == on[first] {
		first++
	}
	if verdict == "neither" {
		verdict += ", " + lean(at(off, first), at(on, first), at(n, first))
	}
	return fmt.Sprintf("node agrees with %s (%s); first difference, line %d:\n    off  | %.200s\n    on   | %.200s\n    node | %.200s",
		verdict, how, first+1, at(off, first), at(on, first), at(n, first))
}

var word = regexp.MustCompile(`[A-Za-z0-9_$]+`)

// lean says which side Node's line is nearer, by the words only one side
// of the loops has (a frame name of a stack, a value): Node words its
// errors and stacks its own way, but has the same names and values
func lean(off, on, node string) string {
	set := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, w := range word.FindAllString(s, -1) {
			m[w] = true
		}
		return m
	}
	a, b, n := set(off), set(on), set(node)
	var offHits, onHits, offOnly, onOnly int
	for w := range a {
		if !b[w] {
			offOnly++
			if n[w] {
				offHits++
			}
		}
	}
	for w := range b {
		if !a[w] {
			onOnly++
			if n[w] {
				onHits++
			}
		}
	}
	// a word only one side has counts for that side when Node has it, and
	// for the other side when Node does not
	offScore, onScore := offHits+onOnly-onHits, onHits+offOnly-offHits
	how := fmt.Sprintf("node has %d of the %d words only off has, %d of the %d only on has", offHits, offOnly, onHits, onOnly)
	switch {
	case offScore > onScore:
		return "leans off (" + how + ")"
	case onScore > offScore:
		return "leans on (" + how + ")"
	}
	return "no lean"
}

func refereeCmd(args []string) {
	fs := flag.NewFlagSet("referee", flag.ExitOnError)
	bin := fs.String("rvdiff", "", "the rvdiff program")
	node := fs.String("node", "node", "the node program (Node 24)")
	strict := fs.Bool("strict", false, "run the scripts strict (rvdiff -strict)")
	fs.IntVar(&limit, "t", 30, "seconds a run may take")
	fs.Parse(args)
	if fs.NArg() == 0 || *bin == "" {
		usage()
	}
	for _, f := range fs.Args() {
		fmt.Printf("REFEREE %s: %s\n", f, referee(*bin, *node, f, *strict))
	}
}

// runNode runs file under node with the host of nodeHost (in dir) and
// returns the lines Node can judge
func runNode(node, dir, file string, flags []string) ([]string, error) {
	host := filepath.Join(dir, "node-host.cjs")
	if _, err := os.Stat(host); err != nil {
		must(os.WriteFile(host, []byte(nodeHost), 0o666))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(limit)*time.Second)
	defer cancel()
	nc := exec.CommandContext(ctx, node, host, file, strings.Join(flags, " "))
	var no, ne bytes.Buffer
	nc.Stdout, nc.Stderr = &no, &ne
	if err := nc.Run(); err != nil && ctx.Err() != nil {
		return nil, fmt.Errorf("node ran out of time")
	}
	n := transcriptLines(no.String())
	if len(n) == 0 {
		return nil, fmt.Errorf("node wrote nothing: %s", strings.TrimSpace(ne.String()))
	}
	return n, nil
}

// nodeCmd: new programs, one run of the engine (the register loop on)
// against Node, for the differences two correct engines may have
//
//	rvgen node -rvdiff BIN [-node NODE] [-portable] [-core|-wide] [-seed S] [-n N] [-j J] [-t T] [-keep DIR]
func nodeCmd(args []string) {
	fs := flag.NewFlagSet("node", flag.ExitOnError)
	bin := fs.String("rvdiff", "", "the rvdiff program")
	node := fs.String("node", "node", "the node program (Node 24)")
	seed := fs.Uint64("seed", 1, "first seed")
	n := fs.Int("n", 100, "programs")
	batch := fs.Int("batch", 100, "programs a run of rvdiff")
	jobs := fs.Int("j", 4, "programs at a time")
	keep := fs.String("keep", "", "a folder for each program that differs and both outputs")
	fs.IntVar(&limit, "t", 30, "seconds a run may take")
	o := opts(fs)
	fs.Parse(args)
	if *bin == "" {
		usage()
	}
	gen := func(s uint64) string { return rvgen.Generate(s, *o) }
	var total, differ, failed int
	for start := 0; start < *n; start += *batch {
		var seeds []uint64
		for i := start; i < *n && i < start+*batch; i++ {
			seeds = append(seeds, *seed+uint64(i))
		}
		bd, td, out, err := runBatch(*bin, *jobs, seeds, gen, "-golden", "DIR", "-update", "-full")
		if err != nil {
			fmt.Fprintf(os.Stderr, "rvgen: rvdiff: %v\n%s", err, out)
			os.Exit(2)
		}
		for _, s := range seeds {
			total++
			name := fmt.Sprintf("p-%d.js", s)
			tb, err := os.ReadFile(filepath.Join(td, name+".txt"))
			if err != nil {
				failed++
				fmt.Printf("NORUN %s: the engine wrote no transcript (a report or the time limit)\n", name)
				continue
			}
			q := transcriptLines(string(tb))
			nl, err := runNode(*node, bd, filepath.Join(bd, "p", name), nil)
			if err != nil {
				failed++
				fmt.Printf("NONODE %s: %v\n", name, err)
				continue
			}
			if same(exact(q), exact(nl)) {
				continue
			}
			differ++
			i := 0
			for i < len(q) && i < len(nl) && exact(q)[i] == exact(nl)[i] {
				i++
			}
			fmt.Printf("NODE %s: line %d\n    qjs  | %.300s\n    node | %.300s\n", name, i+1, at(q, i), at(nl, i))
			if *keep != "" {
				must(os.MkdirAll(*keep, 0o777))
				must(os.WriteFile(filepath.Join(*keep, name), []byte(gen(s)), 0o666))
				must(os.WriteFile(filepath.Join(*keep, name+".qjs"), []byte(strings.Join(q, "\n")+"\n"), 0o666))
				must(os.WriteFile(filepath.Join(*keep, name+".node"), []byte(strings.Join(nl, "\n")+"\n"), 0o666))
			}
		}
		os.RemoveAll(bd)
	}
	fmt.Printf("rvgen node: %d programs, %d differ from Node, %d not run\n", total, differ, failed)
	if differ > 0 {
		os.Exit(1)
	}
}
