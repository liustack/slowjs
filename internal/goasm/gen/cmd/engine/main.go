// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Whole-engine driver: compile every C file of engine/ and engine/libc
// freestanding with LLVM 22.1.8, link them into one module, run the
// shadow-stack pass, llc, the checks and the lowering over every function,
// and emit Go assembly split by C source file, the read-only data and the Go
// declarations. With -keep-going it does not stop at the first function it
// cannot lower: it lists every rejection by class, emits a stub for each
// such function and still writes a package that builds.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	gen "github.com/liustack/slowjs/internal/goasm/gen"
)

var (
	engineDir = flag.String("engine", "engine", "the C tree")
	llvm      = flag.String("llvm", "/usr/lib/llvm-22/bin", "LLVM 22.1.8 bin dir")
	pass      = flag.String("pass", "", "shadow-stack pass plugin")
	arch      = flag.String("arch", "amd64", "amd64, or arm64 (up to llc and a survey of its output: the arm64 lowering takes no whole-engine input yet)")
	mode      = flag.String("mode", "body", "body or pn (preserve_none)")
	work      = flag.String("work", "_engine_work", "work dir")
	out       = flag.String("out", "", "Go package dir to write (empty: no output)")
	pkg       = flag.String("pkg", "cengine", "Go package name")
	reportOut = flag.String("report", "", "write the rejections as JSON here")
	maxFile   = flag.Int("max-file", 512<<10, "most bytes in one .s file")
	leaves    = flag.String("leaves", "memcpy,memmove,memset", "register-entry leaves (llc calls them by itself)")
	extra     = flag.String("allow", "malloc,realloc,free,malloc_usable_size", "external symbols allowed besides the host callbacks")
	keepGoing = flag.Bool("keep-going", false, "survey only: list every rejection and write a package with a stub for each function rejected, instead of stopping at once with nothing written")
	jobs      = flag.Int("j", 2, "parallel clang jobs")
	looseVar  = flag.Bool("loose-varargs", false, "lower variadic calls with their fixed arguments only (wrong code, to measure the output)")
	varBlock  = flag.Bool("varargs-block", false, "experimental: the pass passes variable arguments in a shadow-stack block")
	verifyBin = flag.String("verify-bin", "", "a linked binary of the output package: match every generated function and engine_rodata against llc's object")
	verifyPkg = flag.String("verify-pkg", "", "the output package's import path (link symbol prefix) for -verify-bin")
	verifyAll = flag.Bool("verify-all", false, "with -verify-bin: fail unless every function was in the binary and checked (build it with -tags goasmkeep)")
	nonFormat = flag.String("non-format-variadic", defaultNonFormat, "variadic functions whose extra arguments are not printf arguments, left out of the format check (for -src tests)")
	srcList   = flag.String("src", "", "comma-separated C files to compile instead of the engine tree (for tests)")
	sizeLog2  = flag.Int("shadow-size-log2", 20, "log2 of the shadow-stack region the engine is generated for (the host's region must be at least as big); engine_decl.go records it")
	release   = flag.Bool("release", false, "formal build: refuse every switch made for surveys, tests and negative builds, and fail if any function would be a stub")
	poisonF   = flag.Bool("poison", false, "negative build: write garbage into the registers at each entry, after each unified call and at each return (the engine's tests must still pass; the post-link check does not take it)")
	pollOn    = flag.Bool("poll", true, "count every cycle of the code and call host_poll when the count runs out, where Go can stop the goroutine; -poll=false, which -release refuses, builds an engine without polls for comparison")
	pollOff   = flag.Int("poll-offset", gen.PollCounterOffset, "offset of the poll counter from the base of the shadow-stack region (engine_decl.go records it)")
)

var defaultNonFormat = strings.Join(gen.EngineNonFormatVariadic, ",")

// Rejection is one function the generator could not take, or one rule the
// C breaks.
type Rejection struct {
	Func, Stage, Class, Msg string
}

var rejections []Rejection

func reject(fn, stage, msg string) {
	rejections = append(rejections, Rejection{fn, stage, classify(msg), msg})
}

func main() {
	flag.Parse()
	t0 := time.Now()
	if err := gen.CheckShadowSizeLog2(*sizeLog2); err != nil {
		die(err)
	}
	keepNarrow, err := keepNarrowFromEnv(os.Getenv("GEN_KEEP_NARROW"))
	if err != nil {
		die(err)
	}
	gen.KeepNarrowSigs = keepNarrow
	editLoweredEnv := os.Getenv("GEN_EDIT_LOWERED")
	if *release {
		if err := releaseError(devSwitches{KeepGoing: *keepGoing, LooseVarargs: *looseVar, Poison: *poisonF, KeepNarrow: keepNarrow,
			EditLowered: editLoweredEnv, Arch: *arch, NonFormat: *nonFormat, NoPoll: !*pollOn}, *out); err != nil {
			die(err)
		}
	}
	if *arch != "amd64" && *arch != "arm64" {
		die(fmt.Errorf("-arch %s: amd64 or arm64", *arch))
	}
	if err := gen.CheckPollOffset(*pollOff, *sizeLog2); err != nil {
		die(err)
	}
	must(os.MkdirAll(*work, 0o755))
	clang := filepath.Join(*llvm, "clang")

	// 1. Compile every C file to IR.
	type src struct{ path, group string }
	var srcs []src
	for _, f := range []string{"quickjs", "libregexp", "libunicode", "cutils", "dtoa", "heap", "tlsf"} {
		srcs = append(srcs, src{filepath.Join(*engineDir, f+".c"), f})
	}
	for _, dir := range []string{"libc/string", "libc/math"} {
		ms, _ := filepath.Glob(filepath.Join(*engineDir, dir, "*.c"))
		for _, m := range ms {
			srcs = append(srcs, src{m, "libc"})
		}
	}
	for _, f := range []string{"math.c", "stdio.c", "abort.c"} {
		srcs = append(srcs, src{filepath.Join(*engineDir, "libc", f), "libc"})
	}
	if *srcList != "" {
		srcs = nil
		for _, f := range strings.Split(*srcList, ",") {
			srcs = append(srcs, src{f, strings.TrimSuffix(filepath.Base(f), ".c")})
		}
	}
	if gen.FlagsErr != nil {
		die(gen.FlagsErr)
	}
	cflags := append([]string{}, gen.EngineCFlags...)
	if *arch == "arm64" {
		cflags = append([]string{}, gen.ARM64CFlags...)
	}
	libc := filepath.Join(*engineDir, "libc")
	var lls []string
	defined := make([][]string, len(srcs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan bool, *jobs)
	var cerr error
	formatCalls := 0
	for i, s := range srcs {
		ll := filepath.Join(*work, fmt.Sprintf("%03d_%s.ll", i, strings.TrimSuffix(filepath.Base(s.path), ".c")))
		lls = append(lls, ll)
		args := append([]string{}, cflags...)
		if s.group == "libc" {
			// abort.c asks the host (engine/host.h).
			args = append(args, "-I"+libc, "-I"+filepath.Join(libc, "internal"), "-I"+*engineDir, "-include",
				filepath.Join(libc, "internal/features.h"))
		} else {
			args = append(args, "-I"+libc, "-I"+*engineDir, `-DCONFIG_VERSION="2026-06-04"`)
		}
		// The formats of the engine's own files are read in IR as written,
		// before the optimizer changes the calls that carry them.
		var plainLL string
		var plainArgs []string
		if s.group != "libc" {
			plainLL = strings.TrimSuffix(ll, ".ll") + ".plain.ll"
			plainArgs = append(append(append([]string{}, args...), "-O0"), "-S", "-emit-llvm", "-o", plainLL, s.path)
		}
		args = append(args, "-S", "-emit-llvm", "-o", ll, s.path)
		wg.Add(1)
		go func(s src, ll string, args []string) {
			defer wg.Done()
			sem <- true
			defer func() { <-sem }()
			if _, err := capture(clang, args...); err != nil {
				mu.Lock()
				cerr = err
				mu.Unlock()
				return
			}
			if plainLL != "" {
				if _, err := capture(clang, plainArgs...); err != nil {
					mu.Lock()
					cerr = err
					mu.Unlock()
					return
				}
				// The formats the engine's C gives its libc are all ones stdio.c takes.
				vs, n := gen.CountFormats(readFile(plainLL), strings.Split(*nonFormat, ","))
				mu.Lock()
				formatCalls += n
				for _, v := range vs {
					reject(v.Func, "format", v.Rule+": "+v.Line)
				}
				mu.Unlock()
			}
			b, _ := os.ReadFile(ll)
			var names []string
			for _, m := range reDefine.FindAllStringSubmatch(string(b), -1) {
				names = append(names, strings.Trim(m[1], `"`))
			}
			mu.Lock()
			defined[i] = names
			mu.Unlock()
		}(s, ll, args)
	}
	wg.Wait()
	if cerr != nil {
		die(cerr)
	}
	groups := make([]string, len(srcs))
	for i, s := range srcs {
		groups[i] = s.group
	}
	group := firstGroups(defined, groups)
	fmt.Fprintf(os.Stderr, "format strings: %d calls of the formatter checked\n", formatCalls)
	linked := filepath.Join(*work, "engine.ll")
	must1(capture(filepath.Join(*llvm, "llvm-link"), append([]string{"-S", "-o", linked}, lls...)...))
	tCompile := time.Since(t0)

	// 2. Rules the C breaks, as the IR shows them.
	ll := readFile(linked)
	pre := gen.CheckIRFuncs(ll)
	varDefs := map[string]bool{}
	for _, v := range pre {
		if v.Rule == "variadic definition" {
			varDefs[v.Func] = true
		}
	}

	// 3. The shadow-stack pass.
	// Integer argument registers of a unified call (as in cmd/gen): on
	// arm64 eight in C, six with preserve_none (x20 to x25).
	maxInt := map[string]int{"amd64": 6, "arm64": 8}[*arch]
	if *mode == "pn" {
		maxInt = map[string]int{"amd64": 10, "arm64": 6}[*arch]
	}
	shadowLL := filepath.Join(*work, "shadow.ll")
	// The probe module gives llc's registers of every unified entry's
	// arguments and result to the post-link check.
	probesLL := filepath.Join(*work, "probes.ll")
	os.Remove(probesLL)
	passArgs := []string{"-load-pass-plugin", *pass, "-passes=goasm-shadow-stack", "-shadow-add-sp",
		"-shadow-leaf=" + *leaves, fmt.Sprintf("-shadow-max-int-args=%d", maxInt), "-shadow-report",
		"-shadow-icall-sigs", fmt.Sprintf("-shadow-size-log2=%d", *sizeLog2), "-shadow-probes=" + probesLL}
	if keepNarrow {
		// Negative build: narrow integers as the generator once took them
		// (gen.KeepNarrowSigs makes SigFromCode follow).
		passArgs = append(passArgs, "-shadow-keep-narrow")
	}
	if *keepGoing {
		passArgs = append(passArgs, "-shadow-keep-going")
	}
	if *mode == "pn" {
		passArgs = append(passArgs, "-shadow-cc=preserve_none")
	}
	if *varBlock {
		passArgs = append(passArgs, "-shadow-varargs")
		varDefs = map[string]bool{}
	}
	if *pollOn {
		passArgs = append(passArgs, "-shadow-poll", fmt.Sprintf("-shadow-poll-offset=%d", *pollOff))
	}
	passArgs = append(passArgs, "-S", "-o", shadowLL, linked)
	passOut := must1(capture(filepath.Join(*llvm, "opt"), passArgs...))
	writeFile(filepath.Join(*work, "pass.txt"), passOut)
	rep := gen.ParseReport(passOut)
	for fn, why := range rep.Rejected {
		reject(fn, "pass", why)
	}
	post := gen.CheckIRFuncs(readFile(shadowLL))
	for _, v := range post {
		if v.Rule == "byval aggregate" || v.Rule == "sret aggregate" || v.Rule == "long double" || v.Rule == "thread-local storage" {
			reject(v.Func, "ir", v.Rule+": "+v.Line)
		}
	}
	for fn := range varDefs {
		reject(fn, "ir", "variadic definition")
	}

	// 4. llc, position independent: every data reference is RIP-relative.
	// The object keeps llc's temporary labels (.LCPI, .LJTI, .L.str), so the
	// post-link check can resolve every reference of the text in it.
	asmPath := filepath.Join(*work, "engine.s")
	objPath := filepath.Join(*work, "engine.o")
	os.Remove(asmPath)
	os.Remove(objPath)
	llc := filepath.Join(*llvm, "llc")
	// The pass's shared object also holds the llc plugins that write a
	// symbol with its signature at every indirect call and the registers
	// llc assigns (facts.cpp). Every function stays in .text: llc would put
	// a cold one (abort) in .text.unlikely., which the lowering and the
	// check take for one text.
	llcArgs := []string{"-O2", "-relocation-model=pic", "-enable-shrink-wrap=false", "-profile-guided-section-prefix=false",
		"-load", *pass}
	if *pollOn {
		// The tick in the interpreter's dispatch makes it a block of many
		// predecessors and successors, which llc no longer copies into the
		// end of every handler by default (gen.DispatchCopies checks it).
		llcArgs = append(llcArgs, gen.PollLLCFlags...)
	}
	factsPath := filepath.Join(*work, "engine.facts")
	probeFacts := filepath.Join(*work, "probes.facts")
	os.Remove(factsPath)
	os.Remove(probeFacts)
	must1(capture(llc, append(append([]string{}, llcArgs...), "-o", asmPath, shadowLL)...))
	must1(capture(llc, append(append([]string{}, llcArgs...), "-filetype=obj", "--save-temp-labels",
		"-goasm-facts="+factsPath, "-o", objPath, shadowLL)...))
	must1(capture(llc, append(append([]string{}, llcArgs...), "-goasm-facts="+probeFacts, "-o", os.DevNull, probesLL)...))
	tLLVM := time.Since(t0)

	allow := map[string]bool{}
	for _, h := range append(append([]string{}, gen.HostCallbacks...), gen.GeneratorCallbacks...) {
		allow[h] = true
	}
	for _, e := range strings.Split(*extra, ",") {
		allow[e] = true
	}
	funcs := map[string]bool{}
	for name, def := range rep.Defined {
		funcs[name] = true
		if !def && !allow[name] {
			reject(name, "object", "external reference to "+name+" outside the allow list")
		}
	}
	if err := gen.CheckObject(objPath, allowAll(allow, rep)); err != nil {
		reject("", "object", err.Error())
	}

	// 5. Split the text, place the data, lower every function.
	asm := readFile(asmPath)
	if *arch == "arm64" {
		surveyARM64(asm, rep)
		summary(rep, &gen.DataPlan{}, pre, nil, nil, len(rep.Defined))
		if *reportOut != "" {
			b, _ := json.MarshalIndent(rejections, "", " ")
			writeFile(*reportOut, string(b))
		}
		fmt.Fprintf(os.Stderr, "time: compile %v, llvm %v\n", tCompile, tLLVM)
		return
	}
	unit, err := gen.SplitAsm(asm, "#")
	if err != nil {
		die(err)
	}
	for name, target := range unit.Sets {
		funcs[name] = true
		_ = target
	}
	plan := gen.PlanData(unit, funcs)
	for fn, errs := range plan.Errors {
		for _, e := range errs {
			reject(fn, "data", e.Error())
		}
	}
	leafSet := map[string]bool{}
	for _, l := range strings.Split(*leaves, ",") {
		leafSet[l] = true
	}
	var specs []gen.FuncSpec
	sigs := map[string]gen.Sig{}
	for _, name := range gen.SortedKeys(rep.Sigs) {
		code := rep.Sigs[name]
		s, err := gen.SigFromCode(strings.Replace(code, "...", "", 1))
		if err != nil {
			reject(name, "signature", err.Error())
			continue
		}
		sigs[name] = s
		kind := gen.Unified
		if leafSet[name] {
			kind = gen.Register
		}
		specs = append(specs, gen.FuncSpec{Name: name, Sig: s, Kind: kind})
	}
	gen.SetCallees(specs)
	amd := gen.AMD64()
	if *mode == "pn" {
		amd.PreserveNone = true
	}
	if *poisonF && *verifyBin != "" {
		die(fmt.Errorf("-poison with -verify-bin: the post-link check takes no poison build"))
	}
	amd.Poison = *poisonF
	variadic := map[string]bool{}
	for name, code := range rep.Sigs {
		if strings.Contains(code, "...") {
			variadic[name] = true
		}
	}
	env := &gen.LowerEnv{Plan: plan, Funcs: funcs, Variadic: variadic, LooseVarargs: *looseVar}
	failed := map[string]bool{}
	for _, r := range rejections {
		// A function the pass emptied is a trap: its stub names it instead.
		if r.Func != "" && rep.Defined[r.Func] {
			failed[r.Func] = true
		}
	}
	var lowered strings.Builder
	lowered.WriteString(".text\n")
	var metas, stubs []gen.Meta
	var names []string
	for _, name := range unit.FuncOrder {
		names = append(names, name)
		s, ok := sigs[name]
		if !ok {
			failed[name] = true
		}
		if failed[name] {
			_, _, ab := layoutOf(s)
			stubs = append(stubs, gen.Meta{Name: name, Kind: gen.Unified, ArgBytes: ab, RetType: s.Ret})
			continue
		}
		spec := gen.FuncSpec{Name: name, Sig: s, Kind: gen.Unified}
		var text string
		var meta gen.Meta
		if leafSet[name] {
			spec.Kind = gen.Register
			text, meta, err = gen.LowerRegister(asm, spec)
		} else {
			text, meta, err = gen.LowerUnifiedFunc(amd, unit.Funcs[name].Lines, spec, env, "")
		}
		if err != nil {
			reject(name, "lower", err.Error())
			if leafSet[name] {
				die(fmt.Errorf("register-entry leaf %s: %w", name, err))
			}
			_, _, ab := layoutOf(s)
			stubs = append(stubs, gen.Meta{Name: name, Kind: gen.Unified, ArgBytes: ab, RetType: s.Ret})
			continue
		}
		lowered.WriteString(".p2align 4\n.globl " + name + "\n.type " + name + ",@function\n" + text +
			".size " + name + ", .-" + name + "\n")
		metas = append(metas, meta)
	}
	tLower := time.Since(t0)
	if *pollOn {
		for _, name := range rep.GotoTable {
			if f, ok := unit.Funcs[name]; ok {
				n, err := gen.CheckDispatchCopies(name, f.Lines)
				if err != nil {
					reject(name, "lower", err.Error())
				}
				fmt.Fprintf(os.Stderr, "dispatch copies: %s %d\n", name, n)
			}
		}
	}
	// Every rewrite the lowering made, for inspection and for the check's
	// own tests.
	var recs []*gen.FuncRecord
	for _, m := range metas {
		recs = append(recs, m.Rec)
	}
	b, _ := json.MarshalIndent(recs, "", " ")
	writeFile(filepath.Join(*work, "records.json"), string(b))
	summary(rep, plan, pre, metas, stubs, len(unit.FuncOrder))
	if *reportOut != "" {
		b, _ := json.MarshalIndent(rejections, "", " ")
		writeFile(*reportOut, string(b))
	}
	if len(rejections) > 0 && !*keepGoing {
		hint := "run with -keep-going to list them all"
		if *release {
			hint = "a release build has no stub, so the C must change: -keep-going, without -release, lists them all"
		}
		die(fmt.Errorf("%d rejections (%s)", len(rejections), hint))
	}
	if *release && len(stubs) > 0 {
		die(fmt.Errorf("-release: %d functions would be stubs without a rejection to say why", len(stubs)))
	}

	// 6. Re-assemble (keeping the temporary labels the check looks up) and
	// emit.
	text := lowered.String()
	if e := editLoweredEnv; e != "" {
		text = editLowered(text, e) // negative tests: the check must fail
	}
	loweredPath := filepath.Join(*work, "lowered.s")
	writeFile(loweredPath, text)
	loweredObj := filepath.Join(*work, "lowered.o")
	os.Remove(loweredObj)
	must1(capture(filepath.Join(*llvm, "llvm-mc"), "-filetype=obj", "-triple=x86_64-linux-gnu", "--save-temp-labels",
		"-o", loweredObj, loweredPath))
	for name := range rep.Sigs {
		names = append(names, name)
	}
	gonames, err := gen.GoNames(dedupe(names))
	if err != nil {
		die(err)
	}
	// The post-link check: every generated function, instruction by
	// instruction, and engine_rodata, byte by byte, against llc's object.
	if *verifyBin != "" {
		vpkg := *verifyPkg
		if vpkg == "" {
			vpkg = *pkg
		}
		probes, err := gen.ParseRegFacts(readFile(probeFacts))
		if err != nil {
			die(err)
		}
		facts, err := gen.ParseRegFacts(readFile(factsPath))
		if err != nil {
			die(err)
		}
		// Code with no cycle has no tick, and its binary no host_poll.
		polls := false
		for _, m := range metas {
			for _, c := range m.Rec.Calls {
				polls = polls || c.Target == "host_poll"
			}
		}
		cr, err := gen.CheckEngine(gen.EngineCheck{LLC: objPath, Lowered: loweredObj, Bin: *verifyBin, Pkg: vpkg,
			Names: gonames, Unit: unit, Plan: plan, Metas: metas, Stubs: stubs, Sigs: sigs, Leaves: rep.Leaf,
			PreserveNone: *mode == "pn", Probes: probes, Facts: facts, All: *verifyAll, HostPoll: *pollOn && polls})
		if cr != nil {
			for _, k := range gen.SortedKeys(cr.Checked) {
				fmt.Fprintf(os.Stderr, "checked: %d %s\n", cr.Checked[k], k)
			}
			for _, why := range gen.SortedKeys(cr.Skipped) {
				fns := cr.Skipped[why]
				fmt.Fprintf(os.Stderr, "not checked: %d functions, %s: %s\n", len(fns), why, strings.Join(fns[:min(len(fns), 8)], " "))
			}
		}
		if err != nil {
			die(fmt.Errorf("post-link check: %w", err))
		}
		fmt.Fprintf(os.Stderr, "post-link check passed for %s\n", *verifyBin)
	}
	if *out == "" {
		fmt.Fprintf(os.Stderr, "time: compile %v, llvm %v, lower %v, total %v\n", tCompile, tLLVM, tLower, time.Since(t0))
		return
	}
	// llvm-link renames a static whose name another file also uses
	// (cr_add_interval.156). It goes with the function of the plain name:
	// the groups only bound the file sizes, so the file need not be its own.
	reRenamed := regexp.MustCompile(`^(.+)\.\d+$`)
	for _, m := range metas {
		if _, ok := group[m.Name]; !ok {
			if r := reRenamed.FindStringSubmatch(m.Name); r != nil {
				group[m.Name] = group[r[1]]
			}
		}
	}
	files, err := gen.EmitEngine(loweredObj, gen.EngineEmit{Metas: metas, Names: gonames, Group: group,
		MaxBytes: *maxFile, Stubs: stubs, NoStubs: *release, Plan: plan, Arch: *arch})
	if err != nil {
		die(err)
	}
	must(os.MkdirAll(*out, 0o755))
	old, _ := filepath.Glob(filepath.Join(*out, "*_"+*arch+".s"))
	for _, f := range old {
		os.Remove(f)
	}
	total := 0
	for name, text := range files {
		writeFile(filepath.Join(*out, name), text)
		total += len(text)
	}
	writeFile(filepath.Join(*out, "engine_decl.go"), goDecls(metas, stubs, sigs, gonames, rep, keepNarrow))
	negativeFile := filepath.Join(*out, "engine_negative.go")
	if keepNarrow {
		writeFile(negativeFile, negativeBuildFile())
	} else {
		os.Remove(negativeFile) // an earlier negative build in this folder
	}
	writeFile(filepath.Join(*out, "engine_keep.go"), keepAll(metas, stubs, gonames))
	fmt.Fprintf(os.Stderr, "wrote %d .s files, %d bytes, to %s\n", len(files), total, *out)
	fmt.Fprintf(os.Stderr, "time: compile %v, llvm %v, lower %v, total %v\n", tCompile, tLLVM, tLower, time.Since(t0))
}

// keepNarrowFromEnv reads GEN_KEEP_NARROW, the negative build that leaves
// narrow integers as the generator once took them. Only 1 turns it on, and an
// unset or empty variable leaves it off. Any other value (0 among them) is an
// error, since a build that reads 0 as on is the mistake the switch exists to
// show.
func keepNarrowFromEnv(v string) (bool, error) {
	switch v {
	case "":
		return false, nil
	case "1":
		return true, nil
	}
	return false, fmt.Errorf("GEN_KEEP_NARROW=%q: only 1 turns the negative build on (unset it to build the engine)", v)
}

// devSwitches are the switches a release build refuses: the ones for
// surveys (-keep-going, -arch arm64), wrong code on purpose (-loose-varargs),
// tests of the tests (-poison, GEN_KEEP_NARROW, GEN_EDIT_LOWERED) and a
// format check made narrower than the default. -src is not among them: a
// fixture goes through a release build like the engine does, which is how the
// build is tested.
type devSwitches struct {
	KeepGoing, LooseVarargs, Poison, KeepNarrow, NoPoll bool
	EditLowered, Arch, NonFormat                        string
}

// releaseError says why -release cannot go on with these switches, or is nil.
// A release build also needs -out: it writes the package.
func releaseError(d devSwitches, out string) error {
	var bad []string
	add := func(on bool, name string) {
		if on {
			bad = append(bad, name)
		}
	}
	add(d.KeepGoing, "-keep-going")
	add(d.LooseVarargs, "-loose-varargs")
	add(d.Poison, "-poison")
	add(d.KeepNarrow, "GEN_KEEP_NARROW")
	add(d.EditLowered != "", "GEN_EDIT_LOWERED")
	add(d.Arch != "amd64", "-arch "+d.Arch)
	add(d.NonFormat != defaultNonFormat, "-non-format-variadic")
	add(d.NoPoll, "an engine without polls (-poll=false)")
	if len(bad) > 0 {
		return fmt.Errorf("-release refuses %s: they are for surveys and tests, not for the engine", strings.Join(bad, ", "))
	}
	if out == "" {
		return fmt.Errorf("-release needs -out: a release build writes the package")
	}
	return nil
}

// negativeBuildFile is engine_negative.go, which a negative build writes next
// to its other files. Without the tag goasmnegative the package does not
// compile, so a negative build cannot be taken for an engine.
func negativeBuildFile() string {
	return fmt.Sprintf(`// Code generated by internal/goasm/gen. DO NOT EDIT.

//go:build !goasmnegative

package %s

// This package is a negative build of the generator (GEN_KEEP_NARROW=1): its
// narrow integers are passed as the generator once took them, to show that
// the tests and the post-link check catch the mistake. It is not an engine,
// and it does not compile unless the build is given -tags goasmnegative.
var _ = negative_build_of_the_generator_is_not_an_engine_build_it_with_tag_goasmnegative
`, *pkg)
}

// editLowered applies GEN_EDIT_LOWERED to the lowered text: entries
// separated by "\x1e", each "N" (delete line N, counted from 1) or
// "N\x1fnew text" (replace it). A negative test of the post-link check
// builds the package from the edited text, which the check must reject.
func editLowered(text, edits string) string {
	lines := strings.Split(text, "\n")
	drop := map[int]bool{}
	for _, e := range strings.Split(edits, "\x1e") {
		num, repl, isRepl := strings.Cut(e, "\x1f")
		var n int
		if _, err := fmt.Sscan(num, &n); err != nil || n < 1 || n > len(lines) {
			die(fmt.Errorf("GEN_EDIT_LOWERED: bad line %q", num))
		}
		if isRepl {
			lines[n-1] = repl
		} else {
			drop[n-1] = true
		}
	}
	var out []string
	for i, l := range lines {
		if !drop[i] {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// keepAll writes engine_keep.go: under the goasmkeep tag it holds every
// generated Go-visible function, so a binary built with -tags goasmkeep
// links them all and -verify-all can check each.
func keepAll(metas, stubs []gen.Meta, names gen.NameMap) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by internal/goasm/gen. DO NOT EDIT.\n\n//go:build goasmkeep\n\npackage %s\n\n", *pkg)
	b.WriteString("// goasmKeep holds every generated function for the post-link check.\nvar goasmKeep = []any{\n")
	all := append(append([]gen.Meta{}, metas...), stubs...)
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	for _, m := range all {
		if m.Kind != gen.Register {
			fmt.Fprintf(&b, "\t%s,\n", names[m.Name])
		}
	}
	b.WriteString("}\n\nfunc init() {\n\tfor _, f := range goasmKeep {\n\t\tif f == nil {\n\t\t\tpanic(\"goasmKeep: a nil function\")\n\t\t}\n\t}\n}\n")
	return b.String()
}

var reDefine = regexp.MustCompile(`(?m)^define\b[^@\n]*@("?[^"(\s]+"?)\(`)

func allowAll(allow map[string]bool, rep *gen.Report) map[string]bool {
	m := map[string]bool{}
	for k := range allow {
		m[k] = true
	}
	for k, def := range rep.Defined {
		if def {
			m[k] = true
		}
	}
	return m
}

// firstGroups gives each function the group of the first file that defines
// it, in the order llvm-link takes the files (a static of the same name in a
// later file gets a new name there), so the files come out the same each
// run whatever order the compiles finish in.
func firstGroups(defined [][]string, groups []string) map[string]string {
	group := map[string]string{}
	for i, names := range defined {
		for _, n := range names {
			if _, ok := group[n]; !ok {
				group[n] = groups[i]
			}
		}
	}
	return group
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func layoutOf(s gen.Sig) ([]int, int, int) { return gen.Layout(s) }

// goDecls writes the Go declaration of every unified entry (lowered or
// stub) and the signature each host callback must have.
func goDecls(metas, stubs []gen.Meta, sigs map[string]gen.Sig, names gen.NameMap, rep *gen.Report, negative bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by internal/goasm/gen. DO NOT EDIT.\n\n")
	if negative {
		b.WriteString("// NEGATIVE BUILD (GEN_KEEP_NARROW=1): this is not an engine. See engine_negative.go.\n\n")
	}
	fmt.Fprintf(&b, "package %s\n\n", *pkg)
	fmt.Fprintf(&b, "// shadowSizeLog2 is log2 of the shadow-stack region the engine was generated\n"+
		"// for (-shadow-size-log2): it takes the space left from the stack pointer\n"+
		"// and this size, so the host's region must be aligned to it and no smaller.\n"+
		"const shadowSizeLog2 = %d\n\n", *sizeLog2)
	fmt.Fprintf(&b, "// polls says whether the engine counts its cycles and calls host_poll\n"+
		"// (-poll). pollCounterOffset is where the counter is from the base of\n"+
		"// the shadow-stack region (-poll-offset): host_poll sets it again, so the\n"+
		"// host keeps that word for it.\n"+
		"const (\n\tpolls             = %t\n\tpollCounterOffset = %d\n)\n\n", *pollOn, *pollOff)
	decl := func(name string) {
		s := sigs[name]
		var ps []string
		w := 0
		for _, p := range s.Params {
			switch p {
			case gen.F64:
				ps = append(ps, fmt.Sprintf("a%d float64", w))
				w++
			case gen.JSV:
				ps = append(ps, fmt.Sprintf("a%d, a%d uintptr", w, w+1))
				w += 2
			default:
				ps = append(ps, fmt.Sprintf("a%d uintptr", w))
				w++
			}
		}
		ret := ""
		switch s.Ret {
		case gen.I64, gen.Ptr:
			ret = " uintptr"
		case gen.F64:
			ret = " float64"
		case gen.JSV:
			ret = " (r0, r1 uintptr)"
		}
		fmt.Fprintf(&b, "func %s(%s)%s\n", names[name], strings.Join(ps, ", "), ret)
	}
	all := append(append([]gen.Meta{}, metas...), stubs...)
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	for _, m := range all {
		if m.Kind == gen.Register {
			continue
		}
		decl(m.Name)
	}
	b.WriteString("\n// missingNames lists the stubs in index order.\nvar missingNames = []string{\n")
	for _, m := range stubs {
		fmt.Fprintf(&b, "\t%q,\n", m.Name)
	}
	b.WriteString("}\n\n// Host callbacks the engine calls (implemented in Go):\n")
	for _, name := range gen.SortedKeys(rep.Defined) {
		if !rep.Defined[name] {
			fmt.Fprintf(&b, "//   %s %s\n", name, rep.Sigs[name])
		}
	}
	return b.String()
}

// classify names the class of a rejection message.
func classify(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "format string"):
		return "format string"
	case strings.Contains(m, "variadic") || strings.Contains(m, "va_start"):
		return "varargs"
	case strings.Contains(m, "byval") || strings.Contains(m, "sret"):
		return "byval/sret"
	case strings.Contains(m, "tail"):
		return "tail call"
	case strings.Contains(m, "inline assembly"):
		return "inline asm"
	case strings.Contains(m, "thread-local"):
		return "TLS"
	case strings.Contains(m, "base-pointer") || strings.Contains(m, "%rbp"):
		return "machine stack read"
	case strings.Contains(m, "relocation"):
		return "relocation"
	case strings.Contains(m, "data") || strings.Contains(m, "absolute") || strings.Contains(m, "read-only"):
		return "read-only data"
	case strings.Contains(m, "indirect"):
		return "function pointer"
	case strings.Contains(m, "too many"):
		return "argument slots"
	case strings.Contains(m, "sp write") || strings.Contains(m, "stack access") || strings.Contains(m, "escaping stack"):
		return "stack shape"
	case strings.Contains(m, "allow list"):
		return "external symbol"
	}
	return "other"
}

func summary(rep *gen.Report, plan *gen.DataPlan, pre []gen.IRViolation, metas, stubs []gen.Meta, total int) {
	byClass := map[string][]Rejection{}
	for _, r := range rejections {
		byClass[r.Class] = append(byClass[r.Class], r)
	}
	fmt.Fprintf(os.Stderr, "functions: %d in the text, %d lowered, %d stubs\n", total, len(metas), len(stubs))
	for _, c := range gen.SortedKeys(byClass) {
		rs := byClass[c]
		fns := map[string]bool{}
		for _, r := range rs {
			fns[r.Func] = true
		}
		fmt.Fprintf(os.Stderr, "class %-20s %4d rejections in %4d functions\n", c, len(rs), len(fns))
		for i, r := range rs {
			if i >= 6 {
				break
			}
			msg := r.Msg
			if len(msg) > 160 {
				msg = msg[:160]
			}
			fmt.Fprintf(os.Stderr, "    %s [%s] %s\n", r.Func, r.Stage, msg)
		}
	}
	calls := map[string]int{}
	for _, v := range pre {
		if v.Rule == "variadic call" {
			calls[v.Func]++
		}
	}
	n := 0
	for _, c := range calls {
		n += c
	}
	fmt.Fprintf(os.Stderr, "variadic call sites: %d in %d functions\n", n, len(calls))
	pi := 0
	for _, c := range rep.PackedICalls {
		pi += c
	}
	fmt.Fprintf(os.Stderr, "argument blocks: %d functions, %d indirect call sites; goto tables %v; frame-address readers %d\n",
		len(rep.Packed), pi, rep.GotoTable, len(rep.FrameAddr))
	fmt.Fprintf(os.Stderr, "data: %d objects, %d in engine_rodata (%d bytes, %d pointers), %d in bodies, %d copies, %d dropped\n",
		plan.Objects, plan.BlobObjects, len(plan.Blob), len(plan.Relocs), plan.InBodyObjects, plan.DupObjects, plan.Dropped)
}

func capture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args[max(0, len(args)-3):], " "), err, e.String())
	}
	return e.String() + o.String(), nil
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		die(err)
	}
	return string(b)
}

func writeFile(p, s string) { must(os.WriteFile(p, []byte(s), 0o644)) }
func must(err error) {
	if err != nil {
		die(err)
	}
}
func must1(s string, err error) string {
	if err != nil {
		die(err)
	}
	return s
}
func die(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }

// surveyARM64 counts what the arm64 lowering would meet in llc's output of
// the whole engine, beyond what it takes from the prototype units.
func surveyARM64(asm string, rep *gen.Report) {
	unit, err := gen.SplitAsm(asm, "//")
	if err != nil {
		reject("", "split", err.Error())
		return
	}
	reAdrp := regexp.MustCompile(`^adrp\s+x\d+, (:got:)?([.\w$]+)`)
	reX18 := regexp.MustCompile(`\b[wx]18\b`)
	type count struct {
		n   int
		fns map[string]bool
		ex  string
	}
	c := map[string]*count{}
	add := func(k, fn, l string) {
		x := c[k]
		if x == nil {
			x = &count{fns: map[string]bool{}}
			c[k] = x
		}
		x.n++
		x.fns[fn] = true
		if x.ex == "" {
			x.ex = fn + ": " + l
		}
	}
	for _, name := range unit.FuncOrder {
		for _, raw := range unit.Funcs[name].Lines {
			if strings.HasPrefix(strings.TrimSpace(raw), gen.ICallPrefix) {
				add("indirect-call labels", name, "")
				continue
			}
			l := strings.TrimSpace(raw)
			if i := strings.Index(l, "//"); i >= 0 {
				l = strings.TrimSpace(l[:i])
			}
			l = strings.Join(strings.Fields(l), " ")
			switch {
			case l == "":
			case strings.HasPrefix(l, "blr "):
				add("blr (indirect call)", name, l)
			case strings.HasPrefix(l, "br "):
				add("br (jump table or computed goto)", name, l)
			case strings.HasPrefix(l, "b ") && !strings.HasPrefix(l, "b .L"):
				add("b to a symbol (tail call)", name, l)
			case strings.HasPrefix(l, "adrp "):
				m := reAdrp.FindStringSubmatch(l)
				switch {
				case m == nil:
					add("adrp, other form", name, l)
				case m[1] != "":
					add("adrp through the GOT", name, l)
				case strings.HasPrefix(m[2], ".LCPI"):
					add("adrp of a constant pool", name, l)
				case strings.HasPrefix(m[2], ".LJTI"):
					add("adrp of a jump table", name, l)
				case rep.Defined[m[2]] || rep.Sigs[m[2]] != "":
					add("adrp of a function", name, l)
				default:
					add("adrp of other data", name, l)
				}
			case strings.HasPrefix(l, "adr "):
				add("adr (label address)", name, l)
			case reX18.MatchString(l):
				add("x18 use", name, l)
			case strings.HasPrefix(l, "mrs ") || strings.HasPrefix(l, "msr ") || strings.HasPrefix(l, "svc"):
				add("system instruction", name, l)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "arm64: %d functions in the text, %d data objects\n", len(unit.FuncOrder), len(unit.DataOrder))
	for _, k := range gen.SortedKeys(c) {
		x := c[k]
		ex := x.ex
		if len(ex) > 120 {
			ex = ex[:120]
		}
		fmt.Fprintf(os.Stderr, "  %-34s %6d in %4d functions   e.g. %s\n", k, x.n, len(x.fns), ex)
	}
}
