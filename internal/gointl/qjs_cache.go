// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"strings"
	"sync"

	"github.com/liustack/quickjs/internal/gointl/internal/datedata"
	"github.com/liustack/quickjs/internal/gointl/internal/numdata"
)

// CacheSource shares decoded number and date data and pattern generators.
// Its source must stay unchanged. Each cache has at most 64 entries and
// belongs to this wrapper, so different sources never share decoded data.
// Formatters retain immutable entries after eviction. No formatting call
// takes these locks or changes a cached value.
func CacheSource(src Source) Source { return &qjsCachedSource{Source: src} }

type qjsCachedSource struct {
	Source
	numbers    qjsCache[DataLocale, *numdata.Locale]
	dates      qjsCache[DataLocale, *datedata.Locale]
	calendars  qjsCache[qjsCalendarKey, qjsCalendarValue]
	generators qjsCache[qjsGeneratorKey, *dtpg]
}

const qjsDataCacheLimit = 64

type qjsCache[K comparable, V any] struct {
	mu     sync.Mutex
	values map[K]V
	order  [qjsDataCacheLimit]K
	next   int
}

func (c *qjsCache[K, V]) get(key K, load func() (V, error)) (V, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.values[key]; ok {
		return v, nil
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	if c.values == nil {
		c.values = make(map[K]V)
	}
	if len(c.values) == qjsDataCacheLimit {
		delete(c.values, c.order[c.next])
	}
	c.order[c.next] = key
	c.next = (c.next + 1) % qjsDataCacheLimit
	c.values[key] = v
	return v, nil
}

type qjsCalendarKey struct {
	data *datedata.Locale
	name string
}
type qjsCalendarValue struct {
	calendar *datedata.Calendar
	found    bool
}

func qjsCalendar(src Source, data *datedata.Locale, name string) (*datedata.Calendar, bool, error) {
	c, ok := src.(*qjsCachedSource)
	if !ok {
		return data.Calendar(name)
	}
	v, err := c.calendars.get(qjsCalendarKey{data, name}, func() (qjsCalendarValue, error) {
		cal, found, err := data.Calendar(name)
		return qjsCalendarValue{cal, found}, err
	})
	return v.calendar, v.found, err
}

type qjsGeneratorKey struct {
	patterns, glue   *datedata.Calendar
	names            [datedata.Fields]string
	decimal, allowed string
	hour             byte
	styles           bool
}

func (f *DateTimeFormat) qjsGenerator(hour byte, allowed []string, styles bool) *dtpg {
	load := func() (*dtpg, error) {
		return buildDTPG(f.patterns, f.calendar, f.data.FieldNames, f.decimal, hour, allowed, styles), nil
	}
	c, ok := f.src.(*qjsCachedSource)
	if !ok {
		g, _ := load()
		return g
	}
	key := qjsGeneratorKey{f.patterns, f.calendar, f.data.FieldNames, f.decimal, strings.Join(allowed, " "), hour, styles}
	g, _ := c.generators.get(key, load)
	return g
}
