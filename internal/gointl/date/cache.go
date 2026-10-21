package date

// V8's DateCache: the offset from UTC of the instants asked about, kept in
// segments of constant offset, which it grows by probing on the assumption
// that the offset changes at most once in 19 days.
//
// Where a zone changed twice in less than that, the assumption fails and V8
// answers with an offset the zone never had then: Africa/El_Aaiun, in 1976,
// went from UTC-1 to UTC on 14 April and to UTC+1 on 1 May, and asked about
// those weeks in order Node calls them UTC+1 throughout. Node's Date is what
// the cache answers, so this is the cache, ported: what it answers depends
// on what it has been asked before, as V8's does.

const (
	cacheSize = 32
	// defaultOffsetDelta is kDefaultTimeZoneOffsetDeltaInMs, 19 days.
	defaultOffsetDelta = 19 * msPerDay
)

type cacheItem struct {
	start, end int64
	offset     int64
	lastUsed   int
}

func (c *cacheItem) clear()        { *c = cacheItem{start: 0, end: -1} }
func (c *cacheItem) invalid() bool { return c.start > c.end }

// dateCache is DateCache's segments.
type dateCache struct {
	items         [cacheSize]cacheItem
	usage         int
	before, after *cacheItem
}

func (d *dateCache) reset() {
	for i := range d.items {
		d.items[i].clear()
	}
	d.usage = 0
	d.before, d.after = &d.items[0], &d.items[1]
}

func (d *dateCache) leastRecentlyUsed(skip *cacheItem) *cacheItem {
	var result *cacheItem
	for i := range d.items {
		if &d.items[i] == skip {
			continue
		}
		if result == nil || result.lastUsed > d.items[i].lastUsed {
			result = &d.items[i]
		}
	}
	result.clear()
	return result
}

func (d *dateCache) extendAfter(t, offset int64) {
	a := d.after
	if !a.invalid() && a.offset == offset && a.start-defaultOffsetDelta <= t && t <= a.end {
		a.start = t
		return
	}
	if !a.invalid() {
		d.after = d.leastRecentlyUsed(d.before)
	}
	d.usage++
	*d.after = cacheItem{start: t, end: t, offset: offset, lastUsed: d.usage}
}

func (d *dateCache) probe(t int64) {
	var before, after *cacheItem
	for i := range d.items {
		c := &d.items[i]
		if c.invalid() {
			continue
		}
		if c.start <= t {
			if before == nil || before.start < c.start {
				before = c
			}
		} else if t < c.end {
			if after == nil || after.end > c.end {
				after = c
			}
		}
	}
	if before == nil {
		if d.before.invalid() {
			before = d.before
		} else {
			before = d.leastRecentlyUsed(after)
		}
	}
	if after == nil {
		if d.after.invalid() && before != d.after {
			after = d.after
		} else {
			after = d.leastRecentlyUsed(before)
		}
	}
	d.before, d.after = before, after
}

// localOffset is DateCache::LocalOffsetInMs from UTC, fetching from the
// zone with get.
func (d *dateCache) localOffset(t int64, get func(int64) int64) int64 {
	if d.usage >= 1<<31-1-10 {
		d.usage = 0
		for i := range d.items {
			d.items[i].clear()
		}
	}
	if d.before.start <= t && t <= d.before.end {
		d.usage++
		d.before.lastUsed = d.usage
		return d.before.offset
	}
	d.probe(t)
	b := d.before
	if b.invalid() {
		d.usage++
		*b = cacheItem{start: t, end: t, offset: get(t), lastUsed: d.usage}
		return b.offset
	}
	if t <= b.end {
		d.usage++
		b.lastUsed = d.usage
		return b.offset
	}
	if t-defaultOffsetDelta > b.end {
		offset := get(t)
		d.extendAfter(t, offset)
		d.before, d.after = d.after, d.before
		return offset
	}
	d.usage++
	b.lastUsed = d.usage
	newAfterStart := b.end + defaultOffsetDelta
	if d.after.invalid() || newAfterStart <= d.after.start {
		d.extendAfter(newAfterStart, get(newAfterStart))
	} else {
		d.usage++
		d.after.lastUsed = d.usage
	}
	b = d.before
	a := d.after
	if b.offset == a.offset {
		b.end = a.end
		a.clear()
		return b.offset
	}
	for i := 4; i >= 0; i-- {
		delta := a.start - b.end
		middle := b.end + delta/2
		if i == 0 {
			middle = t
		}
		offset := get(middle)
		if b.offset == offset {
			b.end = middle
			if t <= b.end {
				return offset
			}
		} else {
			a.start = middle
			if t >= a.start {
				d.before, d.after = d.after, d.before
				return offset
			}
		}
	}
	panic("date: the offset cache found no offset")
}
