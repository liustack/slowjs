// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package zonedata

import "strings"

// Build materializes the lazy name tables for the Chromium pack generator.
func (l *Locale) Build() *Built {
	r := &Built{GMTFormat: l.GMTFormat, HourFormat: l.HourFormat, RegionFormat: l.RegionFormat, FallbackFormat: l.FallbackFormat}
	for _, k := range l.metazones.Keys() {
		v, ok := l.Metazone(k)
		if !ok {
			panic("zonedata: unreadable metazone " + k)
		}
		r.Metazones = append(r.Metazones, Entry{k, v})
	}
	for _, k := range l.zones.Keys() {
		v, ok := l.Zone(k)
		if !ok {
			panic("zonedata: unreadable zone " + k)
		}
		r.Zones = append(r.Zones, v)
	}
	for _, k := range l.regions.Keys() {
		v, ok := l.Region(k)
		if !ok {
			panic("zonedata: unreadable region " + k)
		}
		r.Regions = append(r.Regions, RegionEntry{k, v})
	}
	return r
}

// Build materializes the shared zone table for the Chromium pack generator.
func (m *Meta) Build() *BuiltMeta {
	r := &BuiltMeta{}
	for i := 0; i < m.index.Len(); i++ {
		key, value := m.index.At(i)
		kind, id, _ := strings.Cut(string(key), " ")
		switch kind {
		case "zone":
			z, ok := m.Zone(id)
			if !ok {
				panic("zonedata: unreadable zone " + id)
			}
			r.Zones = append(r.Zones, *z)
		case "alias":
			r.Aliases = append(r.Aliases, Alias{id, string(value)})
		case "primary":
			r.Primary = append(r.Primary, Alias{id, string(value)})
		case "golden":
			meta, region, _ := strings.Cut(id, " ")
			r.Golden = append(r.Golden, Golden{meta, region, string(value)})
		}
	}
	return r
}
