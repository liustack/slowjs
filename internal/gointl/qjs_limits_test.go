// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"math/rand"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"
)

// cjkRuns are runs of Chinese and Japanese the dictionary divides as a
// whole, each of the ways divide reads one: plain, with katakana, with
// halfwidth katakana that NFKC changes, with characters past the BMP, and
// with a character NFKC makes six.
var cjkRuns = []struct{ lang, unit string }{
	{"zh", "我们今天去北京大学看书"},
	{"ja", "東京タワーに行きました"},
	{"ja", "ｶﾀｶﾅﾃｽﾄ"},
	{"zh", "𠀀𠀁中文𠀂"},
	// Six characters in NFKC, キロメートル.
	{"ja", "㌖"},
}

// What Room asks for a run of Chinese or Japanese covers what Go takes to
// divide it: room the call does not have ends the call before Go takes it.
func TestDictionaryRoomCoversGo(t *testing.T) {
	if qjsNoCJDict {
		t.Skip("built without the dictionary of Chinese and Japanese")
	}
	const n = 1 << 20
	for _, r := range cjkRuns {
		t.Run(r.unit, func(t *testing.T) {
			s, err := NewSegmenter(qjsLocale(r.lang), SegmenterOptions{Granularity: GranularityWord})
			if err != nil {
				t.Fatal(err)
			}
			text := utf16.Encode([]rune(strings.Repeat(r.unit, n/len([]rune(r.unit)))))
			it := s.Iter(text)
			var asked int64
			it.SetLimits(&Limits{Room: func(n int64) { asked += n }})
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			if _, ok := it.Next(); !ok {
				t.Fatal("no segment")
			}
			runtime.ReadMemStats(&after)
			took := int64(after.TotalAlloc - before.TotalAlloc)
			t.Logf("Go took %.1f bytes a code unit, Room was asked %.1f", float64(took)/float64(len(text)), float64(asked)/float64(len(text)))
			if asked < took {
				t.Errorf("Room was asked for %d bytes, and Go took %d", asked, took)
			}
		})
	}
}

// The limits change no segment: the breaks of a text with them are those
// without them, in every script a dictionary divides, in short runs among
// other text and in one long run, where the checks are made.
func TestLimitsKeepSegments(t *testing.T) {
	for _, c := range []struct{ lang, mixed, run string }{
		{"th", "ภาษาไทยเป็นภาษาที่มีระดับเสียงของคำแน่นอน, สวัสดีครับ 123 ไปไหนมา", "ภาษาไทยเป็นภาษาที่มีระดับเสียงของคำแน่นอน"},
		{"lo", "ພາສາລາວເປັນພາສາທາງການຂອງປະເທດລາວ ສະບາຍດີ", "ພາສາລາວເປັນພາສາທາງການຂອງປະເທດລາວ"},
		{"km", "ភាសាខ្មែរគឺជាភាសាកំណើតរបស់ជនជាតិខ្មែរ សួស្តី", "ភាសាខ្មែរគឺជាភាសាកំណើតរបស់ជនជាតិខ្មែរ"},
		{"my", "မြန်မာဘာသာစကားသည် မြန်မာနိုင်ငံ၏ ရုံးသုံးဘာသာစကား ဖြစ်သည်။", "မြန်မာဘာသာစကားသည်"},
		{"zh", "我们今天去北京大学看书。中华人民共和国成立于1949年，𠀀𠀁中文𠀂", "我们今天去北京大学看书中华人民共和国成立𠀀𠀁中文𠀂"},
		{"ja", "東京タワーに行きました。ｶﾀｶﾅﾃｽﾄとカタカナのテスト、日本語の文章です。", "東京タワーに行きましたｶﾀｶﾅﾃｽﾄとカタカナのテスト"},
	} {
		s, err := NewSegmenter(qjsLocale(c.lang), SegmenterOptions{Granularity: GranularityWord})
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{strings.Repeat(c.mixed, 500), strings.Repeat(c.run, 2000)} {
			u := utf16.Encode([]rune(text))
			var plain, limited []Segment
			for it := s.Iter(u); ; {
				seg, ok := it.Next()
				if !ok {
					break
				}
				plain = append(plain, seg)
			}
			checks := 0
			it := s.Iter(u)
			it.SetLimits(&Limits{Check: func() { checks++ }, Room: func(int64) {}})
			for {
				seg, ok := it.Next()
				if !ok {
					break
				}
				limited = append(limited, seg)
			}
			if !slices.Equal(plain, limited) {
				t.Errorf("%s: the segments of %.20q with limits differ", c.lang, text)
			}
			if text != strings.Repeat(c.mixed, 500) && checks == 0 {
				t.Errorf("%s: no check in a run of %d units", c.lang, len(u))
			}
		}
	}
}

// sortMarks orders a run of accents as a stable sort by combining class
// does, for runs of every length up to past the one it sorts in place.
func TestSortMarksStable(t *testing.T) {
	n := newEmbeddedNormalizer(t)
	// Accents of classes 220, 230, 202, 1 and 232, some of each class.
	marks := []rune{0x0316, 0x0317, 0x0301, 0x0300, 0x0327, 0x0328, 0x0334, 0x0335, 0x031a, 0x0358}
	rnd := rand.New(rand.NewSource(1))
	for size := 0; size < 3*marksSorted; size++ {
		for round := 0; round < 20; round++ {
			run := make([]rune, size)
			for i := range run {
				run[i] = marks[rnd.Intn(len(marks))]
			}
			want := slices.Clone(run)
			sort.SliceStable(want, func(a, b int) bool { return n.tables.Class(want[a]) < n.tables.Class(want[b]) })
			n.sortMarks(run, nil)
			if !slices.Equal(run, want) {
				t.Fatalf("%U: got %U", want, run)
			}
		}
	}
}

// A long run of accents is put in order with checks of the limits, and a
// check that ends the call stops the work. sort.SliceStable took 0.9 s for
// 1 Mi accents, with no check.
func TestNormalizeChecksLongRun(t *testing.T) {
	n := newEmbeddedNormalizer(t)
	s := "a" + strings.Repeat("̖́", 1<<19)
	checks := 0
	var asked int64
	l := &Limits{Check: func() { checks++ }, Room: func(n int64) { asked += n }}
	if got, want := n.normalize(s, NFD, l), n.Normalize(s, NFD); got != want {
		t.Fatal("the run differs with limits")
	}
	if checks < (1<<20)/limitEvery {
		t.Errorf("%d checks for 1 Mi accents", checks)
	}
	if asked == 0 {
		t.Error("no room asked for")
	}
	type stop struct{}
	defer func() {
		if recover() != (stop{}) {
			t.Error("the check did not stop the work")
		}
	}()
	n.normalize(s, NFD, &Limits{Check: func() {
		if checks--; checks == 0 {
			panic(stop{})
		}
	}})
}

func newEmbeddedNormalizer(t *testing.T) *Normalizer {
	t.Helper()
	n, err := NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// divide checks the limits every limitEvery turns of each of its loops, not
// only of the loop over words or code points: a word followed by a long run
// of a mark, or of what is no word, and a character followed by a long run
// that NFKC keeps as one fragment, are one turn of the outer loop.
func TestDivideChecksEveryLoop(t *testing.T) {
	const n = 1 << 20
	for _, c := range []struct{ lang, text string }{
		{"th", "ภาษา" + strings.Repeat("ั", n)},
		{"th", "ภาษา" + strings.Repeat("ๆ", n)},
		{"ja", "中" + strings.Repeat("ﾞ", n)},
	} {
		s, err := NewSegmenter(qjsLocale(c.lang), SegmenterOptions{Granularity: GranularityWord})
		if err != nil {
			t.Fatal(err)
		}
		u := utf16.Encode([]rune(c.text))
		e := s.engines.forChar([]rune(c.text)[0])
		checks := 0
		l := &Limits{Check: func() { checks++ }, Room: func(int64) {}}
		t0 := &utext{text: u}
		var breaks []int
		switch e := e.(type) {
		case *southeastAsianEngine:
			e.divide(t0, 0, len(u), &breaks, l)
		case *cjkEngine:
			e.divide(t0, 0, len(u), &breaks, l)
		case nil:
			if qjsNoCJDict {
				continue
			}
			t.Fatalf("%s: no engine", c.lang)
		default:
			t.Fatalf("%s: engine %T", c.lang, e)
		}
		if checks < n/limitEvery {
			t.Errorf("%.12q: %d checks in divide for %d units, want at least %d", c.text, checks, len(u), n/limitEvery)
		}
	}
}
