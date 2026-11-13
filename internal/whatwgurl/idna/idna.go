// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package idna is the part of golang.org/x/net/idna v0.62.0 (IDNA2008 with
// the compatibility processing of UTS #46) that the URL standard needs: one
// profile, "domain to ASCII", which has Unicode 17's tables and the
// algorithm of UTS #46 revision 35. It was changed for liustack/quickjs:
// the other profiles, the options and ToUnicode are gone, normalization is
// that of internal/gointl, the bidi checks are those of ../bidi, and the
// Punycode encoder is one written here (encode.go), whose work is not the
// square of the length of a label. The caller bounds the length of what it
// passes, and the length of the domain once mapped and normalized
// (ToASCII). SOURCES.md says what changed.
package idna

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	gointl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/whatwgurl/bidi"
)

var (
	normOnce sync.Once
	norm     *gointl.Normalizer
)

func nfc(s string) string {
	normOnce.Do(func() {
		n, err := gointl.NewNormalizer()
		if err != nil {
			panic(err)
		}
		norm = n
	})
	return norm.Normalize(s, gointl.NFC)
}

func isNFC(s string) bool { return isASCII(s) || nfc(s) == s }

// ToASCII is "Unicode ToASCII" of UTS #46 as the URL standard calls it for
// domain to ASCII: with CheckBidi and CheckJoiners on, Transitional_Processing
// off and IgnoreInvalidPunycode off, and with CheckHyphens, UseSTD3ASCIIRules
// and VerifyDnsLength set to strict. Like the original it also returns the
// partial result with the error.
//
// A domain longer than maxUnits UTF-16 units once mapped, or once normalized,
// is an error, found as soon as the mapping makes it longer: a code point can
// map to 18 (Widest), so what follows the mapping, which costs the most for
// each unit, is bounded by maxUnits and not by the length of s.
func ToASCII(s string, strict bool, maxUnits int) (string, error) {
	p := profile{strict: strict, maxUnits: maxUnits}
	return p.process(s)
}

type profile struct {
	strict   bool // checkHyphens, useSTD3Rules, verifyDNSLength
	maxUnits int  // the longest the domain may be once mapped and normalized
}

// errTooLong is the error of a domain longer than maxUnits once mapped or
// normalized.
var errTooLong = labelError{"", "too long"}

type labelError struct{ label, code_ string }

func (e labelError) code() string { return e.code_ }
func (e labelError) Error() string {
	return fmt.Sprintf("idna: invalid label %q", e.label)
}

type runeError struct {
	r     rune
	code_ string
}

func (e runeError) code() string { return e.code_ }
func (e runeError) Error() string {
	return fmt.Sprintf("idna: disallowed rune %U", e.r)
}

// process implements the algorithm described in section 4 of UTS #46
// (revision 35, Unicode 16 and later).
func (p *profile) process(s string) (string, error) {
	var err error
	var isBidi bool
	s, isBidi, err = p.validateAndMap(s)
	if err == nil && p.strict && s == "" {
		err = labelError{s, "A4"}
	}
	labels := labelIter{orig: s}
	for ; !labels.done(); labels.next() {
		label := labels.label()
		if label == "" {
			// Empty labels are not okay. The label iterator skips the last
			// label if it is empty.
			if err == nil && p.strict {
				err = labelError{s, "A4"}
			}
			continue
		}
		if strings.HasPrefix(label, acePrefix) {
			// UTS #46 revision 35, section 4, step 4: a label that starts with xn-- and
			// has a character that is not ASCII is an error, and so is one whose
			// punycode decodes to nothing or to a label that starts with xn-- again
			// (the validity criterion of section 4.1 on the prefix).
			if !isASCII(label) {
				if err == nil {
					err = labelError{label, "P4"}
				}
				continue
			}
			enc := label[len(acePrefix):]
			u, err2 := decode(enc)
			if err2 != nil {
				if err == nil {
					err = err2
				}
				// Spec says keep the old label.
				continue
			}
			if err == nil && (u == "" || strings.HasPrefix(u, acePrefix)) {
				err = punyError(enc)
			}
			if err == nil && len(u) > 0 && isASCII(u) {
				// UTS 46 revision 33 does not classify a xn-- label which
				// contains only ASCII characters as an error, but that is a
				// specification bug and a security issue. Always return an
				// error in this case.
				err = punyError(enc)
			}
			isBidi = isBidi || bidi.IsRTL(u)
			labels.set(u)
			if err == nil {
				err = p.validateFromPunycode(u)
			}
			if err == nil {
				err = p.validateLabel(u)
			}
		} else if err == nil {
			err = p.validateLabel(label)
		}
	}
	if isBidi && err == nil {
		for labels.reset(); !labels.done(); labels.next() {
			if !bidi.ValidString(labels.label()) {
				err = labelError{s, "B"}
				break
			}
		}
	}
	for labels.reset(); !labels.done(); labels.next() {
		label := labels.label()
		if !ascii(label) {
			a, err2 := encode(acePrefix, label)
			if err == nil {
				err = err2
			}
			label = a
			labels.set(a)
		}
		n := len(label)
		if p.strict && err == nil && (n == 0 || n > 63) {
			err = labelError{label, "A4"}
		}
	}
	s = labels.result()
	if p.strict && err == nil {
		if strings.HasSuffix(s, ".") {
			err = labelError{s, "A4"}
		}
		// Compute the length of the domain name minus the root label and its dot.
		n := len(s)
		if n > 0 && s[n-1] == '.' {
			n--
		}
		if len(s) < 1 || n > 253 {
			err = labelError{s, "A4"}
		}
	}
	return s, err
}

func isASCII(s string) bool {
	for _, c := range []byte(s) {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

func (c info) isBidi(s string) bool {
	if !c.isMapped() {
		return c&attributesMask == rtl
	}
	// TODO: also store bidi info for mapped data. This is possible, but a bit
	// cumbersome and not for the common case.
	p, _ := bidi.LookupString(s)
	switch p.Class() {
	case bidi.R, bidi.AL, bidi.AN:
		return true
	}
	return false
}

func (p *profile) validateAndMap(s string) (vm string, isBidi bool, err error) {
	var (
		b []byte
		k int
	)
	// combinedInfoBits contains the or-ed bits of all runes. We use this
	// to derive the mayNeedNorm bit later. This may trigger normalization
	// overeagerly, but it will not do so in the common case.
	var combinedInfoBits info
	n := 0 // the UTF-16 units of the domain mapped so far
	for i := 0; i < len(s); {
		if n > p.maxUnits {
			return "", false, errTooLong
		}
		v, sz := trie.lookupString(s[i:])
		if sz == 0 {
			b = append(b, s[k:i]...)
			b = append(b, "�"...)
			k = len(s)
			if err == nil {
				err = runeError{utf8.RuneError, "P1"}
			}
			break
		}
		combinedInfoBits |= info(v)
		isBidi = isBidi || info(v).isBidi(s[i:])
		start := i
		i += sz
		// Copy bytes not copied so far.
		switch p.simplify(info(v).category()) {
		case valid:
			n += units16(s[start:i])
			continue
		case disallowed:
			// Unicode 16 delays the error until validateLabel.
			n += units16(s[start:i])
			continue
		case deviation:
			// Non-transitional: the deviation characters stay.
			n += units16(s[start:i])
		case mapped:
			b = append(b, s[k:start]...)
			m := len(b)
			b = info(v).appendMapping(b, s[start:i])
			n += units16(string(b[m:]))
		case ignored:
			b = append(b, s[k:start]...)
			// drop the rune
		case unknown:
			b = append(b, s[k:start]...)
			b = append(b, "�"...)
			n++
		}
		k = i
	}
	if n > p.maxUnits {
		return "", false, errTooLong
	}
	if k == 0 {
		// No changes so far.
		if combinedInfoBits&mayNeedNorm != 0 {
			s = nfc(s)
		}
	} else {
		b = append(b, s[k:]...)
		s = string(b)
		if !isASCII(s) {
			s = nfc(s)
		}
	}
	if units16(s) > p.maxUnits {
		return "", false, errTooLong
	}
	return s, isBidi, err
}

// A labelIter allows iterating over domain name labels.
type labelIter struct {
	orig     string
	slice    []string
	curStart int
	curEnd   int
	i        int
}

func (l *labelIter) reset() {
	l.curStart = 0
	l.curEnd = 0
	l.i = 0
}

func (l *labelIter) done() bool {
	return l.curStart >= len(l.orig)
}

func (l *labelIter) result() string {
	if l.slice != nil {
		return strings.Join(l.slice, ".")
	}
	return l.orig
}

func (l *labelIter) label() string {
	if l.slice != nil {
		return l.slice[l.i]
	}
	p := strings.IndexByte(l.orig[l.curStart:], '.')
	l.curEnd = l.curStart + p
	if p == -1 {
		l.curEnd = len(l.orig)
	}
	return l.orig[l.curStart:l.curEnd]
}

// next sets the value to the next label. It skips the last label if it is empty.
func (l *labelIter) next() {
	l.i++
	if l.slice != nil {
		if l.i >= len(l.slice) || l.i == len(l.slice)-1 && l.slice[l.i] == "" {
			l.curStart = len(l.orig)
		}
	} else {
		l.curStart = l.curEnd + 1
		if l.curStart == len(l.orig)-1 && l.orig[l.curStart] == '.' {
			l.curStart = len(l.orig)
		}
	}
}

func (l *labelIter) set(s string) {
	if l.slice == nil {
		l.slice = strings.Split(l.orig, ".")
	}
	l.slice[l.i] = s
}

// acePrefix is the ASCII Compatible Encoding prefix.
const acePrefix = "xn--"

func (p *profile) simplify(cat category) category {
	switch cat {
	case deviation, validNV8, validXV8:
		cat = valid
	}
	return cat
}

func (p *profile) validateFromPunycode(s string) error {
	if !isNFC(s) {
		return labelError{s, "V1"}
	}
	for i := 0; i < len(s); {
		v, sz := trie.lookupString(s[i:])
		if sz == 0 {
			return runeError{utf8.RuneError, "P1"}
		}
		cat := info(v).category()
		if c := p.simplify(cat); c != valid && c != deviation {
			return labelError{s, "V7"}
		}
		i += sz
	}
	return nil
}

const (
	zwnj = "‌"
	zwj  = "‍"
)

type joinState int8

const (
	stateStart joinState = iota
	stateVirama
	stateBefore
	stateBeforeVirama
	stateAfter
	stateFAIL
)

var joinStates = [][numJoinTypes]joinState{
	stateStart: {
		joiningL:   stateBefore,
		joiningD:   stateBefore,
		joinZWNJ:   stateFAIL,
		joinZWJ:    stateFAIL,
		joinVirama: stateVirama,
	},
	stateVirama: {
		joiningL: stateBefore,
		joiningD: stateBefore,
	},
	stateBefore: {
		joiningL:   stateBefore,
		joiningD:   stateBefore,
		joiningT:   stateBefore,
		joinZWNJ:   stateAfter,
		joinZWJ:    stateFAIL,
		joinVirama: stateBeforeVirama,
	},
	stateBeforeVirama: {
		joiningL: stateBefore,
		joiningD: stateBefore,
		joiningT: stateBefore,
	},
	stateAfter: {
		joiningL:   stateFAIL,
		joiningD:   stateBefore,
		joiningT:   stateAfter,
		joiningR:   stateStart,
		joinZWNJ:   stateFAIL,
		joinZWJ:    stateFAIL,
		joinVirama: stateAfter, // no-op as we can't accept joiners here
	},
	stateFAIL: {
		0:          stateFAIL,
		joiningL:   stateFAIL,
		joiningD:   stateFAIL,
		joiningT:   stateFAIL,
		joiningR:   stateFAIL,
		joinZWNJ:   stateFAIL,
		joinZWJ:    stateFAIL,
		joinVirama: stateFAIL,
	},
}

// allowedSTD3 reports whether r is a rune that can appear in a domain name
// according to STD3. We allow all non-ASCII runes and then letters, digits, hyphens.
// We also add dot so that this can be run against the whole name and not just
// a single name element (label). The surrounding code checks dots well enough.
func allowedSTD3(r rune) bool {
	return r >= 0x80 || 'a' <= r && r <= 'z' || '0' <= r && r <= '9' || r == '-' || r == '.'
}

// validateLabel validates the criteria from Section 4.1. Item 1, 4, and 6 are
// already implicitly satisfied by the overall implementation.
func (p *profile) validateLabel(s string) (err error) {
	if s == "" {
		if p.strict {
			return labelError{s, "A4"}
		}
		return nil
	}
	if p.strict {
		if len(s) > 4 && s[2] == '-' && s[3] == '-' {
			return labelError{s, "V2"}
		}
		if s[0] == '-' || s[len(s)-1] == '-' {
			return labelError{s, "V3"}
		}
	}

	// Unicode 16's TR 46 delays the rune validity checks until after the label is decoded.
	// (validateAndMap did not reject them earlier.)
	for i := 0; i < len(s); {
		v, sz := trie.lookupString(s[i:])
		if sz == 0 {
			return runeError{utf8.RuneError, "P1"}
		}
		cat := info(v).category()
		if c := p.simplify(cat); c != valid {
			return labelError{s, "V7"}
		}
		if sz == 1 && p.strict && !allowedSTD3(rune(s[i])) {
			return runeError{rune(s[i]), "U1"}
		}
		i += sz
	}

	// CheckJoiners is on.
	v, sz := trie.lookupString(s)
	x := info(v)
	if x.isModifier() {
		return labelError{s, "V6"}
	}
	// Quickly return in the absence of zero-width (non) joiners.
	if strings.Index(s, zwj) == -1 && strings.Index(s, zwnj) == -1 {
		return nil
	}
	st := stateStart
	for i := 0; ; {
		jt := x.joinType()
		if s[i:i+sz] == zwj {
			jt = joinZWJ
		} else if s[i:i+sz] == zwnj {
			jt = joinZWNJ
		}
		st = joinStates[st][jt]
		if x.isViramaModifier() {
			st = joinStates[st][joinVirama]
		}
		if i += sz; i == len(s) {
			break
		}
		v, sz = trie.lookupString(s[i:])
		x = info(v)
	}
	if st == stateFAIL || st == stateAfter {
		return labelError{s, "C"}
	}

	return nil
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// appendMapping appends the mapping for the respective rune. isMapped must be
// true. A mapping is a categorization of a rune as defined in UTS #46.
func (c info) appendMapping(b []byte, s string) []byte {
	index := int(c >> indexShift)
	if c&xorBit == 0 {
		p := index
		return append(b, mappings[mappingIndex[p]:mappingIndex[p+1]]...)
	}
	b = append(b, s...)
	if c&inlineXOR == inlineXOR {
		// TODO: support and handle two-byte inline masks
		b[len(b)-1] ^= byte(index)
	} else {
		for p := len(b) - int(xorData[index]); p < len(b); p++ {
			index++
			b[p] ^= xorData[index]
		}
	}
	return b
}

// Warm makes what the first normalization builds, which is the same for every
// caller and holds nothing a caller gave: the decompressed tables of
// internal/gointl, 118,786 bytes and what the decompressor takes while it runs.
func Warm() { nfc("é") }
