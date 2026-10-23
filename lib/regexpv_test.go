// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdata/regexpv.js compiles patterns with escaped and unescaped
// punctuators inside and outside classes, with the v flag, the u flag and
// neither, and prints what each matches or the error it throws.
// testdata/regexpv.txt is what Node prints:
//
//	node -e "console.log((0,eval)(require('fs').readFileSync('testdata/regexpv.js','utf8')))" > testdata/regexpv.txt
func TestRegexpVSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "regexpv.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "regexpv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	compareLines(t, eval(t, "regexpv.js", string(js)), strings.TrimSuffix(string(want), "\n"))
}

// testdata/shiki/regexps.json holds the 552 regexps the JavaScript engine of
// shiki 4.5.0 makes to highlight testdata/shiki/input.md (testdata/shiki/
// record.mjs records them), 536 of them with the v flag. Before
// fixClassEscapes the engine threw a SyntaxError for 204 of them.
// testdata/shikire.js compiles each and runs it from every position of every
// line of the input, but for the 3 with more capture groups than the engine
// allows. testdata/shikire.txt is what Node prints:
//
//	node -e "const fs = require('fs'); globalThis.shikiRegexps = JSON.parse(fs.readFileSync('testdata/shiki/regexps.json', 'utf8')); globalThis.shikiInput = fs.readFileSync('testdata/shiki/input.md', 'utf8'); console.log((0,eval)(fs.readFileSync('testdata/shikire.js', 'utf8')))" > testdata/shikire.txt
func TestRegexpShiki(t *testing.T) {
	list, err := os.ReadFile(filepath.Join("testdata", "shiki", "regexps.json"))
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.ReadFile(filepath.Join("testdata", "shiki", "input.md"))
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := json.Marshal(string(input))
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile(filepath.Join("testdata", "shikire.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "shikire.txt"))
	if err != nil {
		t.Fatal(err)
	}
	src := "var shikiRegexps = " + string(list) + ";\nvar shikiInput = " + string(quoted) + ";\n" + string(js)
	compareLines(t, eval(t, "shikire.js", src), strings.TrimSuffix(string(want), "\n"))
}
