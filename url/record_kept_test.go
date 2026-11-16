// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"strconv"
	"strings"
	"testing"

	"github.com/liustack/slowjs"
)

// countFromParts sets fromPartsHook for the test and returns the count of
// records made again from their parts since.
func countFromParts(t *testing.T) *int {
	t.Helper()
	n := new(int)
	fromPartsHook = func() { *n++ }
	t.Cleanup(func() { fromPartsHook = nil })
	return n
}

// TestSetTakesTheLastRecord checks that a setter on the URL whose record the
// VM made last takes that record, and that a setter on any other URL makes
// the record again from its parts, with the same results.
func TestSetTakesTheLastRecord(t *testing.T) {
	n := countFromParts(t)
	vm := newVM(t)
	long := strings.Repeat("a", maxKeptBytes)
	for _, c := range []struct {
		js   string
		want int
	}{
		// a router's link: a parse, then a setter or two on it
		{`var u = new URL("/p?q#h", "http://localhost"); u.pathname = "/zh" + u.pathname; u.search = "?r"; u.href`, 0},
		// a setter on a URL made before the last one
		{`var a = new URL("http://a/x"), b = new URL("http://b/y"); a.pathname = "/z"; a.href + " " + b.href`, 1},
		// then on b, whose record a's setter did not make
		{`b.hash = "#k"; b.href`, 1},
		// a setter that fails leaves the URL and the kept record as they were
		{`var c = new URL("http://c/"); c.port = "x"; c.protocol = "a b"; c.port = "81"; c.href`, 0},
		// a record whose href is longer than what is kept
		{`var d = new URL("http://d/` + long + `"); d.hash = "#1"; d.hash = "#2"; d.hash`, 2},
		// a record a setter made from parts the parser does not make
		{`var f = new URL("http://localhost/C|/x"); f.protocol = "file"; f.search = "?s"; f.host + " " + f.pathname + " " + f.href`, 0},
	} {
		*n = 0
		evalString(t, vm, c.js)
		if *n != c.want {
			t.Errorf("%s: %d records made from parts, want %d", c.js, *n, c.want)
		}
	}
	for js, want := range map[string]string{
		`u.href`:                "http://localhost/zh/p?r#h",
		`a.href + " " + b.href`: "http://a/z http://b/y#k",
		`c.href`:                "http://c:81/",
		`d.hash`:                "#2",
		`f.host + " " + f.pathname + " " + f.href`: "localhost /C|/x file://localhost/C|/x?s",
	} {
		if got := evalString(t, vm, js); got != want {
			t.Errorf("%s: %q, want %q", js, got, want)
		}
	}
}

// keptPerVM is the Go heap that each of 100 VMs holds after it ran js, which
// makes URLs: what the package keeps for it, and what any VM holds.
func keptPerVM(t *testing.T, js string) int64 {
	t.Helper()
	const n = 100
	vms := make([]*slowjs.VM, 0, n)
	defer func() {
		for _, vm := range vms {
			vm.Close()
		}
	}()
	before := heapAfterGC()
	for range n {
		vm, err := slowjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		vms = append(vms, vm)
		if err := Register(vm); err != nil {
			t.Fatal(err)
		}
		evalString(t, vm, js)
	}
	return (int64(heapAfterGC()) - int64(before)) / n
}

// TestKeptIsBounded checks that what a VM keeps stays under keptMost however
// long the inputs that made it were: a short href made from a long path that
// ".." took back, a short query after a long path in the buffer of the
// parser, a long base, setters that made a long path and then a short one,
// and bases and records that keep the most the limits allow. It counts the Go
// heap each VM holds against VMs that made no URL.
func TestKeptIsBounded(t *testing.T) {
	none := keptPerVM(t, `""`)
	long := `"a".repeat(60000)`
	limits := strings.NewReplacer("maxBase", strconv.Itoa(maxBaseBytes), "maxKept", strconv.Itoa(maxKeptBytes))
	for _, js := range []string{
		`new URL("/x?q#f", "http://h/").href`,
		`new URL("http://h/" + ` + long + ` + "/..").href`,
		`new URL("http://h/" + ` + long + ` + "/..?q#f").href`,
		`new URL("http://h/" + "a".repeat(30000) + "/../x/" + "b".repeat(30000) + "/..?q#f").href`,
		`new URL("y", "http://h/" + ` + long + ` + "/../").href`,
		`var u = new URL("http://h/"); u.pathname = "/" + ` + long + `; u.pathname = "/z/" + ` + long + ` + "/.."; u.href`,
		`var v = new URL("http://h/x"); v.hash = "#" + ` + long + `; v.hash = "#a"; v.search = "?" + "%".repeat(300); v.search = "?b"; v.href`,
		// a base of 256 bytes whose path is 740 bytes, and a record of 1,001
		// bytes whose path is 992 bytes in a buffer of 2,048
		`URL.canParse("http://x/", "http://h/" + "\x01".repeat(246) + "/");
			new URL("http://h/" + "a".repeat(991) + "/" + "b".repeat(800) + "/..").href`,
		// the most a base and a record keep: an href at each limit, a path
		// left in a buffer of about twice its length, a long query and fragment
		`URL.canParse("http://x/", "http://h/" + "\x01".repeat((maxBase - 10) / 3) + "/");
			new URL("http://h/" + "a".repeat(maxKept - 11) + "/" + "b".repeat(maxKept) + "/..").href`,
		`URL.canParse("http://x/", "http://h/" + "a".repeat(maxBase - 9));
			new URL("http://h/" + "a".repeat(maxKept / 4) + "/" + "b".repeat(maxKept) + "/..?" + "c".repeat(maxKept / 2) + "#" + "d".repeat(maxKept / 4 - 20)).href`,
		`URL.canParse("http://x/", "sc:" + "a".repeat(maxBase - 3));
			new URL("sc:" + "a".repeat(maxKept / 2) + "?" + "b".repeat(maxKept / 2 - 10)).href`,
	} {
		js = limits.Replace(js)
		n := keptPerVM(t, js) - none
		t.Logf("%.60q: %d", js, n)
		if n >= keptMost {
			t.Errorf("%.70s: each VM holds %d bytes more", js, n)
		}
	}
}
