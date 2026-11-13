// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"sync"

	"github.com/liustack/quickjs"
	gointl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/intlhook"
)

// segmenter is a resolved Intl.Segmenter.
type segmenter struct {
	Locale      string `json:"locale"`
	Granularity string `json:"granularity"`

	s *gointl.Segmenter
}

// resolveSegmenter is the Intl.Segmenter constructor but for reading the
// options, which intl.js does.
func resolveSegmenter(req request) (*segmenter, error) {
	o := req.Options
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	loc, err := goResolve(req.vm, gointl.ServiceSegmenter, req.Locales)
	if err != nil {
		return nil, err
	}

	var opts gointl.SegmenterOptions
	switch g, err := optString(o, "granularity", "grapheme", "word", "sentence"); {
	case err != nil:
		return nil, err
	case g == "word":
		opts.Granularity = gointl.GranularityWord
	case g == "sentence":
		opts.Granularity = gointl.GranularitySentence
	}
	s, err := gointl.NewSegmenter(loc, opts)
	if err != nil {
		return nil, err
	}

	r := s.ResolvedOptions()
	return &segmenter{Locale: r.Locale, Granularity: r.Granularity.String(), s: s}, nil
}

// init has nothing to do: go-intl made the segmenter while resolving it.
func (s *segmenter) init() error { return nil }

// The breaks of a Segments object are found as javascript asks for them,
// with containing or the iterator, a batch at a time: the first batch is
// small, so that the first segments of a long text cost what they cost in a
// short one, and each next batch is as long as all the batches before it, up
// to maxBatch, so that reading every segment takes few calls. A batch goes on
// where the last one stopped, with the iterator of the Segments object,
// which the VM keeps as a cursor: go-intl's breaks depend on what the
// iterator saw before (the runs of a dictionary, the engines it found), so
// they are only the same as one walk of the whole text when one iterator
// finds them all. A cursor the VM no longer keeps is made again from the
// start of the text, and walks past the breaks javascript has: the same
// breaks come out, for more time.

const (
	firstBatch = 64
	maxBatch   = 1 << 16
	// maxCursors is how many cursors a VM keeps, those used last. Each holds
	// its text in UTF-16, twice the bytes of an ASCII string the VM holds.
	maxCursors = 4
)

// A cursor is the iterator of one Segments object and how many breaks it has
// handed out.
type cursor struct {
	id   int
	it   *gointl.SegmentIter
	word bool
	n    int
}

// vmCursors are the cursors of a VM, the one used last at the end, and the
// id of the next one. Only the goroutine running the VM uses them.
type vmCursors struct {
	next int
	list []*cursor
}

// cursors holds the cursors of each VM until it is closed.
var cursors sync.Map // *quickjs.VM -> *vmCursors

func cursorsOf(vm *quickjs.VM) *vmCursors {
	if c, ok := cursors.Load(vm); ok {
		return c.(*vmCursors)
	}
	c := &vmCursors{next: 1}
	cursors.Store(vm, c)
	vm.OnClose(func() { cursors.Delete(vm) })
	return c
}

// take returns the cursor id, which it moves to the end of the list, or nil.
func (v *vmCursors) take(id int) *cursor {
	for i, c := range v.list {
		if c.id == id {
			copy(v.list[i:], v.list[i+1:])
			v.list[len(v.list)-1] = c
			return c
		}
	}
	return nil
}

// drop forgets c, and clears the slot it leaves at the end of the list, so
// that Go can free its text.
func (v *vmCursors) drop(c *cursor) {
	for i, x := range v.list {
		if x == c {
			n := len(v.list) - 1
			copy(v.list[i:], v.list[i+1:])
			v.list[n] = nil
			v.list = v.list[:n]
			return
		}
	}
}

// add keeps c as the cursor used last, dropping the one used first when the
// VM keeps maxCursors.
func (v *vmCursors) add(c *cursor) {
	if len(v.list) == maxCursors {
		copy(v.list, v.list[1:])
		v.list = v.list[:len(v.list)-1]
	}
	v.list = append(v.list, c)
}

// segBreaks is the Builtin javascript asks for the next breaks of a Segments
// object with: the id of its segmenter, its string, its cursor (0 for none
// yet) and the number of breaks it has. It answers the cursor, 1 if the
// segments may be word-like, for the word granularity, else 0, then where
// each next segment ends, in UTF-16 code units, as twice the offset, plus one
// for a segment that is word-like. The breaks are all
// there once the last one is the length of the text.
var segBreaks = intlhook.NewBuiltin("breaks", 4, func(a quickjs.Args) (quickjs.Value, error) {
	vm := a.VM()
	id, _ := a.Number(2)
	have, _ := a.Number(3)
	vc := cursorsOf(vm)
	c := vc.take(int(id))
	// A stop or a lack of room in a long run of a dictionary ends the
	// call inside Next, and leaves the iterator broken: the VM drops the
	// cursor, and a next call makes it again.
	inNext := false
	defer func() {
		if inNext {
			vc.drop(c)
		}
	}()
	if c == nil || c.n != int(have) {
		s, ok, err := formatterOf[*segmenter](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		door := intlhook.Open(a)
		text, _ := door.LongUTF16(1)
		c = &cursor{id: vc.next, it: s.s.Iter(text), word: s.Granularity == "word"}
		c.it.SetLimits(limitsOf(vm))
		vc.next++
		vc.add(c)
		// A cursor made again walks past the breaks javascript has.
		for i := range int(have) {
			checkTurn(vm, i)
			inNext = true
			_, ok := c.it.Next()
			inNext = false
			if !ok {
				return quickjs.Value{}, errors.New("intl: internal error: a Segments object has more breaks than its text")
			}
			c.n++
		}
	}

	n := min(max(c.n, firstBatch), maxBatch)
	r := make([]int, 2, 2+n)
	r[0] = c.id
	if c.word {
		r[1] = 1
	}
	for i := range n {
		checkTurn(vm, i)
		inNext = true
		seg, ok := c.it.Next()
		inNext = false
		if !ok {
			break
		}
		b := 2 * seg.End
		if seg.IsWordLike {
			b++
		}
		r = append(r, b)
		c.n++
	}
	return intlhook.EmitInts(vm, r)
})
