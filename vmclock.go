// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	"time"

	lib "github.com/liustack/slowjs/lib"
)

// SetClock sets the wall clock of this VM. Date.now(), new Date(), Date(),
// Intl's current time and Temporal.Now (unless temporal.Clock overrides it)
// read now. nil restores time.Now. Set it only between calls into JavaScript,
// never concurrently with the VM or from a callback running inside it.
// The callback must not block. If several VMs share it, it must be safe for
// concurrent calls. Close releases it. It does not affect performance.now(),
// eval timeouts, Atomics.wait or the random seed.
func (m *VM) SetClock(now func() time.Time) {
	lib.SetClock(m.runtime.cRuntime, now)
}

// Now reads the VM's wall clock with time.Time precision. Date and Intl use
// whole milliseconds, while Temporal preserves nanoseconds.
// Like the VM, Now is not safe for concurrent use.
func (m *VM) Now() time.Time { return lib.Now(m.runtime.cRuntime) }
