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

// The 46 cases of testdata/objlit.js print what each definition leaves:
// keys in order, values and attributes, or the error. testdata/objlit.txt is
// the transcript of the engine before the fast path of objlit.go, so a
// change to how a new property is defined must leave every line as it was.
func TestObjlitSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "objlit.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "objlit.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, "objlit.js", string(js))
	if got == strings.TrimSuffix(string(want), "\n") {
		return
	}
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
