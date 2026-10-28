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
	reVararg = regexp.MustCompile(`\bva_(start|arg|copy|end)\b|\bllvm\.va`)
	reByval  = regexp.MustCompile(`\bbyval\(`)
	reSret   = regexp.MustCompile(`\bsret\(`)
	// Parameter attributes such as captures(none) hold parentheses, so the
	// variadic marker is matched as the last parameter, not inside the first
	// pair of parentheses.
	reDefine  = regexp.MustCompile(`^(define|declare)\b.*[(,]\s*\.\.\.\)`)
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
		// C const tables that hold pointers: PIC code puts them in
		// .data.rel.ro, writable only while relocations are applied. They
		// become read-only Go data.
		if strings.HasPrefix(s.Name, ".data.rel.ro") {
			continue
		}
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
			sym := syms[sidx-1]
			name := sym.Name
			// A reference into the unit's own read-only data (constant
			// pools) is allowed; everything else must be on the list.
			if int(sym.Section) > 0 && int(sym.Section) < len(f.Sections) {
				sec := f.Sections[sym.Section]
				if sec.Flags&elf.SHF_ALLOC != 0 && sec.Flags&(elf.SHF_WRITE|elf.SHF_EXECINSTR) == 0 {
					continue
				}
			}
			if name != "" && !allow[name] {
				return fmt.Errorf("external reference to %q outside the allow list", name)
			}
		}
	}
	return nil
}

// IRViolation is one rule a function of the C breaks, as the IR shows it.
type IRViolation struct {
	Func, Rule, Line string
}

var reDefName = regexp.MustCompile(`^define\b[^@]*@("?[^"(]+"?)\(`)

// CheckIRFuncs is CheckIR over a whole module that names the function of
// each violation instead of stopping at the first. A call of a variadic
// function counts as a violation of the caller too ("variadic call").
func CheckIRFuncs(ll string) []IRViolation {
	var out []IRViolation
	fn := ""
	for _, line := range strings.Split(ll, "\n") {
		if m := reDefName.FindStringSubmatch(line); m != nil {
			fn = strings.Trim(m[1], `"`)
		}
		if strings.HasPrefix(line, "}") {
			fn = ""
		}
		add := func(rule string) { out = append(out, IRViolation{fn, rule, strings.TrimSpace(line)}) }
		switch {
		case reDefine.MatchString(line) && strings.HasPrefix(line, "define"):
			add("variadic definition")
		case fn != "" && strings.Contains(line, "llvm.va_start"):
			add("va_start")
		case fn != "" && reVarCall.MatchString(line):
			add("variadic call")
		}
		if reByval.MatchString(line) {
			add("byval aggregate")
		}
		if reSret.MatchString(line) {
			add("sret aggregate")
		}
		if reLongDbl.MatchString(line) {
			add("long double")
		}
		if strings.Contains(line, "thread_local") {
			add("thread-local storage")
		}
		if reFMA.MatchString(line) {
			add("fused multiply-add")
		}
		if reFastMath.MatchString(line) || reFastMathAttr.MatchString(line) {
			add("fast-math")
		}
	}
	return out
}

// A multiply and an add rounded once (llvm.fmuladd, llvm.fma) gives other
// results than the two rounded each, and only some targets fuse them: the
// engine is built with -ffp-contract=off so that every target gives the same
// numbers. Fast-math flags on a floating-point operation (or a function
// attribute that implies them) let LLVM reorder or approximate it, which the
// engine never allows.
var (
	reFMA          = regexp.MustCompile(`@llvm\.(fmuladd|fma)\.`)
	reFastMath     = regexp.MustCompile(`= (?:fadd|fsub|fmul|fdiv|frem|fneg|fcmp|call|select|phi)\b(?: [a-z]+)*? (?:fast|reassoc|nnan|ninf|nsz|arcp|contract|afn)\b`)
	reFastMathAttr = regexp.MustCompile(`"(?:unsafe-fp-math|no-nans-fp-math|no-infs-fp-math|no-signed-zeros-fp-math|approx-func-fp-math)"="true"`)
)

// A call of a variadic function: its type in the call reads (..., ...).
var reVarCall = regexp.MustCompile(`\bcall\b[^(]*\([^()]*\.\.\.\)\s*@`)
