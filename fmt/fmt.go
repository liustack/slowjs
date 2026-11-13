// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fmt exposes some functionality of the Go stdlib fmt package to a
// [quickjs.VM].
package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"fmt"
	"io"
	"os"
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
// and the arguments, and when what making the output may make Go allocate,
// sixteen times its bytes and a few KiB for the printer that makes it, is
// more than the VM has left under its memory limit (see
// [quickjs.VM.SetMemoryLimit]), the call ends with the uncatchable out of
// memory error and makes nothing. A VM with no memory limit has room for
// an output of at most 16 MiB. The bound comes from the format
// and the arguments, not from the output, and can be far above it: when an
// argument is longer than what is printed of it, as a string cut by its
// precision, or when a width or a precision is not used, as by a verb that
// does not fit its argument or for a zero printed with a precision of 0.
// Such a call can be refused under a tight memory limit. A call whose bound
// fits in a small part of what is left runs as before.
//
// A call whose bound is more than 64 KiB makes its output a verb at a time,
// and a long string, object or big int a part at a time, in the same bytes
// as Go's fmt, and ends between two parts when [quickjs.VM.Interrupt] or
// the eval timeout asks it to. It takes about as long as Go's fmt for a long
// argument, and about three times as long for many short verbs. The longest
// part is in the conversion of a big int to decimal: one division by
// math/big of the whole big int, about 5 ms on an Apple M4 for the
// 1,048,576 bits of the longest the engine makes.
//
// printf and println write to os.Stdout, or to the writer of [Writer], 64
// KiB at a time, and end the call between two writes when
// [quickjs.VM.Interrupt] or the eval timeout asks it to. A write that
// blocks, as one to a full pipe nobody reads does, holds the call, and the
// goroutine running the VM, until it is done.
func Register(obj quickjs.Value, opts ...Option) (err error) {
	defer obj.Free()

	var o options
	for _, opt := range opts {
		opt(&o)
	}
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
			s, ok := makePrintf(vm, format, a)
			if !ok {
				return 0, nil
			}
			return write(vm, o.writer(), s)
		}, false},
		{"println", func(a ...any) (int, error) {
			s, ok := makePrint(vm, a, true)
			if !ok {
				return 0, nil
			}
			return write(vm, o.writer(), s)
		}, false},
		{"sprint", func(a ...any) string {
			s, _ := makePrint(vm, a, false)
			return s
		}, false},
		{"sprintf", func(format string, a ...any) string {
			s, _ := makePrintf(vm, format, a)
			return s
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

// An Option changes what Register sets up.
type Option func(*options)

type options struct {
	w io.Writer
}

// Writer makes printf and println write to w, where they write to
// os.Stdout without it.
func Writer(w io.Writer) Option { return func(o *options) { o.w = w } }

// writer is the writer of o, or os.Stdout as it is when the call writes.
func (o *options) writer() io.Writer {
	if o.w != nil {
		return o.w
	}
	return os.Stdout
}

// writePiece is the most bytes printf and println write at once.
const writePiece = 64 << 10

// write writes s to w, writePiece bytes at a time, and stops between two
// writes when the call is to end (vmhook.Stop), which throws the error
// that ends it. It returns what it wrote.
func write(vm *quickjs.VM, w io.Writer, s string) (int, error) {
	n := 0
	for {
		k, err := io.WriteString(w, s[:min(len(s), writePiece)])
		n += k
		if err != nil {
			return n, err
		}
		s = s[k:]
		if len(s) == 0 || vmhook.Stop(vm) {
			return n, nil
		}
	}
}

// maxOutput is the most bytes of output a call may make in a VM with no
// memory limit: the call takes the room for it as the room it has left.
// sprintf makes 16 MiB of quoted bytes in about 25 ms on an Apple M4, and
// may allocate 16 times the bound (allocated).
const maxOutput = 16 << 20

// room is the room vm has left under its memory limit (vmhook.Room), or
// that of an output of maxOutput bytes when it has none.
func room(vm *quickjs.VM) int64 {
	if r := vmhook.Room(vm); r >= 0 {
		return r
	}
	return budget(maxOutput, 0)
}

// allocated is how many times the bound on its output a call may make Go
// allocate, counted whole, as Go may collect none of it before the call
// returns. Go's fmt makes the output of a call whose bound is at most
// shortOutput whole (see fmt/print.go and fmt/format.go):
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
// sprintf makes a longer output in a buffer of the size of the bound, which
// never grows: once the bound. Each verb Go's fmt makes for it, it makes as
// a call of that verb alone, which allocates as above but for the copy into
// a string: at most 15 times what the verb writes. A long string or object
// sprintf writes into the buffer with no more than the quotes of one piece
// beside it, and a long big int it converts to decimal in quotients,
// remainders and powers of ten, which came to 3.9 times its part of the
// bound for the longest. That is 16 times the bound too. On the long
// outputs of TestOutputAllocatedUnderBudget, Go allocated up to 8 times the
// bound.
const allocated = 16

// budget is what an output of at most n bytes, with more bytes allocated
// beside it, may make Go's fmt allocate: allocated times n, more, and the
// printer that makes it, the same for any output. That is the printer (pp
// in fmt/print.go), a few hundred bytes, and, when a collection has emptied
// the pool of printers, the slots of the pool, 128 bytes for each P
// (poolLocal in sync/pool.go), which Go rounds up. On "" fmt.Sprintf
// allocated 312 bytes with GOMAXPROCS 1 and 9,656 with 64.
//
// What Go's runtime sets up for its pools is not counted: after a
// collection, the first use of each pool of the program puts it in the
// runtime's list of pools again (allPools in sync/pool.go), which grows
// with how many pools the program uses, not with what the script asks.
func budget(n, more int64) int64 {
	return allocated*n + more + 1<<10 + 256*int64(runtime.GOMAXPROCS(0))
}

// shortOutput is the largest bound on its output a call may have for Go's
// fmt to make it whole: it does so in well under a millisecond, and faster
// than sprintf does verb by verb.
const shortOutput = 1 << 16

// makePrintf is the output of fmt.Sprintf(format, a...), when what it may
// make Go's fmt allocate fits in what vm has left under its memory limit
// (fits), made whole by Go's fmt when it is short and by sprintf otherwise.
// It is false when the call is to end, with the error that ends it thrown.
func makePrintf(vm *quickjs.VM, format string, a []any) (string, bool) {
	room := room(vm)
	n, read, ok := sizePrintf(format, a, room)
	if !ok {
		vmhook.OutOfMemory(vm)
		return "", false
	}
	var more int64
	if len(read) != len(format) {
		more = int64(len(read))
	}
	if !fits(vm, room, n, more) {
		return "", false
	}
	return printfOf(vm, n, read, a)
}

// printfOf is the output of fmt.Sprintf(format, a...), whose bound is n,
// made whole by Go's fmt when it is short and by sprintf otherwise.
func printfOf(vm *quickjs.VM, n int64, format string, a []any) (string, bool) {
	if n <= shortOutput {
		return fmt.Sprintf(format, a...), true
	}
	return sprintf(vm, n, format, a)
}

// makePrint is makePrintf for fmt.Sprint(a...), or fmt.Sprintln(a...) when
// ln is true.
func makePrint(vm *quickjs.VM, a []any, ln bool) (string, bool) {
	n := sizePrint(a)
	if !fits(vm, room(vm), n, 0) {
		return "", false
	}
	return printOf(vm, n, a, ln)
}

// printOf is printfOf for fmt.Sprint(a...), or fmt.Sprintln(a...) when ln
// is true.
func printOf(vm *quickjs.VM, n int64, a []any, ln bool) (string, bool) {
	switch {
	case n <= shortOutput && ln:
		return fmt.Sprintln(a...), true
	case n <= shortOutput:
		return fmt.Sprint(a...), true
	case ln:
		return sprintln(vm, n, a)
	}
	return sprint(vm, n, a)
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
