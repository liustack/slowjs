// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"
)

// This file reads the ICU data file Chromium ships, icudtl.dat: an ICU
// "common data" package of resource bundles. Only what the generator needs
// is read: which bundles there are, which resources they hold, and the
// %%Parent and %%ALIAS strings that steer ICU's fallback. The format is
// ICU's (common/udata.cpp, common/uresdata.cpp), little endian.

// icuPackage is an ICU common data file.
type icuPackage struct {
	b     []byte
	items map[string][]byte // item name without the package prefix -> its data, header included
	name  string            // the package prefix, like "icudt78l"
}

func readICUPackage(b []byte) (*icuPackage, error) {
	if len(b) < 32 || b[2] != 0xda || b[3] != 0x27 {
		return nil, fmt.Errorf("not an ICU data file")
	}

	if b[8] != 0 {
		return nil, fmt.Errorf("big endian ICU data is not supported")
	}

	if string(b[12:16]) != "CmnD" {
		return nil, fmt.Errorf("not an ICU common data package: %q", b[12:16])
	}

	toc := int(binary.LittleEndian.Uint16(b))
	n := int(binary.LittleEndian.Uint32(b[toc:]))
	p := &icuPackage{b: b, items: map[string][]byte{}}
	type entry struct {
		name string
		off  int
	}
	var entries []entry
	for i := range n {
		at := toc + 4 + 8*i
		nameOff := toc + int(binary.LittleEndian.Uint32(b[at:]))
		dataOff := toc + int(binary.LittleEndian.Uint32(b[at+4:]))
		end := nameOff
		for b[end] != 0 {
			end++
		}
		entries = append(entries, entry{string(b[nameOff:end]), dataOff})
	}
	for i, e := range entries {
		end := len(b)
		if i+1 < len(entries) {
			end = entries[i+1].off
		}
		prefix, name, ok := strings.Cut(e.name, "/")
		if !ok {
			continue
		}

		p.name = prefix
		p.items[name] = b[e.off:end]
	}
	return p, nil
}

// icuBundleData is one resource bundle (a .res item).
type icuBundleData struct {
	pkg       *icuPackage
	root      []byte // the bundle data after its header
	rootRes   uint32
	indexes   []uint32
	usesPool  bool
	pool      *icuBundleData
	poolKeys  []byte // the pool's key strings
	poolLimit uint32 // string offsets below it are in the pool
}

func (p *icuPackage) bundle(name string) (*icuBundleData, error) {
	item, ok := p.items[name]
	if !ok {
		return nil, nil
	}

	hs := int(binary.LittleEndian.Uint16(item))
	if string(item[12:16]) != "ResB" {
		return nil, fmt.Errorf("%s: not a resource bundle", name)
	}

	r := &icuBundleData{pkg: p, root: item[hs:]}
	r.rootRes = binary.LittleEndian.Uint32(r.root)
	n := int(binary.LittleEndian.Uint32(r.root[4:]) & 0xff)
	for i := range n {
		r.indexes = append(r.indexes, binary.LittleEndian.Uint32(r.root[4+4*i:]))
	}
	if n <= 7 {
		return nil, fmt.Errorf("%s: resource bundle format too old", name)
	}

	attributes := r.indexes[5]
	r.usesPool = attributes&4 != 0
	r.poolLimit = r.indexes[0]>>8 | (attributes&0xf000)<<12
	if r.usesPool {
		tree, _, _ := strings.Cut(name, "/")
		poolName := "pool.res"
		if strings.Contains(name, "/") {
			poolName = tree + "/pool.res"
		}
		pool, err := p.bundle(poolName)
		if err != nil {
			return nil, err
		}

		if pool == nil {
			return nil, fmt.Errorf("%s: no %s", name, poolName)
		}

		r.pool = pool
		start := 4 * (1 + len(pool.indexes))
		r.poolKeys = pool.root[start : 4*pool.indexes[1]]
	}
	return r, nil
}

// u16 returns the 16 bit unit area: after the keys, up to the 16 bit top.
func (r *icuBundleData) u16() []byte { return r.root[4*r.indexes[1] : 4*r.indexes[6]] }

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// key returns the key at a 16 or 32 bit key offset. Bundles that use a pool
// number its keys first and their own after.
func (r *icuBundleData) key(off uint32) string {
	if !r.usesPool {
		return cString(r.root[off:])
	}

	n := uint32(len(r.poolKeys))
	if off < n {
		return cString(r.poolKeys[off:])
	}

	return cString(r.root[4*(1+len(r.indexes))+int(off-n):])
}

const (
	resString   = 0
	resAlias    = 3
	resTable    = 2
	resTable32  = 4
	resTable16  = 5
	resStringV2 = 6
	resArray    = 8
	resArray16  = 9
)

// arrayLen returns the length of an array resource, -1 if res is not one.
func (r *icuBundleData) arrayLen(res uint32) int {
	typ, off := res>>28, int(res&0x0fffffff)
	switch typ {
	case resArray:
		if off == 0 {
			return 0
		}

		return int(binary.LittleEndian.Uint32(r.root[4*off:]))
	case resArray16:
		return int(binary.LittleEndian.Uint16(r.u16()[2*off:]))
	}
	return -1
}

// currencies returns the entries of the bundle's Currencies table.
func (r *icuBundleData) currencies() map[string]uint32 {
	res, ok := r.get("Currencies")
	if !ok {
		return nil
	}

	return r.table(res)
}

// table returns the entries of a table resource, or nil if res is not one.
func (r *icuBundleData) table(res uint32) map[string]uint32 {
	typ, off := res>>28, int(res&0x0fffffff)
	m := map[string]uint32{}
	switch typ {
	case resTable:
		if off == 0 {
			return m
		}

		p := r.root[4*off:]
		n := int(binary.LittleEndian.Uint16(p))
		items := 2 + 2*n
		if n%2 == 0 {
			items += 2
		}
		for i := range n {
			m[r.key(uint32(binary.LittleEndian.Uint16(p[2+2*i:])))] = binary.LittleEndian.Uint32(p[items+4*i:])
		}
	case resTable16:
		p := r.u16()[2*off:]
		n := int(binary.LittleEndian.Uint16(p))
		for i := range n {
			k := binary.LittleEndian.Uint16(p[2+2*i:])
			v := binary.LittleEndian.Uint16(p[2+2*n+2*i:])
			m[r.key(uint32(k))] = resStringV2<<28 | uint32(v)
		}
	case resTable32:
		p := r.root[4*off:]
		n := int(binary.LittleEndian.Uint32(p))
		for i := range n {
			k := int32(binary.LittleEndian.Uint32(p[4+4*i:]))
			off := uint32(k)
			if k < 0 {
				off = uint32(k) & 0x7fffffff
			}
			m[r.key(off)] = binary.LittleEndian.Uint32(p[4+4*n+4*i:])
		}
	default:
		return nil
	}
	return m
}

// get returns the resource at path below the bundle's root, ok false if
// there is none.
func (r *icuBundleData) get(path ...string) (uint32, bool) {
	res := r.rootRes
	for _, k := range path {
		t := r.table(res)
		v, ok := t[k]
		if !ok {
			return 0, false
		}

		res = v
	}
	return res, true
}

// str returns a string resource.
func (r *icuBundleData) str(res uint32) (string, error) {
	typ, off := res>>28, res&0x0fffffff
	var units []uint16
	switch typ {
	case resStringV2:
		area := r.u16()
		if r.usesPool {
			if off < r.poolLimit {
				area = r.pool.u16()
			} else {
				off -= r.poolLimit
			}
		}
		p := area[2*off:]
		u := func(i int) uint16 { return binary.LittleEndian.Uint16(p[2*i:]) }
		n, start := 0, 0
		switch first := u(0); {
		case first < 0xdc00 || first > 0xdfff:
			for u(n) != 0 {
				n++
			}
		case first < 0xdfef:
			n, start = int(first&0x3ff), 1
		case first < 0xdfff:
			n, start = int(first-0xdfef)<<16|int(u(1)), 2
		default:
			n, start = int(u(1))<<16|int(u(2)), 3
		}
		for i := range n {
			units = append(units, u(start+i))
		}
	case resString, resAlias:
		if off == 0 {
			return "", nil
		}

		p := r.root[4*off:]
		n := int(binary.LittleEndian.Uint32(p))
		for i := range n {
			units = append(units, binary.LittleEndian.Uint16(p[4+2*i:]))
		}
	default:
		return "", fmt.Errorf("resource type %d is not a string", typ)
	}
	return string(utf16.Decode(units)), nil
}

// icuTrees are the bundle trees the locale data comes from, "" for the main
// one.
var icuTrees = []string{"", "curr", "zone", "unit", "region"}

var icuLocaleName = regexp.MustCompile(`^(root|[a-z]{2,3}(_[A-Z][a-z]{3})?(_([A-Z]{2}|[0-9]{3}))?(_[A-Z0-9]{4,8})?)$`)

// summarizeICU returns what the generator needs of Chromium's ICU data.
func summarizeICU(b []byte) (*icuSummary, error) {
	p, err := readICUPackage(b)
	if err != nil {
		return nil, err
	}

	s := &icuSummary{ICU: p.name, Bundles: map[string]icuBundle{}}
	type bundle struct {
		item, tree, name string
		r                *icuBundleData
	}
	var bundles []bundle
	for item := range p.items {
		tree, name := "", item
		if i := strings.IndexByte(item, '/'); i >= 0 {
			tree, name = item[:i], item[i+1:]
		}
		name, ok := strings.CutSuffix(name, ".res")
		if !ok || !icuLocaleName.MatchString(name) || !slices.Contains(icuTrees, tree) {
			continue
		}

		r, err := p.bundle(item)
		if err != nil {
			return nil, err
		}

		bundles = append(bundles, bundle{item, tree, name, r})
	}

	// The currencies Chromium has names for, and the ones some locale
	// formats with a pattern and separators of their own.
	codes, patterned := map[string]bool{}, map[string]bool{}
	for _, b := range bundles {
		if b.tree == "curr" {
			for code, res := range b.r.currencies() {
				codes[code] = true
				if b.r.arrayLen(res) > 2 {
					patterned[code] = true
				}
			}
		}
	}
	for code := range codes {
		s.Currencies = append(s.Currencies, code)
	}
	slices.Sort(s.Currencies)

	paths := map[string]bool{}
	has := map[string][]string{}
	links := map[string]map[string]string{}
	for _, b := range bundles {
		item, tree, name, r := b.item, b.tree, b.name, b.r
		var bundle icuBundle
		for k, dst := range map[string]*string{"%%ALIAS": &bundle.Alias, "%%Parent": &bundle.Parent} {
			if res, ok := r.get(k); ok {
				if *dst, err = r.str(res); err != nil {
					return nil, fmt.Errorf("%s %s: %v", item, k, err)
				}
			}
		}
		key := icuKey(tree, name)
		for _, path := range r.resources(tree, patterned) {
			paths[path] = true
			has[key] = append(has[key], path)
			if res, _ := r.get(strings.Split(path, "/")...); res>>28 == resAlias {
				to, err := r.str(res)
				if err != nil {
					return nil, fmt.Errorf("%s %s: %v", item, path, err)
				}

				target, ok := strings.CutPrefix(to, "/LOCALE/")
				if !ok {
					return nil, fmt.Errorf("%s %s: alias %q is not to the locale's own data", item, path, to)
				}

				if links[key] == nil {
					links[key] = map[string]string{}
				}
				links[key][path] = target
			}
		}
		s.Bundles[key] = bundle
	}
	s.Numbers = map[string]map[string]string{}
	for _, b := range bundles {
		if b.tree == "" {
			m, err := b.r.numbers()
			if err != nil {
				return nil, fmt.Errorf("%s: %v", b.item, err)
			}

			if len(m) > 0 {
				s.Numbers[b.name] = m
			}
		}
	}
	for path := range paths {
		s.Paths = append(s.Paths, path)
	}
	slices.Sort(s.Paths)
	index := map[string]int{}
	for i, path := range s.Paths {
		index[path] = i
	}
	for key, list := range has {
		b := s.Bundles[key]
		for _, path := range list {
			b.Has = append(b.Has, index[path])
		}
		for path, target := range links[key] {
			if b.Links == nil {
				b.Links = map[int]string{}
			}
			b.Links[index[path]] = target
		}
		slices.Sort(b.Has)
		s.Bundles[key] = b
	}
	return s, nil
}

// icuCalendars and icuCalendarParts are the calendar resources the data
// comes from.
var (
	icuCalendars     = []string{"gregorian", "buddhist", "persian", "generic"}
	icuCalendarParts = []string{"monthNames", "dayNames", "eras", "AmPmMarkers", "AmPmMarkersAbbr", "AmPmMarkersNarrow",
		"dayPeriod", "DateTimePatterns", "DateTimePatterns%atTime", "availableFormats", "appendItems"}
)

// resources returns the resources of the bundle the generator maps its keys
// to (see icuPath). For the currencies in patterned it adds Currencies/<code>
// where the bundle has the currency, and Currencies/<code>/pattern where that
// entry has a third element: a pattern, decimal separator and group
// separator for formatting in that currency.
func (r *icuBundleData) resources(tree string, patterned map[string]bool) (paths []string) {
	add := func(path ...string) {
		if _, ok := r.get(path...); ok {
			paths = append(paths, strings.Join(path, "/"))
		}
	}
	switch tree {
	case "":
		for _, c := range icuCalendars {
			for _, part := range icuCalendarParts {
				add("calendar", c, part)
			}
			// ICU falls back width by width for names: om_KE has its
			// narrow stand-alone day names and root's others.
			for _, part := range []string{"monthNames", "dayNames", "dayPeriod", "eras"} {
				res, ok := r.get("calendar", c, part)
				if !ok {
					continue
				}

				for k, v := range r.table(res) {
					if part == "eras" || v>>28 == resAlias {
						paths = append(paths, "calendar/"+c+"/"+part+"/"+k)
						continue
					}

					for width := range r.table(v) {
						paths = append(paths, "calendar/"+c+"/"+part+"/"+k+"/"+width)
					}
				}
			}
		}
		add("fields")
		add("NumberElements", "default")
		add("NumberElements", "native")
		add("NumberElements", "minimumGroupingDigits")
		if res, ok := r.get("NumberElements"); ok {
			for ns, v := range r.table(res) {
				if r.table(v) == nil {
					continue
				}

				// Symbols and patterns one by one: a bundle often has a few
				// of them and takes the rest from its parent.
				for _, part := range []string{"symbols", "patterns"} {
					if res, ok := r.get("NumberElements", ns, part); ok {
						for k := range r.table(res) {
							paths = append(paths, "NumberElements/"+ns+"/"+part+"/"+k)
						}
					}
				}
				add("NumberElements", ns, "patternsShort", "decimalFormat")
				add("NumberElements", ns, "patternsShort", "currencyFormat")
				add("NumberElements", ns, "patternsLong", "decimalFormat")
			}
		}
	case "curr":
		for _, k := range []string{"Currencies", "Currencies%narrow", "CurrencyPlurals", "CurrencyUnitPatterns"} {
			add(k)
		}
		// Plural names are looked up currency by currency: a locale
		// without them for a currency writes its name from Currencies.
		if res, ok := r.get("CurrencyPlurals"); ok {
			for code := range r.table(res) {
				paths = append(paths, "CurrencyPlurals/"+code)
			}
		}
		for code, res := range r.currencies() {
			if patterned[code] {
				paths = append(paths, "Currencies/"+code)
			}
			if r.arrayLen(res) > 2 {
				paths = append(paths, "Currencies/"+code+"/pattern")
			}
		}
	case "unit":
		for _, k := range []string{"units", "unitsShort", "unitsNarrow"} {
			add(k)
		}
	case "zone":
		add("zoneStrings")
	case "region":
		add("Countries")
		add("Countries%short")
	}
	return paths
}

// icuNumberNames are the symbols and patterns of a numbering system the
// runtime reads.
var icuNumberNames = map[string]bool{
	"symbols/decimal": true, "symbols/group": true, "symbols/minusSign": true, "symbols/plusSign": true,
	"symbols/percentSign": true, "symbols/exponential": true, "symbols/infinity": true, "symbols/nan": true,
	"symbols/currencyDecimal": true, "symbols/currencyGroup": true,
	"patterns/decimalFormat": true, "patterns/percentFormat": true, "patterns/scientificFormat": true,
	"patterns/currencyFormat": true, "patterns/accountingFormat": true,
}

// numbers returns the bundle's own symbols and patterns of numbering systems
// other than latn, by <system>/<symbols|patterns>/<name>. cldr-json leaves
// most of them out. An alias is kept as its target, which starts with
// /LOCALE/NumberElements/.
func (r *icuBundleData) numbers() (map[string]string, error) {
	res, ok := r.get("NumberElements")
	if !ok {
		return nil, nil
	}

	m := map[string]string{}
	for ns, nsRes := range r.table(res) {
		if ns == "latn" {
			continue
		}

		for _, part := range []string{"symbols", "patterns"} {
			for name, v := range r.table(r.table(nsRes)[part]) {
				if !icuNumberNames[part+"/"+name] {
					continue
				}

				str, err := r.str(v)
				if err != nil {
					return nil, fmt.Errorf("NumberElements/%s/%s/%s: %v", ns, part, name, err)
				}

				if v>>28 == resAlias && !strings.HasPrefix(str, "/LOCALE/NumberElements/") {
					return nil, fmt.Errorf("NumberElements/%s/%s/%s: alias %q is not to the locale's own numbers", ns, part, name, str)
				}

				m[ns+"/"+part+"/"+name] = str
			}
		}
	}
	return m, nil
}
