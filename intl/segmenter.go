// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"unicode/utf8"

	gointl "github.com/liustack/slowjs/internal/gointl"
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

	loc, err := goResolve(gointl.ServiceSegmenter, req.Locales)
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

// breaks returns 1 if text's segments may be word-like, for the word
// granularity, else 0, then where each of them ends, in UTF-16 code units,
// as twice the offset, plus one for a segment that is word-like.
func (s *segmenter) breaks(text string) []int {
	all := s.s.Segment(utf16Of(text)).All()
	r := make([]int, 1, len(all)+1)
	if s.Granularity == "word" {
		r[0] = 1
	}
	for _, seg := range all {
		n := 2 * seg.End
		if seg.IsWordLike {
			n++
		}
		r = append(r, n)
	}
	return r
}

// utf16Of is the javascript string s, in WTF-8 as Args.WTF8 reads it, in
// UTF-16 again: a lone surrogate, the three bytes of its code point, is that
// code unit.
func utf16Of(s string) []uint16 {
	r := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		if c := s[i]; c < 0x80 {
			r = append(r, uint16(c))
			i++
			continue
		}

		if s[i] == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 {
			r = append(r, 0xD000|uint16(s[i+1]&0x3F)<<6|uint16(s[i+2]&0x3F))
			i += 3
			continue
		}

		c, n := utf8.DecodeRuneInString(s[i:])
		if c >= 0x10000 {
			c -= 0x10000
			r = append(r, 0xD800+uint16(c>>10), 0xDC00+uint16(c&0x3FF))
		} else {
			r = append(r, uint16(c))
		}
		i += n
	}
	return r
}
