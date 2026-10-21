// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// clockFuncs are the functions of the engine that read a clock, and the
// body fixClock gives each, in terms of lib/clock.go. A %s in a body is the
// parameter after tls.
var clockFuncs = []struct{ name, body string }{
	{"_date_now", "\treturn dateNow()\n"},
	{"_js_random_init", "\t(*TJSContext)(unsafe.Pointer(%s)).Frandom_state = randomSeed()\n"},
	{"_get_time_ns", "\treturn monoNanos()\n"},
	{"_get_time_ms", "\treturn monoNanos() / 1000000\n"},
}

const (
	// clockKept is added to the name of a function of clockFuncs, which
	// fixClock keeps, with the other changes of fixlib, so that its text
	// can be checked.
	clockKept = "_libc"
	// clockWait is the one function left that reads a clock of libc.
	clockWait = "_js_atomics_wait"
)

// clockWords are in the name of every function reClockLibc and reClockShim
// take: clockReads looks for them, as a regular expression that does not
// start with a literal reads the files of lib/ too slowly.
var clockWords = []string{"time", "Time", "clock", "Clock", "Counter", "Tick"}

var (
	// A function of libc whose name says it deals with time, used in a
	// call or only by its address.
	reClockLibc = regexp.MustCompile(`^X\w*(?:[Tt]ime|[Cc]lock|Tick|Counter)\w*$`)
	// A function of lib/ or of the generated file that stands for a clock
	// of the C library, as ccgo writes calls of mingw's inline time() or
	// of a shim such as _clock_gettime64 of lib/libquickjs_windows.go.
	// clockReads takes it only when it is called or passed (followed by
	// "(", ")" or ","), so that a comment naming _time.h is not one.
	reClockShim = regexp.MustCompile(`^_{1,3}(?:clock_gettime(?:32|64)?|gettimeofday|_?time(?:32|64)?|_?ftime(?:32|64)?|QueryPerformanceCounter|GetSystemTime\w*|GetTickCount(?:64)?|mach_absolute_time)$`)
	// clockAllowed are the uses of reClockLibc that read no clock:
	// setting the times of a file, the wait of Atomics.wait, which is
	// checked on its own, and the local time functions, which fixTimezone
	// governs (see reLocalTime).
	clockAllowed = regexp.MustCompile(`^libc\.X(?:utimes|pthread_cond_timedwait|localtime(?:_r)?|mktime|strftime|ctime(?:_r)?|tzname|timezone|daylight)$`)
	// The wait of clockWait, which measures its deadline on the clock it
	// reads.
	reClockWait = regexp.MustCompile(`libc\.Xpthread_cond_timedwait\b`)
	// The parameter after tls.
	reClockParam = regexp.MustCompile(`^\nfunc \w+\(tls \*libc\.TLS, (\w+) `)
)

// clockNote is the comment at the top of the function fixClock adds for
// name.
func clockNote(name string) string {
	return "\t// Added by internal/fixlib: the clock is Go's, see lib/clock.go (fixClock). " + name + clockKept + " is the function it replaces, kept for fixlib to check.\n"
}

// fixClock makes the functions of clockFuncs read Go's clocks, through
// lib/clock.go: each gets the body of clockFuncs, and the function it
// replaces stays, renamed with clockKept and with the other changes of
// fixlib, so that its text can be checked.
// The gettimeofday and clock_gettime of modernc.org/libc are system calls,
// while Go reads the clock without entering the kernel where the system
// allows it.
//
// The text of the functions is pinned in clockSites: when a release
// changes one, fixClock stops, and the new code must be read against
// lib/clock.go before the fingerprints are updated (go run ./internal/fixlib
// -clock prints them). A read of a clock of libc anywhere else but in
// clockWait, which hands its deadline to libc, also stops fixClock: the
// engine then reads the time some new way. So does a function of
// clockFuncs that is gone: it must then leave clockFuncs.
//
// fixClock comes after the other changes, and restoreClock, its exact
// inverse, first, so that they see the functions as ccgo writes them.
func fixClock(b []byte, file string) ([]byte, error) {
	bare, err := restoreClock(b)
	if err != nil {
		return nil, err
	}
	got, err := clockFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := clockSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/clock.go, then update clockSites (go run ./internal/fixlib -clock)", file, clockNames(), got, want)
	}
	if err := checkClockCalls(bare); err != nil {
		return nil, err
	}
	if !bytes.Equal(bare, b) {
		out, err := addClock(bare)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(out, b) {
			return nil, fmt.Errorf("fixClock: the file has a part of the change")
		}
		return b, nil
	}
	out, err := addClock(b)
	if err != nil {
		return nil, err
	}
	if back, err := restoreClock(out); err != nil || !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixClock: restoreClock does not give the input back (%v)", err)
	}
	return out, nil
}

// clockNames lists the functions of clockFuncs.
func clockNames() string {
	var names []string
	for _, f := range clockFuncs {
		names = append(names, f.name)
	}
	return strings.Join(names, ", ")
}

// clockBlock returns the function fixClock adds for f, whose header as
// ccgo writes it is head, from the newline before it to the newline after
// its closing brace.
func clockBlock(name, body, head string) (string, error) {
	if strings.Contains(body, "%s") {
		m := reClockParam.FindStringSubmatch(head)
		if m == nil {
			return "", fmt.Errorf("%s: no parameter after tls", name)
		}
		body = fmt.Sprintf(body, m[1])
	}
	return head + clockNote(name) + body + "}\n", nil
}

// addClock makes the change in b, which has none of it.
func addClock(b []byte) ([]byte, error) {
	for _, f := range clockFuncs {
		loc := findAll(funcHead(f.name), b)
		if len(loc) != 1 {
			return nil, fmt.Errorf("want 1 function %s, found %d", f.name, len(loc))
		}
		head := string(b[loc[0][0]:loc[0][1]])
		block, err := clockBlock(f.name, f.body, head)
		if err != nil {
			return nil, err
		}
		kept := strings.Replace(head, "func "+f.name+"(", "func "+f.name+clockKept+"(", 1)
		b = append(append(append(bytes.Clone(b[:loc[0][0]]), block...), kept...), b[loc[0][1]:]...)
	}
	return b, nil
}

// restoreClock undoes fixClock.
func restoreClock(b []byte) ([]byte, error) {
	for _, f := range clockFuncs {
		loc := findAll(funcHead(f.name+clockKept), b)
		if len(loc) == 0 {
			continue
		}
		if len(loc) != 1 {
			return nil, fmt.Errorf("restoreClock: %d functions %s%s", len(loc), f.name, clockKept)
		}
		kept := string(b[loc[0][0]:loc[0][1]])
		head := strings.Replace(kept, "func "+f.name+clockKept+"(", "func "+f.name+"(", 1)
		block, err := clockBlock(f.name, f.body, head)
		if err != nil {
			return nil, err
		}
		start := loc[0][0] - len(block)
		if start < 0 || string(b[start:loc[0][0]]) != block {
			return nil, fmt.Errorf("restoreClock: %s%s does not follow the function fixClock adds", f.name, clockKept)
		}
		b = append(append(bytes.Clone(b[:start]), head...), b[loc[0][1]:]...)
	}
	if n := bytes.Count(b, []byte("the clock is Go's, see lib/clock.go (fixClock)")); n != 0 {
		return nil, fmt.Errorf("restoreClock: %d functions of fixClock left", n)
	}
	return b, nil
}

// clockReads returns, in order, the uses of reClockLibc and reClockShim in
// text that may read a clock, without those of clockAllowed.
func clockReads(text []byte) []string {
	word := func(c byte) bool {
		return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
	}
	names := map[int]string{}
	for _, w := range clockWords {
		for i := 0; ; {
			j := bytes.Index(text[i:], []byte(w))
			if j < 0 {
				break
			}
			start, end := i+j, i+j+len(w)
			for start > 0 && word(text[start-1]) {
				start--
			}
			for end < len(text) && word(text[end]) {
				end++
			}
			i = end
			id := string(text[start:end])
			switch {
			case bytes.HasSuffix(text[:start], []byte("libc.")) && reClockLibc.MatchString(id):
				if name := "libc." + id; !clockAllowed.MatchString(name) {
					names[start] = name
				}
			case (start == 0 || text[start-1] != '.') && end < len(text) && strings.IndexByte("(),", text[end]) >= 0 && reClockShim.MatchString(id):
				names[start] = id
			}
		}
	}
	at := make([]int, 0, len(names))
	for k := range names {
		at = append(at, k)
	}
	sort.Ints(at)
	reads := make([]string, len(at))
	for k, a := range at {
		reads[k] = names[a]
	}
	return reads
}

// checkClockCalls checks that, in b without the change, each function of
// clockFuncs reads a clock of libc once, that clockWait reads one once and
// waits once, and that nothing else reads one.
func checkClockCalls(b []byte) error {
	inFuncs := 0
	for _, name := range append(strings.Split(clockNames(), ", "), clockWait) {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return err
		}
		if reads := clockReads(body); len(reads) != 1 {
			return fmt.Errorf("%s: %d reads of a clock of libc (%s), want 1", name, len(reads), strings.Join(reads, ", "))
		}
		inFuncs++
	}
	wait, _ := funcBody(b, funcHead(clockWait), clockWait)
	if n, all := len(reClockWait.FindAll(wait, -1)), len(reClockWait.FindAll(b, -1)); n != 1 || all != 1 {
		return fmt.Errorf("%d uses of pthread_cond_timedwait, %d of them in %s, want 1 there and none elsewhere: read the new one against fixClock", all, n, clockWait)
	}
	if reads := clockReads(b); len(reads) != inFuncs {
		return fmt.Errorf("%d reads of a clock of libc, %d of them outside %s and %s: the engine reads the time some new way", len(reads), len(reads)-inFuncs, clockNames(), clockWait)
	}
	return nil
}

// checkClockFuncs checks that each function of clockFuncs is in b once,
// renamed or not.
func checkClockFuncs(b []byte) error {
	for _, f := range clockFuncs {
		n, kept := len(findAll(funcHead(f.name), b)), len(findAll(funcHead(f.name+clockKept), b))
		if n == 0 && kept == 0 {
			return fmt.Errorf("%s is gone: upstream removed it, so it must leave clockFuncs in internal/fixlib/clock.go, then update clockSites (go run ./internal/fixlib -clock) and docs/fork-changes/time-and-intl.md", f.name)
		}
		if n != 1 || kept > 1 {
			return fmt.Errorf("want 1 function %s, found %d, and at most 1 %s%s, found %d", f.name, n, f.name, clockKept, kept)
		}
	}
	return nil
}

// clockFingerprint hashes clockFuncs as the changes before fixClock leave
// them, with each C string written out in place of its offset.
func clockFingerprint(b []byte) (string, error) {
	bare, err := restoreClock(b)
	if err != nil {
		return "", err
	}
	if err := checkClockFuncs(bare); err != nil {
		return "", err
	}
	return funcsFingerprint(bare, strings.Split(clockNames(), ", "), func(text []byte) []byte { return text })
}
