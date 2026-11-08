// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fmt exposes some functionality of the Go stdlib fmt package to a
// [quickjs.VM].
package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"fmt"
	"runtime"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/vmhook"
)

const pkg = "fmt"

// Register sets the "fmt" property of 'obj' to the fmt package javascript object.
//
// The functions registered are
//
//   - [fmt.Printf]
//   - [fmt.Println]
//   - [fmt.Sprintf]
//   - [fmt.Sprint]
//
// The javascript functions follow the usual conventions where the first letter
// of a function name is not capitalized. So [fmt.Println] becomes
// 'fmt.println' in javascript, for example.
//
// A call makes its whole output in Go before it returns it or writes it, as
// Go's fmt does, and a short format can ask for a long output. So each
// function first finds a bound on the bytes of its output, from the format
// and the arguments, and when the VM has a memory limit (see
// [quickjs.VM.SetMemoryLimit]) and what Go may allocate to make that many
// bytes, sixteen times them and a few KiB for any call, is more than the VM
// has left under it, the call ends with the uncatchable out of memory error
// and makes nothing. The bound is at most five times the output and a few
// hundred bytes for each verb, more when a string has a precision or for
// %w, so a call whose output fits in a small part of what is left runs as
// before. Go makes the
// output without a check of the VM, so neither [quickjs.VM.Interrupt] nor
// the eval timeout ends it, in a time that grows with the bound.
//
// printf and println write to os.Stdout, and a write that blocks, as one to
// a full pipe nobody reads does, holds the call, and the goroutine running
// the VM, until it is done: neither [quickjs.VM.Interrupt] nor the eval
// timeout ends it.
func Register(obj quickjs.Value) (err error) {
	defer obj.Free()

	vm := obj.VM()
	mod, err := vm.EvalThisValue(obj.Dup(), fmt.Sprintf("this.%s={}; this.%[1]s", pkg), quickjs.EvalGlobal)
	if err != nil {
		return err
	}

	defer mod.Free()

	for _, v := range []struct {
		nm       string
		f        any
		wantThis bool
	}{
		{"printf", func(format string, a ...any) (int, error) {
			format, ok := fitsPrintf(vm, format, a)
			if !ok {
				return 0, nil
			}
			return fmt.Printf(format, a...)
		}, false},
		{"println", func(a ...any) (int, error) {
			if !fits(vm, vmhook.Room(vm), sizePrint(a), 0) {
				return 0, nil
			}
			return fmt.Println(a...)
		}, false},
		{"sprint", func(a ...any) string {
			if !fits(vm, vmhook.Room(vm), sizePrint(a), 0) {
				return ""
			}
			return fmt.Sprint(a...)
		}, false},
		{"sprintf", func(format string, a ...any) string {
			format, ok := fitsPrintf(vm, format, a)
			if !ok {
				return ""
			}
			return fmt.Sprintf(format, a...)
		}, false},
	} {
		longName := fmt.Sprintf("__%s_%s__", pkg, v.nm)
		if err := vm.RegisterFunc(longName, v.f, v.wantThis); err != nil {
			return err
		}

		if _, err := vm.EvalThis(mod.Dup(), fmt.Sprintf("this.%s = %s", v.nm, longName), quickjs.EvalGlobal); err != nil {
			return err
		}
	}

	return nil
}

// allocated is how many times the bound on its output Go's fmt may
// allocate to make it (see fmt/print.go and fmt/format.go), all of it
// counted, as Go may collect none of it before the call returns:
//
//   - its buffer grows by append, to at least 1.25 times what it held
//     (growslice in runtime/slice.go), and by writePadding, to twice its
//     capacity and the padding. So the buffers before the last come to at
//     most 5 times the one before the last, which is less than the output,
//     and the last to at most 3 times the output: 8 times the output;
//   - Sprintf and Sprint copy the buffer into the string they return, once
//     the output;
//   - a verb that quotes a string or writes a float makes its bytes in a
//     slice of its own first, grown by append as the buffer is, to at most
//     7 times what it writes, and an integer with a width or a precision
//     makes a slice of that size, and what the verbs write comes to at most
//     the output. A big int makes its digits, and what it takes to make
//     them, in less than its bound.
//
// That is 16 times the bound, with the bytes Go rounds each object up to.
// On the long outputs of TestAllocatedUnderBound, Go allocated up to 11
// times the bound.
const allocated = 16

// budget is what Go may allocate to make an output of at most n bytes with
// more bytes allocated beside it: allocated times n, more, and what Go's fmt
// allocates for any call. That is its printer (pp in fmt/print.go), a few
// hundred bytes, and, when a collection has emptied the pool of printers,
// the slots of the pool, 128 bytes for each P (poolLocal in sync/pool.go),
// which Go rounds up, and the place of the pool in the list of pools. On
// "" fmt.Sprintf allocated 312 bytes with GOMAXPROCS 1 and 9,656 with 64.
func budget(n, more int64) int64 {
	return allocated*n + more + 1<<10 + 256*int64(runtime.GOMAXPROCS(0))
}

// fitsPrintf is the format sizePrintf hands fmt, and whether what Go
// allocates to make fmt.Sprintf of it fits in what vm has left under its
// memory limit (fits).
func fitsPrintf(vm *quickjs.VM, format string, a []any) (string, bool) {
	room := vmhook.Room(vm)
	n, read, ok := sizePrintf(format, a, room)
	if !ok {
		vmhook.OutOfMemory(vm)
		return "", false
	}
	var more int64
	if len(read) != len(format) {
		more = int64(len(read))
	}
	return read, fits(vm, room, n, more)
}

// fits reports whether an output of at most n bytes, made with more bytes
// allocated beside it, fits in the room vm has left under its memory limit
// (vmhook.Room): allocated times n, and more. When it does not, it throws the
// out of memory error, which ends the call: what the function returns then
// is dropped.
func fits(vm *quickjs.VM, room, n, more int64) bool {
	if room >= 0 && budget(n, more) > room {
		vmhook.OutOfMemory(vm)
		return false
	}
	return true
}
