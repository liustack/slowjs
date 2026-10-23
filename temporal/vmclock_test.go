// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

import (
	"fmt"
	"github.com/liustack/slowjs"
	"testing"
	"time"
)

func TestVMClock(t *testing.T) {
	vm := newVM(t)
	at := time.Unix(1700000000, 123456789)
	vm.SetClock(func() time.Time { return at })
	// A saved native function and an Intl formatter created at module load must
	// continue reading the VM's current clock after it changes.
	code, err := vm.Compile(`const now = Date.now; const f = new Intl.DateTimeFormat('en-US', {timeZone:'UTC', dateStyle:'full', timeStyle:'long'});
 globalThis.readClock = () => [now(), new Date().getTime(), Date() === new Date(now()).toString(), Temporal.Now.instant().epochMilliseconds, f.format() === f.format(now()), JSON.stringify(f.formatToParts()) === JSON.stringify(f.formatToParts(now())), String(Temporal.Now.instant().epochNanoseconds)].join('|');
 globalThis.loadedClock = readClock();`, slowjs.EvalModule)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = vm.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}
	check := func(expr string) {
		t.Helper()
		want := fmt.Sprintf("%d|%d|true|%d|true|true|%d", at.UnixMilli(), at.UnixMilli(), at.UnixMilli(), at.UnixNano())
		got, err := vm.Eval(expr, slowjs.EvalGlobal)
		if err != nil || got != want {
			t.Fatalf("%s = %v, %v, want %s", expr, got, err, want)
		}
	}
	check("loadedClock")
	for _, next := range []time.Time{time.Unix(-1, 999999999), time.Unix(1800000000, 987654321)} {
		at = next
		check("readClock()")
		if _, err := vm.Eval(`Promise.resolve().then(() => { globalThis.laterClock = readClock() })`, slowjs.EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := vm.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		check("laterClock")
	}
	vm.SetClock(nil)
	before := time.Now().UnixMilli()
	got, err := vm.Eval(`Date.now()`, slowjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	n := got.(float64)
	if n < float64(before) || n > float64(time.Now().UnixMilli()) {
		t.Fatalf("reset clock = %v", got)
	}
}

func TestVMClockConcurrent(t *testing.T) {
	for i := range 16 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			vm := newVM(t)
			at := time.Unix(int64(i)*86400, 123456789)
			vm.SetClock(func() time.Time { return at })
			for range 10 {
				evalTests(t, vm, [][2]string{{fmt.Sprintf(`Date.now() === %d && new Date().getTime() === %d && Temporal.Now.instant().epochMilliseconds === %d && new Intl.DateTimeFormat('en', {timeZone:'UTC'}).format() === new Intl.DateTimeFormat('en', {timeZone:'UTC'}).format(%d)`, at.UnixMilli(), at.UnixMilli(), at.UnixMilli(), at.UnixMilli()), "true"}})
			}
		})
	}
}

func TestVMClockTemporalOverride(t *testing.T) {
	vm := newVM(t, Clock(func() time.Time { return time.Unix(2, 0) }))
	vm.SetClock(func() time.Time { return time.Unix(1, 0) })
	evalTests(t, vm, [][2]string{{`Date.now()`, "1000"}, {`Temporal.Now.instant().epochMilliseconds`, "2000"}})
}
