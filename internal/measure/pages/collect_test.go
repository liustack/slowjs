// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// cycleBundle leaves 20000 objects in cycles behind each render, which only the collection after
// it frees.
const cycleBundle = `globalThis.startRender = (input) => {
  for (let i = 0; i < 10000; i++) { const a = {}, b = { a }; a.b = b }
  globalThis.renderResult = '<p>' + input + '</p>'
}
`

// The time of a render with -collect holds the collection the pool runs after it, and p50-ms
// does not.
func TestTimeRendersWithCollection(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "bundle.js")
	if err := os.WriteFile(bundle, []byte(cycleBundle), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(Options{Setup: Setup{Mode: Tifa}, Limit: defaultMemory, Timeout: time.Minute, Bundle: bundle})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var html string
	render, collect, err := timeRenders(r, "x", 1, 5, func(h string) { html = h })
	if err != nil {
		t.Fatal(err)
	}
	if html != "<p>x</p>" {
		t.Fatalf("the first render wrote %s", html)
	}
	if len(render) != 5 || len(collect) != 5 {
		t.Fatalf("%d renders and %d with the collection, want 5 of each", len(render), len(collect))
	}
	for i := range render {
		if collect[i] <= render[i] {
			t.Fatalf("timed render %d: %v with the collection after it, %v without", i, collect[i], render[i])
		}
	}
}
