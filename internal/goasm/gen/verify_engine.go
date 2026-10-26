// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Structural net for the whole engine's read-only data. The generator's own
// parser (encodeDirective, the alignment and string decoding) turns llc's
// data directives into the bytes of engine_rodata. A mistake there is a silent
// miscompile: the bytes ship wrong and nothing downstream notices. So re-run
// the parser over every data object the blob holds, at the offset llc placed
// it, and compare byte for byte against llc's own assembled object. Only the
// symbol-referencing slots are left out: a pointer is a relocation on both
// sides, and a difference of two local symbols is a blob-specific value the
// relocations already carry. Any other difference fails the build.
package gen

import (
	"debug/elf"
	"fmt"
)

// diffData compares the generator's bytes for one data object against llc's
// assembled bytes. A byte both sides mark as a symbol slot (a pointer
// relocation) agrees by construction. A byte the generator marks a slot but
// llc does not is a resolved local difference, a blob-specific value, so it is
// not compared. A byte llc relocates but the generator wrote as a literal
// means the parser turned a pointer into a constant. Every other byte must be
// equal: that catches wrong alignment padding, a mis-decoded string or a
// wrong literal.
func diffData(name string, llc []byte, llcReloc []bool, my []byte, myFix []bool) error {
	if len(llc) != len(my) {
		return fmt.Errorf("data %s: %d bytes in llc's object, %d generated", name, len(llc), len(my))
	}
	for i := range my {
		switch {
		case myFix[i] && llcReloc[i]:
			// both a symbol slot (a relocation): agree
		case myFix[i] && !llcReloc[i]:
			// a resolved local difference: blob-specific, not comparable
		case !myFix[i] && llcReloc[i]:
			return fmt.Errorf("data %s: generator wrote a literal at byte %d where llc keeps a relocation", name, i)
		default:
			if my[i] != llc[i] {
				return fmt.Errorf("data %s: byte %d is %#02x generated, %#02x in llc's object", name, i, my[i], llc[i])
			}
		}
	}
	return nil
}

// diffBody checks a linked function body equals the generator's re-assembled
// body, except in the four displacement bytes of each direct call (at the
// given body offsets), which the linker fills in.
func diffBody(orig, actual []byte, callOffs []int) error {
	if len(orig) != len(actual) {
		return fmt.Errorf("body length %d generated, %d linked", len(orig), len(actual))
	}
	o := append([]byte(nil), orig...)
	a := append([]byte(nil), actual...)
	for _, c := range callOffs {
		for k := 0; k < 4 && c+1+k < len(o); k++ {
			o[c+1+k] = 0
			a[c+1+k] = 0
		}
	}
	if string(o) != string(a) {
		return fmt.Errorf("linked body differs from generated")
	}
	return nil
}

// VerifyEngineFuncs is the function half of the whole-engine post-link check.
// For every lowered function it finds the generator's re-assembled body in
// objPath, locates it in the linked binary binPath, and confirms the linker
// left every byte unchanged except the four displacement bytes of each direct
// call. It also confirms the linked body holds no hidden direct call and no
// cross-function tail jump. It returns the number of functions checked. It
// does not reach for the host-callback or function-table checks VerifyLinked
// makes for the prototype host.
func VerifyEngineFuncs(objPath, binPath, pkg string, metas []Meta) (int, error) {
	obj, err := readObject(objPath)
	if err != nil {
		return 0, err
	}
	bin, err := elf.Open(binPath)
	if err != nil {
		return 0, err
	}
	defer bin.Close()
	text := bin.Section(".text")
	if text == nil {
		return 0, fmt.Errorf("binary has no .text")
	}
	code, err := text.Data()
	if err != nil {
		return 0, err
	}
	syms, err := bin.Symbols()
	if err != nil {
		return 0, err
	}
	addr := map[string][2]uint64{}
	for _, s := range syms {
		addr[s.Name] = [2]uint64{s.Value, s.Size}
	}
	checked := 0
	for _, m := range metas {
		// An ABI0 asm TEXT is linked as pkg.Name; a register-entry leaf keeps
		// that name, a unified entry also gets an ABIInternal wrapper, so the
		// asm body is under pkg.Name or pkg.Name.abi0.
		var vs [2]uint64
		var ok bool
		for _, sn := range []string{pkg + "." + m.GoName, pkg + "." + m.GoName + ".abi0"} {
			if vs, ok = addr[sn]; ok {
				break
			}
		}
		if !ok {
			continue // not a separately linked symbol (inlined wrapper, folded)
		}
		lo := vs[0] - text.Addr
		linked := code[lo : lo+vs[1]]
		ovs, ok := obj.funcs[m.Name]
		if !ok {
			return checked, fmt.Errorf("%s: absent from the generator's object", m.Name)
		}
		orig := append([]byte(nil), obj.code[ovs[0]:ovs[0]+ovs[1]]...)
		// Byte positions the linker fills: the 4-byte displacement of every
		// direct call and of every RIP-relative reference to data or a
		// function (a LEAQ the generator re-emits symbolically).
		reloc := make([]bool, len(orig))
		var callOffs []int
		for off := range obj.calls {
			if int(off) >= int(ovs[0]) && int(off) < int(ovs[0]+ovs[1]) {
				c := int(off - ovs[0])
				callOffs = append(callOffs, c)
				for k := 1; k < 5 && c+k < len(reloc); k++ {
					reloc[c+k] = true
				}
			}
		}
		for roff := range obj.refs {
			if int(roff) >= int(ovs[0]) && int(roff) < int(ovs[0]+ovs[1]) {
				r := int(roff - ovs[0])
				for k := 0; k < 4 && r+k < len(reloc); k++ {
					reloc[r+k] = true
				}
			}
		}
		// Anchor the body in the linked symbol on the first 16-byte window
		// that holds no linker-filled byte, so a relocation near the start
		// does not defeat the search.
		anchor := anchorFree(reloc, 16)
		if anchor < 0 {
			continue // too relocated to anchor; rare, skip
		}
		pos := indexSub(linked, orig[anchor:anchor+16])
		if pos < 0 {
			return checked, fmt.Errorf("%s: generated body not found in linked symbol", m.Name)
		}
		start := pos - anchor
		if start < 0 || start+len(orig) > len(linked) {
			return checked, fmt.Errorf("%s: generated body does not fit the linked symbol", m.Name)
		}
		actual := append([]byte(nil), linked[start:start+len(orig)]...)
		for i, r := range reloc {
			if r {
				orig[i] = 0
				actual[i] = 0
			}
		}
		if err := diffBody(orig, actual, callOffs); err != nil {
			return checked, fmt.Errorf("%s: %w", m.Name, err)
		}
		codeLen := len(orig)
		if cs, ok := obj.funcs[m.Name+".const"]; ok {
			codeLen = int(cs[0] - ovs[0])
		}
		if _, err := scanCalls(linked[start:start+codeLen], callOffs, len(orig)); err != nil {
			return checked, fmt.Errorf("%s: %w", m.Name, err)
		}
		checked++
	}
	if checked == 0 {
		return 0, fmt.Errorf("no generated function found in %s (wrong package %q?)", binPath, pkg)
	}
	return checked, nil
}

// anchorFree returns the first offset where mask holds n consecutive false
// bytes (a window free of linker-filled bytes), or -1 if there is none.
func anchorFree(mask []bool, n int) int {
	if len(mask) < n {
		return -1
	}
	run := 0
	for i, r := range mask {
		if r {
			run = 0
			continue
		}
		run++
		if run == n {
			return i - n + 1
		}
	}
	return -1
}

// VerifyEngineData re-encodes every blob data object with the generator's own
// parser at the offset llc placed it in objPath, and checks it byte for byte
// against llc's assembled object. It returns the number of objects checked.
func VerifyEngineData(objPath string, u *AsmUnit, p *DataPlan) (int, error) {
	f, err := elf.Open(objPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	type section struct {
		data []byte
		mask []bool // byte covered by a relocation
	}
	secs := map[int]*section{}
	for i, s := range f.Sections {
		if s.Type != elf.SHT_PROGBITS || s.Flags&elf.SHF_ALLOC == 0 || s.Flags&elf.SHF_EXECINSTR != 0 {
			continue
		}
		d, err := s.Data()
		if err != nil {
			return 0, err
		}
		secs[i] = &section{data: d, mask: make([]bool, len(d))}
	}
	for _, rs := range f.Sections {
		if rs.Type != elf.SHT_RELA {
			continue
		}
		sc := secs[int(rs.Info)]
		if sc == nil {
			continue
		}
		data, err := rs.Data()
		if err != nil {
			return 0, err
		}
		for off := 0; off+24 <= len(data); off += 24 {
			roff := int(le64(data[off:]))
			n := 0
			switch elf.R_X86_64(le64(data[off+8:]) & 0xffffffff) {
			case elf.R_X86_64_64:
				n = 8
			case elf.R_X86_64_PC32, elf.R_X86_64_PLT32, elf.R_X86_64_32, elf.R_X86_64_32S:
				n = 4
			}
			for k := 0; k < n && roff+k < len(sc.mask); k++ {
				sc.mask[roff+k] = true
			}
		}
	}
	syms, err := f.Symbols()
	if err != nil {
		return 0, err
	}
	type loc struct {
		si int
		v  int
	}
	place := map[string]loc{}
	for _, s := range syms {
		if s.Name != "" && int(s.Section) < len(f.Sections) {
			place[s.Name] = loc{int(s.Section), int(s.Value)}
		}
	}
	checked := 0
	for _, o := range u.DataOrder {
		if _, ok := p.BlobOff[o.Label]; !ok {
			continue // only the objects the generator's parser produced
		}
		l, ok := place[o.Label]
		if !ok {
			continue // no symbol to locate it by
		}
		sc := secs[l.si]
		if sc == nil {
			continue
		}
		var my []byte
		var myFix []bool
		for _, line := range o.Lines {
			b, fx, err := encodeDirective(line, l.v+len(my))
			if err != nil {
				return checked, fmt.Errorf("data %s: re-encode %q: %w", o.Label, line, err)
			}
			my = append(my, b...)
			myFix = append(myFix, make([]bool, len(b))...)
			for _, x := range fx {
				for k := 0; k < x.size; k++ {
					if j := x.off - l.v + k; j >= 0 && j < len(myFix) {
						myFix[j] = true
					}
				}
			}
		}
		if l.v+len(my) > len(sc.data) {
			return checked, fmt.Errorf("data %s: extends %d bytes past its section", o.Label, l.v+len(my)-len(sc.data))
		}
		if err := diffData(o.Label, sc.data[l.v:l.v+len(my)], sc.mask[l.v:l.v+len(my)], my, myFix); err != nil {
			return checked, err
		}
		checked++
	}
	return checked, nil
}
