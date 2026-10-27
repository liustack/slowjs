// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	"slices"
	"strings"
	"testing"
)

// asyncContextVM returns a VM, closed when the test ends, whose globals get
// and set are GetAsyncContext and SetAsyncContext, and run(value, fn), which
// calls fn with the async context value as AsyncLocalStorage.run does.
func asyncContextVM(t *testing.T) *VM {
	t.Helper()
	vm := openAsyncContextVM(t)
	t.Cleanup(func() { vm.Close() })
	return vm
}

// openAsyncContextVM is asyncContextVM, but the test closes the VM.
func openAsyncContextVM(t *testing.T) *VM {
	t.Helper()
	vm, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	define, err := vm.EvalValue(`(get, set) => {
  globalThis.get = get
  globalThis.set = set
  globalThis.run = (value, fn) => {
    const outer = get()
    set(value)
    try {
      return fn()
    } finally {
      set(outer)
    }
  }
}`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer define.Free()
	get, err := vm.NewBuiltinFunction(GetAsyncContext)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Free()
	set, err := vm.NewBuiltinFunction(SetAsyncContext)
	if err != nil {
		t.Fatal(err)
	}
	defer set.Free()
	r, err := define.CallValue(UndefinedValue, get, set)
	if err != nil {
		t.Fatal(err)
	}
	r.Free()
	return vm
}

// asyncContextLog runs script, then the jobs it queued, and returns the
// global log joined by spaces.
func asyncContextLog(t *testing.T, vm *VM, script string) string {
	t.Helper()
	if _, err := vm.Eval("globalThis.log = []\n"+script, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	got, err := vm.Eval("log.join(' ')", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	return got.(string)
}

// The cases give what Node 24 gives for the same code with an
// AsyncLocalStorage als, get for als.getStore, set for als.enterWith and run
// for als.run.
func TestAsyncContextFollowsPromiseJobs(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{
			"each of two overlapping async functions keeps its own context after await",
			`const f = async (tag) => { await null; log.push(tag + get().n); await new Promise((r) => Promise.resolve().then(r)); log.push(tag + get().n) }
run({ n: 1 }, () => f('a'))
run({ n: 2 }, () => f('b'))
log.push('top' + String(get()))`,
			"topundefined a1 b2 a1 b2",
		},
		{
			"a reaction runs in the context of the then that registered it, not of the resolve",
			`let resolve
const p = new Promise((r) => { resolve = r })
run({ n: 'then' }, () => p.then(() => log.push(get().n)))
run({ n: 'resolve' }, () => resolve())
set({ n: 'later' })
Promise.resolve().then(() => log.push(get().n))
set(undefined)`,
			"then later",
		},
		{
			"a reaction registered outside any context runs outside it",
			`const p = Promise.resolve()
p.then(() => log.push(String(get())))
set({ n: 'set after then' })
p.then(() => log.push(get().n))
set(undefined)`,
			"undefined set after then",
		},
		{
			"a handler that throws gives the context back, and the catch has the context it was registered in",
			`run({ n: 'inner' }, () => Promise.resolve().then(() => { throw new Error('x') })).catch(() => log.push(String(get())))
run({ n: 'run' }, () => Promise.reject(1).catch(() => log.push(get().n)))
Promise.resolve().then(() => Promise.resolve()).then(() => log.push(String(get())))`,
			"run undefined undefined",
		},
		{
			"a context set inside a job lasts until the job ends",
			`run({ n: 'job' }, () => Promise.resolve().then(() => { set({ n: 'inside' }); Promise.resolve().then(() => log.push(get().n)) }))
Promise.resolve().then(() => Promise.resolve().then(() => log.push(String(get()))))`,
			"inside undefined",
		},
		{
			"the then of a thenable runs in the context of the resolve",
			`const thenable = { then(r) { log.push('then' + String(get()?.n)); r(1) } }
run({ n: 'resolve' }, () => Promise.resolve(thenable).then(() => log.push('after' + String(get()?.n))))
run({ n: 'await' }, async () => { await thenable; log.push('await' + String(get()?.n)) })
log.push('top' + String(get()?.n))`,
			"topundefined thenresolve thenawait afterresolve awaitawait",
		},
		{
			"Promise.all, finally, async generators and for await carry the context",
			`async function* g() { yield 1; await null; yield get().n }
run({ n: 'all' }, () => Promise.all([Promise.resolve(), 1]).then(() => log.push(get().n)))
run({ n: 'finally' }, () => Promise.resolve().finally(() => log.push(get().n)))
run({ n: 'gen' }, async () => { for await (const v of g()) log.push(String(v)) })`,
			// The engines run these jobs in another order.
			"1 all finally gen",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			vm := asyncContextVM(t)
			got := strings.Fields(asyncContextLog(t, vm, c.script))
			want := strings.Fields(c.want)
			if strings.HasPrefix(c.name, "Promise.all") {
				slices.Sort(got)
			}
			if !slices.Equal(got, want) {
				t.Errorf("log = %q, want %q", got, want)
			}
		})
	}
}

// The context is an object or none: SetAsyncContext of any other value sets
// none. AsyncLocalStorage keeps its stores in an object.
func TestAsyncContextIsAnObject(t *testing.T) {
	vm := asyncContextVM(t)
	got := asyncContextLog(t, vm, `set(1)
log.push(String(get()))
set({ n: 'object' })
log.push(get().n)
set(null)
log.push(String(get()))`)
	if got != "undefined object undefined" {
		t.Errorf("log = %q", got)
	}
}

// A VM closes with a context set and with reactions that hold one still
// pending, and what they hold is freed: memgrind counts it.
func TestAsyncContextClose(t *testing.T) {
	vm := openAsyncContextVM(t)
	if _, err := vm.Eval(`globalThis.pending = new Promise(() => {})
run({ big: new Array(1e5).fill(1) }, () => pending.then(() => {}))
set({ n: 'left' })`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := vm.Close(); err != nil {
		t.Fatal(err)
	}
}

// A context that no reaction and no slot holds any longer is freed.
func TestAsyncContextIsFreed(t *testing.T) {
	vm := asyncContextVM(t)
	vm.RunGC()
	before := vm.MemorySize()
	if _, err := vm.Eval(`run({ big: new Array(1e6).fill(1) }, () => Promise.resolve().then(() => {}))`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	vm.RunGC()
	if after := vm.MemorySize(); after > before+1<<20 {
		t.Errorf("memory %d after the jobs ran, %d before: the context is kept", after, before)
	}
}
