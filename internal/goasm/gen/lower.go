// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Lowering stage: turn the canonical LLVM GNU assembly of each engine function
// into a form Go's stack rules accept, as text, so re-assembly fixes every
// branch offset. Only external calls stay symbolic for elf2go.
//
// The transform per unified function:
//   - strip the canonical prologue (push BP; mov SP,BP; push <callee-saved>;
//     reserve locals) and the single epilogue, recording the frame and the
//     callee-saved set the body uses;
//   - prepend an entry shim that loads each incoming argument from the Go
//     argument area into its native argument register, after Go's prologue;
//   - at each internal call, save the live callee-saved registers to the
//     frame (the split callee's morestack does not preserve them), move the
//     native argument registers into the outgoing argument area, call, move
//     the result back, and restore;
//   - replace every return with a jump to a common epilogue that stores the
//     result into the Go result area and falls into Go's RET.
//
// It rejects any shape it was not built to prove: a non-canonical prologue, a
// body that still touches BP (an unhandled frame spill), an SP write, an
// escaping stack address, a forbidden instruction or an unreviewed indirect
// jump. Failing loudly is the plan's rule: the generator never emits code it
// cannot account for.
package gen

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// FuncSpec names a function in the compiled unit and how to generate it.
type FuncSpec struct {
	Name string
	Sig  Sig
	Kind Kind
	// Indirect lists, in source order, the signature of each call this
	// function makes through a function pointer. The function table and every
	// indirect call target a unified entry, so these marshal like a direct
	// unified call.
	Indirect []Sig
}

// Meta is what the emit stage needs after re-assembly.
type Meta struct {
	Name     string
	Kind     Kind
	GoName   string // exported-to-asm Go identifier, e.g. recAsm
	Frame    int    // Go TEXT frame size
	ArgBytes int    // Go TEXT argument+result area size
	RetType  Type
	// Rec is every rewrite the lowering made, for the instruction-level
	// post-link check (nil for the prototype's units).
	Rec *FuncRecord
}

// layout places params then the result in the Go argument area, each at its
// natural (<=8) alignment, and returns per-param offsets, the result offset
// and the total rounded to 8.
func layout(sig Sig) (argOff []int, retOff, total int) {
	off := 0
	for _, p := range sig.Params {
		a := p.bytes()
		if a > 8 {
			a = 8
		}
		off = align(off, a)
		argOff = append(argOff, off)
		off += p.bytes()
	}
	if sig.Ret != Void {
		a := sig.Ret.bytes()
		if a > 8 {
			a = 8
		}
		off = align(off, a)
		retOff = off
		off += sig.Ret.bytes()
	}
	return argOff, retOff, align(off, 8)
}

func align(x, a int) int {
	if a == 0 {
		return x
	}
	return (x + a - 1) / a * a
}

var (
	reLabel     = regexp.MustCompile(`^(\.?\w[\w.$]*):`)
	rePushCS    = regexp.MustCompile(`^pushq\s+%(rbx|r12|r13|r14|r15)$`)
	rePushLocal = regexp.MustCompile(`^pushq\s+%(rax|rcx)$`)
	reSub       = regexp.MustCompile(`^subq\s+\$(\d+), %rsp$`)
	reAdd       = regexp.MustCompile(`^addq\s+\$(\d+), %rsp$`)
	rePopAny    = regexp.MustCompile(`^popq\s+%(\w+)$`)
	// A direct call; @PLT is how clang spells a call to a symbol defined in
	// another unit. Any other call form is rejected, never passed through.
	reCall     = regexp.MustCompile(`^call(?:q)?\s+([.\w]+)(?:@PLT)?$`)
	reIndirect = regexp.MustCompile(`^callq?\s+\*%(\w+)$`)
	// A call through a pointer in memory. EngineCFlags set
	// +slow-two-mem-ops, so llc loads every call target into a register
	// first and never emits one: the lowering rejects it.
	reIndirectMem = regexp.MustCompile(`^callq?\s+\*[^%]`)
	reForbidden   = regexp.MustCompile(`\b(syscall|sysenter|int\s+\$0x80|rdtsc|rdrand|rdseed|cpuid)\b`)
)

func clean(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// sysvRegs returns the native registers that hold each logical argument of a
// signature in order, as GNU 64-bit register names (or xmmN for doubles).
func (a AMD64Arch) sysvRegs(sig Sig) ([]string, error) {
	var out []string
	ints := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
	if a.PreserveNone {
		ints = []string{"r12", "r13", "r14", "r15", "rdi", "rsi", "rdx", "rcx", "r8", "r9", "r11", "rax"}
	}
	sses := a.SSEArgRegs()
	ni, nx := 0, 0
	for _, p := range sig.Params {
		switch p {
		case I64, Ptr:
			if ni >= len(ints) {
				return nil, fmt.Errorf("too many integer args")
			}
			out = append(out, ints[ni])
			ni++
		case F64:
			if nx >= len(sses) {
				return nil, fmt.Errorf("too many fp args")
			}
			out = append(out, sses[nx])
			nx++
		case JSV:
			if ni+1 >= len(ints) {
				return nil, fmt.Errorf("too many integer args for JSValue")
			}
			out = append(out, ints[ni], ints[ni+1])
			ni += 2
		default:
			return nil, fmt.Errorf("bad param type")
		}
	}
	return out, nil
}

func movFor(reg string) string {
	if strings.HasPrefix(reg, "xmm") {
		return "movsd"
	}
	return "movq"
}

// goReg converts a GNU register name to the Go assembler spelling.
func goReg(reg string) string {
	switch {
	case strings.HasPrefix(reg, "xmm"):
		return "X" + reg[3:]
	case reg == "rax":
		return "AX"
	case reg == "rbx":
		return "BX"
	case reg == "rcx":
		return "CX"
	case reg == "rdx":
		return "DX"
	case reg == "rsi":
		return "SI"
	case reg == "rdi":
		return "DI"
	default: // r8..r15
		return strings.ToUpper(reg)
	}
}

// LowerEnv is what lowering a function of the whole engine needs beyond its
// own text: where its data goes, which symbols are functions, and the
// signatures of indirect calls, which the shadow-stack pass writes before
// each one as a "# goasm.icall <code>" comment.
type LowerEnv struct {
	Plan  *DataPlan
	Funcs map[string]bool
	// Variadic names the variadic functions. A call to one is rejected
	// unless LooseVarargs, which lowers it with its fixed arguments only
	// (wrong code, for measuring the output while the C still has them).
	Variadic     map[string]bool
	LooseVarargs bool
}

// LowerUnified transforms one function's GNU assembly. It returns the rewritten
// GNU text (one label, a trailing return label, calls left symbolic) and the
// Meta for emit.
func LowerUnified(a AMD64Arch, asm string, fn FuncSpec) (string, Meta, error) {
	lines := strings.Split(asm, "\n")
	start := -1
	for i, l := range lines {
		if clean(l) == fn.Name+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return "", Meta{}, fmt.Errorf("%s: not found", fn.Name)
	}
	end := -1
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), ".Lfunc_end") {
			end = i
			break
		}
	}
	if end < 0 {
		return "", Meta{}, fmt.Errorf("%s: no .Lfunc_end", fn.Name)
	}
	return LowerUnifiedFunc(a, lines[start+1:end], fn, nil, asm)
}

// reMarker is the comment the shadow-stack pass puts before an indirect call.
var reMarker = regexp.MustCompile(`#\s*goasm\.icall\s+(\S+)`)

// LowerUnifiedFunc lowers the body lines of one function (after its label,
// up to .Lfunc_end). With env nil it is the prototype's unit: constant pools
// are found in asm and indirect calls take fn.Indirect in order.
func LowerUnifiedFunc(a AMD64Arch, lines []string, fn FuncSpec, env *LowerEnv, asm string) (string, Meta, error) {
	// Gather the instruction/label lines of the body.
	var body []string
	for _, raw := range lines {
		if m := reMarker.FindStringSubmatch(raw); m != nil {
			body = append(body, "@icall "+m[1])
			continue
		}
		l := clean(raw)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, ".cfi_") || strings.HasPrefix(l, ".p2align") ||
			strings.HasPrefix(l, ".size") || strings.HasPrefix(l, ".type") ||
			strings.HasPrefix(l, ".globl") || strings.HasPrefix(l, ".section") ||
			strings.HasPrefix(l, ".addrsig") || strings.HasPrefix(l, ".ident") ||
			strings.HasPrefix(l, ".text") {
			continue
		}
		body = append(body, l)
	}

	// Parse the canonical prologue.
	i := 0
	if len(body) < 2 {
		return "", Meta{}, fmt.Errorf("%s: body too short for the canonical prologue", fn.Name)
	}
	if norm(body[i]) != "pushq %rbp" {
		return "", Meta{}, fmt.Errorf("%s: prologue does not start with push rbp: %q", fn.Name, body[i])
	}
	i++
	if norm(body[i]) != "movq %rsp, %rbp" {
		return "", Meta{}, fmt.Errorf("%s: prologue missing mov rsp,rbp: %q", fn.Name, body[i])
	}
	i++
	var pushed []string
	for i < len(body) {
		if m := rePushCS.FindStringSubmatch(norm(body[i])); m != nil {
			pushed = append(pushed, m[1])
			i++
			continue
		}
		break
	}
	if a.PreserveNone && len(pushed) > 0 {
		return "", Meta{}, fmt.Errorf("%s: preserve_none code saved callee-saved registers %v", fn.Name, pushed)
	}
	// The registers a unified call must keep: the ones the body uses, which
	// are the ones LLVM's prologue pushed.
	saved := pushed
	local := 0
	for i < len(body) {
		n := norm(body[i])
		if m := rePushLocal.FindStringSubmatch(n); m != nil {
			_ = m
			local += 8
			i++
			continue
		}
		if m := reSub.FindStringSubmatch(n); m != nil {
			v, _ := strconv.Atoi(m[1])
			local += v
			i++
			continue
		}
		break
	}
	prologueEnd := i

	// The rest is the body plus one or more epilogues. Each epilogue is a run
	// of frame restores (add $N,%rsp or pops of the callee-saved set and rbp)
	// ending in ret. The restores are dropped and each ret becomes a result
	// store and a jump to the single Go RET, so multiple return blocks are
	// handled uniformly. A pop of any other register in the body is rejected
	// by the SP-write check below.
	inner := body[prologueEnd:]

	sysv, err := a.sysvRegs(fn.Sig)
	if err != nil {
		return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
	}
	_, retOff, argBytes := layout(fn.Sig)

	// Frame: outgoing argument area (max over calls), saved callee-saved slots,
	// then the body's local reservation. Rounded to 16.
	outArea := 0
	hasCall := false
	consider := func(s Sig) {
		if _, _, ab := layout(s); ab > outArea {
			outArea = ab
		}
	}
	for _, l := range inner {
		n := norm(l)
		if m := reCall.FindStringSubmatch(n); m != nil {
			hasCall = true
			if cs, ok := callSig(m[1], fn, calleeSigs); ok && calleeKinds[m[1]] != Register {
				consider(cs)
			}
		} else if reIndirect.MatchString(n) {
			hasCall = true
		} else if env != nil && strings.HasPrefix(n, "@icall ") {
			// The outgoing area must hold the arguments of every
			// indirect call too; a bad code fails at the call below.
			if s, err := SigFromCode(strings.TrimPrefix(n, "@icall ")); err == nil {
				consider(s)
			}
		}
	}
	for _, s := range fn.Indirect {
		consider(s)
	}
	if a.SaveAll && hasCall {
		saved = a.CalleeSaved()
	}
	if os.Getenv("GEN_NO_SAVES") != "" {
		saved = nil // negative build: must fail the behavior tests
	}
	saveBase := align(outArea, 8)
	saveArea := 8 * len(saved)
	localBase := align(saveBase+saveArea, 16)
	frame := align(localBase+local, 16)
	// Go pushes the base pointer only when the frame is non-zero or the
	// function calls: a frameless, callless unified entry is a NOSPLIT leaf
	// whose incoming arguments sit just above the return address.
	bp := 0
	if frame > 0 || hasCall {
		bp = 8
	}
	argBase := frame + 8 + bp
	rec := &FuncRecord{Name: fn.Name, Kind: Unified, Sig: fn.Sig, PreserveNone: a.PreserveNone, SaveAll: a.SaveAll,
		Pushed: pushed, Local: local, LocalBase: localBase, SaveBase: saveBase, Saved: saved, Frame: frame}

	// Build the rewritten body.
	var out []string
	argRegOffsets, _, _ := layout(fn.Sig)
	// Entry shim: load each argument from the Go argument area into its reg.
	for k, p := range fn.Sig.Params {
		switch p {
		case JSV:
			out = append(out, fmt.Sprintf("movq %d(%%rsp), %%%s", argBase+argRegOffsets[k], sysvForJSV(sysv, fn.Sig, k, 0)))
			out = append(out, fmt.Sprintf("movq %d(%%rsp), %%%s", argBase+argRegOffsets[k]+8, sysvForJSV(sysv, fn.Sig, k, 1)))
		default:
			reg := sysvForParam(sysv, fn.Sig, k)
			out = append(out, fmt.Sprintf("%s %d(%%rsp), %%%s", movFor(reg), argBase+argRegOffsets[k], reg))
		}
	}

	retLabel := ".Lret_" + fn.Name
	storeResult := func() []string {
		switch fn.Sig.Ret {
		case I64, Ptr:
			return []string{fmt.Sprintf("movq %%rax, %d(%%rsp)", argBase+retOff)}
		case F64:
			return []string{fmt.Sprintf("movsd %%xmm0, %d(%%rsp)", argBase+retOff)}
		case JSV:
			return []string{fmt.Sprintf("movq %%rax, %d(%%rsp)", argBase+retOff),
				fmt.Sprintf("movq %%rdx, %d(%%rsp)", argBase+retOff+8)}
		}
		return nil
	}
	indIdx := 0
	pending := "" // signature code of the next indirect call
	var flow map[string]string
	if env != nil {
		var err error
		if flow, err = markerFlow(inner, env.jumpTables(fn.Name)); err != nil {
			return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
		}
	}
	for li, l := range inner {
		n := norm(l)
		if strings.HasPrefix(n, "@icall ") {
			if pending != "" {
				return "", Meta{}, fmt.Errorf("%s: two indirect-call markers without a call between", fn.Name)
			}
			pending = strings.TrimPrefix(n, "@icall ")
			continue
		}
		if m := reLabel.FindStringSubmatch(n); m != nil {
			if env == nil && pending != "" {
				return "", Meta{}, fmt.Errorf("%s: indirect-call marker and its call in different blocks", fn.Name)
			}
			// The marker reaching this block along every edge into it.
			pending = flow[m[1]]
		}
		// indirectSig is the signature of an indirect call: from the
		// marker in the whole engine, in declared order in the prototype.
		indirectSig := func() (Sig, error) {
			if env != nil {
				if pending == "" {
					return Sig{}, fmt.Errorf("%s: indirect call without a marker: %q", fn.Name, l)
				}
				s, err := SigFromCode(pending)
				pending = ""
				if err != nil {
					return Sig{}, fmt.Errorf("%s: indirect call: %w", fn.Name, err)
				}
				return s, nil
			}
			if indIdx >= len(fn.Indirect) {
				return Sig{}, fmt.Errorf("%s: more indirect calls than declared", fn.Name)
			}
			indIdx++
			return fn.Indirect[indIdx-1], nil
		}
		// Drop epilogue frame restores (one per return block).
		if reAdd.MatchString(n) {
			continue
		}
		if m := rePopAny.FindStringSubmatch(n); m != nil && (m[1] == "rbp" || isCalleeSaved(m[1])) {
			continue
		}
		if reForbidden.MatchString(n) {
			return "", Meta{}, fmt.Errorf("%s: forbidden instruction: %q", fn.Name, l)
		}
		// Checked before the frame rewrite below, which would otherwise
		// take a call through a frame slot for a plain frame access and
		// keep it unmarshalled.
		if reIndirectMem.MatchString(n) {
			return "", Meta{}, fmt.Errorf("%s: call through memory (the build sets +slow-two-mem-ops so llc never emits one): %q", fn.Name, l)
		}
		// Rewrite frame spills: LLVM addresses locals at -N(%rbp), below the
		// saved callee-saved area. Map each onto the local region of the Go
		// frame, preserving contiguity, so no base-pointer access remains.
		if strings.Contains(n, "%rbp") {
			rewritten, err := rewriteRBPLocals(l, 8*len(pushed), local, localBase)
			if err != nil {
				return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
			}
			fixed, err := unalignedStackAccess(norm(rewritten))
			if err != nil {
				return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
			}
			out = append(out, fixed)
			continue
		}
		if reSub.MatchString(n) || strings.HasPrefix(n, "pushq") || strings.HasPrefix(n, "popq") || strings.HasPrefix(n, "leave") {
			// SysV passes the arguments past the sixth on the stack. The
			// pass gives every other signature an argument block, so a push
			// left in the body feeds a variadic call.
			if strings.HasPrefix(n, "pushq") && env != nil {
				if c := nextCall(inner[li:]); env.Variadic[c] {
					return "", Meta{}, fmt.Errorf("%s: stack arguments of the variadic call to %s: %q", fn.Name, c, l)
				}
			}
			return "", Meta{}, fmt.Errorf("%s: SP write in body: %q", fn.Name, l)
		}
		if strings.HasPrefix(n, "lea") && strings.Contains(n, "%rsp") {
			return "", Meta{}, fmt.Errorf("%s: escaping stack address: %q", fn.Name, l)
		}
		if strings.HasPrefix(n, "jmp") && strings.Contains(n, "*") {
			if !regexp.MustCompile(`^jmpq?\s+\*%r\w+$`).MatchString(n) {
				return "", Meta{}, fmt.Errorf("%s: unreviewed indirect jump: %q", fn.Name, l)
			}
		}
		if n == "retq" || n == "ret" {
			out = append(out, storeResult()...)
			out = append(out, "jmp "+retLabel)
			continue
		}
		if m := reIndirect.FindStringSubmatch(n); m != nil {
			sig, err := indirectSig()
			if err != nil {
				return "", Meta{}, err
			}
			out = append(out, marshalCallTarget(a, "*%"+m[1], sig, saved, saveBase)...)
			rec.Calls = append(rec.Calls, CallRecord{Kind: CallReg, Target: m[1], Sig: sig})
			continue
		}
		if m := reCall.FindStringSubmatch(n); m != nil && pending != "" {
			return "", Meta{}, fmt.Errorf("%s: indirect-call marker before a direct call: %q", fn.Name, l)
		}
		if m := reCall.FindStringSubmatch(n); m != nil && env != nil && env.Variadic[m[1]] && !env.LooseVarargs {
			return "", Meta{}, fmt.Errorf("%s: variadic call to %s", fn.Name, m[1])
		}
		if m := reCall.FindStringSubmatch(n); m != nil {
			cs, ok := callSig(m[1], fn, calleeSigs)
			if !ok {
				return "", Meta{}, fmt.Errorf("%s: call to unknown target %q", fn.Name, m[1])
			}
			if calleeKinds[m[1]] == Register {
				// Direct call to a known leaf: arguments are already in the
				// native registers and the leaf preserves callee-saved, so no
				// marshalling is needed.
				out = append(out, "call "+m[1])
				rec.Calls = append(rec.Calls, CallRecord{Kind: CallLeaf, Target: m[1]})
				continue
			}
			out = append(out, marshalCallTarget(a, m[1], cs, saved, saveBase)...)
			rec.Calls = append(rec.Calls, CallRecord{Kind: CallUnified, Target: m[1], Sig: cs})
			continue
		}
		if strings.HasPrefix(n, "call") {
			return "", Meta{}, fmt.Errorf("%s: unrecognized call form: %q", fn.Name, l)
		}
		if isTailJump(n) {
			return "", Meta{}, fmt.Errorf("%s: tail call: %q", fn.Name, l)
		}
		if env != nil {
			rewritten, err := env.dataRefs(fn.Name, l, &rec.Refs)
			if err != nil {
				return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
			}
			l, n = rewritten, norm(rewritten)
		} else if m := reSymRef.FindStringSubmatch(n); m != nil && !strings.HasPrefix(m[1], ".LCPI") {
			return "", Meta{}, fmt.Errorf("%s: unsupported read-only reference %q: %q", fn.Name, m[1], l)
		}
		// LLVM addresses locals from SP too: shift them onto the Go frame's
		// local region, and check the alignment each stack access assumes.
		if strings.Contains(n, "%rsp") {
			rewritten, err := rewriteRSPLocals(l, local, localBase)
			if err != nil {
				return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
			}
			l, n = rewritten, norm(rewritten)
		}
		if strings.Contains(n, "(%rsp)") {
			fixed, err := unalignedStackAccess(n)
			if err != nil {
				return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
			}
			l = fixed
		}
		out = append(out, l)
	}
	// The result is stored at each return above; this label just falls into
	// Go's RET (appended by emit). A function with constants keeps them in its
	// own body after the code, where the assembler resolves every
	// RIP-relative reference, and jumps over them to the RET.
	if pending != "" {
		return "", Meta{}, fmt.Errorf("%s: indirect-call marker without a call", fn.Name)
	}
	out = append(out, retLabel+":")
	var pool string
	if env != nil {
		pool = env.Plan.BodyData(fn.Name)
	} else if p, err := constantPool(asm, inner); err != nil {
		return "", Meta{}, fmt.Errorf("%s: %w", fn.Name, err)
	} else {
		pool = p
	}
	if pool != "" {
		end := ".Lend_" + fn.Name
		out = append(out, "jmp "+end, ".p2align 4", fn.Name+".const:", pool, end+":")
	}

	text := fn.Name + ":\n\t" + strings.Join(out, "\n\t") + "\n"
	meta := Meta{Name: fn.Name, Kind: Unified, GoName: goIdent(fn.Name), Frame: frame, ArgBytes: argBytes, RetType: fn.Sig.Ret, Rec: rec}
	return text, meta, nil
}

// marshalCall emits the glue around an internal call: save the live
// callee-saved registers, move the native argument registers into the outgoing
// argument area, call, move the result back into the native result register,
// and restore.
func marshalCallTarget(a AMD64Arch, callExpr string, sig Sig, saved []string, saveBase int) []string {
	var g []string
	for k, r := range saved {
		g = append(g, fmt.Sprintf("movq %%%s, %d(%%rsp)", r, saveBase+8*k))
	}
	sysv, _ := a.sysvRegs(sig)
	argOff, retOff, _ := layout(sig)
	for k := range sig.Params {
		switch sig.Params[k] {
		case JSV:
			g = append(g, fmt.Sprintf("movq %%%s, %d(%%rsp)", sysvForJSV(sysv, sig, k, 0), argOff[k]))
			g = append(g, fmt.Sprintf("movq %%%s, %d(%%rsp)", sysvForJSV(sysv, sig, k, 1), argOff[k]+8))
		default:
			reg := sysvForParam(sysv, sig, k)
			g = append(g, fmt.Sprintf("%s %%%s, %d(%%rsp)", movFor(reg), reg, argOff[k]))
		}
	}
	g = append(g, "call "+callExpr)
	switch sig.Ret {
	case I64, Ptr:
		g = append(g, fmt.Sprintf("movq %d(%%rsp), %%rax", retOff))
	case F64:
		g = append(g, fmt.Sprintf("movsd %d(%%rsp), %%xmm0", retOff))
	case JSV:
		g = append(g, fmt.Sprintf("movq %d(%%rsp), %%rax", retOff))
		g = append(g, fmt.Sprintf("movq %d(%%rsp), %%rdx", retOff+8))
	}
	for k, r := range saved {
		g = append(g, fmt.Sprintf("movq %d(%%rsp), %%%s", saveBase+8*k, r))
	}
	return g
}

// sysvForParam returns the single native register for logical param k.
func sysvForParam(sysv []string, sig Sig, k int) string {
	idx := 0
	for i := 0; i < k; i++ {
		idx += regWords(sig.Params[i])
	}
	return sysv[idx]
}

func sysvForJSV(sysv []string, sig Sig, k, half int) string {
	idx := 0
	for i := 0; i < k; i++ {
		idx += regWords(sig.Params[i])
	}
	return sysv[idx+half]
}

func regWords(t Type) int {
	if t == JSV {
		return 2
	}
	return 1
}

func norm(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\t", " ") }

func isCalleeSaved(r string) bool {
	switch r {
	case "rbx", "r12", "r13", "r14", "r15":
		return true
	}
	return false
}

// calleeSigs is set by the driver before lowering.
var calleeSigs map[string]Sig

func callSig(target string, self FuncSpec, sigs map[string]Sig) (Sig, bool) {
	if target == self.Name {
		return self.Sig, true
	}
	s, ok := sigs[target]
	return s, ok
}

func goIdent(cname string) string {
	// rec -> recAsm style is handled by the driver's name map; default identity.
	return cname
}

// SetCallees records the signature and kind of every function the unit may
// call, for the lowering of internal calls.
func SetCallees(specs []FuncSpec) {
	calleeSigs = map[string]Sig{}
	calleeKinds = map[string]Kind{}
	for _, s := range specs {
		calleeSigs[s.Name] = s.Sig
		calleeKinds[s.Name] = s.Kind
	}
}

var calleeKinds map[string]Kind

var reRBP = regexp.MustCompile(`(-?\d+)\(%rbp\)`)

// rewriteRBPLocals maps every -N(%rbp) local access onto the Go frame's local
// region [localBase, localBase+local). savedBytes is the size of the saved
// callee-saved area that sits between rbp and the locals in the LLVM frame.
// The map Go = localBase + (savedBytes+local) - N is contiguity preserving.
func rewriteRBPLocals(line string, savedBytes, local, localBase int) (string, error) {
	var rerr error
	out := reRBP.ReplaceAllStringFunc(line, func(m string) string {
		sub := reRBP.FindStringSubmatch(m)
		n, _ := strconv.Atoi(sub[1])
		if n >= 0 {
			rerr = fmt.Errorf("non-negative base-pointer access %q (not a local)", m)
			return m
		}
		abs := -n
		if abs <= savedBytes || abs > savedBytes+local {
			rerr = fmt.Errorf("base-pointer access %q outside the local area", m)
			return m
		}
		return fmt.Sprintf("%d(%%rsp)", localBase+(savedBytes+local)-abs)
	})
	if rerr != nil {
		return "", rerr
	}
	if strings.Contains(out, "%rbp") {
		return "", fmt.Errorf("unhandled base-pointer use: %q", line)
	}
	return out, nil
}

// AddCallee registers one external callee (for example a host callback) that
// is referenced but not generated here.
func AddCallee(name string, sig Sig, kind Kind) {
	calleeSigs[name] = sig
	calleeKinds[name] = kind
}

// markerFlow gives the indirect-call marker pending at the start of each
// local label. llc may move the call of a marked site into a block shared by
// several predecessors (tail merging), each keeping its own copy of the
// marker, so a marker is followed along jumps and fall-throughs to its call.
// Every edge into a block must carry the same marker, or none.
// markerFlow returns the indirect-call marker pending at the start of each
// local label. tables maps each in-body jump-table label to the blocks it
// lands on; indirectJumpTargets uses it to decide, per indirect jump, which
// blocks that jump may reach, and whether any jump's targets are unknown.
func markerFlow(lines []string, tables map[string]map[string]bool) (map[string]string, error) {
	in := map[string]string{} // present: some edge reaches the label
	// A label no direct jump names is entered by falling through or from a
	// jump table. No marker may be pending at an indirect jump (checked
	// below), so a table entry carries none.
	targets := map[string]bool{}
	for _, l := range lines {
		f := strings.Fields(norm(l))
		if len(f) == 2 && f[0][0] == 'j' && strings.HasPrefix(f[1], ".L") {
			targets[f[1]] = true
		}
	}
	reachable, unknown := indirectJumpTargets(lines, tables)
	for iter := 0; ; iter++ {
		if iter > len(lines)+1 {
			return nil, fmt.Errorf("indirect-call markers do not settle")
		}
		changed := false
		var conflict error
		join := func(lab, st string) {
			prev, ok := in[lab]
			if !ok {
				in[lab], changed = st, true
			} else if prev != st && conflict == nil {
				conflict = fmt.Errorf("blocks reaching %s disagree on the indirect-call marker (%q, %q)", lab, prev, st)
			}
		}
		st, live := "", true
		for _, l := range lines {
			n := norm(l)
			if strings.HasPrefix(n, "@icall ") {
				st = strings.TrimPrefix(n, "@icall ")
				continue
			}
			if m := reLabel.FindStringSubmatch(n); m != nil {
				if live {
					join(m[1], st)
				}
				if v, ok := in[m[1]]; ok {
					st, live = v, true
				} else if !targets[m[1]] {
					st, live = "", true
				} else {
					live = false
				}
				continue
			}
			if !live {
				continue
			}
			f := strings.Fields(n)
			switch {
			case strings.HasPrefix(n, "call"):
				st = ""
			case len(f) == 2 && (f[0] == "jmp" || f[0] == "jmpq") && strings.HasPrefix(f[1], ".L"):
				join(f[1], st)
				live = false
			case len(f) == 2 && n[0] == 'j' && strings.HasPrefix(f[1], ".L"):
				join(f[1], st)
			case n == "retq" || n == "ret" || f[0] == "jmp" || f[0] == "jmpq" || f[0] == "ud2":
				if st != "" {
					return nil, fmt.Errorf("indirect-call marker without a call")
				}
				live = false
			}
		}
		if conflict != nil {
			return nil, conflict
		}
		if !changed {
			// A marker pending at the start of a block reaches its call
			// across a block edge. If an indirect jump could land on that
			// block, the block is reached along an edge that carries no
			// marker, so the marker cannot be trusted. Reject when the block
			// is a target of some resolved jump, or when any jump's targets
			// are unknown (so it could reach any block).
			for _, lab := range SortedKeys(in) {
				if in[lab] != "" && (unknown || reachable[lab]) {
					return nil, fmt.Errorf("indirect-call marker %q reaches %s across a block edge while an indirect jump may reach %s unmarked", in[lab], lab, lab)
				}
			}
			return in, nil
		}
	}
}

// nextCall returns the target of the first direct call in lines.
func nextCall(lines []string) string {
	for _, l := range lines {
		if m := reCall.FindStringSubmatch(norm(l)); m != nil {
			return m[1]
		}
	}
	return ""
}

// isTailJump reports a jump to a symbol rather than to a local label: LLVM's
// sibling-call form, a musttail or a wrapper jump. All are forbidden.
func isTailJump(n string) bool {
	m := regexp.MustCompile(`^j[a-z]+q?\s+([^*%\s]\S*)$`).FindStringSubmatch(n)
	return m != nil && !strings.HasPrefix(m[1], ".L")
}

// reSymRef finds a memory operand addressed from a symbol (sym(%rip),
// sym(,%reg,8)). Only constant-pool entries are supported so far.
var reSymRef = regexp.MustCompile(`(?:^|[\s,*$])([._A-Za-z][\w.$]*)(?:[+-]\d+)?\(`)

var reRSP = regexp.MustCompile(`(-?\d*)\(%rsp\)`)

// rewriteRSPLocals shifts every N(%rsp) access of the LLVM body by localBase:
// LLVM's SP sat at the bottom of its local reservation, which the Go frame
// places at localBase. Only [0, local) is a local; anything else is rejected.
func rewriteRSPLocals(line string, local, localBase int) (string, error) {
	var rerr error
	out := reRSP.ReplaceAllStringFunc(line, func(m string) string {
		sub := reRSP.FindStringSubmatch(m)
		n := 0
		if sub[1] != "" {
			n, _ = strconv.Atoi(sub[1])
		}
		if n < 0 || n >= local {
			rerr = fmt.Errorf("stack access %q outside the local area", m)
			return m
		}
		return fmt.Sprintf("%d(%%rsp)", localBase+n)
	})
	if rerr != nil {
		return "", rerr
	}
	if strings.Count(out, "%rsp") != strings.Count(out, "(%rsp)") {
		return "", fmt.Errorf("unhandled SP use: %q", line)
	}
	return out, nil
}

// The Go stack is only 8-byte aligned. Aligned 16-byte moves to the stack are
// turned into their unaligned forms, which behave the same. Scalar and 8-byte
// SSE accesses have no alignment need. Any other SSE instruction with a stack
// operand (a folded packed load) would fault on a misaligned SP: rejected.
var (
	alignedMov = map[string]string{"movaps": "movups", "movapd": "movupd", "movdqa": "movdqu"}
	noAlignSSE = regexp.MustCompile(`^(movups|movupd|movdqu|movsd|movss|movq|movd|movlps|movhps|movlpd|movhpd|cvt\w+|\w+s[sd]|ucomis[sd]|comis[sd])$`)
)

func unalignedStackAccess(n string) (string, error) {
	if !strings.Contains(n, "%xmm") {
		return n, nil
	}
	f := strings.Fields(n)
	if u, ok := alignedMov[f[0]]; ok {
		return u + n[len(f[0]):], nil
	}
	if !noAlignSSE.MatchString(f[0]) {
		return "", fmt.Errorf("SSE stack access may need 16-byte alignment: %q", n)
	}
	return n, nil
}

var (
	reCPIRef = regexp.MustCompile(`\.LCPI\d+_\d+`)
	reCPIDef = regexp.MustCompile(`^(\.LCPI\d+_\d+):$`)
	reData   = regexp.MustCompile(`^\.(quad|long|short|byte|zero)\s`)
	reAlign  = regexp.MustCompile(`^\.p2align\s+(\d+)`)
)

// constantPool returns the definitions of the constant-pool entries the body
// references (LLVM's floating-point and vector literals), each with its
// alignment, as assembly to place inside the function. Any other read-only
// reference is left for the assembler to reject.
func constantPool(asm string, body []string) (string, error) {
	want := map[string]bool{}
	for _, l := range body {
		for _, m := range reCPIRef.FindAllString(l, -1) {
			want[m] = true
		}
	}
	if len(want) == 0 {
		return "", nil
	}
	var b strings.Builder
	lines := strings.Split(asm, "\n")
	align := "0"
	for i := 0; i < len(lines); i++ {
		l := clean(lines[i])
		if m := reAlign.FindStringSubmatch(l); m != nil {
			align = m[1]
			continue
		}
		m := reCPIDef.FindStringSubmatch(l)
		if m == nil || !want[m[1]] {
			continue
		}
		delete(want, m[1])
		fmt.Fprintf(&b, ".p2align %s\n%s:\n", align, m[1])
		for i+1 < len(lines) && reData.MatchString(clean(lines[i+1])) {
			i++
			b.WriteString(clean(lines[i]) + "\n")
		}
	}
	if len(want) != 0 {
		return "", fmt.Errorf("constant pool entries not found: %v", want)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// dataRefs rewrites the RIP-relative data references of one body line of fn:
// a LEA of an object of engine_rodata becomes a LEA of
// __goasm_rodata+offset, any other read of data goes to the function's own
// copy, a constant pool or jump table keeps its label, and a LEA of a
// function stays symbolic for emit.
// reBlockLabel matches a local block label (.LBB.., .Ltmp..) used as a jump
// target inside a data object, such as a switch table or a computed-goto
// table.
var reBlockLabel = regexp.MustCompile(`\.L(?:BB|tmp)[\w.$]*`)

// jumpTables maps each in-body jump-table label of fn to the block labels it
// lands on. llc keeps a switch or computed-goto table inside the function that
// reads it, so a table is one of fn's own in-body data objects that holds
// block labels. An object with no block labels (a constant pool) is not a
// table.
func (env *LowerEnv) jumpTables(fn string) map[string]map[string]bool {
	if env.Plan == nil {
		return nil
	}
	m := map[string]map[string]bool{}
	for _, o := range env.Plan.InBody[fn] {
		targets := map[string]bool{}
		for _, l := range o.Lines {
			for _, b := range reBlockLabel.FindAllString(l, -1) {
				targets[b] = true
			}
		}
		if len(targets) == 0 {
			continue
		}
		for _, lab := range append([]string{o.Label}, o.Aliases...) {
			m[lab] = targets
		}
	}
	return m
}

// indirectJumpTargets decides, per indirect jump (jmp/jmpq through a register
// or memory), which blocks it may land on. A table vouches only for the jump
// that provably loads its target from it: tableDispatch follows the jump's
// register back through llc's PIC dispatch sequence to the LEA of the table.
// Any other jump has an unknown target set: it could reach any block, so
// unknown is set and every cross-block marker must then be rejected. tables
// maps each in-body jump-table label to its target blocks.
func indirectJumpTargets(lines []string, tables map[string]map[string]bool) (reachable map[string]bool, unknown bool) {
	reachable = map[string]bool{}
	blockStart := 0
	for i, l := range lines {
		n := norm(l)
		if reLabel.MatchString(n) {
			blockStart = i + 1
			continue
		}
		f := strings.Fields(n)
		if len(f) == 0 || (f[0] != "jmp" && f[0] != "jmpq") || !strings.Contains(n, "*") {
			continue
		}
		t, ok := tableDispatch(lines[blockStart:i+1], tables)
		if !ok {
			unknown = true
			continue
		}
		for b := range tables[t] {
			reachable[b] = true
		}
	}
	return reachable, unknown
}

var (
	reJmpReg    = regexp.MustCompile(`^jmpq?\s+\*%(r\w+)$`)
	reTableAdd  = regexp.MustCompile(`^addq\s+%(r\w+), %(r\w+)$`)
	reTableLoad = regexp.MustCompile(`^movslq\s+\(%(r\w+),%(r\w+),4\), %(r\w+)$`)
	reTableLEA  = regexp.MustCompile(`^leaq\s+([.\w$]+)\(%rip\), %(r\w+)$`)
)

// tableDispatch matches the end of block (its last line is the indirect jump)
// against llc's PIC dispatch, following the jump's register:
//
//	leaq  T(%rip), %rB
//	movslq (%rB,%rI,4), %rX
//	addq  %rB, %rX
//	jmpq  *%rX
//
// Lines between them may be anything that names neither %rB nor %rX (in any
// width) and writes no register implicitly. It returns T when T is a known
// table.
func tableDispatch(block []string, tables map[string]map[string]bool) (string, bool) {
	k := len(block) - 1
	m := reJmpReg.FindStringSubmatch(norm(block[k]))
	if m == nil {
		return "", false
	}
	rx := m[1]
	// prev finds the nearest earlier line that names any of regs; every line
	// skipped over must not write a register implicitly.
	prev := func(from int, regs ...string) int {
		for j := from - 1; j >= 0; j-- {
			n := norm(block[j])
			if strings.HasPrefix(n, "@icall ") {
				continue
			}
			for _, r := range regs {
				if namesReg(n, r) {
					return j
				}
			}
			if implicitWrite(n) {
				return -1
			}
		}
		return -1
	}
	a := prev(k, rx)
	if a < 0 {
		return "", false
	}
	am := reTableAdd.FindStringSubmatch(norm(block[a]))
	if am == nil || am[2] != rx || am[1] == rx {
		return "", false
	}
	rb := am[1]
	ld := prev(a, rx, rb)
	if ld < 0 {
		return "", false
	}
	lm := reTableLoad.FindStringSubmatch(norm(block[ld]))
	if lm == nil || lm[1] != rb || lm[3] != rx || lm[2] == rb {
		return "", false
	}
	le := prev(ld, rb)
	if le < 0 {
		return "", false
	}
	em := reTableLEA.FindStringSubmatch(norm(block[le]))
	if em == nil || em[2] != rb || tables[em[1]] == nil {
		return "", false
	}
	return em[1], true
}

// implicitWrite reports an instruction that may write a register it does not
// name: one with no operands (cltq, cqto, a string operation), a call, a rep
// or loop form, cmpxchg (the accumulator), or a one-operand multiply or
// divide (the accumulator and RDX).
func implicitWrite(n string) bool {
	f := strings.Fields(n)
	if len(f) == 0 {
		return false
	}
	mn := f[0]
	if len(f) == 1 {
		return mn != "nop"
	}
	for _, p := range []string{"call", "rep", "loop", "cmpxchg"} {
		if strings.HasPrefix(mn, p) {
			return true
		}
	}
	for _, p := range []string{"mul", "div", "idiv", "imul"} {
		if strings.HasPrefix(mn, p) && !strings.Contains(n, ",") {
			return true
		}
	}
	return false
}

// regAliases gives every name of a 64-bit general register.
func regAliases(r string) []string {
	switch r {
	case "rax", "rbx", "rcx", "rdx":
		c := r[1:2]
		return []string{r, "e" + c + "x", c + "x", c + "l", c + "h"}
	case "rsi", "rdi":
		c := r[1:2]
		return []string{r, "e" + c + "i", c + "i", c + "il"}
	case "rbp", "rsp":
		c := r[1:2]
		return []string{r, "e" + c + "p", c + "p", c + "pl"}
	}
	return []string{r, r + "d", r + "w", r + "b"}
}

// namesReg reports whether instruction n names register r in any width.
func namesReg(n, r string) bool {
	for _, a := range regAliases(r) {
		if regexp.MustCompile(`%` + a + `\b`).MatchString(n) {
			return true
		}
	}
	return false
}

// dataRefs also appends to rec (when not nil) what each reference now points
// to, for the post-link check.
func (env *LowerEnv) dataRefs(fn, l string, rec *[]RefRecord) (string, error) {
	n := norm(l)
	lea := strings.HasPrefix(n, "lea")
	var rerr error
	note := func(r RefRecord) {
		if rec != nil {
			*rec = append(*rec, r)
		}
	}
	out := reRIP.ReplaceAllStringFunc(l, func(m string) string {
		sub := reRIP.FindStringSubmatch(m)
		sym, add := sub[1], sub[2]
		k := 0
		if add != "" {
			var err error
			if k, err = strconv.Atoi(add); err != nil {
				rerr = fmt.Errorf("bad addend in %q", l)
				return m
			}
		}
		if env.Funcs[sym] {
			if !lea {
				rerr = fmt.Errorf("non-LEA reference to the function %s: %q", sym, l)
			}
			note(RefRecord{Label: sym, Add: int64(k), Func: true})
			return m
		}
		if r, ok := env.Plan.Rename[fn][sym]; ok && !(lea && hasBlob(env.Plan, sym)) {
			note(RefRecord{Label: sym, Add: int64(k)})
			return r + add + "(%rip)"
		}
		if off, ok := env.Plan.BlobOff[sym]; ok {
			if !lea {
				rerr = fmt.Errorf("non-LEA reference to %s in engine_rodata: %q", sym, l)
				return m
			}
			note(RefRecord{Label: sym, Add: int64(k), Blob: true})
			return fmt.Sprintf("__goasm_rodata+%d(%%rip)", off+k)
		}
		rerr = fmt.Errorf("unplaced data %s: %q", sym, l)
		return m
	})
	if rerr != nil {
		return "", rerr
	}
	// Absolute references (sym(,%reg,8), $sym) are errors of PlanData.
	return out, nil
}

func hasBlob(p *DataPlan, sym string) bool { _, ok := p.BlobOff[sym]; return ok }

// Layout gives the Go argument area of a signature: the offset of each
// parameter, of the result, and the total size.
func Layout(sig Sig) ([]int, int, int) { return layout(sig) }
