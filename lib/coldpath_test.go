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

// testdata/coldpath.js compares values of every kind in the ways
// js_strict_eq2 does and lists keys of every kind through
// __JS_AtomToValue. testdata/coldpath.txt is the transcript of the engine
// before fixColdFrames moved their cold paths out (Node gives the same), so
// it must stay as it is.
func TestColdPathSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "coldpath.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "coldpath.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, "coldpath.js", string(js))
	gl, wl := strings.Split(got, "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, g, w)
		}
	}
}
