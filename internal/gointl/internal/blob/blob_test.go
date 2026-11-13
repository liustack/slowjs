//go:build gointl_all

package blob

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

// A number wider than 32 bits -- the end ICU gives a metazone use that has
// not ended, 9999-12-31 in minutes, or a collation's 32-bit primary -- reads
// back whole through Uint64 on every platform.
func TestUint64RoundTrip(t *testing.T) {
	values := []int64{0, 1, 1<<31 - 1, 1 << 31, 4223371680, 0xFE000000, math.MaxInt64}
	w := NewWriter(1)
	for _, v := range values {
		w.Uint64(v)
	}
	r, err := NewReader(w.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range values {
		if got := r.Uint64(); got != want {
			t.Errorf("Uint64 = %d, want %d", got, want)
		}
	}
	if err := r.Err(); err != nil {
		t.Error(err)
	}
}

// Uint refuses a number an int cannot hold rather than wrapping it round,
// which is what hid two 32-bit bugs.
func TestUintRefusesWhatAnIntCannotHold(t *testing.T) {
	w := NewWriter(1)
	w.Uint64(4223371680)
	r, err := NewReader(w.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Uint()
	if strconv.IntSize == 64 {
		if int64(got) != 4223371680 || r.Err() != nil {
			t.Errorf("Uint = %d, %v on a 64-bit platform", got, r.Err())
		}
		return
	}
	if got != 0 || r.Err() == nil {
		t.Errorf("Uint = %d, %v on a 32-bit platform, want a failure", got, r.Err())
	}
}

// TestSharedRoundTrip writes two tables through one pool and reads them
// back: a part both write is kept once, and each reads what it wrote.
func TestSharedRoundTrip(t *testing.T) {
	pool := NewPool(3)
	write := func(tail string) []byte {
		w := NewPooledWriter(3, pool)
		w.Shared(func(sub *Writer) {
			sub.SharedString("January")
			sub.SharedString("February")
			sub.Uint(7)
		})
		w.SharedString(tail)
		return w.Bytes()
	}
	a, b := write("a"), write("b")
	// January, February, the part, "a", "b".
	if n := len(pool.parts); n != 5 {
		t.Fatalf("the pool has %d parts, want 5", n)
	}
	shared, err := ReadShared(pool.Bytes(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		table []byte
		tail  string
	}{{a, "a"}, {b, "b"}} {
		r, err := NewPooledReader(c.table, 3, shared)
		if err != nil {
			t.Fatal(err)
		}
		var months []string
		var n int
		r.Shared(func(sub *Reader) {
			months = append(months, sub.SharedString(), sub.SharedString())
			n = sub.Uint()
		})
		tail := r.SharedString()
		if err := r.Err(); err != nil {
			t.Fatal(err)
		}
		if months[0] != "January" || months[1] != "February" || n != 7 || tail != c.tail {
			t.Errorf("read %v %d %q, want January February 7 %q", months, n, tail, c.tail)
		}
	}

	// A part read short is an error, as a table read short is.
	r, _ := NewPooledReader(a, 3, shared)
	r.Shared(func(sub *Reader) { sub.SharedString() })
	r.SharedString()
	if r.Err() == nil {
		t.Error("a shared part read short went unnoticed")
	}
	// A number past the pool's end is an error, not a panic.
	w := NewPooledWriter(3, NewPool(3))
	w.Uint(99)
	r, _ = NewPooledReader(w.Bytes(), 3, shared)
	r.SharedString()
	if r.Err() == nil {
		t.Error("a part past the pool's end went unnoticed")
	}
}

// TestIndex finds each record of an index, and nothing else.
func TestIndex(t *testing.T) {
	records := map[string][]byte{"b": []byte("two"), "a": []byte("one"), "c": nil, "ab": []byte("x")}
	b, err := BuildIndex(7, records)
	if err != nil {
		t.Fatal(err)
	}
	x, err := ReadIndex(b, 7)
	if err != nil {
		t.Fatal(err)
	}
	if x.Len() != 4 {
		t.Fatalf("Len = %d, want 4", x.Len())
	}
	for k, want := range records {
		got, ok := x.Find(k)
		if !ok || string(got) != string(want) {
			t.Errorf("Find(%q) = %q, %v; want %q", k, got, ok, want)
		}
	}
	for _, k := range []string{"", "aa", "d", "0"} {
		if _, ok := x.Find(k); ok {
			t.Errorf("Find(%q) found something", k)
		}
	}
	if k, v := x.At(0); string(k) != "a" || string(v) != "one" {
		t.Errorf("At(0) = %q %q", k, v)
	}
}

func TestSharedTable(t *testing.T) {
	pool := NewPool(1)
	values := map[string]string{"b": "two", "a": "one", "c": "", "ab": "one"}
	keys := []string{"b", "a", "c", "ab"}
	write := func(w *Writer) {
		w.SharedTable(keys, func(k string, w *Writer) { w.SharedString(values[k]) })
	}
	w := NewPooledWriter(1, pool)
	write(w)
	// A second table with the same records is the same part.
	w2 := NewPooledWriter(1, pool)
	write(w2)
	if string(w.Bytes()) != string(w2.Bytes()) {
		t.Errorf("the same table was written twice: % x, % x", w.Bytes(), w2.Bytes())
	}
	shared, err := ReadShared(pool.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewPooledReader(w.Bytes(), 1, shared)
	if err != nil {
		t.Fatal(err)
	}
	table := r.SharedTable()
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	if table.Len() != 4 {
		t.Fatalf("Len = %d, want 4", table.Len())
	}
	for k, want := range values {
		v, ok := table.Find(k)
		if !ok {
			t.Errorf("Find(%q) found nothing", k)
			continue
		}
		if got := v.SharedString(); got != want || v.Err() != nil {
			t.Errorf("Find(%q) = %q, %v; want %q", k, got, v.Err(), want)
		}
	}
	for _, k := range []string{"", "aa", "d", "0", "one"} {
		if _, ok := table.Find(k); ok {
			t.Errorf("Find(%q) found something", k)
		}
	}
	if _, ok := (Table{}).Find("a"); ok {
		t.Error("an empty table found something")
	}
}

func TestSharedTableWideNumbers(t *testing.T) {
	// More than 256 parts, so that a record's numbers take two bytes.
	pool := NewPool(1)
	var keys []string
	for i := 0; i < 300; i++ {
		keys = append(keys, fmt.Sprintf("key%03d", i))
	}
	w := NewPooledWriter(1, pool)
	w.SharedTable(keys, func(k string, w *Writer) { w.SharedString("value " + k) })
	shared, err := ReadShared(pool.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewPooledReader(w.Bytes(), 1, shared)
	if err != nil {
		t.Fatal(err)
	}
	table := r.SharedTable()
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	if table.width != 2 || table.Len() != 300 {
		t.Fatalf("a table of %d-byte numbers and %d records", table.width, table.Len())
	}
	for _, k := range keys {
		v, ok := table.Find(k)
		if !ok {
			t.Fatalf("Find(%q) found nothing", k)
		}
		if got := v.SharedString(); got != "value "+k || v.Err() != nil {
			t.Fatalf("Find(%q) = %q, %v", k, got, v.Err())
		}
	}
}

func TestStringsShareTheTable(t *testing.T) {
	pool := NewPool(1)
	w := NewPooledWriter(1, pool)
	w.String("plain")
	w.SharedString("pooled")
	shared, err := ReadShared(pool.Bytes(), 1)
	if err != nil {
		t.Fatal(err)
	}
	data := w.Bytes()
	var plain, pooled string
	allocs := testing.AllocsPerRun(100, func() {
		r := &Reader{b: data[1:], pool: &shared}
		plain, pooled = r.String(), r.SharedString()
	})
	if plain != "plain" || pooled != "pooled" {
		t.Fatalf("read %q and %q", plain, pooled)
	}
	// The reader itself is the only allocation: the strings are the
	// table's own memory.
	if allocs > 1 {
		t.Errorf("reading two strings made %v allocations", allocs)
	}
}

// An index written under another version, or before indexes had one, is
// refused rather than misread (ISSUES.md RU-6).
func TestIndexVersion(t *testing.T) {
	b, err := BuildIndex(7, map[string][]byte{"a": []byte("one")})
	if err != nil {
		t.Fatal(err)
	}
	if b[0] != 7 {
		t.Fatalf("the index begins %d, not its version", b[0])
	}
	if _, err := ReadIndex(b, 8); err == nil {
		t.Error("version 7 read as 8")
	}
	// The layout before the version byte: the count first.
	if _, err := ReadIndex(b[1:], 7); err == nil {
		t.Error("an index without a version read")
	}
	if _, err := ReadIndex(nil, 7); err == nil {
		t.Error("an empty index read")
	}
}

// A part number too large to multiply out is not in the pool: 4*i+4 had
// overflowed to a small number for i at 2^61 and indexed out of range
// (ISSUES.md DA-1).
func TestHugePartNumber(t *testing.T) {
	p := NewPool(3)
	w := NewPooledWriter(3, p)
	w.SharedString("one")
	shared, err := ReadShared(p.Bytes(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{math.MaxInt/4 + 1, math.MaxInt / 4, math.MaxInt, 1} {
		if part, ok := shared.part(i); ok {
			t.Errorf("part %d: %q", i, part)
		}
	}
	if part, ok := shared.part(0); !ok || string(part) != "one" {
		t.Errorf("part 0: %q, %v", part, ok)
	}
}

// Counts and offsets read from corrupt data are refused on 32 bits as on
// 64: a count of 0x40000000 had multiplied out to 0 and passed the length
// check, and an offset past 2^31 had read as negative (ISSUES.md DA-3).
func TestCorruptCountsAndOffsets(t *testing.T) {
	le := func(b []byte, v uint32) []byte { return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }

	pool := le([]byte{3}, 0x40000000)
	pool = append(pool, 0, 0, 0, 0)
	if _, err := ReadShared(pool, 3); err == nil {
		t.Error("a pool of 0x40000000 parts in 9 bytes read")
	}
	index := le([]byte{3}, 0x40000000)
	index = append(index, 0, 0, 0, 0)
	if _, err := ReadIndex(index, 3); err == nil {
		t.Error("an index of 0x40000000 records in 9 bytes read")
	}

	// One part whose end is past 2^31: the layout check catches it.
	pool = le(le([]byte{3}, 1), 0x80000000)
	if _, err := ReadShared(pool, 3); err == nil {
		t.Error("a part ending at 2^31 read")
	}
	// Two, the first ending past 2^31 and the last at the end: only the
	// part reads it.
	pool = le(le(le([]byte{3}, 2), 0x80000000), 1)
	pool = append(pool, 'x')
	s, err := ReadShared(pool, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if part, ok := s.part(i); ok {
			t.Errorf("part %d: %q", i, part)
		}
	}
	index = le(le(le([]byte{3}, 2), 0xffffffff), 1)
	index = append(index, 0)
	x, err := ReadIndex(index, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if k, v := x.At(i); k != nil || v != nil {
			t.Errorf("record %d: %q %q", i, k, v)
		}
	}
}
