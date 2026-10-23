package zonedata

import (
	"encoding/binary"
	"fmt"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// MetaVersion is the encoding version of the shared zone table.
const MetaVersion = 4

// BuiltMeta is what every locale shares about zones, as a generator builds
// it: which name is the canonical one, where each zone is, which metazone it
// was in when, and which zone stands for a metazone in each region. It comes
// from ICU's metaZones.txt, timezoneTypes.txt and zoneinfo64.txt.
type BuiltMeta struct {
	// Zones are the canonical zones.
	Zones []MetaZone
	// Aliases map every other identifier to its canonical one.
	Aliases []Alias
	// Golden are the reference zones of the metazones -- the zone whose
	// offset a metazone's name means, in a region or in the world ("001").
	Golden []Golden
	// Primary is the zone that stands for a region with several.
	Primary []Alias
}

// A MetaZone is one canonical zone.
type MetaZone struct {
	Zone string
	// Region is where the zone is, "001" for none.
	Region string
	// Uses are the metazones it has belonged to, in order.
	Uses []Use
}

// A Use is a stretch of time a zone belonged to a metazone. From and To are
// minutes since 1970 in UTC, plus one; zero means unbounded, which ICU's
// data never is: a use with no dates runs from 1970 to 9999-12-31 23:59.
// They are 64 bits because that end is more minutes than a 32-bit int
// holds.
type Use struct {
	Metazone string
	From, To int64
}

// An Alias maps one name to another: an identifier to its canonical zone, or
// a region to its primary zone.
type Alias struct {
	From, To string
}

// Golden is a metazone's reference zone in one region.
type Golden struct {
	Metazone, Region, Zone string
}

// Meta is the shared zone table read where it lies, an index (blob.Index)
// of records:
//
//	zone <id>                 the region, then the uses
//	alias <id>                the canonical zone
//	golden <metazone> <region>  the reference zone
//	primary <region>          the primary zone
//	zones <region>            how many canonical zones the region has
type Meta struct {
	index blob.Index
}

// Canonical returns the canonical form of a zone identifier, and whether it
// is one ICU knows.
func (m *Meta) Canonical(id string) (string, bool) {
	if _, ok := m.index.Find("zone " + id); ok {
		return id, true
	}
	to, ok := m.index.Find("alias " + id)
	return string(to), ok
}

// Zone finds a canonical zone.
func (m *Meta) Zone(id string) (*MetaZone, bool) {
	b, ok := m.index.Find("zone " + id)
	if !ok {
		return nil, false
	}
	r, err := blob.NewReader(b, MetaVersion)
	if err != nil {
		return nil, false
	}
	z := &MetaZone{Zone: id, Region: r.String()}
	n := r.Uint()
	if n < 0 || n > r.Left() {
		return nil, false
	}
	z.Uses = make([]Use, n)
	for i := range z.Uses {
		z.Uses[i] = Use{Metazone: r.String(), From: r.Uint64(), To: r.Uint64()}
	}
	if r.Err() != nil {
		return nil, false
	}
	return z, true
}

// Reference returns a metazone's reference zone in a region, or in the
// world where the region has none of its own.
func (m *Meta) Reference(metazone, region string) string {
	if z, ok := m.index.Find("golden " + metazone + " " + region); ok {
		return string(z)
	}
	z, _ := m.index.Find("golden " + metazone + " 001")
	return string(z)
}

// PrimaryZone returns the zone that stands for a region with several.
func (m *Meta) PrimaryZone(region string) string {
	z, _ := m.index.Find("primary " + region)
	return string(z)
}

// ZonesIn returns how many canonical zones a region has.
func (m *Meta) ZonesIn(region string) int {
	b, ok := m.index.Find("zones " + region)
	if !ok {
		return 0
	}
	n, w := binary.Uvarint(b)
	if w <= 0 || n > 1<<20 {
		return 0
	}
	return int(n)
}

// EncodeMeta writes the shared zone table.
func EncodeMeta(m *BuiltMeta) ([]byte, error) {
	records := map[string][]byte{}
	add := func(key string, value []byte) error {
		if _, ok := records[key]; ok {
			return fmt.Errorf("zonedata: %q twice", key)
		}
		records[key] = value
		return nil
	}
	counts := map[string]uint64{}
	for _, z := range m.Zones {
		// Each zone's record is a small table of its own, with the
		// version, so that it is read with a blob.Reader.
		b := blob.NewWriter(MetaVersion)
		b.String(z.Region)
		b.Uint(len(z.Uses))
		for _, u := range z.Uses {
			b.String(u.Metazone)
			b.Uint64(u.From)
			b.Uint64(u.To)
		}
		if err := add("zone "+z.Zone, b.Bytes()); err != nil {
			return nil, err
		}
		counts[z.Region]++
	}
	for region, n := range counts {
		if err := add("zones "+region, binary.AppendUvarint(nil, n)); err != nil {
			return nil, err
		}
	}
	for _, a := range m.Aliases {
		if err := add("alias "+a.From, []byte(a.To)); err != nil {
			return nil, err
		}
	}
	for _, a := range m.Primary {
		if err := add("primary "+a.From, []byte(a.To)); err != nil {
			return nil, err
		}
	}
	for _, g := range m.Golden {
		if err := add("golden "+g.Metazone+" "+g.Region, []byte(g.Zone)); err != nil {
			return nil, err
		}
	}
	index, err := blob.BuildIndex(MetaVersion, records)
	if err != nil {
		return nil, err
	}
	b := blob.NewWriter(MetaVersion)
	b.String(string(index))
	return b.Bytes(), nil
}

// DecodeMeta reads what EncodeMeta wrote, where it lies.
func DecodeMeta(data []byte) (*Meta, error) {
	r, err := blob.NewReader(data, MetaVersion)
	if err != nil {
		return nil, err
	}
	b := r.Bytes()
	if err := r.Err(); err != nil {
		return nil, err
	}
	index, err := blob.ReadIndex(b, MetaVersion)
	if err != nil {
		return nil, err
	}
	return &Meta{index: index}, nil
}
