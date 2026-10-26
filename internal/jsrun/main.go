// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command jsrun runs scripts on the Go engine as engine/test/jsrun.c runs
// them on the C tree, and prints what they give in the same form. Its output
// is the reference that engine/test/compare.sh compares the C tree with.
//
//	go run ./internal/jsrun FILE...
//
// Each file runs as a global script in a VM of its own, then the jobs it
// queued run. jsrun prints "== " and the name of the file, then what the
// script gave: the string of its completion value, or of what that value
// returns when it is a function (called after the jobs), or the exception.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/liustack/slowjs"
)

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
	for _, path := range os.Args[1:] {
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
