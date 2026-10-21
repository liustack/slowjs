package temporal

import (
	"fmt"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Data is what Temporal reckons with, read once from a source: every
// calendar Temporal takes, and its time zones. It is immutable and safe for
// concurrent use; an engine loads one and keeps it.
type Data struct {
	Zones     *Zones
	calendars map[string]*Calendar
}

// LoadData reads Temporal's calendars and zones from src.
func LoadData(src intl.Source) (*Data, error) {
	zs, err := LoadZones(src)
	if err != nil {
		return nil, err
	}
	d := &Data{Zones: zs, calendars: map[string]*Calendar{}}
	for _, id := range calendarIDs {
		c, err := NewCalendarFrom(src, id)
		if err != nil {
			return nil, err
		}
		d.calendars[id] = c
	}
	return d, nil
}

// Calendar is the calendar with a Temporal identifier, as CalendarID or a
// parsed string names it.
func (d *Data) Calendar(id string) (*Calendar, error) {
	if c := d.calendars[id]; c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("temporal: %q is not a calendar", id)
}

// ParseRelativeTo is RelativeTo::try_from_str_with_provider: a PlainDate,
// or a ZonedDateTime where the string has a zone annotation.
func (d *Data) ParseRelativeTo(s []byte) (RelativeTo, error) {
	r, err := parseDateTimeString(s)
	if err != nil {
		if r, err = parseZonedDateTimeString(s); err != nil {
			return RelativeTo{}, err
		}
	}
	calendarOf := func() (*Calendar, error) {
		if !r.hasCal {
			return isoCalendar, nil
		}
		id, err := calendarKindFromBytes(r.calendar)
		if err != nil {
			return nil, err
		}
		return d.Calendar(id)
	}
	if r.tz == nil {
		cal, err := calendarOf()
		if err != nil {
			return RelativeTo{}, err
		}
		pd, err := newPlainDate(int(r.date.year), int(r.date.month), int(r.date.day), cal, Reject)
		if err != nil {
			return RelativeTo{}, err
		}
		return RelativeTo{Date: &pd}, nil
	}
	tz, err := d.Zones.fromTzRecord(*r.tz)
	if err != nil {
		return RelativeTo{}, err
	}
	matchMinutes := true
	var offset *int64
	exact := false
	switch {
	case r.z:
		exact = true
	case r.offset != nil:
		o := r.offset
		ns := int64(o.hour)*3_600_000_000_000 + int64(o.minute)*60_000_000_000
		if o.hasSecond {
			matchMinutes = false
			ns += int64(o.second) * 1_000_000_000
		}
		if o.fraction != nil {
			if f, ok := o.fraction.nanoseconds(); ok {
				ns += int64(f)
			}
		}
		if o.negative {
			ns = -ns
		}
		offset = &ns
	}
	cal, err := calendarOf()
	if err != nil {
		return RelativeTo{}, err
	}
	var t *ISOTime
	if r.time != nil {
		tt, err := timeFromRecord(*r.time)
		if err != nil {
			return RelativeTo{}, err
		}
		t = &tt
	}
	date, err := newISODateWithOverflow(int(r.date.year), int(r.date.month), int(r.date.day), Constrain)
	if err != nil {
		return RelativeTo{}, err
	}
	e, err := interpretISODateTimeOffset(date, t, exact, offset, tz, Compatible, OffsetReject, matchMinutes)
	if err != nil {
		return RelativeTo{}, err
	}
	z, err := newZonedDateTimeCached(e.ns, tz, cal, e.offset)
	if err != nil {
		return RelativeTo{}, err
	}
	return RelativeTo{Zoned: &z}, nil
}
