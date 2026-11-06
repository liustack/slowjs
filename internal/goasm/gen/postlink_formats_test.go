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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	var bins []string
	for _, goos := range systems {
		bin := filepath.Join(work, goos+"-"+arch)
		bins = append(bins, bin)
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
		for _, fault := range []struct {
			name, want string
			edit       func(t *testing.T, bin string, b []byte)
			only       string // the system of the fault, or every one
		}{
			{"a byte of a function", "fib", func(t *testing.T, bin string, b []byte) {
				at := binFileOffset(t, bin, binSymbol(t, bin, arch, "checkmini/mini.fib")+24)
				b[at] ^= 0xff
			}, ""},
			{"a pointer of engine_rodata", "the pointer is", func(t *testing.T, bin string, b []byte) {
				at := binFileOffset(t, bin, firstRodataPointer(t, bin, arch))
				binary.LittleEndian.PutUint64(b[at:], binary.LittleEndian.Uint64(b[at:])+8)
			}, ""},
			{"the fixup of a pointer", "has no fixup", func(t *testing.T, bin string, b []byte) {
				dropFixup(t, bin, b, firstRodataPointer(t, bin, arch))
			}, ""},
			{"a second fixup of a pointer", "two fixups at", func(t *testing.T, bin string, b []byte) {
				lo, hi := rodataRange(t, bin, arch)
				dupFixup(t, bin, b, firstRodataPointer(t, bin, arch), lo, hi)
			}, ""},
			// The other places the PE loader writes: the import address
			// table of a descriptor, the slots the load config names, and
			// what a TLS directory asks for.
			{"an import table in engine_rodata", "an import", func(t *testing.T, bin string, b []byte) {
				peImportInto(t, bin, b, firstRodataPointer(t, bin, arch))
			}, "windows"},
			{"a cookie of the load config in engine_rodata", "the loader writes 1 places", func(t *testing.T, bin string, b []byte) {
				peLoadConfigSlot(t, bin, b, 88, firstRodataPlain(t, bin, arch))
			}, "windows"},
			{"a TLS directory", "TLS", func(t *testing.T, bin string, b []byte) {
				peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_TLS, 0x1000, 0x28)
			}, "windows"},
			{"a delay import directory", "delay import", func(t *testing.T, bin string, b []byte) {
				peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_DELAY_IMPORT, 0x1000, 0x20)
			}, "windows"},
			// A directory the loader reads by its address alone, its size
			// left at 0.
			{"an import table in engine_rodata, its directory with no size", "an import", func(t *testing.T, bin string, b []byte) {
				peImportInto(t, bin, b, firstRodataPointer(t, bin, arch))
				peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_IMPORT, peDirectoryRVA(t, bin, pe.IMAGE_DIRECTORY_ENTRY_IMPORT), 0)
			}, "windows"},
			{"a cookie of the load config in engine_rodata, its directory with no size", "the loader writes 1 places", func(t *testing.T, bin string, b []byte) {
				peLoadConfigSlot(t, bin, b, 88, firstRodataPlain(t, bin, arch))
				peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG, peDirectoryRVA(t, bin, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG), 0)
			}, "windows"},
			{"a directory with a size and no address", "a size and no address", func(t *testing.T, bin string, b []byte) {
				peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_DEBUG, 0, 0x1c)
			}, "windows"},
			// A fixup of what the check reads to find the loader's writes:
			// the loader applies it first. An import address table 64 KiB
			// times k below engine_rodata, with a relocation of its address,
			// is in engine_rodata when the loader slides the program by that.
			{"a relocation of an import descriptor", "inside the import descriptors", func(t *testing.T, bin string, b []byte) {
				desc := peImportShifted(t, bin, b, firstRodataPointer(t, bin, arch))
				peAddReloc(t, bin, b, desc+16, peRelHighLow)
			}, "windows"},
			{"a relocation of the load config", "inside the load config", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peDirectoryAddr(t, bin, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG)+88, peRelDir64)
			}, "windows"},
			// The loader writes only into the sections the linker puts
			// pointers in, and never onto what the check reads to find
			// its writes.
			{"a rebase of the load commands", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				lo, hi := rodataRange(t, bin, arch)
				machoRebaseInto(t, bin, b, lo, hi, "__TEXT", 0x20)
			}, "darwin"},
			{"a rebase of __LINKEDIT", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				lo, hi := rodataRange(t, bin, arch)
				machoRebaseInto(t, bin, b, lo, hi, "__LINKEDIT", 0x20)
			}, "darwin"},
			{"a rebase of code", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				lo, hi := rodataRange(t, bin, arch)
				machoRebaseInto(t, bin, b, lo, hi, "__TEXT", binSymbol(t, bin, arch, "runtime.main")-machoSegAddr(t, bin, "__TEXT"))
			}, "darwin"},
			{"a relocation of the size of the load config, both sizes at 0", "inside the load config", func(t *testing.T, bin string, b []byte) {
				lc := peDirectoryAddr(t, bin, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG)
				binary.LittleEndian.PutUint32(b[binFileOffset(t, bin, lc):], 0)
				peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG, peDirectoryRVA(t, bin, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG), 0)
				peAddReloc(t, bin, b, lc, peRelHighLow)
			}, "windows"},
			{"a relocation of a lookup table", "inside the lookup table of import descriptor 0", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peImportLookup(t, bin), peRelDir64)
			}, "windows"},
			{"a relocation of the headers", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peImageBase(t, bin)+0x100, peRelDir64)
			}, "windows"},
			{"a relocation of the base relocation table", "inside the base relocation table", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peDirectoryAddr(t, bin, pe.IMAGE_DIRECTORY_ENTRY_BASERELOC), peRelHighLow)
			}, "windows"},
			{"a relocation of code", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, binSymbol(t, bin, arch, "runtime.main"), peRelDir64)
			}, "windows"},
			{"a relocation inside an import slot", "overlap", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peDirectoryAddr(t, bin, pe.IMAGE_DIRECTORY_ENTRY_IAT)+4, peRelHighLow)
			}, "windows"},
			// A fixed ELF executable has nothing the loader writes: a
			// relocation table, or a dynamic table that names one, is
			// refused whatever its relocations are.
			{"a dynamic relocation of its own table", "of type SHT_RELA", func(t *testing.T, bin string, b []byte) {
				elfRelaInto(t, bin, b, arch)
			}, "linux"},
			{"a TLSDESC relocation of the last 8 bytes of .data", "of type SHT_RELA", func(t *testing.T, bin string, b []byte) {
				typ := map[string]uint64{"amd64": uint64(elf.R_X86_64_TLSDESC), "arm64": uint64(elf.R_AARCH64_TLSDESC)}[arch]
				elfRelaOne(t, bin, b, true, typ, elfSectionEnd(t, bin, ".data")-8)
			}, "linux"},
			{"a COPY relocation of the last 8 bytes of .data", "of type SHT_RELA", func(t *testing.T, bin string, b []byte) {
				typ := map[string]uint64{"amd64": uint64(elf.R_X86_64_COPY), "arm64": uint64(elf.R_AARCH64_COPY)}[arch]
				elfRelaOne(t, bin, b, true, typ, elfSectionEnd(t, bin, ".data")-8)
			}, "linux"},
			{"a dynamic table the section table does not show", "PT_DYNAMIC", func(t *testing.T, bin string, b []byte) {
				typ := map[string]uint64{"amd64": uint64(elf.R_X86_64_RELATIVE), "arm64": uint64(elf.R_AARCH64_RELATIVE)}[arch]
				elfRelaOne(t, bin, b, false, typ, binSymbol(t, bin, arch, "runtime.main"))
			}, "linux"},
			// The edges of what a section holds in the file: a fixup that
			// ends at its last byte is taken, one a byte further, or in a
			// section of zeros the file does not hold, is not.
			{"a relocation of the last 8 bytes .data holds in the file", "", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peSectionFileEnd(t, bin, ".data")-8, peRelDir64)
			}, "windows"},
			{"a relocation that ends a byte past what .data holds in the file", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				peAddReloc(t, bin, b, peSectionFileEnd(t, bin, ".data")-7, peRelDir64)
			}, "windows"},
			{"a rebase of __DATA,__bss", "outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				lo, hi := rodataRange(t, bin, arch)
				machoRebaseInto(t, bin, b, lo, hi, "__DATA", machoSectAddr(t, bin, "__DATA", "__bss")-machoSegAddr(t, bin, "__DATA"))
			}, "darwin"},
			// What the loader maps from the file, and where: the check
			// reads that, not what the sections say. b has as many bytes
			// again after the file, for the copies these faults make.
			{"a segment that ends in the file before engine_rodata", "not where its segment maps it from the file", func(t *testing.T, bin string, b []byte) {
				lo, _ := rodataRange(t, bin, arch)
				shortSegment(t, bin, b, lo)
			}, "linux"},
			{"a segment that ends in the file before engine_rodata", "not where its segment maps it from the file", func(t *testing.T, bin string, b []byte) {
				lo, _ := rodataRange(t, bin, arch)
				shortSegment(t, bin, b, lo)
			}, "darwin"},
			// The load config, which Go writes since Go 1.27, is read first
			// and refuses it; a binary without one refuses the relocations
			// of what no longer is in the file.
			{"a section that ends in the file before engine_rodata", "in no section of the file|outside the sections the linker puts pointers in", func(t *testing.T, bin string, b []byte) {
				lo, _ := rodataRange(t, bin, arch)
				shortSegment(t, bin, b, lo)
			}, "windows"},
			{"a section read from a copy, the code the loader maps changed", "where its segment maps it from the file", func(t *testing.T, bin string, b []byte) {
				sectionFromCopy(t, bin, b, binSymbol(t, bin, arch, "checkmini/mini.fib")+24)
			}, "linux"},
			{"a section read from a copy, the code the loader maps changed", "where its segment maps it from the file", func(t *testing.T, bin string, b []byte) {
				sectionFromCopy(t, bin, b, binSymbol(t, bin, arch, "checkmini/mini.fib")+24)
			}, "darwin"},
			{"a section the loader reads from a rounded offset", "not a multiple of 0x200", func(t *testing.T, bin string, b []byte) {
				sectionFromCopy(t, bin, b, binSymbol(t, bin, arch, "checkmini/mini.fib")+24)
			}, "windows"},
			{"a second segment over the code", "overlap", func(t *testing.T, bin string, b []byte) {
				elfLoadOver(t, bin, b, arch, binSymbol(t, bin, arch, "checkmini/mini.fib")+24)
			}, "linux"},
		} {
			// A fixed ELF executable has only faults of its own.
			if fault.only != "" && fault.only != goos || !pie && fault.only == "" {
				continue
			}
			// Go writes a load config into a windows binary since Go 1.27.
			// A binary without one has no slot of it for the loader to
			// write, so there is nothing to bend.
			if goos == "windows" && strings.Contains(fault.name, "load config") && !peHasDirectory(t, bin, pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG) {
				t.Logf("%s/%s, %s: no load config in a binary of %s", goos, arch, fault.name, runtime.Version())
				continue
			}
			raw, err := os.ReadFile(bin)
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, make([]byte, len(raw))...)
			fault.edit(t, bin, raw)
			bad := bin + "-bad"
			if err := os.WriteFile(bad, raw, 0o755); err != nil {
				t.Fatal(err)
			}
			out, err := check(bad)
			if fault.want == "" { // a change the check must take
				if err != nil || !strings.Contains(out, "post-link check passed") {
					t.Errorf("%s/%s, %s: refused: %v\n%s", goos, arch, fault.name, err, lastLine(out))
				}
				continue
			}
			if err == nil || strings.Contains(out, "post-link check passed") || !containsOneOf(out, fault.want) {
				t.Errorf("%s/%s, %s: not refused with %q: %v\n%s", goos, arch, fault.name, fault.want, err, lastLine(out))
			} else {
				t.Logf("%s/%s, %s: %s", goos, arch, fault.name, lastLine(out))
			}
		}
	}
	// -verify-bin takes them all at once, separated by commas.
	out, err := check(strings.Join(bins, ","))
	if err != nil || strings.Count(out, "post-link check passed for ") != len(bins) {
		t.Fatalf("the check of %d binaries at once: %v\n%s", len(bins), err, out)
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
		"rebase twice":            {true, []byte{0x11, 0x21, 0x10, 0x51, 0x21, 0x10, 0x51, 0x00}, "two fixups at 0x1010"},
		"bind twice":              {false, []byte{0x71, 0x10, 0x90, 0x71, 0x10, 0x90, 0x00}, "two fixups at 0x1010"},
		"rebase across the end":   {true, []byte{0x11, 0x21, 0xfc, 0x01, 0x51, 0x00}, "outside it"},
		"bind across the end":     {false, []byte{0x71, 0xfc, 0x01, 0x90, 0x00}, "outside it"},
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
	// A 32-bit slot fits in the last 4 bytes of a segment.
	b = &binFile{fixups: map[uint64]fixup{}}
	if err := b.machoRebase([]byte{0x12, 0x21, 0xfc, 0x01, 0x51, 0x00}, segs); err != nil || b.fixups[0x10fc].width != 4 {
		t.Errorf("a 32-bit rebase at the end of a segment: %v", err)
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

// TestMachOChainedBounds: a segment whose bytes the file does not hold, by
// its offset or its size, is refused with an error, not read past the file.
func TestMachOChainedBounds(t *testing.T) {
	const image, data = 0x100000000, 0x100004000
	d := make([]byte, 80)
	binary.LittleEndian.PutUint32(d[4:], 32)
	binary.LittleEndian.PutUint32(d[32:], 3)
	binary.LittleEndian.PutUint32(d[44:], 16)
	binary.LittleEndian.PutUint32(d[48:], 24)
	binary.LittleEndian.PutUint16(d[52:], 0x4000)
	binary.LittleEndian.PutUint16(d[54:], chainedPtr64Offset)
	binary.LittleEndian.PutUint64(d[56:], 0x4000)
	binary.LittleEndian.PutUint16(d[68:], 1)
	binary.LittleEndian.PutUint16(d[70:], 8)
	raw := make([]byte, 0x8000)
	for name, seg := range map[string]macho.SegmentHeader{
		"offset past the file": {Offset: 0x10000, Filesz: 0x4000},
		"offset at the end":    {Offset: 0x8000 - 12, Filesz: 0x4000},
		"offset wraps around":  {Offset: ^uint64(0) - 7, Filesz: 0x4000},
		"size past the file":   {Offset: 0x4000, Filesz: 1 << 40},
		"size and offset wrap": {Offset: 1 << 63, Filesz: 1<<63 + 0x10},
	} {
		seg.Name, seg.Addr, seg.Memsz = "__DATA", data, 0x4000
		segs := []*macho.Segment{{SegmentHeader: macho.SegmentHeader{Name: "__PAGEZERO"}},
			{SegmentHeader: macho.SegmentHeader{Name: "__TEXT", Addr: image, Memsz: 0x4000, Filesz: 0x4000}},
			{SegmentHeader: seg}}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic: %v", name, r)
				}
			}()
			b := &binFile{fixups: map[uint64]fixup{}}
			if err := b.machoChained(d, segs, raw); err == nil || !strings.Contains(err.Error(), "outside") {
				t.Errorf("%s: %v, want a chained entry outside the file", name, err)
			}
		}()
	}
}

// TestMachORebaseAndBind: a slot that the rebase info and the bind info both
// name is refused.
func TestMachORebaseAndBind(t *testing.T) {
	segs := []*macho.Segment{{SegmentHeader: macho.SegmentHeader{Addr: 0, Memsz: 0x1000}},
		{SegmentHeader: macho.SegmentHeader{Addr: 0x1000, Memsz: 0x100}}}
	b := &binFile{fixups: map[uint64]fixup{}}
	if err := b.machoRebase([]byte{0x11, 0x21, 0x10, 0x51, 0x00}, segs); err != nil {
		t.Fatal(err)
	}
	if err := b.machoBind([]byte{0x71, 0x10, 0x90, 0x00}, segs); err == nil || !strings.Contains(err.Error(), "two fixups at 0x1010") {
		t.Errorf("a bind at a rebased slot: %v", err)
	}
}

// ulebIn writes v as an unsigned LEB128 number of exactly n bytes, with
// continuation bytes of no value where it needs fewer, and reports whether
// it fits.
func ulebIn(d []byte, v uint64) bool {
	for i := range d {
		c := byte(v & 0x7f)
		v >>= 7
		if i < len(d)-1 {
			c |= 0x80
		}
		d[i] = c
	}
	return v == 0
}

// dupFixup gives the pointer at addr a second fixup in the file b: in Mach-O
// another rebase of the Go linker's (a segment and offset, then one rebase)
// is made to name it, with its offset written in as many bytes as it had, in
// PE another entry of the same block is made the same as its entry. The
// entry taken is none of engine_rodata (lo to hi), so the second fixup is
// the only fault.
func dupFixup(t *testing.T, bin string, b []byte, addr, lo, hi uint64) {
	t.Helper()
	if f, err := macho.Open(bin); err == nil {
		defer f.Close()
		var segs []*macho.Segment
		for _, l := range f.Loads {
			if s, ok := l.(*macho.Segment); ok {
				segs = append(segs, s)
			}
		}
		for _, l := range f.Loads {
			r := l.Raw()
			if binary.LittleEndian.Uint32(r) != lcDyldInfoOnly {
				continue
			}
			off, n := int(binary.LittleEndian.Uint32(r[8:])), int(binary.LittleEndian.Uint32(r[12:]))
			d := b[off : off+n]
			seg := -1
			for i := 0; i < len(d) && d[i] != 0; {
				op := d[i] & 0xf0
				at := i
				i++
				if op != 0x20 {
					continue
				}
				o, err := uleb(d, &i)
				if err != nil {
					t.Fatal(err)
				}
				s := int(d[at] & 0xf)
				if segs[s].Addr+o == addr {
					seg = s
					continue
				}
				// Another rebase, after the pointer's and outside engine_rodata:
				// make it name the pointer.
				if a := segs[s].Addr + o; seg >= 0 && (a < lo || a >= hi) && d[i] == 0x51 && ulebIn(d[at+1:i], addr-segs[seg].Addr) {
					d[at] = 0x20 | byte(seg)
					return
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
			mine, other := -1, -1
			for j := i + 8; j < i+n; j += 2 {
				e := binary.LittleEndian.Uint16(d[j:])
				switch {
				case e>>12 == peRelDir64 && oh.ImageBase+uint64(page)+uint64(e&0xfff) == addr:
					mine = j
				case other < 0 && (e>>12 == peRelAbsolute || oh.ImageBase+uint64(page)+uint64(e&0xfff) < lo || oh.ImageBase+uint64(page)+uint64(e&0xfff) >= hi):
					other = j
				}
			}
			if mine >= 0 && other >= 0 {
				copy(d[other:other+2], d[mine:mine+2])
				return
			}
			i += n
		}
	}
	t.Fatalf("cannot give %#x a second fixup in %s", addr, bin)
}

// rodataRange is where engine_rodata of checkmini/mini is in bin.
func rodataRange(t *testing.T, bin, arch string) (uint64, uint64) {
	t.Helper()
	s, ok := openTestBin(t, bin, arch).syms["checkmini/mini."+RodataGo]
	if !ok {
		t.Fatalf("no engine_rodata in %s", bin)
	}
	return s.Value, s.Value + s.Size
}

// peHeader gives where the optional header of the PE file b starts.
func peHeader(t *testing.T, b []byte) int {
	t.Helper()
	at := int(binary.LittleEndian.Uint32(b[0x3c:]))
	if string(b[at:at+4]) != "PE\x00\x00" {
		t.Fatal("no PE signature")
	}
	return at + 4 + 20
}

// peDirectory sets data directory i of the PE file b.
func peDirectory(t *testing.T, bin string, b []byte, i int, rva, size uint32) {
	t.Helper()
	at := peHeader(t, b) + 112 + 8*i // the directories of PE32+
	binary.LittleEndian.PutUint32(b[at:], rva)
	binary.LittleEndian.PutUint32(b[at+4:], size)
}

// peImportInto moves the import address table of the first import
// descriptor of bin to addr: the loader then writes the imports there.
func peImportInto(t *testing.T, bin string, b []byte, addr uint64) {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	oh := f.OptionalHeader.(*pe.OptionalHeader64)
	dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
	if dir.Size == 0 {
		t.Fatalf("no imports in %s", bin)
	}
	at := binFileOffset(t, bin, oh.ImageBase+uint64(dir.VirtualAddress))
	binary.LittleEndian.PutUint32(b[at+16:], uint32(addr-oh.ImageBase))
}

// peLoadConfigSlot sets the field at off of the load config of bin to the
// address addr.
func peLoadConfigSlot(t *testing.T, bin string, b []byte, off int, addr uint64) {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	oh := f.OptionalHeader.(*pe.OptionalHeader64)
	dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG]
	if dir.Size < uint32(off+8) {
		t.Fatalf("a load config of %d bytes in %s", dir.Size, bin)
	}
	at := binFileOffset(t, bin, oh.ImageBase+uint64(dir.VirtualAddress))
	binary.LittleEndian.PutUint64(b[at+off:], addr)
}

// firstRodataPlain is the first 8 bytes of engine_rodata that no fixup
// touches.
func firstRodataPlain(t *testing.T, bin, arch string) uint64 {
	t.Helper()
	b := openTestBin(t, bin, arch)
	lo, hi := rodataRange(t, bin, arch)
	for a := lo; a+8 <= hi; a += 8 {
		free := true
		for _, f := range b.fixAt {
			if f+8 > a && f < a+8 {
				free = false
			}
		}
		if free {
			return a
		}
	}
	t.Fatalf("no 8 bytes of engine_rodata without a fixup in %s", bin)
	return 0
}

// peDirectoryRVA is the address of data directory i of bin, relative to its
// image base, and peDirectoryAddr the address itself.
// containsOneOf reports whether s holds one of the texts of want, separated
// by "|".
func containsOneOf(s, want string) bool {
	for _, w := range strings.Split(want, "|") {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// peHasDirectory reports whether bin has the data directory i.
func peHasDirectory(t *testing.T, bin string, i int) bool {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return f.OptionalHeader.(*pe.OptionalHeader64).DataDirectory[i].VirtualAddress != 0
}

func peDirectoryRVA(t *testing.T, bin string, i int) uint32 {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dir := f.OptionalHeader.(*pe.OptionalHeader64).DataDirectory[i]
	if dir.VirtualAddress == 0 {
		t.Fatalf("no directory %d in %s", i, bin)
	}
	return dir.VirtualAddress
}

func peDirectoryAddr(t *testing.T, bin string, i int) uint64 {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return f.OptionalHeader.(*pe.OptionalHeader64).ImageBase + uint64(peDirectoryRVA(t, bin, i))
}

// peImportShifted moves the import address table of the first import
// descriptor of bin to 64 KiB times k below addr, for the first k at which
// it overlaps no fixup, and gives the address of that descriptor.
func peImportShifted(t *testing.T, bin string, b []byte, addr uint64) uint64 {
	t.Helper()
	tb := openTestBin(t, bin, "amd64")
	n := uint64(0)
	for _, f := range tb.fixups {
		if f.what == "an import of descriptor 0" {
			n++
		}
	}
	desc := peDirectoryAddr(t, bin, pe.IMAGE_DIRECTORY_ENTRY_IMPORT)
	for k := uint64(1); k < 64; k++ {
		lo := addr - k<<16
		free := true
		for _, a := range tb.fixAt {
			if a+8 > lo && a < lo+8*n {
				free = false
			}
		}
		if free {
			peImportInto(t, bin, b, lo)
			return desc
		}
	}
	t.Fatalf("no place below %#x for the imports of %s", addr, bin)
	return 0
}

// peAddReloc adds a block to the base relocation table of bin with one
// relocation of type typ at addr, in the room the file has after the table.
func peAddReloc(t *testing.T, bin string, b []byte, addr uint64, typ uint16) {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	oh := f.OptionalHeader.(*pe.OptionalHeader64)
	dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC]
	for i, s := range f.Sections {
		if dir.VirtualAddress < s.VirtualAddress || dir.VirtualAddress >= s.VirtualAddress+s.VirtualSize {
			continue
		}
		end := dir.VirtualAddress - s.VirtualAddress + dir.Size
		if end+12 > s.Size || end != s.VirtualSize {
			t.Fatalf("no room after the base relocations of %s", bin)
		}
		at := int(s.Offset + end)
		rva := uint32(addr - oh.ImageBase)
		binary.LittleEndian.PutUint32(b[at:], rva&^0xfff)
		binary.LittleEndian.PutUint32(b[at+4:], 12)
		binary.LittleEndian.PutUint16(b[at+8:], typ<<12|uint16(rva&0xfff))
		binary.LittleEndian.PutUint16(b[at+10:], 0)
		peDirectory(t, bin, b, pe.IMAGE_DIRECTORY_ENTRY_BASERELOC, dir.VirtualAddress, dir.Size+12)
		sh := peHeader(t, b) + int(f.FileHeader.SizeOfOptionalHeader) + 40*i
		binary.LittleEndian.PutUint32(b[sh+8:], s.VirtualSize+12)
		return
	}
	t.Fatalf("the base relocations of %s are in no section", bin)
}

// machoRebaseInto turns a rebase of bin outside engine_rodata, one that a
// SET_SEGMENT_AND_OFFSET names on its own, into a rebase of offset off of
// segment seg, in a LEB128 number of the length the old offset had.
func machoRebaseInto(t *testing.T, bin string, b []byte, lo, hi uint64, seg string, off uint64) {
	t.Helper()
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var segs []*macho.Segment
	to := -1
	for _, l := range f.Loads {
		if s, ok := l.(*macho.Segment); ok {
			if s.Name == seg {
				to = len(segs)
			}
			segs = append(segs, s)
		}
	}
	if to < 0 || to > 0xf {
		t.Fatalf("no segment %s in %s", seg, bin)
	}
	for _, l := range f.Loads {
		r := l.Raw()
		if binary.LittleEndian.Uint32(r) != lcDyldInfoOnly {
			continue
		}
		at, n := int(binary.LittleEndian.Uint32(r[8:])), int(binary.LittleEndian.Uint32(r[12:]))
		d := b[at : at+n]
		for i := 0; i < len(d) && d[i] != 0; {
			op := d[i] & 0xf0
			set := i
			i++
			if op != 0x20 {
				continue
			}
			o, err := uleb(d, &i)
			if err != nil {
				t.Fatal(err)
			}
			a := segs[d[set]&0xf].Addr + o
			if (a >= lo && a < hi) || i+1 >= len(d) || d[i] != 0x51 || (d[i+1]&0xf0 != 0x20 && d[i+1] != 0) || !padUleb(d[set+1:i], off) {
				continue
			}
			d[set] = 0x20 | byte(to)
			return
		}
	}
	t.Fatalf("no rebase of %s to move", bin)
}

// padUleb writes v into d as a LEB128 number of exactly len(d) bytes, if it
// fits.
func padUleb(d []byte, v uint64) bool {
	if len(d) < 10 && v>>(7*len(d)) != 0 {
		return false
	}
	for i := range d {
		d[i] = byte(v&0x7f) | 0x80
		v >>= 7
	}
	d[len(d)-1] &= 0x7f
	return true
}

// machoSegAddr is the address of segment seg of bin.
func machoSegAddr(t *testing.T, bin, seg string) uint64 {
	t.Helper()
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if s := f.Segment(seg); s != nil {
		return s.Addr
	}
	t.Fatalf("no segment %s in %s", seg, bin)
	return 0
}

// peImageBase is the image base of bin, and peImportLookup the address of
// the lookup table of its first import descriptor.
func peImageBase(t *testing.T, bin string) uint64 {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return f.OptionalHeader.(*pe.OptionalHeader64).ImageBase
}

func peImportLookup(t *testing.T, bin string) uint64 {
	t.Helper()
	raw, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	at := binFileOffset(t, bin, peDirectoryAddr(t, bin, pe.IMAGE_DIRECTORY_ENTRY_IMPORT))
	lookup := binary.LittleEndian.Uint32(raw[at:])
	if lookup == 0 {
		t.Fatalf("the first import descriptor of %s has no lookup table", bin)
	}
	return peImageBase(t, bin) + uint64(lookup)
}

// elfRelaInto makes the section .note.go.buildid of bin a table of dynamic
// relocations, each of a slot of the table itself.
func elfRelaInto(t *testing.T, bin string, b []byte, arch string) {
	t.Helper()
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rel := uint64(elf.R_X86_64_RELATIVE)
	if arch == "arm64" {
		rel = uint64(elf.R_AARCH64_RELATIVE)
	}
	shoff := binary.LittleEndian.Uint64(b[0x28:])
	for i, s := range f.Sections {
		if s.Name != ".note.go.buildid" {
			continue
		}
		if s.Size < 24 || s.Flags&elf.SHF_ALLOC == 0 {
			t.Fatalf("%s of %s: %d bytes, flags %v", s.Name, bin, s.Size, s.Flags)
		}
		for e := uint64(0); e+24 <= s.Size; e += 24 {
			at := s.Offset + e
			binary.LittleEndian.PutUint64(b[at:], s.Addr+e/3)
			binary.LittleEndian.PutUint64(b[at+8:], rel)
			binary.LittleEndian.PutUint64(b[at+16:], 0)
		}
		sh := shoff + 64*uint64(i)
		binary.LittleEndian.PutUint32(b[sh+4:], uint32(elf.SHT_RELA))
		binary.LittleEndian.PutUint64(b[sh+56:], 24) // sh_entsize
		return
	}
	t.Fatalf("no .note.go.buildid in %s", bin)
}

// TestMetaWrites: a fixup that overlaps a byte of what the check read to
// find the loader's writes is refused, and one that ends before it or
// starts after it is not.
func TestMetaWrites(t *testing.T) {
	for _, c := range []struct {
		lo, n, at uint64
		width     int
		refused   bool
	}{
		{0x1000, 0x10, 0x1000 - 8, 8, false},
		{0x1000, 0x10, 0x1000 - 7, 8, true},
		{0x1000, 0x10, 0x1000 - 4, 4, false},
		{0x1000, 0x10, 0x1000 - 3, 4, true},
		{0x1000, 0x10, 0x1000 + 0xf, 8, true},
		{0x1000, 0x10, 0x1000 + 0x10, 8, false},
		{3, 0x10, 0, 8, true},
		{0x1000, 0, 0x1000, 8, false},
		{0x1000, 0, 0x1000 - 4, 8, false},
	} {
		b := &binFile{fixups: map[uint64]fixup{c.at: {width: c.width, what: "a rebase"}}, fixAt: []uint64{c.at},
			meta: []metaRange{{c.lo, c.n, "the table"}}}
		err := b.metaWrites()
		if (err != nil) != c.refused || (err != nil && !strings.Contains(err.Error(), "inside the table")) {
			t.Errorf("a fixup of %d bytes at %#x, the table at %#x+%#x: %v, refused %v", c.width, c.at, c.lo, c.n, err, c.refused)
		}
	}
}

// elfSectionEnd is the address right after section name of bin.
func elfSectionEnd(t *testing.T, bin, name string) uint64 {
	t.Helper()
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := f.Section(name)
	if s == nil {
		t.Fatalf("no %s in %s", name, bin)
	}
	return s.Addr + s.Size
}

// elfRelaOne writes one dynamic relocation of type typ at addr into the
// section .note.gnu.build-id of bin. With section, the section table names
// that section a table of relocations. Without, the section table stays as
// it is, and a dynamic table in .note.go.buildid, which the program header
// of the stack now names, points the loader to it.
func elfRelaOne(t *testing.T, bin string, b []byte, section bool, typ, addr uint64) {
	t.Helper()
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rela, dyn := f.Section(".note.gnu.build-id"), f.Section(".note.go.buildid")
	if rela == nil || dyn == nil || rela.Size < 24 || dyn.Size < 64 {
		t.Fatalf("no room for a relocation in the notes of %s", bin)
	}
	binary.LittleEndian.PutUint64(b[rela.Offset:], addr)
	binary.LittleEndian.PutUint64(b[rela.Offset+8:], typ)
	binary.LittleEndian.PutUint64(b[rela.Offset+16:], 0)
	if section {
		shoff := binary.LittleEndian.Uint64(b[0x28:])
		for i, s := range f.Sections {
			if s == rela {
				sh := shoff + 64*uint64(i)
				binary.LittleEndian.PutUint32(b[sh+4:], uint32(elf.SHT_RELA))
				binary.LittleEndian.PutUint64(b[sh+32:], 24) // sh_size
				binary.LittleEndian.PutUint64(b[sh+56:], 24) // sh_entsize
			}
		}
		return
	}
	for i, tag := range [][2]uint64{{uint64(elf.DT_RELA), rela.Addr}, {uint64(elf.DT_RELASZ), 24}, {uint64(elf.DT_RELAENT), 24}, {uint64(elf.DT_NULL), 0}} {
		binary.LittleEndian.PutUint64(b[dyn.Offset+16*uint64(i):], tag[0])
		binary.LittleEndian.PutUint64(b[dyn.Offset+16*uint64(i)+8:], tag[1])
	}
	phoff, phnum := binary.LittleEndian.Uint64(b[0x20:]), int(binary.LittleEndian.Uint16(b[0x38:]))
	for i := range phnum {
		ph := phoff + 56*uint64(i)
		if elf.ProgType(binary.LittleEndian.Uint32(b[ph:])) != elf.PT_GNU_STACK {
			continue
		}
		binary.LittleEndian.PutUint32(b[ph:], uint32(elf.PT_DYNAMIC))
		binary.LittleEndian.PutUint32(b[ph+4:], uint32(elf.PF_R))
		binary.LittleEndian.PutUint64(b[ph+8:], dyn.Offset)
		binary.LittleEndian.PutUint64(b[ph+16:], dyn.Addr)
		binary.LittleEndian.PutUint64(b[ph+24:], dyn.Addr)
		binary.LittleEndian.PutUint64(b[ph+32:], 64)
		binary.LittleEndian.PutUint64(b[ph+40:], 64)
		binary.LittleEndian.PutUint64(b[ph+48:], 8)
		return
	}
	t.Fatalf("no PT_GNU_STACK in %s", bin)
}

// peSectionFileEnd is the address right after the bytes the file of bin
// holds of section name.
func peSectionFileEnd(t *testing.T, bin, name string) uint64 {
	t.Helper()
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := f.Section(name)
	if s == nil {
		t.Fatalf("no %s in %s", name, bin)
	}
	return f.OptionalHeader.(*pe.OptionalHeader64).ImageBase + uint64(s.VirtualAddress) + uint64(min(int(s.VirtualSize), int(s.Size)))
}

// machoSectAddr is the address of section sect of segment seg of bin.
func machoSectAddr(t *testing.T, bin, seg, sect string) uint64 {
	t.Helper()
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, s := range f.Sections {
		if s.Seg == seg && s.Name == sect {
			return s.Addr
		}
	}
	t.Fatalf("no %s,%s in %s", seg, sect, bin)
	return 0
}

// TestPlacedEdges: a fixup that ends at the last byte of a section the
// loader may write is taken, and one that starts before it or ends a byte
// past it is not.
func TestPlacedEdges(t *testing.T) {
	const lo, n = 0x1000, 0x100
	for _, c := range []struct {
		at    uint64
		width int
		ok    bool
	}{
		{lo, 8, true}, {lo + n - 8, 8, true}, {lo + n - 7, 8, false}, {lo + n - 4, 4, true},
		{lo + n - 3, 4, false}, {lo - 1, 8, false}, {lo + n, 8, false}, {^uint64(0) - 3, 8, false},
	} {
		b := &binFile{fixups: map[uint64]fixup{c.at: {width: c.width, what: "a rebase"}}, fixAt: []uint64{c.at},
			writable: []metaRange{{lo, n, "data"}}}
		if err := b.placed(); (err == nil) != c.ok {
			t.Errorf("a fixup of %d bytes at %#x in %#x+%#x: %v, taken %v", c.width, c.at, lo, n, err, c.ok)
		}
	}
}

// mapHeader is a header of bin that maps file bytes to addresses: a
// program header of ELF, a segment of Mach-O, a section of PE. at is where
// it is in the file; addr, off and n where it maps from and to, with
// fields to write each.
type mapHeader struct {
	at, addr, off, n uint64
	setOff, setN     func(b []byte, v uint64)
}

// mapHeaders gives the headers of bin that say where the loader maps the
// file (segments), or where the sections are in the file (sections).
func mapHeaders(t *testing.T, bin string, b []byte, segments bool) []mapHeader {
	t.Helper()
	put32 := func(at uint64) func(b []byte, v uint64) {
		return func(b []byte, v uint64) { binary.LittleEndian.PutUint32(b[at:], uint32(v)) }
	}
	put64 := func(at uint64) func(b []byte, v uint64) {
		return func(b []byte, v uint64) { binary.LittleEndian.PutUint64(b[at:], v) }
	}
	u32 := func(at uint64) uint64 { return uint64(binary.LittleEndian.Uint32(b[at:])) }
	u64 := func(at uint64) uint64 { return binary.LittleEndian.Uint64(b[at:]) }
	var hs []mapHeader
	switch {
	case string(b[:4]) == "\x7fELF" && segments:
		phoff, phnum := u64(0x20), binary.LittleEndian.Uint16(b[0x38:])
		for i := range uint64(phnum) {
			ph := phoff + 56*i
			if elf.ProgType(u32(ph)) == elf.PT_LOAD {
				hs = append(hs, mapHeader{ph, u64(ph + 16), u64(ph + 8), u64(ph + 32), put64(ph + 8), put64(ph + 32)})
			}
		}
	case string(b[:4]) == "\x7fELF":
		shoff, shnum := u64(0x28), binary.LittleEndian.Uint16(b[0x3c:])
		for i := range uint64(shnum) {
			sh := shoff + 64*i
			if elf.SectionType(u32(sh+4)) == elf.SHT_PROGBITS && u64(sh+8)&uint64(elf.SHF_ALLOC) != 0 {
				hs = append(hs, mapHeader{sh, u64(sh + 16), u64(sh + 24), u64(sh + 32), put64(sh + 24), put64(sh + 32)})
			}
		}
	case u32(0) == uint64(macho.Magic64):
		at, ncmd := uint64(32), u32(16)
		for range ncmd {
			if u32(at) == uint64(macho.LoadCmdSegment64) {
				if segments {
					hs = append(hs, mapHeader{at, u64(at + 24), u64(at + 40), u64(at + 48), put64(at + 40), put64(at + 48)})
				} else {
					for j := range u32(at + 64) {
						s := at + 72 + 80*j
						hs = append(hs, mapHeader{s, u64(s + 32), u32(s + 48), u64(s + 40), put32(s + 48), put64(s + 40)})
					}
				}
			}
			at += u32(at + 4)
		}
	default:
		ph := uint64(peHeader(t, b))
		base := u64(ph + 24)
		sh := ph + u32(ph-20+16)&0xffff
		for i := range u32(ph-20+2) & 0xffff {
			s := sh + 40*i
			hs = append(hs, mapHeader{s, base + u32(s+12), u32(s + 20), min64(u32(s+8), u32(s+16)), put32(s + 20), put32(s + 16)})
		}
	}
	return hs
}

// mapHeaderOf is the header of hs that holds addr.
func mapHeaderOf(t *testing.T, hs []mapHeader, addr uint64) mapHeader {
	t.Helper()
	for _, h := range hs {
		if addr >= h.addr && addr-h.addr < h.n {
			return h
		}
	}
	t.Fatalf("no header maps %#x", addr)
	return mapHeader{}
}

// shortSegment makes the segment of bin that holds addr (a section of PE)
// end in the file before addr, leaving the sections and the bytes as they
// are: the loader fills the rest with zeros.
func shortSegment(t *testing.T, bin string, b []byte, addr uint64) {
	t.Helper()
	h := mapHeaderOf(t, mapHeaders(t, bin, b, true), addr)
	n := addr - h.addr
	if string(b[:2]) == "MZ" {
		n &^= 0x1ff
	}
	h.setN(b, n)
}

// fileSize is the size of bin.
func fileSize(t *testing.T, bin string) uint64 {
	t.Helper()
	st, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	return uint64(st.Size())
}

// sectionFromCopy copies the section of bin that holds addr past the end of
// the file and points the section at the copy, then changes the byte at addr
// where the loader maps it. In PE, where the sections are what the loader
// maps, the copy starts 0x100 bytes past a multiple of 0x200, and the
// loader reads the section from that multiple.
func sectionFromCopy(t *testing.T, bin string, b []byte, addr uint64) {
	t.Helper()
	h := mapHeaderOf(t, mapHeaders(t, bin, b, false), addr)
	dst := (fileSize(t, bin) + 0x1ff) &^ 0x1ff
	if string(b[:2]) == "MZ" {
		dst += 0x100
	}
	if dst+h.n > uint64(len(b)) {
		t.Fatalf("no room for a copy of %d bytes", h.n)
	}
	copy(b[dst:dst+h.n], b[h.off:h.off+h.n])
	h.setOff(b, dst)
	b[h.off+addr-h.addr] ^= 0xff
}

// elfLoadOver adds a PT_LOAD (in place of PT_GNU_STACK) that maps a page
// past the end of the file over the page of addr: a copy of that page with
// the byte at addr changed.
func elfLoadOver(t *testing.T, bin string, b []byte, arch string, addr uint64) {
	t.Helper()
	page := uint64(0x1000)
	if arch == "arm64" {
		page = 0x10000
	}
	lo := addr &^ (page - 1)
	h := mapHeaderOf(t, mapHeaders(t, bin, b, true), lo)
	src := h.off + lo - h.addr
	dst := (fileSize(t, bin) + page - 1) &^ (page - 1)
	copy(b[dst:dst+page], b[src:src+page])
	b[dst+addr-lo] ^= 0xff
	phoff, phnum := binary.LittleEndian.Uint64(b[0x20:]), binary.LittleEndian.Uint16(b[0x38:])
	for i := range uint64(phnum) {
		ph := phoff + 56*i
		if elf.ProgType(binary.LittleEndian.Uint32(b[ph:])) != elf.PT_GNU_STACK {
			continue
		}
		binary.LittleEndian.PutUint32(b[ph:], uint32(elf.PT_LOAD))
		binary.LittleEndian.PutUint32(b[ph+4:], uint32(elf.PF_R|elf.PF_X))
		for j, v := range []uint64{dst, lo, lo, page, page, page} {
			binary.LittleEndian.PutUint64(b[ph+8+8*uint64(j):], v)
		}
		return
	}
	t.Fatalf("no PT_GNU_STACK in %s", bin)
}

// TestMapFile: the check reads the bytes a mapping takes from the file and
// none past them, and refuses mappings that overlap in a page, map more of
// the file than of memory or more than the file has, and a section that is
// not where its mapping takes it from.
func TestMapFile(t *testing.T) {
	raw := make([]byte, 0x3000)
	for i := range raw {
		raw[i] = byte(i)
	}
	b := &binFile{}
	if err := b.mapFile(raw, []mapping{{"text", 0x10000, 0x1000, 0x10, 0x100}, {"data", 0x11000, 0x2000, 0x800, 0x1000}},
		[]mapping{{".text", 0x10008, 0x1008, 8, 8}, {".data", 0x11000, 0x2000, 0x800, 0x800}}, 0x1000); err != nil {
		t.Fatal(err)
	}
	if d, err := b.read(0x10008, 8); err != nil || d[0] != raw[0x1008] {
		t.Errorf("the last 8 bytes text maps from the file: %v", err)
	}
	if _, err := b.read(0x10009, 8); err == nil || !strings.Contains(err.Error(), "the loader maps from the file") {
		t.Errorf("a read past what text maps from the file: %v", err)
	}
	for name, c := range map[string]struct {
		ms, secs []mapping
		want     string
	}{
		"in one page":       {[]mapping{{"a", 0x10000, 0, 0x10, 0x10}, {"b", 0x10800, 0x800, 0x10, 0x10}}, nil, "overlap"},
		"past an empty one": {[]mapping{{"a", 0x10000, 0, 0x3000, 0x3000}, {"e", 0x11000, 0x1000, 0, 0}, {"b", 0x12000, 0x2000, 0x10, 0x10}}, nil, "overlap"},
		"past a small one":  {[]mapping{{"a", 0x10000, 0, 0x3000, 0x3000}, {"c", 0x11000, 0x1000, 0x10, 0x10}, {"b", 0x12000, 0x2000, 0x10, 0x10}}, nil, "overlap"},
		"more file":         {[]mapping{{"a", 0x10000, 0, 0x20, 0x10}}, nil, "into 0x10 bytes of memory"},
		"past the file":     {[]mapping{{"a", 0x10000, 0x2ff0, 0x20, 0x20}}, nil, "of a file of"},
		"section elsewhere": {[]mapping{{"a", 0x10000, 0, 0x100, 0x100}}, []mapping{{".s", 0x10010, 0x20, 8, 8}}, "not where its segment maps it"},
		"section past":      {[]mapping{{"a", 0x10000, 0, 0x100, 0x200}}, []mapping{{".s", 0x100f8, 0xf8, 0x10, 0x10}}, "not where its segment maps it"},
		"at the top":        {[]mapping{{"a", ^uint64(0) - 0x10, 0, 0x10, 0x10}}, nil, "top of the address space"},
	} {
		if err := (&binFile{}).mapFile(raw, c.ms, c.secs, 0x1000); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}
