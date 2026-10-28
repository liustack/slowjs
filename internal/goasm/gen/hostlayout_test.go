// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"go/format"
	"strings"
	"testing"
)

// layoutAsm is llc's text of js_host_layout with the values vs, written as
// llc writes them: a run of zeros as .zero.
func layoutAsm(vs ...string) string {
	return "\t.section\t.rodata,\"a\",@progbits\n\t.globl\tjs_host_layout\n\t.p2align\t4, 0x0\njs_host_layout:\n" +
		strings.Join(vs, "") + "\t.size\tjs_host_layout, 112\n"
}

func TestHostLayout(t *testing.T) {
	var want []int64
	var lines []string
	for i := range HostLayoutNames {
		v := int64(i*3 - 4)
		if i == 1 {
			v = 0x7fffffff
		}
		if i == 2 {
			v = 0x80000000
		}
		if i == 3 {
			v = 0
		}
		want = append(want, v)
		if v == 0 {
			lines = append(lines, "\t.zero\t8\n")
		} else {
			lines = append(lines, fmt.Sprintf("\t.quad\t%d\n", v))
		}
	}
	u, err := SplitAsm(layoutAsm(lines...), "#")
	if err != nil {
		t.Fatal(err)
	}
	got, err := HostLayout(u)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	decls := HostLayoutDecls(got)
	for _, l := range []string{"\tlayoutRefCountOffset = -4\n", "\tlayoutStringLenMask  = 2147483647\n", "\tatomThen             = "} {
		if !strings.Contains(decls, l) {
			t.Errorf("the constants have no %q:\n%s", l, decls)
		}
	}
	// engine_decl_<arch>.go is as gofmt writes it: the constants end with a blank
	// line before what follows them.
	src := "package cengine\n\n" + strings.TrimSuffix(decls, "\n")
	if f, err := format.Source([]byte(src)); err != nil || string(f) != src {
		t.Errorf("the constants are not as gofmt writes them (%v):\n%s", err, decls)
	}

	for _, c := range []struct{ name, asm, err string }{
		{"missing", "\t.section\t.rodata,\"a\",@progbits\nother:\n\t.quad\t1\n", "no js_host_layout in the engine"},
		{"short", layoutAsm("\t.quad\t1\n"), "js_host_layout has 1 entries, the generator names"},
		{"long", layoutAsm(append(lines, "\t.quad\t1\n")...), "has 15 entries"},
		{"not a quad", layoutAsm("\t.long\t1\n"), "is not a quad"},
		{"part of a quad", layoutAsm("\t.zero\t4\n"), "is not a run of whole quads"},
	} {
		u, err := SplitAsm(c.asm, "#")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := HostLayout(u); err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
