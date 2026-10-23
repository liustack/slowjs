// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
	"github.com/liustack/slowjs/internal/gointl/internal/layout"
)

// cutCorpus takes out of go-intl's test corpora in dir (its testdata) the
// cases whose locale is in a language the packs leave out, or, but for the
// plural rules' corpora, keep the plural rules of only, and those whose
// locale a service has in go-intl's lists, which are Node's, and not in
// Chromium's. This only filters locales, not differing resources within a
// locale. It rewrites the files in place and says what it took out of each.
func cutCorpus(dir string, dropped, pluralOnly []string, unlisted map[string]bool) error {
	drop, plural := map[string]bool{}, map[string]bool{}
	for _, l := range dropped {
		drop[l] = true
	}
	for _, l := range pluralOnly {
		plural[l] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".txt") && !strings.HasSuffix(name, ".txt.gz") && !strings.HasSuffix(name, ".json") {
			continue
		}
		file := filepath.Join(dir, name)
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		zipped := strings.HasSuffix(name, ".gz")
		if zipped {
			r, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				return fmt.Errorf("%s: %v", name, err)
			}
			if b, err = io.ReadAll(r); err != nil {
				return fmt.Errorf("%s: %v", name, err)
			}
		}
		pluralCorpus := strings.Contains(name, "plural")
		lines := strings.Split(string(b), "\n")
		var kept []string
		for _, line := range lines {
			if loc, ok := corpusLocale(line); ok {
				l := strings.ToLower(languageOf(loc))
				if drop[l] || !pluralCorpus && plural[l] || unlisted[strings.ToLower(strings.ReplaceAll(loc, "_", "-"))] {
					continue
				}
			}
			kept = append(kept, line)
		}
		if len(kept) == len(lines) {
			continue
		}
		out := []byte(strings.Join(kept, "\n"))
		if zipped {
			var buf bytes.Buffer
			w := gzip.NewWriter(&buf)
			w.Write(out)
			if err := w.Close(); err != nil {
				return err
			}
			out = buf.Bytes()
		}
		if err := os.WriteFile(file, out, 0o644); err != nil {
			return err
		}
		fmt.Printf("%s: took out %d of %d lines\n", name, len(lines)-len(kept), len(lines))
	}
	return nil
}

var (
	corpusTag  = regexp.MustCompile(`^[A-Za-z]{2,3}([-_][A-Za-z0-9]{1,8})*$`)
	corpusCall = regexp.MustCompile(`(?:Intl\.\w+|toLocale\w*)\(\s*"([^"]+)"`)
)

// corpusLocale finds the locale of a line of a corpus, in the forms
// go-intl's corpora have: "case\t<locale>\t...", a JSON array with the locale
// among its first two items, a JS expression, or a bare tag.
func corpusLocale(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "#"):
		return "", false
	case strings.HasPrefix(line, "case\t"):
		f := strings.Split(line, "\t")
		return f[1], true
	case strings.HasPrefix(line, "["):
		var v []any
		if json.Unmarshal([]byte(line), &v) == nil {
			for _, x := range v[:min(2, len(v))] {
				if s, ok := x.(string); ok && corpusTag.MatchString(s) {
					return s, true
				}
			}
			return "", false
		}
		// Not JSON: an expression that starts with an array.
	}
	if m := corpusCall.FindStringSubmatch(line); m != nil {
		return m[1], true
	}
	if s := strings.TrimSpace(line); corpusTag.MatchString(s) {
		return s, true
	}
	return "", false
}

// unlistedTags are the tags, lower case, that a list of available locales
// has in before and not in after.
func unlistedTags(before, after []byte) (map[string]bool, error) {
	b, err := blob.ReadIndex(before, layout.Available)
	if err != nil {
		return nil, err
	}
	a, err := blob.ReadIndex(after, layout.Available)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for i := 0; i < b.Len(); i++ {
		k, _ := b.At(i)
		if _, ok := a.Find(string(k)); !ok {
			_, tag, _ := strings.Cut(string(k), " ")
			out[strings.ToLower(tag)] = true
		}
	}
	return out, nil
}
