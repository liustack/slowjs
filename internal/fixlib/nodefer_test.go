// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every generated file is what fixNoDefer makes of it with the defers put
// back, no defer frees a TLS frame, and each function that allocated one
// frees it before every return.
func TestNoDeferEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		b, err := restoreDefer(raw)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if bytes.Equal(b, raw) {
			t.Fatalf("%s: no defer put back", fn)
		}
		out, err := dropDefers(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%s: fixNoDefer does not give the file", fn)
		}
		frames := bytes.Count(b, []byte("\tdefer tls.Free(")) + bytes.Count(b, []byte("\t"+allocaDefer))
		if marks := bytes.Count(raw, []byte(noDeferMark)); marks != frames || marks < 600 {
			t.Errorf("%s: %d marks for %d frames freed by a defer", fn, marks, frames)
		}
		if err := checkFreed(raw); err != nil {
			t.Errorf("%s: %v", fn, err)
		}
	}
}

// checkFreed checks a file fixNoDefer made, on its own: every return of a
// function with the mark is right after the free, a void function that can
// fall off its end frees there, and no defer is left but those of
// keptDefers.
func checkFreed(b []byte) error {
	fset, funcs, err := fileFuncs(b)
	if err != nil {
		return err
	}
	for _, d := range funcs {
		var defers []string
		ast.Inspect(d.Body, func(n ast.Node) bool {
			if x, ok := n.(*ast.DeferStmt); ok {
				defers = append(defers, string(b[fset.Position(x.Pos()).Offset:fset.Position(x.End()).Offset]))
			}
			return true
		})
		for _, x := range defers {
			if keptDefers[d.Name.Name] != x {
				return fmt.Errorf("%s: %s", d.Name.Name, x)
			}
		}
		fr, err := frameOf(b, fset, d, true)
		if err != nil || fr == nil {
			if err != nil {
				return err
			}
			continue
		}
		rets, err := returns(d.Name.Name, d.Body.List, fr.at, nil)
		if err != nil {
			return err
		}
		free := fr.free[len(fr.free)-1]
		for _, r := range rets {
			i := r.i - 1
			if i < 0 {
				return fmt.Errorf("%s: a return first in its block", d.Name.Name)
			}
			s := unlabel(r.list[i])
			got := string(b[fset.Position(s.Pos()).Offset:fset.Position(s.End()).Offset])
			if got != free {
				return fmt.Errorf("%s: a return after %q", d.Name.Name, got)
			}
			if i > 0 {
				prev := unlabel(r.list[i-1])
				if string(b[fset.Position(prev.Pos()).Offset:fset.Position(prev.End()).Offset]) == free {
					return fmt.Errorf("%s: freed twice before a return", d.Name.Name)
				}
			}
		}
		if fr.result == "" && fallsOff(d.Body.List) {
			last := unlabel(d.Body.List[len(d.Body.List)-1])
			if string(b[fset.Position(last.Pos()).Offset:fset.Position(last.End()).Offset]) != free {
				return fmt.Errorf("%s: falls off its end without the free", d.Name.Name)
			}
		}
	}
	return nil
}

// Each form of return gets the free, after its value is evaluated, a
// return before the frame is allocated does not, and restoreDefer gives the
// input back.
func TestNoDeferReturns(t *testing.T) {
	const in = `package p

func f1(tls *libc.TLS, x int32) (r int32) {
	bp := tls.Alloc(16)
	defer tls.Free(16)
	if x == 0 {
		return g(tls, bp)
	}
	if x == 1 {
		return r
	}
	r = 2
	goto out
out:
	return TJSValue{
		Fu: x,
	}.Ftag
}

func f2(tls *libc.TLS, x int32) {
	bp1 := tls.Alloc(32)
	defer tls.Free(32)
	switch x {
	case 1:
		return
	}
	g(tls, bp1)
	goto _1
_1:
}

func f3(tls *libc.TLS, x int32) {
	bp := tls.Alloc(16)
	defer tls.Free(16)
	for {
		if x == 0 {
			goto done
		}
	}
done:
	return
}

func f4(tls *libc.TLS, ctx uintptr) (r int32) {
	if r, ok := pre(tls, ctx); ok {
		return r
	}
	bp := tls.Alloc(16)
	defer tls.Free(16)
	return g(tls, bp)
}

func f5(tls *libc.TLS, n int32) (r5 TJSValue) {
	frame_size := 160
	var frame_malloc uintptr
	bp := tls.Alloc(frame_size)
	defer func() {
		if frame_malloc != 0 {
			libc.Xfree(tls, frame_malloc)
		}
		tls.Free(frame_size)
	}()
	if n > 0 {
		tls.Free(frame_size)
		frame_size = 160 + int(n)
		bp = tls.Alloc(frame_size)
	}
	return h(tls, bp)
}

func f6(tls *libc.TLS) {
	bp := tls.Alloc(16)
	defer tls.Free(16)
}

func f7(tls *libc.TLS, x int32) (r int32) {
	return x
}
`
	const want = `package p

func f1(tls *libc.TLS, x int32) (r int32) {
	bp := tls.Alloc(16)
	` + noDeferMark + `	if x == 0 {
		r = g(tls, bp)
		tls.Free(16)
		return
	}
	if x == 1 {
		tls.Free(16)
		return r
	}
	r = 2
	goto out
out:
	r = TJSValue{
		Fu: x,
	}.Ftag
	tls.Free(16)
	return
}

func f2(tls *libc.TLS, x int32) {
	bp1 := tls.Alloc(32)
	` + noDeferMark + `	switch x {
	case 1:
		tls.Free(32)
		return
	}
	g(tls, bp1)
	goto _1
_1:
	tls.Free(32)
}

func f3(tls *libc.TLS, x int32) {
	bp := tls.Alloc(16)
	` + noDeferMark + `	for {
		if x == 0 {
			goto done
		}
	}
done:
	tls.Free(16)
	return
}

func f4(tls *libc.TLS, ctx uintptr) (r int32) {
	if r, ok := pre(tls, ctx); ok {
		return r
	}
	bp := tls.Alloc(16)
	` + noDeferMark + `	r = g(tls, bp)
	tls.Free(16)
	return
}

func f5(tls *libc.TLS, n int32) (r5 TJSValue) {
	frame_size := 160
	var frame_malloc uintptr
	bp := tls.Alloc(frame_size)
	` + noDeferMark + `	if n > 0 {
		tls.Free(frame_size)
		frame_size = 160 + int(n)
		bp = tls.Alloc(frame_size)
	}
	r5 = h(tls, bp)
	if frame_malloc != 0 {
		libc.Xfree(tls, frame_malloc)
	}
	tls.Free(frame_size)
	return
}

func f6(tls *libc.TLS) {
	bp := tls.Alloc(16)
	` + noDeferMark + `	tls.Free(16)
}

func f7(tls *libc.TLS, x int32) (r int32) {
	return x
}
`
	out, err := dropDefers([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Fatalf("got\n%s", out)
	}
	if err := checkFreed(out); err != nil {
		t.Fatal(err)
	}
	// Formatted as gofmt has it.
	if _, err := parser.ParseFile(token.NewFileSet(), "", out, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := fixNoDefer(out); err == nil || !strings.Contains(err.Error(), "restoreDefer goes first") {
		t.Fatalf("fixNoDefer on its output: %v", err)
	}
}

// fixNoDefer stops at a frame or a defer it does not know, and restoreDefer
// at a function with the mark that it did not write.
func TestNoDeferRejects(t *testing.T) {
	frame := func(body string) string {
		return "package p\n\nfunc f(tls *libc.TLS, x int32) (r int32) {\n" + body + "}\n"
	}
	for _, c := range []struct {
		name, src, want string
	}{
		{"sizes differ", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(32)\n\treturn 0\n"), `not freed by "defer tls.Free(16)"`},
		{"no defer", frame("\tbp := tls.Alloc(16)\n\treturn g(tls, bp)\n"), "not freed by"},
		{"defer later", frame("\tbp := tls.Alloc(16)\n\tg(tls, bp)\n\tdefer tls.Free(16)\n\treturn 0\n"), "not freed by"},
		{"another defer", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\tdefer libc.Xfree(tls, bp)\n\treturn 0\n"), "f: defer libc.Xfree(tls, bp)"},
		{"another defer without a frame", frame("\tdefer libc.Xfree(tls, x)\n\treturn 0\n"), "f: defer libc.Xfree(tls, x)"},
		{"function literal", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\tg := func() int32 { return 1 }\n\treturn g()\n"), "a function literal"},
		{"local named as the result", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\tif x > 0 {\n\t\tr := x\n\t\t_ = r\n\t}\n\treturn 0\n"), "a local named r"},
		{"bare return", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\tr = 1\n\treturn\n"), "a return without its one value"},
		{"second frame", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\tp := tls.Alloc(8)\n\treturn g(tls, p)\n"), "p := tls.Alloc(8) is not the frame"},
		{"free in the body", frame("\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\ttls.Free(16)\n\treturn 0\n"), "tls.Free(16) outside the defer"},
		{"label before the frame", frame("again:\n\tif x > 0 {\n\t\treturn 0\n\t}\n\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\tgoto again\n"), "a label before the frame"},
		{"unnamed result", "package p\n\nfunc f(tls *libc.TLS) int32 {\n\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n\treturn 0\n}\n", "a result that is not one named value"},
	} {
		_, err := dropDefers([]byte(c.src))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}

	marked := func(body string) string {
		return "package p\n\nfunc f(tls *libc.TLS, x int32) (r int32) {\n\tbp := tls.Alloc(16)\n\t" + noDeferMark + body + "}\n"
	}
	for _, c := range []struct {
		name, src, want string
	}{
		{"return without the free", marked("\tif x > 0 {\n\t\treturn 1\n\t}\n\ttls.Free(16)\n\treturn r\n"), "without the free before it"},
		{"free of another size", marked("\ttls.Free(32)\n\treturn r\n"), "without the free before it"},
		{"bare return without the value", marked("\ttls.Free(16)\n\treturn\n"), "a bare return without r = X"},
	} {
		_, err := restoreDefer([]byte(c.src))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}

	// Freed twice: restoreDefer leaves one free, which fixNoDefer rejects.
	b, err := restoreDefer([]byte(marked("\ttls.Free(16)\n\ttls.Free(16)\n\treturn r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixNoDefer(b); err == nil || !strings.Contains(err.Error(), "outside the defer") {
		t.Errorf("freed twice: got %v", err)
	}
}

// A function upstream adds with a frame, or a defer in a new place, stops
// fixlib on the real files too.
func TestNoDeferRejectsInFile(t *testing.T) {
	fn := filepath.Join("..", "..", "lib", "ccgo_darwin_arm64.go")
	b, err := readEngine(fn)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, add, want string
	}{
		{"defer in a new function", "\nfunc _new_helper(tls *libc.TLS, p uintptr) {\n\tdefer libc.Xfree(tls, p)\n}\n", "_new_helper: defer libc.Xfree(tls, p)"},
		{"frame freed later", "\nfunc _new_helper(tls *libc.TLS) (r int32) {\n\tbp := tls.Alloc(16)\n\tr = _g(tls, bp)\n\tdefer tls.Free(16)\n\treturn\n}\n", "_new_helper: a TLS frame that is not freed"},
	} {
		if _, err := fixNoDefer(append(b[:len(b):len(b)], c.add...)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// fixNoDefer leaves a function fixLazyFrames changed as it is: its TLS
// frames are freed right after the statements that take them. It stops
// when such a function takes or frees a frame some other way.
func TestNoDeferLazyFrames(t *testing.T) {
	lazy := func(body string) string {
		return "package p\n\nfunc f(tls *libc.TLS, x int32) (r int32) {\n\t" + lazyHeadMark(16) + "\tvar localbp [2]uint64\n\tvar bp uintptr\n" + body + "}\n"
	}
	wrap := func(alloc, stmt, free string) string {
		return "\t" + lazyStmtMark + "\t" + alloc + "\n\t" + stmt + "\n\t" + free + "\n"
	}
	ret := "\t" + lazyReturnMark + "\tbp = tls.Alloc(16)\n\tr = g(tls, bp)\n\ttls.Free(16)\n\treturn\n"
	good := lazy(wrap("bp = tls.Alloc(16)", "x = g(tls, bp)", "tls.Free(16)") + wrap("tls.Alloc(16)", "x = h(tls)", "tls.Free(16)") + "\t*(*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(&localbp)))) = x\n" + ret)
	out, err := dropDefers([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != good {
		t.Errorf("fixNoDefer changed a function of fixLazyFrames:\n%s", out)
	}
	if back, err := restoreDefer(out); err != nil || string(back) != good {
		t.Errorf("restoreDefer changed a function of fixLazyFrames: %v", err)
	}
	for _, c := range []struct {
		name, src, want string
	}{
		{"another size", lazy(wrap("bp = tls.Alloc(32)", "x = g(tls, bp)", "tls.Free(32)") + "\treturn x\n"), "f: tls.Alloc(32) in a function whose TLS frame is 16 bytes"},
		{"free of another size", lazy(wrap("bp = tls.Alloc(16)", "x = g(tls, bp)", "tls.Free(32)") + "\treturn x\n"), "f: tls.Free(32) in a function whose TLS frame is 16 bytes"},
		{"no mark", lazy("\tbp = tls.Alloc(16)\n\tx = g(tls, bp)\n\ttls.Free(16)\n\treturn x\n"), "f: 1 tls.Alloc and 1 tls.Free for 0 statements marked"},
		{"not freed", lazy("\t" + lazyStmtMark + "\tbp = tls.Alloc(16)\n\tx = g(tls, bp)\n\treturn x\n"), "f: 1 tls.Alloc and 0 tls.Free for 1 statements marked"},
		{"defer", lazy("\tdefer tls.Free(16)\n\treturn x\n"), "f: defer tls.Free(16)"},
		{"another variable", lazy(wrap("p := tls.Alloc(16)", "x = g(tls, p)", "tls.Free(16)") + "\treturn x\n"), "f: p := tls.Alloc(16) is not bp = tls.Alloc(16)"},
	} {
		_, err := dropDefers([]byte(c.src))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}
