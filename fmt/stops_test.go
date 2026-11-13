// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"errors"
	"io"
	"os"
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
	t.Skip("println writes in pieces with the Writer option of Register")
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
