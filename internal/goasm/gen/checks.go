// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Static checks. Each rule the plan lists is enforced mechanically and has a
// small example it must reject (checks_test.go). The lowering enforces the
// instruction-level rules (forbidden instructions, SP writes, escaping stack
// addresses, frame spills, indirect jumps); these functions enforce the rules
// that read the LLVM IR and the linked/assembled object.
package gen

import (
	"debug/elf"
	"fmt"
	"regexp"
	"strings"
)

var (
	reVararg  = regexp.MustCompile(`\bva_(start|arg|copy|end)\b|\bllvm\.va`)
	reByval   = regexp.MustCompile(`\bbyval\(`)
	reSret    = regexp.MustCompile(`\bsret\(`)
	reDefine  = regexp.MustCompile(`^define[^@]*@[^(]+\([^)]*\.\.\.`)
	reLongDbl = regexp.MustCompile(`\bx86_fp80\b|\bfp128\b`)
)

// CheckIR rejects IR shapes the ABI cannot carry across a movable Go stack:
// variadic functions, byval or sret aggregates, and long double.
func CheckIR(ll string) error {
	for _, line := range strings.Split(ll, "\n") {
		if reVararg.MatchString(line) {
			return fmt.Errorf("variadic construct: %q", strings.TrimSpace(line))
		}
		if reByval.MatchString(line) {
			return fmt.Errorf("byval aggregate (implicit stack copy): %q", strings.TrimSpace(line))
		}
		if reSret.MatchString(line) {
			return fmt.Errorf("sret aggregate (hidden result pointer): %q", strings.TrimSpace(line))
		}
		if reDefine.MatchString(line) {
			return fmt.Errorf("variadic definition: %q", strings.TrimSpace(line))
		}
		if reLongDbl.MatchString(line) {
			return fmt.Errorf("long double is not bit-identical across arches: %q", strings.TrimSpace(line))
		}
	}
	return nil
}

// CheckObject rejects a writable data section with content and any external
// relocation to a symbol outside the closed allow list (the engine's own
// functions plus the named host callbacks).
func CheckObject(objPath string, allow map[string]bool) error {
	f, err := elf.Open(objPath)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, s := range f.Sections {
		if s.Flags&elf.SHF_WRITE != 0 && s.Flags&elf.SHF_ALLOC != 0 && s.Size > 0 {
			if s.Type == elf.SHT_NOBITS || s.Type == elf.SHT_PROGBITS {
				return fmt.Errorf("writable data section %q of %d bytes (engine must have no mutable globals)", s.Name, s.Size)
			}
		}
	}
	syms, err := f.Symbols()
	if err != nil {
		return err
	}
	text := f.Section(".text")
	for _, rs := range f.Sections {
		if rs.Type != elf.SHT_RELA || int(rs.Info) >= len(f.Sections) || f.Sections[rs.Info] != text {
			continue
		}
		data, _ := rs.Data()
		for off := 0; off+24 <= len(data); off += 24 {
			sidx := le64(data[off+8:]) >> 32
			if sidx == 0 || int(sidx) > len(syms) {
				continue
			}
			name := syms[sidx-1].Name
			if name != "" && !allow[name] {
				return fmt.Errorf("external reference to %q outside the allow list", name)
			}
		}
	}
	return nil
}
