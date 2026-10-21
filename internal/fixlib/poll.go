// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// pollCount is the number of interrupt polls of _JS_CallInternal: one at
// the entry and one for each backward jump.
const pollCount = 8

var (
	// A poll as ccgo writes it. The character before it keeps the regexp
	// off ___js_poll_interrupts.
	rePollCall = regexp.MustCompile(`([^\w])_js_poll_interrupts\(tls, (\w+)\) != 0`)
	// A poll as fixPoll leaves it.
	rePollDue = regexp.MustCompile(`pollDue\((\w+)\+(\d+)\) && ___js_poll_interrupts\(tls, (\w+)\) != 0`)
	// The counter js_poll_interrupts counts down.
	rePollCounter = regexp.MustCompile(`\n\tv2 = ctx \+ (\d+)\n\t\*\(\*int32\)\(unsafe\.Pointer\(v2\)\) = \*\(\*int32\)\(unsafe\.Pointer\(v2\)\) - 1\n`)
	pollFuncs     = []string{"_js_poll_interrupts"}
)

// fixPoll makes each interrupt poll of _JS_CallInternal count the
// interrupt counter down in place, through pollDue of lib/poll.go, and call
// the poll itself, ___js_poll_interrupts, only when pollDue says the
// counter ran out:
//
//	_js_poll_interrupts(tls, ctx) != 0
//
// becomes
//
//	pollDue(ctx+OFFSET) && ___js_poll_interrupts(tls, ctx) != 0
//
// js_poll_interrupts is that countdown and that call, but _JS_CallInternal
// is too large for Go to inline a function of its cost into it, so every
// poll was a call. pollDue is small enough to be inlined.
//
// pollDue repeats js_poll_interrupts, and OFFSET is the offset of the
// counter that js_poll_interrupts reads, so the text of js_poll_interrupts
// is pinned in pollSites: when a release changes it, fixPoll stops, and the
// new code must be read against pollDue before the fingerprints are
// updated (go run ./internal/fixlib -poll prints them).
//
// Other changes match _JS_CallInternal as ccgo writes it, so fixPoll comes
// after them, and restorePoll, its exact inverse, comes first.
func fixPoll(b []byte, file string) ([]byte, error) {
	got, err := pollFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := pollSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read it against lib/poll.go, then update pollSites (go run ./internal/fixlib -poll)", file, "js_poll_interrupts", got, want)
	}
	poll, err := funcBody(b, funcHead("_js_poll_interrupts"), "_js_poll_interrupts")
	if err != nil {
		return nil, err
	}
	m := rePollCounter.FindSubmatch(poll)
	if m == nil {
		return nil, fmt.Errorf("_js_poll_interrupts: cannot find the countdown of the counter")
	}
	off := string(m[1])

	body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
	if err != nil {
		return nil, err
	}
	start := bytes.Index(b, body)
	calls, dues := rePollCall.FindAllSubmatch(body, -1), rePollDue.FindAllSubmatch(body, -1)
	for _, d := range dues {
		if string(d[1]) != string(d[3]) || string(d[2]) != off {
			return nil, fmt.Errorf("_JS_CallInternal: %s is not the countdown of fixPoll", d[0])
		}
	}
	switch {
	case len(calls) == pollCount && len(dues) == 0:
	case len(calls) == 0 && len(dues) == pollCount:
		return b, nil
	default:
		return nil, fmt.Errorf("_JS_CallInternal: want %d polls, found %d, and %d countdowns of fixPoll", pollCount, len(calls), len(dues))
	}
	fixed := rePollCall.ReplaceAll(body, []byte("${1}pollDue(${2}+"+off+") && ___js_poll_interrupts(tls, ${2}) != 0"))
	out := append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...)
	back, err := restorePoll(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixPoll: restorePoll does not give the input back")
	}
	return out, nil
}

// restorePoll undoes fixPoll.
func restorePoll(b []byte) ([]byte, error) {
	var err error
	out := rePollDue.ReplaceAllFunc(b, func(m []byte) []byte {
		sub := rePollDue.FindSubmatch(m)
		if string(sub[1]) != string(sub[3]) {
			err = fmt.Errorf("restorePoll: %s is not the countdown of fixPoll", m)
		}
		return []byte("_js_poll_interrupts(tls, " + string(sub[1]) + ") != 0")
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// pollFingerprint hashes pollFuncs, which fixPoll does not change, with
// each C string written out in place of its offset.
func pollFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, pollFuncs, func(text []byte) []byte { return text })
}
