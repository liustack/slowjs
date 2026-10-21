// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"github.com/liustack/quickjs/internal/gointl/internal/qjspack"
)

// The data quickjs builds in, in place of go-intl's data.pack: the packs
// internal/qjspack/gen writes into qjsdata/ from go-intl's data directory,
// cut down to Chromium's locales. A pack is read where it lies, and a
// chunk of it is inflated into the heap the first time a formatter reads one
// of its files, once for the process.
//
// The bytes served are never to be written, as Source says: a chunk kept
// uncompressed is the binary's read-only data, where a write faults, and an
// inflated one is shared by every formatter and VM in the process. Glue that
// hands bytes from here to JS copies them first.

// A qjsBuiltIn is one pack built in.
type qjsBuiltIn struct{ name, data string }

// qjsPackData returns the packs built in, leaving out those a build tag
// leaves out.
func qjsPackData() []qjsBuiltIn {
	var out []qjsBuiltIn
	for _, p := range []qjsBuiltIn{{"core", qjsCore}, {"names", qjsNames}, {"cjdict", qjsCJDict}, {"tz", qjsTZ}} {
		if p.data != "" {
			out = append(out, p)
		}
	}
	return out
}

func qjsOpenPacks(built []qjsBuiltIn) ([]*qjspack.Pack, error) {
	var packs []*qjspack.Pack
	for _, b := range built {
		p, err := qjspack.Open(b.data)
		if err != nil {
			return nil, fmt.Errorf("intl: qjsdata/%s.bin: %w", b.name, err)
		}
		packs = append(packs, p)
	}
	return packs, nil
}

// qjsLazy opens packs the first time they are asked for, with a
// sync.OnceValues made then, not when the package starts, so that the linker
// keeps a pack only in a binary whose code can read it: one that uses only
// EmbeddedTemporalSource links tz.bin and none of the others. As with any
// sync.OnceValues, a panic of open comes back on every call.
type qjsLazy struct {
	once sync.Once
	open func() ([]*qjspack.Pack, error)
}

func (l *qjsLazy) get(open func() ([]*qjspack.Pack, error)) ([]*qjspack.Pack, error) {
	l.once.Do(func() { l.open = sync.OnceValues(open) })
	return l.open()
}

var qjsAll, qjsTemporal qjsLazy

// qjsTemporalPacks is the tz pack alone, which EmbeddedTemporalSource serves.
func qjsTemporalPacks() ([]*qjspack.Pack, error) {
	return qjsTemporal.get(func() ([]*qjspack.Pack, error) {
		return qjsOpenPacks([]qjsBuiltIn{{"tz", qjsTZ}})
	})
}

// qjsPacks is every pack built in, which Embedded serves. The tz pack in it
// is EmbeddedTemporalSource's, so that a chunk both read is inflated once.
func qjsPacks() ([]*qjspack.Pack, error) {
	return qjsAll.get(func() ([]*qjspack.Pack, error) {
		var packs []*qjspack.Pack
		for _, b := range qjsPackData() {
			open := func() ([]*qjspack.Pack, error) { return qjsOpenPacks([]qjsBuiltIn{b}) }
			if b.name == "tz" {
				open = qjsTemporalPacks
			}
			p, err := open()
			if err != nil {
				return nil, err
			}
			packs = append(packs, p...)
		}
		return packs, nil
	})
}

// qjsOpen serves Embedded.
func qjsOpen(m Marker, d DataLocale) ([]byte, error) {
	packs, err := qjsPacks()
	if err != nil {
		return nil, err
	}
	return openFiles(qjsFiles{packs}, m, d)
}

// EmbeddedTemporalSource is the part of Embedded that the temporal package
// reads: ICU's time zones (MarkerTimeZones), the names Temporal takes
// (MarkerTemporalZones) and the calendars' tables (MarkerTemporalCalendars).
// It serves the same bytes as Embedded, and a binary whose code reads only
// it links 91 KB of data, not Embedded's 11 MB. Any other data set is
// ErrNotFound. Its one value is its zero value, and there is no variable of
// it, as go-intl has no exported variable an importer could change.
type EmbeddedTemporalSource struct{}

// Open serves the embedded time zones and calendar tables.
func (EmbeddedTemporalSource) Open(m Marker, d DataLocale) ([]byte, error) {
	packs, err := qjsTemporalPacks()
	if err != nil {
		return nil, err
	}
	return openFiles(qjsFiles{packs}, m, d)
}

// QJSInflated reports how many chunks of the built-in data have been
// inflated so far and how many bytes of heap they hold, which stay for the
// life of the process.
func QJSInflated() (chunks, bytes int) {
	packs, err := qjsPacks()
	if err != nil {
		return 0, 0
	}
	for _, p := range packs {
		n, size := p.Inflated()
		chunks += n
		bytes += size
	}
	return chunks, bytes
}

// qjsFiles finds a data file in packs.
type qjsFiles struct{ packs []*qjspack.Pack }

func (f qjsFiles) readFile(name string) ([]byte, error) {
	for _, p := range f.packs {
		b, err := p.File(name)
		if !errors.Is(err, fs.ErrNotExist) {
			return b, err
		}
	}
	return nil, fs.ErrNotExist
}

// qjsSource serves packs of its own, for tests that watch what is inflated.
type qjsSource struct{ files qjsFiles }

func (s qjsSource) Open(m Marker, d DataLocale) ([]byte, error) { return openFiles(s.files, m, d) }
