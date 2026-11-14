// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package qjspack

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
)

// A pack compressed by another encoder is the same data: Go releases differ
// in how DEFLATE encodes, and the check of qjsdata must not.
func TestSameIgnoresHowAChunkIsCompressed(t *testing.T) {
	chunks := testChunks()
	a, err := Build(chunks)
	if err != nil {
		t.Fatal(err)
	}
	b := rebuildWithLevel(t, chunks, flate.BestSpeed)
	if bytes.Equal(a, b) {
		t.Fatal("the two encoders give the same bytes: the test shows nothing")
	}
	if err := Same(a, b); err != nil {
		t.Errorf("Same: %v", err)
	}
	if err := Same(a, a); err != nil {
		t.Errorf("Same of a pack with itself: %v", err)
	}
}

func TestSameSeesADifferenceInTheData(t *testing.T) {
	base := testChunks()
	a, err := Build(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]Chunk){
		"a byte of a compressed file": func(c []Chunk) { c[0].Files[1].Data = []byte("fr numbers!") },
		"a byte of a stored file":     func(c []Chunk) { c[1].Files[0].Data = []byte("stored as it is") },
		"a name":                      func(c []Chunk) { c[1].Files[0].Name = "likelysubtags2.bin" },
		"the method of a chunk":       func(c []Chunk) { c[2].Deflate = false },
		"a file moved to another chunk": func(c []Chunk) {
			c[1].Files = append(c[1].Files, c[2].Files[0])
			c[2].Files = nil
		},
	} {
		chunks := testChunks()
		mutate(chunks)
		b, err := Build(chunks)
		if err != nil {
			t.Fatal(err)
		}
		if err := Same(a, b); err == nil {
			t.Errorf("Same does not see %s", name)
		}
		if err := Same(b, rebuildWithLevel(t, chunks, flate.BestSpeed)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// rebuildWithLevel builds a pack as Build does, with another DEFLATE level, by
// building it with Build and putting each compressed chunk back in at the level.
func rebuildWithLevel(t *testing.T, chunks []Chunk, level int) []byte {
	t.Helper()
	b, err := Build(chunks)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Open(string(b))
	if err != nil {
		t.Fatal(err)
	}
	// The data of the chunks after the names, recompressed one by one.
	var data []byte
	out := append([]byte(nil), b[:headerSize+fileSize*p.nfiles+chunkSize*len(p.chunks)]...)
	_, _, firstStart, _ := p.chunk(0)
	out = append(out, b[len(out):firstStart]...)
	for i := range p.chunks {
		method, _, start, end := p.chunk(i)
		d := []byte(b[start:end])
		if method == deflate {
			raw, err := p.inflate(i)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			w, _ := flate.NewWriter(&buf, level)
			w.Write(raw)
			w.Close()
			d = buf.Bytes()
		}
		at := headerSize + fileSize*p.nfiles + chunkSize*i
		binary.LittleEndian.PutUint32(out[at+8:], uint32(firstStart+len(data)))
		binary.LittleEndian.PutUint32(out[at+12:], uint32(firstStart+len(data)+len(d)))
		data = append(data, d...)
	}
	return append(out, data...)
}

func testChunks() []Chunk {
	return []Chunk{
		{Deflate: true, Files: []File{
			{Name: "numbers/de.bin", Data: []byte(strings.Repeat("de numbers ", 50))},
			{Name: "numbers/fr.bin", Data: []byte("fr numbers")},
			{Name: "numbers/empty.bin", Data: []byte{}},
		}},
		{Files: []File{{Name: "likelysubtags.bin", Data: []byte("stored as is")}}},
		{Deflate: true, Files: []File{{Name: "numbersshared.bin", Data: bytes.Repeat([]byte{1, 2, 3, 4}, 1000)}}},
	}
}

func mustOpen(t *testing.T, chunks []Chunk) *Pack {
	t.Helper()
	b, err := Build(chunks)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Open(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFilesReadBack(t *testing.T) {
	p := mustOpen(t, testChunks())
	for _, c := range testChunks() {
		for _, f := range c.Files {
			got, err := p.File(f.Name)
			if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			if !bytes.Equal(got, f.Data) {
				t.Errorf("%s: got %q, want %q", f.Name, got, f.Data)
			}
			if got == nil {
				t.Errorf("%s: nil, want an empty slice", f.Name)
			}
		}
	}
	if _, err := p.File("numbers/it.bin"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v, want fs.ErrNotExist", err)
	}
	want := []string{"likelysubtags.bin", "numbers/de.bin", "numbers/empty.bin", "numbers/fr.bin", "numbersshared.bin"}
	if got := p.Names(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Names() = %q, want %q", got, want)
	}
}

// A chunk is inflated the first time one of its files is read, and only
// then, and once for the process: later reads share the bytes.
func TestChunksInflateOnFirstUseOnce(t *testing.T) {
	p := mustOpen(t, testChunks())
	if n, size := p.Inflated(); n != 0 || size != 0 {
		t.Fatalf("before any read: %d chunks, %d bytes inflated", n, size)
	}
	a, _ := p.File("numbers/de.bin")
	n, size := p.Inflated()
	if n != 1 || size != len(strings.Repeat("de numbers ", 50))+len("fr numbers") {
		t.Fatalf("after one read: %d chunks, %d bytes inflated", n, size)
	}
	// A stored chunk is served where it lies.
	p.File("likelysubtags.bin")
	if n2, size2 := p.Inflated(); n2 != n || size2 != size {
		t.Fatalf("a stored chunk inflated: %d chunks, %d bytes", n2, size2)
	}
	b, _ := p.File("numbers/de.bin")
	if &a[0] != &b[0] {
		t.Error("a second read inflated the chunk again")
	}
	fr, _ := p.File("numbers/fr.bin")
	if n3, _ := p.Inflated(); n3 != 1 {
		t.Errorf("a file of the same chunk inflated it again: %d chunks", n3)
	}
	// A file's slice cannot be appended into its neighbour.
	if cap(fr) != len(fr) {
		t.Errorf("cap %d, len %d", cap(fr), len(fr))
	}
}

func TestConcurrentFirstUse(t *testing.T) {
	p := mustOpen(t, testChunks())
	var wg sync.WaitGroup
	got := make([][]byte, 16)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], _ = p.File("numbersshared.bin")
		}()
	}
	wg.Wait()
	for i := range got {
		if &got[i][0] != &got[0][0] {
			t.Fatal("goroutines got different copies")
		}
	}
	if n, _ := p.Inflated(); n != 1 {
		t.Errorf("%d chunks inflated, want 1", n)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	a, err := Build(testChunks())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Build(testChunks())
	if !bytes.Equal(a, b) {
		t.Error("two builds of the same chunks differ")
	}
}

func TestBuildRefusesDuplicateNames(t *testing.T) {
	_, err := Build([]Chunk{
		{Files: []File{{Name: "a.bin", Data: []byte("1")}}},
		{Deflate: true, Files: []File{{Name: "a.bin", Data: []byte("2")}}},
	})
	if err == nil {
		t.Fatal("no error for a name given twice")
	}
}

func TestOpenRefusesBadPacks(t *testing.T) {
	good, err := Build(testChunks())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"",
		"QJP",
		"XXXX" + string(good[4:]),
		string(good[:20]),
		string(good[:len(good)-1]),
	} {
		if _, err := Open(bad); err == nil {
			t.Errorf("Open of %d bytes: no error", len(bad))
		}
	}
}

// Damaged compressed data is an error from File, for every caller, not a
// panic.
func TestFileReportsDamagedChunks(t *testing.T) {
	b, err := Build([]Chunk{{Deflate: true, Files: []File{{Name: "a.bin", Data: bytes.Repeat([]byte("abc"), 100)}}}})
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-3] ^= 0xff
	b[len(b)-6] ^= 0xff
	p, err := Open(string(b))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := p.File("a.bin"); err == nil {
			t.Error("no error from a damaged chunk")
		}
	}
}

// A compressed chunk says how many bytes it inflates to, which File
// allocates: Open refuses more than DEFLATE can inflate the chunk to, so that
// a damaged size cannot make File take gigabytes.
func TestOpenRefusesChunksThatInflateTooFar(t *testing.T) {
	b, err := Build([]Chunk{{Deflate: true, Files: []File{{Name: "a.bin", Data: bytes.Repeat([]byte("abc"), 100)}}}})
	if err != nil {
		t.Fatal(err)
	}
	raw := headerSize + fileSize + 4
	b[raw], b[raw+1], b[raw+2], b[raw+3] = 0xff, 0xff, 0xff, 0x7f
	if _, err := Open(string(b)); err == nil {
		t.Error("Open of a chunk that says it inflates to 2 GB: no error")
	}
}

// Zeros compress about as far as DEFLATE goes, and still open.
func TestOpenTakesTheMostCompressedChunks(t *testing.T) {
	p := mustOpen(t, []Chunk{{Deflate: true, Files: []File{{Name: "zeros.bin", Data: make([]byte, 4<<20)}}}})
	if b, err := p.File("zeros.bin"); err != nil || len(b) != 4<<20 {
		t.Errorf("%d bytes, %v", len(b), err)
	}
}
