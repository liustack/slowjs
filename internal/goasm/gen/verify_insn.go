// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Instruction-level post-link check of the whole engine. The lowering reads
// llc's text with a hand-written parser and rewrites it; a parser mistake
// that still assembles (a misread operand, a dropped line, a wrong frame
// offset or argument move) would reach the binary without any byte check
// noticing, since the generator's own re-assembly is then the reference. This
// check takes llc's object file instead, which llc writes from its own
// instructions without any text, disassembles it and the generated functions
// in the linked Go binary with x/arch, and matches them instruction by
// instruction. A difference is accepted only where the lowering declared a
// rewrite (FuncRecord) and the check confirms the rewrite's form itself:
// the prologue and epilogue it replaced, the frame mapping applied to each
// displacement, each call's saves, argument moves, call target and result,
// and each data reference resolving to the same llc label plus offset. Every
// other llc instruction must appear in order with the same meaning. The read-
// only data is checked the same way: every byte and every relocated pointer
// of engine_rodata in the binary, and every constant pool and jump table a
// function carries, against llc's object.
package gen

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

// EngineCheck is what the post-link check reads.
type EngineCheck struct {
	LLC     string // llc's object of the engine, with temporary labels kept
	Lowered string // the re-assembled lowered text, with temporary labels kept
	Bin     string // a linked Go binary of the generated package
	Pkg     string // the package's link symbol prefix
	Names   NameMap
	Unit    *AsmUnit
	Plan    *DataPlan
	Metas   []Meta // lowered functions (unified and register-entry)
	Stubs   []Meta // in missingNames order
	Sigs    map[string]Sig
	Leaves  map[string]bool
	// All requires every function to be in the binary and checked.
	All bool
}

// CheckReport counts what was checked and lists what was not, by reason.
type CheckReport struct {
	Checked map[string]int
	Skipped map[string][]string
}

// CheckEngine runs the instruction-level and data checks.
func CheckEngine(c EngineCheck) (*CheckReport, error) {
	k := &checker{c: c, rep: &CheckReport{Checked: map[string]int{}, Skipped: map[string][]string{}}}
	var err error
	if k.llc, err = openObj(c.LLC); err != nil {
		return nil, err
	}
	if k.low, err = openObj(c.Lowered); err != nil {
		return nil, err
	}
	if k.bin, err = openBin(c.Bin); err != nil {
		return nil, err
	}
	if err := k.checkRodata(); err != nil {
		return k.rep, err
	}
	// Every function of llc's object is generated: lowered or a stub.
	generated := map[string]bool{}
	for _, m := range append(append([]Meta{}, c.Metas...), c.Stubs...) {
		generated[m.Name] = true
	}
	at := map[uint64]bool{}
	for name := range generated {
		if s, ok := k.llc.byName[name]; ok && int(s.Section) == k.llc.text {
			at[s.Value] = true
		}
	}
	for _, name := range SortedKeys(k.llc.byName) {
		s := k.llc.byName[name]
		// A second name at a generated function's start is an alias of it.
		if int(s.Section) == k.llc.text && elf.ST_TYPE(s.Info) == elf.STT_FUNC && !generated[name] && !at[s.Value] {
			return k.rep, fmt.Errorf("llc's function %s was neither lowered nor stubbed", name)
		}
	}
	for _, m := range c.Metas {
		if m.Rec == nil {
			return k.rep, fmt.Errorf("%s: no rewrite record", m.Name)
		}
		if _, ok := k.bin.abi0(c.Pkg, c.Names[m.Name]); !ok {
			k.skip("not in the binary (the linker left it out)", m.Name)
			continue
		}
		var err error
		if m.Kind == Register {
			err = k.checkLeaf(m)
			k.rep.Checked["register-entry leaves"]++
		} else {
			err = k.checkUnified(m)
			k.rep.Checked["unified functions"]++
		}
		if err != nil {
			return k.rep, fmt.Errorf("%s: %w", m.Name, err)
		}
	}
	for i, m := range c.Stubs {
		if _, ok := k.bin.abi0(c.Pkg, c.Names[m.Name]); !ok {
			k.skip("not in the binary (the linker left it out)", m.Name)
			continue
		}
		if err := k.checkStub(m, i); err != nil {
			return k.rep, fmt.Errorf("stub %s: %w", m.Name, err)
		}
		k.rep.Checked["stubs"]++
	}
	if c.All {
		for why, fns := range k.rep.Skipped {
			return k.rep, fmt.Errorf("%d functions not checked (%s), e.g. %s", len(fns), why, fns[0])
		}
	}
	return k.rep, nil
}

type checker struct {
	c        EngineCheck
	llc, low *objFile
	bin      *binFile
	rep      *CheckReport
}

func (k *checker) skip(why, fn string) { k.rep.Skipped[why] = append(k.rep.Skipped[why], fn) }

// ---- object files ----

type objReloc struct {
	typ elf.R_X86_64
	sym elf.Symbol
	add int64
}

type objFile struct {
	f      *elf.File
	byName map[string]elf.Symbol
	data   map[int][]byte
	rel    map[int]map[uint64]objReloc
	text   int
	starts map[int][]uint64 // section -> sorted values of its named symbols
}

func openObj(path string) (*objFile, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	o := &objFile{f: f, byName: map[string]elf.Symbol{}, data: map[int][]byte{}, rel: map[int]map[uint64]objReloc{},
		text: -1, starts: map[int][]uint64{}}
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	for i, s := range f.Sections {
		if s.Name == ".text" {
			o.text = i
		}
		if s.Type == elf.SHT_PROGBITS && s.Flags&elf.SHF_ALLOC != 0 {
			if o.data[i], err = s.Data(); err != nil {
				return nil, err
			}
		}
	}
	if o.text < 0 {
		return nil, fmt.Errorf("%s: no .text", path)
	}
	for _, s := range syms {
		t := elf.ST_TYPE(s.Info)
		if s.Name == "" || t == elf.STT_SECTION || t == elf.STT_FILE {
			continue
		}
		if _, dup := o.byName[s.Name]; dup {
			return nil, fmt.Errorf("%s: two symbols named %s", path, s.Name)
		}
		o.byName[s.Name] = s
		if s.Section != elf.SHN_UNDEF && int(s.Section) < len(f.Sections) {
			o.starts[int(s.Section)] = append(o.starts[int(s.Section)], s.Value)
		}
	}
	for i := range o.starts {
		sort.Slice(o.starts[i], func(a, b int) bool { return o.starts[i][a] < o.starts[i][b] })
	}
	for _, rs := range f.Sections {
		if rs.Type != elf.SHT_RELA {
			continue
		}
		d, err := rs.Data()
		if err != nil {
			return nil, err
		}
		m := map[uint64]objReloc{}
		for off := 0; off+24 <= len(d); off += 24 {
			info := binary.LittleEndian.Uint64(d[off+8:])
			si := int(info >> 32)
			if si < 1 || si > len(syms) {
				return nil, fmt.Errorf("%s: relocation with symbol index %d", path, si)
			}
			m[binary.LittleEndian.Uint64(d[off:])] = objReloc{elf.R_X86_64(info & 0xffffffff), syms[si-1],
				int64(binary.LittleEndian.Uint64(d[off+16:]))}
		}
		o.rel[int(rs.Info)] = m
	}
	return o, nil
}

// loc is a place in an object: a section and an offset in it.
type loc struct {
	sec int
	off int64
}

// label finds a defined symbol by name.
func (o *objFile) label(name string) (elf.Symbol, error) {
	s, ok := o.byName[name]
	if !ok || s.Section == elf.SHN_UNDEF || int(s.Section) >= len(o.f.Sections) {
		return elf.Symbol{}, fmt.Errorf("label %s not defined in llc's object", name)
	}
	return s, nil
}

// extent is the size of the data object at s: its st_size when llc wrote a
// .size, the entry size of a mergeable constant section, or for a table of
// relative entries the run of relocated 4-byte slots up to the next label,
// with nothing but zero padding after it. Anything else is unknown.
func (o *objFile) extent(s elf.Symbol) (int, error) {
	if s.Size > 0 {
		return int(s.Size), nil
	}
	si := int(s.Section)
	sec := o.f.Sections[si]
	if sec.Flags&elf.SHF_MERGE != 0 && sec.Flags&elf.SHF_STRINGS == 0 && sec.Entsize > 0 {
		return int(sec.Entsize), nil
	}
	end := uint64(len(o.data[si]))
	for _, v := range o.starts[si] {
		if v > s.Value {
			end = v
			break
		}
	}
	n := uint64(0)
	for s.Value+n+4 <= end {
		r, ok := o.rel[si][s.Value+n]
		if !ok || r.typ != elf.R_X86_64_PC32 {
			break
		}
		n += 4
	}
	if n == 0 {
		return 0, fmt.Errorf("size of the data at %s is unknown", s.Name)
	}
	for b := s.Value + n; b < end; b++ {
		if _, rel := o.rel[si][b]; rel || o.data[si][b] != 0 {
			return 0, fmt.Errorf("size of the table at %s is unknown: data follows its relocated entries", s.Name)
		}
	}
	return int(n), nil
}

// ---- the linked binary ----

type binFile struct {
	f    *elf.File
	syms map[string]elf.Symbol
	secs map[*elf.Section][]byte
}

func openBin(path string) (*binFile, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	if f.Type != elf.ET_EXEC || f.Machine != elf.EM_X86_64 {
		return nil, fmt.Errorf("%s: not a linked amd64 executable (PIE pointers are not in the file)", path)
	}
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	b := &binFile{f: f, syms: map[string]elf.Symbol{}, secs: map[*elf.Section][]byte{}}
	for _, s := range syms {
		b.syms[s.Name] = s
	}
	return b, nil
}

// upTo reads at most n bytes at addr, fewer at the end of a section.
func (b *binFile) upTo(addr uint64, n int) ([]byte, error) {
	for _, s := range b.f.Sections {
		if s.Type == elf.SHT_PROGBITS && addr >= s.Addr && addr < s.Addr+s.Size {
			if end := s.Addr + s.Size; addr+uint64(n) > end {
				n = int(end - addr)
			}
			return b.read(addr, n)
		}
	}
	return nil, fmt.Errorf("address %#x not in the binary", addr)
}

func (b *binFile) read(addr uint64, n int) ([]byte, error) {
	for _, s := range b.f.Sections {
		if s.Type != elf.SHT_PROGBITS || addr < s.Addr || addr+uint64(n) > s.Addr+s.Size {
			continue
		}
		d, ok := b.secs[s]
		if !ok {
			var err error
			if d, err = s.Data(); err != nil {
				return nil, err
			}
			b.secs[s] = d
		}
		return d[addr-s.Addr : addr-s.Addr+uint64(n)], nil
	}
	return nil, fmt.Errorf("address %#x+%d not in the binary", addr, n)
}

// abi0 gives the ABI0 entry of a Go-visible function: the assembly body
// (pkg.f, or pkg.f.abi0 when Go also has an ABIInternal wrapper) or the ABI0
// wrapper of a Go function. Every CALL, LEAQ and DATA of the generated
// assembly names this symbol.
func (b *binFile) abi0(pkg, goName string) (elf.Symbol, bool) {
	if s, ok := b.syms[pkg+"."+goName+".abi0"]; ok {
		return s, true
	}
	s, ok := b.syms[pkg+"."+goName]
	return s, ok
}

func (k *checker) funcAddr(cname string) (uint64, error) {
	gn := k.c.Names[cname]
	if gn == "" {
		gn = cname
	}
	s, ok := k.bin.abi0(k.c.Pkg, gn)
	if !ok {
		return 0, fmt.Errorf("function %s not in the binary", cname)
	}
	return s.Value, nil
}

// ---- instruction streams ----

type insn struct {
	at uint64 // section offset (llc) or address (binary)
	in x86asm.Inst
}

// stream is a run of decoded code without its padding: the no-ops are left
// out, and at maps the start of every decoded instruction (a no-op too) to
// the index of the first real one at or after it, so a branch to padding
// lands where execution would.
type stream struct {
	ins []insn
	at  map[uint64]int
}

// isPad reports alignment padding. Only NOP is: XCHG of a 32-bit register
// with itself clears the upper half, so it is a real instruction.
func isPad(in x86asm.Inst) bool { return in.Op == x86asm.NOP }

// decode decodes one instruction and puts its operands in the form the
// check compares: x86asm returns a 32-bit displacement zero-extended, while
// the processor sign-extends it in 64-bit mode.
func decode(b []byte) (x86asm.Inst, error) {
	in, err := x86asm.Decode(b, 64)
	if err != nil {
		return in, err
	}
	for i, a := range in.Args {
		if m, ok := a.(x86asm.Mem); ok {
			m.Disp = int64(int32(m.Disp))
			if m.Index == 0 {
				m.Scale = 0
			}
			in.Args[i] = m
		}
	}
	return in, nil
}

func decodeStream(code []byte, base uint64) (stream, error) {
	s := stream{at: map[uint64]int{}}
	var pads []uint64
	for off := 0; off < len(code); {
		in, err := decode(code[off:])
		if err != nil {
			return s, fmt.Errorf("cannot decode at %#x: %v", base+uint64(off), err)
		}
		a := base + uint64(off)
		if isPad(in) {
			pads = append(pads, a)
		} else {
			for _, p := range pads {
				s.at[p] = len(s.ins)
			}
			pads = pads[:0]
			s.at[a] = len(s.ins)
			s.ins = append(s.ins, insn{a, in})
		}
		off += in.Len
	}
	for _, p := range pads {
		s.at[p] = len(s.ins)
	}
	s.at[base+uint64(len(code))] = len(s.ins)
	return s, nil
}

// ---- registers and the ABI, written out again here so the check does not
// share the lowering's tables ----

var gpRegs = map[string]x86asm.Reg{"rax": x86asm.RAX, "rbx": x86asm.RBX, "rcx": x86asm.RCX, "rdx": x86asm.RDX,
	"rsi": x86asm.RSI, "rdi": x86asm.RDI, "r8": x86asm.R8, "r9": x86asm.R9, "r10": x86asm.R10, "r11": x86asm.R11,
	"r12": x86asm.R12, "r13": x86asm.R13, "r14": x86asm.R14, "r15": x86asm.R15}

var calleeSavedRegs = map[x86asm.Reg]bool{x86asm.RBX: true, x86asm.R12: true, x86asm.R13: true, x86asm.R14: true, x86asm.R15: true}

// frameRegs are every name of RSP and RBP.
var frameRegs = map[x86asm.Reg]bool{x86asm.RSP: true, x86asm.ESP: true, x86asm.SP: true, x86asm.SPB: true,
	x86asm.RBP: true, x86asm.EBP: true, x86asm.BP: true, x86asm.BPB: true}

// argWords gives the native register of each argument word of sig, in order
// (one per 8-byte slot of the Go argument area), and whether it is an SSE one.
func argWords(sig Sig, pn bool) ([]x86asm.Reg, []bool, error) {
	ints := []x86asm.Reg{x86asm.RDI, x86asm.RSI, x86asm.RDX, x86asm.RCX, x86asm.R8, x86asm.R9}
	if pn {
		ints = []x86asm.Reg{x86asm.R12, x86asm.R13, x86asm.R14, x86asm.R15, x86asm.RDI, x86asm.RSI, x86asm.RDX,
			x86asm.RCX, x86asm.R8, x86asm.R9, x86asm.R11, x86asm.RAX}
	}
	sses := []x86asm.Reg{x86asm.X0, x86asm.X1, x86asm.X2, x86asm.X3, x86asm.X4, x86asm.X5, x86asm.X6, x86asm.X7}
	var regs []x86asm.Reg
	var sse []bool
	ni, nx := 0, 0
	for _, p := range sig.Params {
		switch p {
		case I64, Ptr, JSV:
			n := 1
			if p == JSV {
				n = 2
			}
			for w := 0; w < n; w++ {
				if ni >= len(ints) {
					return nil, nil, fmt.Errorf("too many integer arguments")
				}
				regs, sse = append(regs, ints[ni]), append(sse, false)
				ni++
			}
		case F64:
			if nx >= len(sses) {
				return nil, nil, fmt.Errorf("too many floating-point arguments")
			}
			regs, sse = append(regs, sses[nx]), append(sse, true)
			nx++
		default:
			return nil, nil, fmt.Errorf("bad parameter type")
		}
	}
	return regs, sse, nil
}

// result is one register of a function's result.
type result struct {
	r   x86asm.Reg
	sse bool
}

// results gives the registers a native body leaves its result in, in the
// order of the result's words.
func results(t Type) []result {
	switch t {
	case I64, Ptr:
		return []result{{x86asm.RAX, false}}
	case F64:
		return []result{{x86asm.X0, true}}
	case JSV:
		return []result{{x86asm.RAX, false}, {x86asm.RDX, false}}
	}
	return nil
}

func retWords(t Type) int {
	switch t {
	case Void:
		return 0
	case JSV:
		return 2
	}
	return 1
}

func memOf(in x86asm.Inst) (x86asm.Mem, int, bool) {
	for i, a := range in.Args {
		if m, ok := a.(x86asm.Mem); ok {
			return m, i, true
		}
	}
	return x86asm.Mem{}, -1, false
}

func normMem(m x86asm.Mem) x86asm.Mem {
	if m.Index == 0 {
		m.Scale = 0
	}
	m.Disp = int64(int32(m.Disp))
	return m
}

// isStackMove reports a 64-bit (or SSE scalar) move between reg and
// disp(RSP), a store when store is set.
func isStackMove(in x86asm.Inst, store bool, reg x86asm.Reg, disp int64, sse bool) bool {
	op := x86asm.MOV
	if sse {
		op = x86asm.MOVSD_XMM
	}
	mi, ri := 1, 0
	if store {
		mi, ri = 0, 1
	}
	m, ok1 := in.Args[mi].(x86asm.Mem)
	r, ok2 := in.Args[ri].(x86asm.Reg)
	if in.Op != op || !ok1 || !ok2 || in.Args[2] != nil || in.MemBytes != 8 || (!sse && in.DataSize != 64) {
		return false
	}
	m = normMem(m)
	return r == reg && m == (x86asm.Mem{Base: x86asm.RSP, Disp: disp})
}

// semPrefixes lists the prefixes that change what an instruction does
// (LOCK, REP, segment and branch hints), leaving out REX and the size
// prefixes, which DataSize and AddrSize already carry, and the ones the
// decoder marks implied or ignored.
func semPrefixes(in x86asm.Inst) []x86asm.Prefix {
	var out []x86asm.Prefix
	for _, p := range in.Prefix {
		if p == 0 {
			break
		}
		if p&(x86asm.PrefixImplicit|x86asm.PrefixIgnored) != 0 || p.IsREX() || p.IsVEX() {
			continue
		}
		switch b := p & 0xff; b {
		case 0x66, 0x67:
		default:
			out = append(out, b)
		}
	}
	return out
}

// sameMeaning compares two instructions with the memory operand of the
// first replaced by want (when want is set) and with any relative branch
// argument left out (branches are compared through the instruction map).
// remapped is set for an aligned move turned unaligned, whose encodings
// differ in the operand size attribute (MOVDQA has 66, MOVDQU F3).
func sameMeaning(a, b x86asm.Inst, want *x86asm.Mem, remapped bool) bool {
	if a.Op != b.Op || (!remapped && a.DataSize != b.DataSize) || a.AddrSize != b.AddrSize || a.MemBytes != b.MemBytes {
		return false
	}
	pa, pb := semPrefixes(a), semPrefixes(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return false
		}
	}
	for i := range a.Args {
		x, y := a.Args[i], b.Args[i]
		if mx, ok := x.(x86asm.Mem); ok {
			my, ok := y.(x86asm.Mem)
			if !ok {
				return false
			}
			if want != nil {
				mx = *want
			}
			if normMem(mx) != normMem(my) {
				return false
			}
			continue
		}
		if _, ok := x.(x86asm.Rel); ok {
			if _, ok := y.(x86asm.Rel); !ok {
				return false
			}
			continue
		}
		if x != y {
			return false
		}
	}
	return true
}

// ---- the Go wrapper around each body ----

type goFrame struct {
	split     bool   // Go checks the stack and grows it (not NOSPLIT)
	bp        bool   // Go pushed BP
	frame     int64  // Go's SP adjustment below BP
	more      uint64 // the stack check's branch to morestack
	afterLead uint64 // the first byte after Go's prologue
}

func (k *checker) dec(addr uint64) (x86asm.Inst, error) {
	b, err := k.bin.upTo(addr, 16)
	if err != nil {
		return x86asm.Inst{}, err
	}
	in, err := decode(b)
	if err != nil {
		return in, fmt.Errorf("cannot decode the binary at %#x: %v", addr, err)
	}
	return in, nil
}

func isReg(a x86asm.Arg, r x86asm.Reg) bool { x, ok := a.(x86asm.Reg); return ok && x == r }

func spAdjust(in x86asm.Inst) (int64, bool) {
	if !isReg(in.Args[0], x86asm.RSP) || in.DataSize != 64 {
		return 0, false
	}
	imm, ok := in.Args[1].(x86asm.Imm)
	if !ok {
		return 0, false
	}
	v := int64(int32(imm))
	switch in.Op {
	case x86asm.SUB:
		return v, true
	case x86asm.ADD:
		return -v, true
	}
	return 0, false
}

// goPrologue reads the prologue the Go assembler put before a body: the g
// load and stack check of a split function, then BP and the frame.
func (k *checker) goPrologue(start uint64) (goFrame, error) {
	var g goFrame
	a := start
	in, err := k.dec(a)
	if err != nil {
		return g, err
	}
	if in.Op == x86asm.MOV && isReg(in.Args[0], x86asm.R14) {
		m, ok := in.Args[1].(x86asm.Mem)
		if !ok || m.Segment != x86asm.FS || m.Base != 0 || m.Disp != -8 {
			return g, fmt.Errorf("unknown Go prologue at %#x: %v", a, in)
		}
		g.split = true
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		check := x86asm.RSP
		if in.Op == x86asm.LEA && isReg(in.Args[0], x86asm.R12) {
			if m, ok := in.Args[1].(x86asm.Mem); !ok || m.Base != x86asm.RSP || m.Index != 0 {
				return g, fmt.Errorf("unknown Go stack check at %#x: %v", a, in)
			}
			check = x86asm.R12
			a += uint64(in.Len)
			if in, err = k.dec(a); err != nil {
				return g, err
			}
		}
		guard, ok := in.Args[1].(x86asm.Mem)
		if in.Op != x86asm.CMP || !isReg(in.Args[0], check) || !ok || guard.Base != x86asm.R14 || guard.Disp != 16 {
			return g, fmt.Errorf("unknown Go stack check at %#x: %v", a, in)
		}
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		rel, ok := in.Args[0].(x86asm.Rel)
		if in.Op != x86asm.JBE || !ok {
			return g, fmt.Errorf("unknown Go stack check at %#x: %v", a, in)
		}
		g.more = uint64(int64(a) + int64(in.Len) + int64(rel))
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
	}
	if in.Op == x86asm.PUSH && isReg(in.Args[0], x86asm.RBP) {
		g.bp = true
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		if in.Op != x86asm.MOV || !isReg(in.Args[0], x86asm.RBP) || !isReg(in.Args[1], x86asm.RSP) {
			return g, fmt.Errorf("unknown Go prologue at %#x: %v", a, in)
		}
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		if d, ok := spAdjust(in); ok {
			g.frame = d
			a += uint64(in.Len)
		}
	}
	g.afterLead = a
	return g, nil
}

// goEpilogue checks what follows a body at e: Go's expansion of RET, then
// for a split function the morestack call and the jump back to the start.
func (k *checker) goEpilogue(g goFrame, e uint64, sym elf.Symbol) error {
	a := e
	next := func() (x86asm.Inst, error) {
		in, err := k.dec(a)
		if err == nil {
			a += uint64(in.Len)
		}
		return in, err
	}
	in, err := next()
	if err != nil {
		return err
	}
	if g.bp {
		if g.frame != 0 {
			if d, ok := spAdjust(in); !ok || d != -g.frame {
				return fmt.Errorf("Go epilogue at %#x does not free the %d-byte frame: %v", e, g.frame, in)
			}
			if in, err = next(); err != nil {
				return err
			}
		}
		if in.Op != x86asm.POP || !isReg(in.Args[0], x86asm.RBP) {
			return fmt.Errorf("Go epilogue at %#x does not restore BP: %v", e, in)
		}
		if in, err = next(); err != nil {
			return err
		}
	}
	if in.Op != x86asm.RET || in.Args[0] != nil {
		return fmt.Errorf("Go epilogue at %#x does not return: %v", e, in)
	}
	if g.split {
		if a != g.more {
			return fmt.Errorf("the stack check branches to %#x, not to the morestack call at %#x", g.more, a)
		}
		ms, ok := k.bin.syms["runtime.morestack_noctxt.abi0"]
		if !ok {
			return fmt.Errorf("no runtime.morestack_noctxt.abi0 in the binary")
		}
		at := a
		if in, err = next(); err != nil {
			return err
		}
		if rel, ok := in.Args[0].(x86asm.Rel); in.Op != x86asm.CALL || !ok || uint64(int64(a)+int64(rel)) != ms.Value {
			return fmt.Errorf("no morestack call at %#x: %v", at, in)
		}
		at = a
		if in, err = next(); err != nil {
			return err
		}
		if rel, ok := in.Args[0].(x86asm.Rel); in.Op != x86asm.JMP || !ok || uint64(int64(a)+int64(rel)) != sym.Value {
			return fmt.Errorf("no jump back to the start after morestack at %#x: %v", at, in)
		}
	}
	end := sym.Value + sym.Size
	if a > end {
		return fmt.Errorf("Go epilogue runs past the symbol's end")
	}
	pad, err := k.bin.read(a, int(end-a))
	if err != nil {
		return err
	}
	for _, b := range pad {
		if b != 0xcc && b != 0x90 && b != 0 {
			return fmt.Errorf("unexplained bytes after the Go epilogue at %#x", a)
		}
	}
	return nil
}

// ---- one unified function ----

type branch struct {
	llcTarget, binTarget uint64
	at                   uint64
}

type matcher struct {
	k        *checker
	r        *FuncRecord
	L, B     stream
	j        int
	bStart   []int
	branches []branch
	rets     []uint64 // binary targets of the jumps that replace returns
	calls    int
	refs     int
	argBase  int64
	body     uint64 // address of the body in the binary
	lowSym   elf.Symbol
}

func (m *matcher) next(what string) (insn, error) {
	if m.j >= len(m.B.ins) {
		return insn{}, fmt.Errorf("the generated body ends where %s should be", what)
	}
	m.j++
	return m.B.ins[m.j-1], nil
}

// bodyOf locates the lowered function in the re-assembled object: its start
// and size, and where its code ends (its constants follow).
func (k *checker) bodyOf(name string) (elf.Symbol, uint64, error) {
	s, ok := k.low.byName[name]
	if !ok {
		return s, 0, fmt.Errorf("absent from the re-assembled object")
	}
	code := s.Size
	if c, ok := k.low.byName[name+".const"]; ok {
		code = c.Value - s.Value
	}
	return s, code, nil
}

func (k *checker) llcFunc(name string) (stream, elf.Symbol, error) {
	s, err := k.llc.label(name)
	if err != nil {
		return stream{}, s, err
	}
	if int(s.Section) != k.llc.text || elf.ST_TYPE(s.Info) != elf.STT_FUNC {
		return stream{}, s, fmt.Errorf("%s is not a function in llc's .text", name)
	}
	d := k.llc.data[k.llc.text]
	st, err := decodeStream(d[s.Value:s.Value+s.Size], s.Value)
	return st, s, err
}

func (k *checker) checkUnified(meta Meta) error {
	r := meta.Rec
	L, _, err := k.llcFunc(r.Name)
	if err != nil {
		return err
	}
	bsym, _ := k.bin.abi0(k.c.Pkg, k.c.Names[r.Name])
	g, err := k.goPrologue(bsym.Value)
	if err != nil {
		return err
	}
	// A function that calls must check the stack and keep BP; a frameless
	// one that does not is a NOSPLIT leaf with its arguments right above the
	// return address. argBase below follows what Go wrote either way.
	if len(r.Calls) > 0 && (!g.split || !g.bp) {
		return fmt.Errorf("a calling function without Go's stack check or frame: split %v, BP %v", g.split, g.bp)
	}
	if g.frame != int64(r.Frame) {
		return fmt.Errorf("Go frame is %d bytes, the lowering declared %d", g.frame, r.Frame)
	}
	body := alignUp64(g.afterLead, 16)
	if err := k.allPad(g.afterLead, body); err != nil {
		return err
	}
	lowSym, codeLen, err := k.bodyOf(r.Name)
	if err != nil {
		return err
	}
	code, err := k.bin.read(body, int(codeLen))
	if err != nil {
		return err
	}
	B, err := decodeStream(code, body)
	if err != nil {
		return err
	}
	e := body + lowSym.Size
	if err := k.goEpilogue(g, e, bsym); err != nil {
		return err
	}
	argBase := 8 + g.frame
	if g.bp {
		argBase += 8
	}
	m := &matcher{k: k, r: r, L: L, B: B, bStart: make([]int, len(L.ins)), argBase: argBase, body: body, lowSym: lowSym}
	for i := range m.bStart {
		m.bStart[i] = -1
	}
	if err := m.frameLayout(); err != nil {
		return err
	}
	if err := m.run(); err != nil {
		return err
	}
	// The code ends with the result jump target: the Go epilogue, or a jump
	// over the constants to it.
	retAt := map[uint64]bool{e: true}
	if m.j == len(B.ins)-1 {
		last := B.ins[m.j].in
		if rel, ok := last.Args[0].(x86asm.Rel); last.Op == x86asm.JMP && ok &&
			uint64(int64(B.ins[m.j].at)+int64(last.Len)+int64(rel)) == e {
			retAt[B.ins[m.j].at] = true
			m.j++
		}
	}
	if m.j != len(B.ins) {
		return fmt.Errorf("generated instruction at %#x explained by no llc instruction: %v", B.ins[m.j].at, B.ins[m.j].in)
	}
	for _, t := range m.rets {
		if i, ok := B.at[t]; ok && i < len(B.ins) {
			t = B.ins[i].at
		}
		if !retAt[t] {
			return fmt.Errorf("a return jumps to %#x, not to the Go epilogue", t)
		}
	}
	for _, br := range m.branches {
		li, ok := L.at[br.llcTarget]
		if !ok || li >= len(L.ins) || m.bStart[li] < 0 {
			return fmt.Errorf("branch at %#x: llc's target %#x is no instruction of the body", br.at, br.llcTarget)
		}
		bi, ok := B.at[br.binTarget]
		if !ok || bi != m.bStart[li] {
			return fmt.Errorf("branch at %#x goes to %#x, not to where llc's target %#x went", br.at, br.binTarget, br.llcTarget)
		}
	}
	if m.calls != len(r.Calls) || m.refs != len(r.Refs) {
		return fmt.Errorf("the lowering declared %d calls and %d references, llc's object has %d and %d",
			len(r.Calls), len(r.Refs), m.calls, m.refs)
	}
	return m.bodyData()
}

func alignUp64(x, a uint64) uint64 { return (x + a - 1) / a * a }

func (k *checker) allPad(from, to uint64) error {
	if to <= from {
		return nil
	}
	b, err := k.bin.read(from, int(to-from))
	if err != nil {
		return err
	}
	s, err := decodeStream(b, from)
	if err != nil || len(s.ins) != 0 {
		return fmt.Errorf("unexplained code between the Go prologue and the body at %#x", from)
	}
	return nil
}

// frameLayout checks the declared Go frame holds its parts apart: the
// outgoing arguments below the saved registers, those below the locals, all
// inside the frame, and the saved set covering every callee-saved register
// llc's code keeps a value in across a call.
func (m *matcher) frameLayout() error {
	r := m.r
	if r.SaveBase < 0 || r.SaveBase+8*len(r.Saved) > r.LocalBase || r.LocalBase+r.Local > r.Frame {
		return fmt.Errorf("frame parts overlap: saves at %d (%d), locals at %d (%d), frame %d",
			r.SaveBase, len(r.Saved), r.LocalBase, r.Local, r.Frame)
	}
	saved := map[string]bool{}
	for _, s := range r.Saved {
		if !calleeSavedRegs[gpRegs[s]] {
			return fmt.Errorf("saves %s, which is not callee-saved", s)
		}
		saved[s] = true
	}
	if r.PreserveNone {
		if len(r.Pushed) > 0 {
			return fmt.Errorf("preserve_none code pushed %v", r.Pushed)
		}
		return nil
	}
	for _, p := range r.Pushed {
		if !saved[p] {
			return fmt.Errorf("llc keeps %s across calls, but the calls do not save it", p)
		}
	}
	return nil
}

func (m *matcher) run() error {
	L := m.L.ins
	i := 0
	want := func(ok bool, what string) error {
		if !ok {
			at := uint64(0)
			if i < len(L) {
				at = L[i].at
			}
			return fmt.Errorf("llc's prologue at %#x: no %s", at, what)
		}
		return nil
	}
	if err := want(len(L) >= 2 && L[0].in.Op == x86asm.PUSH && isReg(L[0].in.Args[0], x86asm.RBP), "push of RBP"); err != nil {
		return err
	}
	if err := want(L[1].in.Op == x86asm.MOV && isReg(L[1].in.Args[0], x86asm.RBP) && isReg(L[1].in.Args[1], x86asm.RSP), "mov RSP to RBP"); err != nil {
		return err
	}
	i = 2
	var pushed []string
	for i < len(L) && L[i].in.Op == x86asm.PUSH {
		r, ok := L[i].in.Args[0].(x86asm.Reg)
		if !ok || !calleeSavedRegs[r] {
			break
		}
		pushed = append(pushed, strings.ToLower(r.String()))
		i++
	}
	local := 0
	for i < len(L) {
		in := L[i].in
		if in.Op == x86asm.PUSH && (isReg(in.Args[0], x86asm.RAX) || isReg(in.Args[0], x86asm.RCX)) {
			local += 8
			i++
			continue
		}
		if d, ok := spAdjust(in); ok && d > 0 {
			local += int(d)
			i++
			continue
		}
		break
	}
	if strings.Join(pushed, ",") != strings.Join(m.r.Pushed, ",") || local != m.r.Local {
		return fmt.Errorf("llc's prologue pushes %v and reserves %d, the lowering declared %v and %d", pushed, local, m.r.Pushed, m.r.Local)
	}
	// The entry loads each argument from the Go argument area.
	regs, sse, err := argWords(m.r.Sig, m.r.PreserveNone)
	if err != nil {
		return err
	}
	for w := range regs {
		b, err := m.next("an argument load")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, false, regs[w], m.argBase+int64(8*w), sse[w]) {
			return fmt.Errorf("entry at %#x: %v is not the load of argument word %d into %v", b.at, b.in, w, regs[w])
		}
	}
	for i < len(L) {
		m.bStart[i] = m.j
		x := L[i]
		if d, ok := spAdjust(x.in); ok && d < 0 || x.in.Op == x86asm.POP {
			n, err := m.epilogue(i, -d)
			if err != nil {
				return err
			}
			for k := i; k < i+n; k++ {
				m.bStart[k] = m.j
			}
			i += n
			continue
		}
		if err := m.one(x); err != nil {
			return fmt.Errorf("llc instruction at %#x (%v): %w", x.at, x.in, err)
		}
		i++
	}
	return nil
}

// epilogue checks that the frame restore at L[i] is a whole canonical
// epilogue (free the locals, pop the pushed registers in reverse, pop RBP)
// right before a return, and returns how many instructions it drops.
func (m *matcher) epilogue(i int, freed int64) (int, error) {
	L := m.L.ins
	k := i
	if L[k].in.Op != x86asm.POP {
		if freed != int64(m.r.Local) {
			return 0, fmt.Errorf("llc frees %d bytes at %#x, its prologue reserved %d", freed, L[k].at, m.r.Local)
		}
		k++
	} else if m.r.Local != 0 {
		return 0, fmt.Errorf("llc pops at %#x without freeing its %d-byte locals", L[k].at, m.r.Local)
	}
	// The pushed registers in reverse, then RBP.
	var pops []x86asm.Reg
	for p := len(m.r.Pushed) - 1; p >= 0; p-- {
		pops = append(pops, gpRegs[m.r.Pushed[p]])
	}
	pops = append(pops, x86asm.RBP)
	for _, r := range pops {
		if k >= len(L) || r == 0 || L[k].in.Op != x86asm.POP || !isReg(L[k].in.Args[0], r) {
			return 0, fmt.Errorf("llc's epilogue at %#x does not pop %v", L[i].at, r)
		}
		k++
	}
	if k >= len(L) || L[k].in.Op != x86asm.RET {
		return 0, fmt.Errorf("llc's epilogue at %#x is not followed by a return", L[i].at)
	}
	return k - i, nil
}

// forbiddenOps are instructions the engine's code may not hold: the ones that
// reach the system or read machine state, and the ones that move SP or BP
// implicitly outside the prologue and epilogue the check accounts for.
var forbiddenOps = map[x86asm.Op]string{
	x86asm.SYSCALL: "a system call", x86asm.SYSENTER: "a system call", x86asm.SYSEXIT: "a system call",
	x86asm.SYSRET: "a system call", x86asm.INT: "a software interrupt",
	x86asm.RDTSC: "reads the time stamp counter", x86asm.RDTSCP: "reads the time stamp counter",
	x86asm.RDRAND: "reads the hardware random source", x86asm.RDPMC: "reads a performance counter",
	x86asm.RDMSR: "reads a machine register", x86asm.RDFSBASE: "reads a segment base", x86asm.RDGSBASE: "reads a segment base",
	x86asm.CPUID: "reads the processor's identity",
	x86asm.PUSH:  "pushes in the body", x86asm.PUSHF: "pushes in the body", x86asm.PUSHFD: "pushes in the body",
	x86asm.PUSHFQ: "pushes in the body", x86asm.POPF: "pops in the body", x86asm.POPFD: "pops in the body",
	x86asm.POPFQ: "pops in the body",
	x86asm.ENTER: "builds a frame in the body", x86asm.LEAVE: "leaves a frame in the body",
}

// unalignedOK are the 16-byte moves that need no alignment: the Go frame is
// only 8-byte aligned, so any other access wider than 8 bytes may fault.
var unalignedOK = map[x86asm.Op]bool{x86asm.MOVUPS: true, x86asm.MOVUPD: true, x86asm.MOVDQU: true, x86asm.LDDQU: true}

// alignedToUnaligned is the rewrite the lowering makes of an aligned 16-byte
// move to the frame.
var alignedToUnaligned = map[x86asm.Op]x86asm.Op{x86asm.MOVAPS: x86asm.MOVUPS, x86asm.MOVAPD: x86asm.MOVUPD, x86asm.MOVDQA: x86asm.MOVDQU}

// one matches a single llc instruction of the body.
func (m *matcher) one(x insn) error {
	in := x.in
	r := m.k.llc.relIn(m.k.llc.text, x)
	if why, bad := forbiddenOps[in.Op]; bad {
		return fmt.Errorf("%s", why)
	}
	switch in.Op {
	case x86asm.RET:
		if in.Args[0] != nil {
			return fmt.Errorf("a return that pops arguments")
		}
		return m.ret()
	case x86asm.CALL:
		return m.call(x, r)
	}
	for _, a := range in.Args {
		if reg, ok := a.(x86asm.Reg); ok && frameRegs[reg] {
			return fmt.Errorf("uses the stack or frame register outside a memory operand")
		}
	}
	mem, _, hasMem := memOf(in)
	if hasMem && frameRegs[mem.Index] {
		return fmt.Errorf("indexes by the stack or frame register")
	}
	if hasMem && mem.Base == 0 && mem.Index == 0 {
		return fmt.Errorf("an absolute memory operand")
	}
	if hasMem && (mem.Segment == x86asm.FS || mem.Segment == x86asm.GS) {
		return fmt.Errorf("a thread-local memory operand")
	}
	if rel, ok := in.Args[0].(x86asm.Rel); ok {
		if r != nil {
			return fmt.Errorf("a branch with a relocation (a tail call)")
		}
		b, err := m.next("a branch")
		if err != nil {
			return err
		}
		brel, ok := b.in.Args[0].(x86asm.Rel)
		if !ok || !sameMeaning(in, b.in, nil, false) {
			return fmt.Errorf("generated %v at %#x instead", b.in, b.at)
		}
		m.branches = append(m.branches, branch{uint64(int64(x.at) + int64(in.Len) + int64(rel)),
			uint64(int64(b.at) + int64(b.in.Len) + int64(brel)), b.at})
		return nil
	}
	if in.Op == x86asm.JMP {
		// Through a register: kept as it is (its targets are the jump
		// table's entries, checked with the function's data).
		if _, ok := in.Args[0].(x86asm.Reg); !ok {
			return fmt.Errorf("an indirect jump through memory")
		}
	}
	var want *x86asm.Mem
	var target uint64
	remapped := false
	switch {
	case hasMem && mem.Base == x86asm.RIP:
		t, err := m.dataRef(x, r, mem)
		if err != nil {
			return err
		}
		target = t
	case r != nil:
		return fmt.Errorf("a relocation outside a RIP-relative operand")
	case hasMem && (mem.Base == x86asm.RBP || mem.Base == x86asm.RSP):
		if in.Op == x86asm.LEA {
			return fmt.Errorf("takes the address of a stack slot, which Go may move")
		}
		if mem.Index != 0 {
			return fmt.Errorf("an indexed frame access, whose range the check cannot bound")
		}
		d, err := m.frameDisp(mem, in.MemBytes)
		if err != nil {
			return err
		}
		mm := mem
		mm.Base, mm.Disp = x86asm.RSP, d
		want = &mm
		if u, ok := alignedToUnaligned[in.Op]; ok {
			in.Op, remapped = u, true
		}
		if in.MemBytes > 8 && !unalignedOK[in.Op] {
			return fmt.Errorf("a %d-byte frame access that may need alignment the Go frame lacks", in.MemBytes)
		}
	case hasMem && frameRegs[mem.Base]:
		return fmt.Errorf("addresses memory from a narrow frame register")
	}
	b, err := m.next("an instruction")
	if err != nil {
		return err
	}
	if hasMem && mem.Base == x86asm.RIP {
		bm, _, ok := memOf(b.in)
		if !ok || bm.Base != x86asm.RIP {
			return fmt.Errorf("generated %v at %#x instead", b.in, b.at)
		}
		if got := uint64(int64(b.at) + int64(b.in.Len) + bm.Disp); got != target {
			return fmt.Errorf("generated %v at %#x points to %#x, the reference should point to %#x", b.in, b.at, got, target)
		}
		w := mem
		w.Disp = bm.Disp
		want = &w
	}
	if !sameMeaning(in, b.in, want, remapped) {
		return fmt.Errorf("generated %v at %#x instead", b.in, b.at)
	}
	return nil
}

// relIn returns the relocation inside instruction x of section sec, with its
// offset from the instruction's start.
func (o *objFile) relIn(sec int, x insn) *relAt {
	for k := 0; k < x.in.Len; k++ {
		if r, ok := o.rel[sec][x.at+uint64(k)]; ok {
			return &relAt{r, k}
		}
	}
	return nil
}

type relAt struct {
	objReloc
	off int
}

// relTarget is where a PC-relative relocation inside x points: the symbol
// plus the addend, corrected from the relocated field to the end of x.
func (o *objFile) relTarget(x insn, r *relAt) (loc, string) {
	return loc{int(r.sym.Section), int64(r.sym.Value) + r.add + int64(x.in.Len-r.off)}, r.sym.Name
}

// frameDisp maps a frame access of llc's code to the Go frame and checks the
// access lies inside the locals: RBP-relative below the saved registers, or
// RSP-relative inside the reserved area.
func (m *matcher) frameDisp(mem x86asm.Mem, width int) (int64, error) {
	r := m.r
	saved := int64(8 * len(r.Pushed))
	local := int64(r.Local)
	w := int64(width)
	if w <= 0 {
		return 0, fmt.Errorf("frame access of unknown width")
	}
	if mem.Base == x86asm.RBP {
		d := mem.Disp
		if d >= -saved || d < -(saved+local) || d+w > -saved {
			return 0, fmt.Errorf("frame access %d(RBP) of %d bytes outside the %d-byte locals", d, w, local)
		}
		return int64(r.LocalBase) + saved + local + d, nil
	}
	d := mem.Disp
	if d < 0 || d+w > local {
		return 0, fmt.Errorf("stack access %d(RSP) of %d bytes outside the %d-byte locals", d, w, local)
	}
	return int64(r.LocalBase) + d, nil
}

// llcIs checks that a place llc's object reaches (through a relocation
// against sym, or resolved inside the object when sym is nil) is label plus
// add: the same section and offset, or for a symbol llc's object leaves
// undefined (a host function) the same name and addend.
func (o *objFile) llcIs(at loc, sym *elf.Symbol, label string, add int64) error {
	if sym != nil && sym.Section == elf.SHN_UNDEF {
		if sym.Name != label || at.off != add {
			return fmt.Errorf("llc's operand is %s%+d, not %s%+d as declared", sym.Name, at.off, label, add)
		}
		return nil
	}
	lab, err := o.label(label)
	if err != nil {
		return err
	}
	if want := (loc{int(lab.Section), int64(lab.Value) + add}); at != want {
		return fmt.Errorf("llc's operand reaches %v, not %s%+d (%v) as declared", at, label, add, want)
	}
	return nil
}

// dataRef resolves one RIP-relative operand: in llc's object through its
// relocation, in the binary through where the lowering declared it points.
// It returns the absolute address the generated instruction must reach.
func (m *matcher) dataRef(x insn, r *relAt, mem x86asm.Mem) (uint64, error) {
	k := m.k
	if m.refs >= len(m.r.Refs) {
		return 0, fmt.Errorf("a RIP-relative reference the lowering did not declare")
	}
	ref := m.r.Refs[m.refs]
	m.refs++
	var at loc
	var sym *elf.Symbol
	if r != nil {
		if r.typ != elf.R_X86_64_PC32 {
			return 0, fmt.Errorf("a %v relocation in a RIP-relative operand", r.typ)
		}
		at, _ = k.llc.relTarget(x, r)
		sym = &r.sym
	} else {
		at = loc{k.llc.text, int64(x.at) + int64(x.in.Len) + mem.Disp}
	}
	if err := k.llc.llcIs(at, sym, ref.Label, ref.Add); err != nil {
		return 0, err
	}
	switch {
	case ref.Func:
		if x.in.Op != x86asm.LEA {
			return 0, fmt.Errorf("reads a function's code")
		}
		if sym == nil || sym.Section != elf.SHN_UNDEF {
			if lab, _ := k.llc.label(ref.Label); int(lab.Section) != k.llc.text || elf.ST_TYPE(lab.Info) != elf.STT_FUNC {
				return 0, fmt.Errorf("%s is not a function of llc's object", ref.Label)
			}
		}
		a, err := k.funcAddr(ref.Label)
		if err != nil {
			return 0, err
		}
		return uint64(int64(a) + ref.Add), nil
	case ref.Blob:
		if x.in.Op != x86asm.LEA {
			return 0, fmt.Errorf("reads engine_rodata without LEA")
		}
		off, ok := k.c.Plan.BlobOff[ref.Label]
		if !ok {
			return 0, fmt.Errorf("%s is not in engine_rodata", ref.Label)
		}
		rs, ok := k.bin.syms[k.c.Pkg+"."+RodataGo]
		if !ok {
			return 0, fmt.Errorf("no engine_rodata in the binary")
		}
		return uint64(int64(rs.Value) + int64(off) + ref.Add), nil
	}
	label := k.c.Plan.Rename[m.r.Name][ref.Label]
	ls, ok := k.low.byName[label]
	if label == "" || !ok {
		return 0, fmt.Errorf("%s has no copy in the body", ref.Label)
	}
	if x.in.Op != x86asm.LEA {
		lab, err := k.llc.label(ref.Label)
		if err != nil {
			return 0, err
		}
		n, err := k.llc.extent(lab)
		if err != nil {
			return 0, err
		}
		if ref.Add < 0 || ref.Add+int64(x.in.MemBytes) > int64(n) {
			return 0, fmt.Errorf("reads %d bytes at %s%+d, outside its %d bytes", x.in.MemBytes, ref.Label, ref.Add, n)
		}
	}
	return uint64(int64(m.body) + int64(ls.Value-m.lowSym.Value) + ref.Add), nil
}

// ret matches a return: the result stored into the Go result slot, then a
// jump to the Go epilogue (checked once the body is done).
func (m *matcher) ret() error {
	regs, _, err := argWords(m.r.Sig, m.r.PreserveNone)
	if err != nil {
		return err
	}
	off := m.argBase + int64(8*len(regs))
	for w, s := range results(m.r.Sig.Ret) {
		b, err := m.next("a result store")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, s.r, off+int64(8*w), s.sse) {
			return fmt.Errorf("return: %v at %#x is not the store of %v to the result slot", b.in, b.at, s.r)
		}
	}
	b, err := m.next("the jump to the epilogue")
	if err != nil {
		return err
	}
	rel, ok := b.in.Args[0].(x86asm.Rel)
	if b.in.Op != x86asm.JMP || !ok {
		return fmt.Errorf("return: %v at %#x is not a jump to the epilogue", b.in, b.at)
	}
	m.rets = append(m.rets, uint64(int64(b.at)+int64(b.in.Len)+int64(rel)))
	return nil
}

// call matches one call against the next declared call.
func (m *matcher) call(x insn, r *relAt) error {
	k := m.k
	if m.calls >= len(m.r.Calls) {
		return fmt.Errorf("a call the lowering did not declare")
	}
	c := m.r.Calls[m.calls]
	m.calls++
	switch a := x.in.Args[0].(type) {
	case x86asm.Rel:
		if c.Kind != CallUnified && c.Kind != CallLeaf {
			return fmt.Errorf("llc calls directly, the lowering declared %+v", c)
		}
		var at loc
		var sym *elf.Symbol
		if r != nil {
			if r.typ != elf.R_X86_64_PLT32 && r.typ != elf.R_X86_64_PC32 {
				return fmt.Errorf("a call with a %v relocation", r.typ)
			}
			at, _ = k.llc.relTarget(x, r)
			sym = &r.sym
		} else {
			at = loc{k.llc.text, int64(x.at) + int64(x.in.Len) + int64(a)}
		}
		// The target is compared by place, so a call through an alias
		// (musl's weak names) matches the function it names.
		if err := k.llc.llcIs(at, sym, c.Target, 0); err != nil {
			return fmt.Errorf("llc's call is not to %s as declared: %w", c.Target, err)
		}
		if sym == nil || sym.Section != elf.SHN_UNDEF {
			if lab, _ := k.llc.label(c.Target); int(lab.Section) != k.llc.text || elf.ST_TYPE(lab.Info) != elf.STT_FUNC {
				return fmt.Errorf("the call target %s is not a function of llc's object", c.Target)
			}
		}
		name := c.Target
		addr, err := k.funcAddr(name)
		if err != nil {
			return err
		}
		direct := func(b insn) error {
			rel, ok := b.in.Args[0].(x86asm.Rel)
			if b.in.Op != x86asm.CALL || !ok || uint64(int64(b.at)+int64(b.in.Len)+int64(rel)) != addr {
				return fmt.Errorf("generated %v at %#x is not a call to %s", b.in, b.at, name)
			}
			return nil
		}
		if c.Kind == CallLeaf {
			if !k.c.Leaves[name] {
				return fmt.Errorf("a direct call to %s kept unmarshalled, which is not a register-entry leaf", name)
			}
			b, err := m.next("a leaf call")
			if err != nil {
				return err
			}
			return direct(b)
		}
		if k.c.Leaves[name] {
			return fmt.Errorf("a call to the leaf %s marshalled as a unified one", name)
		}
		sig, ok := k.c.Sigs[name]
		if !ok || !sameSig(sig, c.Sig) {
			return fmt.Errorf("call to %s marshalled as %v, its signature is %v", name, c.Sig, sig)
		}
		return m.marshal(c.Sig, direct)
	case x86asm.Reg:
		if c.Kind != CallReg || gpRegs[c.Target] != a {
			return fmt.Errorf("llc calls through %v, the lowering declared %+v", a, c)
		}
		return m.marshal(c.Sig, func(b insn) error {
			if b.in.Op != x86asm.CALL || !isReg(b.in.Args[0], a) {
				return fmt.Errorf("generated %v at %#x is not a call through %v", b.in, b.at, a)
			}
			return nil
		})
	case x86asm.Mem:
		if c.Kind != CallMem {
			return fmt.Errorf("llc calls through memory, the lowering declared %+v", c)
		}
		regs, _, err := argWords(c.Sig, m.r.PreserveNone)
		if err != nil {
			return err
		}
		for _, rg := range regs {
			if rg == x86asm.R11 {
				return fmt.Errorf("R11 carries an argument of this call, so it cannot hold the target")
			}
		}
		want := a
		if frameRegs[a.Index] || a.Base == x86asm.RIP || a.Base == 0 || r != nil {
			return fmt.Errorf("a call through memory the lowering cannot take")
		}
		if a.Base == x86asm.RBP || a.Base == x86asm.RSP {
			d, err := m.frameDisp(a, 8)
			if err != nil {
				return err
			}
			want.Base, want.Disp = x86asm.RSP, d
		} else if frameRegs[a.Base] {
			return fmt.Errorf("a call through memory addressed by a narrow frame register")
		}
		b, err := m.next("the load of the call target")
		if err != nil {
			return err
		}
		bm, ok := b.in.Args[1].(x86asm.Mem)
		if b.in.Op != x86asm.MOV || !isReg(b.in.Args[0], x86asm.R11) || !ok || normMem(bm) != normMem(want) || b.in.DataSize != 64 {
			return fmt.Errorf("generated %v at %#x is not the load of the target into R11", b.in, b.at)
		}
		return m.marshal(c.Sig, func(b insn) error {
			if b.in.Op != x86asm.CALL || !isReg(b.in.Args[0], x86asm.R11) {
				return fmt.Errorf("generated %v at %#x is not a call through R11", b.in, b.at)
			}
			return nil
		})
	}
	return fmt.Errorf("a call form the check does not know")
}

func sameSig(a, b Sig) bool {
	if a.Ret != b.Ret || len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		if a.Params[i] != b.Params[i] {
			return false
		}
	}
	return true
}

// marshal matches the glue around a unified call: the saves, a store of each
// argument word into the outgoing area, the call, the result and the
// restores.
func (m *matcher) marshal(sig Sig, call func(insn) error) error {
	r := m.r
	for k, s := range r.Saved {
		b, err := m.next("a register save")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, gpRegs[s], int64(r.SaveBase+8*k), false) {
			return fmt.Errorf("call: %v at %#x is not the save of %s", b.in, b.at, s)
		}
	}
	regs, sse, err := argWords(sig, r.PreserveNone)
	if err != nil {
		return err
	}
	if 8*(len(regs)+retWords(sig.Ret)) > r.SaveBase {
		return fmt.Errorf("call: the %d-word argument area reaches the saved registers at %d", len(regs)+retWords(sig.Ret), r.SaveBase)
	}
	for w := range regs {
		b, err := m.next("an argument move")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, regs[w], int64(8*w), sse[w]) {
			return fmt.Errorf("call: %v at %#x is not the move of argument word %d from %v", b.in, b.at, w, regs[w])
		}
	}
	b, err := m.next("the call")
	if err != nil {
		return err
	}
	if err := call(b); err != nil {
		return err
	}
	off := int64(8 * len(regs))
	for w, x := range results(sig.Ret) {
		b, err := m.next("a result load")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, false, x.r, off+int64(8*w), x.sse) {
			return fmt.Errorf("call: %v at %#x is not the load of the result into %v", b.in, b.at, x.r)
		}
	}
	for k, s := range r.Saved {
		b, err := m.next("a register restore")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, false, gpRegs[s], int64(r.SaveBase+8*k), false) {
			return fmt.Errorf("call: %v at %#x is not the restore of %s", b.in, b.at, s)
		}
	}
	return nil
}

// sameLabels checks that every other name of a data object (an alias) is,
// in llc's object, the same place as its label, and that the plan sends it
// where it sends the label (placed reports that).
func (k *checker) sameLabels(o *DataObj, lab elf.Symbol, placed func(string) bool) error {
	for _, a := range o.Aliases {
		al, err := k.llc.label(a)
		if err != nil {
			return err
		}
		if al.Section != lab.Section || al.Value != lab.Value {
			return fmt.Errorf("%s is not at %s in llc's object", a, o.Label)
		}
		if !placed(a) {
			return fmt.Errorf("the plan places %s apart from %s", a, o.Label)
		}
	}
	return nil
}

// bodyData checks the data the function carries after its code (constant
// pools, jump tables, copies of data it reads) against llc's object. A jump
// table entry is a difference from the table's start; it must reach, in the
// binary, the instruction llc's entry reaches.
func (m *matcher) bodyData() error {
	k := m.k
	rename := k.c.Plan.Rename[m.r.Name]
	owned := map[string]bool{}
	_, codeLen, _ := k.bodyOf(m.r.Name)
	for _, o := range k.c.Plan.InBody[m.r.Name] {
		label := rename[o.Label]
		ls, ok := k.low.byName[label]
		if !ok {
			return fmt.Errorf("body data %s absent from the re-assembled object", label)
		}
		lab, err := k.llc.label(o.Label)
		if err != nil {
			return err
		}
		if err := k.sameLabels(o, lab, func(a string) bool { return rename[a] == label }); err != nil {
			return err
		}
		owned[o.Label] = true
		for _, a := range o.Aliases {
			owned[a] = true
		}
		n, err := k.llc.extent(lab)
		if err != nil {
			return err
		}
		// The data sits after the code, inside the function's symbol.
		if ls.Value < m.lowSym.Value+codeLen || ls.Value+uint64(n) > m.lowSym.Value+m.lowSym.Size {
			return fmt.Errorf("body data %s lies outside the function's data area", o.Label)
		}
		at := m.body + (ls.Value - m.lowSym.Value)
		got, err := k.bin.read(at, n)
		if err != nil {
			return err
		}
		si := int(lab.Section)
		want := k.llc.data[si][lab.Value : lab.Value+uint64(n)]
		for i := 0; i < n; {
			r, rel := k.llc.rel[si][lab.Value+uint64(i)]
			if !rel {
				if got[i] != want[i] {
					return fmt.Errorf("body data %s: byte %d is %#02x in the binary, %#02x in llc's object", o.Label, i, got[i], want[i])
				}
				i++
				continue
			}
			if r.typ != elf.R_X86_64_PC32 || int(r.sym.Section) != k.llc.text || i+4 > n {
				return fmt.Errorf("body data %s: a %v relocation at byte %d", o.Label, r.typ, i)
			}
			lt := uint64(int64(r.sym.Value) + r.add - int64(i))
			li, ok := m.L.at[lt]
			if !ok || li >= len(m.L.ins) || m.bStart[li] < 0 {
				return fmt.Errorf("body data %s: entry %d reaches %#x, no instruction of the body", o.Label, i/4, lt)
			}
			v := int32(binary.LittleEndian.Uint32(got[i:]))
			bt := uint64(int64(at) + int64(v))
			if bi, ok := m.B.at[bt]; !ok || bi != m.bStart[li] {
				return fmt.Errorf("body data %s: entry %d reaches %#x in the binary, not where llc's %#x went", o.Label, i/4, bt, lt)
			}
			i += 4
		}
		k.rep.Checked["body data objects"]++
	}
	for l := range rename {
		if !owned[l] {
			return fmt.Errorf("the body's label for %s belongs to none of its data objects", l)
		}
	}
	return nil
}

// ---- register-entry leaves ----

func (k *checker) checkLeaf(meta Meta) error {
	r := meta.Rec
	L, _, err := k.llcFunc(r.Name)
	if err != nil {
		return err
	}
	bsym, _ := k.bin.abi0(k.c.Pkg, k.c.Names[r.Name])
	g, err := k.goPrologue(bsym.Value)
	if err != nil {
		return err
	}
	if g.split || g.frame != int64(r.Frame) || g.bp != (r.Frame > 0) {
		return fmt.Errorf("Go prologue of a leaf: split %v, BP %v, frame %d (declared %d)", g.split, g.bp, g.frame, r.Frame)
	}
	body := alignUp64(g.afterLead, 16)
	if err := k.allPad(g.afterLead, body); err != nil {
		return err
	}
	lowSym, codeLen, err := k.bodyOf(r.Name)
	if err != nil {
		return err
	}
	code, err := k.bin.read(body, int(codeLen))
	if err != nil {
		return err
	}
	B, err := decodeStream(code, body)
	if err != nil {
		return err
	}
	e := body + lowSym.Size
	if err := k.goEpilogue(g, e, bsym); err != nil {
		return err
	}
	m := &matcher{k: k, r: r, L: L, B: B, bStart: make([]int, len(L.ins)), body: body, lowSym: lowSym}
	for i := range m.bStart {
		m.bStart[i] = -1
	}
	n := len(r.Pushed)
	slot := map[x86asm.Reg]int64{}
	i := 0
	for ; i < n; i++ {
		reg := gpRegs[r.Pushed[i]]
		if i >= len(L.ins) || L.ins[i].in.Op != x86asm.PUSH || !isReg(L.ins[i].in.Args[0], reg) || !calleeSavedRegs[reg] {
			return fmt.Errorf("llc's leaf does not push %s first", r.Pushed[i])
		}
		slot[reg] = int64(8 * (n - 1 - i))
		b, err := m.next("a register save")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, reg, slot[reg], false) {
			return fmt.Errorf("leaf: %v at %#x is not the save of %s", b.in, b.at, r.Pushed[i])
		}
	}
	for ; i < len(L.ins); i++ {
		m.bStart[i] = m.j
		x := L.ins[i]
		switch x.in.Op {
		case x86asm.POP:
			reg, _ := x.in.Args[0].(x86asm.Reg)
			off, ok := slot[reg]
			if !ok {
				return fmt.Errorf("llc's leaf pops %v, which it did not push", x.in.Args[0])
			}
			b, err := m.next("a register restore")
			if err != nil {
				return err
			}
			if !isStackMove(b.in, false, reg, off, false) {
				return fmt.Errorf("leaf: %v at %#x is not the restore of %v", b.in, b.at, reg)
			}
			continue
		case x86asm.RET:
			b, err := m.next("the jump to the epilogue")
			if err != nil {
				return err
			}
			rel, ok := b.in.Args[0].(x86asm.Rel)
			if b.in.Op != x86asm.JMP || !ok {
				return fmt.Errorf("leaf return: %v at %#x is not a jump to the epilogue", b.in, b.at)
			}
			m.rets = append(m.rets, uint64(int64(b.at)+int64(b.in.Len)+int64(rel)))
			continue
		case x86asm.CALL:
			if err := m.call(x, k.llc.relIn(k.llc.text, x)); err != nil {
				return fmt.Errorf("llc instruction at %#x (%v): %w", x.at, x.in, err)
			}
			if c := r.Calls[m.calls-1]; c.Kind != CallLeaf {
				return fmt.Errorf("a leaf makes a %v call", c.Kind)
			}
			continue
		case x86asm.PUSH:
			return fmt.Errorf("llc's leaf pushes at %#x", x.at)
		}
		if mem, _, ok := memOf(x.in); ok && (frameRegs[mem.Base] || mem.Base == x86asm.RIP) {
			return fmt.Errorf("llc's leaf at %#x addresses memory from %v", x.at, mem.Base)
		}
		if err := m.one(x); err != nil {
			return fmt.Errorf("llc instruction at %#x (%v): %w", x.at, x.in, err)
		}
	}
	if m.j != len(B.ins) {
		return fmt.Errorf("generated instruction at %#x explained by no llc instruction: %v", B.ins[m.j].at, B.ins[m.j].in)
	}
	for _, t := range m.rets {
		if bi, ok := B.at[t]; ok && bi < len(B.ins) {
			t = B.ins[bi].at
		}
		if t != e {
			return fmt.Errorf("a return jumps to %#x, not to the Go epilogue", t)
		}
	}
	for _, br := range m.branches {
		li, ok := L.at[br.llcTarget]
		if !ok || li >= len(L.ins) || m.bStart[li] < 0 {
			return fmt.Errorf("branch at %#x: llc's target %#x is no instruction of the body", br.at, br.llcTarget)
		}
		if bi, ok := B.at[br.binTarget]; !ok || bi != m.bStart[li] {
			return fmt.Errorf("branch at %#x goes to %#x, not to where llc's target %#x went", br.at, br.binTarget, br.llcTarget)
		}
	}
	if m.calls != len(r.Calls) {
		return fmt.Errorf("the lowering declared %d calls, llc's object has %d", len(r.Calls), m.calls)
	}
	return nil
}

// ---- stubs ----

// checkStub confirms a function the generator could not lower reports its
// index: MOVQ $i, 0(SP); CALL goasmMissing.
func (k *checker) checkStub(meta Meta, idx int) error {
	bsym, _ := k.bin.abi0(k.c.Pkg, k.c.Names[meta.Name])
	g, err := k.goPrologue(bsym.Value)
	if err != nil {
		return err
	}
	if !g.split || !g.bp || g.frame != 8 {
		return fmt.Errorf("Go prologue of a stub: split %v, BP %v, frame %d", g.split, g.bp, g.frame)
	}
	a := g.afterLead
	in, err := k.dec(a)
	if err != nil {
		return err
	}
	m, ok := in.Args[0].(x86asm.Mem)
	imm, ok2 := in.Args[1].(x86asm.Imm)
	if in.Op != x86asm.MOV || !ok || !ok2 || normMem(m) != (x86asm.Mem{Base: x86asm.RSP}) || int64(imm) != int64(idx) || in.DataSize != 64 {
		return fmt.Errorf("stub does not pass its index %d: %v", idx, in)
	}
	a += uint64(in.Len)
	if in, err = k.dec(a); err != nil {
		return err
	}
	miss, ok := k.bin.abi0(k.c.Pkg, "goasmMissing")
	rel, ok2 := in.Args[0].(x86asm.Rel)
	if !ok || in.Op != x86asm.CALL || !ok2 || uint64(int64(a)+int64(in.Len)+int64(rel)) != miss.Value {
		return fmt.Errorf("stub does not call goasmMissing: %v", in)
	}
	return k.goEpilogue(g, a+uint64(in.Len), bsym)
}

// ---- engine_rodata ----

// checkRodata compares engine_rodata in the binary with llc's object: every
// blob object's bytes, every pointer (an absolute address the linker wrote,
// which must be the same llc label plus offset, mapped into the binary) and
// every resolved difference of labels. Bytes no object covers must be zero,
// and every pointer and difference the plan declared must belong to an
// object llc's object has one in.
func (k *checker) checkRodata() error {
	p := k.c.Plan
	rs, ok := k.bin.syms[k.c.Pkg+"."+RodataGo]
	if !ok {
		if len(p.Blob) == 0 {
			return nil
		}
		return fmt.Errorf("no engine_rodata in the binary")
	}
	if int(rs.Size) < len(p.Blob) {
		return fmt.Errorf("engine_rodata is %d bytes in the binary, %d planned", rs.Size, len(p.Blob))
	}
	blob, err := k.bin.read(rs.Value, int(rs.Size))
	if err != nil {
		return err
	}
	relocs := map[int]BlobReloc{}
	for _, r := range p.Relocs {
		relocs[r.Off] = r
	}
	diffs := map[int]BlobDiff{}
	for _, d := range p.Diffs {
		diffs[d.Off] = d
	}
	used := map[int]bool{}
	owned := map[string]bool{}
	covered := make([]bool, len(blob))
	for _, o := range k.c.Unit.DataOrder {
		boff, ok := p.BlobOff[o.Label]
		if !ok {
			continue
		}
		lab, err := k.llc.label(o.Label)
		if err != nil {
			return err
		}
		if err := k.sameLabels(o, lab, func(a string) bool { off, ok := p.BlobOff[a]; return ok && off == boff }); err != nil {
			return err
		}
		owned[o.Label] = true
		for _, a := range o.Aliases {
			owned[a] = true
		}
		n, err := k.llc.extent(lab)
		if err != nil {
			return err
		}
		if boff < 0 || boff+n > len(blob) {
			return fmt.Errorf("data %s runs past engine_rodata", o.Label)
		}
		si := int(lab.Section)
		want := k.llc.data[si]
		if want == nil || int(lab.Value)+n > len(want) {
			return fmt.Errorf("data %s: no bytes in llc's object", o.Label)
		}
		for i := 0; i < n; {
			bo := boff + i
			lo := lab.Value + uint64(i)
			width, err := k.blobSlot(blob, rs.Value, bo, si, lo, relocs, diffs)
			if err != nil {
				return fmt.Errorf("data %s, byte %d: %w", o.Label, i, err)
			}
			if i+width > n {
				return fmt.Errorf("data %s, byte %d: a %d-byte value runs past the object", o.Label, i, width)
			}
			if _, ok := relocs[bo]; ok {
				used[bo] = true
			}
			if _, ok := diffs[bo]; ok {
				used[bo] = true
			}
			for w := 0; w < width; w++ {
				if covered[bo+w] {
					return fmt.Errorf("data %s overlaps another object in engine_rodata at %d", o.Label, bo+w)
				}
				covered[bo+w] = true
			}
			i += width
		}
		k.rep.Checked["engine_rodata objects"]++
	}
	for l := range p.BlobOff {
		if !owned[l] {
			return fmt.Errorf("engine_rodata: the label %s belongs to none of its objects", l)
		}
	}
	for off := range relocs {
		if !used[off] {
			return fmt.Errorf("engine_rodata: a planned pointer at %d belongs to no object llc relocates there", off)
		}
	}
	for off := range diffs {
		if !used[off] {
			return fmt.Errorf("engine_rodata: a planned difference at %d belongs to no object", off)
		}
	}
	for i, c := range covered {
		if !c && blob[i] != 0 {
			return fmt.Errorf("engine_rodata: byte %d (%#02x) belongs to no object", i, blob[i])
		}
	}
	return nil
}

// blobSlot checks the value at blob offset bo against llc's section si at
// lo, and returns how many bytes it covered: 8 for a pointer, the size of a
// difference, else one literal byte.
func (k *checker) blobSlot(blob []byte, base uint64, bo, si int, lo uint64, relocs map[int]BlobReloc, diffs map[int]BlobDiff) (int, error) {
	p := k.c.Plan
	r, rel := k.llc.rel[si][lo]
	br, isPtr := relocs[bo]
	d, isDiff := diffs[bo]
	switch {
	case rel && r.typ == elf.R_X86_64_64:
		if !isPtr {
			return 0, fmt.Errorf("llc relocates it, the plan holds no pointer there")
		}
		if err := k.llc.llcIs(loc{int(r.sym.Section), int64(r.sym.Value) + r.add}, &r.sym, br.Label, br.LabelAdd); err != nil {
			return 0, err
		}
		var want uint64
		if br.Func {
			a, err := k.funcAddr(br.Label)
			if err != nil {
				return 0, err
			}
			want = uint64(int64(a) + br.LabelAdd)
		} else {
			lb, ok := p.BlobOff[br.Label]
			if !ok {
				return 0, fmt.Errorf("points to %s, which is not in engine_rodata", br.Label)
			}
			want = uint64(int64(base) + int64(lb) + br.LabelAdd)
		}
		if got := binary.LittleEndian.Uint64(blob[bo:]); got != want {
			return 0, fmt.Errorf("the pointer is %#x in the binary, should be %#x (%s%+d)", got, want, br.Label, br.LabelAdd)
		}
		return 8, nil
	case rel && r.typ == elf.R_X86_64_PC32:
		if !isDiff || d.Size != 4 {
			return 0, fmt.Errorf("llc relocates it PC-relatively, the plan holds no 4-byte difference there")
		}
		pos, err := k.llc.label(d.Pos)
		if err != nil {
			return 0, err
		}
		neg := int64(lo)
		if d.Neg != "." {
			nl, err := k.llc.label(d.Neg)
			if err != nil {
				return 0, err
			}
			if int(nl.Section) != si {
				return 0, fmt.Errorf("a difference from %s, in another section", d.Neg)
			}
			neg = int64(nl.Value)
		}
		// S + A - P = Pos + K - Neg, with the slot P and Neg in this section.
		if int(r.sym.Section) != int(pos.Section) || int64(r.sym.Value)+r.add != int64(pos.Value)+d.K+(int64(lo)-neg) {
			return 0, fmt.Errorf("llc's entry is not %s-%s%+d", d.Pos, d.Neg, d.K)
		}
		return 4, k.blobDiff(blob, bo, d)
	case rel:
		return 0, fmt.Errorf("a %v relocation", r.typ)
	case isPtr:
		return 0, fmt.Errorf("the plan holds a pointer where llc has none")
	case isDiff:
		// llc resolved the difference itself: both labels in one section.
		pos, err := k.llc.label(d.Pos)
		if err != nil {
			return 0, err
		}
		neg := int64(lo)
		if d.Neg != "." {
			nl, err := k.llc.label(d.Neg)
			if err != nil {
				return 0, err
			}
			if nl.Section != pos.Section {
				return 0, fmt.Errorf("a difference of %s and %s, in two sections, that llc did not relocate", d.Pos, d.Neg)
			}
			neg = int64(nl.Value)
		} else if int(pos.Section) != si {
			return 0, fmt.Errorf("a difference of %s and a place in another section that llc did not relocate", d.Pos)
		}
		if int(lo)+d.Size > len(k.llc.data[si]) {
			return 0, fmt.Errorf("a difference past the section")
		}
		for b := 1; b < d.Size; b++ {
			if _, r := k.llc.rel[si][lo+uint64(b)]; r {
				return 0, fmt.Errorf("a relocation inside a difference")
			}
		}
		if lv, v := readInt(k.llc.data[si][lo:], d.Size), int64(pos.Value)-neg+d.K; lv != v {
			return 0, fmt.Errorf("llc's value is %d, %s-%s%+d is %d", lv, d.Pos, d.Neg, d.K, v)
		}
		return d.Size, k.blobDiff(blob, bo, d)
	}
	if blob[bo] != k.llc.data[si][lo] {
		return 0, fmt.Errorf("%#02x in the binary, %#02x in llc's object", blob[bo], k.llc.data[si][lo])
	}
	return 1, nil
}

// blobDiff checks a resolved difference in the linked blob: the distance of
// its two labels in engine_rodata plus its constant.
func (k *checker) blobDiff(blob []byte, bo int, d BlobDiff) error {
	p := k.c.Plan
	pos, ok1 := p.BlobOff[d.Pos]
	neg, ok2 := p.BlobOff[d.Neg]
	if d.Neg == "." {
		neg, ok2 = bo, true
	}
	if !ok1 || !ok2 {
		return fmt.Errorf("the difference %s-%s names data outside engine_rodata", d.Pos, d.Neg)
	}
	if got, want := readInt(blob[bo:], d.Size), int64(pos-neg)+d.K; got != want {
		return fmt.Errorf("the difference is %d in the binary, %s-%s%+d is %d", got, d.Pos, d.Neg, d.K, want)
	}
	return nil
}

func readInt(b []byte, n int) int64 {
	switch n {
	case 1:
		return int64(int8(b[0]))
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(b)))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(b)))
	}
	return int64(binary.LittleEndian.Uint64(b))
}
