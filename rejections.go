// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"unsafe"
)

// MaxTrackedRejections is the most rejections [VM.TrackRejections] keeps. A
// host logs the few a script and its jobs left, and a read holds up to five
// texts of 8 KiB of each in Go (see [VM.UnhandledRejections]).
const MaxTrackedRejections = 1024

// rejectionText is the most bytes of UTF-8 of each text Go reads of a reason
// (cutString): its string, Name, Message, Stack and FileName.
const rejectionText = 8 << 10

// What a read of the rejections takes from the room under the memory limit
// of the VM (textCut.take). Go allocates each text once, at most
// rejectionText bytes and cutMark (8,198), and the Error of each reason, and
// rounds an allocation of up to 32 KiB up to its size class, at most a
// quarter more, or 16 bytes for the smallest (runtime/sizeclasses.go: 8,198
// bytes take 9,472). So the room taken for n bytes is n + n/4 + 16 (roomOf):
//
//	textRoom      = 8,198 + 2,049 + 16                = 10,263
//	errorRoom     = the same of unsafe.Sizeof(Error{}) (112 on 64 bits: 156)
//	rejectionRoom = 5 * textRoom + errorRoom          = 51,471
//	reasonsRoom   = the same of 1,024 pointers, 8,192 = 10,256
//
// reasonsRoom is the slice of reasons, made with the first.
const (
	textRoom      = rejectionText + len(cutMark) + (rejectionText+len(cutMark))/4 + 16
	errorRoom     = int(unsafe.Sizeof(Error{})) + int(unsafe.Sizeof(Error{}))/4 + 16
	rejectionRoom = 5*textRoom + errorRoom
	reasonsSize   = MaxTrackedRejections * int(unsafe.Sizeof((*Error)(nil)))
	reasonsRoom   = reasonsSize + reasonsSize/4 + 16
)

// roomOf is the room taken for n bytes Go allocates (see textRoom).
func roomOf(n int) int { return n + n/4 + 16 }

// textCut is how readError reads the texts of a reason of
// UnhandledRejections, or of the error it ends with: each cut (cutString),
// and with room, each taken first from the room m has under its memory
// limit. held is what the read took so far.
type textCut struct {
	room bool
	held int
}

// take takes the room for n bytes that Go is to allocate, when t takes the
// room. When the room m has left under its memory limit is less than held
// and that, it leaves the out of memory error pending and returns false.
func (t *textCut) take(m *VM, n int) bool {
	if !t.room {
		return true
	}
	r := roomOf(n)
	if room := m.goRoom(); room >= 0 && int64(t.held+r) > room {
		m.goOutOfMemory()
		return false
	}
	t.held += r
	return true
}

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
// What Go allocates for the reasons is taken from the room m has under its
// memory limit (see [VM.SetMemoryLimit]): before each reason, the most it
// may take, about 50 KiB, and then each text before it is copied, after the
// getters that ran before it. When the room left is less, UnhandledRejections
// returns the reasons read before and the out of memory error, and the
// rejections after the one it was reading stay kept. So a read allocates at
// most about 50 MiB in Go for 1,024 reasons, of which it keeps at most
// 40 MiB of texts. Javascript a getter runs after a text was copied can still
// use the room it took, so m and the read together may pass the limit by
// what the read holds in Go.
//
// The error a read ends with has its texts cut the same way, so that all it
// returns is bounded. The errors of the other calls are not cut.
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
	t := &textCut{room: true}
	for len(reasons) < MaxTrackedRejections && m.jsHasRejection(mark) {
		if m.goStop() {
			return reasons, dropped, m.readFailed()
		}
		if reasons == nil {
			t.held = reasonsRoom
		}
		// The room for all a reason may take, before it is taken off the
		// list: when there is not that much, it stays kept. Each text then
		// takes its own, after the getters that ran before it.
		if room := m.goRoom(); room >= 0 && int64(t.held+rejectionRoom) > room {
			m.goOutOfMemory()
			return reasons, dropped, m.readFailed()
		}
		if reasons == nil {
			reasons = make([]*Error, 0, MaxTrackedRejections)
		}
		t.held += errorRoom
		v, _ := m.jsTakeRejection(mark)
		e, ok := m.readError(v, t)
		m.jsFreeValue(v)
		if !ok {
			return reasons, dropped, m.readFailed()
		}
		reasons = append(reasons, e)
	}
	return reasons, dropped, nil
}

// readFailed is the error a read of the rejections ends with, the exception
// pending, its texts cut as those of the reasons, so that all a read returns
// is bounded. It takes no room: it is the error that tells the room ran out.
func (m *VM) readFailed() error {
	err, _ := m.takeExceptionCut(&textCut{})
	return err
}
