// Command conv turns LLVM aarch64 output into Go arm64 assembly.
//
//	conv lower engine.s spec.json lowered.s frames.json
//	conv emit lowered.o spec.json frames.json gen_arm64.s decl.go
//
// lower edits the GNU assembly text: it removes LLVM's SP moves, moves every
// SP offset up by 16 (0(RSP) is Go's saved LR, 8(RSP) is padding), turns each
// ret into a branch to the function end, and rejects reserved registers and
// unknown stack use. llvm-mc assembles the result. emit writes the machine
// code as WORD directives, each call as a symbolic BL, the arguments loaded
// from FP after Go's prologue, Go's RET, the call stubs and the Go
// declarations.
package main

import (
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type sig struct {
	Go   string   `json:"go"`
	Args []string `json:"args"`
	Ret  string   `json:"ret"`
}

type spec struct {
	Functions map[string]sig `json:"functions"`
	Callbacks map[string]sig `json:"callbacks"`
}

type frame struct {
	Leaf  bool     `json:"leaf"`
	Size  int      `json:"size"`  // LLVM frame bytes
	Calls []string `json:"calls"` // C callees
}

func main() {
	if len(os.Args) < 2 {
		fail("usage")
	}
	switch os.Args[1] {
	case "lower":
		lower(os.Args[2], os.Args[3], os.Args[4], os.Args[5])
	case "emit":
		emit(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6])
	default:
		fail("unknown command")
	}
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "conv: "+f+"\n", a...)
	os.Exit(1)
}

func readSpec(p string) spec {
	var s spec
	b, err := os.ReadFile(p)
	if err != nil {
		fail("%v", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		fail("%v", err)
	}
	return s
}

var (
	reForbidden = regexp.MustCompile(`\b([xw](18|26|27|28|29)|fp)\b`)
	reLR        = regexp.MustCompile(`\b(x30|w30|lr)\b`)
	reSP        = regexp.MustCompile(`\bsp\b`)
	rePre       = regexp.MustCompile(`^(stp|str)\s+(.+), \[sp, #-(\d+)\]!$`)
	rePost      = regexp.MustCompile(`^(ldp|ldr)\s+(.+), \[sp\], #(\d+)$`)
	reSub       = regexp.MustCompile(`^sub\s+sp, sp, #(\d+)$`)
	reAdd       = regexp.MustCompile(`^add\s+sp, sp, #(\d+)$`)
	reMemSP     = regexp.MustCompile(`\[sp(, #(\d+))?\]`)
	reBL        = regexp.MustCompile(`^bl\s+(\S+)$`)
	reB         = regexp.MustCompile(`^b\s+(\S+)$`)
)

type fn struct {
	name  string
	lines []string // instructions and local labels
}

func parseFuncs(src string) ([]fn, []string) {
	var fns []fn
	var cur *fn
	var other []string
	for _, raw := range strings.Split(src, "\n") {
		l := raw
		if i := strings.Index(l, "//"); i >= 0 {
			l = l[:i]
		}
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if cur == nil {
			if strings.HasSuffix(l, ":") && !strings.HasPrefix(l, ".") {
				fns = append(fns, fn{name: strings.TrimSuffix(l, ":")})
				cur = &fns[len(fns)-1]
				continue
			}
			other = append(other, l)
			continue
		}
		if strings.HasPrefix(l, ".Lfunc_end") {
			cur = nil
			continue
		}
		if strings.HasPrefix(l, ".") && !strings.HasSuffix(l, ":") {
			fail("%s: directive inside function: %s", cur.name, l)
		}
		cur.lines = append(cur.lines, l)
	}
	return fns, other
}

func lower(in, specPath, out, framesOut string) {
	sp := readSpec(specPath)
	b, err := os.ReadFile(in)
	if err != nil {
		fail("%v", err)
	}
	fns, other := parseFuncs(string(b))
	for _, l := range other {
		ok := strings.HasPrefix(l, ".text") || strings.HasPrefix(l, ".file") || strings.HasPrefix(l, ".globl") ||
			strings.HasPrefix(l, ".p2align") || strings.HasPrefix(l, ".type") || strings.HasPrefix(l, ".size") ||
			strings.HasPrefix(l, ".ident") || strings.HasPrefix(l, ".section\t\".note.GNU-stack\"") ||
			strings.HasPrefix(l, ".addrsig")
		if !ok {
			fail("unreviewed top-level line (data or section): %s", l)
		}
	}
	frames := map[string]frame{}
	var o []string
	o = append(o, ".text")
	for _, f := range fns {
		if _, ok := sp.Functions[f.name]; !ok {
			fail("%s: no signature in spec", f.name)
		}
		leaf := true
		for _, l := range f.lines {
			if reSP.MatchString(l) || reBL.MatchString(l) || reLR.MatchString(l) {
				leaf = false
			}
		}
		fr := frame{Leaf: leaf}
		lines := append([]string(nil), f.lines...)
		if !leaf {
			// Prologue: [stp|str regs, [sp, #-N]!] [sub sp, sp, #M]
			i, pre, preOp, preRegs, m := 0, 0, "", "", 0
			if mm := rePre.FindStringSubmatch(lines[i]); mm != nil {
				preOp, preRegs = mm[1], mm[2]
				pre, _ = strconv.Atoi(mm[3])
				i++
			}
			if mm := reSub.FindStringSubmatch(lines[i]); mm != nil {
				m, _ = strconv.Atoi(mm[1])
				i++
			}
			fr.Size = pre + m
			if fr.Size%16 != 0 {
				fail("%s: frame %d not 16 aligned", f.name, fr.Size)
			}
			var body []string
			if preOp != "" {
				body = append(body, fmt.Sprintf("%s %s, [sp, #%d]", preOp, preRegs, m))
			}
			rest := lines[i:]
			for j := 0; j < len(rest); j++ {
				l := rest[j]
				if l == "ret" {
					// Epilogue: [add sp, sp, #M] [ldp|ldr regs, [sp], #N] ret
					k := len(body)
					if k > 0 {
						if mm := rePost.FindStringSubmatch(body[k-1]); mm != nil {
							n, _ := strconv.Atoi(mm[3])
							if n != pre || mm[1] != map[string]string{"stp": "ldp", "str": "ldr"}[preOp] {
								fail("%s: epilogue pop %d does not match prologue %d", f.name, n, pre)
							}
							body[k-1] = fmt.Sprintf("%s %s, [sp, #%d]", mm[1], mm[2], m)
							k--
						} else if pre != 0 {
							fail("%s: ret without the matching pop", f.name)
						}
					}
					if m != 0 {
						if k == 0 {
							fail("%s: ret without add sp", f.name)
						}
						mm := reAdd.FindStringSubmatch(body[k-1])
						if mm == nil || mm[1] != strconv.Itoa(m) {
							fail("%s: ret not preceded by add sp, sp, #%d: %q", f.name, m, body[k-1])
						}
						body = append(body[:k-1], body[k:]...)
					}
					body = append(body, "b .Lgoret_"+f.name)
					continue
				}
				body = append(body, l)
			}
			lines = body
		} else {
			for j, l := range lines {
				if l == "ret" {
					lines[j] = "b .Lgoret_" + f.name
				}
			}
		}
		// Checks and the SP shift.
		for j, l := range lines {
			if strings.HasSuffix(l, ":") {
				continue
			}
			op := strings.Fields(l)[0]
			switch op {
			case "svc", "hvc", "smc", "mrs", "msr", "blr", "br", "ret", "adr", "adrp":
				fail("%s: forbidden instruction %q", f.name, l)
			}
			if reForbidden.MatchString(l) {
				fail("%s: reserved register in %q", f.name, l)
			}
			if mm := reB.FindStringSubmatch(l); mm != nil && !strings.HasPrefix(mm[1], ".L") {
				fail("%s: tail jump %q", f.name, l)
			}
			if mm := reBL.FindStringSubmatch(l); mm != nil {
				fr.Calls = append(fr.Calls, mm[1])
			}
			if reLR.MatchString(l) && !(strings.HasPrefix(op, "st") || strings.HasPrefix(op, "ld")) {
				fail("%s: LR used as data in %q", f.name, l)
			}
			if reSP.MatchString(l) {
				if !strings.Contains(l, "[sp") || strings.Contains(l, "]!") || regexp.MustCompile(`\], #`).MatchString(l) ||
					len(reSP.FindAllString(l, -1)) != 1 {
					fail("%s: unreviewed stack use %q", f.name, l)
				}
				lines[j] = reMemSP.ReplaceAllStringFunc(l, func(s string) string {
					mm := reMemSP.FindStringSubmatch(s)
					k := 0
					if mm[2] != "" {
						k, _ = strconv.Atoi(mm[2])
					}
					if k < 0 || k >= fr.Size {
						fail("%s: access outside the LLVM frame %q", f.name, l)
					}
					return fmt.Sprintf("[sp, #%d]", k+16)
				})
			}
			if reLR.MatchString(l) && !strings.Contains(l, "[sp") {
				fail("%s: LR outside spill %q", f.name, l)
			}
		}
		frames[f.name] = fr
		o = append(o, ".globl "+f.name, ".p2align 2", ".type "+f.name+",@function", f.name+":")
		o = append(o, lines...)
		o = append(o, ".Lgoret_"+f.name+":", ".size "+f.name+", .Lgoret_"+f.name+"-"+f.name)
	}
	if err := os.WriteFile(out, []byte(strings.Join(o, "\n")+"\n"), 0o644); err != nil {
		fail("%v", err)
	}
	j, _ := json.MarshalIndent(frames, "", "  ")
	if err := os.WriteFile(framesOut, j, 0o644); err != nil {
		fail("%v", err)
	}
}

// Argument layout.

type slot struct {
	name string // Go name
	kind byte   // 'i' word, 'd' double
	reg  string // AAPCS register
	off  int    // FP offset
}

// layout returns the Go stack slots (ABI0, 8 bytes each, JSValue as two
// words) with their AAPCS registers, and the result slots.
func layout(s sig) (args, rets []slot, size int) {
	ngr, nfr, off := 0, 0, 0
	add := func(name string, k byte, out *[]slot, isRet bool) {
		var reg string
		if k == 'd' {
			if isRet {
				reg = fmt.Sprintf("F%d", len(*out))
			} else {
				reg = fmt.Sprintf("F%d", nfr)
				nfr++
			}
		} else {
			if isRet {
				reg = fmt.Sprintf("R%d", len(*out))
			} else {
				reg = fmt.Sprintf("R%d", ngr)
				ngr++
			}
		}
		if ngr > 8 || nfr > 8 {
			fail("too many register arguments")
		}
		*out = append(*out, slot{name, k, reg, off})
		off += 8
	}
	for _, a := range s.Args {
		n, t, _ := strings.Cut(a, ":")
		switch t {
		case "i", "d":
			add(n, t[0], &args, false)
		case "v":
			add(n+"_u", 'i', &args, false)
			add(n+"_tag", 'i', &args, false)
		default:
			fail("type %q", t)
		}
	}
	switch s.Ret {
	case "i", "d":
		add("ret", s.Ret[0], &rets, true)
	case "v":
		add("ret_u", 'i', &rets, true)
		add("ret_tag", 'i', &rets, true)
	case "":
	default:
		fail("type %q", s.Ret)
	}
	return args, rets, off
}

func mov(k byte) string {
	if k == 'd' {
		return "FMOVD"
	}
	return "MOVD"
}

func goDecl(name string, s sig) string {
	args, rets, _ := layout(s)
	typ := func(k byte) string {
		if k == 'd' {
			return "float64"
		}
		return "uint64"
	}
	var a, r []string
	for _, x := range args {
		a = append(a, x.name+" "+typ(x.kind))
	}
	for _, x := range rets {
		r = append(r, x.name+" "+typ(x.kind))
	}
	d := "func " + name + "(" + strings.Join(a, ", ") + ")"
	if len(r) > 0 {
		d += " (" + strings.Join(r, ", ") + ")"
	}
	return d
}

// Callee-saved registers of AAPCS that Go code does not keep: R19-R25 and
// F8-F15. R26-R28 are fixed in C, R29 and R30 are kept by Go's prologue.
var saved = []string{"R19", "R20", "R21", "R22", "R23", "R24", "R25", "F8", "F9", "F10", "F11", "F12", "F13", "F14", "F15"}

// With CONV_NO_SAVES set, the stub skips the save/restore. It is unsafe (Go
// and morestack clobber these registers) and exists only to time the pure
// cost of the 15 saves and restores against the real stub. The benchmark
// checks the result is still correct before trusting such a timing.
func savedRegs() []string {
	if os.Getenv("CONV_NO_SAVES") != "" {
		return nil
	}
	return saved
}

// stub writes a NOSPLIT function with a register entry that keeps the AAPCS
// callee-saved registers, stores the arguments where ABI0 wants them, calls
// target, and loads the results back into registers.
func stub(o *[]string, name, target string, s sig) {
	args, rets, size := layout(s)
	saved := savedRegs()
	saveAt := 8 + size
	frameSize := size + 8*len(saved)
	*o = append(*o, "", fmt.Sprintf("// %s: register entry, calls %s with ABI0 arguments.", name, target),
		fmt.Sprintf("TEXT ·%s(SB), NOSPLIT, $%d-0", name, frameSize), "\tNO_LOCAL_POINTERS")
	for i, r := range saved {
		m := "MOVD"
		if r[0] == 'F' {
			m = "FMOVD"
		}
		*o = append(*o, fmt.Sprintf("\t%s %s, %d(RSP)", m, r, saveAt+8*i))
	}
	for _, a := range args {
		*o = append(*o, fmt.Sprintf("\t%s %s, %d(RSP)", mov(a.kind), a.reg, 8+a.off))
	}
	*o = append(*o, fmt.Sprintf("\tBL ·%s(SB)", target))
	for _, r := range rets {
		*o = append(*o, fmt.Sprintf("\t%s %d(RSP), %s", mov(r.kind), 8+r.off, r.reg))
	}
	for i, r := range saved {
		m := "MOVD"
		if r[0] == 'F' {
			m = "FMOVD"
		}
		*o = append(*o, fmt.Sprintf("\t%s %d(RSP), %s", m, saveAt+8*i, r))
	}
	*o = append(*o, "\tRET")
}

func branchTarget(w uint32, pc int) (int, bool) {
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

func emit(objPath, specPath, framesPath, outS, outGo string) {
	sp := readSpec(specPath)
	var frames map[string]frame
	fb, err := os.ReadFile(framesPath)
	if err != nil {
		fail("%v", err)
	}
	json.Unmarshal(fb, &frames)
	ef, err := elf.Open(objPath)
	if err != nil {
		fail("%v", err)
	}
	if ef.Machine != elf.EM_AARCH64 {
		fail("not aarch64")
	}
	for _, s := range ef.Sections {
		if s.Flags&elf.SHF_WRITE != 0 && s.Size > 0 {
			fail("writable section %s", s.Name)
		}
		if (s.Type == elf.SHT_PROGBITS || s.Type == elf.SHT_NOBITS) && s.Name != ".text" && s.Size > 0 && s.Flags&elf.SHF_ALLOC != 0 {
			fail("unreviewed section %s", s.Name)
		}
	}
	text := ef.Section(".text")
	code, _ := text.Data()
	syms, _ := ef.Symbols()
	calls := map[int]string{}
	for _, s := range ef.Sections {
		if s.Type != elf.SHT_RELA || s.Info != uint32(sectionIndex(ef, text)) {
			continue
		}
		d, _ := s.Data()
		for p := 0; p < len(d); p += 24 {
			off := binary.LittleEndian.Uint64(d[p:])
			info := binary.LittleEndian.Uint64(d[p+8:])
			add := int64(binary.LittleEndian.Uint64(d[p+16:]))
			typ := elf.R_AARCH64(info & 0xffffffff)
			sym := syms[int(info>>32)-1]
			if typ != elf.R_AARCH64_CALL26 || add != 0 {
				fail("relocation %v %s+%d at %#x", typ, sym.Name, add, off)
			}
			calls[int(off)] = sym.Name
		}
	}
	type fsym struct {
		name       string
		start, end int
	}
	var fs []fsym
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) == elf.STT_FUNC {
			fs = append(fs, fsym{s.Name, int(s.Value), int(s.Value + s.Size)})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].start < fs[j].start })

	o := []string{"// Code generated by conv from LLVM output. DO NOT EDIT.", "", "//go:build arm64", "",
		`#include "textflag.h"`, `#include "funcdata.h"`}
	g := []string{"// Code generated by conv. DO NOT EDIT.", "", "package mini", ""}
	var st []string
	stubs := map[string]bool{}
	needStub := func(name, target string, s sig) {
		if !stubs[name] {
			stubs[name] = true
			stub(&st, name, target, s)
		}
	}
	for _, f := range fs {
		fr, ok := frames[f.name]
		if !ok {
			fail("%s: no frame record", f.name)
		}
		s := sp.Functions[f.name]
		args, rets, size := layout(s)
		o = append(o, "")
		if fr.Leaf {
			o = append(o, fmt.Sprintf("// %s: leaf, register entry (AAPCS), no frame.", f.name),
				fmt.Sprintf("TEXT ·%s(SB), NOSPLIT|NOFRAME, $0-0", f.name))
		} else {
			o = append(o, fmt.Sprintf("// %s: unified entry, ABI0 arguments, LLVM frame %d bytes at 16(RSP).", f.name, fr.Size),
				fmt.Sprintf("TEXT ·c_%s(SB), $%d-%d", f.name, fr.Size+8, size), "\tNO_LOCAL_POINTERS")
			for _, a := range args {
				o = append(o, fmt.Sprintf("\t%s %s+%d(FP), %s", mov(a.kind), a.name, a.off, a.reg))
			}
			g = append(g, goDecl("c_"+f.name, s))
		}
		for pc := f.start; pc < f.end; pc += 4 {
			w := binary.LittleEndian.Uint32(code[pc:])
			if t, ok := calls[pc]; ok {
				var target string
				switch {
				case sp.Callbacks[t].Go != "":
					target = "call_" + t
					needStub(target, sp.Callbacks[t].Go, sp.Callbacks[t])
				case frames[t].Leaf:
					target = t
				default:
					if _, ok := frames[t]; !ok {
						fail("%s: call to unknown %s", f.name, t)
					}
					target = "call_" + t
					needStub(target, "c_"+t, sp.Functions[t])
				}
				o = append(o, fmt.Sprintf("\tBL ·%s(SB)", target))
				continue
			}
			if w == 0xd65f03c0 || w&0xfffffc1f == 0xd63f0000 || w&0xfffffc1f == 0xd61f0000 || w&0xfc000000 == 0x94000000 {
				fail("%s: ret, blr, br or bl hidden in bytes at %#x", f.name, pc)
			}
			if t, ok := branchTarget(w, pc); ok && (t < f.start || t > f.end) {
				fail("%s: branch at %#x leaves the function", f.name, pc)
			}
			o = append(o, fmt.Sprintf("\tWORD $0x%08x", w))
		}
		if !fr.Leaf {
			for _, r := range rets {
				o = append(o, fmt.Sprintf("\t%s %s, %s+%d(FP)", mov(r.kind), r.reg, r.name, r.off))
			}
		}
		o = append(o, "\tRET")
		if fr.Leaf {
			// Unified entry of the leaf: ABI0 arguments, calls the register entry.
			o = append(o, "", fmt.Sprintf("// c_%s: unified entry of the leaf %s.", f.name, f.name),
				fmt.Sprintf("TEXT ·c_%s(SB), $0-%d", f.name, size), "\tNO_LOCAL_POINTERS")
			for _, a := range args {
				o = append(o, fmt.Sprintf("\t%s %s+%d(FP), %s", mov(a.kind), a.name, a.off, a.reg))
			}
			o = append(o, fmt.Sprintf("\tBL ·%s(SB)", f.name))
			for _, r := range rets {
				o = append(o, fmt.Sprintf("\t%s %s, %s+%d(FP)", mov(r.kind), r.reg, r.name, r.off))
			}
			o = append(o, "\tRET")
			g = append(g, goDecl("c_"+f.name, s))
		}
	}
	o = append(o, st...)
	if err := os.WriteFile(outS, []byte(strings.Join(o, "\n")+"\n"), 0o644); err != nil {
		fail("%v", err)
	}
	if err := os.WriteFile(outGo, []byte(strings.Join(g, "\n")+"\n"), 0o644); err != nil {
		fail("%v", err)
	}
}

func sectionIndex(f *elf.File, s *elf.Section) int {
	for i, x := range f.Sections {
		if x == s {
			return i
		}
	}
	return -1
}
