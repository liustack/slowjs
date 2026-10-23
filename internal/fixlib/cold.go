// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// fixColdFrames does for the functions of coldSites what fixFrames does
// for those of frameSites, though each of them passes the address of a
// part of its frame to a function it calls, which a Go array cannot take:
// the goroutine stack moves. That part is only used on a path the function
// rarely takes, the cold path: the string of an integer atom in
// __JS_AtomToValue, the comparison of two BigInts in js_strict_eq2. So the
// statements of the cold path go into a function of their own, which takes
// the same TLS frame and gets the values it reads as arguments, and the
// rest of the frame becomes a Go array, as fixFrames does it.
//
// The statements are matched as ccgo writes them (coldShapes), so any
// change to them stops fixColdFrames. What is left of the frame must
// qualify as in fixFrames: every use of bp is a load or a store of a cell,
// so no address of it reaches a callee, and the function cannot reach a
// stack check other than through the cold function, which allocates the
// frame the function had, so a stack check sees the same TLS stack on every
// path. The cells the cold path reads are the copies of the parameters
// that the function stores at its entry and never writes again, so the
// cold function gets the parameters and makes the same copies in its own
// frame. The part of the frame whose address the cold path passes is not
// used outside it.
//
// It runs after fixUp, on the conversions as fixUp leaves them, and before
// fixNoDefer, which frees the frame of the cold function before its
// return.
func fixColdFrames(b []byte, hw []*ast.File, pin bool) ([]byte, error) {
	names := make([]string, 0, len(coldSites))
	for name := range coldSites {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []string
	for _, name := range names {
		site := coldSites[name]
		fixed, err := coldFunc(b, name, site)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		b = fixed
	}
	if len(problems) == 0 {
		var err error
		if b, problems, err = coldFrames(b, hw, names, pin); err != nil {
			return nil, err
		}
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("the cold paths of coldSites changed, review each and update coldShapes or coldSites:\n\t%s", strings.Join(problems, "\n\t"))
	}
	return b, nil
}

// coldSite is a function of coldSites: the function its cold path goes
// to, the frame size and the cell uses left in lib/ccgo_darwin_arm64.go,
// and what the cells and the cold path are for.
type coldSite struct {
	cold         string
	frame, cells int
	why          string
}

// coldShape is one way ccgo writes the cold path of a function: the
// function head with the frame (head), the statements of the cold path
// (path), the statement that replaces them (call), and the cold function
// (fn). They are templates: {X} matches the text coldHoles gives for X,
// the same text everywhere.
type coldShape struct {
	head, path, call, fn string
}

const coldMark = "// Changed by internal/fixlib: the cold path, which uses the TLS frame (see fixColdFrames).\n"

var coldHoles = map[string]string{
	"N":   `\d+`,                  // the frame size
	"K1":  `\d+`,                  // the cell of the first parameter
	"K2":  `\d+`,                  // the cell of the second parameter
	"B1":  `\d+`,                  // the BigInt buffer of the first operand
	"B2":  `\d+`,                  // the BigInt buffer of the second operand
	"BUF": `bp(?: \+ \d+|\+\d+)?`, // the digits of an integer atom
	"LEN": `[^\n]*`,               // the length conversion
}

// coldShapes are the shapes of the cold paths, for each function of
// coldSites: the 64-bit targets keep a JSValue parameter in a cell, the
// 32-bit ones in a uint64.
var coldShapes = map[string][]coldShape{
	"___JS_AtomToValue": {{
		head: "\nfunc ___JS_AtomToValue(tls *libc.TLS, ctx1 uintptr, atom TJSAtom, force_string TBOOL) (r TJSValue) {\n" +
			"\tbp := tls.Alloc({N})\n\tdefer tls.Free({N})\n\tvar len1 Tsize_t\n",
		path: "\tif ___JS_AtomIsTaggedInt(tls, atom) != 0 {\n" +
			"\t\tlen1 = Xu32toa(tls, {BUF}, ___JS_AtomToUInt32(tls, atom))\n" +
			"\t\treturn _js_new_string8_len(tls, ctx1, {BUF}, {LEN})\n" +
			"\t} else {\n",
		call: "\tif ___JS_AtomIsTaggedInt(tls, atom) != 0 {\n" +
			"\t\t" + coldMark +
			"\t\treturn ___JS_AtomToValue_cold(tls, ctx1, atom)\n" +
			"\t} else {\n",
		fn: "\n" + coldMark +
			"func ___JS_AtomToValue_cold(tls *libc.TLS, ctx1 uintptr, atom TJSAtom) (r TJSValue) {\n" +
			"\tbp := tls.Alloc({N})\n\tdefer tls.Free({N})\n\tvar len1 Tsize_t\n" +
			"\tlen1 = Xu32toa(tls, {BUF}, ___JS_AtomToUInt32(tls, atom))\n" +
			"\treturn _js_new_string8_len(tls, ctx1, {BUF}, {LEN})\n" +
			"}\n",
	}},
	"_js_strict_eq2": {{
		head: "\nfunc _js_strict_eq2(tls *libc.TLS, ctx1 uintptr, _op1 TJSValue, _op2 TJSValue, eq_mode TJSStrictEqModeEnum) (r TBOOL) {\n" +
			"\tbp := tls.Alloc({N})\n\tdefer tls.Free({N})\n" +
			"\t*(*TJSValue)(unsafe.Pointer(bp + {K1})) = _op1\n\t*(*TJSValue)(unsafe.Pointer(bp + {K2})) = _op2\n",
		path: "\tif int32((*(*TJSValue)(unsafe.Pointer(bp + {K1}))).Ftag) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp11 = _js_bigint_set_short(tls, bp+{B1}, *(*TJSValue)(unsafe.Pointer(bp + {K1})))\n" +
			"\t} else {\n\t\tp11 = *(*uintptr)(unsafe.Pointer(bp + {K1}))\n\t}\n" +
			"\tif int32((*(*TJSValue)(unsafe.Pointer(bp + {K2}))).Ftag) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp21 = _js_bigint_set_short(tls, bp+{B2}, *(*TJSValue)(unsafe.Pointer(bp + {K2})))\n" +
			"\t} else {\n\t\tp21 = *(*uintptr)(unsafe.Pointer(bp + {K2}))\n\t}\n" +
			"\tres = libc.BoolInt32(_js_bigint_cmp(tls, ctx1, p11, p21) == libc.Int32FromInt32(0))\n",
		call: "\t" + coldMark +
			"\tres = _js_strict_eq2_cold(tls, ctx1, _op1, _op2)\n",
		fn: "\n" + coldMark +
			"func _js_strict_eq2_cold(tls *libc.TLS, ctx1 uintptr, _op1 TJSValue, _op2 TJSValue) (r TBOOL) {\n" +
			"\tbp := tls.Alloc({N})\n\tdefer tls.Free({N})\n" +
			"\t*(*TJSValue)(unsafe.Pointer(bp + {K1})) = _op1\n\t*(*TJSValue)(unsafe.Pointer(bp + {K2})) = _op2\n" +
			"\tvar p11, p21 uintptr\n" +
			"\tif int32((*(*TJSValue)(unsafe.Pointer(bp + {K1}))).Ftag) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp11 = _js_bigint_set_short(tls, bp+{B1}, *(*TJSValue)(unsafe.Pointer(bp + {K1})))\n" +
			"\t} else {\n\t\tp11 = *(*uintptr)(unsafe.Pointer(bp + {K1}))\n\t}\n" +
			"\tif int32((*(*TJSValue)(unsafe.Pointer(bp + {K2}))).Ftag) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp21 = _js_bigint_set_short(tls, bp+{B2}, *(*TJSValue)(unsafe.Pointer(bp + {K2})))\n" +
			"\t} else {\n\t\tp21 = *(*uintptr)(unsafe.Pointer(bp + {K2}))\n\t}\n" +
			"\treturn libc.BoolInt32(_js_bigint_cmp(tls, ctx1, p11, p21) == libc.Int32FromInt32(0))\n" +
			"}\n",
	}, {
		head: "\nfunc _js_strict_eq2(tls *libc.TLS, ctx1 uintptr, op1 TJSValue, op2 TJSValue, eq_mode TJSStrictEqModeEnum) (r TBOOL) {\n" +
			"\tbp := tls.Alloc({N})\n\tdefer tls.Free({N})\n",
		path: "\tif libc.Int32FromUint64(op1>>libc.Int32FromInt32(32)) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp11 = _js_bigint_set_short(tls, bp+{B1}, op1)\n" +
			"\t} else {\n\t\tp11 = uintptr(libc.Int32FromUint64(op1))\n\t}\n" +
			"\tif libc.Int32FromUint64(op2>>libc.Int32FromInt32(32)) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp21 = _js_bigint_set_short(tls, bp+{B2}, op2)\n" +
			"\t} else {\n\t\tp21 = uintptr(libc.Int32FromUint64(op2))\n\t}\n" +
			"\tres = libc.BoolInt32(_js_bigint_cmp(tls, ctx1, p11, p21) == libc.Int32FromInt32(0))\n",
		call: "\t" + coldMark +
			"\tres = _js_strict_eq2_cold(tls, ctx1, op1, op2)\n",
		fn: "\n" + coldMark +
			"func _js_strict_eq2_cold(tls *libc.TLS, ctx1 uintptr, op1 TJSValue, op2 TJSValue) (r TBOOL) {\n" +
			"\tbp := tls.Alloc({N})\n\tdefer tls.Free({N})\n" +
			"\tvar p11, p21 uintptr\n" +
			"\tif libc.Int32FromUint64(op1>>libc.Int32FromInt32(32)) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp11 = _js_bigint_set_short(tls, bp+{B1}, op1)\n" +
			"\t} else {\n\t\tp11 = uintptr(libc.Int32FromUint64(op1))\n\t}\n" +
			"\tif libc.Int32FromUint64(op2>>libc.Int32FromInt32(32)) == int32(EJS_TAG_SHORT_BIG_INT) {\n" +
			"\t\tp21 = _js_bigint_set_short(tls, bp+{B2}, op2)\n" +
			"\t} else {\n\t\tp21 = uintptr(libc.Int32FromUint64(op2))\n\t}\n" +
			"\treturn libc.BoolInt32(_js_bigint_cmp(tls, ctx1, p11, p21) == libc.Int32FromInt32(0))\n" +
			"}\n",
	}},
}

var reHole = regexp.MustCompile(`\{([A-Z0-9]+)\}`)

// holeRegexp turns template t into a pattern with a group for each hole,
// and returns the names of the holes in the order of the groups.
func holeRegexp(t string) (*regexp.Regexp, []string) {
	var re strings.Builder
	var names []string
	last := 0
	for _, m := range reHole.FindAllStringSubmatchIndex(t, -1) {
		re.WriteString(regexp.QuoteMeta(t[last:m[0]]))
		name := t[m[2]:m[3]]
		re.WriteString("(" + coldHoles[name] + ")")
		names = append(names, name)
		last = m[1]
	}
	re.WriteString(regexp.QuoteMeta(t[last:]))
	return regexp.MustCompile(re.String()), names
}

// matchHoles finds template t in b, which must hold it once, and adds the
// text of its holes to vals, which must agree with what vals holds.
func matchHoles(b []byte, t, what string, vals map[string]string) (int, int, error) {
	re, names := holeRegexp(t)
	all := re.FindAllSubmatchIndex(b, -1)
	if len(all) != 1 {
		return 0, 0, fmt.Errorf("%s %d times, want once", what, len(all))
	}
	m := all[0]
	for i, name := range names {
		v := string(b[m[2+2*i]:m[3+2*i]])
		if old, ok := vals[name]; ok && old != v {
			return 0, 0, fmt.Errorf("%s: {%s} is %q, and %q elsewhere", what, name, v, old)
		}
		vals[name] = v
	}
	return m[0], m[1], nil
}

// fill fills the holes of template t.
func fill(t string, vals map[string]string) string {
	return reHole.ReplaceAllStringFunc(t, func(h string) string { return vals[h[1:len(h)-1]] })
}

// coldFunc moves the cold path of the function name of b into its cold
// function, or checks that it is there already, in the shape of one of
// coldShapes.
func coldFunc(b []byte, name string, site coldSite) ([]byte, error) {
	body, err := funcBody(b, funcHead(name), name)
	if err != nil {
		return nil, err
	}
	var errs []string
	for _, shape := range coldShapes[name] {
		out, err := coldShaped(b, body, name, site, shape)
		if err == nil {
			return out, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, fmt.Errorf("%s: no shape of coldShapes fits: %s", name, strings.Join(errs, "; "))
}

func coldShaped(b, body []byte, name string, site coldSite, shape coldShape) ([]byte, error) {
	vals := map[string]string{}
	start := bytes.Index(b, body)
	end := start + len(body)
	if bytes.Contains(body, []byte(coldMark)) {
		// Moved already: the cold function must be the one the call
		// stands for, right after the function.
		if _, _, err := matchHoles(body, shape.call, "the call of the cold path", vals); err != nil {
			return nil, err
		}
		cold, err := funcBody(b, funcHead(site.cold), site.cold)
		if err != nil {
			return nil, err
		}
		fn := append([]byte("\n"+coldMark), cold[1:]...)
		if _, _, err := matchHoles(fn, shape.fn, site.cold, vals); err != nil {
			return nil, err
		}
		if !bytes.HasPrefix(b[end:], []byte(fill(shape.fn, vals))) {
			return nil, fmt.Errorf("%s is not right after %s", site.cold, name)
		}
		return b, nil
	}
	if bytes.Contains(b, []byte("\nfunc "+site.cold+"(")) {
		return nil, fmt.Errorf("%s exists, but %s does not call it", site.cold, name)
	}
	if _, _, err := matchHoles(body, shape.head, "the frame", vals); err != nil {
		return nil, err
	}
	i, j, err := matchHoles(body, shape.path, "the cold path", vals)
	if err != nil {
		return nil, err
	}
	fixed := append(append(append([]byte{}, body[:i]...), fill(shape.call, vals)...), body[j:]...)
	if err := coldRest(name, fixed, vals); err != nil {
		return nil, err
	}
	out := append(append(append([]byte{}, b[:start]...), fixed...), fill(shape.fn, vals)...)
	return append(out, b[end:]...), nil
}

// coldRest checks the function with its cold path moved out: the cells of
// the parameters, K1 and K2, are only stored at the entry, and the BigInt
// buffers, B1 and B2, are not used.
func coldRest(name string, body []byte, vals map[string]string) error {
	f, err := parseFunc(name, body)
	if err != nil {
		return err
	}
	list := f.decl.Body.List
	for _, k := range []string{"K1", "K2"} {
		if vals[k] == "" {
			continue
		}
		var off int
		fmt.Sscan(vals[k], &off)
		// list[0] and list[1] are the frame, list[2] and list[3] the
		// stores of the parameters.
		for _, s := range list[4:] {
			if w := cellWrite(s, off); w != nil {
				return fmt.Errorf("%s: the cell of a parameter, bp+%d, written in %s", name, off, f.text(w))
			}
		}
	}
	for _, k := range []string{"B1", "B2"} {
		if v := vals[k]; v != "" && (bytes.Contains(body, []byte("bp+"+v+",")) || bytes.Contains(body, []byte("bp + "+v+")"))) {
			return fmt.Errorf("%s: bp+%s used outside the cold path", name, v)
		}
	}
	return nil
}

// cellWrite returns the statement of s that writes the cell at bp+off, or
// a field of it, or nil.
func cellWrite(s ast.Stmt, off int) ast.Node {
	var found ast.Node
	writes := func(e ast.Expr) bool {
		hit := false
		ast.Inspect(e, func(n ast.Node) bool {
			if x, ok := n.(ast.Expr); ok {
				if k, ok := frameCell(x); ok && k == off {
					hit = true
				}
			}
			return !hit
		})
		return hit
	}
	ast.Inspect(s, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				if writes(l) {
					found = x
				}
			}
		case *ast.IncDecStmt:
			if writes(x.X) {
				found = x
			}
		}
		return true
	})
	return found
}

// coldFrames replaces what is left of the frame of each function of names
// with a Go array, as fixFrames does, or drops it when nothing is left, and
// checks that the function reaches a stack check only through its cold
// function. pin is set for lib/ccgo_darwin_arm64.go, whose frame sizes and
// cell counts must be those coldSites lists.
func coldFrames(b []byte, hw []*ast.File, names []string, pin bool) ([]byte, []string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	g := callGraph(append(hw[:len(hw):len(hw)], f))
	for _, name := range names {
		n := g[name]
		if n == nil {
			return nil, nil, fmt.Errorf("%s not found", name)
		}
		callees := map[string]bool{}
		for c := range n.callees {
			if c != coldSites[name].cold {
				callees[c] = true
			}
		}
		g[name] = &callNode{callees: callees, indirect: n.indirect, checks: n.checks}
	}
	reach := reaching(g)
	var problems []string
	for _, name := range names {
		site := coldSites[name]
		if r := reach[name]; r != "" {
			problems = append(problems, fmt.Sprintf("%s can reach a stack check other than through %s, through %s", name, site.cold, r))
			continue
		}
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, nil, err
		}
		var frame, cells int
		fixed := body
		switch {
		case bytes.Contains(body, []byte(framesInGo)):
			frame, cells, err = localStats(name, body)
		case bytes.Contains(body, []byte(noFrameLeft)):
		case bytes.Contains(body, []byte("\n\tbp := tls.Alloc(")):
			fixed, frame, cells, err = coldRestFrame(name, body)
		default:
			err = fmt.Errorf("%s: no TLS frame", name)
		}
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if pin && (frame != site.frame || cells != site.cells) {
			problems = append(problems, fmt.Sprintf("%s: a frame of %d bytes with %d cell uses left, coldSites has %d and %d", name, frame, cells, site.frame, site.cells))
			continue
		}
		b = bytes.Replace(b, body, fixed, 1)
	}
	return b, problems, nil
}

const noFrameLeft = "// Changed by internal/fixlib: no TLS frame left (see fixColdFrames).\n"

// coldRestFrame replaces the frame of a function whose cold path has gone
// with a Go array (frameFunc), or drops it when bp is not used any more.
func coldRestFrame(name string, body []byte) ([]byte, int, int, error) {
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, 0, 0, err
	}
	list := f.decl.Body.List
	n, ok := frameHead(list)
	if !ok {
		return nil, 0, 0, fmt.Errorf("%s: the frame is not allocated at the entry", name)
	}
	used := false
	for _, s := range list[2:] {
		ast.Inspect(s, func(x ast.Node) bool {
			if id, ok := x.(*ast.Ident); ok && id.Name == "bp" {
				used = true
			}
			return !used
		})
	}
	if used {
		return frameFunc(name, body)
	}
	start, _ := f.line(list[0])
	_, end := f.line(list[1])
	return f.apply([]edit{{start, end, "\t" + noFrameLeft}}), n, 0, nil
}
