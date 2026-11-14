// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs"
)

// bin is the command, built once by TestMain with a version stamped in, as
// the release job stamps it.
var bin string

const testVersion = "v0.0.0-test"

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "slowjs-test")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "slowjs")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags=-X main.version="+testVersion, "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic("building slowjs: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
	took           time.Duration
}

// runCmd runs the command with args, and fails the test when it is still
// running after two minutes.
func runCmd(t *testing.T, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	r := result{stdout: stdout.String(), stderr: stderr.String(), took: time.Since(start)}
	if ctx.Err() != nil {
		t.Fatalf("slowjs %q did not end in two minutes", args)
	}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.code = exit.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return r
}

// script writes src to a file of the test's own folder and returns its path.
func script(t *testing.T, src string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.js")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (r result) want(t *testing.T, code int, stdout string) {
	t.Helper()
	if r.code != code || r.stdout != stdout {
		t.Fatalf("exit %d, stdout %q, stderr %q\nwant exit %d, stdout %q", r.code, r.stdout, r.stderr, code, stdout)
	}
}

func TestPrint(t *testing.T) {
	r := runCmd(t, script(t, `
print("a", 1, true);
console.log("b", null, undefined, {}, [1, 2], Symbol("s"));
var p = console.log; p("unbound");
var q = print; q();
console.error("to", "stderr");
`))
	r.want(t, 0, "a 1 true\nb null undefined [object Object] 1,2 Symbol(s)\nunbound\n\n")
	if r.stderr != "to stderr\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
}

// TestPrintOrder: what goes to stdout before a line of stderr is written
// before it, so that a terminal shows them in the order the script printed.
func TestPrintOrder(t *testing.T) {
	cmd := exec.Command(bin, script(t, `print(1); console.error(2); print(3)`))
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "1\n2\n3\n" {
		t.Fatalf("%q, %v", out, err)
	}
}

// TestPrintLong: a string is written in pieces, and a piece never ends
// between the two halves of a surrogate pair. A lone surrogate is U+FFFD.
func TestPrintLong(t *testing.T) {
	r := runCmd(t, script(t, `
var s = "a".repeat(65535) + "😀" + "b".repeat(200000) + "\uD800";
print(s);
`))
	want := strings.Repeat("a", 65535) + "\U0001F600" + strings.Repeat("b", 200000) + "�\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("exit %d, %d bytes of stdout, want %d, stderr %q", r.code, len(r.stdout), len(want), r.stderr)
	}
}

func TestPrintThrows(t *testing.T) {
	r := runCmd(t, script(t, `print({ toString() { throw new TypeError("no string") } })`))
	if r.code != exitException || !strings.Contains(r.stderr, "TypeError: no string") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestScriptArgs(t *testing.T) {
	p := script(t, `print(JSON.stringify(scriptArgs))`)
	runCmd(t, p, "x", "-y", "--z").want(t, 0, `["`+strings.ReplaceAll(p, `\`, `\\`)+`","x","-y","--z"]`+"\n")
	runCmd(t, "-e", `print(JSON.stringify(scriptArgs))`, "a", "b").want(t, 0, `["a","b"]`+"\n")
}

func TestUncaughtException(t *testing.T) {
	r := runCmd(t, script(t, `print("before");
function f() { throw new RangeError("boom") }
f();
print("after");
`))
	r.want(t, exitException, "before\n")
	if !strings.Contains(r.stderr, "RangeError: boom\n") || !strings.Contains(r.stderr, "at f (") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

// TestDeepStack: the stack of an exception thrown deep in a recursion is
// cut to its first frames.
func TestDeepStack(t *testing.T) {
	r := runCmd(t, "-e", `function f() { f() } f()`)
	r.want(t, exitException, "")
	lines := strings.Split(strings.TrimRight(r.stderr, "\n"), "\n")
	if len(lines) != stackFrames+2 || !strings.HasPrefix(lines[0], "RangeError") || !strings.HasSuffix(lines[len(lines)-1], " more frames") {
		t.Fatalf("%d lines: first %q, last %q", len(lines), lines[0], lines[len(lines)-1])
	}
}

func TestThrowNotAnError(t *testing.T) {
	r := runCmd(t, "-e", `throw 42`)
	r.want(t, exitException, "")
	if !strings.Contains(r.stderr, "42") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

func TestSyntaxError(t *testing.T) {
	r := runCmd(t, script(t, "print(1);\nvar = ;\n"))
	r.want(t, exitException, "")
	if !strings.Contains(r.stderr, "SyntaxError") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

func TestPendingJobs(t *testing.T) {
	runCmd(t, script(t, `
Promise.resolve(1).then(function (v) { print("then", v) });
(async function () { await null; print("async") })();
print("sync");
`)).want(t, 0, "sync\nthen 1\nasync\n")
}

// TestTimeout: the timeout ends the script, and no catch or finally runs.
func TestTimeout(t *testing.T) {
	r := runCmd(t, "--timeout", "200ms", script(t, `
print("start");
try { for (;;) {} } catch (e) { print("caught") } finally { print("finally") }
`))
	r.want(t, exitTimeout, "start\n")
	if !strings.Contains(r.stderr, "200ms") || !strings.Contains(r.stderr, "--timeout") {
		t.Fatalf("stderr %q", r.stderr)
	}
	if r.took > 30*time.Second {
		t.Fatalf("took %v", r.took)
	}
}

// TestTimeoutCoversJobs: the timeout is for the whole run, the script and
// the promise jobs it queued, not for each.
func TestTimeoutCoversJobs(t *testing.T) {
	r := runCmd(t, "--timeout", "300ms", script(t, `
var end = Date.now() + 200; while (Date.now() < end) {}
Promise.resolve().then(function () { var end = Date.now() + 200; while (Date.now() < end) {} print("job ended") });
`))
	r.want(t, exitTimeout, "")
}

func TestNoTimeout(t *testing.T) {
	runCmd(t, "--timeout", "0", "-e", `var end = Date.now() + 50; while (Date.now() < end) {} print("done")`).want(t, 0, "done\n")
}

// TestMemoryLimit: running out of memory ends the script, and no catch
// sees it.
func TestMemoryLimit(t *testing.T) {
	r := runCmd(t, "--memory-limit", "32MiB", script(t, `
var a = [];
try { for (;;) a.push(new Array(1000).fill(1)) } catch (e) { print("caught") }
`))
	r.want(t, exitMemory, "")
	if !strings.Contains(r.stderr, "memory") || !strings.Contains(r.stderr, "--memory-limit") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

// TestMemoryLimitString: a long string is under the limit too, and one that
// fits under it prints.
func TestMemoryLimitString(t *testing.T) {
	r := runCmd(t, "--memory-limit", "16MiB", "-e", `var s = "x"; for (;;) s = s.repeat(2)`)
	r.want(t, exitMemory, "")
	r = runCmd(t, "--memory-limit", "64MiB", "-e", `print("x".repeat(20 << 20))`)
	r.want(t, 0, strings.Repeat("x", 20<<20)+"\n")
}

func TestNoMemoryLimit(t *testing.T) {
	runCmd(t, "--memory-limit", "0", "-e", `var a = new Array(1e6).fill(0); print(a.length)`).want(t, 0, "1000000\n")
}

func TestVersion(t *testing.T) {
	runCmd(t, "--version").want(t, 0, "slowjs "+testVersion+"\n")
}

func TestHelp(t *testing.T) {
	r := runCmd(t, "-h")
	if r.code != 0 || !strings.Contains(r.stdout, "usage: slowjs") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--no-such-flag", "x.js"},
		{"--timeout", "soon", "x.js"},
		{"--timeout", "-1s", "x.js"},
		{"--memory-limit", "lots", "x.js"},
		{"--memory-limit", "-5", "x.js"},
		{filepath.Join(t.TempDir(), "missing.js")},
	} {
		r := runCmd(t, args...)
		if r.code != exitUsage || r.stdout != "" || r.stderr == "" {
			t.Errorf("slowjs %q: exit %d, stdout %q, stderr %q", args, r.code, r.stdout, r.stderr)
		}
	}
}

func TestParseSize(t *testing.T) {
	for s, want := range map[string]uint64{
		"0": 0, "1024": 1024, "64KiB": 64 << 10, "64K": 64 << 10, "512MiB": 512 << 20,
		"512M": 512 << 20, "1GiB": 1 << 30, "1G": 1 << 30, "2gib": 2 << 30,
	} {
		if got, err := parseSize(s); err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v, want %d", s, got, err, want)
		}
	}
	for _, s := range []string{"", "-1", "1.5G", "G", "1T", "1 G", "99999999999999999999", "17179869184G"} {
		if got, err := parseSize(s); err == nil {
			t.Errorf("parseSize(%q) = %d, want an error", s, got)
		}
	}
}

// v8Dir is the V8 benchmark suite, version 7, that compare/engines runs.
const v8Dir = "../../internal/arewefastyet/v8-v7"

var v8Benchmarks = []string{"Richards", "DeltaBlue", "Crypto", "RayTrace", "EarleyBoyer", "RegExp", "Splay", "NavierStokes"}

// v8Script is the suite as one file, as a benchmark harness that runs
// engines from the command line makes it: run.js with each load('x.js')
// replaced by the file, and print taken from console.log when there is one,
// which calls console.log unbound.
func v8Script(t *testing.T) string {
	t.Helper()
	run, err := os.ReadFile(filepath.Join(v8Dir, "run.js"))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for line := range strings.Lines(string(run)) {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "load('"); ok {
			src, err := os.ReadFile(filepath.Join(v8Dir, strings.TrimSuffix(name, "');")))
			if err != nil {
				t.Fatal(err)
			}
			b.Write(src)
			b.WriteString("\n")
			continue
		}
		if strings.HasPrefix(line, "var success = true;") {
			b.WriteString("if (typeof print === 'undefined' || (typeof console !== 'undefined' && typeof console.log === 'function')) {\n  var print = console.log\n}\n")
		}
		b.WriteString(line)
	}
	return b.String()
}

// TestV8Suite runs the V8 benchmark suite through the command, as a
// harness does, and checks that it prints a score for each benchmark and
// the total, and that the benchmarks whose own checks pass are those that
// pass when the suite runs in a VM of the library. It times nothing.
func TestV8Suite(t *testing.T) {
	if testing.Short() {
		t.Skip("the suite takes about 20 seconds, twice")
	}
	r := runCmd(t, script(t, v8Script(t)))
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q, stdout %q", r.code, r.stderr, r.stdout)
	}
	got := map[string]string{}
	for line := range strings.Lines(r.stdout) {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), ": "); ok {
			got[k] = v
		}
	}
	for _, name := range append(v8Benchmarks, "Score (version 7)") {
		if n, err := strconv.ParseFloat(got[name], 64); err != nil || n <= 0 {
			t.Errorf("%s: %q, want a score\n%s", name, got[name], r.stdout)
		}
	}

	lib := v8Library(t)
	for _, name := range v8Benchmarks {
		_, cmdErr := strconv.ParseFloat(got[name], 64)
		_, libErr := strconv.ParseFloat(lib[name], 64)
		if (cmdErr == nil) != (libErr == nil) {
			t.Errorf("%s: %q through the command, %q through the library", name, got[name], lib[name])
		}
	}
}

// v8Library runs the suite in a VM of the library, each file a global
// script, as compare/engines runs it, and returns each benchmark's result.
func v8Library(t *testing.T) map[string]string {
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	for _, f := range []string{"base.js", "richards.js", "deltablue.js", "crypto.js", "raytrace.js", "earley-boyer.js", "regexp.js", "splay.js", "navier-stokes.js"} {
		src, err := os.ReadFile(filepath.Join(v8Dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := vm.Eval(string(src), quickjs.EvalGlobal); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	out, err := vm.Eval(`var res = {};
BenchmarkSuite.RunSuites({
  NotifyResult: function (name, result) { res[name] = String(result) },
  NotifyError: function (name, error) { res[name] = "error: " + error },
  NotifyScore: function (score) {}
});
JSON.stringify(res)`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	res := map[string]string{}
	if err := json.Unmarshal([]byte(out.(string)), &res); err != nil {
		t.Fatal(err)
	}
	return res
}
