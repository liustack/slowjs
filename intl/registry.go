// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
)

// A formatter is a resolved Intl object, such as a *numberFormat: built once
// from a constructor's locales and options, and read only from then on, so
// that every VM of the process shares it.
type formatter interface {
	// init compiles what formatting needs from the resolved state.
	init() error
}

// entry is a formatter in the registry. id is the handle javascript keeps in
// the object's internal slot (see slot in intl.js).
type entry struct {
	id       uint64
	f        formatter
	options  string // the resolvedOptions() result as json
	resolved string // the resolved state as json, internal fields included
}

const (
	// registrySlots bounds the formatters the process keeps. A page asking
	// for more distinct ones than that, say with options made up in a loop,
	// makes the registry drop the oldest, and an object still holding one
	// resolves it again on its next call.
	registrySlots = 1024
	// registryRequests bounds the requests remembered, the spellings of
	// locales and options that lead to a formatter.
	registryRequests = 4 * registrySlots
	// maxRequestKey bounds the length of a request key the registry
	// remembers. A longer one, such as that of a list of thousands of
	// locales made of an Accept-Language header, is resolved again each
	// time: remembering 4096 of them could take gigabytes.
	maxRequestKey = 512
)

// registry holds the formatters of the process, found by the request that
// made them, by what they resolved to and by their id. The lookups by id, one
// per format call, take no lock.
type registry struct {
	mu         sync.RWMutex
	byRequest  map[string]request2entry
	requests   [registryRequests]string // a ring, the oldest request is dropped first
	nextReq    int
	byResolved map[string]*entry
	slots      [registrySlots]atomic.Pointer[entry]
	made       uint64 // entries made so far, the last one's id

	// dropping makes entry answer nil that many times, as for an entry
	// the registry dropped, for the tests.
	dropping atomic.Int32
}

type request2entry struct {
	e   *entry
	pos int // where the request is in requests
}

var formatters registry

// entry returns the entry javascript holds the id of, or nil when the
// registry has since dropped it.
func (r *registry) entry(id float64) *entry {
	n := uint64(id)
	if float64(n) != id {
		return nil
	}

	if r.dropping.Load() > 0 && r.dropping.Add(-1) >= 0 {
		return nil
	}

	if e := r.slots[n%registrySlots].Load(); e != nil && e.id == n {
		return e
	}

	return nil
}

// live reports whether e is still in its slot.
func (r *registry) live(e *entry) bool { return r.slots[e.id%registrySlots].Load() == e }

// lookup returns the formatter key, a request, made, if the registry still has
// it.
func (r *registry) lookup(key []byte) *entry {
	if len(key) > maxRequestKey {
		return nil
	}

	r.mu.RLock()
	re, ok := r.byRequest[string(key)]
	r.mu.RUnlock()
	if ok && r.live(re.e) {
		return re.e
	}

	return nil
}

// add files f, made for the request key, and returns its entry: that of an
// equal formatter already there, which another request or another VM made,
// when there is one.
func (r *registry) add(key []byte, f formatter) (*entry, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}

	// Formatters of two constructors never share an entry.
	resolved := string(key[:1]) + string(b)
	r.mu.RLock()
	e := r.byResolved[resolved]
	r.mu.RUnlock()
	if e == nil || !r.live(e) {
		// Compile outside the lock: it reads locale data, which can take a
		// millisecond the first time in a process.
		if err := f.init(); err != nil {
			return nil, err
		}

		e = &entry{f: f, options: publicOptions(b), resolved: resolved}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if old := r.byResolved[resolved]; old != nil && r.live(old) {
		e = old
	} else {
		e = r.insert(e)
	}
	if len(key) <= maxRequestKey {
		r.alias(string(key), e)
	}
	return e, nil
}

// insert gives e an id and a slot, dropping the slot's previous entry, and
// returns it. An entry the registry dropped before keeps its old id, which
// stays dropped, and a copy goes in. r.mu is held.
func (r *registry) insert(e *entry) *entry {
	if e.id != 0 {
		e = &entry{f: e.f, options: e.options, resolved: e.resolved}
	}
	r.made++
	e.id = r.made
	slot := &r.slots[e.id%registrySlots]
	if old := slot.Load(); old != nil && r.byResolved[old.resolved] == old {
		delete(r.byResolved, old.resolved)
	}
	if r.byResolved == nil {
		r.byResolved = map[string]*entry{}
	}
	r.byResolved[e.resolved] = e
	slot.Store(e)
	return e
}

// alias makes the request key find e. r.mu is held.
func (r *registry) alias(key string, e *entry) {
	if r.byRequest == nil {
		r.byRequest = map[string]request2entry{}
	}
	if re, ok := r.byRequest[key]; ok {
		r.byRequest[key] = request2entry{e, re.pos}
		return
	}

	pos := r.nextReq % registryRequests
	r.nextReq++
	if old := r.requests[pos]; old != "" {
		if re, ok := r.byRequest[old]; ok && re.pos == pos {
			delete(r.byRequest, old)
		}
	}
	r.requests[pos] = key
	r.byRequest[key] = request2entry{e, pos}
}

// count returns the entries in the registry and the requests it remembers,
// for the tests.
func (r *registry) count() (entries, requests int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.byResolved), len(r.byRequest)
}

// longestRequest returns the length of the longest request key the registry
// remembers, for the tests.
func (r *registry) longestRequest() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := 0
	for k := range r.byRequest {
		n = max(n, len(k))
	}
	return n
}

// madeCount returns the entries made so far, for the tests.
func (r *registry) madeCount() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.made
}

// publicOptions returns the resolved json state b without the fields only Go
// reads, those whose names start with an underscore, as resolvedOptions()
// reports it: in the same order.
func publicOptions(b []byte) string {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil { // {
		return "{}"
	}

	var out strings.Builder
	out.WriteByte('{')
	first := true
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			break
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			break
		}

		k, _ := t.(string)
		if strings.HasPrefix(k, "_") {
			continue
		}

		if !first {
			out.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		out.Write(kb)
		out.WriteByte(':')
		out.Write(raw)
	}
	out.WriteByte('}')
	return out.String()
}
