// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import "fmt"

// MaxTrackedRejections is the most rejections [VM.TrackRejections] keeps.
const MaxTrackedRejections = 1 << 16

// TrackRejections makes m keep the promises that are rejected while no
// handler is attached to them, at most max of them, oldest first, until a
// handler is attached (then, catch, finally or an await) or
// [VM.UnhandledRejections] takes them. A rejection past max, or one m has
// no memory to keep, is counted instead. A call with another max changes
// the limit and keeps what is kept, even past a lower one. max 0 turns
// keeping off and forgets what is kept, which is how a VM starts: a VM that
// keeps none rejects a promise as it always did.
//
// What m keeps is a small block for each rejection, with a reference to the
// promise and to its reason, under the memory limit of m: they stay alive
// until they are taken. A host runs a script and its jobs, then calls
// UnhandledRejections to report the rejections no handler took.
//
// TrackRejections returns an error when max is below 0 or above
// MaxTrackedRejections, and when m has no memory for the list.
func (m *VM) TrackRejections(max int) (err error) {
	if max < 0 || max > MaxTrackedRejections {
		return fmt.Errorf("quickjs: TrackRejections(%d): max is from 0 to %d", max, MaxTrackedRejections)
	}
	if err = m.enter(); err != nil {
		return err
	}
	defer m.leave(&err)
	if !m.jsTrackRejections(uint32(max)) {
		return outOfMemoryError()
	}
	return nil
}

// UnhandledRejections takes the rejections m kept when it is called (see
// [VM.TrackRejections]), oldest first, and forgets them. Each reason is read
// as the error of a call that threw it is: an Error gives its Name, Message
// and Stack, any other value its text. dropped is how many rejections were
// not kept since the last call, past the limit or with no memory: a handler
// may have been attached to some of them since.
//
// Reading a reason runs javascript, such as a getter of its message, inside
// the eval timeout. A rejection that this javascript makes is kept for the
// next call, after its jobs could attach a handler to it. When that
// javascript ends with an uncatchable error, such as the timeout,
// UnhandledRejections returns the reasons read before it and that error,
// and the rejections after the one it was reading stay kept.
func (m *VM) UnhandledRejections() (reasons []*Error, dropped int, err error) {
	if err = m.enter(); err != nil {
		return nil, 0, err
	}
	defer m.leave(&err)
	dropped = int(m.jsTakeDroppedRejections())
	// The rejections kept from here on, by javascript a read runs, are for
	// the next call.
	mark := m.jsRejectionMark()
	for {
		v, ok := m.jsTakeRejection(mark)
		if !ok {
			return reasons, dropped, nil
		}
		e, ok := m.readError(v)
		m.jsFreeValue(v)
		if !ok {
			return reasons, dropped, m.errFromException()
		}
		reasons = append(reasons, e)
	}
}
