// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !cengine

package main

import (
	"fmt"
	"time"

	"github.com/liustack/slowjs"
)

const engineName = "slowjs (package slowjs)"

// timedWhat is what govm.eval does.
const timedWhat = "Eval, which copies the source into the VM and runs the script, then ExecutePendingJobs for its jobs and the result as a string"

type govm struct{ m *slowjs.VM }

// newVM ignores the seed and the stack size, which package slowjs does not
// take.
func newVM(o options) (vm, error) {
	m, err := slowjs.NewVM()
	if err != nil {
		return nil, err
	}
	if !o.clock.IsZero() {
		t := o.clock
		m.SetClock(func() time.Time { return t })
	}
	return govm{m}, nil
}

func (g govm) eval(src, file string) (string, error) {
	r, err := g.m.Eval(src, slowjs.EvalGlobal)
	if err != nil {
		return "", err
	}
	if _, err := g.m.ExecutePendingJobs(); err != nil {
		return "", err
	}
	return fmt.Sprint(r), nil
}

func (g govm) gc() { g.m.RunGC() }

func (g govm) release(keep uintptr) (string, error) {
	return "package slowjs gives no memory back", nil
}

func (g govm) memory() string { return fmt.Sprintf("MemorySize %d", g.m.MemorySize()) }

func (g govm) close() (string, error) { return "closed", g.m.Close() }
