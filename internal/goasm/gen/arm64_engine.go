// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The arm64 lowering of the whole engine. It takes llc's aarch64 text of one
// function (preserve_none on every unified entry, the -mode pn of cmd/engine)
// and rewrites it as text for llvm-mc, as LowerARM64 does for the units of
// the prototype, plus what the engine has and the prototype does not: data
// whose address the code takes, jump tables and the computed goto of the
// interpreter, the label llc writes at each indirect call, and calls of the
// register-entry leaves (memcpy, memmove, memset).
//
// The Go frame of a function with a frame: 0(RSP) is the LR Go saves, the
// outgoing ABI0 area starts at 8(RSP), and the LLVM frame of F bytes sits at
// the next 16-byte boundary L. The TEXT declares $(L+F-8), so that with the
// LR it is L+F, a multiple of 16, and the Go assembler adds 16 bytes on top
// for the frame pointer its callees save there. Every SP move of LLVM's
// prologue and epilogues is dropped, and every access [sp, #k] is rebased on
// the Go frame.
//
// Data. llc writes the engine in the tiny code model, where every reference
// to data is one instruction that names the whole address: an adr of data or
// of a function, or a load of a label. Go assembly cannot attach a
// relocation to the bytes of an instruction, so the lowering writes each one
// on its own as an instruction that reaches the same address: an adr of
// other data or of a function becomes MOVD $sym(SB) in Go (an adrp and an
// add of the whole address), an adr of the function's own data (constant
// pools, jump tables, the table of the computed goto) stays an adr of it in
// the body, and a load of a label loads the data the plan put in the body,
// the function's own or a copy. No rewrite depends on any other instruction,
// so there is nothing to follow through the blocks. The page forms of the
// small code model (adrp and :lo12:), where the second instruction is right
// only while the register still holds the page of the first, are refused.
package gen

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ARM64Env is what lowering one function of the whole engine needs beyond
// its own text.
type ARM64Env struct {
	Plan  *DataPlan
	Funcs map[string]bool
	// Facts are llc's registers of every function and call (facts.cpp),
	// Probes those of every unified definition's arguments. When set, each
	// entry must load its arguments into the registers llc's probe reads
	// them from, and each call must store its arguments from the registers
	// llc passes them in and load its result into the ones llc reads.
	Facts, Probes RegFacts
	// Variadic are the functions the pass left variadic: a call to one is
	// refused, since its arguments past the registers go on the stack.
	Variadic map[string]bool
}

var (
	reA64Lo12   = regexp.MustCompile(`:lo12:([.\w$]+)([+-]\d+)?`)
	reA64BR     = regexp.MustCompile(`^br (x\d+)$`)
	reA64AdrSym = regexp.MustCompile(`^adr (x\d+), ([.\w$]+)([+-]\d+)?$`)
	reA64LdLit  = regexp.MustCompile(`^(ldr|ldrsw) ([xwsdq]\d+), ([.\w$]+)([+-]\d+)?$`)
	reA64LastOp = regexp.MustCompile(`, ([.\w$]+)([+-]\d+)?$`)
	reA64RegTok = regexp.MustCompile(`\b([xw])(\d+)\b`)
	reA64Mod    = regexp.MustCompile(`:(got|abs_|tprel|dtprel|tlsdesc|gottprel)`)
)

// a64NoDest are the mnemonics whose first operand is read, not written.
var a64NoDest = map[string]bool{"str": true, "strb": true, "strh": true, "stp": true, "stur": true, "sturb": true,
	"sturh": true, "stnp": true, "cmp": true, "cmn": true, "tst": true, "ccmp": true, "ccmn": true, "fcmp": true,
	"fccmp": true, "fcmpe": true, "cbz": true, "cbnz": true, "tbz": true, "tbnz": true, "br": true, "blr": true,
	"prfm": true, "st1": true, "st2": true, "st3": true, "st4": true, "movk": true, "bfi": true, "bfxil": true,
	"bfm": true, "ins": true, "mov.": true}

// a64RegNum gives the number of the general register an operand token names
// (x5 and w5 are 5), or -1.
func a64Uses(l string) map[int]bool {
	out := map[int]bool{}
	for _, m := range reA64RegTok.FindAllStringSubmatch(l, -1) {
		n, _ := strconv.Atoi(m[2])
		out[n] = true
	}
	return out
}

// a64Operands splits the operands of an instruction at the top-level commas.
func a64Operands(l string) (op string, ops []string) {
	op, rest, _ := strings.Cut(l, " ")
	depth, start := 0, 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 0 {
				ops = append(ops, strings.TrimSpace(rest[start:i]))
				start = i + 1
			}
		}
	}
	if s := strings.TrimSpace(rest[start:]); s != "" {
		ops = append(ops, s)
	}
	return op, ops
}

// a64Reads says whether an instruction reads general register r, and
// whether it writes it without reading it.
func a64Reads(l string, r int) (reads, writesOnly bool) {
	op, ops := a64Operands(l)
	if len(ops) == 0 {
		return false, false
	}
	mentions := func(s string) bool { return a64Uses(s)[r] }
	// A load of a pair writes its first two operands.
	dests := 1
	if (op == "ldp" || op == "ldnp" || op == "ldpsw") && len(ops) > 2 {
		dests = 2
	}
	first := false
	for _, o := range ops[:dests] {
		first = first || mentions(o)
	}
	rest := false
	for _, o := range ops[dests:] {
		rest = rest || mentions(o)
	}
	if a64NoDest[op] || strings.HasPrefix(op, "st") || strings.HasPrefix(op, "b.") || op == "b" {
		return first || rest, false
	}
	if first && !rest {
		return false, true
	}
	return first || rest, false
}

// LowerARM64EngineFunc lowers one function of the whole engine from the body
// lines of llc's text (after its label, up to .Lfunc_end). The result is the
// text for llvm-mc, from the .globl of the function to its .size.
func LowerARM64EngineFunc(lines []string, fn FuncSpec, env *ARM64Env) (string, Meta, error) {
	fail := func(f string, a ...any) (string, Meta, error) {
		return "", Meta{}, fmt.Errorf("%s: %s", fn.Name, fmt.Sprintf(f, a...))
	}
	// Gather the body: instruction and label lines, the signature of each
	// indirect call from llc's label before it.
	var body []string
	for _, raw := range lines {
		l := a64Clean(raw)
		switch {
		case l == "":
		case strings.HasPrefix(l, ".cfi_"):
		case strings.HasPrefix(l, ICallPrefix) && strings.HasSuffix(l, ":"):
			name := strings.TrimSuffix(l, ":")
			f, _, id, ok := ParseICallSymbol(name)
			if !ok || f != fn.Name {
				return fail("indirect-call label %s is not one of this function", name)
			}
			code, err := ICallCode(id)
			if err != nil {
				return fail("indirect call: %v", err)
			}
			body = append(body, "@icall "+code)
		case strings.HasPrefix(l, ".p2align"):
			body = append(body, l)
		case strings.HasPrefix(l, ".") && !strings.HasSuffix(l, ":"):
			return fail("directive inside the function: %q", l)
		default:
			body = append(body, l)
		}
	}
	leaf := true
	for _, l := range body {
		if !isA64Label(l) && (reA64SP.MatchString(l) || strings.HasPrefix(l, "bl ") ||
			strings.HasPrefix(l, "blr ") || reA64LR.MatchString(l)) {
			leaf = false
		}
	}
	// F: everything the entry run allocates before the first label or branch.
	F := 0
	for _, l := range body {
		if isA64Label(l) || strings.HasPrefix(l, "b") || l == "ret" || strings.HasPrefix(l, "@") {
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
	// The outgoing ABI0 area holds the arguments and result of the largest
	// unified call.
	sigOf := func(code string) (Sig, error) { return SigFromCode(code) }
	outArea, nCalls := 0, 0
	consider := func(s Sig) {
		if _, _, n := layout(s); n > outArea {
			outArea = n
		}
	}
	for _, l := range body {
		switch {
		case strings.HasPrefix(l, "@icall "):
			if s, err := sigOf(strings.TrimPrefix(l, "@icall ")); err == nil {
				consider(s)
			}
			nCalls++
		case reA64BL.MatchString(l):
			t := reA64BL.FindStringSubmatch(l)[1]
			nCalls++
			if calleeKinds[t] == Register {
				continue
			}
			s, ok := callSig(t, fn, calleeSigs)
			if !ok {
				return fail("call to unknown %s", t)
			}
			consider(s)
		}
	}
	L := align(8+outArea, 16)
	goret := ".Lgoret_" + fn.Name
	rec := &FuncRecord{Name: fn.Name, Kind: fn.Kind, Sig: fn.Sig, PreserveNone: true, Local: F, LocalBase: L}
	var ff *FuncFacts
	if env.Facts != nil {
		ff = env.Facts[fn.Name]
		if ff == nil {
			return fail("llc wrote no register facts for it")
		}
		if len(ff.Calls) != nCalls {
			return fail("%d calls in the text, llc's facts have %d", nCalls, len(ff.Calls))
		}
	}
	// The entry: llc's probe of a unified definition reads its arguments
	// from the registers the lowering loads them into.
	if fn.Kind == Unified && env.Probes != nil {
		regs, err := a64ArgRegs(fn.Sig, true)
		if err != nil {
			return fail("%v", err)
		}
		p := env.Probes[ProbePrefix+fn.Name]
		if p == nil {
			return fail("no probe facts")
		}
		if err := a64SameRegs("entry", regs, p.Args); err != nil {
			return fail("%v", err)
		}
	}
	var out []string
	D := 0 // bytes of the LLVM frame allocated at this point
	goOff := func(k int) int { return L + (F - D) + k }
	callIdx := 0
	pending := "" // signature of the indirect call on the next line
	// materialize puts the whole address of sym+add in register x<r>.
	materialize := func(reg, sym string, add int64) ([]string, error) {
		if env.Funcs[sym] {
			if add != 0 {
				return nil, fmt.Errorf("address of the function %s%+d", sym, add)
			}
			rec.Refs = append(rec.Refs, RefRecord{Label: sym, Func: true})
			return []string{"adrp " + reg + ", go_" + sym, "add " + reg + ", " + reg + ", :lo12:go_" + sym}, nil
		}
		if r, ok := env.Plan.Rename[fn.Name][sym]; ok {
			rec.Refs = append(rec.Refs, RefRecord{Label: sym, Add: add})
			return []string{fmt.Sprintf("adr %s, %s%s", reg, r, a64Add(add))}, nil
		}
		if off, ok := env.Plan.BlobOff[sym]; ok {
			rec.Refs = append(rec.Refs, RefRecord{Label: sym, Add: add, Blob: true})
			t := fmt.Sprintf("__goasm_rodata+%d", int64(off)+add)
			return []string{"adrp " + reg + ", " + t, "add " + reg + ", " + reg + ", :lo12:" + t}, nil
		}
		return nil, fmt.Errorf("unplaced data %s", sym)
	}
	// adapter is the call-site glue: store the argument registers into the
	// ABI0 area, call, load the result registers back.
	adapter := func(sig Sig, call string, facts *CallFacts) error {
		regs, err := a64ArgRegs(sig, true)
		if err != nil {
			return err
		}
		if facts != nil {
			if err := a64SameRegs("call", regs, facts.Args); err != nil {
				return err
			}
			if err := a64SameRegs("result", a64RetRegs(sig.Ret), facts.Defs); err != nil {
				return err
			}
		}
		argOff, retOff, _ := layout(sig)
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
		return nil
	}
	nextFacts := func(target string) (*CallFacts, error) {
		if ff == nil {
			callIdx++
			return nil, nil
		}
		c := &ff.Calls[callIdx]
		callIdx++
		if c.Target != target && !(strings.HasPrefix(target, "*") && strings.HasPrefix(c.Target, "*")) {
			return nil, fmt.Errorf("call %d goes to %s, llc's facts say %s", callIdx-1, target, c.Target)
		}
		return c, nil
	}
	for li, l := range body {
		if strings.HasPrefix(l, "@icall ") {
			if li+1 >= len(body) || !reA64BLR.MatchString(body[li+1]) {
				return fail("an indirect-call label is not on an indirect call")
			}
			pending = strings.TrimPrefix(l, "@icall ")
			continue
		}
		if isA64Label(l) || strings.HasPrefix(l, ".p2align") {
			if isA64Label(l) && D != F {
				return fail("label %s while the frame is half built", l)
			}
			out = append(out, l)
			continue
		}
		if reA64Mod.MatchString(l) {
			return fail("unreviewed relocation form %q", l)
		}
		// Data in the tiny code model: an adr holds the whole address of
		// data or of a function, and a load of a label reads the data there,
		// which the plan has put in the function's body.
		if m := reA64AdrSym.FindStringSubmatch(l); m != nil && !isTextLabel(m[2]) {
			add := int64(0)
			if m[3] != "" {
				add, _ = strconv.ParseInt(m[3], 10, 64)
			}
			mat, err := materialize(m[1], m[2], add)
			if err != nil {
				return fail("%v: %q", err, l)
			}
			out = append(out, mat...)
			continue
		}
		if m := reA64LdLit.FindStringSubmatch(l); m != nil {
			lbl, ok := env.Plan.Rename[fn.Name][m[3]]
			if !ok {
				return fail("load of %s, which is not in the body of the function: %q", m[3], l)
			}
			out = append(out, fmt.Sprintf("%s %s, %s%s", m[1], m[2], lbl, m[4]))
			continue
		}
		// The page forms of the small code model: the :lo12: instruction
		// holds only where the register still has the page of its adrp,
		// which no instruction shows on its own.
		if strings.HasPrefix(l, "adrp ") || reA64Lo12.MatchString(l) {
			return fail("page-relative reference %q: the engine is built with the tiny code model", l)
		}
		if m := reA64LastOp.FindStringSubmatch(l); m != nil && a64NamesData(env, fn.Name, m[1]) {
			return fail("unreviewed reference to data %q", l)
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
		} else if m := reA64AddrSP.FindStringSubmatch(l); m != nil {
			// The address of a spill slot of a register tuple, for the st1
			// or ld1 that follows.
			k, _ := strconv.Atoi(m[2])
			if err := a64TupleSlot(body[li+1:], m[1], k, D); err != nil {
				return fail("%q: %v", l, err)
			}
			l = fmt.Sprintf("add %s, sp, #%d", m[1], goOff(k))
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
				if w := a64AccessBytes(l); k < 0 || w == 0 || k+w > D {
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
		case "ret", "eret", "adrp":
			return fail("forbidden instruction %q", l)
		case "adr":
			if m := reA64AdrSym.FindStringSubmatch(l); m == nil || !isTextLabel(m[2]) || m[3] != "" {
				return fail("unreviewed adr %q", l)
			}
		case "br":
			// A jump table or the computed goto: the target is a label of
			// this function, from a table in its body.
			if !reA64BR.MatchString(l) {
				return fail("unreviewed indirect jump %q", l)
			}
		}
		if forbiddenA64[op] {
			return fail("forbidden instruction %q", l)
		}
		switch op {
		case "fmadd", "fmsub", "fnmadd", "fnmsub", "fmla", "fmls":
			// One rounding where amd64 rounds twice (-ffp-contract=off).
			return fail("fused multiply-add %q", l)
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
		if strings.HasPrefix(op, "b.") || op == "cbz" || op == "cbnz" || op == "tbz" || op == "tbnz" {
			if _, ops := a64Operands(l); !strings.HasPrefix(ops[len(ops)-1], ".L") {
				return fail("conditional tail jump %q", l)
			}
		}
		if m := reA64BL.FindStringSubmatch(l); m != nil {
			t := m[1]
			if env.Variadic[t] {
				return fail("variadic call to %s", t)
			}
			facts, err := nextFacts(t)
			if err != nil {
				return fail("%v", err)
			}
			if calleeKinds[t] == Register {
				// A register-entry leaf takes its arguments where llc put
				// them and keeps the AAPCS callee-saved registers.
				out = append(out, "bl go_"+t)
				rec.Calls = append(rec.Calls, CallRecord{Kind: CallLeaf, Target: t})
				continue
			}
			if fn.Kind == Register {
				return fail("a register-entry leaf calls %s, which is no leaf", t)
			}
			sig, _ := callSig(t, fn, calleeSigs)
			if err := adapter(sig, "bl go_"+t, facts); err != nil {
				return fail("call to %s: %v", t, err)
			}
			rec.Calls = append(rec.Calls, CallRecord{Kind: CallUnified, Target: t, Sig: sig})
			continue
		}
		if m := reA64BLR.FindStringSubmatch(l); m != nil {
			if fn.Kind == Register {
				return fail("a register-entry leaf calls through a pointer")
			}
			if pending == "" {
				return fail("indirect call without LLVM's signature label: %q", l)
			}
			sig, err := SigFromCode(pending)
			pending = ""
			if err != nil {
				return fail("indirect call: %v", err)
			}
			facts, err := nextFacts("*")
			if err != nil {
				return fail("%v", err)
			}
			// x9 is no argument register of preserve_none, and the
			// arguments are stored before the call.
			if m[1] != "x9" {
				out = append(out, "mov x9, "+m[1])
			}
			if err := adapter(sig, "blr x9", facts); err != nil {
				return fail("indirect call: %v", err)
			}
			rec.Calls = append(rec.Calls, CallRecord{Kind: CallReg, Target: m[1], Sig: sig})
			continue
		}
		out = append(out, a64FitPair(l)...)
	}
	if D != 0 && D != F {
		return fail("the frame is half freed at the end")
	}
	// The function's own data (constant pools, jump tables, the computed
	// goto table, copies) after its code: the code before it must end in an
	// unconditional branch.
	if data := env.Plan.BodyData(fn.Name); data != "" {
		last := ""
		for i := len(out) - 1; i >= 0 && last == ""; i-- {
			if !isA64Label(out[i]) && !strings.HasPrefix(out[i], ".p2align") {
				last = out[i]
			}
		}
		if !reA64B.MatchString(last) && !reA64BR.MatchString(last) {
			out = append(out, "b "+goret)
		}
		out = append(out, ".p2align 4", fn.Name+".const:", data)
	}
	_, _, argBytes := layout(fn.Sig)
	if fn.Kind == Register {
		argBytes = 0
	}
	meta := Meta{Name: fn.Name, Kind: fn.Kind, GoName: fn.Name, ArgBytes: argBytes, RetType: fn.Sig.Ret, Rec: rec}
	if !leaf {
		meta.Frame = L + F - 8
	}
	rec.Frame = meta.Frame
	var b strings.Builder
	fmt.Fprintf(&b, ".globl %s\n.p2align 4\n.type %s,@function\n%s:\n", fn.Name, fn.Name, fn.Name)
	for _, l := range out {
		b.WriteString("\t" + l + "\n")
	}
	fmt.Fprintf(&b, ".p2align 2\n%s:\n.size %s, %s-%s\n", goret, fn.Name, goret, fn.Name)
	return b.String(), meta, nil
}

// a64NamesData says whether sym is data the plan placed, in engine_rodata or
// in the body of function fn.
func a64NamesData(env *ARM64Env, fn, sym string) bool {
	if _, ok := env.Plan.BlobOff[sym]; ok {
		return true
	}
	_, ok := env.Plan.Rename[fn][sym]
	return ok
}

func a64Add(k int64) string {
	if k == 0 {
		return ""
	}
	return fmt.Sprintf("%+d", k)
}

func a64RegNumber(r string) int {
	n, err := strconv.Atoi(r[1:])
	if err != nil {
		return -1
	}
	return n
}

// a64SameRegs holds the registers the lowering moves to llc's: want in the
// lowering's names (x20, d0), got as llc's facts name them (w20 for the low
// half, b0 to s0 for parts of d0).
func a64SameRegs(what string, want, got []string) error {
	norm := func(r string) string {
		switch r[0] {
		case 'w':
			return "x" + r[1:]
		case 'b', 'h', 's', 'q':
			return "d" + r[1:]
		}
		return r
	}
	var g []string
	seen := map[string]bool{}
	for _, r := range got {
		if r == "lr" || strings.HasSuffix(r, "_hi") || strings.HasPrefix(r, "w30") {
			continue
		}
		n := norm(r)
		if !seen[n] {
			seen[n] = true
			g = append(g, n)
		}
	}
	if strings.Join(g, ",") != strings.Join(want, ",") {
		return fmt.Errorf("%s registers %v, llc's are %v", what, want, got)
	}
	return nil
}

// EmitARM64Engine writes the Go assembly of the lowered engine from the
// object llvm-mc made of the lowered text: one TEXT per function, the body as
// WORDs, every call a BL or CALL (R9) Go sees, each address of data or of a
// function a MOVD $sym(SB) (which the Go assembler writes as the adrp and
// add it stands for), the arguments loaded from the Go argument area after
// Go's prologue and the result stored before its RET.
func EmitARM64Engine(objPath string, e EngineEmit) (map[string]string, error) {
	if e.NoStubs && len(e.Stubs) > 0 {
		return nil, fmt.Errorf("a release build has no stub, but %d functions would be", len(e.Stubs))
	}
	o, err := readA64Object(objPath)
	if err != nil {
		return nil, err
	}
	code, relocs, addr := o.code, o.relocs, o.addr
	header := fmt.Sprintf("// Code generated by internal/goasm/gen. DO NOT EDIT.\n//go:build %s\n\n#include \"textflag.h\"\n#include \"funcdata.h\"\n\n", BuildExpr(e.Tag, e.Arch))
	files := map[string]string{}
	parts := map[string]int{}
	cur := map[string]*strings.Builder{}
	open := func(group string, n int) *strings.Builder {
		b := cur[group]
		if b != nil && b.Len()+n > e.MaxBytes && b.Len() > len(header) {
			files[fmt.Sprintf("%s_%d_%s.s", group, parts[group], e.Arch)] = b.String()
			parts[group]++
			b = nil
		}
		if b == nil {
			b = &strings.Builder{}
			b.WriteString(header)
			cur[group] = b
		}
		return b
	}
	goSym := func(s string) (string, error) {
		if s == "__goasm_rodata" {
			return RodataGo, nil
		}
		t, ok := strings.CutPrefix(s, "go_")
		if !ok || e.Names[t] == "" {
			return "", fmt.Errorf("relocation to %s, which is no generated function", s)
		}
		return e.Names[t], nil
	}
	aligned := 0
	for _, m := range e.Metas {
		r, ok := addr[m.Name]
		if !ok {
			return nil, fmt.Errorf("%s: absent from the object", m.Name)
		}
		var fb strings.Builder
		argOff, retOff, _ := layout(m.Rec.Sig)
		gn := e.Names[m.Name]
		switch {
		case m.Kind == Register && m.Frame == 0:
			fmt.Fprintf(&fb, "TEXT ·%s(SB), NOSPLIT|NOFRAME, $0-0\n", gn)
		case m.Kind == Register:
			fmt.Fprintf(&fb, "TEXT ·%s(SB), NOSPLIT, $%d-0\n", gn, m.Frame)
		default:
			flags := ""
			if m.NoSplit {
				flags = "NOSPLIT, "
			}
			fmt.Fprintf(&fb, "TEXT ·%s(SB), %s$%d-%d\n\tNO_LOCAL_POINTERS\n", gn, flags, m.Frame, m.ArgBytes)
			regs, err := a64ArgRegs(m.Rec.Sig, true)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", m.Name, err)
			}
			k := 0
			for i, p := range m.Rec.Sig.Params {
				for w, fp := range wordTypes([]Type{p}) {
					mov := "MOVD"
					if fp {
						mov = "FMOVD"
					}
					// The names are those of the Go declaration (engine_decl): a
					// word each, numbered across the arguments.
					fmt.Fprintf(&fb, "\t%s a%d+%d(FP), %s\n", mov, k, argOff[i]+8*w, a64GoReg(regs[k]))
					k++
				}
			}
		}
		if off, ok := e.Align64[m.Name]; ok {
			if off < 0 || off >= 64 || off%4 != 0 {
				return nil, fmt.Errorf("%s: body at %d bytes into a line of 64, not a multiple of 4 below 64", m.Name, off)
			}
			aligned++
			fb.WriteString("\tPCALIGN $64\n")
			for ; off > 0; off -= 4 {
				fb.WriteString("\tWORD $0xd503201f // nop\n")
			}
		} else {
			fb.WriteString("\tPCALIGN $16\n")
		}
		dataAt := r[1]
		if c, ok := addr[m.Name+".const"]; ok {
			dataAt = c[0]
		}
		icalls := 0
		for _, c := range m.Rec.Calls {
			if c.Kind == CallReg {
				icalls++
			}
		}
		for pc := r[0]; pc < r[1]; pc += 4 {
			w := binary.LittleEndian.Uint32(code[pc:])
			if pc >= dataAt {
				if _, ok := relocs[pc]; ok {
					return nil, fmt.Errorf("%s: a relocation in its data at %#x", m.Name, pc)
				}
				fmt.Fprintf(&fb, "\tWORD $0x%08x\n", w)
				continue
			}
			if rl, ok := relocs[pc]; ok {
				switch rl.typ {
				case elf.R_AARCH64_CALL26:
					if rl.add != 0 {
						return nil, fmt.Errorf("%s: call to %s%+d", m.Name, rl.sym, rl.add)
					}
					t, err := goSym(rl.sym)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", m.Name, err)
					}
					fmt.Fprintf(&fb, "\tBL ·%s(SB)\n", t)
				case elf.R_AARCH64_ADR_PREL_PG_HI21:
					lo, ok := relocs[pc+4]
					w2 := binary.LittleEndian.Uint32(code[pc+4:])
					rd := w & 0x1f
					if !ok || lo.typ != elf.R_AARCH64_ADD_ABS_LO12_NC || lo.sym != rl.sym || lo.add != rl.add ||
						w2&0xffc003ff != 0x91000000|rd<<5|rd {
						return nil, fmt.Errorf("%s: an adrp at %#x without its add", m.Name, pc)
					}
					t, err := goSym(rl.sym)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", m.Name, err)
					}
					if rl.add != 0 {
						fmt.Fprintf(&fb, "\tMOVD $·%s+%d(SB), R%d\n", t, rl.add, rd)
					} else {
						fmt.Fprintf(&fb, "\tMOVD $·%s(SB), R%d\n", t, rd)
					}
					pc += 4
				default:
					return nil, fmt.Errorf("%s: relocation %v to %s at %#x", m.Name, rl.typ, rl.sym, pc)
				}
				continue
			}
			if w == 0xd63f0120 { // blr x9: an indirect call the lowering made
				if icalls == 0 {
					return nil, fmt.Errorf("%s: more blr x9 than indirect calls", m.Name)
				}
				icalls--
				fb.WriteString("\tCALL (R9)\n")
				continue
			}
			if w == 0xd65f03c0 || w&0xfffffc1f == 0xd63f0000 || w&0xfc000000 == 0x94000000 || w&0x9f000000 == 0x90000000 {
				return nil, fmt.Errorf("%s: ret, blr, bl or adrp hidden in the bytes at %#x", m.Name, pc)
			}
			if t, ok := a64BranchTarget(w, pc); ok && (t < r[0] || t > r[1]) {
				return nil, fmt.Errorf("%s: branch at %#x leaves the function", m.Name, pc)
			}
			if w&0x9f000000 == 0x10000000 { // adr: to the function's own code or data
				imm := int(int32(((w>>5)&0x7ffff)<<2|(w>>29)&3) << 11 >> 11)
				if t := pc + imm; t < r[0] || t >= r[1] {
					return nil, fmt.Errorf("%s: adr at %#x leaves the function", m.Name, pc)
				}
			}
			if why := a64ForbiddenWord(w); why != "" {
				return nil, fmt.Errorf("%s: forbidden instruction %#08x at %#x: %s", m.Name, w, pc, why)
			}
			fmt.Fprintf(&fb, "\tWORD $0x%08x\n", w)
		}
		if icalls != 0 {
			return nil, fmt.Errorf("%s: %d indirect calls not found in the bytes", m.Name, icalls)
		}
		if m.Kind != Register {
			for k, reg := range a64RetRegs(m.Rec.Sig.Ret) {
				mov := "MOVD"
				if reg[0] == 'd' {
					mov = "FMOVD"
				}
				name := "ret"
				if m.Rec.Sig.Ret == JSV {
					name = fmt.Sprintf("r%d", k)
				}
				fmt.Fprintf(&fb, "\t%s %s, %s+%d(FP)\n", mov, a64GoReg(reg), name, retOff+8*k)
			}
		}
		fb.WriteString("\tRET\n\n")
		group := e.Group[m.Name]
		if group == "" {
			group = "engine"
		}
		open(group, fb.Len()).WriteString(fb.String())
	}
	if aligned != len(e.Align64) {
		return nil, fmt.Errorf("%d functions to put on 64 bytes, but only %d of them are lowered", len(e.Align64), aligned)
	}
	for g, b := range cur {
		name := fmt.Sprintf("%s_%d_%s.s", g, parts[g], e.Arch)
		if parts[g] == 0 {
			name = fmt.Sprintf("%s_%s.s", g, e.Arch)
		}
		files[name] = b.String()
	}
	if len(e.Stubs) > 0 {
		var b strings.Builder
		b.WriteString(header)
		b.WriteString("// Functions the generator could not lower: each reports its index.\n\n")
		for i, m := range e.Stubs {
			fmt.Fprintf(&b, "TEXT ·%s(SB), $16-%d\n\tNO_LOCAL_POINTERS\n\tMOVD $%d, R0\n\tMOVD R0, 8(RSP)\n\tCALL ·goasmMissing(SB)\n\tRET\n\n", e.Names[m.Name], m.ArgBytes, i)
		}
		files["engine_stubs_"+e.Arch+".s"] = b.String()
	}
	if e.Plan != nil {
		files["engine_rodata_"+e.Arch+".s"] = EmitRodata(e.Plan, e.Names, header)
	}
	return files, nil
}

// arm64TextRefs lists the data and functions aarch64 code reaches. llc
// writes the engine in the tiny code model: an adr takes the address of data
// or of a function, and a load of a label reads data (which the plan copies
// into the function's body, as for a load through the PC on amd64). The
// page forms of the small code model (adrp, :lo12:) are refused.
func arm64TextRefs(u *AsmUnit, fail func(fn, f string, a ...any)) []textRef {
	var refs []textRef
	for _, fn := range u.FuncOrder {
		for _, raw := range u.Funcs[fn].Lines {
			l := a64Clean(raw)
			if reA64Mod.MatchString(l) {
				fail(fn, "unreviewed relocation form %q", l)
				continue
			}
			if strings.HasPrefix(l, "adrp ") || reA64Lo12.MatchString(l) {
				fail(fn, "page-relative reference %q: the engine is built with the tiny code model", l)
				continue
			}
			if m := reA64AdrSym.FindStringSubmatch(l); m != nil && !isTextLabel(m[2]) {
				refs = append(refs, textRef{fn: fn, sym: m[2], lea: true})
				continue
			}
			if m := reA64LdLit.FindStringSubmatch(l); m != nil {
				refs = append(refs, textRef{fn: fn, sym: m[3]})
				continue
			}
			if m := reA64LastOp.FindStringSubmatch(l); m != nil && u.Data[m[1]] != nil {
				fail(fn, "unreviewed reference to data %q", l)
			}
		}
	}
	return refs
}

var (
	reA64AddrSP = regexp.MustCompile(`^add (x\d+), sp, #(\d+)$`)
	reA64Tuple  = regexp.MustCompile(`^(st1|ld1) \{ ?((?:v\d+\.(?:2d|4s|8h|16b)(?:, )?)+) ?\}, \[(x\d+)\]$`)
)

// a64TupleSlot holds the stack address llc puts in reg (k bytes into a frame
// of which d bytes are allocated) to the one use it is made for: the spill
// or reload of a tuple of vector registers with st1 or ld1 through it, in the
// same block, before any call, with nothing else reading reg. A stack
// address that lives longer could see the goroutine's stack move.
func a64TupleSlot(rest []string, reg string, k, d int) error {
	r := a64RegNumber(reg)
	uses := 0
	for _, l := range rest {
		if strings.HasPrefix(l, "@") || isA64Label(l) || reA64BL.MatchString(l) || reA64BLR.MatchString(l) {
			break
		}
		if m := reA64Tuple.FindStringSubmatch(l); m != nil && m[3] == reg {
			n := strings.Count(m[2], "v")
			if k < 0 || k+16*n > d {
				return fmt.Errorf("a tuple of %d vectors at %d, outside the %d bytes of the frame", n, k, d)
			}
			uses++
			continue
		}
		reads, writes := a64Reads(l, r)
		if reads {
			return fmt.Errorf("the stack address is read by %q", l)
		}
		if writes {
			break
		}
	}
	if uses == 0 {
		return fmt.Errorf("a stack address with no tuple spill through it")
	}
	return nil
}

// a64AccessBytes is how many bytes a load or store of the stack reads or
// writes, or 0 for a form it does not know.
func a64AccessBytes(l string) int {
	op, ops := a64Operands(l)
	if len(ops) < 2 {
		return 0
	}
	switch op {
	case "strb", "ldrb", "ldrsb", "sturb", "ldurb", "ldursb":
		return 1
	case "strh", "ldrh", "ldrsh", "sturh", "ldurh", "ldursh":
		return 2
	case "ldrsw", "ldursw":
		return 4
	case "ldpsw":
		return 8
	}
	size := map[byte]int{'x': 8, 'w': 4, 'd': 8, 's': 4, 'h': 2, 'b': 1, 'q': 16}[ops[0][0]]
	switch op {
	case "str", "ldr", "stur", "ldur":
		return size
	case "stp", "ldp", "stnp", "ldnp":
		return 2 * size
	}
	return 0
}

// a64Object is what the emit and the post-link check read of llvm-mc's
// object of the lowered text: its code, where each function and its data
// (the symbol name.const) are, and the relocations of the code by offset.
type a64Object struct {
	code   []byte
	addr   map[string][2]int
	relocs map[int]a64Reloc
}

type a64Reloc struct {
	typ elf.R_AARCH64
	sym string
	add int64
}

func readA64Object(path string) (*a64Object, error) {
	ef, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer ef.Close()
	if ef.Machine != elf.EM_AARCH64 {
		return nil, fmt.Errorf("%s: not an aarch64 object", path)
	}
	for _, s := range ef.Sections {
		if s.Flags&elf.SHF_WRITE != 0 && s.Size > 0 {
			return nil, fmt.Errorf("writable section %s", s.Name)
		}
		if s.Flags&elf.SHF_ALLOC != 0 && s.Size > 0 && s.Name != ".text" {
			return nil, fmt.Errorf("unreviewed section %s", s.Name)
		}
	}
	text := ef.Section(".text")
	code, err := text.Data()
	if err != nil {
		return nil, err
	}
	syms, err := ef.Symbols()
	if err != nil {
		return nil, err
	}
	o := &a64Object{code: code, addr: map[string][2]int{}, relocs: map[int]a64Reloc{}}
	for _, s := range ef.Sections {
		if s.Type != elf.SHT_RELA || int(s.Info) >= len(ef.Sections) || ef.Sections[s.Info] != text {
			continue
		}
		d, err := s.Data()
		if err != nil {
			return nil, err
		}
		for p := 0; p+24 <= len(d); p += 24 {
			off := binary.LittleEndian.Uint64(d[p:])
			info := binary.LittleEndian.Uint64(d[p+8:])
			add := int64(binary.LittleEndian.Uint64(d[p+16:]))
			if info>>32 == 0 || int(info>>32) > len(syms) {
				return nil, fmt.Errorf("relocation without a symbol at %#x", off)
			}
			o.relocs[int(off)] = a64Reloc{elf.R_AARCH64(info & 0xffffffff), syms[int(info>>32)-1].Name, add}
		}
	}
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) == elf.STT_FUNC || strings.HasSuffix(s.Name, ".const") {
			o.addr[s.Name] = [2]int{int(s.Value), int(s.Value + s.Size)}
		}
	}
	return o, nil
}
