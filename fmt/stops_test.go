// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"errors"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The output of fmt, which a script can make long, is made and written a
// piece at a time, with a read of vmhook.Stop at least once per
// bytesPerStop bytes, or refused. The tests count the reads and read no
// clock (docs/testing.md, "Time in a test").

// bytesPerStop is the most bytes of output fmt makes or writes between two
// reads of vmhook.Stop.
const bytesPerStop = 1 << 16

// countStops evaluates src on vm and returns how many times it read
// vmhook.Stop. The tests that call it do not run in parallel, so no other
// call reads vmhook.Stop.
func countStops(t *testing.T, vm *quickjs.VM, src string) (int, error) {
	t.Helper()
	n := 0
	stop := vmhook.Stop
	vmhook.Stop = func(vm any) bool {
		n++
		return stop(vm)
	}
	defer func() { vmhook.Stop = stop }()
	_, err := vm.Eval("(function () { "+src+"; })(); 0", quickjs.EvalGlobal)
	return n, err
}

// refused reports whether err is a RangeError a script can catch or the end
// of the call for want of memory.
func refused(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && (!e.Uncatchable && e.Name == "RangeError" || e.Uncatchable && e.OutOfMemory)
}

// In a VM with no memory limit, the output of sprintf is bounded by nothing
// but what the script asks: "%[1]q" 64 times of 1 MiB of control characters
// made 256 MiB in Go, and ran 636 ms past an eval timeout of 20 ms. sprintf
// refuses an output whose bound is past 16 MiB, as a VM with that much room
// left under its memory limit does.
func TestSprintfNoLimitStops(t *testing.T) {
	const n = 256 << 10
	const out = 16 * 4 * n
	vm := newVM(t)
	if _, err := vm.Eval(`var s = '\x01'.repeat(262144); 0`, quickjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	stops, err := countStops(t, vm, `fmt.sprintf('%[1]q'.repeat(16), s)`)
	switch {
	case refused(err):
	case err != nil:
		t.Fatal(err)
	case stops < out/bytesPerStop:
		t.Errorf("sprintf of %d bytes read vmhook.Stop %d times, want at least %d", out, stops, out/bytesPerStop)
	}
}

// println writes a long line to its writer a piece at a time, with a read
// of vmhook.Stop between two pieces, so that Interrupt and the eval timeout
// end the call while a slow reader takes the output. It wrote 1 MiB to a
// pipe in one write, which held the call 2 s past Interrupt, until the pipe
// was read.
func TestPrintlnWritesInPieces(t *testing.T) {
	const n = 4 << 20
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	drained := make(chan int64)
	go func() {
		k, _ := io.Copy(io.Discard, r)
		drained <- k
	}()
	vm := newVM(t)
	stops, err := countStops(t, vm, `fmt.println('x'.repeat(4194304))`)
	os.Stdout = stdout
	w.Close()
	if k := <-drained; k != n+1 && err == nil {
		t.Errorf("println wrote %d bytes, want %d", k, n+1)
	}
	switch {
	case err != nil:
		t.Fatal(err)
	case stops < n/bytesPerStop:
		t.Errorf("println of %d bytes read vmhook.Stop %d times, want at least %d", n, stops, n/bytesPerStop)
	}
	r.Close()
}

// interrupter is a writer that interrupts its VM at its write number at.
type interrupter struct {
	vm     *quickjs.VM
	at     int
	writes int
	n      int
}

func (w *interrupter) Write(p []byte) (int, error) {
	w.writes++
	w.n += len(p)
	if w.writes == w.at {
		w.vm.Interrupt()
	}
	return len(p), nil
}

// With Writer, printf and println write to the writer given, and not to
// os.Stdout, 64 KiB at a time, and end the call at the interrupt that came
// during a write, before the next one.
func TestWriterInterrupted(t *testing.T) {
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	w := &interrupter{vm: vm, at: 3}
	g := vm.GlobalObject()
	defer g.Free()
	if err := Register(g.Dup(), Writer(w)); err != nil {
		t.Fatal(err)
	}
	for _, js := range []string{`fmt.println('x'.repeat(4194304))`, `fmt.printf('%s', 'x'.repeat(4194304))`} {
		w.writes, w.n = 0, 0
		_, err := vm.Eval(js, quickjs.EvalGlobal)
		var e *quickjs.Error
		if !errors.As(err, &e) || !e.Interrupted {
			t.Errorf("%s: got %v, want the interrupt", js, err)
		}
		if w.writes != 3 || w.n != 3*writePiece {
			t.Errorf("%s: %d writes of %d bytes in all, want 3 of %d", js, w.writes, w.n, 3*writePiece)
		}
	}
	w.writes, w.n = 0, 0
	if v, err := vm.Eval(`fmt.printf('%d|', 42)`, quickjs.EvalGlobal); err != nil || w.n != 3 {
		t.Errorf("printf after the interrupt: %v, %v, %d bytes", v, err, w.n)
	}
}

// A verb may be as long as the format: a run of flags, or of zeros before a
// width, a precision or an argument index. sprintf reads vmhook.Stop in it
// while it finds the bound of the output and again while it makes the
// output, and hands Go's fmt the verb with each flag once and its numbers
// without the zeros. A verb of 16 MiB ran 100 to 260 ms with no read on an
// Apple M4.
func TestLongVerbsStop(t *testing.T) {
	const n = 4 << 20
	vm := newVM(t)
	for _, c := range []struct{ format, want string }{
		{`'%' + '0'.repeat(n) + '5d'`, "00001"},
		{`'%' + '-+'.repeat(n / 2) + '5d'`, "+1   "},
		{`'%.' + '0'.repeat(n) + '3d'`, "001"},
		{`'%[' + '0'.repeat(n) + '1]d'`, "1"},
	} {
		src := "var n = " + strconv.Itoa(n) + "; globalThis.got = fmt.sprintf(" + c.format + ", 1)"
		stops, err := countStops(t, vm, src)
		if err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		if got, err := vm.Eval("got", quickjs.EvalGlobal); err != nil || got != c.want {
			t.Errorf("sprintf(%s) is %v, %v, want %q", c.format, got, err, c.want)
		}
		if stops < 2*n/bytesPerStop {
			t.Errorf("sprintf(%s) read vmhook.Stop %d times, want at least %d", c.format, stops, 2*n/bytesPerStop)
		}
	}
}
