// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package fmt gives a [slowjs.VM] some of Go's fmt: printf, println, sprint
// and sprintf.
package fmt // import "github.com/liustack/slowjs/fmt"

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"unsafe"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/vmhook"
)

type options struct {
	w io.Writer
}

// An Option configures [Register].
type Option func(*options)

// Writer makes printf and println write to w. Without it they write to
// os.Stdout, as it is at the time of each call.
func Writer(w io.Writer) Option {
	return func(o *options) { o.w = w }
}

const (
	// direct is the longest output, as bounded before it is made, that
	// Go's fmt makes in one go: that is faster than making it a piece at a
	// time (printf.go), and short enough to hold.
	direct = 1 << 16
	// writePiece is the most bytes printf and println hand their writer at
	// once: between two writes they check whether the call is to end.
	writePiece = 64 << 10
	// maxOutput is the output a VM with no memory limit has room for.
	maxOutput = 16 << 20
)

// budget is what Go's fmt may allocate to make n bytes of output, besides
// more allocated for the call, counted whole, as Go may collect none of it
// before the call returns.
//
// fmt's buffer grows by append to at least 1.25 times what it held, and by
// its padding to twice, so the buffers it goes through add up to at most 8
// times the output. A verb that quotes a string or writes a float first
// makes its bytes in a slice of its own, grown the same way, and Sprintf
// copies the buffer into the string it returns: 16 times the output in all.
// Any output also takes a printer, a few hundred bytes, and after a
// collection the slots of the pool of printers, 128 bytes for each P, for
// which the budget has 1 KiB and 256 bytes for each P.
func budget(n, more int64) int64 {
	return 16*n + more + 1024 + 256*int64(runtime.GOMAXPROCS(0))
}

// Register sets obj.fmt, on the object obj, normally the global object, to
// an object with the functions:
//
//	printf(format, ...args)  writes fmt.Sprintf(format, args...), returns [bytes written, error or null]
//	println(...args)         writes fmt.Sprintln(args...), returns the same
//	sprint(...args)          returns fmt.Sprint(args...)
//	sprintf(format, ...args) returns fmt.Sprintf(format, args...)
//
// They are also the global functions __fmt_printf__, __fmt_println__,
// __fmt_sprint__ and __fmt_sprintf__. The arguments convert to Go as the
// arguments of a function of RegisterFunc do. An output whose bound is more
// than the memory limit of the VM allows, or more than 16 MiB in a VM with
// no limit, ends the call with the out of memory error.
//
// Register takes obj over and frees it.
func Register(obj slowjs.Value, opts ...Option) error {
	defer obj.Free()

	o := options{}
	for _, f := range opts {
		f(&o)
	}

	// EvalThis takes over the value it is given as this: each call gets a
	// reference of its own.
	vm := obj.VM()
	if _, err := vm.EvalThis(obj.Dup(), "this.fmt = {}", slowjs.EvalGlobal); err != nil {
		return err
	}

	w := func() io.Writer {
		if o.w != nil {
			return o.w
		}
		return os.Stdout
	}
	funcs := []struct {
		name string
		f    any
	}{
		{"printf", func(format string, a ...any) (int, error) {
			s, give, ok := formatPrintf(vm, format, a)
			if !ok {
				return 0, nil
			}
			defer give()
			return write(vm, w(), s)
		}},
		{"println", func(a ...any) (int, error) {
			s, give, ok := formatPrint(vm, a, true)
			if !ok {
				return 0, nil
			}
			defer give()
			return write(vm, w(), s)
		}},
		{"sprint", func(a ...any) slowjs.Value {
			s, give, ok := formatPrint(vm, a, false)
			if !ok {
				return slowjs.Value{}
			}
			defer give()
			return newString(vm, s)
		}},
		{"sprintf", func(format string, a ...any) slowjs.Value {
			s, give, ok := formatPrintf(vm, format, a)
			if !ok {
				return slowjs.Value{}
			}
			defer give()
			return newString(vm, s)
		}},
	}
	for _, f := range funcs {
		global := "__fmt_" + f.name + "__"
		if err := vm.RegisterFunc(global, f.f, false); err != nil {
			return err
		}

		if _, err := vm.EvalThis(obj.Dup(), "this.fmt."+f.name+" = "+global, slowjs.EvalGlobal); err != nil {
			return err
		}
	}
	return nil
}

// room is what the output of a call on vm may take.
func room(vm *slowjs.VM) int64 {
	if r := vmhook.Room(vm); r >= 0 {
		return r
	}
	return budget(maxOutput, 0)
}

// formatPrintf makes fmt.Sprintf(format, a...) for a call on vm, and the
// function that gives back its memory once the string is no longer used.
// It is false, with the error that ends the call pending, when the call is
// to end or the output would not fit.
func formatPrintf(vm *slowjs.VM, format string, a []any) (string, func(), bool) {
	r := room(vm)
	n, read, ok, stopped := sizePrintfStop(format, a, r, func() bool { return vmhook.Stop(vm) })
	if stopped {
		return "", nil, false
	}

	var more int64
	if read != format {
		more = int64(len(read))
	}
	if !ok || budget(n, more) > r {
		vmhook.OutOfMemory(vm)
		return "", nil, false
	}

	return printfOf(vm, n, read, a)
}

// formatPrint is formatPrintf for fmt.Sprint, or fmt.Sprintln when ln is
// true.
func formatPrint(vm *slowjs.VM, a []any, ln bool) (string, func(), bool) {
	n := sizePrint(a)
	if budget(n, 0) > room(vm) {
		vmhook.OutOfMemory(vm)
		return "", nil, false
	}

	return printOf(vm, n, a, ln)
}

// printfOf makes fmt.Sprintf(format, a...), whose output is at most n bytes:
// in one go when that is short, else a piece at a time, which stops when
// the call on vm is to end. vm may be nil, for the tests.
func printfOf(vm *slowjs.VM, n int64, format string, a []any) (string, func(), bool) {
	if n <= direct {
		return fmt.Sprintf(format, a...), func() {}, true
	}
	return sprintf(vm, n, format, a)
}

// printOf is printfOf for fmt.Sprint, or fmt.Sprintln when ln is true.
func printOf(vm *slowjs.VM, n int64, a []any, ln bool) (string, func(), bool) {
	switch {
	case n > direct && ln:
		return sprintln(vm, n, a)
	case n > direct:
		return sprint(vm, n, a)
	case ln:
		return fmt.Sprintln(a...), func() {}, true
	}
	return fmt.Sprint(a...), func() {}, true
}

// write writes s to w a piece at a time, and stops between two pieces when
// the call on vm is to end. It returns the bytes written.
func write(vm *slowjs.VM, w io.Writer, s string) (int, error) {
	// The bytes may be given back once the call ends: w gets a slice it
	// must not keep, as an io.Writer, rather than a string it could.
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	n := 0
	for len(b) > 0 {
		k := min(len(b), writePiece)
		m, err := w.Write(b[:k])
		n += m
		if err != nil {
			return n, err
		}

		b = b[k:]
		if len(b) > 0 && vmhook.Stop(vm) {
			return n, nil
		}
	}
	return n, nil
}

// newString is s as a javascript string, made before the memory of s is
// given back.
func newString(vm *slowjs.VM, s string) slowjs.Value {
	v, err := vm.NewString(s)
	if err != nil {
		if !vmhook.Stop(vm) {
			vmhook.OutOfMemory(vm)
		}
		return slowjs.Value{}
	}
	return v
}
