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
	// Varargs lists the variadic definitions -shadow-varargs rewrote.
	Varargs []string
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
		case "VARARGS":
			r.Varargs = append(r.Varargs, f[1])
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
	Align   int // log2 of the alignment
	// AlignCond: the start alignment had a max-skip below its step, so llc
	// may have placed the object without it.
	AlignCond bool
	Lines     []string // data directives, cleaned
	Global    bool
	Order     int
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

// dataDirectives are the ones llc writes in data sections; .xword and
// .hword are aarch64's names for 8 and 2 bytes.
var dataDirectives = []string{".byte", ".short", ".long", ".quad", ".zero", ".ascii", ".asciz", ".value", ".word", ".int", ".string", ".xword", ".hword"}

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
	alignCond := false
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
		// Directives match by their exact name, with their operands checked:
		// a near miss (.set0, .p2alignx) or a missing operand is an error,
		// never a guess or a panic.
		dir, rest := l, ""
		if i := strings.IndexAny(l, " \t"); i >= 0 {
			dir, rest = l[:i], strings.TrimSpace(l[i+1:])
		}
		operands := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
		switch dir {
		case ".section":
			if len(operands) == 0 {
				return nil, fmt.Errorf("section without a name: %q", l)
			}
			section = strings.Trim(operands[0], "\"")
			align, alignCond, obj = 0, false, nil
			continue
		case ".text":
			section = ".text"
			align, alignCond, obj = 0, false, nil
			continue
		case ".data", ".bss":
			if rest != "" {
				return nil, fmt.Errorf("unsupported section form: %q", l)
			}
			section = l
			align, alignCond, obj = 0, false, nil
			continue
		case ".p2align":
			// Parsed and range-checked as the encoder does, so a later shift
			// by the power cannot panic. A max-skip below the step may drop
			// the alignment in llc's placement: the next object's start
			// alignment is then conditional.
			pow, _, skip, err := parseP2align(rest)
			if err != nil {
				return nil, fmt.Errorf("%v: %q", err, l)
			}
			align, alignCond = pow, skip >= 0 && skip < 1<<pow-1
			if obj != nil && len(obj.Lines) > 0 {
				// alignment inside an object (padding before a later label)
				obj.Lines = append(obj.Lines, l)
			}
			continue
		case ".globl":
			if len(operands) != 1 {
				return nil, fmt.Errorf("bad .globl: %q", l)
			}
			globals[operands[0]] = true
			continue
		case ".set":
			if len(operands) != 2 {
				return nil, fmt.Errorf("bad .set: %q", l)
			}
			u.Sets[operands[0]] = operands[1]
			continue
		case ".type", ".size", ".hidden", ".weak", ".local", ".file", ".ident", ".addrsig", ".addrsig_sym",
			".intel_syntax", ".att_syntax":
			continue
		}
		if strings.HasPrefix(dir, ".cfi_") {
			continue
		}
		if !strings.HasPrefix(l, ".") && strings.Contains(l, " = ") {
			name, target, _ := strings.Cut(l, " = ")
			u.Sets[strings.TrimSpace(name)] = strings.TrimSpace(target)
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
			obj = &DataObj{Label: name, Section: section, Align: align, AlignCond: alignCond, Order: len(u.DataOrder)}
			align, alignCond = 0, false
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
