// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/liustack/slowjs"
)

// Options say what to load and how.
type Options struct {
	Setup    Setup
	Limit    uintptr       // the memory limit, 0 for none
	Timeout  time.Duration // the eval timeout of the tifa mode
	Preludes []string      // scripts evaluated before the bundle, after the VM is set up
	Bundle   string
	React    bool // the bundle defines render(), which returns the HTML, instead of startRender
	NoPool   bool // no collection between renders, only those the engine starts itself
	Log      io.Writer
	// BeforeBundle, when not nil, runs after the preludes and the guard, right before the bundle.
	BeforeBundle func(*slowjs.VM) error
}

// Runner is a VM with the bundle loaded, which renders pages.
type Runner struct {
	o        Options
	vm       *slowjs.VM
	bytecode *slowjs.SharedBytecode
	// the functions a render of the tifa mode calls, taken once as tifa's Load takes them
	global, reset, start, read slowjs.Value
	pool                       pool
}

// Open makes the VM of o.Setup, runs the preludes, checks the hooks the bundle asks for (Guard),
// loads the bundle and runs the collection that follows a load.
func Open(o Options) (_ *Runner, err error) {
	if o.Log == nil {
		o.Log = io.Discard
	}
	r := &Runner{o: o}
	// r, not the result: a return of nil and the error sets the result to nil first.
	defer func() {
		if err != nil {
			r.Close()
		}
	}()
	src, err := os.ReadFile(o.Bundle)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(o.Log, "pages: %s\n", o.Setup.Describe(o.Limit, o.Timeout))
	if h := o.Setup.Registered(); len(h) > 0 {
		fmt.Fprintf(o.Log, "pages: registered %s\n", strings.Join(h, " "))
	} else {
		fmt.Fprintf(o.Log, "pages: registered no tifa hook\n")
	}
	if o.Setup.Mode == Tifa {
		// tifa compiles the bundle once, in a VM of its own, and every VM loads the bytecode.
		if r.bytecode, err = compile(string(src), o.Limit, o.Timeout); err != nil {
			return nil, fmt.Errorf("%s: %w", o.Bundle, err)
		}
	}
	if r.vm, err = o.Setup.NewVM(o.Limit, o.Timeout); err != nil {
		return nil, err
	}
	for _, p := range o.Preludes {
		code, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(o.Log, "pages: prelude %s sha256 %s\n", p, sum(code))
		if _, err := r.vm.Eval(string(code), slowjs.EvalGlobal); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	fmt.Fprintf(o.Log, "pages: bundle %s sha256 %s\n", o.Bundle, sum(src))
	asked := Asked(string(src))
	have, err := Have(r.vm, asked)
	if err != nil {
		return nil, err
	}
	lines, err := Guard(o.Setup, asked, have)
	if err != nil {
		return nil, err
	}
	if len(asked) == 0 {
		fmt.Fprintf(o.Log, "pages: the bundle asks for no name __tifa_*\n")
	}
	for _, l := range lines {
		fmt.Fprintf(o.Log, "pages: hook %s\n", l)
	}
	if o.BeforeBundle != nil {
		if err := o.BeforeBundle(r.vm); err != nil {
			return nil, err
		}
	}
	if r.bytecode != nil {
		_, err = r.vm.EvalSharedBytecode(r.bytecode)
	} else {
		_, err = r.vm.Eval(string(src), slowjs.EvalGlobal)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.Bundle, err)
	}
	if err := r.jobs(); err != nil {
		return nil, err
	}
	if o.Setup.Mode == Tifa {
		if err := r.take(); err != nil {
			return nil, err
		}
	}
	memory := o.Limit
	if memory == 0 {
		memory = defaultMemory
	}
	r.pool = pool{vm: r.vm, memory: memory, give: o.Setup.Mode == Tifa}
	switch {
	case o.NoPool || o.Setup.Mode == StandIn && o.React:
		// The engine's collections only: the stand-in mode ran React's page so, with a young
		// collection after each render (After).
		r.vm.RunGC()
	default:
		// what tifa's Load does once the bundle ran: a full collection, the threshold, and the
		// memory given back (the stand-in mode gives none back)
		if err := r.pool.full(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func sum(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))[:16]
}

// compile is Compile of tifa's renderer.go, for the script form of a bundle.
func compile(src string, limit uintptr, timeout time.Duration) (*slowjs.SharedBytecode, error) {
	vm, err := slowjs.NewVM()
	if err != nil {
		return nil, err
	}
	defer vm.Close()
	if limit > 0 {
		vm.SetMemoryLimit(limit)
	}
	if err := vm.SetEvalTimeout(timeout); err != nil {
		return nil, err
	}
	code, err := vm.Compile(src, slowjs.EvalGlobal)
	if err != nil {
		return nil, err
	}
	return slowjs.NewSharedBytecode(code)
}

// jobs runs the promise jobs until none is left.
func (r *Runner) jobs() error {
	for {
		n, err := r.vm.ExecutePendingJobs()
		if err != nil || n == 0 {
			return err
		}
	}
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

// take takes the functions a render calls from the global object once, as tifa's Load does.
func (r *Runner) take() error {
	start := "globalThis.startRender"
	if r.o.React {
		start = "globalThis.render"
	}
	for _, t := range []struct {
		v   *slowjs.Value
		src string
	}{{&r.global, "globalThis"}, {&r.reset, resetResult}, {&r.start, start}, {&r.read, readResult}} {
		v, err := r.vm.EvalValue(t.src, slowjs.EvalGlobal)
		if err != nil {
			return err
		}
		*t.v = v
	}
	return nil
}

// VM is the VM of r.
func (r *Runner) VM() *slowjs.VM { return r.vm }

// Render renders one page and returns its HTML and the time the render took, which does not
// count the collections of After. input is the JSON of the page, unused for React's page.
func (r *Runner) Render(input string) (html string, d time.Duration, err error) {
	if r.o.Setup.Mode == Tifa {
		t := time.Now()
		html, err = r.renderVM(input)
		d = time.Since(t)
	} else {
		html, d, err = r.renderStandIn(input)
	}
	if err == nil && !strings.HasPrefix(html, "<") {
		err = fmt.Errorf("pages: the render gave no HTML: %.200q", html)
	}
	return html, d, err
}

// renderVM is RenderVM of tifa's renderer.go: each step gets what is left of the timeout, and the
// microtasks run once.
func (r *Runner) renderVM(input string) (string, error) {
	deadline := time.Now().Add(r.o.Timeout)
	budget := func() error {
		left := time.Until(deadline)
		if left <= 0 {
			return fmt.Errorf("pages: the render took more than -timeout %v (any timeout above 0 does the same work, so raise it under callgrind)", r.o.Timeout)
		}
		return r.vm.SetEvalTimeout(left)
	}
	if r.o.React {
		if err := budget(); err != nil {
			return "", err
		}
		v, err := r.start.Call(r.global)
		if err != nil {
			return "", err
		}
		// The promise jobs the render queued run before the next one, as tifa runs them:
		// TanStack Table queues one that holds the whole table.
		if err := budget(); err != nil {
			return "", err
		}
		if _, err := r.vm.ExecutePendingJobs(); err != nil {
			return "", err
		}
		s, _ := v.(string)
		return s, nil
	}
	steps := []func() (any, error){
		func() (any, error) { return r.reset.Call(slowjs.UndefinedValue) },
		func() (any, error) { return r.start.Call(r.global, input) },
		func() (any, error) { return r.vm.ExecutePendingJobs() },
		func() (any, error) { return r.read.Call(slowjs.UndefinedValue) },
	}
	var value any
	for _, step := range steps {
		if err := budget(); err != nil {
			return "", err
		}
		v, err := step()
		if err != nil {
			return "", err
		}
		value = v
	}
	if stopped, ok := value.(bool); ok && !stopped {
		for _, step := range steps[2:] {
			if err := budget(); err != nil {
				return "", err
			}
			v, err := step()
			if err != nil {
				return "", err
			}
			value = v
		}
	}
	s, ok := value.(string)
	if !ok {
		return "", errors.New("pages: the render left unresolved work after draining microtasks")
	}
	return s, nil
}

// renderStandIn is the render of the program the stand-in mode reproduces: startRender called by
// name and the jobs run until none is left, timed, then the result read with an eval, outside the
// time. React's page is one call of render.
func (r *Runner) renderStandIn(input string) (string, time.Duration, error) {
	if r.o.React {
		t := time.Now()
		v, err := r.vm.Call("render")
		d := time.Since(t)
		if err != nil {
			return "", d, err
		}
		s, _ := v.(string)
		return s, d, nil
	}
	t := time.Now()
	if _, err := r.vm.Call("startRender", input); err != nil {
		return "", 0, err
	}
	if err := r.jobs(); err != nil {
		return "", 0, err
	}
	d := time.Since(t)
	v, err := r.vm.Eval("(function () { var s = globalThis.renderResult; globalThis.renderResult = undefined; return s; })()", 0)
	if err != nil {
		return "", d, err
	}
	s, ok := v.(string)
	if !ok {
		return "", d, fmt.Errorf("pages: renderResult is %T", v)
	}
	return s, d, nil
}

// After runs what follows a render, outside its time: tifa's collections between renders, none
// with NoPool, and a young collection for React's page in the stand-in mode.
func (r *Runner) After() error {
	switch {
	case r.o.NoPool:
		return nil
	case r.o.Setup.Mode == StandIn && r.o.React:
		r.vm.RunYoungGC()
		return nil
	}
	return r.pool.between()
}

// Close frees the functions it took and closes the VM and the bytecode.
func (r *Runner) Close() error {
	var err error
	if r.vm != nil {
		for _, v := range []*slowjs.Value{&r.global, &r.reset, &r.start, &r.read} {
			v.Free()
		}
		err = r.vm.Close()
	}
	if r.bytecode != nil {
		err = errors.Join(err, r.bytecode.Close())
	}
	return err
}

// pool keeps a VM as tifa's pool does (pooledVM, release, runGC and gcThreshold of
// internal/ssr/pool.go of tifa): a young collection after each render and a full one now and
// then. give is whether a full collection gives the memory the VM holds free back, as tifa's
// does and the stand-in mode's does not.
type pool struct {
	vm                            *slowjs.VM
	memory, live, need, threshold uintptr
	young                         int
	give                          bool
}

// gcThreshold is that of tifa.
func gcThreshold(live, need, memory uintptr) uintptr {
	return min(max(2*live, need), live/2+memory/2)
}

func (p *pool) setThreshold() {
	p.threshold = gcThreshold(p.live, p.need, p.memory)
	p.vm.SetGCThreshold(p.threshold)
}

func (p *pool) full() error {
	p.vm.RunGC()
	p.young = 0
	p.live = p.vm.MemorySize()
	p.setThreshold()
	if p.give {
		if _, err := p.vm.ReleaseMemory(0); err != nil {
			return err
		}
	}
	return nil
}

// between is release and collect of tifa's pool, after a render.
func (p *pool) between() error {
	engineGC := p.vm.GCThreshold() != p.threshold
	if engineGC {
		p.need = min(max(p.need, p.vm.GCThreshold()), 4*p.live)
	} else if p.need > 2*p.live {
		p.need -= (p.need - 2*p.live) / 1024
	}
	p.setThreshold()
	p.young++
	if !engineGC && p.young < 256 {
		p.vm.RunYoungGC()
		if p.vm.MemorySize() < p.live+p.live/2 {
			return nil
		}
	}
	return p.full()
}
