// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The linked binary the post-link check reads, in the three formats of the
// targets the module builds for: ELF (linux), Mach-O (darwin) and PE
// (windows). Each gives the check the same view: the symbols with their
// addresses and sizes, the bytes the file holds at an address, and the places
// the loader writes when it starts the program (its fixups).
//
// A position-independent program is placed by the loader at an address of
// its choosing, so a pointer in it only holds once the loader has added the
// distance it moved the program (the slide). The Go linker writes each
// pointer in the file as the address at the image base the file names, and
// records the place for the loader: in Mach-O a rebase of the dyld info
// (LC_DYLD_INFO_ONLY, cmd/link/internal/ld/macho.go machoDyldInfo), in PE a
// DIR64 entry of the base relocation table (.reloc, ld/pe.go
// addPEBaseReloc). ld64, which links a darwin program with cgo, writes
// chained fixups instead (LC_DYLD_CHAINED_FIXUPS): the slot holds the target
// as an offset from the image base and the distance to the next slot, which
// the loader replaces with the pointer. So the check holds a pointer to two
// things: the address the slot gives at the image base (the bytes, or the
// target of its chained entry) is its target, and the loader slides exactly
// that slot. Every other byte the check reads must be one the loader leaves
// alone: a fixup inside the code or data the check read, at a place that is
// no pointer it checked, fails (loaderWrites).
//
// What the check does not read it refuses, so nothing passes unread: an ELF
// PIE (ET_DYN), chained fixups of a pointer format other than the two of
// arm64 and x86_64 (DYLD_CHAINED_PTR_64 and _64_OFFSET, not arm64e), lazy or
// weak binds, threaded binds, Mach-O local relocations, PE base relocations
// other than DIR64 and HIGHLOW, and a PE directory that makes the loader write
// or run more than the imports and the slots of the load config (TLS, delay
// and bound imports, CLR, and the parts of the load config that patch code).
// The PE imports are fixups too: the loader fills each slot of an import
// address table. A fixup of what the check reads to find the fixups (the
// headers, the load commands, the tables of fixups and of imports) is
// refused, as the loader would apply it before it reads them (metaWrites).
package gen

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// binSym is a symbol of the linked binary.
type binSym struct {
	Value, Size uint64
}

// binSec is the part of a section the file holds bytes for, at its address.
type binSec struct {
	name string
	addr uint64
	size uint64
	load func() ([]byte, error)
	data []byte
}

// fixup is a place the loader writes when it starts the program.
type fixup struct {
	width int
	// slide: the loader adds the slide to the 8-byte pointer the file
	// holds. Any other fixup (a bind to an import, a 4-byte relocation)
	// writes a value the file does not show.
	slide bool
	what  string
	// chained: the file holds a chained entry, not the pointer; target is
	// the pointer it makes at the image base.
	chained bool
	target  uint64
}

type binFile struct {
	format string // "ELF", "Mach-O", "PE"
	goos   string // the system the format is for: linux, darwin, windows
	syms   map[string]binSym
	secs   []*binSec
	closer io.Closer
	// pie: the loader may place the program at another address, so every
	// pointer needs a slide fixup.
	pie bool
	// fixups by the address of their first byte, and fixAt the same
	// addresses sorted.
	fixups map[uint64]fixup
	fixAt  []uint64
	// touched are the fixups inside bytes the check read, and pointers the
	// slots it checked as pointers (with a slide fixup where pie).
	touched  map[uint64]bool
	pointers map[uint64]bool
	// meta is what the check read to find where the loader writes (the
	// headers, the load commands, the tables of fixups and of imports), so
	// no fixup may be there (metaWrites).
	meta []metaRange
}

type metaRange struct {
	lo, n uint64
	what  string
}

// binArch is the machine of a linked binary in each format.
var binArch = map[string]struct {
	elf   elf.Machine
	macho macho.Cpu
	pe    uint16
}{
	"amd64": {elf.EM_X86_64, macho.CpuAmd64, pe.IMAGE_FILE_MACHINE_AMD64},
	"arm64": {elf.EM_AARCH64, macho.CpuArm64, pe.IMAGE_FILE_MACHINE_ARM64},
}

// openBin opens a linked executable of the architecture arch (amd64 or
// arm64) in any of the three formats.
func openBin(path, arch string) (*binFile, error) {
	m, ok := binArch[arch]
	if !ok {
		return nil, fmt.Errorf("no binary format known for %s", arch)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.Close()
	var b *binFile
	switch {
	case string(magic[:]) == "\x7fELF":
		b, err = openELF(path, m.elf)
	case binary.LittleEndian.Uint32(magic[:]) == macho.Magic64:
		b, err = openMachO(path, m.macho)
	case magic[0] == 'M' && magic[1] == 'Z':
		b, err = openPE(path, m.pe)
	default:
		return nil, fmt.Errorf("%s: not an ELF, Mach-O or PE executable", path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	b.touched, b.pointers = map[uint64]bool{}, map[uint64]bool{}
	for a := range b.fixups {
		b.fixAt = append(b.fixAt, a)
	}
	sort.Slice(b.fixAt, func(i, j int) bool { return b.fixAt[i] < b.fixAt[j] })
	if err := b.metaWrites(); err != nil {
		b.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// metaWrites refuses a fixup inside what the check read to find where the
// loader writes. The loader applies its fixups before it reads its imports,
// and a fixup of a table or a header changes what the loader then reads, so
// the file would not say where it writes.
func (b *binFile) metaWrites() error {
	for _, m := range b.meta {
		lo := m.lo - min64(m.lo, 7)
		i := sort.Search(len(b.fixAt), func(i int) bool { return b.fixAt[i] >= lo })
		for ; i < len(b.fixAt); i++ {
			a := b.fixAt[i]
			if a >= m.lo && a-m.lo >= m.n {
				break
			}
			if a >= m.lo || m.lo-a < uint64(b.fixups[a].width) {
				return fmt.Errorf("%s at %#x is inside %s, which the check reads to find where the loader writes", b.fixups[a].what, a, m.what)
			}
		}
	}
	return nil
}

func (b *binFile) Close() error {
	if b.closer == nil {
		return nil
	}
	return b.closer.Close()
}

func openELF(path string, m elf.Machine) (*binFile, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	if f.Machine != m {
		f.Close()
		return nil, fmt.Errorf("an ELF file for %v, not %v", f.Machine, m)
	}
	// An ELF PIE holds its pointers for the loader in .rela.dyn, which this
	// check does not read. The default build of linux is not one.
	if f.Type != elf.ET_EXEC {
		f.Close()
		return nil, fmt.Errorf("not an ELF executable linked at a fixed address (%v): the check does not read the dynamic relocations of an ELF PIE, build it with -buildmode=exe", f.Type)
	}
	b := &binFile{format: "ELF", goos: "linux", syms: map[string]binSym{}, closer: f, fixups: map[uint64]fixup{}}
	syms, err := f.Symbols()
	if err != nil {
		f.Close()
		return nil, err
	}
	for _, s := range syms {
		b.syms[s.Name] = binSym{s.Value, s.Size}
	}
	for _, s := range f.Sections {
		if s.Type == elf.SHT_PROGBITS {
			b.secs = append(b.secs, &binSec{name: s.Name, addr: s.Addr, size: s.Size, load: s.Data})
		}
	}
	// A fixed executable is not moved, but a dynamic one still has the
	// loader write its relocations: none of them may be in what the check
	// reads (a static Go program has none).
	for _, s := range f.Sections {
		if s.Flags&elf.SHF_ALLOC == 0 || (s.Type != elf.SHT_RELA && s.Type != elf.SHT_REL) {
			continue
		}
		d, err := s.Data()
		if err != nil {
			f.Close()
			return nil, err
		}
		step := 24
		if s.Type == elf.SHT_REL {
			step = 16
		}
		b.meta = append(b.meta, metaRange{s.Addr, s.Size, "the dynamic relocations in " + s.Name})
		for i := 0; i+step <= len(d); i += step {
			at := binary.LittleEndian.Uint64(d[i:])
			typ := binary.LittleEndian.Uint64(d[i+8:]) & 0xffffffff
			if err := b.addFixup(at, fixup{width: 8, what: fmt.Sprintf("a dynamic relocation of type %d in %s", typ, s.Name)}); err != nil {
				f.Close()
				return nil, err
			}
		}
	}
	return b, nil
}

// Mach-O constants of mach-o/loader.h that debug/macho does not name.
const (
	machoPIE           = 0x200000
	lcDyldInfo         = 0x22
	lcDyldInfoOnly     = 0x80000022
	lcDyldChained      = 0x80000034
	machoZerofill      = 0x1
	machoGBZerofill    = 0xc
	machoTLVZerofill   = 0x12
	machoRebasePointer = 1
	machoHeaderSize    = 32 // mach_header_64
)

func openMachO(path string, cpu macho.Cpu) (*binFile, error) {
	f, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*binFile, error) { f.Close(); return nil, e }
	if f.Cpu != cpu {
		return fail(fmt.Errorf("a Mach-O file for %v, not %v", f.Cpu, cpu))
	}
	if f.Type != macho.TypeExec {
		return fail(fmt.Errorf("a Mach-O file of type %v, not an executable", f.Type))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fail(err)
	}
	b := &binFile{format: "Mach-O", goos: "darwin", syms: map[string]binSym{}, closer: f, pie: f.Flags&machoPIE != 0, fixups: map[uint64]fixup{}}
	for _, s := range f.Sections {
		switch s.Flags & 0xff {
		case machoZerofill, machoGBZerofill, machoTLVZerofill:
			continue
		}
		if s.Offset == 0 {
			continue
		}
		b.secs = append(b.secs, &binSec{name: s.Seg + "," + s.Name, addr: s.Addr, size: s.Size, load: s.Data})
	}
	if f.Symtab == nil {
		return fail(fmt.Errorf("no symbol table (a binary linked with -s cannot be checked)"))
	}
	// Mach-O gives no symbol sizes: each runs to the next symbol of its
	// section, or to the end of the section (sizeToNext).
	var named []namedSym
	for _, s := range f.Symtab.Syms {
		if s.Sect == 0 || s.Type&0xe0 != 0 || s.Type&0xe != 0xe { // N_SECT, no debugging entry
			continue
		}
		if int(s.Sect) > len(f.Sections) {
			return fail(fmt.Errorf("symbol %s in section %d of %d", s.Name, s.Sect, len(f.Sections)))
		}
		sec := f.Sections[s.Sect-1]
		named = append(named, namedSym{strings.TrimPrefix(s.Name, "_"), s.Value, int(s.Sect), sec.Addr + sec.Size})
	}
	b.syms = sizeToNext(named)
	if f.Dysymtab != nil && (f.Dysymtab.Nlocrel != 0 || f.Dysymtab.Nextrel != 0) {
		return fail(fmt.Errorf("%d local and %d external relocations, which the check does not read", f.Dysymtab.Nlocrel, f.Dysymtab.Nextrel))
	}
	var segs []*macho.Segment
	for _, l := range f.Loads {
		if s, ok := l.(*macho.Segment); ok {
			segs = append(segs, s)
			// The segment that maps the start of the file holds the
			// header and the load commands, and __LINKEDIT the tables of
			// fixups.
			if s.Offset == 0 && s.Filesz != 0 {
				b.meta = append(b.meta, metaRange{s.Addr, machoHeaderSize + uint64(f.Cmdsz), "the header and load commands"})
			}
			if s.Name == "__LINKEDIT" {
				b.meta = append(b.meta, metaRange{s.Addr, s.Memsz, "__LINKEDIT"})
			}
		}
	}
	info := 0
	for _, l := range f.Loads {
		r := l.Raw()
		if _, ok := l.(*macho.Segment); ok || len(r) < 8 {
			continue
		}
		switch binary.LittleEndian.Uint32(r) {
		case lcDyldChained:
			if len(r) < 16 {
				return fail(fmt.Errorf("a short LC_DYLD_CHAINED_FIXUPS"))
			}
			info++
			off, n := uint64(binary.LittleEndian.Uint32(r[8:])), uint64(binary.LittleEndian.Uint32(r[12:]))
			if off+n > uint64(len(raw)) {
				return fail(fmt.Errorf("chained fixups past the end of the file"))
			}
			if err := b.machoChained(raw[off:off+n], segs, raw); err != nil {
				return fail(fmt.Errorf("chained fixups: %w", err))
			}
		case lcDyldInfo, lcDyldInfoOnly:
			if len(r) < 48 {
				return fail(fmt.Errorf("a short LC_DYLD_INFO"))
			}
			info++
			part := func(i int) ([]byte, error) {
				off, n := uint64(binary.LittleEndian.Uint32(r[8+8*i:])), uint64(binary.LittleEndian.Uint32(r[12+8*i:]))
				if off+n > uint64(len(raw)) {
					return nil, fmt.Errorf("dyld info past the end of the file")
				}
				return raw[off : off+n], nil
			}
			rebase, err := part(0)
			if err != nil {
				return fail(err)
			}
			bind, err := part(1)
			if err != nil {
				return fail(err)
			}
			for i, name := range []string{"weak bind", "lazy bind"} {
				if d, err := part(2 + i); err != nil || len(d) != 0 {
					return fail(fmt.Errorf("%s info (%d bytes), which the check does not read", name, len(d)))
				}
			}
			if err := b.machoRebase(rebase, segs); err != nil {
				return fail(fmt.Errorf("rebase info: %w", err))
			}
			if err := b.machoBind(bind, segs); err != nil {
				return fail(fmt.Errorf("bind info: %w", err))
			}
		}
	}
	switch {
	case info > 1:
		return fail(fmt.Errorf("more than one table of fixups (dyld info and chained fixups)"))
	case b.pie && info == 0:
		return fail(fmt.Errorf("a PIE with no dyld info and no chained fixups: where the loader slides its pointers is not known"))
	}
	return b, nil
}

// Pointer formats of chained fixups (mach-o/fixup-chains.h).
const (
	chainedPtr64       = 2 // DYLD_CHAINED_PTR_64: the target is an address
	chainedPtr64Offset = 6 // DYLD_CHAINED_PTR_64_OFFSET: the target is an offset from the image base
	chainedStartNone   = 0xffff
	chainedStartMulti  = 0x8000
)

// machoChained reads the chained fixups of mach-o/fixup-chains.h: for each
// segment with fixups, the first slot of each page, and from each slot the
// next one, 4 bytes times its next field further on. A rebase holds its
// target in the low 36 bits (an address, or an offset from the image base,
// the address of __TEXT) and the top byte of the pointer in the 8 above; a
// bind (the top bit) is a pointer to an import, which the file does not show.
func (b *binFile) machoChained(d []byte, segs []*macho.Segment, raw []byte) error {
	u16 := func(at int) (uint64, error) {
		if at < 0 || at+2 > len(d) {
			return 0, fmt.Errorf("cut short at %d", at)
		}
		return uint64(binary.LittleEndian.Uint16(d[at:])), nil
	}
	u32 := func(at int) (uint64, error) {
		if at < 0 || at+4 > len(d) {
			return 0, fmt.Errorf("cut short at %d", at)
		}
		return uint64(binary.LittleEndian.Uint32(d[at:])), nil
	}
	if v, err := u32(0); err != nil || v != 0 {
		return fmt.Errorf("version %d, not 0", v)
	}
	starts, err := u32(4)
	if err != nil {
		return err
	}
	image := uint64(0)
	for _, s := range segs {
		if s.Name == "__TEXT" {
			image = s.Addr
		}
	}
	if image == 0 {
		return fmt.Errorf("no __TEXT segment")
	}
	nseg, err := u32(int(starts))
	if err != nil {
		return err
	}
	if nseg > uint64(len(segs)) {
		return fmt.Errorf("fixups for %d segments, the file has %d", nseg, len(segs))
	}
	for si := 0; si < int(nseg); si++ {
		so, err := u32(int(starts) + 4 + 4*si)
		if err != nil {
			return err
		}
		if so == 0 {
			continue
		}
		at := int(starts + so)
		psize, err1 := u16(at + 4)
		format, err2 := u16(at + 6)
		segOff, err3 := u32(at + 8) // the low half of a 64-bit offset
		high, err4 := u32(at + 12)
		pages, err5 := u16(at + 20)
		for _, e := range []error{err1, err2, err3, err4, err5} {
			if e != nil {
				return e
			}
		}
		seg := segs[si]
		// Each entry is read from the bytes of its segment in the file, so
		// those are first held to the file.
		if seg.Offset > uint64(len(raw)) || seg.Filesz > uint64(len(raw))-seg.Offset {
			return fmt.Errorf("segment %s has %#x bytes at %#x of the file, outside its %d bytes", seg.Name, seg.Filesz, seg.Offset, len(raw))
		}
		if high != 0 || image+segOff != seg.Addr {
			return fmt.Errorf("segment %s at offset %#x of the image, the file has it at %#x", seg.Name, high<<32|segOff, seg.Addr-image)
		}
		if format != chainedPtr64 && format != chainedPtr64Offset {
			return fmt.Errorf("pointer format %d in %s, which the check does not read", format, seg.Name)
		}
		for p := 0; p < int(pages); p++ {
			start, err := u16(at + 22 + 2*p)
			if err != nil {
				return err
			}
			if start == chainedStartNone {
				continue
			}
			if start&chainedStartMulti != 0 {
				return fmt.Errorf("a page of %s with several starts, which the check does not read", seg.Name)
			}
			off := uint64(p)*psize + start
			for n := 0; ; n++ {
				if off+8 > seg.Filesz || n > 1<<20 {
					return fmt.Errorf("a chained entry at %#x of %s, outside its bytes in the file", off, seg.Name)
				}
				v := binary.LittleEndian.Uint64(raw[seg.Offset+off:])
				a := seg.Addr + off
				f := fixup{width: 8, chained: true, what: fmt.Sprintf("a chained bind to import %d", v&0xffffff)}
				if v>>63 == 0 {
					t := v & (1<<36 - 1)
					if format == chainedPtr64Offset {
						t += image
					}
					f = fixup{width: 8, slide: true, chained: true, target: v>>36&0xff<<56 | t, what: "a chained rebase"}
				}
				if err := b.addFixup(a, f); err != nil {
					return err
				}
				next := v >> 51 & 0xfff
				if next == 0 {
					break
				}
				off += 4 * next
			}
		}
	}
	return nil
}

// addFixup records the fixup f at a, and refuses a second record of the
// same place: two rebases or base relocations of one pointer slide it twice,
// and a bind and a rebase of one slot leave it to the order the loader takes
// them in, so the file says what the loader does only with one.
func (b *binFile) addFixup(a uint64, f fixup) error {
	if g, dup := b.fixups[a]; dup {
		return fmt.Errorf("two fixups at %#x: %s and %s", a, g.what, f.what)
	}
	b.fixups[a] = f
	return nil
}

// uleb reads an unsigned LEB128 number at d[*i].
func uleb(d []byte, i *int) (uint64, error) {
	var v uint64
	for s := uint(0); *i < len(d); s += 7 {
		c := d[*i]
		*i++
		if s >= 64 {
			return 0, fmt.Errorf("a LEB128 number too long")
		}
		v |= uint64(c&0x7f) << s
		if c&0x80 == 0 {
			return v, nil
		}
	}
	return 0, fmt.Errorf("a LEB128 number cut short")
}

// machoSlot is the address of the slot of width bytes at offset off in
// segment seg, which must hold all of it.
func machoSlot(segs []*macho.Segment, seg, off, width uint64) (uint64, error) {
	if seg >= uint64(len(segs)) || off > segs[seg].Memsz || width > segs[seg].Memsz-off {
		return 0, fmt.Errorf("offset %#x of segment %d is outside it", off, seg)
	}
	return segs[seg].Addr + off, nil
}

// machoRebase reads the rebase opcodes of mach-o/loader.h (REBASE_OPCODE_*).
func (b *binFile) machoRebase(d []byte, segs []*macho.Segment) error {
	var seg, off, typ uint64
	segSet := false
	rebase := func() error {
		if !segSet {
			return fmt.Errorf("a rebase before its segment")
		}
		f := fixup{width: 4, what: fmt.Sprintf("a rebase of type %d", typ)}
		if typ == machoRebasePointer {
			f = fixup{width: 8, slide: true, what: "a rebase"}
		}
		a, err := machoSlot(segs, seg, off, uint64(f.width))
		if err != nil {
			return err
		}
		off += 8
		return b.addFixup(a, f)
	}
	for i := 0; i < len(d); {
		op, imm := d[i]&0xf0, uint64(d[i]&0x0f)
		i++
		var err error
		var n, skip uint64
		switch op {
		case 0x00: // DONE
			return nil
		case 0x10: // SET_TYPE_IMM
			typ = imm
		case 0x20: // SET_SEGMENT_AND_OFFSET_ULEB
			seg, segSet = imm, true
			off, err = uleb(d, &i)
		case 0x30: // ADD_ADDR_ULEB
			n, err = uleb(d, &i)
			off += n
		case 0x40: // ADD_ADDR_IMM_SCALED
			off += imm * 8
		case 0x50: // DO_REBASE_IMM_TIMES
			for ; imm > 0 && err == nil; imm-- {
				err = rebase()
			}
		case 0x60: // DO_REBASE_ULEB_TIMES
			if n, err = uleb(d, &i); err == nil && n > uint64(len(b.fixups))+1<<24 {
				err = fmt.Errorf("a run of %d rebases", n)
			}
			for ; n > 0 && err == nil; n-- {
				err = rebase()
			}
		case 0x70: // DO_REBASE_ADD_ADDR_ULEB
			if err = rebase(); err == nil {
				n, err = uleb(d, &i)
				off += n
			}
		case 0x80: // DO_REBASE_ULEB_TIMES_SKIPPING_ULEB
			if n, err = uleb(d, &i); err == nil {
				skip, err = uleb(d, &i)
			}
			if err == nil && n > 1<<24 {
				err = fmt.Errorf("a run of %d rebases", n)
			}
			for ; n > 0 && err == nil; n-- {
				if err = rebase(); err == nil {
					off += skip
				}
			}
		default:
			err = fmt.Errorf("unknown opcode %#02x", op)
		}
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("no DONE at the end")
}

// machoBind reads the bind opcodes (BIND_OPCODE_*): each bound slot is a
// fixup the file does not show.
func (b *binFile) machoBind(d []byte, segs []*macho.Segment) error {
	var seg, off uint64
	segSet := false
	sym := ""
	bind := func() error {
		if !segSet {
			return fmt.Errorf("a bind before its segment")
		}
		a, err := machoSlot(segs, seg, off, 8)
		if err != nil {
			return err
		}
		off += 8
		return b.addFixup(a, fixup{width: 8, what: "a bind to " + sym})
	}
	for i := 0; i < len(d); {
		op, imm := d[i]&0xf0, uint64(d[i]&0x0f)
		i++
		var err error
		var n, skip uint64
		switch op {
		case 0x00: // DONE
			return nil
		case 0x10, 0x30, 0x50: // SET_DYLIB_ORDINAL_IMM, SET_DYLIB_SPECIAL_IMM, SET_TYPE_IMM
		case 0x20, 0x60: // SET_DYLIB_ORDINAL_ULEB, SET_ADDEND_SLEB (read as LEB128 bytes)
			_, err = uleb(d, &i)
		case 0x40: // SET_SYMBOL_TRAILING_FLAGS_IMM, then the name
			e := i
			for e < len(d) && d[e] != 0 {
				e++
			}
			if e == len(d) {
				return fmt.Errorf("a symbol name cut short")
			}
			sym, i = string(d[i:e]), e+1
		case 0x70: // SET_SEGMENT_AND_OFFSET_ULEB
			seg, segSet = imm, true
			off, err = uleb(d, &i)
		case 0x80: // ADD_ADDR_ULEB
			n, err = uleb(d, &i)
			off += n
		case 0x90: // DO_BIND
			err = bind()
		case 0xa0: // DO_BIND_ADD_ADDR_ULEB
			if err = bind(); err == nil {
				n, err = uleb(d, &i)
				off += n
			}
		case 0xb0: // DO_BIND_ADD_ADDR_IMM_SCALED
			if err = bind(); err == nil {
				off += imm * 8
			}
		case 0xc0: // DO_BIND_ULEB_TIMES_SKIPPING_ULEB
			if n, err = uleb(d, &i); err == nil {
				skip, err = uleb(d, &i)
			}
			if err == nil && n > 1<<24 {
				err = fmt.Errorf("a run of %d binds", n)
			}
			for ; n > 0 && err == nil; n-- {
				if err = bind(); err == nil {
					off += skip
				}
			}
		default: // THREADED and anything newer
			err = fmt.Errorf("opcode %#02x, which the check does not read", op)
		}
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("no DONE at the end")
}

// PE base relocation types (IMAGE_REL_BASED_*).
const (
	peRelAbsolute = 0
	peRelHighLow  = 3
	peRelDir64    = 10
)

func openPE(path string, m uint16) (*binFile, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*binFile, error) { f.Close(); return nil, e }
	if f.Machine != m {
		return fail(fmt.Errorf("a PE file for machine %#x, not %#x", f.Machine, m))
	}
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return fail(fmt.Errorf("not a PE32+ file"))
	}
	if f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || f.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return fail(fmt.Errorf("not an executable (characteristics %#x)", f.Characteristics))
	}
	base := oh.ImageBase
	b := &binFile{format: "PE", goos: "windows", syms: map[string]binSym{}, closer: f, fixups: map[uint64]fixup{}}
	b.meta = append(b.meta, metaRange{base, uint64(oh.SizeOfHeaders), "the headers"})
	for _, s := range f.Sections {
		n := uint64(s.VirtualSize)
		if raw := uint64(s.Size); raw < n {
			n = raw // the rest is zero in memory, and not in the file
		}
		if n == 0 {
			continue
		}
		b.secs = append(b.secs, &binSec{name: s.Name, addr: base + uint64(s.VirtualAddress), size: n, load: s.Data})
	}
	var named []namedSym
	for _, s := range f.Symbols {
		if s.SectionNumber <= 0 || int(s.SectionNumber) > len(f.Sections) {
			continue
		}
		sec := f.Sections[s.SectionNumber-1]
		lo := base + uint64(sec.VirtualAddress)
		named = append(named, namedSym{s.Name, lo + uint64(s.Value), int(s.SectionNumber), lo + uint64(sec.VirtualSize)})
	}
	if len(named) == 0 {
		return fail(fmt.Errorf("no symbol table (a binary linked with -s cannot be checked)"))
	}
	b.syms = sizeToNext(named)
	dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC]
	relocs := dir.Size != 0
	stripped := f.Characteristics&pe.IMAGE_FILE_RELOCS_STRIPPED != 0
	if relocs && stripped {
		return fail(fmt.Errorf("base relocations in a file that says it has none"))
	}
	// The loader places an image elsewhere when it asks for that (dynamic
	// base, the default of Go) or when its preferred base is taken and it
	// has relocations to do it with.
	b.pie = oh.DllCharacteristics&pe.IMAGE_DLLCHARACTERISTICS_DYNAMIC_BASE != 0 || relocs
	if b.pie && !relocs {
		return fail(fmt.Errorf("a dynamic base with no base relocations: where the loader slides its pointers is not known"))
	}
	if err := b.peLoaderWrites(f, oh); err != nil {
		return fail(err)
	}
	if !relocs {
		return b, nil
	}
	table, err := peRVA(f, uint64(dir.VirtualAddress), uint64(dir.Size))
	if err != nil {
		return fail(fmt.Errorf("the base relocation table: %w", err))
	}
	b.meta = append(b.meta, metaRange{base + uint64(dir.VirtualAddress), uint64(dir.Size), "the base relocation table"})
	for i := 0; i < len(table); {
		if i+8 > len(table) {
			return fail(fmt.Errorf("a base relocation block cut short"))
		}
		page := binary.LittleEndian.Uint32(table[i:])
		n := int(binary.LittleEndian.Uint32(table[i+4:]))
		if n < 8 || n%2 != 0 || i+n > len(table) {
			return fail(fmt.Errorf("a base relocation block of %d bytes", n))
		}
		for j := i + 8; j < i+n; j += 2 {
			e := binary.LittleEndian.Uint16(table[j:])
			a := base + uint64(page) + uint64(e&0xfff)
			var err error
			switch e >> 12 {
			case peRelAbsolute: // padding, the loader does nothing
			case peRelDir64:
				err = b.addFixup(a, fixup{width: 8, slide: true, what: "a DIR64 base relocation"})
			case peRelHighLow:
				err = b.addFixup(a, fixup{width: 4, what: "a HIGHLOW base relocation"})
			default:
				err = fmt.Errorf("a base relocation of type %d, which the check does not read", e>>12)
			}
			if err != nil {
				return fail(err)
			}
		}
		i += n
	}
	return b, nil
}

// peRVA gives the n bytes the file holds at rva, inside one section.
func peRVA(f *pe.File, rva, n uint64) ([]byte, error) {
	for _, s := range f.Sections {
		lo, raw := uint64(s.VirtualAddress), uint64(min(int(s.VirtualSize), int(s.Size)))
		if rva < lo || rva-lo >= raw || n > raw-(rva-lo) {
			continue
		}
		d, err := s.Data()
		if err != nil {
			return nil, err
		}
		if uint64(len(d)) < raw {
			return nil, fmt.Errorf("section %s is cut short", s.Name)
		}
		return d[rva-lo : rva-lo+n], nil
	}
	return nil, fmt.Errorf("%d bytes at %#x are in no section of the file", n, rva)
}

// PE data directories that make the loader write into the image or run
// code in it, which the check does not read (winnt.h IMAGE_DIRECTORY_ENTRY_*).
var peRefused = map[int]string{
	pe.IMAGE_DIRECTORY_ENTRY_ARCHITECTURE:   "architecture",
	pe.IMAGE_DIRECTORY_ENTRY_GLOBALPTR:      "global pointer",
	pe.IMAGE_DIRECTORY_ENTRY_TLS:            "TLS",
	pe.IMAGE_DIRECTORY_ENTRY_BOUND_IMPORT:   "bound import",
	pe.IMAGE_DIRECTORY_ENTRY_DELAY_IMPORT:   "delay import",
	pe.IMAGE_DIRECTORY_ENTRY_COM_DESCRIPTOR: "CLR",
}

// peLoadConfigSlots are the fields of IMAGE_LOAD_CONFIG_DIRECTORY64 that
// hold the address of a slot the loader writes when it starts the program:
// the security cookie and the pointers of the control flow guard.
var peLoadConfigSlots = map[int]string{
	88:  "SecurityCookie",
	112: "GuardCFCheckFunctionPointer",
	120: "GuardCFDispatchFunctionPointer",
	216: "GuardRFFailureRoutineFunctionPointer",
	232: "GuardRFVerifyStackPointerFunctionPointer",
	280: "GuardXFGCheckFunctionPointer",
	288: "GuardXFGDispatchFunctionPointer",
	296: "GuardXFGTableDispatchFunctionPointer",
	304: "CastGuardOsDeterminedFailureMode",
	312: "GuardMemcpyFunctionPointer",
}

// peLoadConfigRefused are the fields of the load config that make the
// loader change code or data in ways the check does not read: the LOCK
// prefixes it may remove, and the dynamic value relocations, the hybrid
// metadata and the hot patches.
var peLoadConfigRefused = map[int]string{
	40:  "LockPrefixTable",
	192: "DynamicValueRelocTable",
	200: "CHPEMetadataPointer",
	224: "DynamicValueRelocTableOffset",
	240: "HotPatchTableOffset",
}

// peLoaderWrites records the places the PE loader writes besides the base
// relocations: the import address table of each import descriptor (the
// slots its FirstThunk names, one for each entry of its lookup table), and
// the slots the load config names. A directory that makes the loader write
// or run more than that is refused. A Go 1.27.2 binary has an import
// directory, a load config with no slots, an exception directory and the
// base relocations, and nothing else that writes.
func (b *binFile) peLoaderWrites(f *pe.File, oh *pe.OptionalHeader64) error {
	// The loader finds a directory by its address and takes the size of
	// some from what is there (the imports end at a descriptor of zeros, the
	// load config gives its own size), so a directory with an address is
	// read whatever its size, and one with a size and no address refused.
	for i, d := range oh.DataDirectory {
		if d.VirtualAddress == 0 && d.Size != 0 {
			return fmt.Errorf("data directory %d has a size and no address", i)
		}
	}
	for i, name := range peRefused {
		if i < len(oh.DataDirectory) && (oh.DataDirectory[i].Size != 0 || oh.DataDirectory[i].VirtualAddress != 0) {
			return fmt.Errorf("a %s directory, which the check does not read", name)
		}
	}
	base := oh.ImageBase
	u32 := func(rva uint64) (uint64, error) {
		d, err := peRVA(f, rva, 4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint32(d)), nil
	}
	u64 := func(rva uint64) (uint64, error) {
		d, err := peRVA(f, rva, 8)
		if err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint64(d), nil
	}
	if dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]; dir.VirtualAddress != 0 {
		// The loader reads descriptors up to one of zeros.
		for i := uint64(0); ; i++ {
			if i >= 1<<12 {
				return fmt.Errorf("more than %d import descriptors", i)
			}
			d, err := peRVA(f, uint64(dir.VirtualAddress)+20*i, 20)
			if err != nil {
				return fmt.Errorf("import descriptor %d: %w", i, err)
			}
			lookup, first := uint64(binary.LittleEndian.Uint32(d)), uint64(binary.LittleEndian.Uint32(d[16:]))
			if string(d) == string(make([]byte, 20)) {
				b.meta = append(b.meta, metaRange{base + uint64(dir.VirtualAddress), 20 * (i + 1), "the import descriptors"})
				break
			}
			if first == 0 {
				return fmt.Errorf("import descriptor %d has no import address table", i)
			}
			if lookup == 0 {
				lookup = first
			}
			for e := uint64(0); ; e++ {
				if e >= 1<<16 {
					return fmt.Errorf("import descriptor %d: more than %d imports", i, e)
				}
				v, err := u64(lookup + 8*e)
				if err != nil {
					return fmt.Errorf("import descriptor %d: %w", i, err)
				}
				if v == 0 {
					// The lookup table, up to its zero entry. When it is
					// the import address table, that is the entry after
					// the imports.
					if lookup != first {
						b.meta = append(b.meta, metaRange{base + lookup, 8 * (e + 1), fmt.Sprintf("the lookup table of import descriptor %d", i)})
					} else {
						b.meta = append(b.meta, metaRange{base + first + 8*e, 8, fmt.Sprintf("the end of the import address table of descriptor %d", i)})
					}
					break
				}
				if err := b.addFixup(base+first+8*e, fixup{width: 8, what: fmt.Sprintf("an import of descriptor %d", i)}); err != nil {
					return err
				}
			}
		}
	}
	if dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_LOAD_CONFIG]; dir.VirtualAddress != 0 {
		// Its own size, or the size of the directory when that is more:
		// the fields either names.
		n, err := u32(uint64(dir.VirtualAddress))
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		n = max64(n, uint64(dir.Size))
		b.meta = append(b.meta, metaRange{base + uint64(dir.VirtualAddress), n, "the load config"})
		field := func(off int, width uint64) (uint64, bool, error) {
			if uint64(off)+width > n {
				return 0, false, nil
			}
			if width == 4 {
				v, err := u32(uint64(dir.VirtualAddress) + uint64(off))
				return v, true, err
			}
			v, err := u64(uint64(dir.VirtualAddress) + uint64(off))
			return v, true, err
		}
		for off, name := range peLoadConfigRefused {
			width := uint64(8)
			if off == 224 || off == 240 {
				width = 4
			}
			v, _, err := field(off, width)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if v != 0 {
				return fmt.Errorf("the load config has a %s, which the check does not read", name)
			}
		}
		for off, name := range peLoadConfigSlots {
			v, _, err := field(off, 8)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if v != 0 {
				if err := b.addFixup(v, fixup{width: 8, what: "the slot of " + name + " in the load config"}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type namedSym struct {
	name    string
	addr    uint64
	sect    int
	sectEnd uint64
}

// sizeToNext gives each symbol of a format without sizes the bytes up to the
// next higher symbol of its section, or to the end of the section. That is
// at least the symbol, with the padding after it: the check holds what it
// finds there to padding, and a read past it fails.
func sizeToNext(named []namedSym) map[string]binSym {
	sort.Slice(named, func(i, j int) bool {
		if named[i].sect != named[j].sect {
			return named[i].sect < named[j].sect
		}
		return named[i].addr < named[j].addr
	})
	out := map[string]binSym{}
	for i, s := range named {
		end := s.sectEnd
		for j := i + 1; j < len(named) && named[j].sect == s.sect; j++ {
			if named[j].addr > s.addr {
				end = named[j].addr
				break
			}
		}
		if end < s.addr {
			end = s.addr
		}
		out[s.name] = binSym{s.addr, end - s.addr}
	}
	return out
}

func (b *binFile) sec(addr uint64) *binSec {
	for _, s := range b.secs {
		if addr >= s.addr && addr-s.addr < s.size {
			return s
		}
	}
	return nil
}

// upTo reads at most n bytes at addr, fewer at the end of a section.
func (b *binFile) upTo(addr uint64, n int) ([]byte, error) {
	s := b.sec(addr)
	if s == nil {
		return nil, fmt.Errorf("address %#x not in the binary", addr)
	}
	if rest := s.size - (addr - s.addr); n > 0 && uint64(n) > rest {
		n = int(rest)
	}
	return b.read(addr, n)
}

// read gives the n bytes the file holds at addr, and notes each fixup among
// them (loaderWrites). The bounds are held as lengths inside the section, so
// no address wraps around.
func (b *binFile) read(addr uint64, n int) ([]byte, error) {
	s := b.sec(addr)
	if s == nil || n < 0 || uint64(n) > s.size-(addr-s.addr) {
		return nil, fmt.Errorf("address %#x+%d not in the binary", addr, n)
	}
	if s.data == nil {
		d, err := s.load()
		if err != nil {
			return nil, err
		}
		if uint64(len(d)) < s.size {
			return nil, fmt.Errorf("section %s holds %d bytes in the file, %d in memory", s.name, len(d), s.size)
		}
		s.data = d
	}
	// A fixup starts at most 7 bytes before addr and overlaps it.
	lo := addr - min64(addr, 7)
	i := sort.Search(len(b.fixAt), func(i int) bool { return b.fixAt[i] >= lo })
	for ; i < len(b.fixAt); i++ {
		a := b.fixAt[i]
		if a >= addr && a-addr >= uint64(n) {
			break
		}
		if a >= addr || addr-a < uint64(b.fixups[a].width) {
			if b.touched == nil {
				b.touched = map[uint64]bool{}
			}
			b.touched[a] = true
		}
	}
	off := addr - s.addr
	return s.data[off : off+uint64(n)], nil
}

// pointer checks the 8-byte slot at addr, which the plan holds a pointer to
// want in: the slot gives want (the address at the image base the file
// names: the bytes, or the target of a chained entry), and in a program the
// loader may move, the loader slides exactly that slot.
func (b *binFile) pointer(addr, want uint64) error {
	d, err := b.read(addr, 8)
	if err != nil {
		return err
	}
	got := binary.LittleEndian.Uint64(d)
	if f, ok := b.fixups[addr]; ok && f.chained {
		if !f.slide {
			return fmt.Errorf("the pointer at %#x has %s: the loader puts an import there", addr, f.what)
		}
		got = f.target
	}
	if got != want {
		return fmt.Errorf("the pointer is %#x in the binary, should be %#x", got, want)
	}
	if b.pie {
		if f, ok := b.fixups[addr]; !ok || !f.slide {
			what := "no fixup"
			if ok {
				what = f.what
			}
			return fmt.Errorf("the pointer at %#x has %s: the loader would not slide it when it places the program elsewhere", addr, what)
		}
	}
	if b.pointers == nil {
		b.pointers = map[uint64]bool{}
	}
	b.pointers[addr] = true
	return nil
}

// loaderWrites fails when the loader writes into any byte the check read,
// except the pointers it checked (pointer): there the program would run
// other bytes or data than the check saw. It gives the number of pointers
// the loader slides.
func (b *binFile) loaderWrites() (int, error) {
	var bad []uint64
	for a := range b.touched {
		if !b.pointers[a] || !b.fixups[a].slide {
			bad = append(bad, a)
		}
	}
	if len(bad) > 0 {
		sort.Slice(bad, func(i, j int) bool { return bad[i] < bad[j] })
		return 0, fmt.Errorf("the loader writes %d places of the code or data the check read, which are no pointers it checked: %s at %#x first", len(bad), b.fixups[bad[0]].what, bad[0])
	}
	n := 0
	for a := range b.pointers {
		if b.fixups[a].slide {
			n++
		}
	}
	return n, nil
}

// abi0 gives the ABI0 entry of a Go-visible function: the assembly body
// (pkg.f, or pkg.f.abi0 when Go also has an ABIInternal wrapper) or the ABI0
// wrapper of a Go function. Every CALL, LEAQ and DATA of the generated
// assembly names this symbol.
func (b *binFile) abi0(pkg, goName string) (binSym, bool) {
	if s, ok := b.syms[pkg+"."+goName+".abi0"]; ok {
		return s, true
	}
	s, ok := b.syms[pkg+"."+goName]
	return s, ok
}

// dataSym gives the data symbol name with its size: the symbol's own in ELF;
// in a format without sizes, size, which must fit before the next symbol.
func (b *binFile) dataSym(name string, size uint64) (binSym, bool, error) {
	s, ok := b.syms[name]
	if !ok || b.format == "ELF" {
		return s, ok, nil
	}
	if s.Size < size {
		return s, ok, fmt.Errorf("%s has %d bytes before the next symbol, %d emitted", name, s.Size, size)
	}
	s.Size = size
	return s, ok, nil
}

// describe says what the check read the binary as.
func (b *binFile) describe() string {
	if !b.pie {
		return fmt.Sprintf("%s for %s, loaded at the addresses of the file, %d fixups", b.format, b.goos, len(b.fixups))
	}
	slides := 0
	for _, f := range b.fixups {
		if f.slide {
			slides++
		}
	}
	return fmt.Sprintf("%s for %s, position independent, %d fixups of which %d slide a pointer", b.format, b.goos, len(b.fixups), slides)
}

func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
