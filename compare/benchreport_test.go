// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package compare

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBenchReport: benchreport.go (the report of make benchmark) refuses the
// output of a run that is not complete, and reports one that is.
func TestBenchReport(t *testing.T) {
	const good = "BenchmarkA/x-8 10 100 ns/op\nBenchmarkA/y-8 10 200 ns/op\nBenchmarkB/x-8 10 300 ns/op\nBenchmarkB/y-8 10 400 ns/op\nPASS\n"
	for _, c := range []struct {
		name, in, want string // want: "" passes, else a part of the message
	}{
		{"a complete run", good, ""},
		{"no result", "PASS\n", "no benchmark result"},
		{"a failed benchmark", "BenchmarkA/x-8 10 100 ns/op\n--- FAIL: BenchmarkB/x\nFAIL\n", "the run failed"},
		{"a failed package", good + "FAIL\texample\t1.2s\n", "the run failed"},
		{"a missing result", "BenchmarkA/x-8 10 100 ns/op\nBenchmarkA/y-8 10 200 ns/op\nBenchmarkB/x-8 10 300 ns/op\n", "B has no result for y"},
		{"a zero time", strings.Replace(good, "300", "0", 1), "not a positive time"},
		{"a time that is no number", strings.Replace(good, "300", "NaN", 1), "not a positive time"},
		{"a negative time", strings.Replace(good, "300", "-3", 1), "not a positive time"},
	} {
		cmd := exec.Command("go", "run", "benchreport.go")
		cmd.Stdin = strings.NewReader(c.in)
		out, err := cmd.CombinedOutput()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v\n%s", c.name, err, out)
		case c.want == "" && !strings.Contains(string(out), "geomean"):
			t.Errorf("%s: no report:\n%s", c.name, out)
		case c.want != "" && (err == nil || !strings.Contains(string(out), c.want)):
			t.Errorf("%s: err %v, output %q, want %q", c.name, err, out, c.want)
		}
	}
}
