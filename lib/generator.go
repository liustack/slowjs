// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	cc "modernc.org/cc/v4"
	ccgo "modernc.org/ccgo/v4/lib"
	util "modernc.org/fileutil/ccgo"
)

const (
	qjsCommit     = "3d5e064e9dd67c70f7962836505a7fa067bf0a4e"
	qjsDiff       = "quickjs.diff"
	qjsUnzip      = "quickjs-" + qjsCommit
	qjsZip        = qjsCommit + ".zip"
	test262Commit = "5c8206929d81b2d3d727ca6aac56c18358c8d790"
	test262Diff   = "test262.diff"
	test262Dir    = "test262"
	test262Unzip  = "test262-" + test262Commit
	test262Zip    = test262Commit + ".zip"
	tmp           = "tmp"
)

var (
	ccArgs = []string{
		"--prefix-enumerator=E",
		"--prefix-external=x_",
		"--prefix-field=F",
		"--prefix-macro=M",
		"--prefix-static-internal=_",
		"--prefix-static-none=_",
		"--prefix-tagged-enum=_",
		"--prefix-tagged-struct=T",
		"--prefix-tagged-union=T",
		"--prefix-typename=T",
		"--prefix-undefined=_",
		"-DNDEBUG",
		"-UHAVE_CLOSEFROM",
		"-U__SIZEOF_INT128__",
		"-eval-all-macros",
		"-extended-errors",
		"-hide", "js_check_stack_overflow",
		"-ignore-link-errors",
		"-ignore-unsupported-alignment",
	}
	dev    = os.Getenv("GO_GENERATE_DEV") != ""
	gmake  = "make"
	goarch = env("TARGET_GOARCH", env("GOARCH", runtime.GOARCH))
	goos   = env("TARGET_GOOS", env("GOOS", runtime.GOOS))
	gsed   = "sed"
	j      = strconv.Itoa(runtime.GOMAXPROCS(-1))
	target = fmt.Sprintf("%s/%s", goos, goarch)
	w64    = os.Getenv("GO_GENERATE_W64") != ""
)

func env(name, deflt string) (r string) {
	r = deflt
	if s := os.Getenv(name); s != "" {
		r = s
	}
	return r
}

func fail(rc int, msg string, args ...any) {
	fmt.Fprintln(os.Stderr, strings.TrimSpace(fmt.Sprintf("FAIL: "+msg, args...)))
	panic(rc)
}

// w64gcc resolves the llvm-mingw x86_64 Windows cross toolchain (gcc and ar) in
// a version-agnostic way, so that relocating the install directory - which is
// exactly what silently broke windows cross-generation once (llvm/bin ->
// llvm-mingw-<date>/bin) - cannot recur. It MUST be llvm-mingw (clang): GNU
// mingw's <_mingw.h> uses __int128 in a way ccgo cannot type-check, so a GNU
// x86_64-w64-mingw32-gcc found on $PATH is rejected rather than used. Candidates
// are tried in order: $LLVM_MINGW_BIN, $HOME/bin/llvm/bin (the canonical
// location on the builder), the newest $HOME/bin/llvm-mingw-*/bin, then $PATH;
// the first whose gcc reports "clang" wins.
func w64gcc() (gcc, ar string) {
	isClang := func(g string) bool {
		out, err := exec.Command(g, "--version").Output()
		return err == nil && strings.Contains(string(out), "clang")
	}
	try := func(dir string) bool {
		if dir == "" {
			return false
		}
		g := filepath.Join(dir, "x86_64-w64-mingw32-gcc")
		if _, err := os.Stat(g); err != nil || !isClang(g) {
			return false
		}
		gcc = g
		ar = filepath.Join(dir, "x86_64-w64-mingw32-ar")
		return true
	}
	home := os.Getenv("HOME")
	cands := []string{
		os.Getenv("LLVM_MINGW_BIN"),
		filepath.Join(home, "bin", "llvm", "bin"),
	}
	globs, _ := filepath.Glob(filepath.Join(home, "bin", "llvm-mingw-*", "bin"))
	sort.Strings(globs)
	for i := len(globs) - 1; i >= 0; i-- { // newest first
		cands = append(cands, globs[i])
	}
	if p, err := exec.LookPath("x86_64-w64-mingw32-gcc"); err == nil {
		cands = append(cands, filepath.Dir(p))
	}
	for _, d := range cands {
		if try(d) {
			return gcc, ar
		}
	}
	fail(1, "no llvm-mingw (clang) x86_64-w64-mingw32-gcc found; set LLVM_MINGW_BIN")
	return "", ""
}

func main() {
	if ccgo.IsExecEnv() {
		if err := ccgo.NewTask(goos, goarch, os.Args, os.Stdout, os.Stderr, nil).Main(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		fail(1, "%v", err)
	}

	if wd, err = filepath.Abs(wd); err != nil {
		fail(1, "%v", err)
	}

	// ccgo_windows.go serves windows/{amd64,arm64} but has no native builder, so
	// nothing regenerates it on a QuickJS bump unless it is folded into the
	// linux/amd64 autogen run - which is what caused it to go ~9 months stale.
	// Cross-generate it here (via `make w64`) and snapshot the go.mod for both
	// windows arches. Set GO_GENERATE_NOWIN=1 to skip on hosts without an
	// llvm-mingw toolchain.
	if !w64 && target == "linux/amd64" && os.Getenv("GO_GENERATE_NOWIN") == "" {
		defer func() {
			shell(gmake, "w64")
			copyFile("internal/autogen/windows_amd64.mod", "go.mod")
		}()
	}

	switch {
	case w64:
		goos = "windows"
		goarch = "amd64"
		os.Setenv("TARGET_GOOS", goos)
		os.Setenv("TARGET_GOARCH", goarch)
		xgcc, xar := w64gcc()
		os.Setenv("CC", xgcc)
		os.Setenv("CCGO_CC", xgcc)
		os.Setenv("CCGO_GCC", xgcc)
		os.Setenv("CCGO_CLANG", xgcc)
		os.Setenv("CCGO_CPP", xgcc)
		ccArgs = append(ccArgs,
			"--cpp", xgcc,
			"--goarch", goarch,
			"--goos", goos,
			"-D__builtin_bit_cast(T, V)=__builtin_bit_cast()",
			"-D_FILE_OFFSET_BITS",
			"-D_FILE_OFFSET_BITS=64",
			"-build-lines", "//go:build windows && (amd64 || arm64)\n",
			"-map", "gcc="+xgcc+",ar="+xar,
		)
	}
	if s := cc.LongDouble64Flag(goos, goarch); s != "" {
		ccArgs = append(ccArgs, s)
	}
	if dev {
		ccArgs = append(ccArgs,
			"-absolute-paths",
			"-keep-object-files",
			"-positions",
		)
	}

	switch target {
	case "darwin/arm64":
		gsed = "gsed"
		ccArgs = append(ccArgs, "-ignore-static-asserts")
	case "darwin/amd64":
		gsed = "gsed"
	case "freebsd/arm64", "freebsd/amd64":
		gmake = "gmake"
		gsed = "gsed"
	}

	removeAll(tmp)
	mkdirAll(tmp)
	unzip(qjsZip, tmp)
	quickjs := filepath.Join(tmp, qjsUnzip)
	unzip(test262Zip, quickjs)
	if err := os.Rename(filepath.Join(quickjs, test262Unzip), filepath.Join(quickjs, test262Dir)); err != nil {
		fail(1, "%v", err)
	}

	test262 := filepath.Join(quickjs, test262Dir)
	copyFile(filepath.Join("internal", "patch", test262Diff), filepath.Join(quickjs, "tests", "test262.patch"))
	copyFile(filepath.Join(test262, "test262.patch"), filepath.Join(quickjs, "tests", "test262.patch"))
	git("-C", test262, "init")
	git("-C", test262, "apply", "-v", "test262.patch")
	copyFile("LICENSE-LIBQUICKJS", filepath.Join(quickjs, "LICENSE"))
	copyFile(filepath.Join("testdata", "test262.conf"), filepath.Join(quickjs, "test262.conf"))
	copyFile(filepath.Join(quickjs, qjsDiff), filepath.Join(wd, "internal", "patch", qjsDiff))
	git("-C", quickjs, "init")
	git("-C", quickjs, "apply", "-v", qjsDiff)
	if w64 {
		shell(gsed, "-i", `s/__SIZEOF_INT128__/__nothing__/`, filepath.Join(quickjs, "quickjs.h"))
	}
	switch target {
	case "freebsd/amd64", "freebsd/arm64":
		ccExec(append(ccArgs,
			"-std=c99",
			"-D__sync_fetch_and_add(p, v, ...)=__atomic_fetch_add((p), (v), 0)",
			"-D__sync_fetch_and_and(p, v, ...)=__atomic_fetch_and((p), (v), 0)",
			"-D__sync_fetch_and_or(p, v, ...)=__atomic_fetch_or((p), (v), 0)",
			"-D__sync_fetch_and_sub(p, v, ...)=__atomic_fetch_sub((p), (v), 0)",
			"-D__sync_fetch_and_xor(p, v, ...)=__atomic_fetch_xor((p), (v), 0)",
			"-D__sync_lock_test_and_set(p, v, ...)=__c11_atomic_exchange((p), (v), 0)",
			"-exec", gmake, "-C", quickjs, "libquickjs.a", "run-test262"))
	case "linux/arm":
		ccExec(append(ccArgs, "-exec", gmake, "LIBS=-lm -latomic", "-C", quickjs, "libquickjs.a", "run-test262"))
	default:
		switch {
		case w64:
			ccExec(append(ccArgs, "-exec", gmake, "CONFIG_WIN32=y", "-C", quickjs, "libquickjs.a", "run-test262.exe"))
		default:
			ccExec(append(ccArgs, "-exec", gmake, "-C", quickjs, "libquickjs.a", "run-test262"))
		}
	}
	copyFile("test262_errors.txt", filepath.Join(quickjs, "test262_errors.txt"))
	genTargetConf(wd, quickjs)
	result := fmt.Sprintf("ccgo_%s_%s.go", goos, goarch)
	switch {
	case w64:
		result = "ccgo_windows.go"
	}
	ccMain(append(ccArgs,
		"--package-name", "libquickjs",
		"-o", result,
		filepath.Join(quickjs, "libquickjs.a"),
	))
	shell(gsed, "-i", `s/\<T__\([a-zA-Z0-9][a-zA-Z0-9_]\+\)/t__\1/g`, result)
	shell(gsed, "-i", `s/\<x_\([a-zA-Z0-9_][a-zA-Z0-9_]\+\)/X\1/g`, result)
	shell(gsed, "-i", `s/\<X__JS_\([a-zA-Z0-9][a-zA-Z0-9_]\+\)/x__JS_\1/g`, result)
	// llvm-mingw (clang) constant-folds some JSValue sentinels to
	// uint32(-int32(1)), which the Go compiler rejects as an overflowing
	// constant conversion; rewrite to the identical unsigned literal. Applied to
	// both the engine and the run-test262 copy below.
	fixW64Overflow := func(path string) {
		shell(gsed, "-i", `s/uint32(-int32(1))/uint32(0xffffffff)/g`, path)
	}
	if w64 {
		fixW64Overflow(result)
	}
	switch {
	case w64:
		rt := filepath.Join("internal", "run-test262", result)
		copyFile(rt, filepath.Join(quickjs, "run-test262.exe.go"))
		fixW64Overflow(rt)
	default:
		copyFile(filepath.Join("internal", "run-test262", result), filepath.Join(quickjs, "run-test262.go"))
	}
	util.MustCopyDir(true, filepath.Join("testdata", "tests"), filepath.Join(quickjs, "tests"), nil)
}

// reNewError matches the leading "test262/....js:LINE:" of a run-test262
// error line, both the plain and the "strict mode:" variant.
var reNewError = regexp.MustCompile(`(?m)^(test262/\S+\.js):\d+:`)

// genTargetConf runs the freshly built native C QuickJS test262 reference
// (quickjs/run-test262) against the full suite using the base testdata/
// test262.conf and the shared test262_errors.txt baseline, then records the
// tests that fail on THIS target but are absent from that baseline into
// testdata/test262_<goos>_<goarch>.conf. This keeps the Go build from being
// held to a stricter standard than upstream QuickJS on the same hardware
// (e.g. libm ULP differences such as Math.sinh on darwin/arm64) and, because
// the per-target conf is regenerated verbatim from the current base conf on
// every run, prevents the overrides from silently going stale across QuickJS
// bumps.
//
// The reference binary must run on the generating host, so this is skipped for
// cross builds (windows via mingw and any GOOS/GOARCH != the host).
func genTargetConf(wd, quickjs string) {
	if w64 || goos != runtime.GOOS || goarch != runtime.GOARCH {
		return
	}

	confPath := filepath.Join(wd, "testdata", fmt.Sprintf("test262_%s_%s.conf", goos, goarch))
	absQuickjs := filepath.Join(wd, quickjs)
	// run-test262 exits non-zero when there are new errors; that is expected.
	c := exec.Command(filepath.Join(absQuickjs, "run-test262"), "-t", "-m", "-c", "test262.conf", "-a")
	c.Dir = absQuickjs
	out, _ := c.CombinedOutput()
	seen := map[string]bool{}
	var paths []string
	for _, m := range reNewError.FindAllStringSubmatch(string(out), -1) {
		if p := m[1]; !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		// No target-specific failures: the base conf suffices; drop any stale
		// override so it cannot rot.
		removeAll(confPath)
		return
	}

	base, err := os.ReadFile(filepath.Join(wd, "testdata", "test262.conf"))
	if err != nil {
		fail(1, "%v", err)
	}

	var block strings.Builder
	fmt.Fprintf(&block, "# Target-specific excludes auto-generated by generator.go: tests that fail\n")
	fmt.Fprintf(&block, "# in the native C QuickJS reference on %s but are not in the shared\n", target)
	fmt.Fprintf(&block, "# test262_errors.txt baseline. Regenerate via `make generate`. DO NOT EDIT.\n")
	for _, p := range paths {
		fmt.Fprintf(&block, "%s\n", p)
	}
	block.WriteByte('\n')

	s := string(base)
	i := strings.Index(s, "[tests]")
	if i < 0 {
		fail(1, "malformed base test262.conf: missing [tests] section")
	}
	out2 := s[:i] + block.String() + s[i:]
	if err := os.WriteFile(confPath, []byte(out2), 0664); err != nil {
		fail(1, "%v", err)
	}
}

func shell(cmd string, args ...string) {
	if out, err := util.Shell(nil, cmd, args...); err != nil {
		fail(1, "err=%v out=%s", err, out)
	}
}

func copyFile(dst, src string) {
	if _, err := util.CopyFile(dst, src, nil); err != nil {
		fail(1, "err=%v", err)
	}
}

func git(args ...string) {
	shell("git", args...)
}

func ccMain(args []string) {
	if err := ccgo.NewTask(goos, goarch, append([]string{"ccgo"}, args...), os.Stdout, os.Stderr, nil).Main(); err != nil {
		fail(1, "%v", err)
	}
}

func ccExec(args []string) {
	if err := ccgo.NewTask(goos, goarch, append([]string{"ccgo"}, args...), os.Stdout, os.Stderr, nil).Exec(); err != nil {
		fail(1, "%v", err)
	}
}

func mkdirAll(path string) {
	if err := os.MkdirAll(path, 0770); err != nil {
		fail(1, "%v", err)
	}
}

func unzip(src, dest string) {
	shell("unzip", "-q", src, "-d", dest)
}

func removeAll(path string) {
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		fail(1, "%v", err)
	}
}
