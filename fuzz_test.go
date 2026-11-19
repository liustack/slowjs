// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// The fuzz targets of the Go API: the parser and compiler, RegExp and JSON,
// each run in VMs with an eval timeout and a memory limit. Every input must
// keep the contract of a call:
//
//   - no panic and no crash of the process,
//   - an error is an *Error made of an exception, or one of the errors the
//     API documents (fuzzReasoned), never another error,
//   - the eval timeout and the memory limit end every call: a call returns
//     within fuzzSlack of its timeout, and fuzzWatchdog ends the process on a
//     call that does not return at all,
//   - a call that returns a value leaves no exception pending,
//   - Close frees all a VM holds: the engine has no block in use when its
//     runtime is freed, and the host keeps no chunk and no stack.
//
// Run one with go test -run '^$' -fuzz '^FuzzEval$' -fuzztime 3m . (see
// docs/testing.md). While fuzzing, the scripts of the repository are seeds
// too (fuzzRepoSeeds), and SLOWJS_FUZZ_SEEDS lists folders of more, such as
// test262/test.

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

const (
	// fuzzTimeout is the eval timeout of a VM of a fuzz target.
	fuzzTimeout = 100 * time.Millisecond
	// fuzzSlack is how long past its timeout a call may return: the kill
	// grace (50 ms and KillPolls polls), with room for a loaded machine.
	fuzzSlack = 2 * time.Second
	// fuzzWatchdog ends the process when a call has not returned this long
	// after it started: the fuzzer keeps the input as a crash.
	fuzzWatchdog = 30 * time.Second
	// fuzzMaxInput is the longest input a target runs. A test262 file is
	// shorter.
	fuzzMaxInput = 256 << 10
)

// fuzzLimits are the memory limits a target picks from: room for a script
// to work, and less, so that running out of memory comes in the parser, the
// compiler and the built-ins too.
var fuzzLimits = [...]uintptr{32 << 20, 4 << 20, 1 << 20, 384 << 10}

// fuzzContract is the name of the error the scripts of the targets throw
// when what they check of the engine does not hold.
const fuzzContract = "FuzzContract"

// fuzzVM is a VM of a fuzz target and what it checks.
type fuzzVM struct {
	*VM
	t      *testing.T
	killed bool // a call was killed: only Close is left
}

// newFuzzVM makes a VM with the eval timeout and the memory limit
// fuzzLimits[limit%len].
func newFuzzVM(t *testing.T, limit int) *fuzzVM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetEvalTimeout(fuzzTimeout); err != nil {
		m.Close()
		t.Fatal(err)
	}
	m.SetMemoryLimit(fuzzLimits[limit%len(fuzzLimits)])
	return &fuzzVM{VM: m, t: t}
}

// call runs f, a call into javascript, under the watchdog, and checks how
// long it took and the error it returned. It reports whether the VM can
// take more calls: false after an uncatchable error.
func (f *fuzzVM) call(what string, fn func() error) (ok bool) {
	f.t.Helper()
	if f.killed {
		return false
	}
	start := time.Now()
	dog := time.AfterFunc(fuzzWatchdog, func() {
		debug.SetTraceback("all")
		panic(fmt.Sprintf("fuzz: %s has not returned after %v, with an eval timeout of %v", what, fuzzWatchdog, fuzzTimeout))
	})
	err := fn()
	dog.Stop()
	if took := time.Since(start); took > fuzzTimeout+fuzzSlack {
		f.t.Fatalf("%s took %v with an eval timeout of %v (%v)", what, took, fuzzTimeout, err)
	}
	if err == nil {
		f.noException(what)
		return true
	}
	if errors.Is(err, ErrKilled) {
		f.killed = true
		return false
	}
	var e *Error
	if errors.As(err, &e) && e.engine {
		if e.Name == fuzzContract {
			f.t.Fatalf("%s: %s", what, e.Message)
		}
		return !e.Uncatchable
	}
	if fuzzReasoned(err) {
		return true
	}
	f.t.Fatalf("%s: an error with no reason: %T %v", what, err, err)
	return false
}

// fuzzReasoned reports whether err is one of the errors the API returns for
// a reason it documents, which a script can cause.
func fuzzReasoned(err error) bool {
	if errors.Is(err, errAwaitPending) {
		return true // a module whose top-level await never settles
	}
	switch s := err.Error(); {
	case strings.HasPrefix(s, "slowjs: aborted draining after"):
		return true // a job queue that feeds itself (ExecutePendingJobs)
	case s == "failed to get BigInt.toString()", s == "big.Int.SetString failed":
		return true // the script replaced BigInt.prototype.toString
	}
	return false
}

// noException checks that no exception is pending after a call that
// returned no error.
func (f *fuzzVM) noException(what string) {
	f.t.Helper()
	e := f.jsGetException()
	if tag(e) != tagUninitialized {
		f.jsFreeValue(e)
		f.t.Fatalf("%s returned with an exception pending (tag %d)", what, tag(e))
	}
}

// eval runs EvalValue and frees what it returns.
func (f *fuzzVM) eval(what, js string, flags int) bool {
	f.t.Helper()
	return f.call(what, func() error {
		v, err := f.EvalValue(js, flags)
		if err == nil {
			v.Free()
		}
		return err
	})
}

// jobs runs the pending jobs.
func (f *fuzzVM) jobs() bool {
	f.t.Helper()
	return f.call("ExecutePendingJobs", func() error {
		_, err := f.ExecutePendingJobs()
		return err
	})
}

// set sets the global name to the string s.
func (f *fuzzVM) set(name, s string) {
	f.t.Helper()
	g := f.GlobalObject()
	defer g.Free()
	a, err := f.NewAtom(name)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.SetProperty(g, a, s); err != nil {
		var e *Error
		if errors.As(err, &e) && e.OutOfMemory {
			f.t.Skip("no room for the input under the memory limit")
		}
		f.t.Fatal(err)
	}
}

// close closes the VM and checks it gave back all it held.
func (f *fuzzVM) close() {
	f.t.Helper()
	cvm := f.cvm
	if err := f.Close(); err != nil {
		f.t.Fatal(err)
	}
	n, err := cvm.Close()
	if err != nil {
		f.t.Fatal(err)
	}
	if n != 0 {
		f.t.Fatalf("the runtime was freed with %d blocks still in use", n)
	}
	if u := cvm.Host().Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		f.t.Fatalf("after Close the host still holds %+v", u)
	}
}

// fuzzing reports whether the test runs with -fuzz.
func fuzzing() bool {
	f := flag.Lookup("test.fuzz")
	return f != nil && f.Value.String() != ""
}

// fuzzRepoSeeds gives each script of the folders of the repository, and of
// the folders SLOWJS_FUZZ_SEEDS lists (as PATH does), to add, with whether it is a module,
// when the test runs with -fuzz. A plain go test runs only the seeds in
// testdata/fuzz.
func fuzzRepoSeeds(add func(src string, module bool)) {
	if !fuzzing() {
		return
	}
	dirs := []string{"engine/test/compare", "engine/test/regvm/corpus", "engine/test/regvm/js", "testdata", "compare/testdata"}
	if d := os.Getenv("SLOWJS_FUZZ_SEEDS"); d != "" {
		dirs = append(dirs, filepath.SplitList(d)...)
	}
	for _, d := range dirs {
		filepath.WalkDir(d, func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".js" && ext != ".mjs" {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil || len(b) > fuzzMaxInput {
				return nil
			}
			add(string(b), ext == ".mjs" || strings.Contains(string(b), "flags: [module]"))
			return nil
		})
	}
}

// fuzzNativeSeeds gives each input of the folders SLOWJS_JS_FUZZ_CORPUS
// lists (as PATH does), in the form of engine/test/js-fuzz.c, whose kind
// (the low two bits of its first byte) is kind, to add with the memory limit
// it picks, when the test runs with -fuzz. So the Go targets run on the
// generated engine what js-fuzz found with the coverage of the C.
func fuzzNativeSeeds(kind byte, add func(limit byte, rest []byte)) {
	d := os.Getenv("SLOWJS_JS_FUZZ_CORPUS")
	if !fuzzing() || d == "" {
		return
	}
	for _, d := range filepath.SplitList(d) {
		filepath.WalkDir(d, func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil || len(b) == 0 || len(b) > fuzzMaxInput || b[0]&3 != kind {
				return nil
			}
			add(b[0]>>2&3, b[1:])
			return nil
		})
	}
}

// FuzzEval compiles and runs a source as a script or a module, then runs its
// jobs, in one VM, and compiles it in a second VM and runs that bytecode with
// EvalBytecode. mode picks the module (bit 0), what the compiler strips (bits
// 1 and 2) and the memory limit (bits 3 and 4).
func FuzzEval(f *testing.F) {
	for _, s := range []string{
		`1 + 1`,
		`function f(a, ...b) { return a + b.length } f(1, 2, 3)`,
		`class A { #x = 1; static y = 2; get x() { return this.#x } } new A().x`,
		`async function* g() { yield* [1, 2] } (async () => { for await (const x of g()) ; })()`,
		`label: for (let i = 0; i < 3; i++) { switch (i) { case 1: continue label; default: break } }`,
		"`a${1}b${[2, 3]}c`",
		`var o = { a: 1, ...{ b: 2 }, [Symbol.iterator]: function* () {} }; [...o]`,
		`try { null.x } catch ({ message }) { message } finally { 0 }`,
		`(function () { 'use strict'; return typeof this })()`,
		`let { a = 1, b: [c, ...d] = [2, 3] } = {}; a + c + d.length`,
		`/(?<y>\d{4})-(?<m>\d\d)/v.exec('2026-11').groups.y`,
		`new Proxy({}, { get: (t, k) => k }).hello`,
		`Object.groupBy([1, 2, 3], x => x % 2)`,
		`10n ** 30n % 7n`,
		`using x = { [Symbol.dispose]() {} }`,
		`for (;;) {}`,
		`function r() { return r() } r()`,
		`'x'.repeat(1 << 28)`,
		`eval('1 +')`,
	} {
		f.Add(s, byte(0))
	}
	for _, s := range []string{
		`export const a = 1; export default function () {}`,
		`await Promise.resolve(1); export let x = await 2`,
		`import.meta.url`,
		`import x from 'nowhere'`,
		`export * from 'nowhere'`,
	} {
		f.Add(s, byte(1))
	}
	fuzzRepoSeeds(func(src string, module bool) {
		m := byte(0)
		if module {
			m = 1
		}
		f.Add(src, m)
	})
	for kind := range byte(2) {
		fuzzNativeSeeds(kind, func(limit byte, rest []byte) { f.Add(string(rest), kind|limit<<3) })
	}
	f.Fuzz(func(t *testing.T, src string, mode byte) {
		if len(src) > fuzzMaxInput {
			return
		}
		flags := EvalGlobal
		if mode&1 != 0 {
			flags = EvalModule
		}
		strip := Strip(mode>>1) & (StripSource | StripDebug)
		limit := int(mode >> 3)

		m := newFuzzVM(t, limit)
		m.SetStrip(strip)
		if m.eval("Eval", src, flags) {
			m.jobs()
		}
		m.close()

		m = newFuzzVM(t, limit)
		m.SetStrip(strip)
		var code []byte
		if m.call("Compile", func() (err error) {
			code, err = m.Compile(src, flags)
			return err
		}) && code != nil {
			ran := m.call("EvalBytecode", func() error {
				v, err := m.EvalBytecodeValue(code)
				if err == nil {
					v.Free()
				}
				return err
			})
			if ran {
				m.jobs()
			}
		}
		m.close()
	})
}
