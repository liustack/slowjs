// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// arm64 backend. Lowering edits llc's aarch64 assembly as text, llvm-mc
// re-assembles it, and emit writes Go assembly: the body as WORDs, every call
// a symbolic BL, the arguments loaded from FP after Go's prologue, one Go RET
// and the function table.
//
// Frame: 0(RSP) is Go's saved LR, the outgoing ABI0 area starts at 8(RSP),
// the LLVM frame of F bytes sits at the next 16-byte boundary L, call-site
// save slots follow it, and the TEXT declares $(L+F+saves-8). Every SP move
// of LLVM's prologue and epilogues is dropped and each SP-relative access is
// rebased on the Go frame, tracking how much of the frame is allocated at
// that point, so any prologue shape and frames over 4096 bytes lower the
// same way.
//
// A C call to a unified entry or a Go callback stores the argument registers
// into the outgoing area, calls the Go symbol and loads the result back, all
// inline at the call site. Go code and morestack clobber R19-R25 and F8-F15,
// which AAPCS code expects preserved. ARM64Mode picks how that is handled.
package gen

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ARM64Mode picks how a unified call keeps the AAPCS callee-saved registers.
type ARM64Mode int

const (
	// ARM64Stub calls a NOSPLIT stub per callee that saves and restores
	// all of R19-R25 and F8-F15 and moves the arguments (the first backend).
	ARM64Stub ARM64Mode = iota
	// ARM64SaveAll moves the arguments at the call site and saves all 15.
	ARM64SaveAll
	// ARM64SaveBody saves at each call site the callee-saved registers the
	// calling function's body uses: the ones LLVM's prologue saved.
	ARM64SaveBody
	// ARM64PreserveNone lowers code compiled with preserve_none on every
	// unified entry and host callback: LLVM keeps across a call only what is
	// live, in its frame, and no call site saves a register.
	ARM64PreserveNone
)

func ParseARM64Mode(s string) (ARM64Mode, error) {
	switch s {
	case "stub":
		return ARM64Stub, nil
	case "all":
		return ARM64SaveAll, nil
	case "body":
		return ARM64SaveBody, nil
	case "pn":
		return ARM64PreserveNone, nil
	}
	return 0, fmt.Errorf("unknown arm64 mode %q (stub, all, body, pn)", s)
}

// ARM64Func is one lowered function.
type ARM64Func struct {
	Meta
	Sig       Sig
	Leaf      bool // no stack use and no call
	LLVMFrame int  // F
	SaveSlots int  // call-site save slots after the LLVM frame
}

// ARM64Stub is a call stub of the stub mode: a direct one to a Go symbol, or
// (Target "") an indirect one through R9.
type ARM64CallStub struct {
	Name, Target string
	Sig          Sig
}

// ARM64Unit is the lowered unit.
type ARM64Unit struct {
	Mode    ARM64Mode
	NoSaves bool // negative build: no call keeps a register
	Funcs   []ARM64Func
	Stubs   []ARM64CallStub
	Lowered string
	// Unified call sites and registers saved around them, over the unit.
	CallSites, SavedRegs int
}

var arm64CSR = []string{"x19", "x20", "x21", "x22", "x23", "x24", "x25",
	"d8", "d9", "d10", "d11", "d12", "d13", "d14", "d15"}

// a64ArgRegs gives the register of each argument word: AAPCS x0-x7 and
// d0-d7, or preserve_none x20-x25 and d0-d7. A JSValue takes two x registers.
func a64ArgRegs(sig Sig, pn bool) ([]string, error) {
	ints := []string{"x0", "x1", "x2", "x3", "x4", "x5", "x6", "x7"}
	if pn {
		ints = []string{"x20", "x21", "x22", "x23", "x24", "x25"}
	}
	var out []string
	ni, nf := 0, 0
	for _, p := range sig.Params {
		switch p {
		case I64, Ptr:
			if ni >= len(ints) {
				return nil, fmt.Errorf("too many integer arguments")
			}
			out = append(out, ints[ni])
			ni++
		case JSV:
			if ni+1 >= len(ints) {
				return nil, fmt.Errorf("too many integer arguments for a JSValue")
			}
			out = append(out, ints[ni], ints[ni+1])
			ni += 2
		case F64:
			if nf >= 8 {
				return nil, fmt.Errorf("too many floating-point arguments")
			}
			out = append(out, fmt.Sprintf("d%d", nf))
			nf++
		default:
			return nil, fmt.Errorf("bad parameter type")
		}
	}
	return out, nil
}

func a64RetRegs(t Type) []string {
	switch t {
	case I64, Ptr:
		return []string{"x0"}
	case F64:
		return []string{"d0"}
	case JSV:
		return []string{"x0", "x1"}
	}
	return nil
}

// wordTypes lists, per 8-byte word of the params (or the result), whether it
// is a double, in layout order.
func wordTypes(ts []Type) []bool {
	var out []bool
	for _, t := range ts {
		switch t {
		case JSV:
			out = append(out, false, false)
		case F64:
			out = append(out, true)
		case Void:
		default:
			out = append(out, false)
		}
	}
	return out
}

func a64GoReg(r string) string {
	if r[0] == 'd' {
		return "F" + r[1:]
	}
	return "R" + r[1:]
}

func a64Clean(l string) string {
	if i := strings.Index(l, "//"); i >= 0 {
		l = l[:i]
	}
	return strings.Join(strings.Fields(l), " ")
}

func a64Body(asm, name string) ([]string, error) {
	lines := strings.Split(asm, "\n")
	start := -1
	for i, l := range lines {
		if a64Clean(l) == name+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("%s: not found", name)
	}
	var body []string
	for _, raw := range lines[start+1:] {
		l := a64Clean(raw)
		switch {
		case l == "":
		case strings.HasPrefix(l, ".Lfunc_end"):
			return body, nil
		case strings.HasPrefix(l, ".cfi_"):
		case strings.HasPrefix(l, ".p2align"):
			body = append(body, l) // loop alignment: assembled as NOPs
		case strings.HasPrefix(l, ".") && !strings.HasSuffix(l, ":"):
			return nil, fmt.Errorf("%s: directive inside function: %q", name, l)
		default:
			body = append(body, l)
		}
	}
	return nil, fmt.Errorf("%s: no .Lfunc_end", name)
}

var (
	reA64Pre      = regexp.MustCompile(`^(stp|str) (.+), \[sp, #-(\d+)\]!$`)
	reA64Post     = regexp.MustCompile(`^(ldp|ldr) (.+), \[sp\], #(\d+)$`)
	reA64SubSP    = regexp.MustCompile(`^sub sp, sp, #(\d+)(?:, lsl #(12))?$`)
	reA64AddSP    = regexp.MustCompile(`^add sp, sp, #(\d+)(?:, lsl #(12))?$`)
	reA64MemSP    = regexp.MustCompile(`\[sp(?:, #(-?\d+))?\]`)
	reA64SP       = regexp.MustCompile(`\bsp\b`)
	reA64BL       = regexp.MustCompile(`^bl (\S+)$`)
	reA64BLR      = regexp.MustCompile(`^blr (x\d+)$`)
	reA64B        = regexp.MustCompile(`^b (\S+)$`)
	reA64Reserved = regexp.MustCompile(`\b([xw](18|26|27|28|29)|fp)\b`)
	reA64LR       = regexp.MustCompile(`\b(x30|w30|lr)\b`)
)

var reA64PairSP = regexp.MustCompile(`^(stp|ldp) ([xwdsq])(\d+|zr), ([xwdsq])(\d+|zr), \[sp, #(\d+)\]$`)

// a64FitPair splits a register pair store or load whose rebased offset no
// longer fits the pair form (a signed 7-bit multiple of the register size)
// into two single ones, whose unsigned 12-bit scaled offset reaches far
// enough for any frame the lowering accepts.
func a64FitPair(l string) []string {
	m := reA64PairSP.FindStringSubmatch(l)
	if m == nil {
		return []string{l}
	}
	size := map[string]int{"x": 8, "d": 8, "w": 4, "s": 4, "q": 16}[m[2]]
	off, _ := strconv.Atoi(m[6])
	if off <= 63*size {
		return []string{l}
	}
	op := map[string]string{"stp": "str", "ldp": "ldr"}[m[1]]
	return []string{
		fmt.Sprintf("%s %s%s, [sp, #%d]", op, m[2], m[3], off),
		fmt.Sprintf("%s %s%s, [sp, #%d]", op, m[4], m[5], off+size),
	}
}

var (
	reA64PoolPage = regexp.MustCompile(`^adrp (x\d+), (\.LCPI\w+)$`)
	reA64PoolLoad = regexp.MustCompile(`^(ldr [bhsdqxw]\d+,) \[(x\d+), :lo12:(\.LCPI\w+)\]$`)
	reA64PoolAdd  = regexp.MustCompile(`^add (x\d+), (x\d+), :lo12:(\.LCPI\w+)$`)
	reA64Data     = regexp.MustCompile(`^\.(xword|word|hword|byte|quad|long|short)\s`)
)

// a64Pools collects the constant pools of an llc assembly file: each
// .LCPI label with its data lines.
func a64Pools(asm string) map[string][]string {
	pools := map[string][]string{}
	cur := ""
	for _, raw := range strings.Split(asm, "\n") {
		l := a64Clean(raw)
		switch {
		case strings.HasPrefix(l, ".LCPI") && strings.HasSuffix(l, ":"):
			cur = strings.TrimSuffix(l, ":")
		case cur != "" && reA64Data.MatchString(l):
			pools[cur] = append(pools[cur], l)
		case l != "":
			cur = ""
		}
	}
	return pools
}

func a64Imm(n, shift string) int {
	v, _ := strconv.Atoi(n)
	if shift == "12" {
		v <<= 12
	}
	return v
}

func isA64Label(l string) bool { return strings.HasSuffix(l, ":") }

// LowerARM64 lowers every spec'd function: unified ones from the engine
// unit's llc output, register-entry leaves from the leaf unit's.
func LowerARM64(engineAsm, leafAsm string, specs []FuncSpec, mode ARM64Mode, noSaves bool) (*ARM64Unit, error) {
	u := &ARM64Unit{Mode: mode, NoSaves: noSaves}
	pools := a64Pools(engineAsm)
	for k, v := range a64Pools(leafAsm) {
		pools[k] = v
	}
	var text strings.Builder
	text.WriteString(".text\n")
	stubs := map[string]bool{}
	var indirect []Sig
	for _, fn := range specs {
		asm := engineAsm
		if fn.Kind == Register {
			asm = leafAsm
		}
		lines, f, err := lowerA64Func(asm, fn, mode, noSaves, u, stubs, &indirect, pools)
		if err != nil {
			return nil, err
		}
		u.Funcs = append(u.Funcs, f)
		fmt.Fprintf(&text, ".globl %s\n.p2align 4\n.type %s,@function\n%s:\n\t%s\n.Lgoret_%s:\n.size %s, .Lgoret_%s-%s\n",
			fn.Name, fn.Name, fn.Name, strings.Join(lines, "\n\t"), fn.Name, fn.Name, fn.Name, fn.Name)
	}
	for i, s := range indirect {
		u.Stubs = append(u.Stubs, ARM64CallStub{Name: fmt.Sprintf("icall_%d", i), Sig: s})
	}
	u.Lowered = text.String()
	return u, nil
}

var reA64CSRReg = regexp.MustCompile(`\b(?:[xw](19|2[0-5])|[bhsdqv]([89]|1[0-5]))\b`)

// a64BodyCSR returns, in arm64CSR order, the callee-saved registers a body
// names. AAPCS code must save any it changes, so this is the set its
// prologue saves, which is what a unified call must keep for it.
func a64BodyCSR(body []string) []string {
	used := map[string]bool{}
	for _, l := range body {
		if isA64Label(l) {
			continue
		}
		for _, g := range reA64CSRReg.FindAllStringSubmatch(l, -1) {
			if g[1] != "" {
				used["x"+g[1]] = true
			} else {
				used["d"+g[2]] = true
			}
		}
	}
	var out []string
	for _, r := range arm64CSR {
		if used[r] {
			out = append(out, r)
		}
	}
	return out
}

func lowerA64Func(asm string, fn FuncSpec, mode ARM64Mode, noSaves bool, u *ARM64Unit,
	stubs map[string]bool, indirect *[]Sig, pools map[string][]string) ([]string, ARM64Func, error) {
	fail := func(f string, a ...any) ([]string, ARM64Func, error) {
		return nil, ARM64Func{}, fmt.Errorf("%s: %s", fn.Name, fmt.Sprintf(f, a...))
	}
	body, err := a64Body(asm, fn.Name)
	if err != nil {
		return nil, ARM64Func{}, err
	}
	leaf := true
	for _, l := range body {
		if !isA64Label(l) && (reA64SP.MatchString(l) || strings.HasPrefix(l, "bl ") ||
			strings.HasPrefix(l, "blr ") || reA64LR.MatchString(l)) {
			leaf = false
		}
	}
	if fn.Kind == Register && !leaf {
		return fail("a register-entry leaf must not use the stack or call")
	}
	pn := mode == ARM64PreserveNone
	// F: everything the entry run allocates before the first label or branch.
	F := 0
	for _, l := range body {
		if isA64Label(l) || strings.HasPrefix(l, "b") || l == "ret" {
			break
		}
		if m := reA64Pre.FindStringSubmatch(l); m != nil {
			F += a64Imm(m[3], "")
		} else if m := reA64SubSP.FindStringSubmatch(l); m != nil {
			F += a64Imm(m[1], m[2])
		}
	}
	if F%16 != 0 {
		return fail("frame %d not 16-byte aligned", F)
	}
	// The outgoing ABI0 area at 8(RSP) is as large as the largest unified
	// call's arguments and results. The stubs of the stub mode have their own.
	outArea, nIndirect, nCalls := 0, 0, 0
	for _, l := range body {
		var sig Sig
		if m := reA64BL.FindStringSubmatch(l); m != nil {
			if calleeKinds[m[1]] == Register {
				continue
			}
			s, ok := calleeSigs[m[1]]
			if !ok {
				return fail("call to unknown %s", m[1])
			}
			sig = s
		} else if reA64BLR.MatchString(l) {
			if nIndirect >= len(fn.Indirect) {
				return fail("more indirect calls than declared")
			}
			sig = fn.Indirect[nIndirect]
			nIndirect++
		} else {
			continue
		}
		nCalls++
		if _, _, n := layout(sig); n > outArea && mode != ARM64Stub {
			outArea = n
		}
	}
	L := align(8+outArea, 16) // the LLVM frame's place in the Go frame
	var saves []string
	switch mode {
	case ARM64SaveAll:
		saves = arm64CSR
	case ARM64SaveBody:
		saves = a64BodyCSR(body)
	}
	if noSaves || nCalls == 0 {
		saves = nil
	}
	saveAt := L + F
	goret := ".Lgoret_" + fn.Name
	var out []string
	indIdx := 0
	D := 0 // bytes of the LLVM frame allocated at this point
	goOff := func(k int) int { return L + (F - D) + k }
	var usedPools []string          // constant pools this function reads, in first-use order
	lastAdrp := map[string]string{} // register -> pool of its last page address, in text order
	// adapter is the inline call-site glue: keep the callee-saved registers
	// the mode asks for, store the argument registers into the ABI0 area,
	// call, load the result registers back.
	adapter := func(sig Sig, call string) error {
		regs, err := a64ArgRegs(sig, pn)
		if err != nil {
			return err
		}
		argOff, retOff, _ := layout(sig)
		for j, r := range saves {
			out = append(out, fmt.Sprintf("str %s, [sp, #%d]", r, saveAt+8*j))
		}
		k := 0
		for i, p := range sig.Params {
			for w := range wordTypes([]Type{p}) {
				out = append(out, fmt.Sprintf("str %s, [sp, #%d]", regs[k], 8+argOff[i]+8*w))
				k++
			}
		}
		out = append(out, call)
		for k, r := range a64RetRegs(sig.Ret) {
			out = append(out, fmt.Sprintf("ldr %s, [sp, #%d]", r, 8+retOff+8*k))
		}
		for j, r := range saves {
			out = append(out, fmt.Sprintf("ldr %s, [sp, #%d]", r, saveAt+8*j))
		}
		u.CallSites++
		u.SavedRegs += len(saves)
		return nil
	}
	for _, l := range body {
		// A constant pool moves into the function, after its code: the
		// page address becomes a PC-relative ADR to it, and the page offset
		// in the load becomes 0.
		if m := reA64PoolPage.FindStringSubmatch(l); m != nil {
			if _, ok := pools[m[2]]; !ok {
				return fail("address of unknown data %s", m[2])
			}
			if !slices.Contains(usedPools, m[2]) {
				usedPools = append(usedPools, m[2])
			}
			lastAdrp[m[1]] = m[2]
			out = append(out, fmt.Sprintf("adr %s, %s", m[1], m[2]))
			continue
		}
		if strings.Contains(l, ":lo12:") {
			// The offset must name the pool whose address the base register
			// got: a page shared by two pools would otherwise read the
			// wrong one.
			if m := reA64PoolLoad.FindStringSubmatch(l); m != nil && lastAdrp[m[2]] == m[3] {
				out = append(out, fmt.Sprintf("%s [%s]", m[1], m[2]))
				continue
			}
			if m := reA64PoolAdd.FindStringSubmatch(l); m != nil && lastAdrp[m[2]] == m[3] {
				if m[1] != m[2] {
					out = append(out, fmt.Sprintf("mov %s, %s", m[1], m[2]))
				}
				continue
			}
			return fail("unreviewed page-offset use %q", l)
		}
		if isA64Label(l) || strings.HasPrefix(l, ".p2align") {
			if isA64Label(l) && D != F {
				return fail("label %s while the frame is half built", l)
			}
			out = append(out, l)
			continue
		}
		if m := reA64Pre.FindStringSubmatch(l); m != nil {
			D += a64Imm(m[3], "")
			if D > F {
				return fail("frame grows past its prologue: %q", l)
			}
			l = fmt.Sprintf("%s %s, [sp, #%d]", m[1], m[2], goOff(0))
		} else if m := reA64SubSP.FindStringSubmatch(l); m != nil {
			D += a64Imm(m[1], m[2])
			if D > F {
				return fail("frame grows past its prologue: %q", l)
			}
			continue
		} else if m := reA64AddSP.FindStringSubmatch(l); m != nil {
			D -= a64Imm(m[1], m[2])
			if D < 0 {
				return fail("frame freed twice: %q", l)
			}
			continue
		} else if m := reA64Post.FindStringSubmatch(l); m != nil {
			l = fmt.Sprintf("%s %s, [sp, #%d]", m[1], m[2], goOff(0))
			D -= a64Imm(m[3], "")
			if D < 0 {
				return fail("frame freed twice: %q", l)
			}
		} else if l == "ret" {
			if D != 0 {
				return fail("ret with %d bytes of frame still allocated", D)
			}
			out = append(out, "b "+goret)
			D = F // the code after a return block runs with the full frame
			continue
		} else if reA64SP.MatchString(l) {
			if len(reA64SP.FindAllString(l, -1)) != 1 || !reA64MemSP.MatchString(l) ||
				strings.Contains(l, "]!") || strings.Contains(l, "], #") {
				return fail("stack address escapes or unreviewed stack use: %q", l)
			}
			var rerr error
			l = reA64MemSP.ReplaceAllStringFunc(l, func(s string) string {
				m := reA64MemSP.FindStringSubmatch(s)
				k := 0
				if m[1] != "" {
					k, _ = strconv.Atoi(m[1])
				}
				if k < 0 || k >= D {
					rerr = fmt.Errorf("access outside the LLVM frame: %q", l)
				}
				return fmt.Sprintf("[sp, #%d]", goOff(k))
			})
			if rerr != nil {
				return fail("%v", rerr)
			}
		}
		op := strings.Fields(l)[0]
		switch op {
		case "br", "ret", "adr", "adrp", "eret":
			return fail("forbidden instruction %q", l)
		}
		if forbiddenA64[op] {
			return fail("forbidden instruction %q", l)
		}
		if reA64Reserved.MatchString(l) {
			return fail("reserved register in %q", l)
		}
		if reA64LR.MatchString(l) && !((strings.HasPrefix(op, "st") || strings.HasPrefix(op, "ld")) && strings.Contains(l, "[sp")) {
			return fail("LR used outside a stack save: %q", l)
		}
		if m := reA64B.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], ".L") {
			return fail("tail jump %q", l)
		}
		if m := reA64BL.FindStringSubmatch(l); m != nil {
			t := m[1]
			if calleeKinds[t] == Register {
				out = append(out, "bl go_"+t)
				continue
			}
			sig := calleeSigs[t]
			if mode == ARM64Stub {
				name := "call_" + t
				if !stubs[name] {
					stubs[name] = true
					u.Stubs = append(u.Stubs, ARM64CallStub{Name: name, Target: t, Sig: sig})
				}
				u.CallSites++
				u.SavedRegs += len(arm64CSR)
				out = append(out, "bl go_"+name)
				continue
			}
			if err := adapter(sig, "bl go_"+t); err != nil {
				return fail("call to %s: %v", t, err)
			}
			continue
		}
		if m := reA64BLR.FindStringSubmatch(l); m != nil {
			sig := fn.Indirect[indIdx]
			indIdx++
			if m[1] != "x9" {
				out = append(out, "mov x9, "+m[1])
			}
			if mode == ARM64Stub {
				k := -1
				for i, s := range *indirect {
					if fmt.Sprint(s) == fmt.Sprint(sig) {
						k = i
					}
				}
				if k < 0 {
					k = len(*indirect)
					*indirect = append(*indirect, sig)
				}
				u.CallSites++
				u.SavedRegs += len(arm64CSR)
				out = append(out, fmt.Sprintf("bl go_icall_%d", k))
				continue
			}
			if err := adapter(sig, "bl go_icall"); err != nil {
				return fail("indirect call: %v", err)
			}
			continue
		}
		out = append(out, a64FitPair(l)...)
	}
	if len(usedPools) > 0 {
		// Nothing may run into the data: the code before it must end in an
		// unconditional branch.
		last := ""
		for i := len(out) - 1; i >= 0 && last == ""; i-- {
			if !isA64Label(out[i]) && !strings.HasPrefix(out[i], ".p2align") {
				last = out[i]
			}
		}
		if !reA64B.MatchString(last) {
			out = append(out, "b "+goret)
		}
		out = append(out, ".p2align 4", fn.Name+".const:")
		for _, p := range usedPools {
			out = append(out, ".p2align 4", p+":")
			out = append(out, pools[p]...)
		}
		out = append(out, fn.Name+".cend:")
	}
	f := ARM64Func{Sig: fn.Sig, Leaf: leaf, LLVMFrame: F, SaveSlots: len(saves)}
	_, _, argBytes := layout(fn.Sig)
	f.Meta = Meta{Name: fn.Name, Kind: fn.Kind, GoName: fn.Name, ArgBytes: argBytes, RetType: fn.Sig.Ret}
	if !leaf {
		// 0(RSP) is Go's saved LR: the frame is $N with N+8 a multiple of 16.
		f.Frame = L + F + align(8*len(saves), 16) - 8
	}
	return out, f, nil
}

// ---- Emit ----

// EmitARM64 writes the Go assembly of the lowered unit from its object.
func EmitARM64(objPath string, u *ARM64Unit, buildTag string) (string, error) {
	ef, err := elf.Open(objPath)
	if err != nil {
		return "", err
	}
	defer ef.Close()
	if ef.Machine != elf.EM_AARCH64 {
		return "", fmt.Errorf("not an aarch64 object")
	}
	for _, s := range ef.Sections {
		if s.Flags&elf.SHF_WRITE != 0 && s.Size > 0 {
			return "", fmt.Errorf("writable section %s", s.Name)
		}
		if s.Flags&elf.SHF_ALLOC != 0 && s.Size > 0 && s.Name != ".text" {
			return "", fmt.Errorf("unreviewed section %s", s.Name)
		}
	}
	text := ef.Section(".text")
	code, err := text.Data()
	if err != nil {
		return "", err
	}
	syms, err := ef.Symbols()
	if err != nil {
		return "", err
	}
	calls := map[int]string{}
	for _, s := range ef.Sections {
		if s.Type != elf.SHT_RELA || int(s.Info) >= len(ef.Sections) || ef.Sections[s.Info] != text {
			continue
		}
		d, _ := s.Data()
		for p := 0; p+24 <= len(d); p += 24 {
			off := binary.LittleEndian.Uint64(d[p:])
			info := binary.LittleEndian.Uint64(d[p+8:])
			add := int64(binary.LittleEndian.Uint64(d[p+16:]))
			sym := syms[int(info>>32)-1]
			if elf.R_AARCH64(info&0xffffffff) != elf.R_AARCH64_CALL26 || add != 0 || !strings.HasPrefix(sym.Name, "go_") {
				return "", fmt.Errorf("relocation %v %s%+d at %#x: only CALL26 to a stub or leaf", elf.R_AARCH64(info&0xffffffff), sym.Name, add, off)
			}
			calls[int(off)] = strings.TrimPrefix(sym.Name, "go_")
		}
	}
	addr := map[string][2]int{}
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) == elf.STT_FUNC || strings.HasSuffix(s.Name, ".const") || strings.HasSuffix(s.Name, ".cend") {
			addr[s.Name] = [2]int{int(s.Value), int(s.Value + s.Size)}
		}
	}
	pn := u.Mode == ARM64PreserveNone
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by internal/goasm/gen. DO NOT EDIT.\n//go:build %s\n\n#include \"textflag.h\"\n#include \"funcdata.h\"\n", buildTag)
	for _, f := range u.Funcs {
		r, ok := addr[f.Name]
		if !ok {
			return "", fmt.Errorf("%s: absent from the object", f.Name)
		}
		var argRegs []string
		argOff, retOff, _ := layout(f.Sig)
		if f.Kind == Register {
			fmt.Fprintf(&b, "\n// %s: register-entry leaf (AAPCS).\nTEXT ·%s(SB), NOSPLIT|NOFRAME, $0-0\n", f.Name, f.GoName)
		} else {
			argRegs, err = a64ArgRegs(f.Sig, pn)
			if err != nil {
				return "", fmt.Errorf("%s: %w", f.Name, err)
			}
			fmt.Fprintf(&b, "\n// %s: unified entry, LLVM frame %d bytes, %d save slots.\nTEXT ·%s(SB), $%d-%d\n\tNO_LOCAL_POINTERS\n",
				f.Name, f.LLVMFrame, f.SaveSlots, f.GoName, f.Frame, f.ArgBytes)
			k := 0
			for i, p := range f.Sig.Params {
				for w, fp := range wordTypes([]Type{p}) {
					mov := "MOVD"
					if fp {
						mov = "FMOVD"
					}
					fmt.Fprintf(&b, "\t%s arg%d_%d+%d(FP), %s\n", mov, i, w, argOff[i]+8*w, a64GoReg(argRegs[k]))
					k++
				}
			}
		}
		c0, hasPool := addr[f.Name+".const"]
		c1 := addr[f.Name+".cend"]
		for pc := r[0]; pc < r[1]; pc += 4 {
			w := binary.LittleEndian.Uint32(code[pc:])
			if hasPool && pc >= c0[0] && pc < c1[0] {
				fmt.Fprintf(&b, "\tWORD $0x%08x\n", w) // constant pool data
				continue
			}
			if t, ok := calls[pc]; ok {
				if t == "icall" {
					b.WriteString("\tCALL (R9)\n")
				} else {
					fmt.Fprintf(&b, "\tBL ·%s(SB)\n", t)
				}
				continue
			}
			if w == 0xd65f03c0 || w&0xfffffc1f == 0xd63f0000 || w&0xfffffc1f == 0xd61f0000 || w&0xfc000000 == 0x94000000 {
				return "", fmt.Errorf("%s: ret, blr, br or bl hidden in the bytes at %#x", f.Name, pc)
			}
			if t, ok := a64BranchTarget(w, pc); ok && (t < r[0] || t > r[1]) {
				return "", fmt.Errorf("%s: branch at %#x leaves the function", f.Name, pc)
			}
			if why := a64ForbiddenWord(w); why != "" {
				return "", fmt.Errorf("%s: forbidden instruction %#08x at %#x: %s", f.Name, w, pc, why)
			}
			fmt.Fprintf(&b, "\tWORD $0x%08x\n", w)
		}
		if f.Kind != Register {
			for k, reg := range a64RetRegs(f.Sig.Ret) {
				mov := "MOVD"
				if reg[0] == 'd' {
					mov = "FMOVD"
				}
				fmt.Fprintf(&b, "\t%s %s, ret_%d+%d(FP)\n", mov, a64GoReg(reg), k, retOff+8*k)
			}
		}
		b.WriteString("\tRET\n")
	}
	for _, s := range u.Stubs {
		if err := emitA64Stub(&b, s, u.Mode, u.NoSaves); err != nil {
			return "", err
		}
	}
	var table []string
	for _, f := range u.Funcs {
		if f.Kind == Unified {
			table = append(table, f.GoName)
		}
	}
	fmt.Fprintf(&b, "\n// Function table: unified entries only.\nGLOBL ·unifiedTable(SB), RODATA, $%d\n", 8*len(table))
	for i, n := range table {
		fmt.Fprintf(&b, "DATA ·unifiedTable+%d(SB)/8, $·%s(SB)\n", 8*i, n)
	}
	return b.String(), nil
}

// emitA64Stub writes a NOSPLIT stub with a register entry: it stores the
// argument registers into the ABI0 slots, calls, and loads the result back.
// In stub mode it also saves and restores R19-R25 and F8-F15 around the call.
func emitA64Stub(b *strings.Builder, s ARM64CallStub, mode ARM64Mode, noSaves bool) error {
	regs, err := a64ArgRegs(s.Sig, mode == ARM64PreserveNone)
	if err != nil {
		return fmt.Errorf("stub %s: %w", s.Name, err)
	}
	argOff, retOff, size := layout(s.Sig)
	var saved []string
	if !noSaves {
		saved = arm64CSR
	}
	saveAt := 8 + size
	fmt.Fprintf(b, "\n// %s: register entry, ABI0 call.\nTEXT ·%s(SB), NOSPLIT, $%d-0\n\tNO_LOCAL_POINTERS\n", s.Name, s.Name, size+8*len(saved))
	mov := func(r string) string {
		if r[0] == 'd' {
			return "FMOVD"
		}
		return "MOVD"
	}
	for i, r := range saved {
		fmt.Fprintf(b, "\t%s %s, %d(RSP)\n", mov(r), a64GoReg(r), saveAt+8*i)
	}
	k := 0
	for i, p := range s.Sig.Params {
		for w := range wordTypes([]Type{p}) {
			fmt.Fprintf(b, "\t%s %s, %d(RSP)\n", mov(regs[k]), a64GoReg(regs[k]), 8+argOff[i]+8*w)
			k++
		}
	}
	if s.Target == "" {
		b.WriteString("\tCALL (R9)\n")
	} else {
		fmt.Fprintf(b, "\tBL ·%s(SB)\n", s.Target)
	}
	for k, r := range a64RetRegs(s.Sig.Ret) {
		fmt.Fprintf(b, "\t%s %d(RSP), %s\n", mov(r), 8+retOff+8*k, a64GoReg(r))
	}
	for i, r := range saved {
		fmt.Fprintf(b, "\t%s %d(RSP), %s\n", mov(r), saveAt+8*i, a64GoReg(r))
	}
	b.WriteString("\tRET\n")
	return nil
}

func a64BranchTarget(w uint32, pc int) (int, bool) {
	sext := func(v uint32, bits uint) int { return int(int32(v<<(32-bits)) >> (32 - bits)) }
	switch {
	case w&0xfc000000 == 0x14000000: // B
		return pc + 4*sext(w&0x3ffffff, 26), true
	case w&0xff000010 == 0x54000000: // B.cond
		return pc + 4*sext((w>>5)&0x7ffff, 19), true
	case w&0x7e000000 == 0x34000000: // CBZ, CBNZ
		return pc + 4*sext((w>>5)&0x7ffff, 19), true
	case w&0x7e000000 == 0x36000000: // TBZ, TBNZ
		return pc + 4*sext((w>>5)&0x3fff, 14), true
	}
	return 0, false
}

// FuncSizes returns the byte size of every function symbol of an ELF object.
func FuncSizes(objPath string) (map[string]int, error) {
	ef, err := elf.Open(objPath)
	if err != nil {
		return nil, err
	}
	defer ef.Close()
	syms, err := ef.Symbols()
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) == elf.STT_FUNC {
			out[s.Name] = int(s.Size)
		}
	}
	return out, nil
}
