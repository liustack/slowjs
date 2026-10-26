// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Whole-program input: the engine's C compiled as one module. This file reads
// what the shadow-stack pass reports (the signature of every function, the
// address-taken ones, the call graph) and splits llc's assembly into
// functions and data objects, so the lowering can look a function or a data
// label up instead of scanning the whole text each time.
package gen

import (
	"fmt"
	"sort"
	"strings"
)

// Report is what the shadow-stack pass printed with -shadow-report.
type Report struct {
	// Sigs gives the code of every function the module defines or calls
	// (see SigFromCode), Defined whether the module defines it, Leaf
	// whether it is a register-entry leaf.
	Sigs    map[string]string
	Defined map[string]bool
	Leaf    map[string]bool
	// AddrTaken lists the functions whose address is used other than by a
	// direct call.
	AddrTaken map[string]bool
	// Calls is the direct call graph, ICalls the signature codes each
	// function calls through a pointer.
	Calls  map[string][]string
	ICalls map[string][]string
	// FrameAddr lists the functions that read the machine frame address
	// (QuickJS's stack overflow checks), Packed the ones the pass gave an
	// argument block, Rejected the ones the pass emptied, with the reason.
	FrameAddr    map[string]bool
	Packed       map[string]bool
	PackedICalls map[string]int
	Rejected     map[string]string
	GotoTable    []string
	Mismatch     [][2]string
}

// ParseReport reads the pass's report lines; other lines are ignored.
func ParseReport(text string) *Report {
	r := &Report{Sigs: map[string]string{}, Defined: map[string]bool{}, Leaf: map[string]bool{},
		AddrTaken: map[string]bool{}, Calls: map[string][]string{}, ICalls: map[string][]string{},
		FrameAddr: map[string]bool{}, Packed: map[string]bool{}, PackedICalls: map[string]int{},
		Rejected: map[string]string{}}
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "SIG":
			if len(f) >= 5 {
				r.Sigs[f[1]] = f[2]
				r.Defined[f[1]] = f[3] == "def"
				r.Leaf[f[1]] = f[4] == "leaf"
			}
		case "ADDR":
			r.AddrTaken[f[1]] = true
		case "CALL":
			if len(f) >= 3 {
				r.Calls[f[1]] = append(r.Calls[f[1]], f[2])
			}
		case "ICALL":
			if len(f) >= 3 {
				r.ICalls[f[1]] = append(r.ICalls[f[1]], f[2])
			}
		case "FRAMEADDR":
			r.FrameAddr[f[1]] = true
		case "PACKED":
			r.Packed[f[1]] = true
		case "PACKED-ICALL":
			r.PackedICalls[f[1]]++
		case "REJECT":
			r.Rejected[f[1]] = strings.Join(f[2:], " ")
		case "GOTO-TABLE":
			r.GotoTable = append(r.GotoTable, f[1])
		case "MISMATCH":
			if len(f) >= 3 {
				r.Mismatch = append(r.Mismatch, [2]string{f[1], f[2]})
			}
		}
	}
	return r
}

// SigFromCode turns a pass signature code into a Sig. Every integer or
// pointer argument is one word, moved as the whole register, so whatever
// extension the C convention relies on survives the Go argument area. A
// float is moved as the low half of a double word. 'j' is a pair of 64-bit
// integers (a JSValue). Variadic, byval ('B') and unknown ('?') types are
// errors: the IR check names the C that has them.
func SigFromCode(code string) (Sig, error) {
	ps, ret, ok := strings.Cut(code, ">")
	if !ok {
		return Sig{}, fmt.Errorf("bad signature code %q", code)
	}
	if strings.HasSuffix(ps, "...") {
		return Sig{}, fmt.Errorf("variadic signature %q", code)
	}
	var s Sig
	for _, c := range ps {
		switch c {
		case 'i':
			s.Params = append(s.Params, I64)
		case 'd', 'f':
			s.Params = append(s.Params, F64)
		case 'j':
			s.Params = append(s.Params, JSV)
		case 'B':
			return Sig{}, fmt.Errorf("byval aggregate in signature %q", code)
		default:
			return Sig{}, fmt.Errorf("unsupported argument type in signature %q", code)
		}
	}
	switch ret {
	case "v":
		s.Ret = Void
	case "i":
		s.Ret = I64
	case "d", "f":
		s.Ret = F64
	case "j":
		s.Ret = JSV
	default:
		return Sig{}, fmt.Errorf("unsupported result type in signature %q", code)
	}
	return s, nil
}

// AsmFunc is one function of llc's text: its body lines up to .Lfunc_end.
type AsmFunc struct {
	Name  string
	Lines []string // raw lines after the label
}

// DataObj is one labelled object of a data section in llc's text.
type DataObj struct {
	Label   string
	Aliases []string
	Section string
	Align   int      // log2 of the alignment
	Lines   []string // data directives, cleaned
	Global  bool
	Order   int
}

// AsmUnit is llc's text split up.
type AsmUnit struct {
	Funcs     map[string]*AsmFunc
	FuncOrder []string
	Data      map[string]*DataObj // by label and by every alias
	DataOrder []*DataObj
	// LabelFunc maps each local text label (.LBB, .Ltmp) to its function.
	LabelFunc map[string]string
	// Sets are the symbol assignments (name = target), such as musl's weak
	// aliases.
	Sets map[string]string
}

// dataDirectives are the ones llc writes in data sections.
var dataDirectives = []string{".byte", ".short", ".long", ".quad", ".zero", ".ascii", ".asciz", ".value", ".word", ".int", ".string"}

func isDataDirective(l string) bool {
	for _, d := range dataDirectives {
		if strings.HasPrefix(l, d+" ") || strings.HasPrefix(l, d+"\t") {
			return true
		}
	}
	return false
}

// SplitAsm splits llc's x86-64 or aarch64 GNU text into functions and data
// objects. comment is the comment marker ("#" on x86-64, "//" on aarch64).
func SplitAsm(asm, comment string) (*AsmUnit, error) {
	u := &AsmUnit{Funcs: map[string]*AsmFunc{}, Data: map[string]*DataObj{}, LabelFunc: map[string]string{}, Sets: map[string]string{}}
	section := ".text"
	align := 0
	var cur *AsmFunc
	var obj *DataObj
	pendingLabels := []string{}
	globals := map[string]bool{}
	lines := strings.Split(asm, "\n")
	for _, raw := range lines {
		l := raw
		if !strings.Contains(l, "goasm.icall") {
			l = stripComment(l, comment)
		}
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if cur != nil {
			if strings.HasPrefix(l, ".Lfunc_end") {
				cur = nil
				continue
			}
			cur.Lines = append(cur.Lines, raw)
			if strings.HasSuffix(l, ":") && strings.HasPrefix(l, ".L") {
				u.LabelFunc[strings.TrimSuffix(l, ":")] = cur.Name
			}
			continue
		}
		switch {
		case strings.HasPrefix(l, ".section"):
			f := strings.FieldsFunc(strings.TrimPrefix(l, ".section"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
			section = strings.Trim(f[0], "\"")
			align, obj = 0, nil
			continue
		case l == ".text" || strings.HasPrefix(l, ".text\t") || strings.HasPrefix(l, ".text "):
			section = ".text"
			align, obj = 0, nil
			continue
		case l == ".data" || l == ".bss":
			section = l
			align, obj = 0, nil
			continue
		case strings.HasPrefix(l, ".p2align"):
			f := strings.FieldsFunc(strings.TrimPrefix(l, ".p2align"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
			fmt.Sscan(f[0], &align)
			if obj != nil && len(obj.Lines) > 0 {
				// alignment inside an object (padding before a later label)
				obj.Lines = append(obj.Lines, l)
			}
			continue
		case strings.HasPrefix(l, ".globl"):
			f := strings.Fields(l)
			globals[f[1]] = true
			continue
		case strings.HasPrefix(l, ".set") || !strings.HasPrefix(l, ".") && strings.Contains(l, " = "):
			var name, target string
			if strings.HasPrefix(l, ".set") {
				f := strings.FieldsFunc(strings.TrimPrefix(l, ".set"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
				name, target = f[0], f[1]
			} else {
				name, target, _ = strings.Cut(l, " = ")
			}
			u.Sets[strings.TrimSpace(name)] = strings.TrimSpace(target)
			continue
		case strings.HasPrefix(l, ".type") || strings.HasPrefix(l, ".size") || strings.HasPrefix(l, ".hidden") ||
			strings.HasPrefix(l, ".weak") || strings.HasPrefix(l, ".local") || strings.HasPrefix(l, ".file") ||
			strings.HasPrefix(l, ".ident") || strings.HasPrefix(l, ".addrsig") || strings.HasPrefix(l, ".cfi_") ||
			strings.HasPrefix(l, ".intel_syntax") || strings.HasPrefix(l, ".att_syntax"):
			continue
		}
		if strings.HasSuffix(l, ":") {
			name := strings.TrimSuffix(l, ":")
			if section == ".text" {
				if strings.HasPrefix(name, ".L") {
					return nil, fmt.Errorf("text label %s outside a function", name)
				}
				cur = &AsmFunc{Name: name}
				u.Funcs[name] = cur
				u.FuncOrder = append(u.FuncOrder, name)
				continue
			}
			if obj != nil && len(obj.Lines) == 0 {
				obj.Aliases = append(obj.Aliases, name)
				u.Data[name] = obj
				continue
			}
			obj = &DataObj{Label: name, Section: section, Align: align, Order: len(u.DataOrder)}
			align = 0
			u.Data[name] = obj
			u.DataOrder = append(u.DataOrder, obj)
			_ = pendingLabels
			continue
		}
		if section == ".text" {
			return nil, fmt.Errorf("text line outside a function: %q", l)
		}
		if obj == nil {
			return nil, fmt.Errorf("data line before any label in %s: %q", section, l)
		}
		if !isDataDirective(l) {
			return nil, fmt.Errorf("unknown directive in %s: %q", section, l)
		}
		obj.Lines = append(obj.Lines, l)
	}
	for _, o := range u.DataOrder {
		o.Global = globals[o.Label]
	}
	return u, nil
}

// stripComment cuts a line at the comment marker outside string literals.
func stripComment(l, marker string) string {
	in := false
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\' && in:
			i++
		case l[i] == '"':
			in = !in
		case !in && strings.HasPrefix(l[i:], marker):
			return l[:i]
		}
	}
	return l
}

// SortedKeys returns the keys of a set in order.
func SortedKeys[T any](m map[string]T) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
