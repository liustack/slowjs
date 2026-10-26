// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// The format strings engine C hands to its libc. engine/libc/stdio.c does
// only what the engine uses and calls abort() for the rest (a floating-point
// conversion, %n, positional arguments, a length on %c, and so on), and it
// does so at run time, in front of a user. CheckFormats reads the IR of the C
// before the pass and finds every call that reaches the formatter, so that a
// format the formatter would refuse is refused when the engine is built.
//
// The rules (phase2-recursion/format-strings.md in the plan):
//
//   - A call of snprintf or vsnprintf has its format as the third argument.
//   - A call of any other variadic function has its format as the last fixed
//     argument, so a new printf-like wrapper is covered without a table of
//     their names. The functions in the list the caller gives (for the
//     engine, EngineNonFormatVariadic) take opcodes there and are left out.
//   - The format must be a string constant that FormatError accepts, or a
//     parameter that a function forwards. A function forwards a parameter
//     when it is variadic and the parameter is its last fixed one (its own
//     callers are checked), or when it is in vaListFormatters, the one
//     function that takes a va_list from such a wrapper and formats it.
//
// FormatError is the accept table. It mirrors stdio.c and has to change with
// it: testdata/formats/cases.txt lists the formats both must agree on, and a
// libc test that runs each through vsnprintf and sees whether it aborts
// belongs with the C (engine/test).
package gen

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// EngineNonFormatVariadic are the variadic functions of the engine whose
// arguments after the fixed ones are opcodes, not a format. A run on other C
// gives its own list.
var EngineNonFormatVariadic = []string{"code_match", "unicode_prop_ops"}

// vaListFormatters are the functions that format a va_list a variadic
// wrapper made: their format is a parameter, and they are reached only
// through those wrappers, whose own calls are checked.
var vaListFormatters = map[string]bool{"JS_ThrowError2": true}

// FormatError says why stdio.c's vsnprintf would abort on the format f, or
// returns nil if it takes it. It mirrors the conversion loop there: flags
// "-+ 0#", a width and a precision (digits that fit an int, or *), the
// lengths hh h l ll z j t on d i u x X o only, and the conversions d i u x
// X o c s p and %. A "%%" takes nothing else.
func FormatError(f string) error {
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			continue
		}
		start := i
		i++
		flags := false
		for i < len(f) && strings.IndexByte("-+ 0#", f[i]) >= 0 {
			flags = true
			i++
		}
		number := func() error {
			n := 0
			for ; i < len(f) && f[i] >= '0' && f[i] <= '9'; i++ {
				if n > (math.MaxInt32-int(f[i]-'0'))/10 {
					return fmt.Errorf("a width or precision that does not fit an int in %q", f[start:i+1])
				}
				n = n*10 + int(f[i]-'0')
			}
			return nil
		}
		hasWidth, hasPrec := false, false
		if i < len(f) && f[i] == '*' {
			hasWidth = true
			i++
		} else if i < len(f) && f[i] >= '0' && f[i] <= '9' {
			hasWidth = true
			if err := number(); err != nil {
				return err
			}
		}
		if i < len(f) && f[i] == '.' {
			hasPrec = true
			i++
			if i < len(f) && f[i] == '*' {
				i++
			} else if err := number(); err != nil {
				return err
			}
		}
		length := false
		if i < len(f) {
			switch f[i] {
			case 'h', 'l':
				length = true
				if i+1 < len(f) && f[i+1] == f[i] {
					i++
				}
				i++
			case 'z', 'j', 't':
				length = true
				i++
			}
		}
		if i >= len(f) {
			return fmt.Errorf("the format ends inside the conversion %q", f[start:])
		}
		switch c := f[i]; c {
		case 'd', 'i', 'u', 'x', 'X', 'o':
		case 'c', 's', 'p':
			if length {
				return fmt.Errorf("a length on %%%c in %q", c, f[start:i+1])
			}
		case '%':
			if length || flags || hasWidth || hasPrec {
				return fmt.Errorf("a flag, width, precision or length on %%%% in %q", f[start:i+1])
			}
		default:
			return fmt.Errorf("the conversion %q is not supported (in %q)", string(c), f[start:i+1])
		}
	}
	return nil
}

var (
	// @.str.3 = private unnamed_addr constant [7 x i8] c"abc%d\00", align 1
	reStrConst = regexp.MustCompile(`^(@[\w.$]+) = [^\n]*?\bconstant \[\d+ x i8\] c"(.*)"`)
	// The callee of a call, after its return type and, for a variadic one,
	// its function type with the fixed parameters in front of the dots.
	reFmtCall = regexp.MustCompile(`\bcall\b[^@]*?(\((?:[^()]|\([^()]*\))*\))?\s*@([\w.$]+)\(`)
	reDefFunc = regexp.MustCompile(`^define\b[^@]*@([\w.$]+)\(`)
	reGEPStr  = regexp.MustCompile(`getelementptr[^(]*\(\s*i8,\s*ptr (@[\w.$]+),\s*i(?:32|64) (\d+)\s*\)\s*$`)
	reLastSym = regexp.MustCompile(`@[\w.$]+\s*$`)
	reLocalV  = regexp.MustCompile(`%[\w.$]+\s*$`)
)

// llString decodes the body of an LLVM c"..." constant.
func llString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '\\':
			b.WriteByte('\\')
			i++
		case s[i] == '\\' && i+2 < len(s):
			v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err != nil {
				b.WriteByte(s[i])
				continue
			}
			b.WriteByte(byte(v))
			i += 2
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// balanced returns the text inside the parentheses that open at s[open],
// and the index of the one that closes them.
func balanced(s string, open int) (string, int) {
	depth, quoted := 0, false
	for i := open; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '(':
			depth++
		case c == ')':
			if depth--; depth == 0 {
				return s[open+1 : i], i
			}
		}
	}
	return s[open+1:], len(s)
}

// splitTop splits at the commas outside brackets.
func splitTop(s string) []string {
	var out []string
	depth, last := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[last:i]))
				last = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(s[last:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

// fnIR is one function of a module: its parameters and the definition of
// each local value that can matter to a format.
type fnIR struct {
	name, lastFixed string
	variadic        bool
	params          map[string]bool
	defs            map[string]string   // %x -> the instruction that defines it
	stores          map[string][]string // %slot -> the values stored in it
}

var (
	reStore = regexp.MustCompile(`^\s*store\s+(?:volatile\s+)?ptr\s+(?:\w+\s+)*([@%][\w.$]+|null),\s*ptr\s+(%[\w.$]+)`)
	reDef   = regexp.MustCompile(`^\s*(%[\w.$]+)\s*=\s*(.*)$`)
	rePhi   = regexp.MustCompile(`^phi\s+ptr\s+(.*)$`)
	reSel   = regexp.MustCompile(`^select\s+i1\s+[^,]+,\s*ptr\s+(?:\w+\s+)*([@%][\w.$]+|null),\s*ptr\s+(?:\w+\s+)*([@%][\w.$]+|null)`)
	reLoad  = regexp.MustCompile(`^load\s+(?:volatile\s+)?ptr,\s*ptr\s+(%[\w.$]+)`)
	rePhiIn = regexp.MustCompile(`\[\s*([@%][\w.$]+|null)\s*,`)
)

// formatValues follows a format argument back to what it can be: the
// constants it may name, and the parameters it may be (in a function that
// stores a parameter in a slot, as clang does without optimization, and
// reads it back). ok is false when it can be anything else.
func (f *fnIR) formatValues(v string, seen map[string]bool) (consts, params []string, ok bool) {
	if strings.HasPrefix(v, "@") {
		return []string{v}, nil, true
	}
	if !strings.HasPrefix(v, "%") || seen[v] {
		return nil, nil, false
	}
	seen[v] = true
	if f.params[v] {
		return nil, []string{v}, true
	}
	def := f.defs[v]
	var from []string
	switch {
	case reLoad.MatchString(def):
		slot := reLoad.FindStringSubmatch(def)[1]
		from = f.stores[slot]
		if len(from) == 0 {
			return nil, nil, false
		}
	case rePhi.MatchString(def):
		for _, m := range rePhiIn.FindAllStringSubmatch(def, -1) {
			from = append(from, m[1])
		}
	case reSel.MatchString(def):
		m := reSel.FindStringSubmatch(def)
		from = []string{m[1], m[2]}
	default:
		return nil, nil, false
	}
	for _, w := range from {
		c, p, ok := f.formatValues(w, seen)
		if !ok {
			return nil, nil, false
		}
		consts, params = append(consts, c...), append(params, p...)
	}
	return consts, params, true
}

// CheckFormats finds every call that reaches the formatter in the IR of C
// and reports each whose format the formatter would refuse, or whose format
// is not a constant and is not forwarded (see the rules above). It reads IR
// as clang writes it without optimization (-O0): the optimizer drops the
// variable arguments of a wrapper nobody reads them in, which is how a call
// loses its variadic type.
func CheckFormats(ll string, nonFormat []string) []IRViolation {
	vs, _ := CountFormats(ll, nonFormat)
	return vs
}

// CountFormats is CheckFormats that also says how many calls it held to the
// rules, so a run can show the check looked at something.
func CountFormats(ll string, nonFormat []string) ([]IRViolation, int) {
	strs := map[string]string{}
	lines := strings.Split(ll, "\n")
	for _, l := range lines {
		if m := reStrConst.FindStringSubmatch(l); m != nil {
			s := llString(m[2])
			if z := strings.IndexByte(s, 0); z >= 0 {
				s = s[:z]
			}
			strs[m[1]] = s
		}
	}
	skip := map[string]bool{}
	for _, n := range nonFormat {
		skip[n] = true
	}
	var out []IRViolation
	var f *fnIR
	var body []string
	calls := 0
	flush := func() {
		if f == nil {
			return
		}
		for _, line := range body {
			vs, isCall := f.checkCall(line, strs, skip)
			out = append(out, vs...)
			if isCall {
				calls++
			}
		}
	}
	for _, line := range lines {
		if m := reDefFunc.FindStringSubmatchIndex(line); m != nil {
			flush()
			f = &fnIR{name: line[m[2]:m[3]], params: map[string]bool{}, defs: map[string]string{}, stores: map[string][]string{}}
			body = nil
			params, _ := balanced(line, m[1]-1)
			ps := splitTop(params)
			for _, p := range ps {
				if v := strings.TrimSpace(reLocalV.FindString(p)); v != "" {
					f.params[v] = true
				}
			}
			if n := len(ps); n > 0 && ps[n-1] == "..." {
				f.variadic = true
				if n > 1 {
					f.lastFixed = strings.TrimSpace(reLocalV.FindString(ps[n-2]))
				}
			}
			continue
		}
		if f == nil {
			continue
		}
		if strings.HasPrefix(line, "}") {
			flush()
			f = nil
			continue
		}
		if m := reDef.FindStringSubmatch(line); m != nil {
			f.defs[m[1]] = m[2]
		}
		if m := reStore.FindStringSubmatch(line); m != nil {
			f.stores[m[2]] = append(f.stores[m[2]], m[1])
		}
		body = append(body, line)
	}
	flush()
	return out, calls
}

// checkCall holds one line of the function to the rules, if it is a call
// that reaches the formatter.
func (f *fnIR) checkCall(line string, strs map[string]string, skip map[string]bool) (vs []IRViolation, isCall bool) {
	m := reFmtCall.FindStringSubmatchIndex(line)
	if m == nil {
		return nil, false
	}
	callee := line[m[4]:m[5]]
	args, _ := balanced(line, m[1]-1)
	idx := -1
	switch {
	case callee == "snprintf" || callee == "vsnprintf":
		idx = 2
	case m[2] >= 0 && strings.Contains(line[m[2]:m[3]], "..."):
		if skip[callee] {
			return nil, false
		}
		// The format is the last of the fixed parameters, before the dots.
		fixed, _ := balanced(line[m[2]:m[3]], 0)
		idx = len(splitTop(fixed)) - 2
	default:
		return nil, false
	}
	viol := func(rule string) ([]IRViolation, bool) {
		return []IRViolation{{Func: f.name, Rule: "format string: " + rule, Line: strings.TrimSpace(line)}}, true
	}
	as := splitTop(args)
	if idx < 0 || idx >= len(as) {
		return viol(fmt.Sprintf("call of %s has no format argument", callee))
	}
	arg := as[idx]
	var consts []string
	if g := reGEPStr.FindStringSubmatch(arg); g != nil {
		off, _ := strconv.Atoi(g[2])
		s, ok := strs[g[1]]
		if !ok || off > len(s) {
			return viol(fmt.Sprintf("the format of %s is not a string constant the check can read", callee))
		}
		return f.formats(callee, []string{s[off:]}, line), true
	}
	v := strings.TrimSpace(reLastSym.FindString(arg))
	if v == "" {
		v = strings.TrimSpace(reLocalV.FindString(arg))
	}
	cs, ps, ok := f.formatValues(v, map[string]bool{})
	if !ok {
		return viol(fmt.Sprintf("the format of %s is not a string constant the check can read", callee))
	}
	for _, p := range ps {
		// A parameter some wrapper forwards.
		if !((f.variadic && p == f.lastFixed) || vaListFormatters[f.name]) {
			return viol(fmt.Sprintf("the format of %s is not a constant (the parameter %s)", callee, p))
		}
	}
	for _, c := range cs {
		s, ok := strs[c]
		if !ok {
			return viol(fmt.Sprintf("the format of %s is not a string constant the check can read (%s)", callee, c))
		}
		consts = append(consts, s)
	}
	return f.formats(callee, consts, line), true
}

func (f *fnIR) formats(callee string, fs []string, line string) []IRViolation {
	for _, s := range fs {
		if err := FormatError(s); err != nil {
			return []IRViolation{{Func: f.name, Line: strings.TrimSpace(line),
				Rule: fmt.Sprintf("format string: %v, which engine/libc/stdio.c does not take, in the format %q of %s", err, s, callee)}}
		}
	}
	return nil
}
