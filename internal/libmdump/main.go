// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command libmdump calls the math functions of modernc.org/libc, the musl that
// ccgo translated and that the Go engine of this module runs on linux/amd64, on
// the inputs of engine/test/mathcmp.js, and prints the bits of the results in
// the format of engine/test/libmath.c, which does the same with engine/libc.
// engine/test/math.sh compares the two.
//
//	go run ./internal/libmdump INPUTS
//
// INPUTS is the output of engine/test/mathdump, whose "in" lines hold the
// inputs. Results are written as they are, with no change of a NaN.
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"modernc.org/libc"
)

const count = 100000

type fn struct {
	name  string // the function of libc
	input string // the name of the inputs in mathdump's output
	nargs int
	f1    func(*libc.TLS, float64) float64
	f2    func(*libc.TLS, float64, float64) float64
	kind  int
}

const (
	plain = iota
	lrint
	modf
)

var fns = []fn{
	{"acos", "acos", 1, libc.Xacos, nil, plain},
	{"acosh", "acosh", 1, libc.Xacosh, nil, plain},
	{"asin", "asin", 1, libc.Xasin, nil, plain},
	{"asinh", "asinh", 1, libc.Xasinh, nil, plain},
	{"atan", "atan", 1, libc.Xatan, nil, plain},
	{"atan2", "atan2", 2, nil, libc.Xatan2, plain},
	{"atanh", "atanh", 1, libc.Xatanh, nil, plain},
	{"cbrt", "cbrt", 1, libc.Xcbrt, nil, plain},
	{"ceil", "ceil", 1, libc.Xceil, nil, plain},
	{"cos", "cos", 1, libc.Xcos, nil, plain},
	{"cosh", "cosh", 1, libc.Xcosh, nil, plain},
	{"exp", "exp", 1, libc.Xexp, nil, plain},
	{"expm1", "expm1", 1, libc.Xexpm1, nil, plain},
	{"fabs", "abs", 1, libc.Xfabs, nil, plain},
	{"floor", "floor", 1, libc.Xfloor, nil, plain},
	{"fmax", "max", 2, nil, libc.Xfmax, plain},
	{"fmin", "min", 2, nil, libc.Xfmin, plain},
	{"fmod", "mod", 2, nil, libc.Xfmod, plain},
	{"hypot", "hypot", 2, nil, libc.Xhypot, plain},
	{"log", "log", 1, libc.Xlog, nil, plain},
	{"log10", "log10", 1, libc.Xlog10, nil, plain},
	{"log1p", "log1p", 1, libc.Xlog1p, nil, plain},
	{"log2", "log2", 1, libc.Xlog2, nil, plain},
	{"lrint", "clamp", 1, nil, nil, lrint},
	{"modf", "pause", 1, nil, nil, modf},
	{"pow", "pow", 2, nil, libc.Xpow, plain},
	{"round", "round", 1, libc.Xround, nil, plain},
	{"sin", "sin", 1, libc.Xsin, nil, plain},
	{"sinh", "sinh", 1, libc.Xsinh, nil, plain},
	{"sqrt", "sqrt", 1, libc.Xsqrt, nil, plain},
	{"tan", "tan", 1, libc.Xtan, nil, plain},
	{"tanh", "tanh", 1, libc.Xtanh, nil, plain},
	{"trunc", "trunc", 1, libc.Xtrunc, nil, plain},
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "libmdump: "+format+"\n", args...)
	os.Exit(2)
}

func readInputs(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		fail("%v", err)
	}
	defer f.Close()
	in := map[string]string{}
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadString('\n')
		if strings.HasPrefix(line, "in ") {
			fields := strings.Split(strings.TrimRight(line, "\n"), " ")
			if len(fields) != 4 {
				fail("bad input line")
			}
			in[fields[1]] = fields[3]
		}
		if err != nil {
			break
		}
	}
	return in
}

func word(data string, i int) float64 {
	u, err := strconv.ParseUint(data[i*16:i*16+16], 16, 64)
	if err != nil {
		fail("%v", err)
	}
	return math.Float64frombits(u)
}

func main() {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		fail("modernc.org/libc is musl's C only on linux/amd64, and this is %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if len(os.Args) != 2 {
		fail("usage: libmdump INPUTS")
	}
	in := readInputs(os.Args[1])
	tls := libc.NewTLS()
	defer tls.Close()
	out := bufio.NewWriterSize(os.Stdout, 1<<20)
	defer out.Flush()
	for _, f := range fns {
		data, ok := in[f.input]
		if !ok {
			fail("no inputs named %s", f.input)
		}
		fmt.Fprintf(out, "in %s %d %s\nout %s %d ", f.name, count, data, f.name, count)
		for k := 0; k < count; k++ {
			x := word(data, k*f.nargs)
			switch f.kind {
			case lrint:
				var v int64
				if math.Abs(x) < 4.0e18 {
					// libc.long is 32 bits on windows/amd64: the conversion lets
					// the tool compile on every target, and the guard in main
					// keeps it from running anywhere but linux/amd64.
					v = int64(libc.Xlrint(tls, x))
				}
				fmt.Fprintf(out, "%016x", uint64(v))
			case modf:
				var ipart float64
				frac := libc.Xmodf(tls, x, uintptr(unsafe.Pointer(&ipart)))
				fmt.Fprintf(out, "%016x%016x", math.Float64bits(frac), math.Float64bits(ipart))
			default:
				var r float64
				if f.nargs == 2 {
					r = f.f2(tls, x, word(data, k*2+1))
				} else {
					r = f.f1(tls, x)
				}
				fmt.Fprintf(out, "%016x", math.Float64bits(r))
			}
		}
		fmt.Fprintln(out)
	}
}
