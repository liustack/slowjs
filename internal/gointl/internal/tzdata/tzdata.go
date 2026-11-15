// Package tzdata is the encoding of one time zone's file: its names, and its
// offsets as ICU's OlsonTimeZone keeps them, from zoneinfo64.
//
// A formatter reads its zone's file every time it is built, so the offsets
// are written as arrays of fixed-width little-endian numbers, which are
// read in one pass rather than parsed.
package tzdata

import (
	"encoding/binary"
	"fmt"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// FinalFields is how many numbers a final rule is: the raw offset, the year
// the rule governs from, and ICU's eleven numbers for the rule -- start
// month, day, day of week, time and time mode, the same for the end, and
// the saving.
const FinalFields = 13

// A Zone is one name's file.
type Zone struct {
	// Name is the name as ICU spells it, and Canonical the name
	// ZoneMeta::getCanonicalCLDRID gives it.
	Name      string
	Canonical string
	// Link, for a name that is a link in the tz data, is the zone whose
	// rules it keeps, in lowercase, and the zone has nothing else.
	Link string

	// Types are the offsets, the first in force before any transition.
	Types []Offset
	// Trans are the transitions' instants, in seconds since 1970, and
	// TransTypes the type each starts.
	Trans      []int64
	TransTypes []uint8
	// Final is empty, or the rule the zone ends in, FinalFields numbers.
	Final []int32
}

// An Offset is a raw offset and a daylight saving, in seconds.
type Offset struct {
	Raw, DST int32
}

// Encode writes a zone's file.
func Encode(z *Zone) []byte {
	w := blob.NewWriter(Version)
	w.String(z.Name)
	w.String(z.Canonical)
	w.String(z.Link)
	var b []byte
	for _, t := range z.Types {
		b = binary.LittleEndian.AppendUint32(b, uint32(t.Raw))
		b = binary.LittleEndian.AppendUint32(b, uint32(t.DST))
	}
	w.String(string(b))
	b = nil
	for _, t := range z.Trans {
		b = binary.LittleEndian.AppendUint64(b, uint64(t))
	}
	w.String(string(b))
	w.String(string(z.TransTypes))
	b = nil
	for _, v := range z.Final {
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	w.String(string(b))
	return w.Bytes()
}

// Decode reads what Encode wrote. The transition types are the file's own
// memory.
func Decode(data []byte) (*Zone, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	z := &Zone{Name: r.String(), Canonical: r.String(), Link: r.String()}
	types, trans, transTypes, final := r.Bytes(), r.Bytes(), r.Bytes(), r.Bytes()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if len(types)%8 != 0 || len(trans)%8 != 0 || len(transTypes) != len(trans)/8 ||
		len(final) != 0 && len(final) != 4*FinalFields {
		return nil, fmt.Errorf("tzdata: %d bytes of types, %d of transitions, %d of their types, %d of a final rule",
			len(types), len(trans), len(transTypes), len(final))
	}
	if len(types) > 0 {
		z.Types = make([]Offset, len(types)/8)
		for i := range z.Types {
			z.Types[i] = Offset{
				Raw: int32(binary.LittleEndian.Uint32(types[8*i:])),
				DST: int32(binary.LittleEndian.Uint32(types[8*i+4:])),
			}
		}
	}
	if len(trans) > 0 {
		z.Trans = make([]int64, len(trans)/8)
		for i := range z.Trans {
			z.Trans[i] = int64(binary.LittleEndian.Uint64(trans[8*i:]))
		}
		z.TransTypes = transTypes
	}
	if len(final) > 0 {
		z.Final = make([]int32, FinalFields)
		for i := range z.Final {
			z.Final[i] = int32(binary.LittleEndian.Uint32(final[4*i:]))
		}
	}
	return z, nil
}
