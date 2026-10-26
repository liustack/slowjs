// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Read-only data of the whole engine. Go assembly cannot attach a relocation
// to the raw bytes of an instruction, so each data object of llc's text goes
// one of three ways:
//
//   - into the body of the function that uses it, after its code, where the
//     re-assembly resolves every PC-relative reference: constant pools, jump
//     tables and the computed goto table (which hold offsets of the
//     function's own labels), and a copy of each table a function reads
//     with an instruction other than LEA (such as musl's exp data);
//   - into one Go data symbol, engine_rodata, for everything whose address
//     a function takes with LEA or another object holds: strings, function
//     tables, Unicode tables. The lowering turns each LEA of it into a LEA
//     of __goasm_rodata+offset, which emit writes as a Go LEAQ of
//     ·engine_rodata. Pointers inside it become Go DATA relocations, and
//     offsets between two of its objects are resolved here;
//   - nowhere, if nothing uses it.
//
// Anything else is an error: a writable section, an absolute 32-bit address,
// a pointer in data that must sit inside a function, a non-LEA reference to
// data that holds pointers.
package gen

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// BlobReloc is an 8-byte pointer inside engine_rodata: to Sym+Add, where Sym
// is a function (Func) or engine_rodata itself.
type BlobReloc struct {
	Off  int
	Sym  string
	Add  int64
	Func bool
}

// DataPlan says where every data object goes.
type DataPlan struct {
	Blob    []byte
	BlobOff map[string]int // label or alias -> offset in engine_rodata
	Relocs  []BlobReloc
	// InBody lists, per function, the objects placed after its code: its
	// own (constant pools, jump tables, tables of its labels) and the
	// copies of objects it reads without LEA.
	InBody map[string][]*DataObj
	// Rename maps, per function, a data label to the label its body uses.
	Rename map[string]map[string]string
	// Errors are per function (key "" for data that belongs to none).
	Errors map[string][]error
	// Stats for the report.
	Objects, BlobObjects, InBodyObjects, DupObjects, Dropped int
}

// RIP-relative operand: sym, sym+N or sym-N before (%rip).
var reRIP = regexp.MustCompile(`([.\w$]+)([+-]\d+)?\(%rip\)`)

// A symbol in an absolute operand (not RIP-relative): sym(,%reg,8),
// sym(%reg) or an immediate $sym.
var reAbsSym = regexp.MustCompile(`(?:^|[\s,])\$?([._A-Za-z][\w.$]*)(?:[+-]\d+)?\((?:%r[a-z0-9]+)?,`)
var reImmSym = regexp.MustCompile(`\$([._A-Za-z][\w.$]*)`)

type textRef struct {
	fn, sym string
	lea     bool
}

func isTextLabel(s string) bool {
	return strings.HasPrefix(s, ".LBB") || strings.HasPrefix(s, ".Ltmp") || strings.HasPrefix(s, ".Lfunc_begin")
}

// PlanData classifies the data objects of an amd64 unit. funcs are the
// functions of the module (defined or external).
func PlanData(u *AsmUnit, funcs map[string]bool) *DataPlan {
	p := &DataPlan{BlobOff: map[string]int{}, InBody: map[string][]*DataObj{}, Rename: map[string]map[string]string{},
		Errors: map[string][]error{}}
	fail := func(fn string, f string, a ...any) { p.Errors[fn] = append(p.Errors[fn], fmt.Errorf(f, a...)) }
	p.Objects = len(u.DataOrder)
	for _, o := range u.DataOrder {
		if !strings.HasPrefix(o.Section, ".rodata") && !strings.HasPrefix(o.Section, ".data.rel.ro") {
			fail("", "data object %s in section %s: the engine may have read-only data only", o.Label, o.Section)
		}
	}
	// References from code.
	var refs []textRef
	for _, name := range u.FuncOrder {
		for _, raw := range u.Funcs[name].Lines {
			l := clean(raw)
			if l == "" || strings.HasSuffix(l, ":") || strings.HasPrefix(l, ".") {
				continue
			}
			mn := strings.Fields(l)[0]
			for _, m := range reRIP.FindAllStringSubmatch(l, -1) {
				refs = append(refs, textRef{name, m[1], strings.HasPrefix(mn, "lea")})
			}
			if m := reAbsSym.FindStringSubmatch(l); m != nil && !strings.Contains(l, "(%rip)") {
				fail(name, "absolute address of %s (needs a 32-bit relocation): %q", m[1], l)
			}
			if m := reImmSym.FindStringSubmatch(l); m != nil {
				fail(name, "absolute address of %s as an immediate: %q", m[1], l)
			}
		}
	}
	// References from data, and the owner of each object that holds text
	// labels.
	dataSyms := map[*DataObj][]string{}
	owner := map[*DataObj]string{}
	for _, o := range u.DataOrder {
		for _, l := range o.Lines {
			for _, s := range exprSymbols(l) {
				if isTextLabel(s) {
					fn := u.LabelFunc[s]
					if owner[o] != "" && owner[o] != fn {
						fail(fn, "data %s holds labels of two functions", o.Label)
					}
					owner[o] = fn
					continue
				}
				dataSyms[o] = append(dataSyms[o], s)
			}
		}
	}
	// Classify.
	inBlob := map[*DataObj]bool{}
	var work []*DataObj
	addBlob := func(o *DataObj) {
		if !inBlob[o] {
			inBlob[o] = true
			work = append(work, o)
		}
	}
	nonLEA := map[*DataObj][]string{} // object -> functions reading it without LEA
	usedBy := map[*DataObj]map[string]bool{}
	for _, r := range refs {
		if funcs[r.sym] {
			if !r.lea {
				fail(r.fn, "non-LEA reference to the function %s", r.sym)
			}
			continue
		}
		o := u.Data[r.sym]
		if o == nil {
			fail(r.fn, "reference to unknown symbol %s", r.sym)
			continue
		}
		if usedBy[o] == nil {
			usedBy[o] = map[string]bool{}
		}
		usedBy[o][r.fn] = true
		private := strings.HasPrefix(o.Label, ".LCPI") || strings.HasPrefix(o.Label, ".LJTI") || owner[o] != ""
		if private {
			continue
		}
		if r.lea {
			addBlob(o)
		} else {
			nonLEA[o] = append(nonLEA[o], r.fn)
		}
	}
	for len(work) > 0 {
		o := work[0]
		work = work[1:]
		for _, s := range dataSyms[o] {
			if funcs[s] {
				continue
			}
			t := u.Data[s]
			if t == nil {
				fail("", "data %s refers to unknown symbol %s", o.Label, s)
				continue
			}
			if owner[t] != "" {
				fail(owner[t], "data %s points into the table of labels %s", o.Label, t.Label)
				continue
			}
			addBlob(t)
		}
	}
	addBody := func(fn string, o *DataObj, label string) {
		p.InBody[fn] = append(p.InBody[fn], o)
		if p.Rename[fn] == nil {
			p.Rename[fn] = map[string]string{}
		}
		p.Rename[fn][o.Label] = label
		for _, a := range o.Aliases {
			p.Rename[fn][a] = label
		}
	}
	for _, o := range u.DataOrder {
		private := strings.HasPrefix(o.Label, ".LCPI") || strings.HasPrefix(o.Label, ".LJTI") || owner[o] != ""
		switch {
		case private:
			users := SortedKeys(usedBy[o])
			fn := owner[o]
			if fn == "" && len(users) == 1 {
				fn = users[0]
			}
			if len(users) > 1 || (owner[o] != "" && len(users) == 1 && users[0] != owner[o]) {
				fail(fn, "private data %s used by %v", o.Label, users)
				continue
			}
			if fn == "" {
				p.Dropped++
				continue
			}
			for _, s := range dataSyms[o] {
				if u.Data[s] == o {
					continue // an offset from the object itself
				}
				fail(fn, "data %s inside %s holds the address of %s, which needs a relocation", o.Label, fn, s)
			}
			label := o.Label
			if !strings.HasPrefix(label, ".L") {
				label = ".Lpriv." + label
			}
			addBody(fn, o, label)
			p.InBodyObjects++
		default:
			if fns := nonLEA[o]; len(fns) > 0 {
				if len(dataSyms[o]) > 0 {
					for _, fn := range fns {
						fail(fn, "non-LEA reference to %s, which holds pointers", o.Label)
					}
				} else {
					seen := map[string]bool{}
					for _, fn := range fns {
						if !seen[fn] {
							seen[fn] = true
							addBody(fn, o, ".Ldup."+fn+"."+strings.TrimPrefix(o.Label, "."))
							p.DupObjects++
						}
					}
				}
			}
			if !inBlob[o] && len(nonLEA[o]) == 0 {
				p.Dropped++
			}
		}
	}
	// Lay the blob out in text order and fill it.
	type fix struct {
		off, size int
		expr      string
		obj       *DataObj
	}
	var fixes []fix
	for _, o := range u.DataOrder {
		if !inBlob[o] {
			continue
		}
		p.BlobObjects++
		start := alignUp(len(p.Blob), 1<<o.Align)
		p.Blob = append(p.Blob, make([]byte, start-len(p.Blob))...)
		p.BlobOff[o.Label] = start
		for _, a := range o.Aliases {
			p.BlobOff[a] = start
		}
		for _, l := range o.Lines {
			b, fx, err := encodeDirective(l, len(p.Blob)-start)
			if err != nil {
				fail("", "data %s: %v", o.Label, err)
				continue
			}
			for _, f := range fx {
				fixes = append(fixes, fix{start + f.off, f.size, f.expr, o})
			}
			p.Blob = append(p.Blob, b...)
		}
	}
	for _, f := range fixes {
		v, reloc, err := p.evalExpr(f.expr, f.off, funcs)
		if err != nil {
			fail("", "data %s: %v", f.obj.Label, err)
			continue
		}
		if reloc != nil {
			if f.size != 8 {
				fail("", "data %s: a %d-byte absolute address of %s", f.obj.Label, f.size, reloc.Sym)
				continue
			}
			reloc.Off = f.off
			p.Relocs = append(p.Relocs, *reloc)
			continue
		}
		if f.size == 4 && (v < -1<<31 || v >= 1<<32) {
			fail("", "data %s: value %d does not fit 32 bits", f.obj.Label, v)
		}
		for k := 0; k < f.size; k++ {
			p.Blob[f.off+k] = byte(v >> (8 * k))
		}
	}
	sort.Slice(p.Relocs, func(i, j int) bool { return p.Relocs[i].Off < p.Relocs[j].Off })
	return p
}

func alignUp(x, a int) int { return (x + a - 1) / a * a }

type exprFix struct {
	off, size int
	expr      string
}

// reSymTok matches a number or a symbol. Numbers are matched whole so that
// the tail of a hex literal (0x8000...) never reads as a symbol x8000...;
// isSymTok tells the two apart.
var reSymTok = regexp.MustCompile(`0[xX][0-9a-fA-F]+|[0-9]+|[._A-Za-z$][\w.$]*`)

func isSymTok(t string) bool { return t[0] < '0' || t[0] > '9' }

// exprSymbols lists the symbols a data directive refers to.
func exprSymbols(l string) []string {
	f := strings.Fields(l)
	if len(f) < 2 || f[0] == ".ascii" || f[0] == ".asciz" || f[0] == ".string" {
		return nil
	}
	var out []string
	for _, t := range reSymTok.FindAllString(strings.Join(f[1:], " "), -1) {
		if isSymTok(t) {
			out = append(out, t)
		}
	}
	return out
}

// encodeDirective returns the bytes of one data directive; values that name
// a symbol come back as fixes, left zero for now.
func encodeDirective(l string, at int) ([]byte, []exprFix, error) {
	dir, arg := l, ""
	if i := strings.IndexAny(l, " \t"); i >= 0 {
		dir, arg = l[:i], strings.TrimSpace(l[i+1:])
	}
	size := map[string]int{".byte": 1, ".short": 2, ".value": 2, ".word": 2, ".long": 4, ".int": 4, ".quad": 8}
	switch dir {
	case ".p2align":
		a := 0
		fmt.Sscan(strings.Split(arg, ",")[0], &a)
		return make([]byte, alignUp(at, 1<<a)-at), nil, nil
	case ".zero":
		parts := strings.Split(arg, ",")
		n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, nil, fmt.Errorf("bad .zero %q", arg)
		}
		b := make([]byte, n)
		if len(parts) > 1 {
			v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 0, 64)
			if err != nil {
				return nil, nil, fmt.Errorf("bad .zero fill %q", arg)
			}
			for i := range b {
				b[i] = byte(v)
			}
		}
		return b, nil, nil
	case ".ascii", ".asciz", ".string":
		s, err := unquoteGNU(arg)
		if err != nil {
			return nil, nil, err
		}
		if dir != ".ascii" {
			s = append(s, 0)
		}
		return s, nil, nil
	}
	n, ok := size[dir]
	if !ok {
		return nil, nil, fmt.Errorf("unknown directive %q", l)
	}
	var out []byte
	var fixes []exprFix
	for _, v := range strings.Split(arg, ",") {
		v = strings.TrimSpace(v)
		if x, err := strconv.ParseInt(v, 0, 64); err == nil {
			for k := 0; k < n; k++ {
				out = append(out, byte(x>>(8*k)))
			}
			continue
		}
		if x, err := strconv.ParseUint(v, 0, 64); err == nil {
			for k := 0; k < n; k++ {
				out = append(out, byte(x>>(8*k)))
			}
			continue
		}
		fixes = append(fixes, exprFix{at + len(out), n, v})
		out = append(out, make([]byte, n)...)
	}
	return out, fixes, nil
}

// evalExpr evaluates sym, sym+N, sym-N, symA-symB(+N) at blob offset at. A
// lone symbol (or with an addend) is a pointer: it comes back as a reloc.
func (p *DataPlan) evalExpr(e string, at int, funcs map[string]bool) (int64, *BlobReloc, error) {
	var pos, neg []string
	var k int64
	tok := regexp.MustCompile(`[+-]?\s*(?:0x[0-9a-fA-F]+|\d+|[._A-Za-z$][\w.$]*)`)
	rest := strings.ReplaceAll(e, " ", "")
	for _, t := range tok.FindAllString(rest, -1) {
		sign := int64(1)
		if t[0] == '-' {
			sign, t = -1, t[1:]
		} else if t[0] == '+' {
			t = t[1:]
		}
		if x, err := strconv.ParseInt(t, 0, 64); err == nil {
			k += sign * x
			continue
		}
		if x, err := strconv.ParseUint(t, 0, 64); err == nil {
			k += sign * int64(x)
			continue
		}
		if sign > 0 {
			pos = append(pos, t)
		} else {
			neg = append(neg, t)
		}
	}
	if len(pos) != 1 || len(neg) > 1 {
		return 0, nil, fmt.Errorf("unsupported expression %q", e)
	}
	off := func(s string) (int, error) {
		if s == "." {
			return at, nil
		}
		o, ok := p.BlobOff[s]
		if !ok {
			return 0, fmt.Errorf("%q: %s is not in engine_rodata", e, s)
		}
		return o, nil
	}
	if len(neg) == 1 {
		a, err := off(pos[0])
		if err != nil {
			return 0, nil, err
		}
		b, err := off(neg[0])
		if err != nil {
			return 0, nil, err
		}
		return int64(a-b) + k, nil, nil
	}
	if funcs[pos[0]] {
		return 0, &BlobReloc{Sym: pos[0], Add: k, Func: true}, nil
	}
	a, err := off(pos[0])
	if err != nil {
		return 0, nil, err
	}
	return 0, &BlobReloc{Sym: "", Add: int64(a) + k}, nil
}

// unquoteGNU decodes a GNU assembler string literal.
func unquoteGNU(s string) ([]byte, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return nil, fmt.Errorf("bad string %q", s)
	}
	s = s[1 : len(s)-1]
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(s) {
			return nil, fmt.Errorf("bad escape")
		}
		switch e := s[i]; {
		case e >= '0' && e <= '7':
			v, n := 0, 0
			for n < 3 && i < len(s) && s[i] >= '0' && s[i] <= '7' {
				v = v*8 + int(s[i]-'0')
				i++
				n++
			}
			i--
			out = append(out, byte(v))
		case e == 'x':
			v := 0
			i++
			for i < len(s) && strings.IndexByte("0123456789abcdefABCDEF", s[i]) >= 0 {
				d, _ := strconv.ParseInt(s[i:i+1], 16, 8)
				v = v*16 + int(d)
				i++
			}
			i--
			out = append(out, byte(v))
		case e == 'n':
			out = append(out, '\n')
		case e == 't':
			out = append(out, '\t')
		case e == 'r':
			out = append(out, '\r')
		case e == 'b':
			out = append(out, '\b')
		case e == 'f':
			out = append(out, '\f')
		case e == 'v':
			out = append(out, '\v')
		case e == 'a':
			out = append(out, 7)
		default:
			out = append(out, e)
		}
	}
	return out, nil
}

// RenameData rewrites the data labels a body line names, for the body of fn.
func (p *DataPlan) RenameLine(fn, l string) string {
	m := p.Rename[fn]
	if len(m) == 0 {
		return l
	}
	return reSymTok.ReplaceAllStringFunc(l, func(s string) string {
		if r, ok := m[s]; ok {
			return r
		}
		return s
	})
}

// BodyData returns the assembly of the data objects placed in fn's body.
func (p *DataPlan) BodyData(fn string) string {
	var b strings.Builder
	for _, o := range p.InBody[fn] {
		fmt.Fprintf(&b, ".p2align %d\n%s:\n", o.Align, p.Rename[fn][o.Label])
		for _, l := range o.Lines {
			if strings.HasPrefix(l, ".ascii") || strings.HasPrefix(l, ".asciz") || strings.HasPrefix(l, ".string") {
				b.WriteString(l + "\n")
				continue
			}
			b.WriteString(p.RenameLine(fn, l) + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}
