// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/liustack/slowjs/internal/gohost"
)

// The functions of the host of the tests, by id, and the VMs they run in, by
// the id of their host: the HostFunc of the tests finds both.
var (
	testHostFuncs = map[int32]func(v *VM, sp uintptr, args []gohost.Value) (u, tag uintptr){}
	testVMs       = map[uint32]*VM{}
)

func init() {
	SetHostFunc(func(sp uintptr, vm uint32, this uintptr, argc int32, argv uintptr, id int32) (uintptr, uintptr) {
		f, v := testHostFuncs[id], testVMs[vm]
		if f == nil || v == nil {
			panic(fmt.Sprintf("cengine test: host function %d of VM %d", id, vm))
		}
		return f(v, sp, unsafe.Slice((*gohost.Value)(addr(argv)), argc))
	})
}

// The tag of an int and the JSErrorKind of a RangeError (engine/quickjs.h).
const (
	tagInt         = 0
	errorKindRange = 1
)

func align16(p uintptr) uintptr { return (p + 15) &^ 15 }

// defineHostFunc makes a function of the host named name with the id and
// sets it as a global. It returns the exception's message when
// JS_NewHostFunction threw.
func defineHostFunc(v *VM, name string, length int, id int32) (thrown string) {
	v.enter(func() {
		n := v.cstring(name)
		fu, ftag := JS_NewHostFunction(v.sp, v.ctx, n, uintptr(len(name)), uintptr(length), uintptr(id))
		if int32(ftag) == tagException {
			thrown = v.exception().Error()
			return
		}
		gu, gtag := JS_GetGlobalObject(v.sp, v.ctx)
		JS_SetPropertyStr(v.sp, v.ctx, gu, gtag, v.cstring(name), fu, ftag)
		v.free(gu, gtag)
	})
	return thrown
}

// TestHostFunction: a script calls functions of the host made with
// JS_NewHostFunction, which reach the HostFunc registered with their id and
// arguments. They return numbers, throw with JS_ThrowErrorKind, make strings
// with JS_NewStringChars and the layout constants, and call back into the
// engine above the shadow pointer of their call, which leaves the frames of
// the call that was running as they were.
func TestHostFunction(t *testing.T) {
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	testVMs[v.Host().ID()] = v
	defer delete(testVMs, v.Host().ID())

	testHostFuncs[1] = func(v *VM, sp uintptr, args []gohost.Value) (uintptr, uintptr) {
		sum := int32(0)
		for _, a := range args {
			if int32(a.Tag) == tagInt {
				sum += int32(a.U)
			}
		}
		return uintptr(uint32(sum)), tagInt
	}
	testHostFuncs[2] = func(v *VM, sp uintptr, args []gohost.Value) (uintptr, uintptr) {
		msg := "bad " + strings.Repeat("x", 300)
		p := align16(sp)
		copy(bytesAt(p, uintptr(len(msg))), msg)
		return JS_ThrowErrorKind(align16(p+uintptr(len(msg))), v.ctx, errorKindRange, p, uintptr(len(msg)))
	}
	testHostFuncs[3] = func(v *VM, sp uintptr, args []gohost.Value) (uintptr, uintptr) {
		u, tag := JS_NewStringChars(align16(sp), v.ctx, 3, 0)
		if int32(tag) == tagException {
			return u, tag
		}
		copy(bytesAt(u+layoutStringChars, 3), "abc")
		return u, tag
	}
	testHostFuncs[4] = func(v *VM, sp uintptr, args []gohost.Value) (uintptr, uintptr) {
		// A call back into the engine: its entries start above sp, and
		// where they started before is put back after.
		old := v.sp
		v.sp = align16(sp)
		defer func() { v.sp = old }()
		var out string
		var err error
		v.enterScript(func() { out, err = v.eval(`var inner = [1, 2, 3].map(x => x * 10); inner.join("+")`, "inner.js") })
		if err != nil || out != "10+20+30" {
			panic(fmt.Sprintf("the inner eval gave %q, %v", out, err))
		}
		return uintptr(len(out)), tagInt
	}
	for _, f := range []struct {
		name   string
		length int
		id     int32
	}{{"add", 2, 1}, {"fail", 0, 2}, {"abc", 0, 3}, {"nested", 0, 4}} {
		if thrown := defineHostFunc(v, f.name, f.length, f.id); thrown != "" {
			t.Fatalf("%s: %s", f.name, thrown)
		}
	}

	for _, c := range []struct{ src, want string }{
		{`add(1, 2, 39)`, "42"},
		{`add()`, "0"},
		{`[add.name, add.length, typeof add.prototype, String(add).includes("[native code]")].join()`, "add,2,undefined,true"},
		{`try { new add(1) } catch (e) { e.constructor.name }`, "TypeError"},
		{`try { fail() } catch (e) { [e.constructor.name, e.message.length, e.message.slice(0, 5)].join() }`, "RangeError,255,bad x"},
		{`abc() + abc().length`, "abc3"},
		{`function outer(a, b) { var local = [a, b, a * b]; var r = nested(); return local.join() + ":" + r } outer(6, 7)`, "6,7,42:8"},
		{`var s = 0; for (var i = 0; i < 1000; i++) s += add(i, 1); s`, "500500"},
	} {
		if got, err := v.Eval(c.src, "host.js"); err != nil || got != c.want {
			t.Errorf("%s: %q, %v, want %q", c.src, got, err, c.want)
		}
	}
	if v.sp != v.base+scratchSize {
		t.Errorf("the entries start at %#x after the calls, want %#x", v.sp, v.base+scratchSize)
	}

	// An id below 0 is a request of the engine: JS_NewHostFunction refuses
	// it with a RangeError.
	if thrown := defineHostFunc(v, "bad", 0, -1); !strings.Contains(thrown, "RangeError") {
		t.Errorf("JS_NewHostFunction of id -1 threw %q, want a RangeError", thrown)
	}
}
