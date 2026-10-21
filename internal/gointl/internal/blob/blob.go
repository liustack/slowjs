// Package blob is the length-prefixed encoding the generated tables are
// written in.
//
// It is deliberately dull. The seam that matters is the Source the bytes
// arrive through, not a clever encoding, and a dull one can be read by eye
// when a generator and a reader disagree about what is in a file.
//
// Every table begins with a version byte, so data written by an older command
// is refused rather than misread.
//
// A table is read where it lies: the strings a Reader returns share the
// table's memory rather than copying it, so the bytes a Reader is given
// must never change for as long as anything read from them is kept.
package blob

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// str is b as a string, sharing its memory, which never changes.
func str(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// A Writer builds a table.
type Writer struct {
	b    []byte
	pool *Pool
}

// NewWriter starts a table of the given version.
func NewWriter(version byte) *Writer { return &Writer{b: []byte{version}} }

// String appends a length-prefixed string.
func (w *Writer) String(s string) {
	w.b = binary.AppendUvarint(w.b, uint64(len(s)))
	w.b = append(w.b, s...)
}

// Uint appends a number.
func (w *Writer) Uint(v int) {
	if v < 0 {
		v = 0
	}
	w.b = binary.AppendUvarint(w.b, uint64(v))
}

// Uint64 appends a number that may not fit a 32-bit int. It is written as
// Uint writes it; only the reading differs.
func (w *Writer) Uint64(v int64) {
	if v < 0 {
		v = 0
	}
	w.b = binary.AppendUvarint(w.b, uint64(v))
}

// Bytes returns the finished table.
func (w *Writer) Bytes() []byte { return w.b }

// A Reader takes a table apart.
type Reader struct {
	b    []byte
	err  error
	pool *Shared
}

// NewReader starts reading a table, checking its version.
func NewReader(b []byte, version byte) (*Reader, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("blob: the data is empty")
	}
	if b[0] != version {
		return nil, fmt.Errorf("blob: the data is version %d, not %d", b[0], version)
	}
	return &Reader{b: b[1:]}, nil
}

// String reads the next length-prefixed string. Once a read has failed every
// later one returns nothing, so a caller can read a whole table and check once
// at the end.
func (r *Reader) String() string {
	n := r.Uint()
	if r.err != nil {
		return ""
	}
	if n > len(r.b) {
		r.err = fmt.Errorf("blob: a string of %d bytes with %d left", n, len(r.b))
		return ""
	}
	s := str(r.b[:n])
	r.b = r.b[n:]
	return s
}

// Bytes reads the next length-prefixed string without copying it: what it
// returns shares the table's memory, so a table read this way is read where it
// lies, and the caller must not change it.
func (r *Reader) Bytes() []byte {
	n := r.Uint()
	if r.err != nil {
		return nil
	}
	if n > len(r.b) {
		r.err = fmt.Errorf("blob: a string of %d bytes with %d left", n, len(r.b))
		return nil
	}
	b := r.b[:n:n]
	r.b = r.b[n:]
	return b
}

// Uint reads the next number. One too large for an int, which on a 32-bit
// platform is 32 bits, is a failure rather than a number wrapped round; a
// table that holds such numbers reads them with Uint64.
func (r *Reader) Uint() int {
	v := r.Uint64()
	if int64(int(v)) != v {
		r.err = fmt.Errorf("blob: %d does not fit an int", v)
		return 0
	}
	return int(v)
}

// Uint64 reads the next number as 64 bits, whatever the platform.
func (r *Reader) Uint64() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = fmt.Errorf("blob: a number is cut short")
		return 0
	}
	if v > 1<<63-1 {
		r.err = fmt.Errorf("blob: %d does not fit an int64", v)
		return 0
	}
	r.b = r.b[n:]
	return int64(v)
}

// Left reports how many bytes have not been read.
func (r *Reader) Left() int { return len(r.b) }

// Err returns the first failure, and reports anything left over as one, so
// that a table longer than the reader expected is noticed rather than ignored.
func (r *Reader) Err() error {
	if r.err != nil {
		return r.err
	}
	if len(r.b) != 0 {
		return fmt.Errorf("blob: %d bytes left over", len(r.b))
	}
	return nil
}

// Sharing: the parts many tables have in common, kept once.
//
// A data set kept per locale says much the same in every locale: the same
// skeletons, the same interval patterns, the same month names in a
// language's dozen regions. A generator writing such a set gives all its
// writers one Pool. A part written through Shared or SharedString goes
// into the pool once, however many tables write it, and each table holds
// only its number. The pool is written beside the tables and read where it
// lies: a part is found by its number without reading any other.

// A Pool gathers the parts a data set's tables share, for a generator.
// Parts are numbered in the order they are first written, so a generator
// that writes its tables in a fixed order writes the same pool every time.
type Pool struct {
	version byte
	index   map[string]int
	parts   []string
}

// NewPool starts a pool for tables of the given version.
func NewPool(version byte) *Pool {
	return &Pool{version: version, index: map[string]int{}}
}

func (p *Pool) intern(b []byte) int {
	if i, ok := p.index[string(b)]; ok {
		return i
	}
	i := len(p.parts)
	p.index[string(b)] = i
	p.parts = append(p.parts, string(b))
	return i
}

// Bytes returns the pool as it is written: the version, the number of
// parts and where each ends, four bytes each, then the parts.
func (p *Pool) Bytes() []byte {
	size := 0
	for _, s := range p.parts {
		size += len(s)
	}
	out := make([]byte, 0, 1+4+4*len(p.parts)+size)
	out = append(out, p.version)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(p.parts)))
	end := 0
	for _, s := range p.parts {
		end += len(s)
		out = binary.LittleEndian.AppendUint32(out, uint32(end))
	}
	for _, s := range p.parts {
		out = append(out, s...)
	}
	return out
}

// NewPooledWriter starts a table whose shared parts go into pool.
func NewPooledWriter(version byte, pool *Pool) *Writer {
	return &Writer{b: []byte{version}, pool: pool}
}

// Shared writes a part through the pool: what write writes is kept once
// in the pool, and the table holds its number.
func (w *Writer) Shared(write func(*Writer)) {
	sub := &Writer{pool: w.pool}
	write(sub)
	w.Uint(w.pool.intern(sub.b))
}

// SharedString writes a string through the pool.
func (w *Writer) SharedString(s string) {
	w.Uint(w.pool.intern([]byte(s)))
}

// A Shared is a pool read where it lies.
type Shared struct {
	ends []byte // four bytes a part
	data []byte
}

// ReadShared checks a pool's version and layout.
func ReadShared(b []byte, version byte) (Shared, error) {
	if len(b) < 5 {
		return Shared{}, fmt.Errorf("blob: a pool of %d bytes", len(b))
	}
	if b[0] != version {
		return Shared{}, fmt.Errorf("blob: the pool is version %d, not %d", b[0], version)
	}
	// The count is compared with what the bytes can hold, not multiplied
	// out, which overflows: 0x40000000 parts are 0 bytes on 32 bits.
	n := int(binary.LittleEndian.Uint32(b[1:]))
	if n < 0 || n > (len(b)-5)/4 {
		return Shared{}, fmt.Errorf("blob: a pool of %d parts in %d bytes", n, len(b))
	}
	s := Shared{ends: b[5 : 5+4*n], data: b[5+4*n:]}
	if n > 0 && int(binary.LittleEndian.Uint32(s.ends[4*(n-1):])) != len(s.data) {
		return Shared{}, fmt.Errorf("blob: a pool whose parts do not fill it")
	}
	return s, nil
}

func (s Shared) part(i int) ([]byte, bool) {
	// i is compared with the parts there are, not multiplied out, which
	// overflows past 2^61; an end past 2^31 is negative on 32 bits.
	if i < 0 || i >= len(s.ends)/4 {
		return nil, false
	}
	start := 0
	if i > 0 {
		start = int(binary.LittleEndian.Uint32(s.ends[4*(i-1):]))
	}
	end := int(binary.LittleEndian.Uint32(s.ends[4*i:]))
	if start < 0 || start > end || end > len(s.data) {
		return nil, false
	}
	return s.data[start:end:end], true
}

// NewPooledReader starts reading a table whose shared parts are in pool.
func NewPooledReader(b []byte, version byte, pool Shared) (*Reader, error) {
	r, err := NewReader(b, version)
	if err != nil {
		return nil, err
	}
	r.pool = &pool
	return r, nil
}

// Shared reads a part written by Writer.Shared, with read, which must read
// all of it.
func (r *Reader) Shared(read func(*Reader)) {
	part := r.sharedPart()
	if r.err != nil {
		return
	}
	sub := &Reader{b: part, pool: r.pool}
	read(sub)
	if sub.err == nil && len(sub.b) != 0 {
		sub.err = fmt.Errorf("blob: %d bytes of a shared part left over", len(sub.b))
	}
	if sub.err != nil {
		r.err = sub.err
	}
}

// SharedString reads a string written by Writer.SharedString.
func (r *Reader) SharedString() string {
	return str(r.sharedPart())
}

func (r *Reader) sharedPart() []byte {
	i := r.Uint()
	if r.err != nil {
		return nil
	}
	if r.pool == nil {
		r.err = fmt.Errorf("blob: a shared part in a table read without its pool")
		return nil
	}
	part, ok := r.pool.part(i)
	if !ok {
		r.err = fmt.Errorf("blob: shared part %d is not in the pool", i)
		return nil
	}
	return part
}

// SharedNumber reads the number of a part written by Writer.Shared,
// without reading the part, for a reader that reads it later with Read or
// not at all.
func (r *Reader) SharedNumber() int { return r.Uint() }

// Read reads part number i, with read, which must read all of it.
func (s Shared) Read(i int, read func(*Reader)) error {
	part, ok := s.part(i)
	if !ok {
		return fmt.Errorf("blob: shared part %d is not in the pool", i)
	}
	sub := &Reader{b: part, pool: &s}
	read(sub)
	return sub.Err()
}
