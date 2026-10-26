// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/arch/x86/x86asm"
)

func TestICallID(t *testing.T) {
	for _, code := range []string{">v", "i>i", "ii>i", "iid>d", "iiii>j", "ij>v", "if>f", "iiiiiiiiiiii>j"} {
		id, ok := ICallID(code)
		if !ok || id>>31 != 1 {
			t.Fatalf("%s: id %#x, %v", code, id, ok)
		}
		back, err := ICallCode(id)
		if err != nil || back != code {
			t.Fatalf("%s: %#x decodes as %q, %v", code, id, back, err)
		}
	}
	// Different codes, different ids.
	a, _ := ICallID("ii>i")
	b, _ := ICallID("i>i")
	c, _ := ICallID("id>i")
	if a == b || a == c || b == c {
		t.Fatalf("ids collide: %#x %#x %#x", a, b, c)
	}
	for _, code := range []string{"iiiiiiiiiiiii>i", "iB>i", "i...>i", "i?>i", "i>?", "i", "i>ii"} {
		if id, ok := ICallID(code); ok || id != ICallUnencodable {
			t.Fatalf("%s: encoded as %#x", code, id)
		}
	}
	// An id the pass gives no signature, or the unencodable one, decodes
	// to an error.
	for _, id := range []uint32{0, ICallUnencodable, 0x7fffffff, 1<<31 | 1<<27 | 5<<24, 1<<31 | 1<<27 | 1<<24 | 1<<2} {
		if code, err := ICallCode(id); err == nil {
			t.Fatalf("%#x decodes as %q", id, code)
		}
	}
	for name, want := range map[string]string{
		"goasm.icall.f.0.2164260865":     "f 0 2164260865",
		"goasm.icall.cr_op.156.3.42":     "cr_op.156 3 42",
		"goasm.icall.f.x.1":              "",
		"goasm.icall.f.1":                "",
		"goasm.icall..0.1":               "",
		"other.f.0.1":                    "",
		"goasm.icall.f.0.99999999999999": "",
	} {
		fn, n, id, ok := ParseICallSymbol(name)
		got := ""
		if ok {
			got = fn + " " + strconv.Itoa(n) + " " + strconv.FormatUint(uint64(id), 10)
		}
		if got != want {
			t.Fatalf("%s: %q, want %q", name, got, want)
		}
	}
}

// TestICallSymbols compiles testdata/icall through the pass with
// -shadow-icall-sigs and llc with the plugin. Every indirect call in llc's
// object must have a symbol at its address carrying the signature the pass
// reported for that function's indirect calls, the text must have the label
// right before the call, and no symbol may sit anywhere else. Without the
// pass option, llc with the plugin must stop. It needs LLVM 22.1.8
// (SLOWJS_LLVM) and the built pass (SLOWJS_SHADOW_PASS).
func TestICallSymbols(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("decodes amd64 code")
	}
	work := t.TempDir()
	run := func(name string, args ...string) (string, error) {
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		return string(out), err
	}
	must := func(name string, args ...string) string {
		t.Helper()
		out, err := run(name, args...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return out
	}
	markers, err := sigMarkers(filepath.Join("testdata", "icall", "icall.c"))
	if err != nil {
		t.Fatal(err)
	}
	ir, shadow := filepath.Join(work, "c.ll"), filepath.Join(work, "c.shadow.ll")
	must("clang", append(append([]string{"--target=x86_64-linux-gnu", "-gline-tables-only"}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir,
		filepath.Join("testdata", "icall", "icall.c"))...)
	for _, mode := range []string{"body", "pn"} {
		args := []string{"-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-report", "-shadow-icall-sigs"}
		if mode == "pn" {
			args = append(args, "-shadow-cc=preserve_none")
		}
		rep := ParseReport(must("opt", append(args, "-S", "-o", shadow, ir)...))
		asm, obj := filepath.Join(work, mode+".s"), filepath.Join(work, mode+".o")
		must("llc", "-O2", "-relocation-model=pic", "-enable-shrink-wrap=false", "-load", pass, "-o", asm, shadow)
		must("llc", "-O2", "-relocation-model=pic", "-enable-shrink-wrap=false", "-load", pass, "-filetype=obj", "-o", obj, shadow)
		text, err := os.ReadFile(asm)
		if err != nil {
			t.Fatal(err)
		}
		// The label is the line right before its call (debug line tables put
		// .loc lines between them). Each call, in the order of the text, is
		// held to the signature C gives its source line.
		calls, err := icallsInText(string(text))
		if err != nil || len(calls) == 0 {
			t.Fatalf("%s: %v\n%s", mode, err, text)
		}
		if err := checkICallsBySource(calls, markers); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		labels := calls
		f, err := elf.Open(obj)
		if err != nil {
			t.Fatal(err)
		}
		syms, err := f.Symbols()
		if err != nil {
			t.Fatal(err)
		}
		code, err := f.Section(".text").Data()
		if err != nil {
			t.Fatal(err)
		}
		at := map[uint64]string{}
		var inObj []string
		got := map[string]map[string]bool{}
		for _, s := range syms {
			if !strings.HasPrefix(s.Name, ICallPrefix) {
				continue
			}
			fn, _, id, ok := ParseICallSymbol(s.Name)
			c, err := ICallCode(id)
			if !ok || err != nil || elf.ST_BIND(s.Info) != elf.STB_LOCAL || f.Sections[s.Section].Name != ".text" {
				t.Fatalf("%s: symbol %s: %v", mode, s.Name, err)
			}
			at[s.Value] = s.Name
			if got[fn] == nil {
				got[fn] = map[string]bool{}
			}
			got[fn][c] = true
		}
		n := 0
		for pc := 0; pc < len(code); {
			in, err := x86asm.Decode(code[pc:], 64)
			if err != nil {
				t.Fatalf("%s: decode at %#x: %v", mode, pc, err)
			}
			_, reg := in.Args[0].(x86asm.Reg)
			name, sym := at[uint64(pc)]
			if in.Op == x86asm.CALL && reg {
				n++
				if !sym {
					t.Fatalf("%s: the indirect call at %#x has no symbol", mode, pc)
				}
			} else if sym {
				t.Fatalf("%s: %s at %#x is on %v, not an indirect call", mode, name, pc, in)
			}
			pc += in.Len
		}
		if n != len(at) || n != len(labels) {
			t.Fatalf("%s: %d indirect calls, %d symbols, %d labels", mode, n, len(at), len(labels))
		}
		// The object's symbols, by address, are the text's labels, in
		// order: each call in the object has the id of the call at that
		// place in the text, which the source lines above vouch for.
		var addrs []uint64
		for a := range at {
			addrs = append(addrs, a)
		}
		sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
		for i, a := range addrs {
			inObj = append(inObj, at[a])
			if at[a] != labels[i].name {
				t.Fatalf("%s: the call at %#x has %s, the text has %s at that place in the order", mode, a, at[a], labels[i].name)
			}
		}
		for _, fn := range []string{"branches", "mixed", "pair", "loop"} {
			want := map[string]bool{}
			for _, c := range rep.ICalls[fn] {
				want[c] = true
			}
			if strings.Join(SortedKeys(got[fn]), " ") != strings.Join(SortedKeys(want), " ") || len(want) == 0 {
				t.Fatalf("%s: %s's calls carry %v, the pass reported %v", mode, fn, SortedKeys(got[fn]), SortedKeys(want))
			}
		}
		if n := len(got["branches"]); n != 2 {
			t.Fatalf("%s: branches has %d signatures, want 2", mode, n)
		}
	}
	// Without ids the plugin stops llc.
	must("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-S", "-o", shadow, ir)
	out, err := run("llc", "-O2", "-load", pass, "-filetype=obj", "-o", filepath.Join(work, "bare.o"), shadow)
	if err == nil || !strings.Contains(out, "indirect call without a signature id") {
		t.Fatalf("llc with the plugin took indirect calls without ids: %v\n%s", err, out)
	}
}

// icallAt is one indirect call in llc's text: the label before it and the
// source line of the call, from the last .loc at or before the call itself.
type icallAt struct {
	name string
	line int
}

var reLoc = regexp.MustCompile(`^\s*\.loc\s+\d+\s+(\d+)\b`)

// icallsInText lists the indirect calls of llc's text in order. A label must
// be followed, past .loc lines and comments, by an indirect call, and
// every indirect call must have a label.
func icallsInText(asm string) ([]icallAt, error) {
	var out []icallAt
	line, pending := 0, ""
	for _, l := range strings.Split(asm, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case reLoc.MatchString(l):
			line, _ = strconv.Atoi(reLoc.FindStringSubmatch(l)[1])
		case strings.HasPrefix(t, ICallPrefix) && strings.HasSuffix(t, ":"):
			if pending != "" {
				return nil, fmt.Errorf("label %s is followed by label %s, not a call", pending, t)
			}
			pending = strings.TrimSuffix(t, ":")
		case t == "" || t[0] == '.' || t[0] == '#' || strings.HasSuffix(t, ":"):
		default:
			ind := strings.HasPrefix(t, "callq") && strings.Contains(t, "*%")
			switch {
			case pending != "" && ind:
				out = append(out, icallAt{pending, line})
				pending = ""
			case pending != "":
				return nil, fmt.Errorf("label %s is followed by %q, not an indirect call", pending, t)
			case ind:
				return nil, fmt.Errorf("the indirect call %q has no label", t)
			}
		}
	}
	if pending != "" {
		return nil, fmt.Errorf("label %s is followed by no call", pending)
	}
	return out, nil
}

var reSigMarker = regexp.MustCompile(`/\* sig: (\S+) \*/`)

// sigMarkers reads the signature comments of a C file by line. The pass
// counts the shadow-stack pointer as a first integer parameter of every
// call, which C does not write.
func sigMarkers(path string) (map[int]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[int]string{}
	for i, l := range strings.Split(string(b), "\n") {
		if g := reSigMarker.FindStringSubmatch(l); g != nil {
			m[i+1] = g[1]
		}
	}
	return m, nil
}

// checkICallsBySource holds each call's id to the signature C gives the
// call's source line, one call at a time: the pass's id, which is a number
// the check cannot recompute from anywhere else, against the types in the
// source.
func checkICallsBySource(calls []icallAt, markers map[int]string) error {
	for _, c := range calls {
		_, _, id, ok := ParseICallSymbol(c.name)
		got, err := ICallCode(id)
		if !ok || err != nil {
			return fmt.Errorf("%s: %v", c.name, err)
		}
		want, ok := markers[c.line]
		if !ok {
			return fmt.Errorf("%s is on line %d, which has no sig: comment", c.name, c.line)
		}
		if want = "i" + want; got != want {
			return fmt.Errorf("%s on line %d carries %s, C gives the call %s", c.name, c.line, got, want)
		}
	}
	return nil
}

// The per-call check refuses ids that were swapped or put on the wrong call,
// which a comparison of the set of ids per function cannot see.
func TestICallBySource(t *testing.T) {
	id := func(code string) uint32 { v, _ := ICallID(code); return v }
	text := func(a, b string) string {
		return fmt.Sprintf("f:\n\t.loc 0 10 3\n%s%d:\n\t.loc 0 10 8\n\tcallq *%%rax\n\t.loc 0 11 3\n%s%d:\n\tcallq *%%rbx\n",
			ICallPrefix+"f.0.", id(a), ICallPrefix+"f.1.", id(b))
	}
	markers := map[int]string{10: "i>i", 11: "ii>i"}
	calls, err := icallsInText(text("ii>i", "iii>i"))
	if err != nil || len(calls) != 2 || calls[0].line != 10 || calls[1].line != 11 {
		t.Fatalf("%+v, %v", calls, err)
	}
	if err := checkICallsBySource(calls, markers); err != nil {
		t.Fatal(err)
	}
	// The same two ids on the other two calls: each function keeps its set of
	// signatures, and each call is wrong.
	calls, _ = icallsInText(text("iii>i", "ii>i"))
	if err := checkICallsBySource(calls, markers); err == nil || !strings.Contains(err.Error(), "C gives the call") {
		t.Fatalf("swapped ids accepted: %v", err)
	}
	for name, asm := range map[string]string{
		"a label on a direct call": "f:\n" + ICallPrefix + "f.0.1:\n\tcallq g\n",
		"a call with no label":     "f:\n\tcallq *%rax\n",
		"a label on no call":       "f:\n" + ICallPrefix + "f.0.1:\n\tmovq %rax, %rbx\n",
		"a label at the end":       "f:\n" + ICallPrefix + "f.0.1:\n",
		"two labels in a row":      "f:\n" + ICallPrefix + "f.0.1:\n" + ICallPrefix + "f.1.1:\n\tcallq *%rax\n",
	} {
		if _, err := icallsInText(asm); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := checkICallsBySource([]icallAt{{ICallPrefix + "f.0." + strconv.Itoa(int(id("i>i"))), 99}}, markers); err == nil {
		t.Error("a call on a line with no sig: comment accepted")
	}
}
