// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Command beastjs runs a JavaScript file in the engine of this module, set up
// as tifa sets it up, without writing Go:
//
//	beastjs [flags] file.js [args...]
//	beastjs [flags] -e code [args...]
//
// The script runs as a global script in one VM, then the promise jobs it
// queued. The VM has what package tifa gives it, as tifa gives it to each VM
// of its pool: Intl, Temporal, URL and URLSearchParams, and the functions
// named __tifa_* of tifa.Hooks. It also has Go's fmt as fmt, and calls may
// nest 10000 deep, as in tifa. The script also has print, console.log and
// console.error, which write their arguments, each made a string, to stdout
// (stderr for console.error), and scriptArgs, the arguments after the flags.
// It has no file system, network, timers or modules. The run ends at the
// timeout and at the memory limit whatever the script catches.
// docs/use/command.md has the exit codes and why the defaults are what they are.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/liustack/beastjs"
	qjsfmt "github.com/liustack/beastjs/fmt"
	"github.com/liustack/beastjs/tifa"
)

// The exit codes, in docs/use/command.md.
const (
	exitOK        = 0
	exitException = 1 // an uncaught exception, a syntax error included
	exitUsage     = 2 // a wrong command line, or a script that cannot be read
	exitTimeout   = 3 // the run passed --timeout
	exitMemory    = 4 // the VM passed --memory-limit
)

// The defaults of the limits, in docs/use/command.md: a script run by hand, or
// one that shows a problem, that hangs or keeps allocating ends while
// someone still waits for it, and a longer or larger run asks for it.
const (
	defaultTimeout     = time.Minute
	defaultMemoryLimit = "1GiB"
)

// stackDepth is how deep calls may nest: tifa's default, the most the
// engine allows.
const stackDepth = 10000

// version is the release, stamped by the release job with
// -ldflags "-X main.version=<tag>". A build without it reports the version
// the go command recorded, such as the one of go install ...@<tag>.
var version string

func main() {
	os.Exit(run(os.Args[1:]))
}

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

const usage = `usage: beastjs [flags] file.js [args...]
       beastjs [flags] -e code [args...]

Runs a JavaScript file, or the code of -e, as a global script, then the
promise jobs it queued, in a VM set up as tifa sets it up: Intl, Temporal,
URL and URLSearchParams, the functions named __tifa_* that tifa gives its
bundles, and calls nested up to 10000 deep. The script also has Go's fmt
as fmt, print, console.log, console.error and scriptArgs (the arguments
after the flags). It has no file system, network, timers or modules.

Flags:
  -e code              run code instead of a file
  --timeout d          end the run after d, such as 30s or 2m (0: never) (default %v)
  --memory-limit n     end the run when the VM takes more than n bytes,
                       with K, M or G (KiB, MiB, GiB) allowed (0: no limit) (default %s)
  --version            print the version, the Go release and the platform, and exit
  -h, --help           print this and exit

Exit status: 0 done, 1 uncaught exception, 2 wrong command line or unreadable
file, 3 timeout, 4 out of memory.
`

func run(args []string) int {
	fs := flag.NewFlagSet("beastjs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	eval := fs.String("e", "", "")
	timeout := fs.Duration("timeout", defaultTimeout, "")
	limitText := fs.String("memory-limit", defaultMemoryLimit, "")
	showVersion := fs.Bool("version", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Printf(usage, defaultTimeout, defaultMemoryLimit)
			return exitOK
		}
		return usageError(err.Error())
	}
	if *showVersion {
		fmt.Printf("beastjs %s %s %s/%s\n", versionString(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return exitOK
	}
	if *timeout < 0 {
		return usageError(fmt.Sprintf("--timeout %v is negative", *timeout))
	}
	limit, err := parseSize(*limitText)
	if err != nil {
		return usageError(fmt.Sprintf("--memory-limit %s: %v", *limitText, err))
	}
	evalSet := false
	fs.Visit(func(f *flag.Flag) { evalSet = evalSet || f.Name == "e" })
	src, name := *eval, ""
	if !evalSet {
		if fs.NArg() == 0 {
			return usageError("no script to run")
		}
		b, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			return usageError(err.Error())
		}
		src, name = string(b), fs.Arg(0)
	}
	return runScript(name, src, fs.Args(), *timeout, limit, *limitText)
}

func usageError(msg string) int {
	fmt.Fprintf(os.Stderr, "beastjs: %s\n", msg)
	fmt.Fprintf(os.Stderr, "usage: beastjs [flags] file.js [args...], or beastjs -h for the flags\n")
	return exitUsage
}

// parseSize reads a number of bytes, with an optional suffix K, M or G
// (KiB, MiB and GiB, in any case).
func parseSize(s string) (uint64, error) {
	num, shift := s, 0
	lower := strings.ToLower(s)
	for _, u := range []struct {
		suffix string
		shift  int
	}{{"kib", 10}, {"mib", 20}, {"gib", 30}, {"k", 10}, {"m", 20}, {"g", 30}} {
		if strings.HasSuffix(lower, u.suffix) {
			num, shift = s[:len(s)-len(u.suffix)], u.shift
			break
		}
	}
	n, err := strconv.ParseUint(num, 10, 64)
	if err != nil {
		return 0, errors.New("not a number of bytes, such as 512MiB")
	}
	if n > math.MaxUint64>>shift {
		return 0, errors.New("too large")
	}
	return n << shift, nil
}

// out holds stdout and stderr. Each print or console call writes its line
// and then flushes, and console.error flushes stdout first, so the two
// streams keep the order the script wrote them in.
var out = [3]*bufio.Writer{
	1: bufio.NewWriterSize(os.Stdout, 64<<10),
	2: bufio.NewWriterSize(os.Stderr, 64<<10),
}

// write is write(fd, piece, last): it writes piece, a string of at most
// 65,536 UTF-16 units, to stdout (1) or stderr (2), and flushes after
// the last piece of a line. A write error ends the run as an exception.
var write = beastjs.NewBuiltin("write", 3, func(a beastjs.Args) (beastjs.Value, error) {
	fd, _ := a.Number(0)
	s, ok := a.String(1)
	if !ok {
		// Out of memory or a stop of the call, which ends it whatever
		// is returned.
		return beastjs.UndefinedValue, nil
	}
	last, _ := a.Bool(2)
	w := out[int(fd)]
	if fd == 2 {
		if err := out[1].Flush(); err != nil {
			return beastjs.UndefinedValue, err
		}
	}
	if _, err := w.WriteString(s); err != nil {
		return beastjs.UndefinedValue, err
	}
	if last {
		if err := w.Flush(); err != nil {
			return beastjs.UndefinedValue, err
		}
	}
	return beastjs.UndefinedValue, nil
})

// setup defines print, console and scriptArgs. A line is cut into pieces of
// 65,536 UTF-16 units, never between the two halves of a surrogate pair, so
// Go converts at most a piece at a time: a string the script made under its
// memory limit never has to be copied into Go whole. It keeps what it uses of
// String in its own variables, so a script that replaces them changes
// nothing here.
const setup = `(function (write, args) {
  "use strict";
  var str = String, units = 65536;
  var call = Function.prototype.call;
  var slice = call.bind(String.prototype.slice), code = call.bind(String.prototype.charCodeAt);
  function line(fd, a) {
    var s = "";
    for (var i = 0; i < a.length; i++) s += (i > 0 ? " " : "") + str(a[i]);
    s += "\n";
    for (var p = 0; p < s.length;) {
      var q = p + units;
      if (q >= s.length) q = s.length;
      else if ((code(s, q - 1) & 0xfc00) === 0xd800) q--;
      write(fd, slice(s, p, q), q === s.length);
      p = q;
    }
  }
  function print() { line(1, arguments) }
  globalThis.print = print;
  globalThis.console = {
    log: function log() { line(1, arguments) },
    error: function error() { line(2, arguments) }
  };
  globalThis.scriptArgs = JSON.parse(args);
})`

// runScript runs src, named name in its stack (the path as given, or
// <eval> for -e), and the jobs it queued in a new VM, under the limits, and
// returns the exit code.
func runScript(name, src string, args []string, timeout time.Duration, limit uint64, limitText string) (code int) {
	defer func() {
		out[1].Flush()
		out[2].Flush()
	}()
	vm, err := beastjs.NewVM()
	if err != nil {
		return failed(err, exitException)
	}
	defer vm.Close()
	if limit == 0 {
		vm.SetMemoryLimit(^uintptr(0))
	} else {
		vm.SetMemoryLimit(uintptr(limit))
	}

	// The timeout is for the whole run: each call into the VM gets the
	// time that is left.
	deadline := time.Now().Add(timeout)
	arm := func() error {
		if timeout == 0 {
			return vm.SetEvalTimeout(0)
		}
		return vm.SetEvalTimeout(max(time.Until(deadline), time.Nanosecond))
	}

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return failed(err, exitException)
	}
	if err = arm(); err == nil {
		err = register(vm)
	}
	if err == nil {
		err = arm()
	}
	if err == nil {
		err = define(vm, string(argsJSON))
	}
	if err == nil {
		err = arm()
	}
	if err == nil {
		var v beastjs.Value
		if v, err = vm.EvalFile(name, src, beastjs.EvalGlobal); err == nil {
			v.Free()
		}
	}
	if err == nil {
		err = arm()
	}
	if err == nil {
		_, err = vm.ExecutePendingJobs()
	}
	if err == nil {
		return exitOK
	}

	var e *beastjs.Error
	switch {
	case errors.As(err, &e) && e.Interrupted:
		return failed(fmt.Errorf("the run did not end within its timeout of %v (--timeout)", timeout), exitTimeout)
	case errors.As(err, &e) && e.OutOfMemory:
		return failed(fmt.Errorf("the script ran out of memory under its limit of %s (--memory-limit)", limitText), exitMemory)
	case errors.As(err, &e) && !e.Uncatchable:
		out[1].Flush()
		msg := e.Error()
		if e.Stack != "" {
			msg = strings.TrimRight(msg, "\n") + "\n" + stack(e.Stack)
		}
		fmt.Fprintln(out[2], msg)
		return exitException
	}
	return failed(err, exitException)
}

// register sets the VM up as tifa's renderer does, after the eval timeout:
// the call depth, then what tifa.Register defines (URL, Intl, Temporal and
// the functions named __tifa_*), then fmt, which tifa does not have.
func register(vm *beastjs.VM) error {
	vm.SetMaxStackSize(stackDepth)
	err := tifa.Register(vm)
	if err == nil {
		err = qjsfmt.Register(vm.GlobalObject())
	}
	return err
}

// define runs setup with the write Builtin and the script arguments.
func define(vm *beastjs.VM, argsJSON string) error {
	f, err := vm.EvalValue(setup, beastjs.EvalGlobal)
	if err != nil {
		return err
	}
	defer f.Free()
	w, err := vm.NewBuiltinFunction(write)
	if err != nil {
		return err
	}
	defer w.Free()
	r, err := f.CallValue(beastjs.UndefinedValue, w, argsJSON)
	if err == nil {
		r.Free()
	}
	return err
}

func failed(err error, code int) int {
	out[1].Flush()
	fmt.Fprintf(out[2], "beastjs: %v\n", err)
	return code
}

// stackFrames is how many frames of an uncaught exception are printed. The
// engine keeps every frame, thousands after a deep recursion.
const stackFrames = 50

// stack returns the first stackFrames lines of s, and how many it left out.
func stack(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= stackFrames {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:stackFrames], "\n") + fmt.Sprintf("\n    ... %d more frames", len(lines)-stackFrames)
}
