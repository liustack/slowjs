// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// The pages, and a VM kept as tifa's pool keeps one: Compile, Load, RenderVM
// and the collections between renders of internal/ssr of tifa, as
// internal/measure/pages copies them, with the bundle in the script form of
// engine/pgo/pages.

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/tifa"
)

const (
	// vmMemory is tifa's ssr.vm_memory, the memory limit of a VM.
	vmMemory = 128 << 20
	// stackDepth is tifa's ssr.vm_stack_depth.
	stackDepth = 10000
	// startTimeout is how long tifa lets a compile or a load take.
	startTimeout = time.Minute
	// fullGCEvery is how many renders tifa's pool lets go by with young
	// collections only.
	fullGCEvery = 256
)

// fixedNow is the clock of every VM, so that a page dated by the clock
// renders the same HTML for the whole run.
var fixedNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// kindFiles are the kinds of pages: the bundle and its inputs, from the root
// of the repository.
var kindFiles = map[string]struct{ bundle, inputs string }{
	"store": {"engine/pgo/pages/shop/shop.js", "engine/pgo/pages/shop/in/store-*.json"},
	"wb":    {"engine/pgo/pages/shop/shop.js", "engine/pgo/pages/shop/in/wb-*.json"},
	"mui":   {"engine/pgo/pages/mui/mui.js", "engine/pgo/pages/mui/in/*.json"},
}

// page is one page, with what its reference renders gave.
type page struct {
	name  string
	input string
	hash  [32]byte      // the SHA-256 of its HTML
	took  time.Duration // the shortest of its reference renders
	grows uintptr       // what a render took from the engine with the collector off
	push  int           // the calls of Array.prototype.push a render makes
}

// bundle is a bundle compiled once, as tifa compiles it, with its pages.
type bundle struct {
	name  string
	code  *slowjs.SharedBytecode
	pages []*page
}

// openBundles compiles the bundles of kinds in the repository repo and reads
// the inputs of their pages.
func openBundles(repo string, kinds []string) (bs []*bundle, err error) {
	defer func() {
		if err != nil {
			closeBundles(bs)
			bs = nil
		}
	}()
	byFile := map[string]*bundle{}
	for _, k := range kinds {
		f, ok := kindFiles[k]
		if !ok {
			return bs, fmt.Errorf("stress: no kind %q (store, wb or mui)", k)
		}
		b := byFile[f.bundle]
		if b == nil {
			src, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(f.bundle)))
			if err != nil {
				return bs, err
			}
			code, err := compile(string(src))
			if err != nil {
				return bs, fmt.Errorf("%s: %w", f.bundle, err)
			}
			b = &bundle{name: strings.TrimSuffix(filepath.Base(f.bundle), ".js"), code: code}
			byFile[f.bundle] = b
			bs = append(bs, b)
		}
		inputs, err := filepath.Glob(filepath.Join(repo, filepath.FromSlash(f.inputs)))
		if err != nil {
			return bs, err
		}
		if len(inputs) == 0 {
			return bs, fmt.Errorf("stress: no input %s in %s", f.inputs, repo)
		}
		slices.Sort(inputs)
		for _, in := range inputs {
			data, err := os.ReadFile(in)
			if err != nil {
				return bs, err
			}
			b.pages = append(b.pages, &page{name: strings.TrimSuffix(filepath.Base(in), ".json"), input: string(data)})
		}
	}
	return bs, nil
}

func closeBundles(bs []*bundle) error {
	var errs []error
	for _, b := range bs {
		errs = append(errs, b.code.Close())
	}
	return errors.Join(errs...)
}

// compile is Compile of tifa's renderer.go, for the script form of a bundle.
func compile(src string) (*slowjs.SharedBytecode, error) {
	vm, err := slowjs.NewVM()
	if err != nil {
		return nil, err
	}
	defer vm.Close()
	vm.SetMemoryLimit(vmMemory)
	if err := vm.SetEvalTimeout(startTimeout); err != nil {
		return nil, err
	}
	code, err := vm.Compile(src, slowjs.EvalGlobal)
	if err != nil {
		return nil, err
	}
	return slowjs.NewSharedBytecode(code)
}

// pvm is a VM of a pool, with the state tifa's pool keeps for it.
type pvm struct {
	*slowjs.VM
	b  *bundle
	id int
	// the functions a render calls, taken once as tifa's Load takes them
	global, reset, start, read slowjs.Value
	// live is the memory after the last full collection, need what renders
	// showed they need, threshold the one the pool set last, young the
	// renders since the last full collection (pooledVM of tifa)
	live, need, threshold uintptr
	young                 int
	renders               int
	// stops counts the calls of its renders that a stop ended, after which
	// it went on: an Interrupt, the eval timeout or the memory limit.
	stops int
	// steady says it is never killed and goes on after every stop, so that
	// it lives the whole run.
	steady bool
	// pushes says Array.prototype.push is wrapped for a kill (killJS).
	pushes bool
	killed bool
	closed bool
}

// load is Load of tifa's renderer.go: a VM with the memory limit, the call
// depth, tifa.Register and the bundle, collected, its threshold set and its
// free memory given back.
func load(b *bundle, id int, timeout time.Duration) (v *pvm, err error) {
	vm, err := slowjs.NewVM()
	if err != nil {
		return nil, err
	}
	v = &pvm{VM: vm, b: b, id: id}
	defer func() {
		if err != nil {
			err = errors.Join(err, v.close())
			v = nil
		}
	}()
	vm.SetMemoryLimit(vmMemory)
	vm.SetMaxStackSize(stackDepth)
	if err := vm.SetEvalTimeout(startTimeout); err != nil {
		return v, err
	}
	if err := tifa.Register(vm); err != nil {
		return v, err
	}
	vm.SetClock(func() time.Time { return fixedNow })
	if _, err := vm.EvalSharedBytecode(b.code); err != nil {
		return v, err
	}
	for _, t := range []struct {
		v   *slowjs.Value
		src string
	}{{&v.global, "globalThis"}, {&v.reset, resetResult}, {&v.start, "globalThis.startRender"}, {&v.read, readResult}} {
		if *t.v, err = vm.EvalValue(t.src, slowjs.EvalGlobal); err != nil {
			return v, err
		}
	}
	vm.RunGC()
	if err := v.finalize(timeout); err != nil {
		return v, err
	}
	v.live = vm.MemorySize()
	v.setThreshold()
	if _, err := vm.ReleaseMemory(0); err != nil {
		return v, err
	}
	return v, nil
}

// close frees the functions it took and closes the VM. After a kill Free does
// nothing, and Close is what frees them.
func (v *pvm) close() error {
	v.closed = true
	for _, x := range []*slowjs.Value{&v.global, &v.reset, &v.start, &v.read} {
		x.Free()
	}
	return v.Close()
}

// resetResult and readResult are those of tifa's renderer.go.
const (
	resetResult = `() => { globalThis.renderResult = undefined }`
	readResult  = `() => {
  const result = globalThis.renderResult
  globalThis.renderResult = undefined
  if (result !== undefined || typeof globalThis.stopRender !== 'function') return result
  globalThis.stopRender()
  return false
}`
)

// The errors of a render that do not come from the engine.
var (
	// errBudget is a render whose time ran out between two of its calls.
	errBudget = errors.New("stress: the render ran out of time between two calls")
	// errUnresolved is a render that left no result once no job was left.
	errUnresolved = errors.New("stress: the render left unresolved work after draining microtasks")
)

// resultError is a render whose result is not HTML: the JSON of an error or a
// redirect of the page.
type resultError struct{ text string }

func (e *resultError) Error() string {
	return fmt.Sprintf("stress: the render gave no HTML: %.200q", e.text)
}

// render is RenderVM of tifa's renderer.go: each call gets what is left of
// the timeout, and the microtasks run once.
func (v *pvm) render(input string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	budget := func() error {
		left := time.Until(deadline)
		if left <= 0 {
			return errBudget
		}
		return v.SetEvalTimeout(left)
	}
	steps := []func() (any, error){
		func() (any, error) { return v.reset.Call(slowjs.UndefinedValue) },
		func() (any, error) { return v.start.Call(v.global, input) },
		func() (any, error) { return v.ExecutePendingJobs() },
		func() (any, error) { return v.read.Call(slowjs.UndefinedValue) },
	}
	var value any
	for _, step := range steps {
		if err := budget(); err != nil {
			return "", err
		}
		r, err := step()
		if err != nil {
			return "", err
		}
		value = r
	}
	if stopped, ok := value.(bool); ok && !stopped {
		for _, step := range steps[2:] {
			if err := budget(); err != nil {
				return "", err
			}
			r, err := step()
			if err != nil {
				return "", err
			}
			value = r
		}
	}
	s, ok := value.(string)
	if !ok {
		return "", errUnresolved
	}
	if !strings.HasPrefix(s, "<") {
		return "", &resultError{s}
	}
	return s, nil
}

// gcThreshold is that of tifa's pool.
func gcThreshold(live, need, memory uintptr) uintptr {
	return min(max(2*live, need), live/2+memory/2)
}

func (v *pvm) setThreshold() {
	v.threshold = gcThreshold(v.live, v.need, vmMemory)
	v.SetGCThreshold(v.threshold)
}

// collect is release and runGC of tifa's pool after a render: a young
// collection, or a full one when the engine collected in the render, when
// the young one left 1.5 times the live size or more, or every fullGCEvery
// renders.
func (v *pvm) collect() error {
	engineGC := v.GCThreshold() != v.threshold
	if engineGC {
		v.need = min(max(v.need, v.GCThreshold()), 4*v.live)
	} else if v.need > 2*v.live {
		v.need -= (v.need - 2*v.live) / 1024
	}
	v.setThreshold()
	v.young++
	if !engineGC && v.young < fullGCEvery {
		v.RunYoungGC()
		if v.MemorySize() < v.live+v.live/2 {
			return nil
		}
	}
	return v.full()
}

// full is the full collection of tifa's pool: the live size and threshold
// from what is left, and the free memory given back.
func (v *pvm) full() error {
	v.RunGC()
	v.young = 0
	v.live = v.MemorySize()
	v.setThreshold()
	_, err := v.ReleaseMemory(0)
	return err
}

// finalize is runFinalizers of tifa's renderer.go: the jobs a collection
// outside a render queued, under the whole timeout.
func (v *pvm) finalize(timeout time.Duration) error {
	if err := v.SetEvalTimeout(timeout); err != nil {
		return err
	}
	_, err := v.ExecutePendingJobs()
	return err
}

// countPushJS wraps Array.prototype.push to count its calls in
// globalThis.__stress_pushes.
const countPushJS = `(() => {
  const push = Array.prototype.push
  globalThis.__stress_pushes = 0
  Array.prototype.push = function (...args) { globalThis.__stress_pushes++; return push.apply(this, args) }
})()`

// killJS wraps Array.prototype.push so that its call number %d runs
// lastIndexOf over an array-like object of 2^31-1 elements, a loop of the
// engine that never reads the interrupt flag and takes minutes: once the
// eval timeout or an Interrupt asks the render to stop, a poll kills it,
// deep inside React's stack.
const killJS = `(() => {
  const push = Array.prototype.push, lastIndexOf = Array.prototype.lastIndexOf, a = { length: 2 ** 31 - 1 }
  let n = %d
  Array.prototype.push = function (...args) {
    if (--n === 0) lastIndexOf.call(a, -1)
    return push.apply(this, args)
  }
})()`

// reference renders every page of b on a VM of its own, which no stop
// touches: its HTML three times, which must be the same, the shortest time,
// what a render takes from the engine with the collector off, and, with
// Array.prototype.push counted, the calls of push and the same HTML again.
func reference(b *bundle, timeout time.Duration) error {
	v, err := load(b, -1, timeout)
	if err != nil {
		return err
	}
	defer v.close()
	for _, p := range b.pages {
		for i := range 3 {
			t := time.Now()
			html, err := v.render(p.input, timeout)
			d := time.Since(t)
			if err != nil {
				return fmt.Errorf("%s: reference render: %w", p.name, err)
			}
			h := sha256.Sum256([]byte(html))
			switch {
			case i == 0:
				p.hash, p.took = h, d
			case h != p.hash:
				return fmt.Errorf("%s: the reference renders gave other HTML", p.name)
			default:
				p.took = min(p.took, d)
			}
			if err := v.collect(); err != nil {
				return err
			}
			if err := v.finalize(timeout); err != nil {
				return err
			}
		}
		// what a render takes with no collection in it
		if err := v.full(); err != nil {
			return err
		}
		before := v.MemorySize()
		v.SetGCThreshold(^uintptr(0))
		if _, err := v.render(p.input, timeout); err != nil {
			return fmt.Errorf("%s: render with the collector off: %w", p.name, err)
		}
		after := v.MemorySize()
		if after < before {
			return fmt.Errorf("%s: a render with the collector off left %d bytes, less than the %d before it", p.name, after, before)
		}
		p.grows = after - before
		if err := v.full(); err != nil {
			return err
		}
	}
	if _, err := v.Eval(countPushJS, slowjs.EvalGlobal); err != nil {
		return err
	}
	for _, p := range b.pages {
		if _, err := v.Eval(`globalThis.__stress_pushes = 0`, slowjs.EvalGlobal); err != nil {
			return err
		}
		html, err := v.render(p.input, timeout)
		if err != nil {
			return fmt.Errorf("%s: render with push counted: %w", p.name, err)
		}
		if sha256.Sum256([]byte(html)) != p.hash {
			return fmt.Errorf("%s: a render with push wrapped gave other HTML", p.name)
		}
		n, err := v.Eval(`globalThis.__stress_pushes`, slowjs.EvalGlobal)
		if err != nil {
			return err
		}
		c, ok := n.(int)
		if !ok {
			return fmt.Errorf("%s: the count of push is %T %v", p.name, n, n)
		}
		p.push = c
		if err := v.collect(); err != nil {
			return err
		}
	}
	return nil
}
