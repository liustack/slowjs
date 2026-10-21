package intl

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

// Intl.Segmenter, as V8 builds it on ICU's break iterators: ICU's rules,
// compiled, and its dictionaries for the scripts written without spaces
// (see brkdata.go, breaker.go and dictbe.go, and internal/segmentgen).

// Granularity is what a Segmenter breaks text into.
type Granularity int

const (
	// GranularityGrapheme is user-perceived characters, the default.
	GranularityGrapheme Granularity = iota
	GranularityWord
	GranularitySentence
)

// SegmenterOptions are Intl.Segmenter's options.
type SegmenterOptions struct {
	Granularity Granularity
}

// A Segmenter breaks text in one locale. It never changes after it is
// built and is safe for any number of goroutines to share.
type Segmenter struct {
	locale      Locale
	granularity Granularity
	rules       *breakRules
	// engines are the dictionary engines, for rules that hand characters
	// to them.
	engines *breakEngines
}

// ResolvedSegmenter is what a segmenter settled on, mirroring
// Intl.Segmenter.prototype.resolvedOptions.
type ResolvedSegmenter struct {
	Locale      string
	Granularity Granularity
}

// ResolvedOptions returns what the segmenter settled on: its locale, which
// keeps no Unicode extension keyword since a segmenter reads none, and its
// granularity.
func (s *Segmenter) ResolvedOptions() ResolvedSegmenter {
	return ResolvedSegmenter{Locale: s.locale.onlyKeywords().String(), Granularity: s.granularity}
}

// NewSegmenter builds a Segmenter from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewSegmenter(loc Locale, opts SegmenterOptions) (*Segmenter, error) {
	return NewSegmenterFrom(Embedded, loc, opts)
}

// NewSegmenterFrom builds a Segmenter from a source of the caller's own.
func NewSegmenterFrom(src Source, loc Locale, opts SegmenterOptions) (*Segmenter, error) {
	name, err := breakRulesName(src, loc, opts.Granularity)
	if err != nil {
		return nil, err
	}
	b, err := src.Open(Marker("brkitr/"+name), DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the break rules %s: %w", name, err)
	}
	rules, err := decodeBreakRules(b)
	if err != nil {
		return nil, fmt.Errorf("intl: the break rules %s: %w", name, err)
	}
	s := &Segmenter{locale: loc, granularity: opts.Granularity, rules: rules}
	if rules.forward.dictStart < rules.catCount {
		norm, err := NewNormalizerFrom(src)
		if err != nil {
			return nil, err
		}
		if s.engines, err = loadBreakEngines(src, norm); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// breakRulesName is the rules ICU's BreakIterator::makeInstance opens for
// a locale: those the first bundle of the brkitr tree along its fallback
// that names any for the granularity.
func breakRulesName(src Source, loc Locale, g Granularity) (string, error) {
	b, err := src.Open(Marker("brkitr/boundaries"), DataLocale{})
	if err != nil {
		return "", fmt.Errorf("intl: the break rules: %w", err)
	}
	named := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) == 4 && f[0] == "boundaries" && f[2] == g.String() {
			named[f[1]] = strings.TrimSuffix(f[3], ".brk")
		}
	}
	var names []string
	// "-u-va-posix" is ICU's variant POSIX, which the data locale holds:
	// en_US_POSIX, which falls back to en_US.
	chain := loc.Data().Fallback()
	if fb, err := NewFallbacker(src); err == nil {
		chain = fb.ChainIn(treeBrkitr, loc.Data())
	}
	for _, d := range chain {
		names = append(names, icuName(d))
	}
	for _, n := range names {
		if rules, ok := named[n]; ok {
			return rules, nil
		}
	}
	if rules, ok := named["root"]; ok {
		return rules, nil
	}
	return "", fmt.Errorf("intl: no %s break rules", g)
}

// A Segment is a piece of a text: its offsets in UTF-16 code units, as
// JavaScript counts them, and, for words, whether it is a word rather than
// spaces or punctuation.
type Segment struct {
	Index, End int
	IsWordLike bool
}

// Segments are a text broken up, Intl.Segmenter's segments object.
type Segments struct {
	text        []uint16
	granularity Granularity
	rules       *breakRules
	bounds      []boundary
}

// Segment breaks a text given in UTF-16, as JavaScript strings are.
func (s *Segmenter) Segment(text []uint16) *Segments {
	b := breaker{rules: s.rules, engines: s.engines, t: utext{text: text}}
	return &Segments{text: text, granularity: s.granularity, rules: s.rules, bounds: b.boundaries()}
}

// SegmentString breaks a Go string; the offsets are still UTF-16's.
func (s *Segmenter) SegmentString(text string) *Segments {
	return s.Segment(utf16.Encode([]rune(text)))
}

// wordLike is V8's CurrentSegmentIsWordLike: a rule status among ICU's
// numbers, letters, kana and ideographs.
func (s *Segments) wordLike(b boundary) bool {
	if s.granularity != GranularityWord {
		return false
	}
	st := s.rules.ruleStatus(b.status)
	return st >= 100 && st < 500
}

// All are the segments, in order.
func (s *Segments) All() []Segment {
	out := make([]Segment, 0, len(s.bounds))
	for i := 1; i < len(s.bounds); i++ {
		out = append(out, Segment{s.bounds[i-1].pos, s.bounds[i].pos, s.wordLike(s.bounds[i])})
	}
	return out
}

// Containing is %Segments.prototype%.containing: the segment an offset
// falls in, V8 moving an offset inside a surrogate pair to the pair's
// start before asking ICU. It is false for an offset outside the text.
func (s *Segments) Containing(n int) (Segment, bool) {
	if len(s.bounds) < 2 || n < 0 || n >= len(s.text) {
		return Segment{}, false
	}
	if n > 0 && isTrail(s.text[n]) && isLead(s.text[n-1]) {
		n--
	}
	// The first boundary past n ends the segment, and the one before it
	// begins it.
	i := sort.Search(len(s.bounds), func(i int) bool { return s.bounds[i].pos > n })
	start, end := s.bounds[i-1], s.bounds[i]
	return Segment{start.pos, end.pos, s.wordLike(end)}, true
}
