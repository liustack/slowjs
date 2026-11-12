// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// regvmApp is TestRegvmRun's shop and mui: every page of a tifa app, from
// REGVM_APPS (the folder that holds prelude/fakeintl.js, APP/bundles/APP.js
// and APP/in/PAGE.json), with tifa's four functions, in one VM. A render is
// one of each page, in the order of their names, as tifa makes it:
// startRender(json), the jobs, renderResult. One warm render, then n. It
// prints the bytes and a sum of the HTML of all the pages, which the old and
// the register loop must give the same.
func regvmApp(t *testing.T, on bool, n int, app string) {
	dir := os.Getenv("REGVM_APPS")
	if dir == "" {
		t.Skip("REGVM_APPS not set")
	}
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	m := newTestVM(t)
	regvm(m, on)
	for _, err := range []error{m.RegisterEscapeText("__tifa_escape"), m.RegisterStartTag("__tifa_start_tag"),
		m.RegisterEndTag("__tifa_end_tag"), m.RegisterCopyProps("__tifa_copy_props")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"prelude/fakeintl.js", app + "/bundles/" + app + ".js"} {
		if _, err := m.Eval(read(p), EvalGlobal); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	names, err := filepath.Glob(filepath.Join(dir, app, "in", "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no pages in %s/%s/in: %v", dir, app, err)
	}
	sort.Strings(names)
	inputs := make([]string, len(names))
	for i, p := range names {
		inputs[i] = read(app + "/in/" + filepath.Base(p))
	}
	g := m.GlobalObject()
	rr, err := m.NewAtom("renderResult")
	if err != nil {
		t.Fatal(err)
	}
	st, err := m.NewAtom("startRender")
	if err != nil {
		t.Fatal(err)
	}
	start, err := g.GetPropertyValue(st)
	if err != nil {
		t.Fatal(err)
	}
	render := func() string {
		var all strings.Builder
		for _, json := range inputs {
			if _, err := start.Call(g, json); err != nil {
				t.Fatal(err)
			}
			if _, err := m.ExecutePendingJobs(); err != nil {
				t.Fatal(err)
			}
			r, err := g.Property(rr)
			if err != nil {
				t.Fatal(err)
			}
			s, ok := r.(string)
			if !ok {
				t.Fatalf("no renderResult after the jobs ran: %T", r)
			}
			if err := g.SetProperty(rr, Undefined{}); err != nil {
				t.Fatal(err)
			}
			all.WriteString(s)
		}
		return all.String()
	}
	html := render()
	times := make([]time.Duration, n)
	for i := range n {
		t0 := time.Now()
		if h := render(); len(h) != len(html) {
			t.Fatal("the HTML changed between renders")
		}
		times[i] = time.Since(t0)
	}
	fmt.Printf("regvm %s on=%v n=%d pages=%d result=%d sum=%x converted=%d refused=%d entries=%d ns=", app, on, n, len(inputs), len(html), sha256.Sum256([]byte(html)), regvmStat(m, 0), regvmStat(m, 1), regvmStat(m, 2))
	for _, d := range times {
		fmt.Printf("%d ", d.Nanoseconds())
	}
	fmt.Println()
}
