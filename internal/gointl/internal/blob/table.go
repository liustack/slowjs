package blob

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// A Table is an Index kept in a pool: records sorted by a key, where the
// key and everything recorded under it are parts of the pool, so that a list
// of names is kept once however many tables hold it, and each name once
// however many lists do. A list a formatter looks one or two things up in --
// what a locale calls a metazone, or a zone -- is written as one rather than
// read whole every time a formatter is built.
//
// Every record is the same number of part numbers, the key's first, each
// written in the same number of bytes, little-endian, so that record i is
// found without an index of where records end. The part is that width in
// bytes, the number of part numbers a record holds, then the records.
type Table struct {
	width, fields int
	records       []byte
	pool          *Shared
}

// SharedTable writes a table of records through the pool: value writes the
// record of each key, with Shared and SharedString only, and the same number
// of them for every key. keys must not repeat.
func (w *Writer) SharedTable(keys []string, value func(key string, w *Writer)) {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	var rows [][]uint64
	biggest := uint64(0)
	for _, k := range sorted {
		sub := &Writer{pool: w.pool}
		sub.SharedString(k)
		value(k, sub)
		var row []uint64
		for b := sub.b; len(b) > 0; {
			n, size := binary.Uvarint(b)
			row = append(row, n)
			biggest = max(biggest, n)
			b = b[size:]
		}
		if len(rows) > 0 && len(row) != len(rows[0]) {
			panic(fmt.Sprintf("blob: a table record of %d parts after one of %d", len(row), len(rows[0])))
		}
		rows = append(rows, row)
	}
	width := 1
	for biggest>>(8*width) != 0 {
		width++
	}
	fields := 0
	if len(rows) > 0 {
		fields = len(rows[0])
	}
	w.Shared(func(w *Writer) {
		w.b = append(w.b, byte(width), byte(fields))
		for _, row := range rows {
			for _, n := range row {
				for i := 0; i < width; i++ {
					w.b = append(w.b, byte(n>>(8*i)))
				}
			}
		}
	})
}

// SharedTable reads a table written by Writer.SharedTable, where it lies.
func (r *Reader) SharedTable() Table {
	part := r.sharedPart()
	if r.err != nil {
		return Table{}
	}
	if len(part) < 2 {
		r.err = fmt.Errorf("blob: a table of %d bytes", len(part))
		return Table{}
	}
	t := Table{width: int(part[0]), fields: int(part[1]), records: part[2:], pool: r.pool}
	if t.width < 1 || t.width > 4 || t.fields < 1 && len(t.records) > 0 ||
		t.fields > 0 && len(t.records)%(t.width*t.fields) != 0 {
		r.err = fmt.Errorf("blob: a table of %d-byte numbers, %d a record, in %d bytes",
			t.width, t.fields, len(t.records))
		return Table{}
	}
	return t
}

// Len is the number of records.
func (t Table) Len() int {
	if t.fields == 0 {
		return 0
	}
	return len(t.records) / (t.width * t.fields)
}

// number returns the jth part number of record i.
func (t Table) number(i, j int) int {
	at := (i*t.fields + j) * t.width
	n := 0
	for k := t.width - 1; k >= 0; k-- {
		n = n<<8 | int(t.records[at+k])
	}
	return n
}

// Find returns a reader of the rest of the record with a key, which must
// read all of it.
func (t Table) Find(key string) (*Reader, bool) {
	i := sort.Search(t.Len(), func(i int) bool {
		k, _ := t.pool.part(t.number(i, 0))
		return string(k) >= key
	})
	if i == t.Len() {
		return nil, false
	}
	if k, ok := t.pool.part(t.number(i, 0)); !ok || string(k) != key {
		return nil, false
	}
	// The reader reads the part numbers as a table writes them.
	var b []byte
	for j := 1; j < t.fields; j++ {
		b = binary.AppendUvarint(b, uint64(t.number(i, j)))
	}
	return &Reader{b: b, pool: t.pool}, true
}
