// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bidi has the Bidi class of a rune and implements the Bidi Rule defined by RFC 5893.
//
// This package is under development. The API may change without notice and
// without preserving backward compatibility.
package bidi

import (
	"errors"
	"sync"
	"unicode/utf8"
)

// This file contains an implementation of RFC 5893: Right-to-Left Scripts for
// Internationalized Domain Names for Applications (IDNA)
//
// A label is an individual component of a domain name.  Labels are usually
// shown separated by dots; for example, the domain name "www.example.com" is
// composed of three labels: "www", "example", and "com".
//
// An RTL label is a label that contains at least one character of class R, AL,
// or AN. An LTR label is any label that is not an RTL label.
//
// A "Bidi domain name" is a domain name that contains at least one RTL label.
//
//  The following guarantees can be made based on the above:
//
//  o  In a domain name consisting of only labels that satisfy the rule,
//     the requirements of Section 3 are satisfied.  Note that even LTR
//     labels and pure ASCII labels have to be tested.
//
//  o  In a domain name consisting of only LDH labels (as defined in the
//     Definitions document [RFC5890]) and labels that satisfy the rule,
//     the requirements of Section 3 are satisfied as long as a label
//     that starts with an ASCII digit does not come after a
//     right-to-left label.
//
//  No guarantee is given for other combinations.

// ErrInvalid indicates a label is invalid according to the Bidi Rule.
var ErrInvalid = errors.New("bidirule: failed Bidi Rule")

type ruleState uint8

const (
	ruleInitial ruleState = iota
	ruleLTR
	ruleLTRFinal
	ruleRTL
	ruleRTLFinal
	ruleInvalid
)

type ruleTransition struct {
	next ruleState
	mask uint16
}

var transitions = [...][2]ruleTransition{
	// [2.1] The first character must be a character with Bidi property L, R, or
	// AL. If it has the R or AL property, it is an RTL label; if it has the L
	// property, it is an LTR label.
	ruleInitial: {
		{ruleLTRFinal, 1 << L},
		{ruleRTLFinal, 1<<R | 1<<AL},
	},
	ruleRTL: {
		// [2.3] In an RTL label, the end of the label must be a character with
		// Bidi property R, AL, EN, or AN, followed by zero or more characters
		// with Bidi property NSM.
		{ruleRTLFinal, 1<<R | 1<<AL | 1<<EN | 1<<AN},

		// [2.2] In an RTL label, only characters with the Bidi properties R,
		// AL, AN, EN, ES, CS, ET, ON, BN, or NSM are allowed.
		// We exclude the entries from [2.3]
		{ruleRTL, 1<<ES | 1<<CS | 1<<ET | 1<<ON | 1<<BN | 1<<NSM},
	},
	ruleRTLFinal: {
		// [2.3] In an RTL label, the end of the label must be a character with
		// Bidi property R, AL, EN, or AN, followed by zero or more characters
		// with Bidi property NSM.
		{ruleRTLFinal, 1<<R | 1<<AL | 1<<EN | 1<<AN | 1<<NSM},

		// [2.2] In an RTL label, only characters with the Bidi properties R,
		// AL, AN, EN, ES, CS, ET, ON, BN, or NSM are allowed.
		// We exclude the entries from [2.3] and NSM.
		{ruleRTL, 1<<ES | 1<<CS | 1<<ET | 1<<ON | 1<<BN},
	},
	ruleLTR: {
		// [2.6] In an LTR label, the end of the label must be a character with
		// Bidi property L or EN, followed by zero or more characters with Bidi
		// property NSM.
		{ruleLTRFinal, 1<<L | 1<<EN},

		// [2.5] In an LTR label, only characters with the Bidi properties L,
		// EN, ES, CS, ET, ON, BN, or NSM are allowed.
		// We exclude the entries from [2.6].
		{ruleLTR, 1<<ES | 1<<CS | 1<<ET | 1<<ON | 1<<BN | 1<<NSM},
	},
	ruleLTRFinal: {
		// [2.6] In an LTR label, the end of the label must be a character with
		// Bidi property L or EN, followed by zero or more characters with Bidi
		// property NSM.
		{ruleLTRFinal, 1<<L | 1<<EN | 1<<NSM},

		// [2.5] In an LTR label, only characters with the Bidi properties L,
		// EN, ES, CS, ET, ON, BN, or NSM are allowed.
		// We exclude the entries from [2.6].
		{ruleLTR, 1<<ES | 1<<CS | 1<<ET | 1<<ON | 1<<BN},
	},
	ruleInvalid: {
		{ruleInvalid, 0},
		{ruleInvalid, 0},
	},
}

// [2.4] In an RTL label, if an EN is present, no AN may be present, and
// vice versa.
const exclusiveRTL = uint16(1<<EN | 1<<AN)

// From RFC 5893
// An RTL label is a label that contains at least one character of type
// R, AL, or AN.
//
// An LTR label is any label that is not an RTL label.

// IsRTL reports whether the label has a character of class R, AL or AN, which
// makes it an RTL label by RFC 5893. The Bidi Rule need not be applied to a
// label that is not one.
func IsRTL(s string) bool {
	for i := 0; i < len(s); {
		e, sz := LookupString(s[i:])
		if sz == 0 {
			i++
			continue
		}
		c := e.Class()
		if c == R || c == AL || c == AN {
			return true
		}
		i += sz
	}
	return false
}

// ValidString reports whether s conforms to the BiDi rule.
func ValidString(s string) bool {
	var t Transformer
	if n, ok := t.advanceString(s); !ok || n < len(s) {
		return false
	}
	return t.isFinal()
}

// Transformer is the state of the check of the Bidi Rule.
type Transformer struct {
	state  ruleState
	hasRTL bool
	seen   uint16
}

// A rule can only be violated for "Bidi Domain names", meaning if one of the
// following categories has been observed.
func (t *Transformer) isRTL() bool {
	const isRTL = 1<<R | 1<<AL | 1<<AN
	return t.seen&isRTL != 0
}

// Precomputing the ASCII values decreases running time for the ASCII fast path
// by about 30%.
var asciiTable [128]Properties
var asciiTableOnce sync.Once

func initASCIITable() {
	for i := range asciiTable {
		p, _ := LookupRune(rune(i))
		asciiTable[i] = p
	}
}

func (t *Transformer) advanceString(s string) (n int, ok bool) {
	asciiTableOnce.Do(initASCIITable)

	var e Properties
	var sz int
	for n < len(s) {
		if s[n] < utf8.RuneSelf {
			e, sz = asciiTable[s[n]], 1
		} else {
			e, sz = LookupString(s[n:])
			if sz <= 1 {
				if sz == 1 {
					return n, false // invalid UTF-8
				}
				return n, true // incomplete UTF-8 encoding
			}
		}
		// TODO: using CompactClass results in noticeable speedup.
		// See unicode/bidi/prop.go:Properties.CompactClass.
		c := uint16(1 << e.Class())
		t.seen |= c
		if t.seen&exclusiveRTL == exclusiveRTL {
			t.state = ruleInvalid
			return n, false
		}
		switch tr := transitions[t.state]; {
		case tr[0].mask&c != 0:
			t.state = tr[0].next
		case tr[1].mask&c != 0:
			t.state = tr[1].next
		default:
			t.state = ruleInvalid
			if t.isRTL() {
				return n, false
			}
		}
		n += sz
	}
	return n, true
}

func (t *Transformer) isFinal() bool {
	return t.state == ruleLTRFinal || t.state == ruleRTLFinal || t.state == ruleInitial
}
