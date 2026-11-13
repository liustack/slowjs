// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// New programs against Node, for the differences two correct engines may
// have:
//
//	rvgen node -rvdiff BIN [-node NODE] [-portable] [-core|-wide] [-captures] [-seed S] [-n N] [-j J] [-t T] [-keep DIR]
//
// It runs each program once on the engine (rvdiff -golden -full into an
// empty folder, which writes the run) and once under Node 24, with a small
// host that gives the script print and console.log and writes the same
// lines rvdiff does (out, result, exception, job). Only those kinds are
// compared: stacks, memory counts and the status are not something Node can
// say anything about. The text of an error differs between the two engines,
// so an exception is compared by its name. -portable makes programs that
// write the name of a caught error, not its message.

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

var errName = regexp.MustCompile(`([A-Za-z]*Error)\b.*$`)

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
