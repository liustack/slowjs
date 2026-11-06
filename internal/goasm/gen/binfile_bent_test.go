// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// TestOpenBinBent opens Go programs of every target the module builds for,
// cut short or with bytes changed where the reader looks (the headers, the
// load commands, the fixups, the imports, the symbols), and reads them as
// the check does. A bent binary may be refused or read, but must never make
// the reader panic. The changes come from a fixed seed, so a failure repeats.
// It needs the go tool and runs on any host: the programs are cross-built.
func TestOpenBinBent(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go tool")
	}
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module bent\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nimport \"os\"\n\nvar table = []*int{new(int), new(int)}\n\nfunc main() { os.Exit(*table[len(os.Args)%2]) }\n"
	if err := os.WriteFile(filepath.Join(work, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct{ goos, arch string }{
		{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"},
	} {
		t.Run(target.goos+"-"+target.arch, func(t *testing.T) {
			bin := filepath.Join(work, target.goos+"-"+target.arch)
			cmd := exec.Command("go", "build", "-o", bin, ".")
			cmd.Dir = work
			cmd.Env = append(os.Environ(), "GOOS="+target.goos, "GOARCH="+target.arch, "CGO_ENABLED=0")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("go build: %v\n%s", err, out)
			}
			good, err := os.ReadFile(bin)
			if err != nil {
				t.Fatal(err)
			}
			b, err := openBin(bin, target.arch)
			if err != nil {
				t.Fatalf("the program as built: %v", err)
			}
			readAll(b)
			b.Close()
			regions := bentRegions(t, bin, len(good))
			rng := rand.New(rand.NewPCG(1, 2))
			bent := filepath.Join(work, "bent")
			opened := 0
			for i := range 600 {
				d := append([]byte(nil), good...)
				what := ""
				if i%4 == 0 {
					n := rng.IntN(len(d))
					d, what = d[:n], fmt.Sprintf("cut to %d bytes", n)
				} else {
					r := regions[rng.IntN(len(regions))]
					for range 1 + rng.IntN(4) {
						at := r[0] + rng.IntN(r[1]-r[0])
						v := byte(rng.Uint32())
						switch rng.IntN(3) {
						case 0:
							v = 0
						case 1:
							v = 0xff
						}
						d[at] = v
						what += fmt.Sprintf(" %#x=%#02x", at, v)
					}
				}
				if err := os.WriteFile(bent, d, 0o644); err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("change %d (%s): panic: %v", i, what, r)
						}
					}()
					b, err := openBin(bent, target.arch)
					if err != nil {
						return
					}
					opened++
					defer b.Close()
					readAll(b)
				}()
			}
			t.Logf("%d of 600 bent programs read", opened)
		})
	}
}

// readAll reads a binary as the check does: the bytes of its symbols and
// the pointers among them, then what the loader writes there.
func readAll(b *binFile) {
	names := make([]string, 0, len(b.syms))
	for n := range b.syms {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := b.syms[n]
		if d, err := b.upTo(s.Value, int(min64(s.Size, 256))); err == nil && len(d) >= 8 {
			b.pointer(s.Value, binary.LittleEndian.Uint64(d))
		}
	}
	b.loaderWrites()
	b.describe()
}

// bentRegions gives the ranges of the file at path that its reader reads
// besides code and data: the headers, and where the format keeps its
// fixups, imports and symbols.
func bentRegions(t *testing.T, path string, size int) [][2]int {
	t.Helper()
	add := func(rs [][2]int, lo, n uint64) [][2]int {
		if n == 0 || lo >= uint64(size) {
			return rs
		}
		return append(rs, [2]int{int(lo), int(min64(lo+n, uint64(size)))})
	}
	rs := add(nil, 0, 4096)
	if f, err := elf.Open(path); err == nil {
		defer f.Close()
		for _, s := range f.Sections {
			if s.Type == elf.SHT_SYMTAB || s.Type == elf.SHT_STRTAB || s.Type == elf.SHT_RELA {
				rs = add(rs, s.Offset, s.Size)
			}
		}
		return rs
	}
	if f, err := macho.Open(path); err == nil {
		defer f.Close()
		for _, l := range f.Loads {
			r := l.Raw()
			if len(r) >= 48 && binary.LittleEndian.Uint32(r) == lcDyldInfoOnly {
				for i := range 5 {
					rs = add(rs, uint64(binary.LittleEndian.Uint32(r[8+8*i:])), uint64(binary.LittleEndian.Uint32(r[12+8*i:])))
				}
			}
			if s, ok := l.(*macho.Symtab); ok {
				rs = add(rs, uint64(s.Symoff), uint64(len(s.Syms))*16)
			}
		}
		return rs
	}
	f, err := pe.Open(path)
	if err != nil {
		t.Fatalf("%s: no ELF, Mach-O or PE file", path)
	}
	defer f.Close()
	oh := f.OptionalHeader.(*pe.OptionalHeader64)
	for _, i := range []int{pe.IMAGE_DIRECTORY_ENTRY_IMPORT, pe.IMAGE_DIRECTORY_ENTRY_BASERELOC, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG, pe.IMAGE_DIRECTORY_ENTRY_IAT} {
		dir := oh.DataDirectory[i]
		for _, s := range f.Sections {
			if dir.VirtualAddress >= s.VirtualAddress && dir.VirtualAddress < s.VirtualAddress+s.Size {
				rs = add(rs, uint64(s.Offset+dir.VirtualAddress-s.VirtualAddress), uint64(dir.Size))
			}
		}
	}
	rs = add(rs, uint64(f.FileHeader.PointerToSymbolTable), uint64(f.FileHeader.NumberOfSymbols)*18)
	return rs
}

// TestMachOChainedBent reads chained fixups with bytes of the table and the
// fields of the segments changed, and the reader must refuse or read each,
// never panic.
func TestMachOChainedBent(t *testing.T) {
	const image, data = 0x100000000, 0x100004000
	good := make([]byte, 80)
	binary.LittleEndian.PutUint32(good[4:], 32)
	binary.LittleEndian.PutUint32(good[32:], 3)
	binary.LittleEndian.PutUint32(good[44:], 16)
	binary.LittleEndian.PutUint32(good[48:], 24)
	binary.LittleEndian.PutUint16(good[52:], 0x4000)
	binary.LittleEndian.PutUint16(good[54:], chainedPtr64Offset)
	binary.LittleEndian.PutUint64(good[56:], 0x4000)
	binary.LittleEndian.PutUint16(good[68:], 1)
	binary.LittleEndian.PutUint16(good[70:], 8)
	raw := make([]byte, 0x8000)
	binary.LittleEndian.PutUint64(raw[0x4008:], 4<<51|0x1234)
	binary.LittleEndian.PutUint64(raw[0x4018:], 1<<63|3)
	rng := rand.New(rand.NewPCG(3, 4))
	big := []uint64{0, 8, 0x4000, 0x7ff8, 0x8000, 1 << 32, 1 << 63, ^uint64(0) - 7, ^uint64(0)}
	for i := range 20000 {
		d := append([]byte(nil), good...)
		if i%3 != 0 {
			for range 1 + rng.IntN(3) {
				d[rng.IntN(len(d))] = byte(rng.Uint32())
			}
		}
		seg := macho.SegmentHeader{Name: "__DATA", Addr: data, Memsz: 0x4000, Offset: 0x4000, Filesz: 0x4000}
		if i%3 != 1 {
			seg.Offset, seg.Filesz = big[rng.IntN(len(big))], big[rng.IntN(len(big))]
		}
		segs := []*macho.Segment{{SegmentHeader: macho.SegmentHeader{Name: "__PAGEZERO"}},
			{SegmentHeader: macho.SegmentHeader{Name: "__TEXT", Addr: image, Memsz: 0x4000, Filesz: 0x4000}},
			{SegmentHeader: seg}}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("change %d (table %x, segment at %#x, %#x bytes): panic: %v", i, d, seg.Offset, seg.Filesz, r)
				}
			}()
			b := &binFile{fixups: map[uint64]fixup{}}
			b.machoChained(d, segs, raw)
		}()
	}
}

// TestReadBounds: a read that runs past the end of a section is refused, also
// when the section ends just below the top of the address space.
func TestReadBounds(t *testing.T) {
	const top = ^uint64(0) - 0xff
	b := &binFile{secs: []*binSec{{name: "high", addr: top, size: 0xf0, data: make([]byte, 0xf0)}}}
	for _, c := range []struct {
		addr uint64
		n    int
	}{{top + 0xe8, 0x20}, {top + 0xe8, 9}, {top, 0xf1}, {top + 0x10, -1}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("read(%#x, %d): panic: %v", c.addr, c.n, r)
				}
			}()
			if _, err := b.read(c.addr, c.n); err == nil {
				t.Errorf("read(%#x, %d) past the end of the section taken", c.addr, c.n)
			}
		}()
	}
	if d, err := b.upTo(top+0xe8, 0x20); err != nil || len(d) != 8 {
		t.Errorf("upTo at the end of the section: %d bytes, %v", len(d), err)
	}
	if d, err := b.read(top+0xe8, 8); err != nil || len(d) != 8 {
		t.Errorf("read of the last 8 bytes: %d bytes, %v", len(d), err)
	}
}
