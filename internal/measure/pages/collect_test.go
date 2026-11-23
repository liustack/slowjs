// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package pages

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// cycleBundle leaves 200 objects in cycles behind each render, too few for the engine to collect
// by itself, so that only the collection after it frees them. A FinalizationRegistry watches one
// of them, and each render notes in cycles.missed how many of the cycles the renders before it
// left are not freed yet, so that a test sees that the collection ran after each render without a
// clock.
const cycleBundle = `const cycles = globalThis.cycles = { renders: 0, freed: 0, missed: [] }
const fr = new FinalizationRegistry(() => { cycles.freed++ })
globalThis.startRender = (input) => {
  cycles.missed.push(cycles.renders - cycles.freed)
  cycles.renders++
  for (let i = 0; i < 100; i++) { const a = {}, b = { a }; a.b = b; if (i === 0) fr.register(a, 0) }
  globalThis.renderResult = '<p>' + input + '</p>'
}
`

// The time of a render with -collect holds the collection the pool runs after it, and p50-ms
// does not. The collection is seen to run after each render by the cycles it frees, not by the
// clock, which on windows can read the same before and after it: the time with it is only checked
// not to be less.
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
	// a render and the collection after it: the cycle the render left is freed before the next
	// one, and the last one's before the test reads it
	missed, err := r.vm.Eval("JSON.stringify(cycles.missed.concat(cycles.renders - cycles.freed))", 0)
	if err != nil {
		t.Fatal(err)
	}
	if missed != "[0,0,0,0,0,0,0]" {
		t.Fatalf("the cycles each render found not freed of the renders before it, and after the last: %v", missed)
	}
	for i := range render {
		if collect[i] < render[i] {
			t.Fatalf("timed render %d: %v with the collection after it, %v without", i, collect[i], render[i])
		}
	}
}
