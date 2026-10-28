// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && ((linux && (amd64 || arm64)) || (darwin && arm64)) && testhooks

package slowjs // import "github.com/liustack/slowjs"

// What the system refusing memory in the middle of a render does on the C
// engine: hostmem.SetFault makes the calls to the OS fail. The one VM whose
// call was refused ends the call with an out of memory error, the process
// stays up, and the other VMs and the VMs made afterwards run as before.

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liustack/slowjs/internal/gohost"
	"github.com/liustack/slowjs/internal/hostmem"
)

// growScript holds a few MiB in many strings and objects, which the engine
// asks the system for as chunks, then lets go of them.
const growScript = `(() => {
  const keep = [];
  for (let i = 0; i < 600; i++) keep.push({ id: i, text: 'item' + i + 'x'.repeat(4000) + i, list: new Array(100).fill(i) });
  return keep.length;
})()`

// refusal is the hook of hostmem.SetFault for these tests. It stays set for a whole test, so that
// no VM on another goroutine meets it being set, and its behaviour is in atomics.
type refusal struct {
	call string // the call to refuse, as hostmem names it
	// armed turns the refusals on. from is the first matching call to refuse after arming (1 is the
	// next one) and count how many to refuse from there, 0 for all of them. goroutine limits it to
	// the calls of one goroutine (its number from goid), 0 for any.
	armed     atomic.Bool
	from      atomic.Int64
	count     atomic.Int64
	goroutine atomic.Int64
	seen      atomic.Int64 // matching calls since arming
	refused   atomic.Int64
}

// goid is the number of the calling goroutine, read from the head of its stack trace. Only the
// tests use it, for the hook of a refusal that is meant for one goroutine.
func goid() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id int64
	fmt.Sscanf(string(buf[:n]), "goroutine %d ", &id)
	return id
}

func (r *refusal) install(t *testing.T) {
	t.Helper()
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call != r.call || !r.armed.Load() {
			return nil
		}
		if g := r.goroutine.Load(); g != 0 && g != goid() {
			return nil
		}
		n := r.seen.Add(1)
		if n < r.from.Load() {
			return nil
		}
		if c := r.count.Load(); c != 0 && n >= r.from.Load()+c {
			return nil
		}
		r.refused.Add(1)
		return errors.New("refused by the test")
	})
	t.Cleanup(func() { hostmem.SetFault(nil) })
}

// arm refuses count matching calls from the from-th one on (count 0: all of them), of goroutine g
// (0: any).
func (r *refusal) arm(from, count, g int64) {
	r.seen.Store(0)
	r.refused.Store(0)
	r.from.Store(from)
	r.count.Store(count)
	r.goroutine.Store(g)
	r.armed.Store(true)
}

func (r *refusal) disarm() { r.armed.Store(false) }

func noLeftovers(t *testing.T) {
	t.Helper()
	if l := gohost.Leftovers(); len(l) != 0 {
		t.Fatalf("closed VMs still hold memory: %v", l)
	}
}

// TestMapRefusedSweep refuses the system's chunks while a script grows the heap, for each n up to
// the number the script asks for and a few past it, in two ways: all the chunks from the nth on
// (the system is out of memory), and only the nth (a refusal that passes). The first ends the call
// with an out of memory error or, when the script needed fewer chunks, a result. The second may
// pass unseen, since the engine asks again for less. Nothing panics, Close gives everything back,
// and the next VM runs the script.
func TestMapRefusedSweep(t *testing.T) {
	r := &refusal{call: "map"}
	r.install(t)
	// How many chunks does a clean run ask for?
	m := plainVM(t)
	r.arm(1<<30, 0, 0)
	if _, err := m.Eval(growScript, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	total := int(r.seen.Load())
	r.disarm()
	m.Close()
	if total < 3 {
		t.Fatalf("the script asked for %d chunks, too few to sweep", total)
	}
	t.Logf("a clean run asks the system for %d chunks", total)
	for _, mode := range []struct {
		name  string
		count int64
	}{{"all from the nth", 0}, {"only the nth", 1}} {
		var oom, ok, absorbed, stillWorks int
		for n := 1; n <= total+2; n++ {
			m := plainVM(t)
			r.arm(int64(n), mode.count, 0)
			out, err := m.Eval(growScript, EvalGlobal)
			refused := r.refused.Load()
			r.disarm()
			var e *Error
			switch {
			case err == nil:
				ok++
				if refused != 0 {
					absorbed++
				}
				if out != 600 {
					t.Errorf("%s n=%d: result %v", mode.name, n, out)
				}
			case errors.As(err, &e) && e.OutOfMemory:
				oom++
				if refused == 0 {
					t.Errorf("%s n=%d: out of memory with no chunk refused", mode.name, n)
				}
				if v, err2 := m.Eval(`1 + 1`, EvalGlobal); err2 == nil && v == 2 {
					stillWorks++
				}
			default:
				t.Errorf("%s n=%d: unexpected error %v", mode.name, n, err)
			}
			if err := m.Close(); err != nil {
				t.Errorf("%s n=%d: Close: %v", mode.name, n, err)
			}
			noLeftovers(t)
			// A VM made after the refusal is as good as new.
			m2 := plainVM(t)
			if v, err := m2.Eval(growScript, EvalGlobal); err != nil || v != 600 {
				t.Errorf("%s n=%d: the next VM: %v, %v", mode.name, n, v, err)
			}
			m2.Close()
		}
		t.Logf("%s: %d calls ended with out of memory (%d of those VMs ran a small script after), %d gave a result (%d of them after a refusal the engine got round)",
			mode.name, oom, stillWorks, ok, absorbed)
		if mode.count == 0 && oom == 0 {
			t.Error("no refusal ended a call: the hook does not reach the engine's allocations")
		}
	}
}

// TestMapRefusedAmongVMs runs 4 VMs on goroutines of their own, each making a new VM for every
// render, as a pool does when it replaces one. For 400 ms the system refuses every chunk to one of
// the goroutines: its renders end with out of memory and the VMs it makes after a refusal are
// closed, while the other three go on without one failure. Then the refusal ends and the first
// goroutine renders again.
func TestMapRefusedAmongVMs(t *testing.T) {
	r := &refusal{call: "map"}
	r.install(t)
	const workers = 4
	type count struct{ renders, oom, newFailed, wrong atomic.Int64 }
	counts := make([]*count, workers)
	var victim atomic.Int64
	ready := make(chan struct{}, workers)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := range workers {
		counts[w] = &count{}
		wg.Go(func() {
			c := counts[w]
			if w == 0 {
				victim.Store(goid())
			}
			ready <- struct{}{}
			for {
				select {
				case <-stop:
					return
				default:
				}
				m, err := NewVM()
				if err != nil {
					c.newFailed.Add(1)
					time.Sleep(5 * time.Millisecond)
					continue
				}
				v, err := m.Eval(growScript, EvalGlobal)
				var e *Error
				switch {
				case err == nil && v == 600:
					c.renders.Add(1)
				case errors.As(err, &e) && e.OutOfMemory:
					c.oom.Add(1)
				default:
					c.wrong.Add(1)
					t.Errorf("worker %d: %v, %v", w, v, err)
				}
				if err := m.Close(); err != nil {
					c.wrong.Add(1)
					t.Errorf("worker %d: Close: %v", w, err)
				}
			}
		})
	}
	for range workers {
		<-ready
	}
	snapshot := func() (out [workers][4]int64) {
		for w, c := range counts {
			out[w] = [4]int64{c.renders.Load(), c.oom.Load(), c.newFailed.Load(), c.wrong.Load()}
		}
		return
	}
	time.Sleep(500 * time.Millisecond)
	before := snapshot()
	r.arm(1, 0, victim.Load())
	time.Sleep(400 * time.Millisecond)
	r.disarm()
	during := snapshot()
	time.Sleep(500 * time.Millisecond)
	after := snapshot()
	close(stop)
	wg.Wait()
	for w := range workers {
		t.Logf("worker %d: renders %d, out of memory %d, no VM %d (before the refusal), then +%d, +%d, +%d, then +%d renders",
			w, before[w][0], before[w][1], before[w][2], during[w][0]-before[w][0], during[w][1]-before[w][1], during[w][2]-before[w][2], after[w][0]-during[w][0])
	}
	t.Logf("%d chunks refused", r.refused.Load())
	if r.refused.Load() == 0 {
		t.Fatal("nothing was refused")
	}
	if during[0][1]-before[0][1]+during[0][2]-before[0][2] == 0 {
		t.Error("the refused goroutine saw no failure")
	}
	for w := 1; w < workers; w++ {
		if during[w][1] != before[w][1] || during[w][2] != before[w][2] || after[w][3] != 0 {
			t.Errorf("worker %d met a failure it was not meant to", w)
		}
		if during[w][0] == before[w][0] {
			t.Errorf("worker %d rendered nothing while worker 0 was refused", w)
		}
	}
	if after[0][0]-during[0][0] == 0 {
		t.Error("the refused goroutine did not render again once the system was back")
	}
	if after[0][3] != 0 {
		t.Error("the refused goroutine saw something else than out of memory")
	}
	noLeftovers(t)
}

// TestReleaseRefused refuses the calls that give free memory back to the system. ReleaseMemory
// reports what it did give back, and the VM runs on.
func TestReleaseRefused(t *testing.T) {
	r := &refusal{call: "release"}
	r.install(t)
	m := plainVM(t)
	if _, err := m.Eval(growScript, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	r.arm(1, 0, 0)
	given, err := m.ReleaseMemory(0)
	r.disarm()
	t.Logf("with every release refused (%d of them): ReleaseMemory gave back %d bytes, error %v", r.refused.Load(), given, err)
	if v, err := m.Eval(growScript, EvalGlobal); err != nil || v != 600 {
		t.Fatalf("after the refused release: %v, %v", v, err)
	}
	m.RunGC()
	given, err = m.ReleaseMemory(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("with the system back: ReleaseMemory gave back %d bytes", given)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	noLeftovers(t)
}

// TestRemapRefused refuses the growth in place of a big block. A big array and a big string grow
// by remap; the call must end well, with a result or out of memory.
func TestRemapRefused(t *testing.T) {
	r := &refusal{call: "remap"}
	r.install(t)
	const script = `(() => { let s = ''; for (let i = 0; i < 200; i++) s += 'y'.repeat(100000); const a = []; for (let i = 0; i < 2e6; i++) a.push(i); return s.length + a.length })()`
	m := plainVM(t)
	r.arm(1<<30, 0, 0)
	want, err := m.Eval(script, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	total := r.seen.Load()
	r.disarm()
	m.Close()
	t.Logf("a clean run asks for %d remaps", total)
	for n := int64(1); n <= min(total, 40); n++ {
		m := plainVM(t)
		r.arm(n, 0, 0)
		v, err := m.Eval(script, EvalGlobal)
		r.disarm()
		var e *Error
		if err != nil && !(errors.As(err, &e) && e.OutOfMemory) {
			t.Errorf("n=%d: %v", n, err)
		}
		if err == nil && fmt.Sprint(v) != fmt.Sprint(want) {
			t.Errorf("n=%d: result %v, want %v", n, v, want)
		}
		if err := m.Close(); err != nil {
			t.Errorf("n=%d: Close: %v", n, err)
		}
	}
	noLeftovers(t)
}

// plainVM is a VM the test closes itself, once, since a refusal test closes the VM it was
// refused.
func plainVM(t *testing.T) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	return m
}
