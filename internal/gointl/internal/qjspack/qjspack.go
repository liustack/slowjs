// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package qjspack is the layout of the go-intl data quickjs embeds: the files
// of go-intl's data directory, grouped into chunks, each chunk stored as it is
// or compressed with DEFLATE.
//
// A program embeds a pack as a string. Nothing is decoded until a file is
// asked for: the records are searched where they lie, a stored chunk is served
// where it lies, and a compressed one is inflated into the Go heap the first
// time one of its files is read, once for the process, so every VM shares it.
//
// The layout, every number a little-endian uint32:
//
//	"QJP1"
//	the number of files, the number of chunks
//	per file, sorted by name: where its name starts and ends in the pack,
//	    its chunk, where it starts and ends in the chunk's bytes
//	per chunk: its method (0 stored, 1 DEFLATE), its size once inflated,
//	    where its data starts and ends in the pack
//	the names, then the chunks' data
package qjspack // import "github.com/liustack/quickjs/internal/gointl/internal/qjspack"

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	magic      = "QJP1"
	headerSize = len(magic) + 8
	fileSize   = 20
	chunkSize  = 16

	stored  = 0
	deflate = 1

	// maxRatio is the most DEFLATE inflates a byte to: a match of 258
	// bytes, in two bits at the shortest, is 1032 bytes for every byte of
	// the stream.
	maxRatio = 1032
)

// A File is one file of the data directory, named by its slash-separated
// path, as go-intl names it: "numbers/de.bin".
type File struct {
	Name string
	Data []byte
}

// A Chunk is files that are inflated together.
type Chunk struct {
	// Deflate compresses the chunk. A chunk that is not compressed is
	// served from the binary's read-only data.
	Deflate bool
	Files   []File
}

// Build lays chunks out as a pack. The same chunks give the same bytes.
func Build(chunks []Chunk) ([]byte, error) {
	type rec struct {
		name     string
		chunk    int
		off, end int
	}
	var files []rec
	datas := make([][]byte, len(chunks))
	raws := make([]int, len(chunks))
	seen := map[string]bool{}
	for i, c := range chunks {
		var raw []byte
		for _, f := range c.Files {
			if seen[f.Name] {
				return nil, fmt.Errorf("qjspack: %s is given twice", f.Name)
			}
			seen[f.Name] = true
			files = append(files, rec{f.Name, i, len(raw), len(raw) + len(f.Data)})
			raw = append(raw, f.Data...)
		}
		raws[i] = len(raw)
		datas[i] = raw
		if c.Deflate {
			var b bytes.Buffer
			w, err := flate.NewWriter(&b, flate.BestCompression)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(raw); err != nil {
				return nil, err
			}
			if err := w.Close(); err != nil {
				return nil, err
			}
			datas[i] = b.Bytes()
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	namesAt := headerSize + fileSize*len(files) + chunkSize*len(chunks)
	size := namesAt
	for _, f := range files {
		size += len(f.name)
	}
	dataAt := size
	for _, d := range datas {
		size += len(d)
	}
	if uint64(size) > 1<<32-1 {
		return nil, fmt.Errorf("qjspack: %d bytes do not fit a pack", size)
	}

	out := make([]byte, 0, size)
	u32 := func(v int) { out = binary.LittleEndian.AppendUint32(out, uint32(v)) }
	out = append(out, magic...)
	u32(len(files))
	u32(len(chunks))
	at := namesAt
	for _, f := range files {
		u32(at)
		at += len(f.name)
		u32(at)
		u32(f.chunk)
		u32(f.off)
		u32(f.end)
	}
	at = dataAt
	for i, c := range chunks {
		method := stored
		if c.Deflate {
			method = deflate
		}
		u32(method)
		u32(raws[i])
		u32(at)
		at += len(datas[i])
		u32(at)
	}
	for _, f := range files {
		out = append(out, f.name...)
	}
	for _, d := range datas {
		out = append(out, d...)
	}
	return out, nil
}

// A Pack is a pack read where it lies. It is safe for concurrent use.
type Pack struct {
	s      string
	nfiles int
	chunks []chunk
	// inflated counts the chunks inflated so far and their bytes.
	inflated, inflatedBytes atomic.Int64
}

type chunk struct {
	once sync.Once
	b    []byte
	err  error
}

func u32(s string, at int) int {
	_ = s[at+3]
	return int(uint32(s[at]) | uint32(s[at+1])<<8 | uint32(s[at+2])<<16 | uint32(s[at+3])<<24)
}

// Open checks a pack's layout: that every record points inside it.
func Open(s string) (*Pack, error) {
	if len(s) < headerSize || s[:len(magic)] != magic {
		return nil, fmt.Errorf("qjspack: not a pack")
	}
	nfiles, nchunks := u32(s, 4), u32(s, 8)
	// Compared, not multiplied out, which overflows on 32 bits.
	if nfiles < 0 || nchunks < 0 || nfiles > (len(s)-headerSize)/fileSize ||
		nchunks > (len(s)-headerSize-fileSize*nfiles)/chunkSize {
		return nil, fmt.Errorf("qjspack: %d files and %d chunks in %d bytes", nfiles, nchunks, len(s))
	}
	p := &Pack{s: s, nfiles: nfiles, chunks: make([]chunk, nchunks)}
	for i := 0; i < nchunks; i++ {
		method, raw, start, end := p.chunk(i)
		// A compressed chunk inflates to no more than DEFLATE can make of
		// it, which File allocates at once.
		if method != stored && method != deflate || start < 0 || start > end || end > len(s) ||
			raw < 0 || method == stored && end-start != raw || raw/maxRatio > end-start {
			return nil, fmt.Errorf("qjspack: chunk %d is malformed", i)
		}
	}
	prev := ""
	for i := 0; i < nfiles; i++ {
		at := headerSize + fileSize*i
		ns, ne, c, off, end := u32(s, at), u32(s, at+4), u32(s, at+8), u32(s, at+12), u32(s, at+16)
		if ns < 0 || ns > ne || ne > len(s) || c < 0 || c >= nchunks {
			return nil, fmt.Errorf("qjspack: file %d is malformed", i)
		}
		if _, raw, _, _ := p.chunk(c); off < 0 || off > end || end > raw {
			return nil, fmt.Errorf("qjspack: file %d is malformed", i)
		}
		if name := s[ns:ne]; i > 0 && name <= prev {
			return nil, fmt.Errorf("qjspack: the files are not sorted at %q", name)
		} else {
			prev = name
		}
	}
	return p, nil
}

func (p *Pack) chunk(i int) (method, raw, start, end int) {
	at := headerSize + fileSize*p.nfiles + chunkSize*i
	return u32(p.s, at), u32(p.s, at+4), u32(p.s, at+8), u32(p.s, at+12)
}

func (p *Pack) name(i int) string {
	at := headerSize + fileSize*i
	return p.s[u32(p.s, at):u32(p.s, at+4)]
}

// Names lists the files, sorted.
func (p *Pack) Names() []string {
	names := make([]string, p.nfiles)
	for i := range names {
		names[i] = p.name(i)
	}
	return names
}

// File returns a file's bytes, inflating its chunk the first time. The bytes
// may be read-only memory and are never to be changed. A file the pack does
// not have is fs.ErrNotExist.
func (p *Pack) File(name string) ([]byte, error) {
	i := sort.Search(p.nfiles, func(i int) bool { return p.name(i) >= name })
	if i == p.nfiles || p.name(i) != name {
		return nil, fs.ErrNotExist
	}
	at := headerSize + fileSize*i
	c, off, end := u32(p.s, at+8), u32(p.s, at+12), u32(p.s, at+16)
	b, err := p.inflate(c)
	if err != nil {
		return nil, err
	}
	return b[off:end:end], nil
}

func (p *Pack) inflate(i int) ([]byte, error) {
	method, raw, start, end := p.chunk(i)
	if method == stored {
		if raw == 0 {
			return []byte{}, nil
		}
		return unsafe.Slice(unsafe.StringData(p.s[start:end]), raw), nil
	}
	c := &p.chunks[i]
	c.once.Do(func() {
		b := make([]byte, raw)
		r := flate.NewReader(strings.NewReader(p.s[start:end]))
		if _, err := io.ReadFull(r, b); err != nil {
			c.err = fmt.Errorf("qjspack: chunk %d: %v", i, err)
			return
		}
		// The stream has to end where the chunk says it does.
		if n, err := r.Read(make([]byte, 1)); n != 0 || err != io.EOF {
			c.err = fmt.Errorf("qjspack: chunk %d is longer than %d bytes", i, raw)
			return
		}
		c.b = b
		p.inflated.Add(1)
		p.inflatedBytes.Add(int64(raw))
	})
	return c.b, c.err
}

// Inflated reports how many chunks have been inflated so far and the bytes
// they take up in the heap.
func (p *Pack) Inflated() (chunks, bytes int) {
	return int(p.inflated.Load()), int(p.inflatedBytes.Load())
}
