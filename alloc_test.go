// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"sync"
	"testing"
)

// allocScript builds and drops objects, arrays and strings of many sizes
// and shapes, as a render does, and returns a sum of what it built.
const allocScript = `
function work(seed) {
	let sum = 0
	for (let i = 0; i < 300; i++) {
		const o = {id: i, name: "item" + (seed + i), tags: [i, i + 1, i + 2]}
		if (i % 3 == 0) o.extra = {x: i, y: String(i).repeat(i % 7)}
		const copy = {...o, more: Object.keys(o).length}
		sum += copy.more + copy.tags.length + copy.name.length
	}
	return sum
}
`

// VMs on several goroutines build and drop the same objects with the same
// results, and after a GC each VM's memory stays where it was after the
// first rounds: what one VM frees stays with it, and nothing builds up.
func TestConcurrentVMsMemory(t *testing.T) {
	const vms, rounds = 8, 40
	var wg sync.WaitGroup
	errs := make(chan error, vms)
	for i := range vms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := NewVM()
			if err != nil {
				errs <- err
				return
			}

			defer m.Close()

			if _, err := m.Eval(allocScript, EvalGlobal); err != nil {
				errs <- err
				return
			}

			var want any
			var settled int64
			for n := range rounds {
				v, err := m.Call("work", i)
				if err != nil {
					errs <- err
					return
				}

				if n == 0 {
					want = v
				} else if v != want {
					errs <- fmt.Errorf("vm %d round %d: got %v, want %v", i, n, v, want)
					return
				}

				used := int64(vmMemory(m).Fmalloc_size)
				switch {
				case n == 4:
					settled = used
				case n > 4 && used > settled:
					errs <- fmt.Errorf("vm %d round %d: %d bytes after GC, %d after round 4", i, n, used, settled)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
