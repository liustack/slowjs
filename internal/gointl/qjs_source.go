// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
	for _, p := range []qjsBuiltIn{{"core", qjsCore}, {"names", qjsNames}, {"cjdict", qjsCJDict}} {
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

var qjsPacks = sync.OnceValues(func() ([]*qjspack.Pack, error) { return qjsOpenPacks(qjsPackData()) })

// qjsOpen serves Embedded.
func qjsOpen(m Marker, d DataLocale) ([]byte, error) {
	packs, err := qjsPacks()
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
