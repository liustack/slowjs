// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/liustack/slowjs"
)

// TestTamper runs pages that replace or change built-ins, Array.prototype.push,
// Function.prototype.call, Object.defineProperty, WeakMap.prototype.get and
// the like, before their first use of Intl, and compares what Intl answers
// with Chromium's answers (testdata/chromium/tamper.js). Intl's own code
// must not depend on anything a page can change.
func TestTamper(t *testing.T) {
	src, err := os.ReadFile("testdata/chromium/tamper.js")
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile("testdata/chromium/tamper.json")
	if err != nil {
		t.Fatal(err)
	}

	var want map[string]string
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}

	plain, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer plain.Close()

	var scripts [][2]string
	if err := json.Unmarshal([]byte(eval(t, plain, string(src)+"\nJSON.stringify(scripts)").(string)), &scripts); err != nil {
		t.Fatal(err)
	}

	if len(scripts) != len(want) {
		t.Fatalf("%d scripts, %d answers", len(scripts), len(want))
	}

	for _, s := range scripts {
		name, src := s[0], s[1]
		t.Run(name, func(t *testing.T) {
			got, err := newVM(t).Eval(src, slowjs.EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}

			g := strings.Split(fmt.Sprint(got), "\n")
			w := strings.Split(want[name], "\n")
			for i := range max(len(g), len(w)) {
				var gi, wi string
				if i < len(g) {
					gi = g[i]
				}
				if i < len(w) {
					wi = w[i]
				}
				if gi != wi {
					t.Errorf("use %d\n\tgot  %s\n\twant %s", i, gi, wi)
				}
			}
		})
	}
}
