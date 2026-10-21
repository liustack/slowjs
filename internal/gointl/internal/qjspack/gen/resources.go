// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/blob"
	"github.com/liustack/quickjs/internal/gointl/internal/datedata"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
	"github.com/liustack/quickjs/internal/gointl/internal/namedata"
	"github.com/liustack/quickjs/internal/gointl/internal/numdata"
	"github.com/liustack/quickjs/internal/gointl/internal/rbnfdata"
	"github.com/liustack/quickjs/internal/gointl/internal/zonedata"
)

// chromiumResources selects resources from the same go-intl models the runtime
// reads. The ICU inventory decides their source, including missing bundles.
// Re-encoding uses one pool per data set, shared by all locales and VMs.
func chromiumResources(files, out map[string][]byte) error {
	c := loadChromiumICU(*icuSummaryFile, files)
	rules, err := rbnfdata.Decode(files["rbnf.bin"])
	if err != nil {
		return err
	}
	if len(c.rbnf) == 0 {
		return fmt.Errorf("ICU summary has no RBNF inventory: record it again")
	}
	rules.Groups = slices.DeleteFunc(rules.Groups, func(g rbnfdata.Group) bool {
		locale, _, _ := strings.Cut(g.Name, "/")
		return !slices.Contains(c.rbnf, locale)
	})
	out["rbnf.bin"] = rbnfdata.Encode(rules)
	src := intl.NewFS(os.DirFS(*dataDir))
	f, err := intl.NewFallbacker(src)
	if err != nil {
		return err
	}
	n := newModels(files, "numbers", numdata.Version, func(b []byte, p blob.Shared) (*numdata.Locale, error) {
		l, e := numdata.Decode(b, p)
		if e != nil {
			return nil, e
		}
		return l.Build(), nil
	}, src, f)
	d := newModels(files, "dates", datedata.Version, func(b []byte, p blob.Shared) (*datedata.Locale, error) {
		l, e := datedata.Decode(b, p)
		if e != nil {
			return nil, e
		}
		return l.Build()
	}, src, f)
	z := newModels(files, "zonenames", zonedata.Version, func(b []byte, p blob.Shared) (*zonedata.Built, error) {
		l, e := zonedata.Decode(b, p)
		if e != nil {
			return nil, e
		}
		return l.Build(), nil
	}, src, f)
	names := newModels(files, "names", namedata.Version, namedata.Decode, src, f)
	systems, err := numdata.DecodeSystems(files["numberingsystems.bin"])
	if err != nil {
		return err
	}
	if err := rewriteModels(out, n, numdata.Encode, func(tag string, l *numdata.Locale) *numdata.Locale {
		r := *l
		id := icuID(tag)
		choose := func(tree, path string) *numdata.Locale {
			s := c.resolve(tree, id, path)
			if s == "" {
				return nil
			}
			return n.get(s)
		}
		// Currency resources belong to a separate ICU tree. Its names and unit
		// patterns can come from English even when the number patterns do not.
		curr := choose("curr", "Currencies")
		narrow := choose("curr", "Currencies%narrow")
		r.Currencies = nil
		for _, code := range sortedKeys(c.currencies) {
			v := numdata.Currency{Code: code}
			if curr != nil {
				if from, ok := curr.Currency(code); ok {
					v.Symbol = from.Symbol
					v.DisplayName = from.DisplayName
				}
			}
			if narrow != nil {
				if from, ok := narrow.Currency(code); ok {
					v.Narrow = from.Narrow
				}
			}
			plural := choose("curr", "CurrencyPlurals/"+code)
			if plural != nil {
				if from, ok := plural.Currency(code); ok {
					v.Names = from.Names
				}
			}
			if len(v.Names) == 0 && v.DisplayName != "" {
				v.Names = []numdata.CountedText{{Count: "other", Text: v.DisplayName}}
			}
			if (tag == "uk" || strings.HasPrefix(tag, "uk-")) && code == "UAH" {
				if v.Symbol != "₴" {
					fail(fmt.Errorf("%s UAH symbol changed: %q", tag, v.Symbol))
				}
				v.Symbol = "грн"
			}
			if (tag == "ru" || strings.HasPrefix(tag, "ru-")) && code == "UAH" {
				if v.Symbol != "₴" {
					fail(fmt.Errorf("%s UAH symbol changed: %q", tag, v.Symbol))
				}
				v.Symbol = "грн."
			}
			r.Currencies = append(r.Currencies, v)
		}
		r.CurrencyFormats = nil
		for _, v := range l.CurrencyFormats {
			if c.currencies[v.Code] {
				if from := choose("curr", "Currencies/"+v.Code+"/pattern"); from != nil {
					if v, ok := from.CurrencyFormat(v.Code); ok {
						r.CurrencyFormats = append(r.CurrencyFormats, v)
					}
				}
			}
		}
		if from := choose("curr", "CurrencyUnitPatterns"); from != nil {
			r.UnitPatterns = from.UnitPatterns
		}
		if from := choose("", "NumberElements/minimumGroupingDigits"); from != nil {
			r.MinimumGroupingDigits = from.MinimumGroupingDigits
		}
		fixSystem := func(s numdata.System) numdata.System {
			ns := s.NumberingSystem
			prefix := "NumberElements/" + ns + "/"
			take := func(path string) *numdata.System {
				v := choose("", prefix+path)
				if v == nil {
					return nil
				}
				sel, ok := v.Select(ns, systems)
				if !ok {
					fail(fmt.Errorf("no %s system in %s", ns, tag))
				}
				return &sel.System
			}
			fields := []string{"Decimal", "Group", "PercentSign", "PlusSign", "MinusSign", "Exponential", "NaN", "Infinity", "CurrencyDecimal", "CurrencyGroup", "ApproximatelySign"}
			paths := []string{"decimal", "group", "percentSign", "plusSign", "minusSign", "exponential", "nan", "infinity", "currencyDecimal", "currencyGroup", "approximatelySign"}
			for i, k := range fields {
				if from := take("symbols/" + paths[i]); from != nil {
					reflect.ValueOf(&s.Symbols).Elem().FieldByName(k).Set(reflect.ValueOf(from.Symbols).FieldByName(k))
				}
			}
			for _, field := range []struct{ key, path string }{{"DecimalPattern", "decimalFormat"}, {"PercentPattern", "percentFormat"}, {"CurrencyPattern", "currencyFormat"}, {"AccountingPattern", "accountingFormat"}} {
				if from := take("patterns/" + field.path); from != nil {
					reflect.ValueOf(&s).Elem().FieldByName(field.key).Set(reflect.ValueOf(*from).FieldByName(field.key))
				}
			}
			if from := take("patternsShort/decimalFormat"); from != nil {
				s.CompactShort = from.CompactShort
			}
			from := take("patternsLong/decimalFormat")
			if from == nil && ns != "latn" {
				if v := choose("", "NumberElements/latn/patternsLong/decimalFormat"); v != nil {
					latn, ok := v.Select("latn", systems)
					if !ok {
						fail(fmt.Errorf("no Latin compact patterns for %s", tag))
					}
					from = &latn.System
				}
			}
			if from != nil {
				s.CompactLong = from.CompactLong
			} else {
				s.CompactLong = s.CompactShort
			}
			if from := take("patternsShort/currencyFormat"); from != nil {
				s.CurrencyCompact = from.CurrencyCompact
			}
			return s
		}
		defaultFrom := choose("", "NumberElements/default")
		if defaultFrom == nil {
			fail(fmt.Errorf("no default numbering system for %s", tag))
		}
		selected, ok := l.Select(defaultFrom.NumberingSystem, systems)
		if !ok {
			fail(fmt.Errorf("no default numbering system model for %s", tag))
		}
		r.System = fixSystem(selected.System)
		r.Others = nil
		for _, s := range append([]numdata.System{l.System}, l.Others...) {
			if s.NumberingSystem != r.NumberingSystem {
				r.Others = append(r.Others, fixSystem(s))
			}
		}
		slices.SortFunc(r.Others, func(a, b numdata.System) int { return strings.Compare(a.NumberingSystem, b.NumberingSystem) })
		chromiumNumbers(c, id, &r, systems)
		return &r
	}); err != nil {
		return err
	}
	if err := rewriteModels(out, d, datedata.Encode, func(tag string, l *datedata.Locale) *datedata.Locale {
		r := *l
		r.Calendars = slices.Clone(l.Calendars)
		id := icuID(tag)
		for i := range r.Calendars {
			cal := &r.Calendars[i]
			icucal := map[string]string{"gregory": "gregorian", "buddhist": "buddhist", "persian": "persian", "hebrew": "hebrew"}[cal.Name]
			if icucal == "" {
				continue
			} // The recorded inventory covers these calendars.
			choose := func(path string) *datedata.Calendar {
				s := c.resolve("", id, "calendar/"+icucal+"/"+path)
				if s == "" {
					return nil
				}
				from, ok, err := d.get(s).Calendar(cal.Name)
				if err != nil {
					fail(err)
				}
				if !ok {
					fail(fmt.Errorf("%s has no %s calendar", s, cal.Name))
				}
				return from
			}
			v := &cal.Calendar
			for ctx, ctxName := range []string{"format", "stand-alone"} {
				for width, wName := range []string{"wide", "abbreviated", "narrow", "short"} {
					at := ctx*datedata.Widths + width
					if values := c.calendarValues(id, "calendar/"+icucal+"/monthNames/"+ctxName+"/"+wName); values != nil {
						v.Months[at] = datedata.Names{Text: values}
					}
					if from := choose("dayNames/" + ctxName + "/" + wName); from != nil {
						v.Days[at] = from.Days[at]
					}
				}
			}
			for width, wName := range []string{"wide", "abbreviated", "narrow"} {
				if from := choose("eras/" + wName); from != nil {
					v.Eras[width] = from.Eras[width]
				}
				if from := choose([]string{"AmPmMarkers", "AmPmMarkersAbbr", "AmPmMarkersNarrow"}[width]); from != nil {
					v.AM[width] = from.AM[width]
					v.PM[width] = from.PM[width]
				}
				from := choose("dayPeriod/format/" + wName)
				if from == nil && icucal != "gregorian" {
					if src := c.resolve("", id, "calendar/gregorian/dayPeriod/format/"+wName); src != "" {
						from, _, err = d.get(src).Calendar("gregory")
						if err != nil {
							fail(err)
						}
					}
				}
				if from != nil {
					v.Periods[width] = from.Periods[width]
				} else {
					v.Periods[width] = nil
				}
			}
			if from := choose("DateTimePatterns"); from != nil {
				v.DateFormats = from.DateFormats
				v.TimeFormats = from.TimeFormats
				v.DateTimeFormats = from.DateTimeFormats
				v.DateNumbers = from.DateNumbers
				v.TimeNumbers = from.TimeNumbers
			}
			if icucal == "hebrew" {
				path := "calendar/hebrew/DateTimePatterns"
				values := c.calendarValues(id, path)
				if len(values) != 13 {
					fail(fmt.Errorf("%s: Hebrew styles have %d patterns", tag, len(values)))
				}
				copy(v.TimeFormats[:], values[:4])
				copy(v.DateFormats[:], values[4:8])
				copy(v.DateTimeFormats[:], values[9:13])
				v.TimeNumbers = [datedata.Lengths]string{}
				v.DateNumbers = [datedata.Lengths]string{}
				src := c.resolve("", id, path)
				overrides := c.calendarStrings[src][path+"%numbers"]
				if len(overrides) > 0 {
					copy(v.TimeNumbers[:], overrides[:4])
					copy(v.DateNumbers[:], overrides[4:8])
				}
			}
			values := c.calendarValues(id, "calendar/"+icucal+"/DateTimePatterns%atTime")
			if values == nil && icucal != "gregorian" {
				values = c.calendarValues(id, "calendar/gregorian/DateTimePatterns%atTime")
			}
			v.AtTimeFormats = v.DateTimeFormats
			if values != nil {
				if len(values) != datedata.Lengths {
					fail(fmt.Errorf("%s: invalid at-time patterns", tag))
				}
				copy(v.AtTimeFormats[:], values)
			}
			if from := choose("availableFormats"); from != nil {
				v.Available = from.Available
			}
			if from := choose("appendItems"); from != nil {
				v.AppendItems = from.AppendItems
			}
		}
		return &r
	}); err != nil {
		return err
	}
	if err := rewriteModels(out, z, zonedata.Encode, func(tag string, l *zonedata.Built) *zonedata.Built {
		id := icuID(tag)
		s := c.resolve("zone", id, "zoneStrings")
		if s == "" {
			fail(fmt.Errorf("no zone strings for %s", tag))
		}
		r := *z.get(s)
		r.Zones = slices.Clone(r.Zones)
		for i := range r.Zones {
			r.Zones[i].City = ""
		}
		r.Metazones = slices.Clone(r.Metazones)
		zoneValue := func(key string) string {
			for _, b := range c.chain("zone", id) {
				if text, ok := c.zoneStrings[b][key]; ok {
					if text == "∅∅∅" {
						return ""
					}
					return text
				}
			}
			return ""
		}
		zoneNames := func(key string) zonedata.Names {
			return zonedata.Names{LongGeneric: zoneValue(key + "/lg"), LongStandard: zoneValue(key + "/ls"), LongDaylight: zoneValue(key + "/ld"), ShortGeneric: zoneValue(key + "/sg"), ShortStandard: zoneValue(key + "/ss"), ShortDaylight: zoneValue(key + "/sd")}
		}
		for i := range r.Metazones {
			v := &r.Metazones[i]
			v.Names = zoneNames("meta:" + v.Metazone)
		}
		for i := range r.Zones {
			v := &r.Zones[i]
			key := strings.ReplaceAll(v.Zone, "/", ":")
			v.Names = zoneNames(key)
			v.City = zoneValue(key + "/ec")
		}
		r.GMTFormat = zoneValue("gmtFormat")
		r.HourFormat = zoneValue("hourFormat")
		r.RegionFormat = zoneValue("regionFormat")
		r.FallbackFormat = zoneValue("fallbackFormat")

		region := c.resolve("region", id, "Countries")
		if region != "" {
			from := names.get(region)
			r.Regions = slices.Clone(r.Regions)
			for i := range r.Regions {
				v := &r.Regions[i]
				width := namedata.Long
				if v.Region == "HK" || v.Region == "MO" {
					width = namedata.Short
				}
				if text, ok := from.Lookup(namedata.Region, width, v.Region); ok {
					v.Name = text
				}
			}
		}
		return &r
	}); err != nil {
		return err
	}
	return chromiumMetazones(out)
}

type models[T any] struct {
	dir      string
	pool     blob.Shared
	src      intl.Source
	fallback *intl.Fallbacker
	decode   func([]byte, blob.Shared) (*T, error)
	cache    map[string]*T
}

func newModels[T any](files map[string][]byte, dir string, version byte, decode func([]byte, blob.Shared) (*T, error), src intl.Source, f *intl.Fallbacker) *models[T] {
	p, err := blob.ReadShared(files[dir+"shared.bin"], version)
	if err != nil {
		fail(err)
	}
	return &models[T]{dir, p, src, f, decode, map[string]*T{}}
}
func (m *models[T]) get(tag string) *T {
	if tag == "root" {
		tag = "und"
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	if v, ok := m.cache[tag]; ok {
		return v
	}
	loc, err := intl.ParseLocale(tag)
	if err != nil {
		fail(err)
	}
	tree := "locales"
	if m.dir == "zonenames" {
		tree = "zone"
	}
	if m.dir == "names" {
		tree = "region"
	}
	for _, d := range append([]intl.DataLocale{loc.Data()}, m.fallback.ChainIn(tree, loc.Data())...) {
		b, err := m.src.Open(intl.Marker(m.dir), d)
		if err != nil {
			continue
		}
		v, err := m.decode(b, m.pool)
		if err != nil {
			fail(err)
		}
		m.cache[tag] = v
		return v
	}
	fail(fmt.Errorf("no %s model for %s", m.dir, tag))
	return nil
}
func rewriteModels[T any](out map[string][]byte, m *models[T], encode func(*T, *blob.Pool) []byte, change func(string, *T) *T) error {
	tags := map[string]bool{}
	list := "date"
	if m.dir == "numbers" {
		list = "number"
	}
	available, err := blob.ReadIndex(out["available.bin"], layout.Available)
	if err != nil {
		return err
	}
	for i := 0; i < available.Len(); i++ {
		key, _ := available.At(i)
		if tag, ok := strings.CutPrefix(string(key), list+" "); ok {
			tags[tag] = true
		}
	}
	for name := range out {
		if rest, ok := strings.CutPrefix(name, m.dir+"/"); ok && rest != "same.bin" {
			tags[strings.TrimSuffix(rest, ".bin")] = true
		}
	}
	pairs, err := readPairs(out[m.dir+"/same.bin"])
	if err != nil {
		return err
	}
	for _, p := range pairs {
		tags[p[0].String()] = true
	}
	pool := blob.NewPool(out[m.dir+"shared.bin"][0])
	for _, tag := range sortedKeys(tags) {
		out[m.dir+"/"+tag+".bin"] = encode(change(tag, m.get(tag)), pool)
	}
	out[m.dir+"shared.bin"] = pool.Bytes()
	// Every alias now has its own model because resource inheritance can
	// distinguish two locales that the upstream data made identical.
	out[m.dir+"/same.bin"] = writePairs(nil, func(intl.DataLocale) bool { return true })
	return nil
}
func sortedKeys[V any](m map[string]V) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	slices.Sort(r)
	return r
}

// Chromium removes GMT's names from zones of the British Isles.
func chromiumMetazones(out map[string][]byte) error {
	m, err := zonedata.DecodeMeta(out["metazones.bin"])
	if err != nil {
		return err
	}
	built := m.Build()
	removed := 0
	for i := range built.Zones {
		z := &built.Zones[i]
		switch z.Region {
		case "GB", "IE", "GG", "JE", "IM":
			uses := z.Uses[:0]
			for _, u := range z.Uses {
				if u.Metazone == "GMT" {
					removed++
				} else {
					uses = append(uses, u)
				}
			}
			z.Uses = uses
		}
	}
	if removed == 0 {
		return fmt.Errorf("no British Isles GMT metazone to remove")
	}
	out["metazones.bin"], err = zonedata.EncodeMeta(built)
	return err
}

// Chromium's non-Latin resources include aliases back to the locale's Latin
// symbols. Keep only differences from Select's root-over-Latin model.
func chromiumNumbers(c *chromiumICU, id string, l *numdata.Locale, systems []numdata.NumberingSystem) {
	l.Partials = nil
	fields := map[string]func(*numdata.System) *string{
		"symbols/decimal":           func(s *numdata.System) *string { return &s.Symbols.Decimal },
		"symbols/group":             func(s *numdata.System) *string { return &s.Symbols.Group },
		"symbols/minusSign":         func(s *numdata.System) *string { return &s.Symbols.MinusSign },
		"symbols/plusSign":          func(s *numdata.System) *string { return &s.Symbols.PlusSign },
		"symbols/percentSign":       func(s *numdata.System) *string { return &s.Symbols.PercentSign },
		"symbols/exponential":       func(s *numdata.System) *string { return &s.Symbols.Exponential },
		"symbols/infinity":          func(s *numdata.System) *string { return &s.Symbols.Infinity },
		"symbols/nan":               func(s *numdata.System) *string { return &s.Symbols.NaN },
		"symbols/currencyDecimal":   func(s *numdata.System) *string { return &s.Symbols.CurrencyDecimal },
		"symbols/currencyGroup":     func(s *numdata.System) *string { return &s.Symbols.CurrencyGroup },
		"patterns/decimalFormat":    func(s *numdata.System) *string { return &s.DecimalPattern },
		"patterns/percentFormat":    func(s *numdata.System) *string { return &s.PercentPattern },
		"patterns/currencyFormat":   func(s *numdata.System) *string { return &s.CurrencyPattern },
		"patterns/accountingFormat": func(s *numdata.System) *string { return &s.AccountingPattern },
	}
	latin, ok := l.Select("latn", systems)
	if !ok {
		fail(fmt.Errorf("no Latin system for %s", id))
	}
	find := func(key string) (string, bool) {
		for range 8 {
			ns, path, _ := strings.Cut(key, "/")
			if ns == "latn" {
				return *fields[path](&latin.System), true
			}
			value, found := "", false
			for _, b := range c.chain("", id) {
				if value, found = c.numbers[b][key]; found {
					break
				}
			}
			if !found {
				return "", false
			}
			if to, alias := strings.CutPrefix(value, "/LOCALE/NumberElements/"); alias {
				key = to
				continue
			}
			return value, true
		}
		fail(fmt.Errorf("number alias loop at %s %s", id, key))
		return "", false
	}
	for _, system := range systems {
		ns := system.Name
		if ns == "latn" {
			continue
		}
		var own *numdata.System
		if l.NumberingSystem == ns {
			own = &l.System
		}
		for i := range l.Others {
			if l.Others[i].NumberingSystem == ns {
				own = &l.Others[i]
				break
			}
		}
		selected, ok := l.Select(ns, systems)
		if !ok {
			fail(fmt.Errorf("no system %s for %s", ns, id))
		}
		partial := numdata.System{NumberingSystem: ns}
		changed := false
		for _, path := range sortedKeys(fields) {
			value, found := find(ns + "/" + path)
			if !found {
				continue
			}
			field := fields[path]
			if own != nil {
				*field(own) = value
				continue
			}
			if *field(&selected.System) != value {
				*field(&partial) = value
				changed = true
			}
		}
		if changed {
			l.Partials = append(l.Partials, partial)
		}
	}
}
