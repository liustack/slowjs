package temporal

import (
	"fmt"
	"strings"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Zones is the time zones Temporal takes, as V8's provider has them:
// timezone_provider's names, read in any case, with the offsets of ICU's
// zoneinfo64 by ICU4X's rules. It is immutable and safe for concurrent use.
type Zones struct {
	names   []string
	lower   map[string]int
	primary map[int]int
	utc     int
	// infos is each name's zone, nil where ICU has no zone of that exact
	// name, which zoneinfo64 then does not find.
	infos []*zoneInfo
}

// LoadZones reads Temporal's zone names, and each zone's data, from src.
func LoadZones(src intl.Source) (*Zones, error) {
	b, err := src.Open(intl.MarkerTemporalZones, intl.DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("temporal: the zone names: %w", err)
	}
	z := &Zones{lower: map[string]int{}, primary: map[int]int{}}
	links := map[int]string{}
	for i, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if i == 0 {
			if !strings.HasPrefix(line, "version ") {
				return nil, fmt.Errorf("temporal: the zone names begin %q", line)
			}
			continue
		}
		name, link, _ := strings.Cut(line, " ")
		n := len(z.names)
		z.names = append(z.names, name)
		z.lower[strings.ToLower(name)] = n
		if link != "" {
			links[n] = link
		}
	}
	for n, link := range links {
		p, ok := z.lower[strings.ToLower(link)]
		if !ok {
			return nil, fmt.Errorf("temporal: the zone names link %s to %s", z.names[n], link)
		}
		z.primary[n] = p
	}
	utc, ok := z.lower["utc"]
	if !ok {
		return nil, fmt.Errorf("temporal: the zone names have no UTC")
	}
	z.utc = utc
	for _, name := range z.names {
		var zi *zoneInfo
		if r, err := intl.LoadZoneRecord(src, name); err == nil && r.Name == name {
			if zi, err = newZoneInfo(r); err != nil {
				return nil, fmt.Errorf("temporal: the zone %s: %w", name, err)
			}
		}
		z.infos = append(z.infos, zi)
	}
	return z, nil
}

// A TimeZone is a Temporal time zone: one of Temporal's named zones, or a
// fixed offset from UTC.
type TimeZone struct {
	named  bool
	offset int64 // nanoseconds, for an offset zone
	id     int   // the name's index, for a named zone
	zones  *Zones
	info   *zoneInfo
}

// get is TimeZoneProvider::get: a name, in any case.
func (zs *Zones) get(name []byte) (TimeZone, error) {
	i, ok := zs.lower[strings.ToLower(string(name))]
	if !ok {
		return TimeZone{}, rangeError("Unknown time zone identifier")
	}
	if zs.infos[i] == nil {
		return TimeZone{}, rangeError("Unknown timezone identifier")
	}
	return TimeZone{named: true, id: i, zones: zs, info: zs.infos[i]}, nil
}

// UTC is the zone "UTC".
func (zs *Zones) UTC() TimeZone {
	tz, err := zs.get([]byte("UTC"))
	if err != nil {
		return TimeZone{named: true, id: zs.utc, zones: zs, info: &zoneInfo{types: [][2]int32{{0, 0}}}}
	}
	return tz
}

// OffsetTimeZone is a zone of a fixed offset, in nanoseconds. Temporal's
// offset zones are whole minutes less than a day either way, as
// UtcOffset::from_minutes and the ±HH:MM it parses make them; another
// offset is an error, where it had written an identifier no parser reads.
func OffsetTimeZone(ns int64) (TimeZone, error) {
	const day = 24 * 60 * 60_000_000_000
	if ns%60_000_000_000 != 0 || ns <= -day || ns >= day {
		return TimeZone{}, rangeError("Offset time zones are whole minutes less than a day.")
	}
	return TimeZone{offset: ns}, nil
}

func fromMinuteRecord(o offsetRecord) TimeZone {
	minutes := int64(o.hour)*60 + int64(o.minute)
	if o.negative {
		minutes = -minutes
	}
	return TimeZone{offset: minutes * 60_000_000_000}
}

func (zs *Zones) fromTzRecord(r tzRecord) (TimeZone, error) {
	if r.isName {
		return zs.get(r.name)
	}
	return fromMinuteRecord(r.offset), nil
}

// TimeZoneFromIdentifier is TimeZone::try_from_identifier_str: a zone's
// name, or an offset to the minute.
func (zs *Zones) TimeZoneFromIdentifier(s []byte) (TimeZone, error) {
	c := &cursor{src: s}
	r, err := parseTimeZone(c)
	if err == nil {
		err = c.close()
	}
	if err != nil {
		return TimeZone{}, rangeError("Invalid TimeZone Identifier")
	}
	return zs.fromTzRecord(r)
}

// TimeZoneFromString is TimeZone::try_from_str: an identifier, or the zone
// of a string in any of Temporal's formats.
func (zs *Zones) TimeZoneFromString(s []byte) (TimeZone, error) {
	if tz, err := zs.TimeZoneFromIdentifier(s); err == nil {
		return tz, nil
	}
	if tz, ok := zs.parseAllowedTimeZoneFormats(s); ok {
		return tz, nil
	}
	return TimeZone{}, rangeError("Not a valid time zone string")
}

// parseAllowedTimeZoneFormats is parse_allowed_timezone_formats.
func (zs *Zones) parseAllowedTimeZoneFormats(s []byte) (TimeZone, bool) {
	var r parseRecord
	var err error
	if r, err = parseIXDTF(s, variantDateTime); err != nil {
		if r, err = parseAnnotatedTime(&cursor{src: s}, keepAll); err != nil {
			if r, err = parseIXDTF(s, variantYearMonth); err != nil {
				if r, err = parseIXDTF(s, variantMonthDay); err != nil {
					return TimeZone{}, false
				}
			}
		}
	}
	if r.tz != nil {
		tz, err := zs.fromTzRecord(*r.tz)
		return tz, err == nil
	}
	switch {
	case r.z:
		return zs.UTC(), true
	case r.offset != nil:
		if r.offset.hasSecond {
			return TimeZone{}, false
		}
		return fromMinuteRecord(*r.offset), true
	}
	return TimeZone{}, false
}

// ParseOffset is UtcOffset::from_utf8: an offset to the nanosecond, in
// nanoseconds.
func ParseOffset(s []byte) (int64, error) {
	c := &cursor{src: s}
	o, err := parseUTCOffset(c)
	if err == nil {
		err = c.close()
	}
	if err != nil {
		return 0, err.(ixdtfError).asTemporal()
	}
	return offsetNanoseconds(o)
}

// offsetNanoseconds is UtcOffset::from_ixdtf_record.
func offsetNanoseconds(o offsetRecord) (int64, error) {
	minutes := int64(o.hour)*60 + int64(o.minute)
	sign := int64(1)
	if o.negative {
		sign = -1
	}
	if !o.hasSecond {
		return minutes * sign * 60_000_000_000, nil
	}
	ns := (minutes*60 + int64(o.second)) * 1_000_000_000
	if o.fraction != nil {
		f, ok := o.fraction.nanoseconds()
		if !ok {
			return 0, rangeError("Fractional time exceeds nine digits.")
		}
		ns += int64(f)
	}
	return ns * sign, nil
}

// IsOffset is whether the zone is a fixed offset rather than named.
func (tz TimeZone) IsOffset() bool { return !tz.named }

// Identifier is the zone's name as Temporal writes it, or its offset.
func (tz TimeZone) Identifier() string {
	if tz.named {
		return tz.zones.names[tz.id]
	}
	return formatOffset(tz.offset)
}

// primary is the name's primary name, by index.
func (tz TimeZone) primary() int {
	if p, ok := tz.zones.primary[tz.id]; ok {
		return p
	}
	return tz.id
}

// PrimaryIdentifier is the zone under its primary name.
func (tz TimeZone) PrimaryIdentifier() TimeZone {
	if tz.named {
		tz.id = tz.primary()
	}
	return tz
}

// Equals is TimeZoneEquals: named zones by their primary names, offsets
// by value.
func (tz TimeZone) Equals(o TimeZone) bool {
	if tz.named != o.named {
		return false
	}
	if tz.named {
		return tz.primary() == o.primary()
	}
	return tz.offset == o.offset
}

// offsetNanosFor is GetOffsetNanosecondsFor.
func (tz TimeZone) offsetNanosFor(ns int128) (int64, error) {
	if !tz.named {
		return tz.offset, nil
	}
	secs := ns.quo(i128(1_000_000_000))
	if !secs.fitsInt64() {
		return 0, rangeError("Epoch nanoseconds out of range")
	}
	s := secs.int64()
	if s < 0 && !ns.rem(i128(1_000_000_000)).isZero() {
		s--
	}
	return int64(tz.info.forTimestamp(s).offset) * 1_000_000_000, nil
}

// OffsetNanosecondsFor is the zone's offset at an instant, in nanoseconds.
func (tz TimeZone) OffsetNanosecondsFor(i Instant) (int64, error) { return tz.offsetNanosFor(i.ns) }

// isoDateTimeFor is GetISODateTimeFor.
func (tz TimeZone) isoDateTimeFor(ns int128) (ISODateTime, error) {
	off, err := tz.offsetNanosFor(ns)
	if err != nil {
		return ISODateTime{}, err
	}
	return isoDateTimeFromEpochNanoseconds(ns, off), nil
}

// epochAndOffset is EpochNanosecondsAndOffset.
type epochAndOffset struct {
	ns     int128
	offset int64 // seconds
}

// candidates is CandidateEpochNanoseconds: none, one or two instants, and
// for none the offsets either side of the gap and the transition.
type candidates struct {
	n                         int
	list                      [2]epochAndOffset
	offsetBefore, offsetAfter int64 // seconds
	transition                int128
}

// possibleEpochNanoseconds is GetPossibleEpochNanoseconds.
func (tz TimeZone) possibleEpochNanoseconds(dt ISODateTime) (candidates, error) {
	var c candidates
	if !tz.named {
		minutes := tz.offset / 60_000_000_000
		b := balanceISODateTime(int32(dt.Date.Year), int32(dt.Date.Month), int32(dt.Date.Day), int64(dt.Time.Hour),
			int64(int16(dt.Time.Minute)-int16(minutes)), int64(dt.Time.Second), int64(dt.Time.Millisecond),
			i128(int64(dt.Time.Microsecond)), i128(int64(dt.Time.Nanosecond)))
		if err := b.Date.checkDayRange(); err != nil {
			return c, err
		}
		c.n = 1
		c.list[0] = epochAndOffset{b.epochNanoseconds(), tz.offset / 1_000_000_000}
	} else {
		local := dt.epochNanoseconds()
		p := tz.info.forDateTime(int32(dt.Date.Year), uint8(dt.Date.Month), uint8(dt.Date.Day), uint8(dt.Time.Hour),
			uint8(dt.Time.Minute), uint8(dt.Time.Second))
		switch p.n {
		case 0:
			c.offsetBefore, c.offsetAfter = int64(p.before.offset), int64(p.after.offset)
			c.transition = i128(p.transition).mul64(1_000_000_000)
		case 1:
			c.n = 1
			c.list[0] = epochAndOffset{local.sub(i128(int64(p.single.offset)).mul64(1_000_000_000)), int64(p.single.offset)}
		default:
			c.n = 2
			c.list[0] = epochAndOffset{local.sub(i128(int64(p.before.offset)).mul64(1_000_000_000)), int64(p.before.offset)}
			c.list[1] = epochAndOffset{local.sub(i128(int64(p.after.offset)).mul64(1_000_000_000)), int64(p.after.offset)}
		}
	}
	for _, e := range c.list[:c.n] {
		if !validEpochNanoseconds(e.ns) {
			return c, rangeError("Instant nanoseconds are not within a valid epoch range.")
		}
	}
	return c, nil
}

func validEpochNanoseconds(ns int128) bool {
	return ns.cmp(nsMinInstant) >= 0 && ns.cmp(nsMaxInstant) <= 0
}

// epochNanosecondsFor is GetEpochNanosecondsFor.
func (tz TimeZone) epochNanosecondsFor(dt ISODateTime, d Disambiguation) (epochAndOffset, error) {
	c, err := tz.possibleEpochNanoseconds(dt)
	if err != nil {
		return epochAndOffset{}, err
	}
	return tz.disambiguate(c, dt, d)
}

// disambiguate is DisambiguatePossibleEpochNanoseconds.
func (tz TimeZone) disambiguate(c candidates, dt ISODateTime, d Disambiguation) (epochAndOffset, error) {
	switch c.n {
	case 1:
		return c.list[0], nil
	case 2:
		switch d {
		case Compatible, Earlier:
			return c.list[0], nil
		case Later:
			return c.list[1], nil
		}
		return epochAndOffset{}, rangeError("Rejecting ambiguous time zones.")
	}
	if d == DisambiguationReject {
		return epochAndOffset{}, rangeError("Rejecting ambiguous time zones.")
	}
	ns := i128(c.offsetAfter - c.offsetBefore).mul64(1_000_000_000)
	if d == Earlier {
		days, t := dt.Time.add(timeDuration{ns.neg()})
		date, err := tryBalanceISODate(int32(dt.Date.Year), int32(dt.Date.Month), int64(dt.Date.Day)+days)
		if err != nil {
			return epochAndOffset{}, err
		}
		p, err := tz.possibleEpochNanoseconds(ISODateTime{date, t})
		if err != nil {
			return epochAndOffset{}, err
		}
		if p.n == 0 {
			return epochAndOffset{}, assertError()
		}
		return p.list[0], nil
	}
	days, t := dt.Time.add(timeDuration{ns})
	date, err := tryBalanceISODate(int32(dt.Date.Year), int32(dt.Date.Month), int64(dt.Date.Day)+days)
	if err != nil {
		return epochAndOffset{}, err
	}
	p, err := tz.possibleEpochNanoseconds(ISODateTime{date, t})
	if err != nil {
		return epochAndOffset{}, err
	}
	if p.n == 0 {
		return epochAndOffset{}, assertError()
	}
	return p.list[p.n-1], nil
}

// startOfDay is GetStartOfDay.
func (tz TimeZone) startOfDay(d ISODate) (epochAndOffset, error) {
	c, err := tz.possibleEpochNanoseconds(ISODateTime{Date: d})
	if err != nil {
		return epochAndOffset{}, err
	}
	if c.n > 0 {
		return c.list[0], nil
	}
	return epochAndOffset{c.transition, c.offsetAfter}, nil
}

// transition is get_time_zone_transition: the next or previous change of
// offset, false for none.
func (tz TimeZone) transition(ns int128, next bool) (int128, bool, error) {
	secs := ns.divEuclid(i128(1_000_000_000))
	if !secs.fitsInt64() {
		return int128{}, false, rangeError("Epoch nanoseconds out of range")
	}
	s := secs.int64()
	var t zoneTransition
	var ok bool
	if next {
		t, ok = tz.info.nextTransition(s, true)
	} else {
		exact := ns.rem(i128(1_000_000_000)).isZero()
		t, ok = tz.info.prevTransition(s, exact, true)
	}
	if !ok {
		return int128{}, false, nil
	}
	return i128(t.since).mul64(1_000_000_000), true, nil
}
