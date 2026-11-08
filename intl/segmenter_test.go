// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// segmentText is a text of about 100,000 code units in several scripts, with
// words a dictionary breaks (Thai, Chinese, Japanese, Khmer), emoji, marks
// and lone surrogates.
func segmentText() []uint16 {
	piece := "Hello, world! สวัสดีครับ ผมชื่อสมชาย 我们今天去北京大学 日本語のテキストです។ ខ្ញុំស្រលាញ់អ្នក 👍🏽 👨‍👩‍👧 é̃ 123.45 Mr. Smith left. Did he? Yes.\n"
	var u []uint16
	for i := 0; len(u) < 100_000; i++ {
		u = append(u, utf16.Encode([]rune(piece))...)
		if i%7 == 3 {
			u = append(u, 0xD800, 'a', 0xDC00)
		}
	}
	return u
}

// jsString is the javascript literal of the code units u.
func jsString(u []uint16) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range u {
		fmt.Fprintf(&b, "\\u%04x", c)
	}
	b.WriteByte('"')
	return b.String()
}

// smallText is how much of segmentText the test of many Segments objects
// reads, in code units.
const smallText = 8000

// joined is segs as the test's javascript writes them: the index, length and
// word-likeness (2 for none) of each.
func joined(segs []gointl.Segment, granularity string) string {
	var w strings.Builder
	for i, seg := range segs {
		if i > 0 {
			w.WriteByte(',')
		}
		word := 2
		if granularity == "word" {
			word = map[bool]int{false: 0, true: 1}[seg.IsWordLike]
		}
		fmt.Fprintf(&w, "%d,%d,%d", seg.Index, seg.End-seg.Index, word)
	}
	return w.String()
}

// The segments of a Segments object, found a batch at a time as javascript
// reads them, are those of one walk of the whole text: through the iterator,
// through containing at any offset, and while more Segments objects are read
// in turn than a VM keeps cursors for, so that some are made again.
func TestSegmentsInBatches(t *testing.T) {
	text := segmentText()
	vm := newVM(t)
	eval(t, vm, "var text = "+jsString(text)+"; text.length")
	for _, c := range []struct{ locale, granularity string }{
		{"en", "grapheme"}, {"en", "word"}, {"en", "sentence"}, {"th", "word"}, {"ja", "word"}, {"zh", "sentence"},
	} {
		t.Run(c.locale+"/"+c.granularity, func(t *testing.T) {
			s, err := resolveSegmenter(request{Locales: []string{c.locale}, Options: map[string]any{"granularity": c.granularity}})
			if err != nil {
				t.Fatal(err)
			}
			want := s.s.Segment(text).All()
			if len(want) < 2*maxBatch/4 && c.granularity == "grapheme" {
				t.Fatalf("only %d segments", len(want))
			}
			w := joined(want, c.granularity)

			setup := fmt.Sprintf(`var seg = new Intl.Segmenter(%q, {granularity: %q});
				function entry(x) { return [x.index, x.segment.length, x.isWordLike === undefined ? 2 : +x.isWordLike]; }`, c.locale, c.granularity)
			eval(t, vm, setup)
			if got := eval(t, vm, `var out = []; for (const x of seg.segment(text)) out.push(...entry(x)); out.join()`); got != w {
				t.Fatalf("the iterator gives other segments")
			}

			// containing, each on a new Segments object, and on one that
			// is asked for offsets in any order.
			var at []string
			var wantAt strings.Builder
			for i := 0; i < len(want); i += 1 + len(want)/50 {
				for _, j := range []int{i, len(want) - 1 - i} {
					seg := want[j]
					at = append(at, strconv.Itoa(seg.End-1))
					fmt.Fprintf(&wantAt, "%d,%d;", seg.Index, seg.End-seg.Index)
				}
			}
			js := `var one = seg.segment(text); [` + strings.Join(at, ",") + `].map(n => { const a = seg.segment(text).containing(n), b = one.containing(n); return a.index === b.index && a.segment === b.segment ? a.index + "," + a.segment.length + ";" : "differ"; }).join("")`
			if got := eval(t, vm, js); got != wantAt.String() {
				t.Fatalf("containing gives other segments:\n%v\nwant\n%v", got, wantAt.String())
			}

			// More Segments objects than a VM keeps cursors for, read in
			// turn, a batch or so at a time, on the start of the text: each
			// cursor made again walks again from there.
			js = fmt.Sprintf(`var its = Array.from({length: %d}, () => seg.segment(text.slice(0, %d))[Symbol.iterator]()), outs = its.map(() => []), live = its.length;
				while (live > 0) {
					live = 0;
					its.forEach((it, k) => { for (let i = 0; i < 100 + 37 * k; i++) { const r = it.next(); if (r.done) return; outs[k].push(...entry(r.value)); } live++; });
				}
				outs.map(o => o.join()).every(o => o === outs[0].join()) ? outs[0].join() : "differ"`, maxCursors+2, smallText)
			if got := eval(t, vm, js); got != joined(s.s.Segment(text[:smallText]).All(), c.granularity) {
				t.Fatalf("Segments read in turn give other segments")
			}
		})
	}
}

// The first segments of a long text cost Go about what the text costs in
// UTF-16, not what every segment of it would: containing(0) on 8 MiB of
// ASCII made go-intl list all 8,388,608 graphemes, 1.2 GiB of allocations in
// Go for a VM limited to 64 MiB.
func TestSegmentsMemory(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `new Intl.Segmenter('en').segment('a').containing(0); var text = 'a'.repeat(8 << 20); 1`)
	vm.SetMemoryLimit(64 << 20)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got := eval(t, vm, `const s = new Intl.Segmenter('en').segment(text), it = s[Symbol.iterator]();
		[s.containing(0).segment, s.containing(0).input === text, it.next().value.index, it.next().value.index, s.containing(100).index].join()`)
	runtime.ReadMemStats(&after)
	if got != "a,true,0,1,100" {
		t.Fatalf("got %v", got)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 64<<20 {
		t.Fatalf("Go allocated %d MiB for the first segments of 8 MiB", alloc>>20)
	}
}
