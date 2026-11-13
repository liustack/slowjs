// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"fmt"
	"sync"
	"testing"
)

// parallelInputs are inputs whose host needs NFC (a base letter and a
// combining mark, decomposed Hangul, compatibility mappings) next to plain
// ones.
func parallelInputs() []string {
	in := []string{
		"http://éxample.com/a?b#c",
		"http://한글.kr/",
		"https://ｅｘａｍｐｌｅ.com:8443/x",
		"http://ç.example/",
		"http://å̈.example/",
		"http://éxample.א̈/",
		"http://х́.example/",
		"http://العربية.example/",
		"http://example.com/é́?é#é",
		"http://user:pw@host:80/p/q/../r;x?y=z#w",
		"http://[::1]:3000/",
		"http://0x7f.1/",
	}
	for i := 0; i < 40; i++ {
		in = append(in, fmt.Sprintf("http://h%d.é́%d.example/p%d?q=%d", i, i, i, i))
	}
	return in
}

// TestParallel runs the parser on many goroutines at once, over inputs that
// need the shared normalizer and the shared tables, and compares every result
// with the one made serially. It is for the race detector.
func TestParallel(t *testing.T) {
	inputs := parallelInputs()
	want := make([]string, len(inputs))
	for i, in := range inputs {
		u, err := parse(in, nil)
		if err != nil {
			want[i] = "failure"
		} else {
			want[i] = u.Href()
		}
	}
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			base, _ := parse("http://localhost/", nil)
			for n := 0; n < 100; n++ {
				i := (n*7 + g) % len(inputs)
				u, err := parse(inputs[i], nil)
				got := "failure"
				if err == nil {
					got = u.Href()
				}
				if got != want[i] {
					t.Errorf("%q = %q, want %q", inputs[i], got, want[i])
				}
				if r, err := parse("/a/../b?c", base); err != nil || r.Href() != "http://localhost/b?c" {
					t.Errorf("relative: %v %v", r, err)
				}
				if err == nil {
					u.SetHostname(mustInput("é́.example"))
				}
			}
		}(g)
	}
	wg.Wait()
}
