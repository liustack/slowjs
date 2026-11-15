// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// engine/test/compare/number_to_string.js prints in the Go build of the
// engine what Node 24 prints.
func TestNumberToStringNode(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("engine", "test", "compare", "number_to_string.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("engine", "test", "compare", "number_to_string.txt"))
	if err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	got, err := m.Eval(string(src), EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string)+"\n" != string(want) {
		g := strings.Split(got.(string), "\n")
		w := strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
		for i := 0; i < len(g) && i < len(w); i++ {
			if g[i] != w[i] {
				t.Fatalf("line %d: got %q, want %q", i+1, g[i], w[i])
			}
		}
		t.Fatalf("got %d lines, want %d", len(g), len(w))
	}
}

// numberToString is Number::toString of the specification in radix 10, from
// the shortest digits Go's strconv finds, which are the closest to x of the
// shortest that read back.
func numberToString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case x == 0:
		return "0"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x < 0:
		return "-" + numberToString(-x)
	}
	e := strconv.FormatFloat(x, 'e', -1, 64) // d.ddde±xx
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	k := len(digits)
	p, _ := strconv.Atoi(exp)
	n := p + 1
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}
	s := digits[:1]
	if k > 1 {
		s += "." + digits[1:]
	}
	if n-1 < 0 {
		return s + "e-" + strconv.Itoa(1-n)
	}
	return s + "e+" + strconv.Itoa(n-1)
}

func TestNumberToStringFormat(t *testing.T) {
	for _, c := range []struct {
		x    float64
		want string
	}{
		{0.1, "0.1"}, {1e21, "1e+21"}, {1e20, "100000000000000000000"}, {123.456, "123.456"},
		{1e-6, "0.000001"}, {1e-7, "1e-7"}, {1.5e-7, "1.5e-7"}, {-2.5, "-2.5"}, {5e-324, "5e-324"},
		{math.MaxFloat64, "1.7976931348623157e+308"}, {math.Ldexp(1, -44), "5.684341886080802e-14"},
	} {
		if got := numberToString(c.x); got != c.want {
			t.Errorf("numberToString(%v) = %q, want %q", c.x, got, c.want)
		}
	}
}

// Random float64 values, and every power of two with the values next to it,
// convert in the engine to what numberToString gives. The script makes the
// bit patterns with the same 32-bit generator as the test (xorshift32 on two
// words).
func TestNumberToStringShortest(t *testing.T) {
	const n = 200000
	m := newTestVM(t)
	got, err := m.Eval(`(function () {
  var dv = new DataView(new ArrayBuffer(8)), out = [], a = 2463534242, b = 88675123;
  function next(s) { s ^= s << 13; s ^= s >>> 17; s ^= s << 5; return s >>> 0; }
  for (var i = 0; i < `+strconv.Itoa(n)+`; i++) {
    a = next(a); b = next(b);
    dv.setUint32(0, a); dv.setUint32(4, b);
    out.push(String(dv.getFloat64(0)));
  }
  for (var e = 1; e < 2047; e++) {
    for (var d = -2; d <= 2; d++) {
      if (d < 0) { dv.setUint32(0, e * 1048576 - 1); dv.setUint32(4, 4294967296 + d); }
      else { dv.setUint32(0, e * 1048576); dv.setUint32(4, d); }
      out.push(String(dv.getFloat64(0)));
    }
  }
  return out.join("\n");
})()`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got.(string), "\n")
	next := func(s uint32) uint32 { s ^= s << 13; s ^= s >> 17; s ^= s << 5; return s }
	var want []float64
	a, b := uint32(2463534242), uint32(88675123)
	for i := 0; i < n; i++ {
		a, b = next(a), next(b)
		want = append(want, math.Float64frombits(uint64(a)<<32|uint64(b)))
	}
	for e := uint64(1); e < 2047; e++ {
		for d := int64(-2); d <= 2; d++ {
			want = append(want, math.Float64frombits(uint64(int64(e<<52)+d)))
		}
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d strings, want %d", len(lines), len(want))
	}
	bad := 0
	for i, x := range want {
		if w := numberToString(x); lines[i] != w {
			if bad++; bad <= 10 {
				t.Errorf("%#016x: engine %q, want %q", math.Float64bits(x), lines[i], w)
			}
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d differ", bad, len(want))
	}
}
