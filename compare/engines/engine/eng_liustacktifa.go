// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustacktifa

package engine

// liustack-tifa is slowjs set up the way tifa sets up a VM of its pool
// (internal/ssr/renderer.go and pool.go of github.com/liustack/tifa, MIT),
// for the tifa page runs only. The small pieces of Go it needs are copied
// here, not imported, and when tifa changes them they are copied again.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/intl"
	"github.com/liustack/slowjs/temporal"
)

// The defaults of tifa's ssr.vm_memory, ssr.vm_stack_depth and ssr.timeout
// (internal/config/config.go).
const (
	tifaMemory  = uintptr(128 << 20)
	tifaStack   = uintptr(10000)
	tifaTimeout = 3 * time.Second
)

func init() {
	register(&Engine{
		Name:           tifaName,
		Module:         "github.com/liustack/slowjs",
		New:            newTifa,
		SharedPrograms: true,
		CompileShared:  compileTifa,
		Interrupt:      true,
		MemoryLimit:    true,
		Notes: []string{
			"slowjs set up as tifa sets up a VM of its pool: its own Intl and Temporal, no polyfill, __tifa_murmur2__, __tifa_random_bytes__ and __tifa_async_context__ as Go functions, a memory limit of 128 MiB, a stack of 10000 slots and a 3 s time budget set before each step of a render.",
			"The bundle is compiled once into a SharedBytecode that every VM loads, as an ES module when its file name ends in .mjs, as tifa compiles the bundles it builds since they are ES modules.",
			"A bundle with @emotion/hash gets the one-line change the tifa plugin makes to murmur2 (see hashInGo), so that it asks __tifa_murmur2__ first.",
			"A render is startRender, the promise jobs and the read of the result, as ssr.RenderVM does them. After each render comes a young GC and, by the pool's rules, a full GC (Settle), counted in the CPU time per render but not in the wall time.",
		},
	})
}

type tifaVM struct {
	vm *slowjs.VM
	// The state of tifa's pooledVM (internal/ssr/pool.go).
	live, baseline, need, threshold uintptr
	young                           int
	renderGCs, youngGCs, fullGCs    int
}

// needDecay and fullGCEvery are the constants of the pool.
const (
	needDecay   = 1024
	fullGCEvery = 256
)

// gcThreshold is the GC threshold of a VM for its next render (see the pool).
func gcThreshold(live, need, memory uintptr) uintptr {
	return min(max(2*live, need), live/2+memory/2)
}

var (
	tifaCompileVM *slowjs.VM
	tifaPrograms  = map[string]*slowjs.SharedBytecode{}
	// bundleRewrites counts the functions hashInGo changed.
	bundleRewrites int
)

// compileTifa turns src into bytecode once in a throwaway VM, as ssr.Compile
// does, and returns the same SharedBytecode for the same text compiled the
// same way: as an ES module when name ends in .mjs, else as a script.
func compileTifa(name, src string) (Program, error) {
	flags, key := slowjs.EvalGlobal, "script:"+src
	if strings.HasSuffix(name, ".mjs") {
		flags, key = slowjs.EvalModule, "module:"+src
	}
	if p, ok := tifaPrograms[key]; ok {
		return p, nil
	}
	if tifaCompileVM == nil {
		vm, err := slowjs.NewVM()
		if err != nil {
			return nil, err
		}
		vm.SetMemoryLimit(tifaMemory)
		vm.SetEvalTimeout(tifaTimeout)
		tifaCompileVM = vm
	}
	text, n := hashInGo(src)
	bundleRewrites += n
	code, err := tifaCompileVM.Compile(text, flags)
	if err != nil {
		return nil, qjsErr(err, true)
	}
	p, err := slowjs.NewSharedBytecode(code)
	if err != nil {
		return nil, err
	}
	tifaPrograms[key] = p
	return p, nil
}

// newTifa makes a VM as ssr.Load does, up to the bundle.
func newTifa(o Options) (VM, error) {
	vm, err := slowjs.NewVM()
	if err != nil {
		return nil, err
	}
	vm.SetMemoryLimit(tifaMemory)
	vm.SetMaxStackSize(tifaStack)
	if err = vm.SetEvalTimeout(tifaTimeout); err == nil {
		err = intl.Register(vm)
	}
	if err == nil {
		err = temporal.Register(vm)
	}
	if err == nil {
		err = registerRandom(vm)
	}
	if err == nil {
		err = registerHash(vm)
	}
	if err == nil {
		err = registerAsyncContext(vm)
	}
	if err != nil {
		return nil, errors.Join(err, vm.Close())
	}
	return &tifaVM{vm: vm}, nil
}

func (v *tifaVM) Eval(name, src string) (string, error) {
	r, err := v.vm.Eval(src, slowjs.EvalGlobal)
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *tifaVM) Compile(name, src string) (Program, error) { return compileTifa(name, src) }

func (v *tifaVM) Run(p Program) (string, error) {
	r, err := v.vm.EvalSharedBytecode(p.(*slowjs.SharedBytecode))
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *tifaVM) Call(fn string) (string, error) {
	r, err := v.vm.Call(fn)
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *tifaVM) RunJobs() error {
	if _, err := v.vm.ExecutePendingJobs(); err != nil {
		return qjsErr(err, false)
	}
	return nil
}

func (v *tifaVM) Interrupt() { v.vm.Interrupt() }
func (v *tifaVM) Close()     { v.vm.Close() }

// Loaded is what ssr.Load does after the bundle: what loading left as
// garbage goes now, and the first render gets the threshold of the next ones.
func (v *tifaVM) Loaded() error {
	v.vm.RunGC()
	v.live = v.vm.MemorySize()
	v.baseline = v.live
	v.threshold = gcThreshold(v.live, 0, tifaMemory)
	v.vm.SetGCThreshold(v.threshold)
	return nil
}

// Render is ssr.RenderVM: startRender(input), then the microtasks it queued,
// then the result, each step with the time left of the budget.
func (v *tifaVM) Render(input string) (string, error) {
	vm := v.vm
	deadline := time.Now().Add(tifaTimeout)
	budget := func() error {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("ssr: render timed out after %s", tifaTimeout)
		}
		return vm.SetEvalTimeout(remaining)
	}
	vm.SetClock(nil)
	defer vm.SetClock(nil)
	if err := budget(); err != nil {
		return "", err
	}
	if _, err := vm.Eval("globalThis.renderResult = undefined", slowjs.EvalGlobal); err != nil {
		return "", qjsErr(err, false)
	}
	if err := budget(); err != nil {
		return "", err
	}
	if _, err := vm.Call("startRender", input); err != nil {
		return "", qjsErr(err, false)
	}
	if err := budget(); err != nil {
		return "", err
	}
	if _, err := vm.ExecutePendingJobs(); err != nil {
		return "", qjsErr(err, false)
	}
	if err := budget(); err != nil {
		return "", err
	}
	value, err := vm.Eval("(() => { const result = globalThis.renderResult; globalThis.renderResult = undefined; return result })()", slowjs.EvalGlobal)
	if err != nil {
		return "", qjsErr(err, false)
	}
	text, ok := value.(string)
	if !ok {
		return "", errors.New("ssr: render left unresolved work after draining microtasks")
	}
	if !strings.HasPrefix(text, "<") {
		return "", fmt.Errorf("ssr: render returned %.80q, not HTML", text)
	}
	return text, nil
}

// Settle is what the pool does with a VM after a render it survived
// (Pool.release and Pool.collect), run here before the next render.
func (v *tifaVM) Settle() error {
	engineGC := v.vm.GCThreshold() != v.threshold
	if engineGC {
		v.need = min(max(v.need, v.vm.GCThreshold()), 4*v.live)
		v.renderGCs++
	} else if v.need > 2*v.live {
		v.need -= (v.need - 2*v.live) / needDecay
	}
	v.setThreshold()
	v.young++
	if !engineGC && v.young < fullGCEvery {
		v.vm.RunYoungGC()
		if v.vm.MemorySize() < v.live+v.live/2 {
			v.youngGCs++
			return nil
		}
	}
	v.vm.RunGC()
	v.young = 0
	v.live = v.vm.MemorySize()
	v.setThreshold()
	v.fullGCs++
	return nil
}

func (v *tifaVM) setThreshold() {
	v.threshold = gcThreshold(v.live, v.need, tifaMemory)
	v.vm.SetGCThreshold(v.threshold)
}

// Counters say how the pool's rules ran and how often the bundle used the Go
// functions.
func (v *tifaVM) Counters() map[string]int {
	return map[string]int{
		"render_gcs": v.renderGCs, "young_gcs": v.youngGCs, "full_gcs": v.fullGCs,
		"live_bytes": int(v.live), "baseline_bytes": int(v.baseline),
		"murmur2_calls": hookCalls.murmur2, "random_bytes_calls": hookCalls.randomBytes,
		"bundle_rewrites": bundleRewrites,
	}
}

func qjsErr(err error, compile bool) error {
	var e *slowjs.Error
	if errors.As(err, &e) {
		return &JSError{Name: e.Name, Message: e.Message, Compile: compile}
	}
	return Classify(err, compile)
}
