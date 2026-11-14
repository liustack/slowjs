// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import "fmt"

// MaxTrackedRejections is the most rejections [VM.TrackRejections] keeps. A
// host logs the few a script and its jobs left, and a read holds up to five
// texts of 8 KiB of each in Go (see [VM.UnhandledRejections]).
const MaxTrackedRejections = 1024

// rejectionText is the most bytes of UTF-8 of each text Go reads of a reason
// (cutString): its string, Name, Message, Stack and FileName.
const rejectionText = 8 << 10

// rejectionRoom is the most bytes a read of one reason makes Go allocate:
// each of its five texts written into a buffer and copied, with cutMark,
// and the Error.
const rejectionRoom = 2*5*(rejectionText+len(cutMark)) + 256

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
// MaxTrackedRejections, when m has no memory for the list, and when it is
// called while UnhandledRejections reads m, from javascript the read runs:
// it then changes nothing.
// errReadingRejections is the error of TrackRejections and
// UnhandledRejections called from javascript that a read of the rejections
// runs.
func errReadingRejections(name string) error {
	return fmt.Errorf("quickjs: %s: called while UnhandledRejections reads the rejections", name)
}

func (m *VM) TrackRejections(max int) (err error) {
	if m.readingRejections {
		return errReadingRejections("TrackRejections")
	}
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
// [VM.TrackRejections]), oldest first, at most MaxTrackedRejections, and
// forgets them. Each reason is read as the error of a call that threw it is:
// an Error gives its Name, Message and Stack, any other value its text. Each
// text is cut to its first 8 KiB of UTF-8, or up to three bytes fewer so
// that no character is cut, followed by " [cut]" when it was longer: a
// reason is for a log, and javascript can make a string of hundreds of MiB,
// or a thousand references to one. dropped is how many rejections were not
// kept since the last call, past the limit or with no memory: a handler may
// have been attached to some of them since.
//
// Reading a reason runs javascript, such as a getter of its message, inside
// the eval timeout. A rejection that this javascript makes is kept for the
// next call, after its jobs could attach a handler to it. The timeout and
// [VM.Interrupt] are read before each reason too. When one ends the call,
// UnhandledRejections returns the reasons read before it and the interrupt,
// and the rejections after the one it was reading stay kept.
//
// What Go keeps of the reasons is taken from the room m has under its memory
// limit (see [VM.SetMemoryLimit]): before each reason, UnhandledRejections
// checks that the room left holds what reading it may make Go allocate,
// about 80 KiB, twice its five texts. When it does not, it returns the
// reasons read before and the out of memory error, and the others stay kept.
// So a read holds at most 40 MiB of texts in Go (1,024 reasons of five texts
// of 8 KiB), and less than the room under the limit.
//
// A read is the only one on m while it runs: called from javascript it runs,
// UnhandledRejections returns an error and takes nothing, as TrackRejections
// does.
func (m *VM) UnhandledRejections() (reasons []*Error, dropped int, err error) {
	if m.readingRejections {
		return nil, 0, errReadingRejections("UnhandledRejections")
	}
	if err = m.enter(); err != nil {
		return nil, 0, err
	}
	defer m.leave(&err)
	m.readingRejections = true
	defer func() { m.readingRejections = false }()
	dropped = int(m.jsTakeDroppedRejections())
	// The rejections kept from here on, by javascript a read runs, are for
	// the next call.
	mark := m.jsRejectionMark()
	var held int64
	for len(reasons) < MaxTrackedRejections && m.jsHasRejection(mark) {
		if m.goStop() {
			return reasons, dropped, m.errFromException()
		}
		if room := m.goRoom(); room >= 0 && held+int64(rejectionRoom) > room {
			m.goOutOfMemory()
			return reasons, dropped, m.errFromException()
		}
		v, _ := m.jsTakeRejection(mark)
		e, ok := m.readError(v, rejectionText)
		m.jsFreeValue(v)
		if !ok {
			return reasons, dropped, m.errFromException()
		}
		held += 2*int64(len(e.raw)+len(e.Name)+len(e.Message)+len(e.Stack)+len(e.FileName)) + 256
		reasons = append(reasons, e)
	}
	return reasons, dropped, nil
}
