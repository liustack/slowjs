// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// fixFastFrames takes and frees the TLS frames of the generated files of
// linux through tlsTake, tlsDrop and tlsSignals of lib/fastframes_linux.go,
// which Go inlines, instead of TLS.Alloc and TLS.Free, which it does not:
//
//	bp := tls.Alloc(48)         bp := tlsTake(tls, 48)
//	                     ->     if bp == 0 {
//	                                bp = tls.Alloc(48)
//	                            }
//
//	tls.Free(48)         ->     if tlsDrop(tls, 48) {
//	                                tlsSignals(tls)
//	                            }
//
// and a tls.Alloc whose result is not used (the frames of fixLazyFrames)
// becomes "if tlsTake(tls, N) == 0 { tls.Alloc(N) }". Every tls.Alloc and
// tls.Free of the file outside the functions of fastFrameSkip must be a
// statement of its own, of one of these forms, or it stops.
//
// tlsTake and tlsDrop do what TLS.Alloc and TLS.Free do on linux, where
// libc keeps one block per slot of the TLS stack: they read the fields of
// libc.TLS, so the text of TLS.Alloc, TLS.Free and of the structs they use,
// in the libc release of go.mod, is pinned by a fingerprint
// (fastFrameSites). Elsewhere libc keeps the stack in segments, and those
// files are not changed.
//
// fixNoDefer writes the frees, so fixFastFrames is a final change (see
// finalChanges), and checks that restoreFastFrames, its exact inverse,
// gives its input back.

var fastFramesF = flag.Bool("fastframes", false, "print the fingerprint of the text of modernc.org/libc that lib/fastframes_linux.go relies on, in the form of fastFrameSites, to update it after a review")

// fastFrameFile reports whether fixFastFrames changes the generated file
// name.
func fastFrameFile(name string) bool {
	return strings.HasPrefix(name, "ccgo_linux_")
}

// fastFrameSkip are the functions whose TLS frames fixFastFrames leaves to
// TLS.Alloc and TLS.Free. Go does not inline tlsTake into JS_CallInternal,
// which is too large, so its frame gains little, and the inlined frees at
// its returns made the interpreter slower: renders of tifa's bench pages
// took 1.6% more time than with JS_CallInternal left as it is.
var fastFrameSkip = []string{"_JS_CallInternal"}

// fastFrameNames are the functions of lib/fastframes_linux.go, which the
// generated files must not have before fixFastFrames.
var fastFrameNames = []string{"tlsTake(", "tlsDrop(", "tlsSignals("}

var (
	// A tls.Alloc or tls.Free in the code, not in a comment.
	reTLSCall = regexp.MustCompile(`(?m)^\t+(?:[^\t/\n][^\n]*)?\btls\.(?:Alloc|Free)\(`)
	// The statements fixFastFrames changes, and what it makes of them.
	reTLSAllocTo  = regexp.MustCompile(`(?m)^(\t+)(\w+) (:?=) tls\.Alloc\(([^()\n]+)\)$`)
	reTLSAllocAll = regexp.MustCompile(`(?m)^(\t+)tls\.Alloc\(([^()\n]+)\)$`)
	reTLSFree     = regexp.MustCompile(`(?m)^(\t+)tls\.Free\(([^()\n]+)\)$`)
	reTakeTo      = regexp.MustCompile(`(?m)^(\t+)(\w+) (:?=) tlsTake\(tls, ([^()\n]+)\)\n(\t+)if (\w+) == 0 \{\n(\t+)(\w+) = tls\.Alloc\(([^()\n]+)\)\n(\t+)\}$`)
	reTakeAll     = regexp.MustCompile(`(?m)^(\t+)if tlsTake\(tls, ([^()\n]+)\) == 0 \{\n(\t+)tls\.Alloc\(([^()\n]+)\)\n(\t+)\}$`)
	reDrop        = regexp.MustCompile(`(?m)^(\t+)if tlsDrop\(tls, ([^()\n]+)\) \{\n(\t+)tlsSignals\(tls\)\n(\t+)\}$`)
)

// fixFastFrames makes the change in b, the generated file name.
func fixFastFrames(b []byte, name string) ([]byte, error) {
	for _, f := range fastFrameNames {
		if bytes.Contains(b, []byte(f)) {
			return nil, fmt.Errorf("fixFastFrames: %s has a call of %s already, restoreFastFrames goes first", name, strings.TrimSuffix(f, "("))
		}
	}
	if !fastFrameFile(name) {
		return b, nil
	}
	if err := checkFastFrameSites(); err != nil {
		return nil, err
	}
	// The functions of fastFrameSkip keep TLS.Alloc and TLS.Free.
	var keep [][2]int
	for _, fn := range fastFrameSkip {
		f, err := funcBody(b, funcHead(fn), fn)
		if err != nil {
			return nil, fmt.Errorf("fixFastFrames: %s: %v", name, err)
		}
		i := bytes.Index(b, f)
		keep = append(keep, [2]int{i, i + len(f)})
	}
	sort.Slice(keep, func(i, j int) bool { return keep[i][0] < keep[j][0] })
	var n, made int
	var out []byte
	at := 0
	for _, k := range append(keep, [2]int{len(b), len(b)}) {
		seg := b[at:k[0]]
		n += len(reTLSCall.FindAll(seg, -1))
		seg = reTLSAllocTo.ReplaceAllFunc(seg, func(m []byte) []byte {
			made++
			s := reTLSAllocTo.FindSubmatch(m)
			ind, v, op, size := s[1], s[2], s[3], s[4]
			return fmt.Appendf(nil, "%s%s %s tlsTake(tls, %s)\n%sif %s == 0 {\n%s\t%s = tls.Alloc(%s)\n%s}", ind, v, op, size, ind, v, ind, v, size, ind)
		})
		seg = reTLSAllocAll.ReplaceAllFunc(seg, func(m []byte) []byte {
			made++
			s := reTLSAllocAll.FindSubmatch(m)
			ind, size := s[1], s[2]
			return fmt.Appendf(nil, "%sif tlsTake(tls, %s) == 0 {\n%s\ttls.Alloc(%s)\n%s}", ind, size, ind, size, ind)
		})
		seg = reTLSFree.ReplaceAllFunc(seg, func(m []byte) []byte {
			made++
			s := reTLSFree.FindSubmatch(m)
			ind, size := s[1], s[2]
			return fmt.Appendf(nil, "%sif tlsDrop(tls, %s) {\n%s\ttlsSignals(tls)\n%s}", ind, size, ind, ind)
		})
		out = append(out, seg...)
		out = append(out, b[k[0]:k[1]]...)
		at = k[1]
	}
	if made != n {
		return nil, fmt.Errorf("fixFastFrames: %s has %d calls of tls.Alloc and tls.Free, %d of them statements of a form it knows: read the others (see docs/fork-changes/calls-and-tls-frames.md)", name, n, made)
	}
	if n == 0 {
		return nil, fmt.Errorf("fixFastFrames: %s has no TLS frame", name)
	}
	back, err := restoreFastFrames(out, name)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixFastFrames: restoreFastFrames does not give %s back", name)
	}
	return out, nil
}

// restoreFastFrames undoes fixFastFrames in b, the generated file name. A
// file without its change comes back as it is.
func restoreFastFrames(b []byte, name string) ([]byte, error) {
	if !bytes.Contains(b, []byte("tlsTake(")) && !bytes.Contains(b, []byte("tlsDrop(")) {
		return b, nil
	}
	if !fastFrameFile(name) {
		return nil, fmt.Errorf("restoreFastFrames: %s has a change of fixFastFrames, which changes only the files of linux", name)
	}
	var bad error
	out := reTakeTo.ReplaceAllFunc(b, func(m []byte) []byte {
		s := reTakeTo.FindSubmatch(m)
		ind, v, op, size := s[1], s[2], s[3], s[4]
		if !bytes.Equal(s[5], ind) || !bytes.Equal(s[6], v) || !bytes.Equal(s[7], append(bytes.Clone(ind), '\t')) || !bytes.Equal(s[8], v) || !bytes.Equal(s[9], size) || !bytes.Equal(s[10], ind) {
			bad = fmt.Errorf("restoreFastFrames: %s: %q is not as fixFastFrames writes it", name, m)
			return m
		}
		return fmt.Appendf(nil, "%s%s %s tls.Alloc(%s)", ind, v, op, size)
	})
	out = reTakeAll.ReplaceAllFunc(out, func(m []byte) []byte {
		s := reTakeAll.FindSubmatch(m)
		ind, size := s[1], s[2]
		if !bytes.Equal(s[3], append(bytes.Clone(ind), '\t')) || !bytes.Equal(s[4], size) || !bytes.Equal(s[5], ind) {
			bad = fmt.Errorf("restoreFastFrames: %s: %q is not as fixFastFrames writes it", name, m)
			return m
		}
		return fmt.Appendf(nil, "%stls.Alloc(%s)", ind, size)
	})
	out = reDrop.ReplaceAllFunc(out, func(m []byte) []byte {
		s := reDrop.FindSubmatch(m)
		ind, size := s[1], s[2]
		if !bytes.Equal(s[3], append(bytes.Clone(ind), '\t')) || !bytes.Equal(s[4], ind) {
			bad = fmt.Errorf("restoreFastFrames: %s: %q is not as fixFastFrames writes it", name, m)
			return m
		}
		return fmt.Appendf(nil, "%stls.Free(%s)", ind, size)
	})
	if bad != nil {
		return nil, bad
	}
	for _, f := range fastFrameNames {
		if bytes.Contains(out, []byte(f)) {
			return nil, fmt.Errorf("restoreFastFrames: %s has a call of %s in a form fixFastFrames does not write", name, strings.TrimSuffix(f, "("))
		}
	}
	return out, nil
}

// fastFrameText is the text of libc that tlsTake and tlsDrop repeat or
// rely on, in libc_musl.go: the TLS struct, the slot struct, and the
// methods that take, free and count the slots.
var fastFrameText = []string{
	"type tlsStackSlot struct {",
	"type TLS struct {",
	"func (tls *TLS) StackSlots() int {",
	"func (tls *TLS) Alloc(n0 int) (r uintptr) {",
	"func (tls *TLS) Free(n int) {",
}

var fastFrameOnce struct {
	sync.Once
	fp  string
	err error
}

// fastFrameFingerprint returns the fingerprint of fastFrameText in the libc
// release of go.mod, read from the module cache. It is computed once.
func fastFrameFingerprint() (string, error) {
	fastFrameOnce.Do(func() {
		fastFrameOnce.fp, fastFrameOnce.err = libcFingerprint()
	})
	return fastFrameOnce.fp, fastFrameOnce.err
}

func libcFingerprint() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Version}} {{.Dir}}", "modernc.org/libc").Output()
	if err != nil {
		return "", fmt.Errorf("fixFastFrames: go list -m modernc.org/libc: %v", err)
	}
	version, dir, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !ok || dir == "" {
		return "", fmt.Errorf("fixFastFrames: modernc.org/libc %s is not in the module cache, run go mod download", version)
	}
	src, err := os.ReadFile(filepath.Join(dir, "libc_musl.go"))
	if err != nil {
		return "", fmt.Errorf("fixFastFrames: %v", err)
	}
	return libcTextFingerprint(src)
}

// libcTextFingerprint returns the fingerprint of fastFrameText in src, the
// text of libc_musl.go: each declaration from its first line to the
// closing brace at the start of a line.
func libcTextFingerprint(src []byte) (string, error) {
	h := sha256.New()
	for _, head := range fastFrameText {
		i := bytes.Index(src, []byte("\n"+head+"\n"))
		if i < 0 || bytes.Count(src, []byte("\n"+head+"\n")) != 1 {
			return "", fmt.Errorf("fixFastFrames: libc_musl.go of modernc.org/libc does not have %q once", head)
		}
		j := bytes.Index(src[i+1:], []byte("\n}\n"))
		if j < 0 {
			return "", fmt.Errorf("fixFastFrames: the end of %q in libc_musl.go", head)
		}
		h.Write(src[i+1 : i+1+j+3])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// fastFrameReport prints fastFrameSites as the libc release of go.mod
// gives it.
func fastFrameReport(func(fn string) ([]byte, error)) error {
	fp, err := fastFrameFingerprint()
	if err != nil {
		return err
	}
	fmt.Printf("var fastFrameSites = %q\n", fp)
	return nil
}

// checkFastFrameSites checks the text of libc that lib/fastframes_linux.go
// relies on against fastFrameSites.
func checkFastFrameSites() error {
	got, err := fastFrameFingerprint()
	if err != nil {
		return err
	}
	if got != fastFrameSites && !*fastFramesF {
		return fmt.Errorf("fixFastFrames: TLS.Alloc, TLS.Free, StackSlots or the TLS structs of modernc.org/libc have the fingerprint %s, fastFrameSites has %q: read them against lib/fastframes_linux.go, tlsTake and tlsDrop must still do what TLS.Alloc and TLS.Free do, then put the new fingerprint in (go run ./internal/fixlib -fastframes prints it)", got, fastFrameSites)
	}
	return nil
}
