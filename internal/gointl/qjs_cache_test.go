// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type qjsCountSource struct {
	Source
	reads  atomic.Int64
	broken bool
}

func (s *qjsCountSource) Open(m Marker, d DataLocale) ([]byte, error) {
	s.reads.Add(1)
	if s.broken && m == MarkerNumbers {
		return nil, errors.New("broken number data")
	}
	return s.Source.Open(m, d)
}

func TestQJSCachedSource(t *testing.T) {
	raw := &qjsCountSource{Source: Embedded}
	cached := CacheSource(raw)
	locales := []Locale{qjsLocale("en-US"), qjsLocale("zh-CN"), qjsLocale("ar-EG-u-nu-latn")}
	for _, loc := range locales {
		plain, err := NewNumberFormatFrom(raw, loc, NumberFormatOptions{Style: StyleCurrency, Currency: "USD", Compat: NodeICU})
		if err != nil {
			t.Fatal(err)
		}
		f, err := NewNumberFormatFrom(cached, loc, NumberFormatOptions{Style: StyleCurrency, Currency: "USD", Compat: NodeICU})
		if err != nil {
			t.Fatal(err)
		}
		if f.Format(-12345.67) != plain.Format(-12345.67) {
			t.Fatal("cache changed number text")
		}
		opts := DateTimeFormatOptions{DateStyle: LengthFull, TimeStyle: LengthLong, TimeZone: "Asia/Kathmandu", Compat: NodeICU}
		d, err := NewDateTimeFormatFrom(raw, loc, opts)
		if err != nil {
			t.Fatal(err)
		}
		instant := time.UnixMilli(-2208988800000)
		want := d.Format(instant)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 4 {
					f, err := NewDateTimeFormatFrom(cached, loc, opts)
					if err != nil {
						t.Error(err)
						return
					}
					if got := f.Format(instant); got != want {
						t.Errorf("%q != %q", got, want)
					}
				}
			}()
		}
		wg.Wait()
	}
	loc := qjsLocale("en-US")
	if _, err := loadNumbers(cached, loc); err != nil {
		t.Fatal(err)
	}
	count := raw.reads.Load()
	if _, err := loadNumbers(cached, loc); err != nil {
		t.Fatal(err)
	}
	if raw.reads.Load() != count {
		t.Fatal("cached locale was decoded again")
	}
	other := CacheSource(&qjsCountSource{Source: Embedded, broken: true})
	if _, err := loadNumbers(other, loc); err == nil {
		t.Fatal("another source reused cached numbers")
	}
}

func TestQJSDataCacheBound(t *testing.T) {
	var c qjsCache[int, int]
	for i := range 2 * qjsDataCacheLimit {
		got, err := c.get(i, func() (int, error) { return i, nil })
		if err != nil || got != i {
			t.Fatal(got, err)
		}
	}
	if len(c.values) != qjsDataCacheLimit {
		t.Fatal("unbounded cache", len(c.values))
	}
	if _, ok := c.values[0]; ok {
		t.Fatal("old entry was not evicted")
	}
	bad := errors.New("load failure")
	if _, err := c.get(-1, func() (int, error) { return 0, bad }); !errors.Is(err, bad) {
		t.Fatal(err)
	}
	if _, ok := c.values[-1]; ok {
		t.Fatal("failed load was cached")
	}
}
