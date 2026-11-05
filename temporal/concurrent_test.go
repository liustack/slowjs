// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/liustack/quickjs"
)

// TestConcurrentVMs uses Temporal on several VMs at once, each with a clock
// of its own. The bytecode, the zone and calendar data and the zones they
// share are made by whichever gets there first, so the VMs start together
// and are checked against a lone one after. Run it with -race.
func TestConcurrentVMs(t *testing.T) {
	probe := probeScript()
	const workers = 6
	var got [workers]string
	var now [workers]string
	var wg sync.WaitGroup
	start := make(chan struct{})
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm, err := quickjs.NewVM()
			if err != nil {
				t.Error(err)
				return
			}

			defer vm.Close()

			at := time.Unix(int64(w)*86400, int64(w))
			if err := Register(vm, Clock(func() time.Time { return at })); err != nil {
				t.Error(err)
				return
			}

			<-start
			for range 3 {
				v, err := vm.Eval(probe, quickjs.EvalGlobal)
				if err != nil {
					t.Error(err)
					return
				}
				got[w] = fmt.Sprint(v)
				v, err = vm.Eval(`Temporal.Now.instant().toString() + ' ' + Temporal.Now.zonedDateTimeISO('Pacific/Chatham').offset`, quickjs.EvalGlobal)
				if err != nil {
					t.Error(err)
					return
				}
				now[w] = fmt.Sprint(v)
			}
		}()
	}
	close(start)
	wg.Wait()

	want := eval(t, newVM(t), probe).(string)
	for w := range workers {
		if got[w] != want {
			t.Errorf("VM %d:", w)
			diff(t, got[w], want)
		}
		frac := ""
		if w > 0 {
			frac = fmt.Sprintf(".%09d", w)
		}
		if want := fmt.Sprintf("1970-01-%02dT00:00:00%sZ +12:45", w+1, frac); now[w] != want {
			t.Errorf("VM %d: Now %q, want %q", w, now[w], want)
		}
	}
}
