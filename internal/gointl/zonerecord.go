package intl

// A ZoneRecord is a zone as ICU's zoneinfo64 holds it, for code that
// reckons with the data itself rather than as ICU's OlsonTimeZone does:
// ICU4X's zoneinfo64 crate, which Node's Temporal runs, reads the same
// resource and answers differently at the edges.
type ZoneRecord struct {
	// Name is the name as ICU spells it. For a link it is the link's name;
	// the rest is the zone it links to.
	Name string
	// Types are zoneinfo64's typeOffsets: each a raw offset and a daylight
	// saving, in seconds, the first in force before any transition.
	Types [][2]int
	// Transitions are each transition's instant, in seconds since 1970, in
	// order, and TransitionTypes the type each starts. The record is the
	// caller's own, every slice in it a copy.
	Transitions     []int64
	TransitionTypes []uint8
	// FinalRule is the rule the zone ends in, nil for none: ICU's eleven
	// numbers for it (start month, day, day of week, time and time mode,
	// the same for the end, and the saving), from FinalYear on, with the raw
	// offset FinalRaw.
	FinalRule []int
	FinalRaw  int
	FinalYear int
}

// LoadZoneRecord is the zoneinfo64 record of a zone ICU knows by name, in
// any case.
func LoadZoneRecord(src Source, name string) (*ZoneRecord, error) {
	z, err := loadTimeZone(src, name)
	if err != nil {
		return nil, err
	}
	// The transitions' types are the Source's memory, which nothing may
	// change, and the embedded data's cannot be written: the record has a
	// copy.
	r := &ZoneRecord{
		Name:            z.name,
		Transitions:     z.trans,
		TransitionTypes: append([]uint8(nil), z.transTypes...),
		FinalRaw:        z.finalRaw,
		FinalYear:       z.finalYear,
	}
	for _, t := range z.types {
		r.Types = append(r.Types, [2]int{t.raw, t.dst})
	}
	if z.final != nil {
		r.FinalRule = append([]int(nil), z.finalRule...)
	}
	return r, nil
}
