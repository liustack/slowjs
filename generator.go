// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	goruntime "runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
)

const (
	docFn = "quickjs.go"
	modFn = "go.mod"
	qemu  = "    (qemu)"

	// fingerprintPrefix starts the first line of a results file, recording the
	// root go.mod the numbers below it were measured against. The benchmarks
	// re-run when that no longer matches, ie. exactly when a builder
	// regenerates: its autogen gate compares the same go.mod against
	// internal/autogen/<goos>_<goarch>.mod. Nothing needs bumping by hand.
	fingerprintPrefix = "# root go.mod sha256 "
)

var (
	goos       = runtime.GOOS
	goarch     = runtime.GOARCH
	target     = fmt.Sprintf("%s_%s", goos, goarch) + ".txt"
	resultsDir = path.Join("testdata", "benchmarks")
	notes      = map[string]string{
		"freebsd/amd64": qemu,
		"freebsd/arm64": qemu,
		"linux/386":     qemu,
		"linux/s390x":   qemu,
		"openbsd/amd64": qemu,
		"windows/386":   qemu,
	}
)

func fail(rc int, s string, args ...any) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(s, args...))
	os.Exit(rc)
}

func main() {
	fp, err := fingerprint()
	if err != nil {
		fail(1, "%v", err)
	}

	resultsFn := path.Join(resultsDir, target)
	ok, err := measured(resultsFn, fp)
	if err != nil {
		fail(1, "%v", err)
	}

	switch {
	case ok:
		fmt.Printf("%s: measured against the current %s, skipping benchmarks\n", resultsFn, modFn)
	default:
		fmt.Printf("%s: missing or measured against a different %s, benchmarking\n", resultsFn, modFn)
		if err := benchmark(resultsFn, fp); err != nil {
			fail(1, "%v", err)
		}
	}

	// Unconditional: keeps the package documentation in sync with
	// testdata/benchmarks even when this platform had nothing to measure.
	if err := document(); err != nil {
		fail(1, "%v", err)
	}
}

// fingerprint returns the first line of a results file produced now.
func fingerprint() (string, error) {
	b, err := os.ReadFile(modFn)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%x", fingerprintPrefix, sha256.Sum256(b)), nil
}

// measured reports whether fn holds results recorded for fingerprint fp. A
// missing file, or one predating fingerprintPrefix, is not measured.
func measured(fn, fp string) (bool, error) {
	b, err := os.ReadFile(fn)
	switch {
	case err == nil:
		line, _, _ := bytes.Cut(b, []byte("\n"))
		return string(line) == fp, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

// benchmark runs the ./compare module and records its report in fn, stamped
// with fp.
func benchmark(fn, fp string) error {
	if err := os.MkdirAll(resultsDir, 0775); err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	// Through a shell: on Windows the builders run 'go generate' from git-bash
	// and make lives there.
	cmd := exec.Command("sh", "-c", "make benchmark")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("make benchmark: %v\nstdout=%s\nstderr=%s", err, stdout.Bytes(), stderr.Bytes())
	}

	// Only on success: a partial file carrying the current fingerprint would
	// pass for measured and suppress the retry the next cycle owes us.
	return os.WriteFile(fn, append([]byte(fp+"\n"), stdout.Bytes()...), 0660)
}

// requires returns the versions of the modules required by the go.mod at fn,
// keyed by module path.
func requires(fn string) (map[string]string, error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	mod, err := modfile.ParseLax(fn, b, nil)
	if err != nil {
		return nil, err
	}

	r := map[string]string{}
	for _, v := range mod.Require {
		r[v.Mod.Path] = v.Mod.Version
	}
	return r, nil
}

// document rewrites the '# Performance' section of the docFn package
// documentation from every results file present.
func document() error {
	// The measured engine comes from the root go.mod. The compare module gets
	// this one through a 'replace' pointing at the work tree, so its own
	// modernc.org/quickjs require records nothing about what ran.
	root, err := requires(modFn)
	if err != nil {
		return err
	}

	compare, err := requires(path.Join("compare", "go.mod"))
	if err != nil {
		return err
	}

	m, err := filepath.Glob(path.Join(resultsDir, "*.txt"))
	if err != nil {
		return err
	}

	for i, v := range m {
		m[i] = filepath.ToSlash(v)
	}
	var results [][]string
	for _, fn := range m {
		result, err := os.ReadFile(fn)
		if err != nil {
			return err
		}

		key := path.Base(fn)                    // os_arch.txt
		key = key[:len(key)-len(".txt")]        // os_arch
		key = strings.Replace(key, "_", "/", 1) // os/arch
		a := strings.Split(string(result), "\n")
		for _, v := range a {
			if f := strings.Fields(v); len(f) >= 4 && f[0] == "geomean" { // geomean 1.000 1.019 0.848
				f[0] = key
				results = append(results, f)
				continue
			}
		}
	}

	slices.SortFunc(results, func(a, b []string) int {
		switch {
		case a[0] < b[0]:
			return -1
		case a[0] == b[0]:
			return 0
		default:
			return 1
		}
	})
	doc, err := os.ReadFile(docFn)
	if err != nil {
		return err
	}

	a := strings.Split(string(doc), "\n")
	buf := bytes.NewBuffer(nil)
	state := 0
	ver := func(ip string, versions map[string]string) string {
		return fmt.Sprintf("%s@%s", ip, versions[ip])
	}
	for _, v := range a {
		switch state {
		case 0:
			fmt.Fprintf(buf, "%s\n", v)
			if strings.HasPrefix(v, "// # Performance") {
				state++
			}
		case 1:
			fmt.Fprintf(buf, `//
// Geomeans of time/op over a set of benchmarks, relative to CCGO, lower number
// is better. Detailed results available in the testdata/benchmarks directory.
//
//  CCGO: %s
//  GOJA: %s
//   QJS: %s
//
//	                        CCGO     GOJA     QJS
//	-----------------------------------------------
`,
				ver("modernc.org/libquickjs", root),
				ver("github.com/dop251/goja", compare),
				ver("github.com/fastschema/qjs", compare),
			)
			for _, v := range results {
				fmt.Fprintf(buf, "//	%20s%9s%9s%9s%s\n", v[0], v[1], v[2], v[3], notes[v[0]])
			}
			fmt.Fprintf(buf, `//	-----------------------------------------------
//	                        CCGO     GOJA     QJS
`)
			state++
		case 2:
			switch {
			case strings.HasPrefix(v, "// # Notes"):
				fmt.Fprintf(buf, "%s\n", v)
				state++
			}
		case 3:
			fmt.Fprintf(buf, "%s\n", v)
		}
	}
	if err := os.WriteFile(docFn, buf.Bytes(), 0660); err != nil {
		return err
	}

	// The builders gofmt after generating, a local 'go generate' does not.
	if out, err := exec.Command("gofmt", "-s", "-w", docFn).CombinedOutput(); err != nil {
		return fmt.Errorf("gofmt %s: %v\n%s", docFn, err, out)
	}

	return nil
}

// origin returns caller's short position, skipping skip frames.
func origin(skip int) string {
	pc, fn, fl, _ := goruntime.Caller(skip)
	f := goruntime.FuncForPC(pc)
	var fns string
	if f != nil {
		fns = f.Name()
		if x := strings.LastIndex(fns, "."); x > 0 {
			fns = fns[x+1:]
		}
		if strings.HasPrefix(fns, "func") {
			num := true
			for _, c := range fns[len("func"):] {
				if c < '0' || c > '9' {
					num = false
					break
				}
			}
			if num {
				return origin(skip + 2)
			}
		}
	}
	return fmt.Sprintf("%s:%d:%s", filepath.Base(fn), fl, fns)
}

// todo prints and return caller's position and an optional message tagged with TODO. Output goes to stderr.
func todo(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	r := fmt.Sprintf("%s\n\tTODO %s", origin(2), s)
	// fmt.Fprintf(os.Stderr, "%s\n", r)
	// os.Stdout.Sync()
	return r
}

// trc prints and return caller's position and an optional message tagged with TRC. Output goes to stderr.
//
//lint:ignore U1000 debug helper
func trc(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	r := fmt.Sprintf("%s: TRC %s", origin(2), s)
	fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05.000"), r)
	os.Stderr.Sync()
	return r
}
