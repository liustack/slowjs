// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackedGo runs the Go functions PackedGo writes against a Go stand-in
// for the packed entry: each argument must arrive in the block at its slot,
// the call's shadow pointer above the block, and the result come back. A
// block that would end past the region panics, and a signature with an
// aggregate by copy or an unknown type gets no function.
func TestPackedGo(t *testing.T) {
	var src strings.Builder
	src.WriteString("package main\n\nimport (\n\t\"fmt\"\n\t\"unsafe\"\n)\n\nconst shadowSizeLog2 = 16\n\n" + PackedSlot)
	for _, c := range []struct{ name, was string }{
		{"seven", "iijij>i"}, // the shadow pointer, then 6 words: 48 bytes
		{"mixed", "idnj>j"},  // 32 bytes
		{"none", "ii>v"},
	} {
		s, ok, err := PackedGo(c.name, c.name+PackedEntry, c.was)
		if err != nil || !ok {
			t.Fatalf("%s: %v %v", c.name, ok, err)
		}
		src.WriteString("\n" + s)
	}
	src.WriteString(`
func word(a uintptr) uintptr { return *(*uintptr)(packedSlot(a)) }

func seven_block(sp, blk uintptr) uintptr {
	if sp != blk+48 {
		panic("seven: shadow pointer")
	}
	var s uintptr
	for k := uintptr(0); k < 6; k++ {
		s = s*10 + word(blk+8*k)
	}
	return s
}

func mixed_block(sp, blk uintptr) (uintptr, uintptr) {
	if sp != blk+32 {
		panic("mixed: shadow pointer")
	}
	return uintptr(*(*float64)(packedSlot(blk))) + word(blk+8), word(blk+16)*word(blk+24)
}

var noneSeen uintptr

func none_block(sp, blk uintptr) { noneSeen = word(blk) + sp - blk }

func main() {
	region := make([]uintptr, 1<<16/8*2)
	base := (uintptr(unsafe.Pointer(&region[0])) + 1<<16 - 1) &^ (1<<16 - 1)
	fmt.Println(seven(base, 1, 2, 3, 4, 5, 6))
	r0, r1 := mixed(base+64, 2.5, 3, 4, 5)
	fmt.Println(r0, r1)
	none(base, 7)
	fmt.Println(noneSeen)
	defer func() { fmt.Println("panic:", recover()) }()
	seven(base+1<<16-48, 1, 2, 3, 4, 5, 6)
}
`)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module packed\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	want := "123456\n5 20\n23\npanic: cengine: the block of seven ends past the shadow-stack region\n"
	if string(out) != want {
		t.Fatalf("got\n%s\nwant\n%s\nsource:\n%s", out, want, src.String())
	}
	for _, was := range []string{"iB>i", "if>i", "ii>f", "ii>?"} {
		if _, ok, err := PackedGo("x", "x_block", was); ok || err != nil {
			t.Errorf("%s: a function (%v, %v)", was, ok, err)
		}
	}
	for _, was := range []string{"", "d>i", "ii"} {
		if _, _, err := PackedGo("x", "x_block", was); err == nil {
			t.Errorf("%q: no error", was)
		}
	}
}
