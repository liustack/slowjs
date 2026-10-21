// Package datapack lays out a directory of data files as one pack, which
// the intl package embeds as a string and reads in place.
//
// go:embed puts a directory into an embed.FS, whose ReadFile copies a file
// out on every call; a formatter reads a dozen files as it is built, and
// nothing may be cached, so the copying was most of what building one cost.
// A single file embeds as a string, which is read where it lies. So the
// data directory, which the generators write and a caller can serve with
// NewFS, is also packed into one file, and that is what is embedded.
//
// A pack is the magic "IPAK", a version byte, the number of files, then a
// record of four little-endian uint32s per file -- where its name starts
// and ends, where its data starts and ends -- sorted by name, then the
// names, then the data. A file is found by a binary search of the records.
package datapack

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"sort"
)

// Version is the layout's version.
const Version = 1

const (
	magic      = "IPAK"
	headerSize = len(magic) + 1 + 4
	recordSize = 16
)

// Build packs every file under fsys, named by its slash-separated path.
func Build(fsys fs.FS) ([]byte, error) {
	var names []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	files := make([][]byte, len(names))
	nameBytes, dataBytes := 0, 0
	for i, name := range names {
		if files[i], err = fs.ReadFile(fsys, name); err != nil {
			return nil, err
		}
		nameBytes += len(name)
		dataBytes += len(files[i])
	}
	namesAt := headerSize + recordSize*len(names)
	dataAt := namesAt + nameBytes
	if uint64(dataAt)+uint64(dataBytes) > 1<<32-1 {
		return nil, fmt.Errorf("datapack: %d bytes do not fit a pack", dataAt+dataBytes)
	}
	out := make([]byte, 0, dataAt+dataBytes)
	out = append(out, magic...)
	out = append(out, Version)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(names)))
	nameEnd, dataEnd := namesAt, dataAt
	for i, name := range names {
		out = binary.LittleEndian.AppendUint32(out, uint32(nameEnd))
		nameEnd += len(name)
		out = binary.LittleEndian.AppendUint32(out, uint32(nameEnd))
		out = binary.LittleEndian.AppendUint32(out, uint32(dataEnd))
		dataEnd += len(files[i])
		out = binary.LittleEndian.AppendUint32(out, uint32(dataEnd))
	}
	for _, name := range names {
		out = append(out, name...)
	}
	for _, f := range files {
		out = append(out, f...)
	}
	return out, nil
}

// Find returns a file's bounds in a pack: where its data starts and ends.
// It reads the pack where it lies and allocates nothing.
func Find(pack string, name string) (start, end int, ok bool) {
	if len(pack) < headerSize || pack[:len(magic)] != magic || pack[len(magic)] != Version {
		return 0, 0, false
	}
	// A count past what the pack holds, compared without multiplying it
	// out, which overflows on 32 bits, as does a count past 2^31.
	n := int(u32(pack, len(magic)+1))
	if n < 0 || n > (len(pack)-headerSize)/recordSize {
		return 0, 0, false
	}
	lo, hi := 0, n
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		at := headerSize + recordSize*mid
		nameStart, nameEnd := int(u32(pack, at)), int(u32(pack, at+4))
		if nameStart < 0 || nameStart > nameEnd || nameEnd > len(pack) {
			return 0, 0, false
		}
		switch c := compare(pack[nameStart:nameEnd], name); {
		case c == 0:
			start, end = int(u32(pack, at+8)), int(u32(pack, at+12))
			if start < 0 || start > end || end > len(pack) {
				return 0, 0, false
			}
			return start, end, true
		case c < 0:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return 0, 0, false
}

func u32(s string, at int) uint32 {
	return uint32(s[at]) | uint32(s[at+1])<<8 | uint32(s[at+2])<<16 | uint32(s[at+3])<<24
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
