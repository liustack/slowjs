// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl

import (
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/liustack/tifajs/internal/hostmem"
)

// beforeEngine is the engine of Chinese and Japanese with divide as it was
// before it took its arrays from blocks a piece at a time (divideBefore).
type beforeEngine struct{ *cjkEngine }

func (e beforeEngine) findBreaks(t *utext, start, end int, breaks *[]int, l *Limits) int {
	return dictionaryFindBreaks(t, start, end, breaks, e.set, l, e.divideBefore)
}

// cjkPieces are what the random texts of TestCJKDivideAsBefore are made of:
// Han, kana, halfwidth katakana and voiced marks that NFKC changes, a
// character NFKC makes six, Han past the BMP, Hangul, which stays together,
// and what ends a run.
var cjkPieces = []string{
	"我们今天去北京大学看书", "中华人民共和国", "中", "文", "東京タワーに行きました",
	"カタカナ", "ｶﾀｶﾅﾃｽﾄ", "ﾞ", "ﾟ", "ﾞﾞﾞﾞ", "㌖", "𠀀", "𠀁中文𠀂", "한국어",
	"。", " ", "1949", "abc", "゙", "ア゙",
}

// The division of runs of Chinese and Japanese is the one before blocks
// and checks went into it, with no Limits, with checks and with blocks:
// random texts of cjkPieces, and long runs, past the pieces of a checked
// copy.
func TestCJKDivideAsBefore(t *testing.T) {
	if qjsNoCJDict {
		t.Skip("built without the dictionary of Chinese and Japanese")
	}
	s, err := NewSegmenter(qjsLocale("ja"), SegmenterOptions{Granularity: GranularityWord})
	if err != nil {
		t.Fatal(err)
	}
	cjk := s.Iter([]uint16{'a'}).b.engines.forChar('中').(*cjkEngine)
	segments := func(u []uint16, before bool, l *Limits) []Segment {
		it := s.Iter(u)
		if before {
			it.b.stack = append(it.b.stack, beforeEngine{cjk})
		}
		it.SetLimits(l)
		var r []Segment
		for {
			seg, ok := it.Next()
			if !ok {
				return r
			}
			r = append(r, seg)
		}
	}
	blocks := func(size int) (unsafe.Pointer, func()) {
		b, give, err := hostmem.Take[byte](size)
		if err != nil {
			panic(err)
		}
		return unsafe.Pointer(unsafe.SliceData(b)), give
	}
	limits := []*Limits{
		nil,
		{Check: func() {}, Room: func(int64) {}},
		{Check: func() {}, Room: func(int64) {}, Blocks: blocks},
	}
	held := hostmem.Blocks()
	rnd := rand.New(rand.NewPCG(3, 4))
	var texts [][]uint16
	for range 3000 {
		var b strings.Builder
		for range rnd.IntN(40) {
			b.WriteString(cjkPieces[rnd.IntN(len(cjkPieces))])
		}
		u := utf16.Encode([]rune(b.String()))
		if rnd.IntN(10) == 0 && len(u) > 0 {
			u[rnd.IntN(len(u))] = 0xd800
		}
		texts = append(texts, u)
	}
	for _, unit := range []string{"我们今天去北京大学看书", "ｶﾀｶﾅﾃｽﾄ", "𠀀𠀁中文𠀂"} {
		texts = append(texts, utf16.Encode([]rune(strings.Repeat(unit, 200000/len(unit)))))
	}
	texts = append(texts, utf16.Encode([]rune("中"+strings.Repeat("ﾞ", 100000))))
	for _, u := range texts {
		want := segments(u, true, nil)
		for i, l := range limits {
			if got := segments(u, false, l); !slices.Equal(got, want) {
				t.Fatalf("limits %d, %d units %q: the segments differ from those before", i, len(u), string(utf16.Decode(u[:min(len(u), 60)])))
			}
		}
	}
	if got := hostmem.Blocks() - held; got != 0 {
		t.Errorf("%d blocks held after the texts were divided", got)
	}
}

// divideBefore is cjkEngine.divide before it took its arrays from blocks a
// piece at a time.
func (e *cjkEngine) divideBefore(t *utext, rangeStart, rangeEnd int, breaks *[]int, l *Limits) int {
	if rangeStart >= rangeEnd {
		return 0
	}
	in := t.text[rangeStart:rangeEnd]
	if l != nil && l.Room != nil {
		l.room(int64(e.expandedUnits(in, l)) * cjkRoomPerUnit)
	}
	var inputMap []int // nil for positions offset by rangeStart
	s := string(utf16.Decode(in))
	turns := 0
	if e.norm.normalize(s, NFKC, l) != s {
		var normalized []uint16
		var nmap []int
		u := utext{text: in}
		for u.index() < len(in) {
			l.every(&turns)
			fragStart := u.index()
			var frag []rune
			c := u.next32()
			for {
				l.every(&turns)
				frag = append(frag, c)
				if u.index() == len(in) {
					break
				}
				c = u.current32()
				if e.hasBoundaryBefore(c) {
					break
				}
				u.next32()
			}
			nf := utf16.Encode([]rune(e.norm.normalize(string(frag), NFKC, l)))
			normalized = append(normalized, nf...)
			for len(nmap) < len(normalized) {
				nmap = append(nmap, fragStart+rangeStart)
			}
		}
		nmap = append(nmap, len(in)+rangeStart)
		inputMap, in = nmap, normalized
	}
	runes := utf16.Decode(in)
	numCodePts := len(runes)
	if numCodePts != len(in) {
		// Supplementary characters: the dictionary counts code points.
		var cp []int
		cu := 0
		for i := 0; ; i++ {
			l.every(&turns)
			if inputMap != nil {
				cp = append(cp, inputMap[cu])
			} else {
				cp = append(cp, cu+rangeStart)
			}
			if cu == len(in) {
				break
			}
			if isLead(in[cu]) && cu+1 < len(in) && isTrail(in[cu+1]) {
				cu += 2
			} else {
				cu++
			}
		}
		inputMap = cp
	}
	const maxUint32 = ^uint32(0)
	bestSnlp := make([]uint32, numCodePts+1)
	prev := make([]int, numCodePts+1)
	for i := range bestSnlp {
		bestSnlp[i], prev[i] = maxUint32, -1
	}
	bestSnlp[0] = 0
	values := make([]int, numCodePts+1)
	lengths := make([]int, numCodePts+1)
	fu := utext{text: in}
	ix := 0
	prevKatakana := false
	for i := 0; i < numCodePts; i++ {
		l.every(&turns)
		c, n := fu.at(ix)
		if bestSnlp[i] != maxUint32 {
			fu.setIndex(ix)
			count, _ := e.dict.matches(&fu, cjkMaxWordSize, numCodePts, nil, lengths[:numCodePts], values[:numCodePts])
			// A character that is no one-character word is one anyway, at
			// the highest cost, but for Hangul, which stays together.
			if (count == 0 || lengths[0] != 1) && !(c >= 0xac00 && c <= 0xd7a3) {
				values[count] = cjkMaxSnlp
				lengths[count] = 1
				count++
			}
			for j := 0; j < count; j++ {
				snlp := bestSnlp[i] + uint32(values[j])
				if k := lengths[j] + i; snlp < bestSnlp[k] {
					bestSnlp[k] = snlp
					prev[k] = i
				}
			}
			katakana := isKatakana(c)
			if !prevKatakana && katakana {
				run := 1
				j := ix + n
				for j < len(in) && run < cjkMaxKatakanaGroupLength {
					cj, m := fu.at(j)
					if !isKatakana(cj) {
						break
					}
					j += m
					run++
				}
				if run < cjkMaxKatakanaGroupLength {
					snlp := bestSnlp[i] + getKatakanaCost(run)
					if snlp < bestSnlp[i+run] {
						bestSnlp[i+run] = snlp
						prev[i+run] = i
					}
				}
			}
			prevKatakana = katakana
		}
		ix += n
	}
	var tb []int
	if bestSnlp[numCodePts] == maxUint32 {
		tb = append(tb, numCodePts)
	} else {
		for i := numCodePts; i > 0; i = prev[i] {
			tb = append(tb, i)
		}
	}
	if len(*breaks) == 0 || peek(*breaks) < rangeStart {
		tb = append(tb, 0)
	}
	numBreaks := len(tb)
	prevUTextPos := -1
	corrected := 0
	for i := numBreaks - 1; i >= 0; i-- {
		pos := tb[i] + rangeStart
		if inputMap != nil {
			pos = inputMap[tb[i]]
		}
		if pos > prevUTextPos {
			if pos != rangeStart {
				*breaks = append(*breaks, pos)
				corrected++
			}
		}
		prevUTextPos = pos
	}
	if len(*breaks) > 0 && peek(*breaks) == rangeEnd {
		*breaks = (*breaks)[:len(*breaks)-1]
		corrected--
	}
	return corrected
}

// The work on a long run of a dictionary checks the call between pieces of
// a size that does not grow with the run: between two checks, the arrays of
// the run get at most 1 MiB written, in a copy piece or a few thousand
// turns of a loop, and the iterator moves at most two pieces of turns over
// the text. The test hands the work the range after an "a" whole, as the
// rules do, so that the position at each check shows how far it went. The
// arrays come from blocks the test hands out and watches fill: each is
// written from its start, with no zero word, as no position in the range is
// 0. Before, after the last growth of the map of a long fragment NFKC
// changes, 512 Ki positions went into it with no check, and the walk over
// the marks after a dictionary run, to the next run, made none.
func TestDictionaryChecksEveryPiece(t *testing.T) {
	if qjsNoCJDict {
		t.Skip("built without the dictionary of Chinese and Japanese")
	}
	const n = 1 << 20
	for _, c := range []struct{ lang, name, text string }{
		{"ja", "a Han character and halfwidth voiced marks", "a中" + strings.Repeat("ﾞ", n)},
		{"ja", "a Han character and acute accents", "a中" + strings.Repeat("\u0301", n) + strings.Repeat("中", 3*limitEvery)},
		{"th", "a Thai word and acute accents", "aภาษา" + strings.Repeat("\u0301", n) + strings.Repeat("ภาษา", limitEvery)},
		{"zh", "a run of Chinese", "a" + strings.Repeat("我们今天去北京大学看书", n/11)},
		{"ja", "a run of halfwidth katakana", "a" + strings.Repeat("ｶﾀｶﾅﾃｽﾄ", n/7)},
		{"zh", "a run past the BMP", "a" + strings.Repeat("𠀀𠀁中文𠀂", n/8)},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, err := NewSegmenter(qjsLocale(c.lang), SegmenterOptions{Granularity: GranularityWord})
			if err != nil {
				t.Fatal(err)
			}
			u := utf16.Encode([]rune(c.text))
			it := s.Iter(u)
			var blocks [][]uint64
			filled := map[int]int{}
			var written, maxWritten, maxMoved int
			last := 1
			l := &Limits{Room: func(int64) {}, Blocks: func(size int) (unsafe.Pointer, func()) {
				b := make([]uint64, (size+7)/8)
				blocks = append(blocks, b)
				return unsafe.Pointer(unsafe.SliceData(b)), func() {}
			}}
			l.Check = func() {
				for i, b := range blocks {
					f := sort.Search(len(b), func(k int) bool { return b[k] == 0 })
					written += 8 * (f - filled[i])
					filled[i] = f
				}
				maxWritten, written = max(maxWritten, written), 0
				maxMoved, last = max(maxMoved, it.b.t.pos-last), it.b.t.pos
			}
			it.SetLimits(l)
			it.b.populateDictionary(1, len(u), 0, 0)
			l.Check()
			if maxWritten > 1<<20 {
				t.Errorf("the arrays of the run got %d KiB written between two checks, want at most 1024", maxWritten>>10)
			}
			if maxMoved > 2*limitEvery {
				t.Errorf("the iterator moved %d units between two checks, want at most %d", maxMoved, 2*limitEvery)
			}
		})
	}
}
