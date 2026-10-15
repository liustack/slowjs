// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"compress/flate"
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// cldrBin is the locale data, generated from CLDR by ./internal/gen. See the
// generator's documentation for the layout. It stays in the binary's read only
// data: a locale is decoded into the Go heap the first time a formatter needs
// it, once per process, and every VM shares the result.
//
//go:embed cldr.bin
var cldrBin string

// cldrIndex is the decoded table of contents of cldrBin.
type cldrIndex struct {
	keys   []string
	keyIdx map[string]uint32
	chunks []cldrChunk
	byName map[string]int
	base   int // offset of the first chunk
}

type cldrChunk struct {
	name      string
	parent    int // -1 for none
	off, size int
	once      sync.Once
	data      *cldrData
}

// cldrData is one decoded chunk: the values of the keys it sets, sorted by
// key index, and the chunk it inherits the others from.
type cldrData struct {
	name   string
	parent *cldrData
	keys   []uint32
	vals   []string
}

var cldr = sync.OnceValue(func() *cldrIndex {
	x, err := parseCLDR(cldrBin)
	if err != nil {
		// The data is generated and embedded, so this is a build problem
		// that every formatter would run into, not something to recover from.
		panic(err)
	}

	return x
})

func parseCLDR(s string) (*cldrIndex, error) {
	if !strings.HasPrefix(s, "QJI1") {
		return nil, fmt.Errorf("intl: cldr.bin: bad magic")
	}

	r := &reader{s: s, i: 4}
	x := &cldrIndex{keyIdx: map[string]uint32{}, byName: map[string]int{}}
	n := r.uvarint()
	x.keys = make([]string, n)
	for i := range x.keys {
		x.keys[i] = r.str()
		x.keyIdx[x.keys[i]] = uint32(i)
	}
	n = r.uvarint()
	x.chunks = make([]cldrChunk, n)
	for i := range x.chunks {
		c := &x.chunks[i]
		c.name = r.str()
		c.parent = r.uvarint() - 1
		c.off = r.uvarint()
		c.size = r.uvarint()
		x.byName[c.name] = i
	}
	if r.err != nil {
		return nil, fmt.Errorf("intl: cldr.bin: %v", r.err)
	}

	x.base = r.i
	return x, nil
}

type reader struct {
	s   string
	i   int
	err error
}

func (r *reader) uvarint() int {
	var v uint64
	for shift := 0; r.err == nil; shift += 7 {
		if r.i >= len(r.s) || shift > 63 {
			r.err = io.ErrUnexpectedEOF
			break
		}

		b := r.s[r.i]
		r.i++
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return int(v)
		}
	}
	return 0
}

func (r *reader) str() string {
	n := r.uvarint()
	if r.err != nil || r.i+n > len(r.s) {
		r.err = io.ErrUnexpectedEOF
		return ""
	}

	s := r.s[r.i : r.i+n]
	r.i += n
	return s
}

// load returns the decoded chunk i, decoding it and its ancestors on first
// use.
func (x *cldrIndex) load(i int) *cldrData {
	c := &x.chunks[i]
	c.once.Do(func() {
		var parent *cldrData
		if c.parent >= 0 {
			parent = x.load(c.parent)
		}
		raw, err := io.ReadAll(flate.NewReader(strings.NewReader(cldrBin[x.base+c.off : x.base+c.off+c.size])))
		if err != nil {
			panic(fmt.Errorf("intl: cldr.bin: %s: %v", c.name, err))
		}

		// One string holds every value of the chunk, so decoding allocates
		// it and the two slices, not a string per value.
		all := string(raw)
		d := &cldrData{name: c.name, parent: parent}
		r := &reader{s: all}
		for r.i < len(all) && r.err == nil {
			d.keys = append(d.keys, uint32(r.uvarint()))
			d.vals = append(d.vals, r.str())
		}
		if r.err != nil {
			panic(fmt.Errorf("intl: cldr.bin: %s: %v", c.name, r.err))
		}

		d.keys = d.keys[:len(d.keys):len(d.keys)]
		d.vals = d.vals[:len(d.vals):len(d.vals)]
		c.data = d
	})
	return c.data
}

// locale returns the data of the cldr-json locale name, or nil if there is
// none.
func (x *cldrIndex) locale(name string) *cldrData {
	i, ok := x.byName[name]
	if !ok || name == "@" {
		return nil
	}

	return x.load(i)
}

// supplemental returns the locale independent data.
func (x *cldrIndex) supplemental() *cldrData { return x.load(x.byName["@"]) }

// noValue is CLDR's marker for a value that is deliberately absent, as
// opposed to inherited.
const noValue = "∅∅∅"

// get returns the value of key in d or the closest ancestor setting it.
func (d *cldrData) get(key string) (string, bool) {
	k, ok := cldr().keyIdx[key]
	if !ok {
		return "", false
	}

	for ; d != nil; d = d.parent {
		i := sort.Search(len(d.keys), func(i int) bool { return d.keys[i] >= k })
		if i < len(d.keys) && d.keys[i] == k {
			if d.vals[i] == noValue {
				return "", false
			}

			return d.vals[i], true
		}
	}
	return "", false
}

// str is get without the ok.
func (d *cldrData) str(key string) string {
	s, _ := d.get(key)
	return s
}

// eachNearestFirst is each in the order ICU reads a table with fallback:
// the keys d sets itself first, then those its parent sets, and so on, each
// in key order.
func (d *cldrData) eachNearestFirst(prefix string, f func(key, value string)) {
	x := cldr()
	lo := sort.SearchStrings(x.keys, prefix)
	hi := lo
	for hi < len(x.keys) && strings.HasPrefix(x.keys[hi], prefix) {
		hi++
	}
	seen := map[uint32]bool{}
	for level := d; level != nil; level = level.parent {
		i := sort.Search(len(level.keys), func(i int) bool { return level.keys[i] >= uint32(lo) })
		for ; i < len(level.keys) && level.keys[i] < uint32(hi); i++ {
			k := level.keys[i]
			if seen[k] {
				continue
			}

			seen[k] = true
			if v, ok := d.get(x.keys[k]); ok {
				f(x.keys[k], v)
			}
		}
	}
}

// each calls f with every key starting with prefix that d or an ancestor sets,
// the value being the one d resolves it to, in key order.
func (d *cldrData) each(prefix string, f func(key, value string)) {
	x := cldr()
	lo := sort.SearchStrings(x.keys, prefix)
	hi := lo
	for hi < len(x.keys) && strings.HasPrefix(x.keys[hi], prefix) {
		hi++
	}
	for k := lo; k < hi; k++ {
		if v, ok := d.get(x.keys[k]); ok {
			f(x.keys[k], v)
		}
	}
}
