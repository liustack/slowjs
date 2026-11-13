//go:build gointl_all

package datapack

import (
	"testing"
	"testing/fstest"
)

// A pack finds each file it holds, and nothing else.
func TestBuildAndFind(t *testing.T) {
	files := fstest.MapFS{
		"a.bin":     {Data: []byte("alpha")},
		"dir/b.bin": {Data: []byte("beta")},
		"empty.bin": {Data: nil},
	}
	pack, err := Build(files)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range files {
		start, end, ok := Find(string(pack), name)
		if !ok || string(pack[start:end]) != string(f.Data) {
			t.Errorf("%s: %q, %v", name, pack[start:end], ok)
		}
	}
	for _, name := range []string{"", "b.bin", "dir", "z.bin"} {
		if _, _, ok := Find(string(pack), name); ok {
			t.Errorf("%s found", name)
		}
	}
}

// A corrupt pack is no pack, on 32 bits as on 64: a count of 0x40000000
// had multiplied out to 0 and passed the length check, and an offset past
// 2^31 had read as negative and panicked (ISSUES.md DA-3).
func TestCorruptPack(t *testing.T) {
	pack, err := Build(fstest.MapFS{"a.bin": {Data: []byte("alpha")}})
	if err != nil {
		t.Fatal(err)
	}
	put := func(b []byte, at int, v uint32) []byte {
		c := append([]byte(nil), b...)
		c[at], c[at+1], c[at+2], c[at+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
		return c
	}
	for name, b := range map[string][]byte{
		"count 0x40000000": put(pack, len(magic)+1, 0x40000000),
		"count 0x80000000": put(pack, len(magic)+1, 0x80000000),
		"name at 2^31":     put(pack, headerSize, 0x80000000),
		"data at 2^31":     put(pack, headerSize+8, 0x80000000),
		"truncated":        pack[:headerSize+recordSize-1],
	} {
		if _, _, ok := Find(string(b), "a.bin"); ok {
			t.Errorf("%s: found", name)
		}
	}
}
