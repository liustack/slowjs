// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdata/regex.js runs a set of regexps on a set of strings through every
// API that runs a regexp. testdata/regex.txt is the transcript of the engine
// before lreSkip, so skipping the positions where a match cannot start must
// leave every line as it was.
func TestRegexSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "regex.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "regex.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, "regex.js", string(js))
	if got == strings.TrimSuffix(string(want), "\n") {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	errs := 0
	for i := 0; (i < len(gl) || i < len(wl)) && errs < 20; i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, g, w)
			errs++
		}
	}
}
