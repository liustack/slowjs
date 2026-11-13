// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

// A SegmentIter gives the segments of a text in order, one at a time, as
// Segments.All lists them: the same break iterator, stopped after each
// boundary. A caller that wants the first few segments of a long text breaks
// only as far as it reads, and keeps no boundary it has read. It is not safe
// for use by several goroutines at once.
type SegmentIter struct {
	s    *Segments // the text, its granularity and rules: bounds stays nil
	b    breaker
	cur  boundary
	done bool
}

// Iter starts the segments of a text given in UTF-16, which the iterator
// keeps: the caller does not change it.
func (s *Segmenter) Iter(text []uint16) *SegmentIter {
	it := &SegmentIter{
		s: &Segments{text: text, granularity: s.granularity, rules: s.rules},
		b: breaker{rules: s.rules, engines: s.engines, t: utext{text: text}},
	}
	it.cur = it.b.first()
	return it
}

// SetLimits makes the iterator check l in a long run a dictionary divides
// (Limits). A panic of l leaves the iterator broken: the caller drops it.
func (it *SegmentIter) SetLimits(l *Limits) { it.b.limits = l }

// Next is the next segment, and false once the text has none left.
func (it *SegmentIter) Next() (Segment, bool) {
	if it.done {
		return Segment{}, false
	}
	next, ok := it.b.following(it.cur)
	if !ok {
		it.done = true
		return Segment{}, false
	}
	seg := Segment{it.cur.pos, next.pos, it.s.wordLike(next)}
	it.cur = next
	return seg, true
}
