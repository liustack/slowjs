// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/gohost"
)

// op is what a step does to a render.
type op int

const (
	opNormal    op = iota // a render as tifa makes it
	opOOM                 // under a memory limit drawn at random, a little above what the VM holds
	opTimeout             // under an eval timeout drawn at random, shorter than the render
	opInterrupt           // Interrupt from another goroutine at a moment drawn at random
	opKill                // a stop the engine does not see in time, deep in a render: a kill
	opAbort               // the engine aborts at a poll drawn at random (-tags testhooks)
	nOps
)

var opNames = [nOps]string{"normal", "oom", "timeout", "interrupt", "kill", "abort"}

// outcome is how a render ended.
type outcome int

const (
	outHTML        outcome = iota // the HTML of the page
	outInterrupted                // *Error with Interrupted, not a kill
	outOOM                        // *Error with OutOfMemory
	outBudget                     // the time ran out between two calls of the render
	outKilled                     // *Error that matches ErrKilled
	outAborted                    // the panic of an abort of the engine
	outThrown                     // an exception the page threw: catchable, not uncatchable
	outUnresolved                 // no result once no job was left
	outResult                     // a result that is not HTML
	outWrongHTML                  // HTML other than the page's
	outOther                      // any other error
	nOutcomes
)

var outcomeNames = [nOutcomes]string{"html", "interrupted", "oom", "budget", "killed", "aborted", "thrown", "unresolved", "result", "WRONG-HTML", "OTHER"}

// counts are the renders of the run, by op, by outcome, and by whether the
// VM went on after a stop before.
type counts struct {
	n [2][nOps][nOutcomes]atomic.Int64 // [reused][op][outcome]
}

func (c *counts) add(reused bool, o op, out outcome) {
	r := 0
	if reused {
		r = 1
	}
	c.n[r][o][out].Add(1)
}

func (c *counts) total() int64 {
	var t int64
	for r := range c.n {
		for o := range c.n[r] {
			for x := range c.n[r][o] {
				t += c.n[r][o][x].Load()
			}
		}
	}
	return t
}

// allowed says whether a render that op o made may end with out: on a VM
// that never stopped, a normal render gives its HTML and a render under a
// stop gives its HTML or the error of that stop. On a VM that went on after a
// stop, a render may also fail cleanly: async work of the render that
// stopped may never settle (docs/api.md), and the page code that stopped
// halfway may throw.
func allowed(o op, reused bool, out outcome) bool {
	switch out {
	case outHTML:
		return true
	case outThrown, outUnresolved, outResult:
		return reused
	case outInterrupted:
		return o == opTimeout || o == opInterrupt || o == opKill
	case outBudget:
		return o == opTimeout || o == opKill
	case outOOM:
		return o == opOOM
	case outKilled:
		return o == opKill
	case outAborted:
		return o == opAbort
	}
	return false
}

// classify says how a render ended. A kill must give the error docs/api.md
// describes.
func classify(p *page, html string, err error) (outcome, string) {
	if err == nil {
		if sha256.Sum256([]byte(html)) != p.hash {
			return outWrongHTML, fmt.Sprintf("%d bytes: %.300q", len(html), html)
		}
		return outHTML, ""
	}
	var e *slowjs.Error
	var re *resultError
	switch {
	case errors.Is(err, errBudget):
		return outBudget, ""
	case errors.Is(err, errUnresolved):
		return outUnresolved, ""
	case errors.As(err, &re):
		return outResult, re.Error()
	case !errors.As(err, &e):
		return outOther, fmt.Sprintf("%T: %v", err, err)
	case errors.Is(err, slowjs.ErrKilled):
		if !e.Interrupted || !e.Uncatchable || e.OutOfMemory || err.Error() != "InternalError: interrupted" {
			return outOther, fmt.Sprintf("a kill with the error %#v", e)
		}
		return outKilled, ""
	case e.Uncatchable && e.Interrupted && !e.OutOfMemory:
		return outInterrupted, e.Stack
	case e.Uncatchable && e.OutOfMemory && !e.Interrupted:
		return outOOM, ""
	case e.Uncatchable:
		return outOther, fmt.Sprintf("uncatchable, neither interrupted nor out of memory: %v\n%s", err, e.Stack)
	}
	return outThrown, fmt.Sprintf("%v\n%s", err, e.Stack)
}

// worker owns a pool of VMs: one steady VM of each bundle, which lives the
// whole run, and churn VMs, which are killed, and closed after a stop half
// of the time, as tifa closes every VM whose render failed.
type worker struct {
	s    *stress
	id   int
	rng  *rand.Rand
	pool []*pvm
	// engine is the sum of MemorySize of the VMs of the pool after each step.
	engine atomic.Int64
}

// randDur returns a duration from 0 to d.
func (w *worker) randDur(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return time.Duration(w.rng.Int64N(int64(d) + 1))
}

func (w *worker) pickOp(v *pvm) op {
	c := w.s.cfg
	n := w.rng.IntN(100)
	switch {
	case n < c.oom:
		return opOOM
	case n < c.oom+c.timeout:
		return opTimeout
	case n < c.oom+c.timeout+c.interrupt:
		return opInterrupt
	case n < c.oom+c.timeout+c.interrupt+c.kill && !v.steady:
		return opKill
	case n >= c.oom+c.timeout+c.interrupt+c.kill && n < c.oom+c.timeout+c.interrupt+c.kill+c.abort && !v.steady:
		return opAbort
	}
	return opNormal
}

// interrupter calls Interrupt on a VM after a while, from a goroutine of its
// own, unless it is ended first.
type interrupter struct {
	stop, done chan struct{}
	once       sync.Once
}

func (s *stress) interruptAfter(vm *slowjs.VM, d time.Duration) *interrupter {
	it := &interrupter{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(it.done)
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			vm.Interrupt()
			s.interrupts.Add(1)
		case <-it.stop:
		}
	}()
	return it
}

// end stops the interrupter, and returns once its goroutine has. Calling
// it again does nothing.
func (it *interrupter) end() {
	if it == nil {
		return
	}
	it.once.Do(func() {
		close(it.stop)
		<-it.done
	})
}

func (w *worker) run(end time.Time) {
	for len(w.pool) > 0 && time.Now().Before(end) && !w.s.stopNow() {
		i := w.rng.IntN(len(w.pool))
		if !w.step(i) {
			w.replace(i, end)
		}
	}
	for i, v := range w.pool {
		if v == nil {
			continue
		}
		w.closeVM(v, "at the end")
		w.pool[i] = nil
	}
	w.engine.Store(0)
}

// replace loads a new VM in place of slot i of the pool, which is closed. A
// load runs no stop and has the whole memory limit, so it fails only when the
// OS refuses memory (-refuse), and then it is tried again, or on a bug: then
// the slot is dropped.
func (w *worker) replace(i int, end time.Time) {
	old := w.pool[i]
	for time.Now().Before(end) {
		v, err := load(old.b, w.s.nextID(), w.s.cfg.renderTimeout)
		if err == nil {
			v.steady = old.steady
			w.pool[i] = v
			w.s.loaded.Add(1)
			return
		}
		if !w.s.refusing || !isRefusal(err) {
			w.s.unexpected(w.id, old.id, "load", "", fmt.Sprintf("a load of %s failed: %v", old.b.name, err))
			break
		}
		w.s.loadsRefused.Add(1)
	}
	w.pool = append(w.pool[:i], w.pool[i+1:]...)
}

// errRefused is what the OS answers a map that -refuse refuses.
var errRefused = errors.New("stress: the OS refused the memory (-refuse)")

// isRefusal says that err is what a map -refuse refused leads to: that error,
// or running out of memory.
func isRefusal(err error) bool {
	var e *slowjs.Error
	return errors.Is(err, errRefused) || errors.As(err, &e) && e.OutOfMemory
}

// closeVM closes v, which must free all and return no error.
func (w *worker) closeVM(v *pvm, when string) {
	defer func() {
		if r := recover(); r != nil {
			w.s.unexpected(w.id, v.id, "close", "", fmt.Sprintf("Close %s panicked: %v\n%s", when, r, debug.Stack()))
		}
	}()
	if err := v.close(); err != nil {
		w.s.unexpected(w.id, v.id, "close", "", fmt.Sprintf("Close %s: %v", when, err))
	}
	w.s.closed.Add(1)
}

// step renders a page on slot i of the pool under an op drawn at random,
// checks how it ended and what the VM is good for after it, and collects. It
// returns false when the VM was closed and is to be replaced.
func (w *worker) step(i int) (keep bool) {
	v := w.pool[i]
	o := w.pickOp(v)
	p := v.b.pages[w.rng.IntN(len(v.b.pages))]
	var it *interrupter
	defer func() {
		if r := recover(); r != nil {
			// An abort of the engine, the one of opAbort or any other, leaves
			// the VM good only for Close, which must free it all, and the
			// other VMs go on: the renders after it are counted.
			aborted := isAbort(r)
			if aborted {
				w.s.aborts.Add(1)
			}
			if aborted && o == opAbort {
				w.s.counts.add(v.stops > 0, o, outAborted)
			} else {
				w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
			}
			if !v.closed {
				w.closeVM(v, "after a panic")
			}
			keep = false
		}
		it.end()
		w.updateEngine()
	}()

	timeout := w.s.cfg.renderTimeout
	switch o {
	case opOOM:
		// From what the VM holds to a quarter past what a render takes with
		// no collection: most stop at a random point, some fit.
		v.SetMemoryLimit(v.MemorySize() + uintptr(w.rng.Int64N(int64(p.grows+p.grows/4)+1)))
	case opTimeout:
		timeout = max(50*time.Microsecond, w.randDur(p.took*3/2))
	case opInterrupt:
		it = w.s.interruptAfter(v.VM, w.randDur(p.took*3/2))
	case opKill:
		if p.push == 0 {
			o = opInterrupt
			it = w.s.interruptAfter(v.VM, w.randDur(p.took*3/2))
			break
		}
		// The wrapped push may come after the render ends, which then
		// gives its HTML. Before it, or once the loop runs, the eval
		// timeout or the Interrupt stops the render: the loop never reads
		// the flag, so a poll kills it 50 ms and 1000 polls later.
		n := 1 + w.rng.IntN(p.push+p.push/4)
		if _, err := v.Eval(fmt.Sprintf(killJS, n), slowjs.EvalGlobal); err != nil {
			w.s.unexpected(w.id, v.id, "kill setup", p.name, err.Error())
			w.closeVM(v, "after its kill setup failed")
			return false
		}
		v.pushes = true
		timeout = time.Millisecond + w.randDur(2*p.took)
		if w.rng.IntN(2) == 0 {
			it = w.s.interruptAfter(v.VM, w.randDur(2*p.took))
		}
	case opAbort:
		if p.polls == 0 {
			o = opInterrupt
			it = w.s.interruptAfter(v.VM, w.randDur(p.took*3/2))
			break
		}
		// The poll may come after the render ends, which then gives its
		// HTML.
		abortAfterPolls(v.VM, 1+w.rng.Int64N(p.polls+p.polls/4+1))
	}

	reused := v.stops > 0
	var html string
	var err error
	if w.s.cfg.cpuprofile != "" {
		// the labels pick the renders of one VM out of the profile:
		// go tool pprof -tagfocus 'vm=^ID$'
		labels := pprof.Labels("worker", strconv.Itoa(w.id), "vm", strconv.Itoa(v.id), "op", opNames[o], "page", p.name)
		pprof.Do(context.Background(), labels, func(context.Context) { html, err = v.render(p.input, timeout) })
	} else {
		html, err = v.render(p.input, timeout)
	}
	if o == opAbort {
		abortAfterPolls(v.VM, 0)
	}
	v.renders++
	out, detail := classify(p, html, err)
	w.s.counts.add(reused, o, out)
	if out == outHTML && w.s.aborts.Load() > 0 {
		w.s.htmlAfterAbort.Add(1)
	}
	if !allowed(o, reused, out) && !(w.s.refusing && out == outOOM) {
		w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("%s (VM of %d renders, %d stops before): %s", outcomeNames[out], v.renders, v.stops, detail))
	}

	if out == outKilled {
		v.killed = true
		w.checkKilled(v, o, p)
		w.closeVM(v, "after a kill")
		w.s.kills.Add(1)
		return false
	}
	if o == opOOM {
		v.SetMemoryLimit(vmMemory)
	}
	stopped := out != outHTML
	if stopped {
		v.stops++
		if v.pushes || !v.steady && w.rng.IntN(2) == 0 {
			// tifa's pool: the VM of a failed render is closed.
			w.closeVM(v, "after a stop")
			w.s.retired.Add(1)
			return false
		}
		w.checkAfterStop(v, o, p)
	} else if v.pushes {
		// push is still wrapped: the VM is done
		w.closeVM(v, "after a kill that never came")
		w.s.retired.Add(1)
		return false
	}
	// What follows a render runs on a goroutine of its own, as the collection of
	// tifa's pool does: the VM changes goroutine between two calls.
	var ok bool
	handOver(func() { ok = w.after(v, o, p, it) })
	return ok
}

// handOver runs f on a new goroutine and waits for it. A panic of f goes on
// in the caller, with the stack where it began.
func handOver(f func()) {
	var p any
	var wg sync.WaitGroup
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				p = fmt.Sprintf("%v\n%s", r, debug.Stack())
			}
		}()
		f()
	})
	wg.Wait()
	if p != nil {
		panic(p)
	}
}

// after collects after a render, ends the interrupter, runs the jobs the
// collection queued and measures a steady VM now and then. It returns false
// when the VM was closed and is to be replaced.
func (w *worker) after(v *pvm, o op, p *page, it *interrupter) bool {
	if !w.between(v, o, p) {
		w.closeVM(v, "after its collection failed")
		return false
	}
	it.end()
	// The jobs the collection queued, with no stop pending: an Interrupt
	// that came after the render, while the VM collected, is cleared by
	// this call, the outermost (docs/api.md).
	if err := v.finalize(w.s.cfg.renderTimeout); err != nil {
		fout, detail := classify(p, "", err)
		switch {
		case w.s.refusing && fout == outOOM:
			w.s.finalizeFails.Add(1)
		case !v.reusedAfterStop() || fout == outOther || fout == outInterrupted || fout == outOOM || fout == outKilled:
			w.s.unexpected(w.id, v.id, opNames[o]+" finalize", p.name, fmt.Sprintf("%s: %s", outcomeNames[fout], detail))
		default:
			w.s.finalizeFails.Add(1)
		}
		if fout == outKilled {
			v.killed = true
		}
		w.closeVM(v, "after its jobs failed")
		return false
	}
	if v.steady && v.renders%w.s.cfg.measureEvery == 0 {
		w.measure(v)
	}
	return true
}

func (v *pvm) reusedAfterStop() bool { return v.stops > 0 }

// between collects after a render: tifa's collection most of the time, and
// at random a full collection, a young one, a full one with the memory given
// back, or none.
func (w *worker) between(v *pvm, o op, p *page) bool {
	var err error
	switch n := w.rng.IntN(100); {
	case n < 70:
		err = v.collect()
	case n < 80:
		err = v.full()
	case n < 90:
		v.RunYoungGC()
	case n < 95:
		v.RunGC()
		_, err = v.ReleaseMemory(0)
	}
	if err != nil {
		w.s.unexpected(w.id, v.id, opNames[o]+" collect", p.name, err.Error())
		return false
	}
	return true
}

// checkKilled holds a killed VM to docs/api.md: Interrupt, MemorySize,
// MemoryFree and GCThreshold still work, and a method that returns an error
// returns one that matches ErrKilled.
func (w *worker) checkKilled(v *pvm, o op, p *page) {
	v.Interrupt()
	size, free := v.MemorySize(), v.MemoryFree()
	_ = v.GCThreshold()
	if size == 0 || free > size {
		w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("after a kill MemorySize %d and MemoryFree %d", size, free))
	}
	if _, err := v.Eval(`1`, slowjs.EvalGlobal); !errors.Is(err, slowjs.ErrKilled) {
		w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("an Eval after a kill gave %v, want ErrKilled", err))
	}
	if _, err := v.ExecutePendingJobs(); !errors.Is(err, slowjs.ErrKilled) {
		w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("ExecutePendingJobs after a kill gave %v, want ErrKilled", err))
	}
}

// checkAfterStop holds a VM a stop that was not a kill ended a call of to
// docs/api.md: every method works.
func (w *worker) checkAfterStop(v *pvm, o op, p *page) {
	if err := v.SetEvalTimeout(w.s.cfg.renderTimeout); err != nil {
		w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("SetEvalTimeout after a stop: %v", err))
		return
	}
	r, err := v.Eval(`6 * 7`, slowjs.EvalGlobal)
	if (err != nil || r != 42) && !(w.s.refusing && isRefusal(err)) {
		w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("an Eval after a stop gave %v, %v", r, err))
	}
	if w.rng.IntN(10) == 0 {
		u := v.MemoryUsage()
		if size := v.MemorySize(); u.MallocSize != int64(size) || u.FreeSize < 0 || u.FreeSize > u.MallocSize {
			w.s.unexpected(w.id, v.id, opNames[o], p.name, fmt.Sprintf("MemoryUsage after a stop: MallocSize %d FreeSize %d, MemorySize %d", u.MallocSize, u.FreeSize, size))
		}
	}
}

// measure records what the steady VM v keeps after a full collection: what
// the objects it keeps take (MemorySize less MemoryFree) and MemorySize.
func (w *worker) measure(v *pvm) {
	if err := v.full(); err != nil {
		w.s.unexpected(w.id, v.id, "measure", "", err.Error())
		return
	}
	size, free := v.MemorySize(), v.MemoryFree()
	w.s.steady.record(w.id, v.b.name, int64(size-free), int64(size), v.renders)
}

func (w *worker) updateEngine() {
	var sum int64
	for _, v := range w.pool {
		if v != nil && !v.killed && !v.closed {
			sum += int64(v.MemorySize())
		}
	}
	w.engine.Store(sum)
}

// isAbort says whether r, the panic of a render, is an abort of the engine.
func isAbort(r any) bool {
	err, ok := r.(error)
	return ok && errors.Is(err, gohost.ErrAbort)
}
