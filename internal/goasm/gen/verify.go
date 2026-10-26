// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Post-link verification. After the Go linker has placed the generated code in
// a program, confirm it is still the code the generator produced: every body
// byte survived except the four displacement bytes of each symbolic CALL, each
// split-checked unified entry kept its stack-check prologue, the function table
// points only at unified entries, and the body holds no hidden call or
// cross-function tail jump.
package gen

import (
	"debug/elf"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

type VerifyReport struct {
	Func         string
	BodyBytes    int
	Calls        int
	HasMorestack bool
}

// VerifyLinked checks the functions of metas inside binPath against objPath.
func VerifyLinked(objPath, binPath, pkg string, metas []Meta) ([]VerifyReport, error) {
	obj, err := readObject(objPath)
	if err != nil {
		return nil, err
	}
	bin, err := elf.Open(binPath)
	if err != nil {
		return nil, err
	}
	defer bin.Close()
	text := bin.Section(".text")
	code, err := text.Data()
	if err != nil {
		return nil, err
	}
	syms, err := bin.Symbols()
	if err != nil {
		return nil, err
	}
	addr := map[string][2]uint64{}
	morestack := map[uint64]bool{}
	for _, s := range syms {
		addr[s.Name] = [2]uint64{s.Value, s.Size}
		if strings.Contains(s.Name, "morestack") {
			morestack[s.Value] = true
		}
	}
	symName := func(m Meta) string {
		if m.Kind == Register {
			return pkg + "." + m.GoName
		}
		return pkg + "." + m.GoName + ".abi0"
	}

	var reports []VerifyReport
	unifiedAddrs := map[uint64]bool{}
	for _, m := range metas {
		sn := symName(m)
		vs, ok := addr[sn]
		if !ok {
			return nil, fmt.Errorf("symbol %s absent from binary", sn)
		}
		lo := vs[0] - text.Addr
		linked := code[lo : lo+vs[1]]
		ovs := obj.funcs[m.Name]
		orig := append([]byte(nil), obj.code[ovs[0]:ovs[0]+ovs[1]]...)
		// Find the object body inside the linked symbol (skip alignment NOPs).
		start := indexSub(linked, orig[:min(16, len(orig))])
		if start < 0 {
			return nil, fmt.Errorf("%s: generated body not found in linked symbol", m.Name)
		}
		actual := append([]byte(nil), linked[start:start+len(orig)]...)
		// Every direct call's four displacement bytes are filled by the
		// linker, so diffBody skips them; every other byte must be unchanged.
		var callOffs []int
		for off := range obj.calls {
			if int(off) >= int(ovs[0]) && int(off) < int(ovs[0]+ovs[1]) {
				callOffs = append(callOffs, int(off-ovs[0]))
			}
		}
		if err := diffBody(orig, actual, callOffs); err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		// No hidden calls or cross-function tail jumps: every CALL in the
		// linked body is one of our known sites, and no JMP leaves the body.
		codeLen := len(orig)
		if cs, ok := obj.funcs[m.Name+".const"]; ok {
			codeLen = int(cs[0] - ovs[0])
		}
		nCalls, err := scanCalls(linked[start:start+codeLen], callOffs, len(orig))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Name, err)
		}
		// Split-checked unified entries must keep the morestack prologue.
		hasMore := false
		if m.Kind == Unified && (m.Frame > 0 || len(callOffs) > 0) {
			hasMore = callsTarget(linked, vs[0], morestack)
			if !hasMore {
				return nil, fmt.Errorf("%s: missing morestack prologue", m.Name)
			}
		}
		if m.Kind == Unified {
			unifiedAddrs[vs[0]] = true
		}
		reports = append(reports, VerifyReport{m.Name, len(orig), nCalls, hasMore})
	}

	// The interrupt poll is the only place GC can stop a C loop, so its Go
	// body must keep the split-check prologue (a leaf would be NOSPLIT).
	if pv, ok := addr[pkg+".host_poll"]; ok {
		lo := pv[0] - text.Addr
		if !callsTarget(code[lo:lo+pv[1]], pv[0], morestack) {
			return nil, fmt.Errorf("host_poll has no stack-check prologue: GC cannot stop C loops")
		}
	} else {
		return nil, fmt.Errorf("host_poll absent from binary")
	}

	// Function table: every entry points at a unified entry.
	if tv, ok := addr[pkg+".unifiedTable"]; ok {
		ro := sectionContaining(bin, tv[0])
		if ro == nil {
			return nil, fmt.Errorf("unifiedTable not in a section")
		}
		data, _ := ro.Data()
		base := tv[0] - ro.Addr
		for off := uint64(0); off < tv[1]; off += 8 {
			entry := le64(data[base+off:])
			if !unifiedAddrs[entry] {
				return nil, fmt.Errorf("unifiedTable[%d]=%#x is not a unified entry", off/8, entry)
			}
		}
	}
	return reports, nil
}

// scanCalls disassembles a body and checks every CALL is a known direct site
// or a register-indirect call, and no JMP targets outside the body.
func scanCalls(body []byte, known []int, n int) (int, error) {
	sort.Ints(known)
	kset := map[int]bool{}
	for _, k := range known {
		kset[k] = true
	}
	count := 0
	for i := 0; i < len(body); {
		inst, err := x86asm.Decode(body[i:], 64)
		if err != nil {
			return 0, fmt.Errorf("decode at %d: %w", i, err)
		}
		switch inst.Op {
		case x86asm.CALL:
			count++
			if body[i] == 0xe8 && !kset[i] {
				return 0, fmt.Errorf("hidden direct call at %d", i)
			}
		case x86asm.JMP:
			if rel, ok := inst.Args[0].(x86asm.Rel); ok {
				dst := i + inst.Len + int(rel)
				if dst < 0 || dst > n {
					return 0, fmt.Errorf("cross-function tail jump at %d to %d", i, dst)
				}
			}
		}
		i += inst.Len
	}
	return count, nil
}

func callsTarget(body []byte, base uint64, targets map[uint64]bool) bool {
	for i := 0; i+5 <= len(body); i++ {
		if body[i] == 0xe8 {
			rel := int32(uint32(body[i+1]) | uint32(body[i+2])<<8 | uint32(body[i+3])<<16 | uint32(body[i+4])<<24)
			if targets[base+uint64(i)+5+uint64(int64(rel))] {
				return true
			}
		}
	}
	return false
}

func sectionContaining(f *elf.File, a uint64) *elf.Section {
	for _, s := range f.Sections {
		if s.Addr <= a && a < s.Addr+s.Size && s.Type == elf.SHT_PROGBITS {
			return s
		}
	}
	return nil
}

func indexSub(h, n []byte) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if string(h[i:i+len(n)]) == string(n) {
			return i
		}
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
