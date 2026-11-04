// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"
)

// Built with quickjs_intl_nonames, Intl.DisplayNames has no data and says
// so, and the rest is as before.
func TestQJSWithoutNames(t *testing.T) {
	_, err := NewDisplayNames(qjsLocale("zh"), DisplayNamesOptions{Kind: DisplayRegion})
	if !qjsNoNames {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("NewDisplayNames: %v, want ErrNotFound", err)
	}
}

// Built with quickjs_intl_nocjdict, a run of Chinese or Japanese is one
// word, as in an ICU without the dictionary, and Thai still goes by its
// own.
func TestQJSWithoutCJDict(t *testing.T) {
	want := map[string]string{
		"ja": "今日|は|良い|天気|です",
		"zh": "我们|在|北京|工作",
		"th": "สวัสดี|ครับ",
	}
	if qjsNoCJDict {
		want["ja"] = "今日は良い天気です"
		want["zh"] = "我们在北京工作"
	}
	for l, text := range map[string]string{"ja": "今日は良い天気です", "zh": "我们在北京工作", "th": "สวัสดีครับ"} {
		s, err := NewSegmenter(qjsLocale(l), SegmenterOptions{Granularity: GranularityWord})
		if err != nil {
			t.Fatal(err)
		}
		u := utf16.Encode([]rune(text))
		var words []string
		for _, seg := range s.Segment(u).All() {
			words = append(words, string(utf16.Decode(u[seg.Index:seg.End])))
		}
		if got := strings.Join(words, "|"); got != want[l] {
			t.Errorf("%s: %s, want %s", l, got, want[l])
		}
	}
}

// Every break dictionary the packs carry loads: one missing from a pack
// would leave its script without an engine, and dictbe.go takes that for a
// build without the dictionary rather than an error.
func TestQJSBreakDictionaries(t *testing.T) {
	norm, err := NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	e, err := loadBreakEngines(Embedded, norm)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for script := range e.byScript {
		got = append(got, script)
	}
	sort.Strings(got)
	// Chinese and Japanese (Hani, Hira, Kana), Khmer, Lao, Burmese, Thai.
	want := "Hani Hira Kana Khmr Laoo Mymr Thai"
	if qjsNoCJDict {
		want = "Khmr Laoo Mymr Thai"
	}
	if strings.Join(got, " ") != want {
		t.Errorf("engines for %v, want %s", got, want)
	}
}
