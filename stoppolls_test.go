// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"testing"
	"time"
)

// The stop tests judge a stop by the work the engine did after the request
// (Interrupt or the eval timeout), not by the clock: the polls that saw the
// request (gohost StopPolls), each 10,000 ticks of the generated code, 1 when
// the engine stopped before the poll after the one that set the flag. The
// count depends on the code the engine runs between two readings of the
// flag, not on how fast the machine is, how loaded it is or whether the race
// detector is on, so a runner that stalls the thread does not fail a test.

// stopPolls is the most polls a stop test lets see the request. The loops of
// the tests read the flag at least every few thousand ticks, or every few
// tens of thousands where a step is long, and so stop within 4 polls.
// Each of the old gaps these tests met (a rope made flat in one copy, the
// long loops of the tag functions) was 13 polls or more.
const stopPolls = 5

// stepStopPolls is the limit of a loop that reads the flag every 65,536
// characters and spends about a tick on each (repeat writing one character
// at a time, the backward search of lastIndexOf, a search in a string of 16
// bits): a step is 6.6 polls of 10,000 ticks, so up to 7 polls see the
// request. A call of 'x'.repeat(1e5) that reads no flag is 10.
const stepStopPolls = 9

// checkStopped fails t unless err is the uncatchable interrupt and the engine
// of m saw the request in at most limit polls. took is only for the message.
func checkStopped(t *testing.T, m *VM, err error, took time.Duration, limit int64, what string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
		t.Fatalf("%sgot %v, want interrupted", what, err)
	}
	if n := m.h.StopPolls(); n > limit {
		t.Errorf("%s%d polls saw the request to stop, want at most %d (stopped %v after the call started)", what, n, limit, took)
	}
}
