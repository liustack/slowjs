// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command jsrun runs scripts on the Go engine as engine/test/jsrun.c runs
// them on the C tree, and prints what they give in the same form. Its output
// is what engine/test/compare.sh compares the C tree with when COMPARE_GO=1
// asks for it. The repository holds files for the C tree to be compared with
// when the Go engine is gone.
//
//	go run ./internal/jsrun [-regexp-cache=N,BYTES] FILE...
//
// Each file runs as a global script in a VM of its own, then the jobs it
// queued run. jsrun prints "== " and the name of the file, then what the
// script gave: the string of its completion value, or of what that value
// returns when it is a function (called after the jobs), or the exception.
// The scripts find the Builtins GetAsyncContext and SetAsyncContext as the
// globals getAsyncContext and setAsyncContext.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/liustack/slowjs"
)

// regexpCache is the setting of -regexp-cache: the entries and the bytes, or
// nil for the default of the VM.
var regexpCache []int

// formatSrc is what both runners call with the completion value. A lone
// surrogate is printed as \u{...}: Go would turn it into U+FFFD.
const formatSrc = `(v) => {
  const s = typeof v === 'function' ? String(v()) : String(v)
  if (s.isWellFormed()) return s
  let r = ''
  for (const c of s) {
    const u = c.charCodeAt(0)
    r += c.length === 1 && u >= 0xd800 && u <= 0xdfff ? '\\u{' + u.toString(16) + '}' : c
  }
  return r
}`

func main() {
	args := os.Args[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && len(args[0]) > 1 {
		v, ok := strings.CutPrefix(args[0], "-regexp-cache=")
		n, bytes, ok2 := strings.Cut(v, ",")
		if !ok || !ok2 {
			fmt.Fprintf(os.Stderr, "jsrun: unknown option %s\n", args[0])
			os.Exit(2)
		}
		a, err1 := strconv.Atoi(n)
		b, err2 := strconv.Atoi(bytes)
		if err1 != nil || err2 != nil {
			fmt.Fprintf(os.Stderr, "jsrun: %s: want N,BYTES\n", args[0])
			os.Exit(2)
		}
		regexpCache = []int{a, b}
		args = args[1:]
	}
	for _, path := range args {
		fmt.Printf("== %s\n", filepath.Base(path))
		out, err := run(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "jsrun:", err)
			os.Exit(1)
		}
		fmt.Print(out)
	}
}

func run(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	vm, err := slowjs.NewVM()
	if err != nil {
		return "", err
	}
	defer vm.Close()
	if err := defineGlobals(vm); err != nil {
		return "", err
	}
	if regexpCache != nil {
		if err := vm.SetRegExpCache(regexpCache[0], regexpCache[1]); err != nil {
			return "", err
		}
	}
	v, err := vm.EvalValue(string(src), slowjs.EvalGlobal)
	if err != nil {
		return fmt.Sprintf("<exception=%s>\n", err), nil
	}
	defer v.Free()
	out := ""
	if _, err := vm.ExecutePendingJobs(); err != nil {
		out = fmt.Sprintf("<job exception=%s>\n", err)
	}
	format, err := vm.EvalValue(formatSrc, slowjs.EvalGlobal)
	if err != nil {
		return out + fmt.Sprintf("<format=%s>\n", err), nil
	}
	defer format.Free()
	s, err := format.Call(slowjs.UndefinedValue, v)
	if err != nil {
		return out + fmt.Sprintf("<exception=%s>\n", err), nil
	}
	str, ok := s.(string)
	if !ok {
		return "", fmt.Errorf("%s: the format gave %T", path, s)
	}
	return out + str + "\n", nil
}

// defineGlobals gives the global object of vm what engine/test/jsrun.c
// gives it.
func defineGlobals(vm *slowjs.VM) error {
	define, err := vm.EvalValue(`(get, set) => {
  globalThis.getAsyncContext = get
  globalThis.setAsyncContext = set
}`, slowjs.EvalGlobal)
	if err != nil {
		return err
	}
	defer define.Free()
	get, err := vm.NewBuiltinFunction(slowjs.GetAsyncContext)
	if err != nil {
		return err
	}
	defer get.Free()
	set, err := vm.NewBuiltinFunction(slowjs.SetAsyncContext)
	if err != nil {
		return err
	}
	defer set.Free()
	r, err := define.CallValue(slowjs.UndefinedValue, get, set)
	if err != nil {
		return err
	}
	r.Free()
	return nil
}
