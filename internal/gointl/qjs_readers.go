// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"

	"github.com/liustack/slowjs/internal/gointl/internal/qjspack"
)

// The packs only some loaders read. Each is referenced from one function,
// which those loaders call before they open anything (qjs.patch adds the
// calls), so the linker keeps a pack only in a binary that links one of its
// readers: a program whose DateTimeFormat is not go-intl's leaves out the
// 2.8 MB of dates and zone names. Embedded serves a pack from the first
// call of its function on. TestQJSReadersUsePacks checks that every
// function that opens a data set of one of these packs calls it.
//
//	dates.bin      dates/, datesshared.bin, patterncalendars.bin, rbnf.bin,
//	               ummalqura.bin, japaneseeras.bin: DateTimeFormat
//	zonenames.bin  zonenames/, zonenamesshared.bin, metazones.bin: the zone
//	               names (loadZoneNames)
//	hostzones.bin  windowszones.bin, abbreviationzones.bin: the guess at the
//	               host's zone

//go:embed qjsdata/dates.bin
var qjsDates string

//go:embed qjsdata/zonenames.bin
var qjsZoneNames string

//go:embed qjsdata/hostzones.bin
var qjsHostZones string

var qjsDatesPack, qjsZoneNamesPack, qjsHostZonesPack qjsReader

// qjsUseDates makes Embedded serve dates.bin. DateTimeFormat's loaders call it.
func qjsUseDates() {
	qjsDatesPack.use(func() (*qjspack.Pack, error) { return qjsOpenReader("dates", qjsDates) })
}

// qjsUseZoneNames makes Embedded serve zonenames.bin. loadZoneNames calls it.
func qjsUseZoneNames() {
	qjsZoneNamesPack.use(func() (*qjspack.Pack, error) { return qjsOpenReader("zonenames", qjsZoneNames) })
}

// qjsUseHostZones makes Embedded serve hostzones.bin. windowsZone and
// guessHostZone call it.
func qjsUseHostZones() {
	qjsHostZonesPack.use(func() (*qjspack.Pack, error) { return qjsOpenReader("hostzones", qjsHostZones) })
}

func qjsOpenReader(name, data string) (*qjspack.Pack, error) {
	p, err := qjspack.Open(data)
	if err != nil {
		return nil, fmt.Errorf("intl: qjsdata/%s.bin: %w", name, err)
	}
	return p, nil
}

// A qjsReader is one of those packs, opened the first time a file is read
// from it after its first use. As with qjsLazy, a panic of open comes back
// on every read.
type qjsReader struct {
	once sync.Once
	open func() (*qjspack.Pack, error)
}

// qjsReaders are the readers' packs in use, which qjsOpen looks in after the
// others.
var qjsReaders struct {
	mu   sync.Mutex
	list atomic.Pointer[[]*qjsReader]
}

func (r *qjsReader) use(open func() (*qjspack.Pack, error)) {
	r.once.Do(func() {
		r.open = sync.OnceValues(open)
		qjsReaders.mu.Lock()
		defer qjsReaders.mu.Unlock()
		var list []*qjsReader
		if old := qjsReaders.list.Load(); old != nil {
			list = append(list, *old...)
		}
		list = append(list, r)
		qjsReaders.list.Store(&list)
	})
}

// qjsReaderPacks returns the readers' packs in use.
func qjsReaderPacks() ([]*qjspack.Pack, error) {
	list := qjsReaders.list.Load()
	if list == nil {
		return nil, nil
	}
	packs := make([]*qjspack.Pack, 0, len(*list))
	for _, r := range *list {
		p, err := r.open()
		if err != nil {
			return nil, err
		}
		packs = append(packs, p)
	}
	return packs, nil
}

// qjsReadersFile finds a data file in the readers' packs in use.
func qjsReadersFile(name string) ([]byte, error) {
	list := qjsReaders.list.Load()
	if list == nil {
		return nil, fs.ErrNotExist
	}
	for _, r := range *list {
		p, err := r.open()
		if err != nil {
			return nil, err
		}
		b, err := p.File(name)
		if !errors.Is(err, fs.ErrNotExist) {
			return b, err
		}
	}
	return nil, fs.ErrNotExist
}
