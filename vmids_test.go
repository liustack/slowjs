// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"sync"
	"testing"
)

// VMs made and closed on many goroutines at once take the ids of the VMs
// closed before them: a new VM never finds its id still taken.
func TestNewCloseInParallel(t *testing.T) {
	workers, rounds := 8, 200
	if testing.Short() {
		rounds = 50
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				m, err := NewVM()
				if err != nil {
					errs <- err
					return
				}
				if _, err := m.Eval(`1 + 1`, EvalGlobal); err != nil {
					errs <- err
					return
				}
				if err := m.Close(); err != nil {
					errs <- err
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
