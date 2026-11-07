// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fmt exposes some functionality of the Go stdlib fmt package to a
// [quickjs.VM].
package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"fmt"

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
// [quickjs.VM.SetMemoryLimit]) and the bound is more than the VM has left
// under it, the call ends with the uncatchable out of memory error and makes
// nothing. The bound is at most five times the output, more when a precision
// cuts a string short, so a call whose output fits in a fraction of what is
// left runs as before.
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
			if !fits(vm, sizePrintf(format, a)) {
				return 0, nil
			}
			return fmt.Printf(format, a...)
		}, false},
		{"println", func(a ...any) (int, error) {
			if !fits(vm, sizePrint(a)) {
				return 0, nil
			}
			return fmt.Println(a...)
		}, false},
		{"sprint", func(a ...any) string {
			if !fits(vm, sizePrint(a)) {
				return ""
			}
			return fmt.Sprint(a...)
		}, false},
		{"sprintf", func(format string, a ...any) string {
			if !fits(vm, sizePrintf(format, a)) {
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

// fits reports whether an output of n bytes fits in what vm has left under
// its memory limit. When it does not, it throws the out of memory error,
// which ends the call: what the function returns then is dropped.
func fits(vm *quickjs.VM, n int64) bool {
	if room := vmhook.Room(vm); room >= 0 && n > room {
		vmhook.OutOfMemory(vm)
		return false
	}
	return true
}
