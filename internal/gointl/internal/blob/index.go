package blob

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// An Index is records sorted by key, read where they lie: finding one is a
// binary search of the keys, and nothing is decoded but the record found.
// A table a formatter looks a few things up in -- which bundles a tree
// has, what a zone is called -- is written as one rather than read whole
// every time a formatter is built.
//
// It is a version byte, which the reader requires as NewReader does, the
// number of records, then where each record ends, four bytes each, then
// the records: a key, length-prefixed, then the value.
type Index struct {
	ends    []byte
	records []byte
}

// BuildIndex writes records, which it sorts by key, under a version. A key
// given twice is an error.
func BuildIndex(version byte, records map[string][]byte) ([]byte, error) {
	keys := make([]string, 0, len(records))
	for k := range records {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var body []byte
	ends := make([]byte, 0, 4*len(keys))
	for _, k := range keys {
		body = binary.AppendUvarint(body, uint64(len(k)))
		body = append(body, k...)
		body = append(body, records[k]...)
		if uint64(len(body)) > 1<<32-1 {
			return nil, fmt.Errorf("blob: an index of more than 4 GB")
		}
		ends = binary.LittleEndian.AppendUint32(ends, uint32(len(body)))
	}
	out := binary.LittleEndian.AppendUint32([]byte{version}, uint32(len(keys)))
	out = append(out, ends...)
	return append(out, body...), nil
}

// ReadIndex reads an index's layout, refusing one written under another
// version.
func ReadIndex(b []byte, version byte) (Index, error) {
	if len(b) < 1 || b[0] != version {
		got := -1
		if len(b) > 0 {
			got = int(b[0])
		}
		return Index{}, fmt.Errorf("blob: an index of version %d, want %d", got, version)
	}
	b = b[1:]
	if len(b) < 4 {
		return Index{}, fmt.Errorf("blob: an index of %d bytes", len(b))
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < 0 || n > (len(b)-4)/4 {
		return Index{}, fmt.Errorf("blob: an index of %d records in %d bytes", n, len(b))
	}
	x := Index{ends: b[4 : 4+4*n], records: b[4+4*n:]}
	if n > 0 && int(binary.LittleEndian.Uint32(x.ends[4*(n-1):])) != len(x.records) {
		return Index{}, fmt.Errorf("blob: an index whose records do not fill it")
	}
	return x, nil
}

// Len is the number of records.
func (x Index) Len() int { return len(x.ends) / 4 }

// At returns record i's key and value, which share the index's memory. A
// malformed record reads as empty.
func (x Index) At(i int) (key, value []byte) {
	start := 0
	if i > 0 {
		start = int(binary.LittleEndian.Uint32(x.ends[4*(i-1):]))
	}
	end := int(binary.LittleEndian.Uint32(x.ends[4*i:]))
	if start < 0 || start > end || end > len(x.records) {
		return nil, nil
	}
	rec := x.records[start:end:end]
	n, w := binary.Uvarint(rec)
	if w <= 0 || n > uint64(len(rec)-w) {
		return nil, nil
	}
	return rec[w : w+int(n)], rec[w+int(n):]
}

// Find returns the value of the record with a key.
func (x Index) Find(key string) ([]byte, bool) {
	i := sort.Search(x.Len(), func(i int) bool {
		k, _ := x.At(i)
		return string(k) >= key
	})
	if i < x.Len() {
		if k, v := x.At(i); string(k) == key {
			return v, true
		}
	}
	return nil, false
}
