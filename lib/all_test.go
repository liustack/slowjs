// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	_ "modernc.org/ccgo/v4/lib"
	util "modernc.org/fileutil/ccgo"
	"modernc.org/libc"
)

const (
	hash262 = "5c8206929d81b2d3d727ca6aac56c18358c8d790"
	zip262  = hash262 + ".zip"
)

var (
	builder    = os.Getenv("MODERNC_BUILDER") != ""
	goarch     = runtime.GOARCH
	goos       = runtime.GOOS
	keep       bool
	re         *regexp.Regexp
	singleFile string
	target     = fmt.Sprintf("%s/%s", goos, goarch)
	xtags      string
)

func TestMain(m *testing.M) {
	oRe := flag.String("re", "", "")
	flag.BoolVar(&keep, "keep", false, "")
	flag.StringVar(&xtags, "xtags", os.Getenv("XTAGS"), "")
	flag.StringVar(&singleFile, "f", "", "")
	flag.Parse()
	if s := *oRe; s != "" {
		re = regexp.MustCompile(s)
	}
	os.Exit(m.Run())
}

func Test(t *testing.T) {
	for _, v := range []string{
		"test_bigint.js",
		"test_closure.js",
		"test_language.js",
		"test_loop.js",
		"test_std.js",
	} {
		if re != nil && !re.MatchString(v) {
			continue
		}
		t.Run(v, func(t *testing.T) {
			fn := filepath.Join("testdata", "tests", v)
			evalFile(t, fn)
		})
	}
}

func Test2(t *testing.T) {
	tests := []struct{ in, out string }{
		{"12+34", "46"},
		{"new Date(1899, 0).getYear()", "-1"},
		{"new Date(1899, 11, 31, 23, 59, 59, 999).getYear()", "-1"},
		{"new Date(1900, 0).getYear()", "0"},
		{"new Date(1900, 11, 31, 23, 59, 59, 999).getYear()", "0"},
		{"new Date(1970, 0).getYear()", "70"},
		{"new Date(1970, 11, 31, 23, 59, 59, 999).getYear()", "70"},
		{"new Date(2000, 0).getYear()", "100"},
		{"new Date(2000, 11, 31, 23, 59, 59, 999).getYear()", "100"},
	}
	for i, v := range tests {
		fn := fmt.Sprint(i)
		if re != nil && !re.MatchString(fn) {
			continue
		}

		t.Run(fn, func(t *testing.T) {
			switch g, e := eval(t, fn, v.in), v.out; {
			case g != e:
				t.Errorf("%v: src=%s got=%q expected=%q", fn, v.in, g, e)
			default:
				t.Logf("%3d: %q\n%[2]s", i, g)
			}
		})
	}
	tls := libc.NewTLS()

	defer tls.Close()

	tz, err := libc.CString("TZ")
	if err != nil {
		t.Fatalf("FAIL: err=%v", err)
	}

	defer libc.Xfree(tls, tz)

	const la = "America/Los_Angeles"
	loc, err := libc.CString(la)
	if err != nil {
		t.Fatalf("FAIL: err=%v", err)
	}

	defer libc.Xfree(tls, loc)

	libc.Xsetenv(tls, tz, loc, 1)
	if g, e := libc.GoString(libc.Xgetenv(tls, tz)), la; g != e {
		t.Fatalf("%q", g)
	}

	for i, v := range tests {
		fn := fmt.Sprint(i)
		if re != nil && !re.MatchString(fn) {
			continue
		}

		t.Run(fn, func(t *testing.T) {
			switch g, e := eval(t, fn, v.in), v.out; {
			case g != e:
				t.Errorf("%v: src=%s got=%q expected=%q", fn, v.in, g, e)
			default:
				t.Logf("%3d: %q\n%[2]s", i, g)
			}
		})
	}
}

var evalFN = [...]byte{'<', 'e', 'v', 'a', 'l', '>', 0}

func evalFile(t testing.TB, fn string) (r string) {
	b, err := os.ReadFile(filepath.FromSlash(fn))
	if err != nil {
		t.Fatalf("FAIL: err=%v", err)
	}

	return eval(t, fn, string(b))
}

func eval0(t testing.TB, tls *libc.TLS, ctx uintptr, js string) (r TJSValue) {
	cs, err := libc.CString(js)
	if err != nil {
		t.Fatalf("FAIL: err=%v", err)
	}

	defer libc.Xfree(tls, cs)

	return XJS_Eval(tls, ctx, cs, Size_t(len(js)), uintptr(unsafe.Pointer(&evalFN)), MJS_EVAL_TYPE_GLOBAL)
}

func eval1(t testing.TB, tls *libc.TLS, ctx uintptr, js string) (r string) {
	return toString(t, tls, ctx, eval0(t, tls, ctx, js))
}

func toString(t testing.TB, tls *libc.TLS, ctx uintptr, val TJSValue) (r string) {
	tv := tag(val)
	if tv < 0 {
		defer XFreeValue(tls, ctx, val)
	}

	switch tv {
	case EJS_TAG_UNDEFINED:
		return "<undefined>"
	case EJS_TAG_EXCEPTION:
		e := XJS_GetException(tls, ctx)

		defer XFreeValue(tls, ctx, e)

		p := XToCString(tls, ctx, e)

		defer XJS_FreeCString(tls, ctx, p)

		return fmt.Sprintf("<exception=%s>", libc.GoString(p))
	case EJS_TAG_INT:
		return fmt.Sprint(jsvToInt32(val))
	case EJS_TAG_FLOAT64:
		return fmt.Sprint(jsvToFloat64(val))
	case
		EJS_TAG_STRING_ROPE, // -6,
		EJS_TAG_STRING:      // -7,

		p := XToCString(tls, ctx, val)

		defer XJS_FreeCString(tls, ctx, p)

		return libc.GoString(p)
	}

	t.Fatalf("FAIL: tag=%v", tv)
	panic(todo(""))
}

func eval(t testing.TB, fn, js string) (r string) {
	tls := libc.NewTLS()

	defer tls.Close()

	rt := XJS_NewRuntime(tls)

	defer XJS_FreeRuntime(tls, rt)

	ctx := XJS_NewContext(tls, rt)

	defer XJS_FreeContext(tls, ctx)

	cs, err := libc.CString(js)
	if err != nil {
		t.Fatalf("FAIL: err=%v", err)
	}

	defer libc.Xfree(tls, cs)

	if r = eval1(t, tls, ctx, js); strings.HasPrefix(r, "<") {
		r = fmt.Sprintf("%s: result=%s", r, eval1(t, tls, ctx, "result"))
	}
	return r
}

func shell(t *testing.T, cmd string, args ...string) (out []byte, err error) {
	if out, err = util.Shell(nil, cmd, args...); err != nil && t != nil {
		t.Fatalf("FAIL: err=%v out=%s", err, out)
	}

	return out, err
}

func git(t *testing.T, args ...string) {
	shell(t, "git", args...)
}

func copyFile(t *testing.T, dst, src string) {
	if _, err := util.CopyFile(dst, src, nil); err != nil {
		t.Fatal(err)
	}
}

func fileExists(fn string) bool {
	switch fi, err := os.Stat(fn); {
	case err == nil && !fi.IsDir():
		return true
	default:
		return false
	}
}

func Test262(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const dir = "test262"
	if !fileExists(zip262) {
		if builder {
			t.Fatal("missing test262 archive")
		}

		t.Skip("to download the test262 archive issue '$ make download'")
	}

	os.RemoveAll(dir)

	if !keep {
		defer func() {
			os.RemoveAll(dir)
		}()
	}

	switch goos {
	case "windows":
		shell(t, "git-bash", "-c", fmt.Sprintf("unzip -q %s", zip262))
	default:
		shell(t, "unzip", "-q", zip262)
	}
	err := os.Rename("test262-"+hash262, dir)
	if err != nil {
		t.Fatal(err)
	}

	cnf := filepath.Join("testdata", fmt.Sprintf("test262_%s_%s.conf", goos, goarch))
	if !fileExists(cnf) {
		cnf = filepath.Join("testdata", "test262.conf")
	}
	t.Logf("using %s", cnf)
	copyFile(t, "test262.conf", cnf)
	copyFile(t, filepath.Join(dir, "test262.diff"), filepath.Join("internal", "patch", "test262.diff"))
	git(t, "-C", dir, "init")
	git(t, "-C", dir, "apply", "-v", "test262.diff")
	bin := filepath.Join(dir, "run-test262")
	if goos == "windows" {
		bin += ".exe"
	}
	if bin, err = filepath.Abs(bin); err != nil {
		t.Fatal(err)
	}

	// internal/run-test262 is a module of its own, so the large transpiled
	// runner stays out of the module zip users download. Build it from there.
	shell(t, "go", "build", "-C", filepath.Join("internal", "run-test262"), "-o", bin, fmt.Sprintf("-tags=%s", xtags), ".")
	var out []byte
	args := []string{"-v", "-t", "-m", "-c", "test262.conf"}
	switch {
	case singleFile != "":
		args = append(args, "-f", singleFile)
	default:
		args = append(args, "-a")
	}
	// Copy any curated per-target exclude file into the cwd and pass it by bare
	// basename: run-test262 resolves namelist entries relative to the file's
	// directory, so a "testdata/" prefix would be prepended to every entry and
	// nothing would match. This file holds tests that fail only in the Go build
	// (Go/libc discrepancies not seen by the native C reference, e.g.
	// Math/sinh-approx.js on darwin/arm64).
	if xbase := fmt.Sprintf("test262_exclude_%s_%s", goos, goarch); fileExists(filepath.Join("testdata", xbase)) {
		copyFile(t, xbase, filepath.Join("testdata", xbase))
		args = append(args, "-x", xbase)
	}
	out, err = shell(nil, bin, args...)
	if err != nil {
		t.Logf("err=%v", err)
	}
	s := strings.TrimSpace(string(out))
	x := strings.Index(s, "Result: ")
	if x < 0 {
		t.Fatal("cannot find test results")
	}

	s = s[x:]
	if x = strings.IndexByte(s, '\n'); x > 0 {
		s = s[:x]
	}
	resultLine := strings.TrimSpace(s)
	switch newErrors, err := parseResultLine(resultLine); {
	case err != nil:
		t.Fatalf("FAIL: err=%v", err)
	case newErrors != 0:
		t.Fatalf("FAIL: new errors=%v", newErrors)
	}
	t.Log(resultLine)
}

// Example: "Result: 36/76685 errors, 1519 excluded, 8703 skipped, 2 fixed\n"
func parseResultLine(s string) (newErrors int64, err error) {
	s = strings.TrimSpace(s)
	a := strings.Fields(s)
	// Result: 36/76685 errors, 1519 excluded, 8703 skipped, 2 fixed
	// [0: "Result:" 1: "36/76685"  2: "errors," 3: "1519" 4: "excluded," 5: "8703" 6: "skipped," 7: "2"   8: "fixed"]
	// Result: 189/82123 errors, 1609 excluded, 5461 skipped, 144 new, 8 changed
	// [0: "Result:" 1: "189/82123" 2: "errors," 3: "1609" 4: "excluded," 5: "5461" 6: "skipped," 7: "144" 8: "new," 9: "8" 10: "changed"]
	if len(a) == 0 || a[0] != "Result:" {
		return 0, fmt.Errorf("malformed result line: `%s`", s)
	}

	for i, v := range a {
		if v == "new" {
			return strconv.ParseInt(a[i-1], 0, 64)
		}
	}

	return 0, nil
}
