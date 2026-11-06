// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPostLinkFormats runs the post-link check on a binary of
// testdata/check/mini.c for every target the module builds for, each built as
// go build makes it by default: ELF for linux (amd64 and arm64, linked at a
// fixed address), Mach-O for darwin (amd64 and arm64) and PE for windows
// (amd64), the last three position independent. The check must pass on each,
// and on the position-independent ones it must have held every pointer of
// engine_rodata to its fixup. Then it changes each position-independent
// binary one fault at a time, and the check must refuse each: a byte of a
// generated function, a pointer of engine_rodata, and the fixup of that
// pointer, without which the loader would leave it at the address of the
// file. It needs LLVM 22.1.8 (QUICKJS_LLVM), the built pass
// (QUICKJS_SHADOW_PASS) and the go tool, and runs on any host: the binaries
// are cross-built.
func TestPostLinkFormats(t *testing.T) {
	llvm, pass := os.Getenv("QUICKJS_LLVM"), os.Getenv("QUICKJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set QUICKJS_LLVM and QUICKJS_SHADOW_PASS")
	}
	work := t.TempDir()
	drv := filepath.Join(work, "engine")
	build := exec.Command("go", "build", "-o", drv, "./cmd/engine")
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	src, err := filepath.Abs(filepath.Join("testdata", "check", "mini.c"))
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) { postLinkFormatsArch(t, work, drv, llvm, pass, src, arch) })
	}
}

func postLinkFormatsArch(t *testing.T, work, drv, llvm, pass, src, arch string) {
	mod := filepath.Join(work, "mod-"+arch)
	pkgDir := filepath.Join(mod, "mini")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"go.mod":  "module checkmini\n\ngo 1.22\n",
		"main.go": "package main\n\nimport _ \"checkmini/mini\"\n\nfunc main() {}\n",
		"mini/host.go": "package mini\n\nfunc goasmMissing(i int64) { panic(missingNames[i]) }\n\n" +
			"//go:noinline\nfunc pollWork(sp uintptr) uintptr { return sp*3 + 1 }\n\nvar pollSink uintptr\n\n" +
			"// host_poll is called by the polls of the loops: it has a stack check.\n" +
			"func host_poll(sp uintptr) { pollSink = pollWork(sp) }\n",
	} {
		if err := os.WriteFile(filepath.Join(mod, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gen := func(extra ...string) (string, error) {
		args := []string{"-src", src, "-llvm", llvm, "-pass", pass, "-arch", arch, "-mode", "pn", "-keep-going",
			"-leaves", "leafmix,leafwrap", "-work", filepath.Join(work, "w-"+arch), "-pkg", "mini"}
		out, err := exec.Command(drv, append(args, extra...)...).CombinedOutput()
		return string(out), err
	}
	if out, err := gen("-out", pkgDir); err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	check := func(bin string) (string, error) {
		return gen("-verify-bin", bin, "-verify-pkg", "checkmini/mini", "-verify-all")
	}
	systems := []string{"linux", "darwin", "windows"}
	if arch == "arm64" {
		systems = systems[:2] // the module does not build for windows/arm64
	}
	format := map[string]string{"linux": "ELF", "darwin": "Mach-O", "windows": "PE"}
	for _, goos := range systems {
		bin := filepath.Join(work, goos+"-"+arch)
		b := exec.Command("go", "build", "-tags", "goasmkeep", "-o", bin, ".")
		b.Dir = mod
		b.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+arch, "GOFLAGS=")
		if out, err := b.CombinedOutput(); err != nil {
			t.Fatalf("%s: go build: %v\n%s", goos, err, out)
		}
		out, err := check(bin)
		if err != nil || !strings.Contains(out, "post-link check passed") {
			t.Fatalf("%s: the check rejects correct code: %v\n%s", goos, err, out)
		}
		if want := "binary: " + format[goos] + " for " + goos; !strings.Contains(out, want) {
			t.Fatalf("%s: %q missing from the report:\n%s", goos, want, out)
		}
		pie := goos != "linux"
		if strings.Contains(out, "pointers the loader slides") != pie {
			t.Fatalf("%s: the report says pointers the loader slides: %v, want %v\n%s", goos, !pie, pie, out)
		}
		t.Logf("%s/%s: %s", goos, arch, reportLines(out))
		if !pie {
			continue
		}
		for _, fault := range []struct {
			name, want string
			edit       func(t *testing.T, bin string, b []byte)
		}{
			{"a byte of a function", "fib", func(t *testing.T, bin string, b []byte) {
				at := binFileOffset(t, bin, binSymbol(t, bin, arch, "checkmini/mini.fib")+24)
				b[at] ^= 0xff
			}},
			{"a pointer of engine_rodata", "the pointer is", func(t *testing.T, bin string, b []byte) {
				at := binFileOffset(t, bin, firstRodataPointer(t, bin, arch))
				binary.LittleEndian.PutUint64(b[at:], binary.LittleEndian.Uint64(b[at:])+8)
			}},
			{"the fixup of a pointer", "has no fixup", func(t *testing.T, bin string, b []byte) {
				dropFixup(t, bin, b, firstRodataPointer(t, bin, arch))
			}},
		} {
			raw, err := os.ReadFile(bin)
			if err != nil {
				t.Fatal(err)
			}
			fault.edit(t, bin, raw)
			bad := bin + "-bad"
			if err := os.WriteFile(bad, raw, 0o755); err != nil {
				t.Fatal(err)
			}
			out, err := check(bad)
			if err == nil || strings.Contains(out, "post-link check passed") || !strings.Contains(out, fault.want) {
				t.Errorf("%s/%s, %s: not refused with %q: %v\n%s", goos, arch, fault.name, fault.want, err, lastLine(out))
			} else {
				t.Logf("%s/%s, %s: %s", goos, arch, fault.name, lastLine(out))
			}
		}
	}
}

func reportLines(out string) string {
	var r []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "binary: ") || strings.HasPrefix(l, "checked: ") {
			r = append(r, l)
		}
	}
	return strings.Join(r, "; ")
}

func openTestBin(t *testing.T, bin, arch string) *binFile {
	t.Helper()
	b, err := openBin(bin, arch)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

// binSymbol is the address of the ABI0 entry of a function in bin.
func binSymbol(t *testing.T, bin, arch, name string) uint64 {
	t.Helper()
	s, ok := openTestBin(t, bin, arch).abi0(name[:strings.LastIndex(name, ".")], name[strings.LastIndex(name, ".")+1:])
	if !ok {
		t.Fatalf("%s not in %s", name, bin)
	}
	return s.Value
}

// firstRodataPointer is the address of the first slot of engine_rodata the
// loader slides.
func firstRodataPointer(t *testing.T, bin, arch string) uint64 {
	t.Helper()
	b := openTestBin(t, bin, arch)
	s, ok := b.syms["checkmini/mini."+RodataGo]
	if !ok {
		t.Fatalf("no engine_rodata in %s", bin)
	}
	first := uint64(0)
	for _, a := range b.fixAt {
		if a >= s.Value && a < s.Value+s.Size && b.fixups[a].slide {
			first = a
			break
		}
	}
	if first == 0 {
		t.Fatalf("no pointer the loader slides in engine_rodata of %s", bin)
	}
	return first
}

// binFileOffset is where the byte at addr is in the file bin.
func binFileOffset(t *testing.T, bin string, addr uint64) int {
	t.Helper()
	if f, err := macho.Open(bin); err == nil {
		defer f.Close()
		for _, s := range f.Sections {
			if s.Offset != 0 && addr >= s.Addr && addr < s.Addr+s.Size {
				return int(s.Offset) + int(addr-s.Addr)
			}
		}
	} else if f, err := pe.Open(bin); err == nil {
		defer f.Close()
		base := f.OptionalHeader.(*pe.OptionalHeader64).ImageBase
		for _, s := range f.Sections {
			lo := base + uint64(s.VirtualAddress)
			if addr >= lo && addr < lo+uint64(min(int(s.VirtualSize), int(s.Size))) {
				return int(s.Offset) + int(addr-lo)
			}
		}
	}
	t.Fatalf("%#x is not in the file %s", addr, bin)
	return 0
}

// dropFixup takes the fixup of the pointer at addr out of the file b: in
// Mach-O the rebase of it becomes a step over it (the Go linker writes one
// rebase of one pointer after each segment and offset), in PE its entry of the
// base relocation table becomes padding.
func dropFixup(t *testing.T, bin string, b []byte, addr uint64) {
	t.Helper()
	if f, err := macho.Open(bin); err == nil {
		defer f.Close()
		var segs []*macho.Segment
		for _, l := range f.Loads {
			if s, ok := l.(*macho.Segment); ok {
				segs = append(segs, s)
			}
			r := l.Raw()
			if binary.LittleEndian.Uint32(r) != lcDyldInfoOnly {
				continue
			}
			off, n := int(binary.LittleEndian.Uint32(r[8:])), int(binary.LittleEndian.Uint32(r[12:]))
			d := b[off : off+n]
			for i := 0; i < len(d) && d[i] != 0; {
				op := d[i] & 0xf0
				seg := uint64(d[i] & 0xf)
				i++
				switch op {
				case 0x10:
				case 0x20:
					o, err := uleb(d, &i)
					if err != nil {
						t.Fatal(err)
					}
					if segs[seg].Addr+o == addr && d[i] == 0x51 {
						d[i] = 0x41
						return
					}
				case 0x50:
				default:
					t.Fatalf("rebase opcode %#x, which the Go linker does not write", op)
				}
			}
		}
	} else if f, err := pe.Open(bin); err == nil {
		defer f.Close()
		oh := f.OptionalHeader.(*pe.OptionalHeader64)
		dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC]
		at := binFileOffset(t, bin, oh.ImageBase+uint64(dir.VirtualAddress))
		d := b[at : at+int(dir.Size)]
		for i := 0; i < len(d); {
			page, n := binary.LittleEndian.Uint32(d[i:]), int(binary.LittleEndian.Uint32(d[i+4:]))
			for j := i + 8; j < i+n; j += 2 {
				e := binary.LittleEndian.Uint16(d[j:])
				if e>>12 == peRelDir64 && oh.ImageBase+uint64(page)+uint64(e&0xfff) == addr {
					binary.LittleEndian.PutUint16(d[j:], 0)
					return
				}
			}
			i += n
		}
	}
	t.Fatalf("no fixup of %#x found in %s", addr, bin)
}

// TestLoaderWrites: the check of a position-independent binary takes a
// pointer only with a slide fixup at it, and refuses a fixup in anything else
// it read: in code, a bind where a pointer was checked, and a fixup that
// starts before a read and reaches into it. A binary linked at a fixed
// address takes a pointer without one.
func TestLoaderWrites(t *testing.T) {
	const base = 0x10000
	mk := func(pie bool, fix map[uint64]fixup) *binFile {
		data := make([]byte, 64)
		binary.LittleEndian.PutUint64(data[32:], 0x1234)
		b := &binFile{format: "Mach-O", pie: pie, fixups: fix, secs: []*binSec{{name: "s", addr: base, size: 64, data: data}}}
		for a := range fix {
			b.fixAt = append(b.fixAt, a)
		}
		for i := range b.fixAt {
			for j := i + 1; j < len(b.fixAt); j++ {
				if b.fixAt[j] < b.fixAt[i] {
					b.fixAt[i], b.fixAt[j] = b.fixAt[j], b.fixAt[i]
				}
			}
		}
		return b
	}
	slide := fixup{width: 8, slide: true, what: "a rebase"}
	for name, c := range map[string]struct {
		pie       bool
		fix       map[uint64]fixup
		want      string // from pointer, then from loaderWrites
		wantSlide int
	}{
		"slid pointer":             {true, map[uint64]fixup{base + 32: slide}, "", 1},
		"fixed binary":             {false, nil, "", 0},
		"no fixup":                 {true, nil, "has no fixup", 0},
		"bound, not slid":          {true, map[uint64]fixup{base + 32: {width: 8, what: "a bind to _x"}}, "has a bind to _x", 0},
		"fixup in code":            {true, map[uint64]fixup{base + 32: slide, base + 4: slide}, "writes 1 places", 0},
		"fixup reaching into code": {true, map[uint64]fixup{base + 32: slide, base - 4: slide}, "writes 1 places", 0},
		"fixup in a fixed binary":  {false, map[uint64]fixup{base + 8: {width: 8, what: "a dynamic relocation"}}, "a dynamic relocation", 0},
		"4 bytes beside a pointer": {true, map[uint64]fixup{base + 32: slide, base + 40: {width: 4, what: "a HIGHLOW base relocation"}}, "HIGHLOW", 0},
	} {
		b := mk(c.pie, c.fix)
		// The code of a function, then the pointer, then data after it.
		if _, err := b.read(base, 32); err != nil {
			t.Fatal(err)
		}
		err := b.pointer(base+32, 0x1234)
		if err == nil {
			if _, err = b.read(base+40, 24); err != nil {
				t.Fatal(err)
			}
			var n int
			if n, err = b.loaderWrites(); err == nil && n != c.wantSlide {
				err = fmt.Errorf("%d pointers slid, want %d", n, c.wantSlide)
			}
		}
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	if err := mk(true, nil).pointer(base+32, 0x1235); err == nil || !strings.Contains(err.Error(), "the pointer is 0x1234") {
		t.Errorf("a wrong pointer: %v", err)
	}
}

// TestMachOFixupOpcodes reads every rebase and bind opcode the reader knows,
// in the compact forms ld64 writes and the Go linker does not, and refuses a
// slot outside its segment, an opcode it does not know and a table with no
// end.
func TestMachOFixupOpcodes(t *testing.T) {
	segs := []*macho.Segment{{SegmentHeader: macho.SegmentHeader{Addr: 0, Memsz: 0x1000}},
		{SegmentHeader: macho.SegmentHeader{Addr: 0x1000, Memsz: 0x100}}}
	got := func(b *binFile) string {
		var s []string
		for _, a := range sortedFixups(b) {
			f := b.fixups[a]
			s = append(s, fmt.Sprintf("%#x/%d/%v", a, f.width, f.slide))
		}
		return strings.Join(s, " ")
	}
	rebase := []byte{
		0x11,       // type pointer
		0x21, 0x10, // segment 1, offset 0x10
		0x52,       // two rebases: 0x1010, 0x1018
		0x30, 0x08, // skip 8: 0x1028
		0x60, 0x02, // two rebases: 0x1028, 0x1030
		0x70, 0x08, // a rebase at 0x1038, then skip 8: 0x1048
		0x80, 0x02, 0x08, // two rebases 16 bytes apart: 0x1048, 0x1058, and on to 0x1068
		0x41, // skip 8: 0x1070
		0x12, // type text absolute 32
		0x51, // one rebase: 0x1070
		0x00,
	}
	b := &binFile{fixups: map[uint64]fixup{}}
	if err := b.machoRebase(rebase, segs); err != nil {
		t.Fatal(err)
	}
	want := "0x1010/8/true 0x1018/8/true 0x1028/8/true 0x1030/8/true 0x1038/8/true 0x1048/8/true 0x1058/8/true 0x1070/4/false"
	if g := got(b); g != want {
		t.Errorf("rebase:\n got %s\nwant %s", g, want)
	}
	bind := []byte{
		0x11,                 // dylib 1
		0x40, '_', 'x', 0x00, // the symbol _x
		0x51,       // type pointer
		0x71, 0x00, // segment 1, offset 0
		0x90,             // bind 0x1000
		0xb1,             // bind 0x1008, then skip 8: 0x1018
		0xc0, 0x02, 0x08, // two binds 16 bytes apart: 0x1018, 0x1028
		0xa0, 0x00, // bind 0x1038
		0x00,
	}
	b = &binFile{fixups: map[uint64]fixup{}}
	if err := b.machoBind(bind, segs); err != nil {
		t.Fatal(err)
	}
	if g, w := got(b), "0x1000/8/false 0x1008/8/false 0x1018/8/false 0x1028/8/false 0x1038/8/false"; g != w {
		t.Errorf("bind:\n got %s\nwant %s", g, w)
	}
	if b.fixups[0x1000].what != "a bind to _x" {
		t.Errorf("bind: %q", b.fixups[0x1000].what)
	}
	for name, c := range map[string]struct {
		rebase bool
		d      []byte
		want   string
	}{
		"rebase past the segment": {true, []byte{0x11, 0x21, 0x80, 0x02, 0x51, 0x00}, "outside it"},
		"rebase with no segment":  {true, []byte{0x11, 0x51, 0x00}, "before its segment"},
		"rebase opcode unknown":   {true, []byte{0x11, 0x90, 0x00}, "unknown opcode"},
		"rebase with no end":      {true, []byte{0x11, 0x21, 0x00, 0x51}, "no DONE"},
		"rebase LEB cut short":    {true, []byte{0x21, 0x80}, "cut short"},
		"threaded bind":           {false, []byte{0xd0, 0x00}, "does not read"},
		"bind with no end":        {false, []byte{0x71, 0x00, 0x90}, "no DONE"},
	} {
		b := &binFile{fixups: map[uint64]fixup{}}
		var err error
		if c.rebase {
			err = b.machoRebase(c.d, segs)
		} else {
			err = b.machoBind(c.d, segs)
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func sortedFixups(b *binFile) []uint64 {
	var a []uint64
	for x := range b.fixups {
		a = append(a, x)
	}
	for i := range a {
		for j := i + 1; j < len(a); j++ {
			if a[j] < a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
	return a
}

// TestGLoad: a split ABI0 function loads g in the form of the system of its
// binary, and the check takes each form only for its own system.
func TestGLoad(t *testing.T) {
	const at, tls = 0x1000, 0x3000
	forms := map[string][]byte{
		"linux":  {0x64, 0x4c, 0x8b, 0x34, 0x25, 0xf8, 0xff, 0xff, 0xff}, // MOVQ FS:-8, R14
		"darwin": {0x65, 0x4c, 0x8b, 0x34, 0x25, 0x30, 0x00, 0x00, 0x00}, // MOVQ GS:0x30, R14
		// MOVQ runtime.tls_g(SB), R14; MOVQ GS:0(R14), R14; MOVQ 0(R14), R14
		"windows": {0x4c, 0x8b, 0x35, 0xf9, 0x1f, 0x00, 0x00, 0x65, 0x4d, 0x8b, 0x36, 0x4d, 0x8b, 0x36},
	}
	for form, code := range forms {
		for _, goos := range []string{"linux", "darwin", "windows"} {
			data := append(append([]byte(nil), code...), make([]byte, 32)...)
			b := &binFile{format: "test", goos: goos, syms: map[string]binSym{"runtime.tls_g": {Value: tls}},
				secs: []*binSec{{name: "text", addr: at, size: uint64(len(data)), data: data}}}
			k := &checker{bin: b}
			in, err := k.dec(at)
			if err != nil {
				t.Fatal(err)
			}
			end, err := k.gLoad(at, in)
			switch {
			case form == goos && (err != nil || end != at+uint64(len(code))):
				t.Errorf("%s: %#x, %v, want %#x", goos, end, err, at+len(code))
			case form != goos && err == nil:
				t.Errorf("the load of g of %s taken for %s", form, goos)
			}
		}
	}
	// The windows form with runtime.tls_g elsewhere, or the last load left out.
	for name, edit := range map[string]func(c []byte){
		"tls_g elsewhere": func(c []byte) { c[3]++ },
		"one load short":  func(c []byte) { c[11], c[12], c[13] = 0x90, 0x90, 0x90 },
	} {
		code := append(append([]byte(nil), forms["windows"]...), make([]byte, 32)...)
		edit(code)
		b := &binFile{format: "test", goos: "windows", syms: map[string]binSym{"runtime.tls_g": {Value: tls}},
			secs: []*binSec{{name: "text", addr: at, size: uint64(len(code)), data: code}}}
		k := &checker{bin: b}
		in, err := k.dec(at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := k.gLoad(at, in); err == nil || !strings.Contains(err.Error(), "unknown load of g for windows") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestMachOChained reads a chain of a rebase and a bind in the pointer format
// ld64 writes for arm64 and x86_64, and refuses the pointer formats of arm64e
// and a segment the table puts elsewhere than the file does.
func TestMachOChained(t *testing.T) {
	const image, data = 0x100000000, 0x100004000
	segs := []*macho.Segment{{SegmentHeader: macho.SegmentHeader{Name: "__PAGEZERO"}},
		{SegmentHeader: macho.SegmentHeader{Name: "__TEXT", Addr: image, Memsz: 0x4000, Filesz: 0x4000}},
		{SegmentHeader: macho.SegmentHeader{Name: "__DATA", Addr: data, Memsz: 0x4000, Offset: 0x4000, Filesz: 0x4000}}}
	table := func(format uint16, segOff uint64) []byte {
		d := make([]byte, 80)
		binary.LittleEndian.PutUint32(d[4:], 32)  // starts_offset
		binary.LittleEndian.PutUint32(d[32:], 3)  // seg_count
		binary.LittleEndian.PutUint32(d[44:], 16) // the third segment's starts, at 48
		binary.LittleEndian.PutUint32(d[48:], 24)
		binary.LittleEndian.PutUint16(d[52:], 0x4000)
		binary.LittleEndian.PutUint16(d[54:], format)
		binary.LittleEndian.PutUint64(d[56:], segOff)
		binary.LittleEndian.PutUint16(d[68:], 1) // page_count
		binary.LittleEndian.PutUint16(d[70:], 8) // the chain starts at 8
		return d
	}
	raw := make([]byte, 0x8000)
	// A rebase to 0x1234 past the image with the top byte 0x5a, the next
	// entry 16 bytes on, and a bind to import 3 that ends the chain.
	binary.LittleEndian.PutUint64(raw[0x4008:], 4<<51|0x5a<<36|0x1234)
	binary.LittleEndian.PutUint64(raw[0x4018:], 1<<63|3)
	b := &binFile{fixups: map[uint64]fixup{}}
	if err := b.machoChained(table(chainedPtr64Offset, 0x4000), segs, raw); err != nil {
		t.Fatal(err)
	}
	if f := b.fixups[data+8]; !f.slide || !f.chained || f.target != 0x5a<<56|(image+0x1234) {
		t.Errorf("the rebase: %+v", f)
	}
	if f := b.fixups[data+0x18]; f.slide || !f.chained || f.what != "a chained bind to import 3" {
		t.Errorf("the bind: %+v", f)
	}
	if len(b.fixups) != 2 {
		t.Errorf("%d fixups, want 2", len(b.fixups))
	}
	// DYLD_CHAINED_PTR_64 holds the address itself.
	binary.LittleEndian.PutUint64(raw[0x4008:], image+0x1234)
	b = &binFile{fixups: map[uint64]fixup{}}
	if err := b.machoChained(table(chainedPtr64, 0x4000), segs, raw); err != nil || b.fixups[data+8].target != image+0x1234 {
		t.Errorf("DYLD_CHAINED_PTR_64: %v, %+v", err, b.fixups[data+8])
	}
	for name, c := range map[string]struct {
		d    []byte
		want string
	}{
		"arm64e":          {table(1, 0x4000), "pointer format 1"},
		"segment moved":   {table(chainedPtr64Offset, 0x8000), "the file has it at 0x4000"},
		"other version":   {append([]byte{1}, table(chainedPtr64Offset, 0x4000)[1:]...), "version 1"},
		"table cut short": {table(chainedPtr64Offset, 0x4000)[:60], "cut short"},
	} {
		b := &binFile{fixups: map[uint64]fixup{}}
		if err := b.machoChained(c.d, segs, raw); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	// A pointer at a chained rebase is its target, and at a chained bind an
	// import.
	b = &binFile{format: "Mach-O", pie: true, fixups: map[uint64]fixup{
		data + 8:  {width: 8, slide: true, chained: true, target: image + 0x1234, what: "a chained rebase"},
		data + 16: {width: 8, chained: true, what: "a chained bind to import 3"}},
		secs: []*binSec{{name: "data", addr: data, size: 32, data: make([]byte, 32)}}}
	if err := b.pointer(data+8, image+0x1234); err != nil {
		t.Errorf("a chained rebase: %v", err)
	}
	if err := b.pointer(data+8, image+0x1235); err == nil {
		t.Error("a chained rebase to another target taken")
	}
	if err := b.pointer(data+16, 0); err == nil || !strings.Contains(err.Error(), "an import") {
		t.Errorf("a chained bind: %v", err)
	}
}
