// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"strings"
	"testing"
	"unsafe"
)

// stackAddress returns the address of a local of its own frame: called
// twice from the same frame, it differs only when the goroutine stack moved.
//
//go:noinline
func stackAddress() uintptr {
	var b [1]byte
	return uintptr(unsafe.Pointer(&b))
}

// TestStackGrowthInDeepRecursion renders HTML through deep JS recursion on a
// new goroutine, which starts with a small stack. Go must grow (and so move)
// that stack while interpreter frames are live, in the middle of the
// recursion. On linux/amd64, unless slowjs_nogoasm is given, the frames are
// those of the assembly interpreter: a frame must hold no Go stack address for the copy to be
// right. Both JS calling JS directly and JS called back from a native
// function (Array.prototype.map) are covered.
func TestStackGrowthInDeepRecursion(t *testing.T) {
	const depth = 1500
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	const js = `
function tree(d) { return d === 0 ? { tag: 'span', children: [] } : { tag: 'div', children: [tree(d - 1)] } }
function direct(n) { return n.children.length === 0 ? '<' + n.tag + '></' + n.tag + '>' : '<' + n.tag + '>' + direct(n.children[0]) + '</' + n.tag + '>' }
function mapped(n) { return '<' + n.tag + '>' + n.children.map(mapped).join('') + '</' + n.tag + '>' }
const t = tree(DEPTH);
direct(t) + '|' + mapped(t)`
	type result struct {
		v     any
		err   error
		moved bool
	}
	done := make(chan result)
	go func() {
		before := stackAddress()
		v, err := m.Eval(strings.Replace(js, "DEPTH", "1500", 1), EvalGlobal)
		done <- result{v, err, stackAddress() != before}
	}()
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	html := strings.Repeat("<div>", depth) + "<span></span>" + strings.Repeat("</div>", depth)
	if r.v != html+"|"+html {
		s, _ := r.v.(string)
		t.Fatalf("wrong HTML of %d bytes, want %d", len(s), 2*len(html)+1)
	}
	if !r.moved {
		t.Fatal("the goroutine stack did not move: the test no longer forces growth")
	}
}
