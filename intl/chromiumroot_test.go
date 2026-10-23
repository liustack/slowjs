// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"compress/gzip"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gointl "github.com/liustack/slowjs/internal/gointl"
)

// chromiumMissingSources are, for the data sets of chromiumMissing whose
// resources the summary of Chromium's ICU data in
// testdata/chromium/icu.json.gz has, where Chromium has them: the tree, and
// the resources of the data set in the language's bundle there, by the
// start of their paths, or none for a data set whose tree has no bundle for
// the language. The summary has no list patterns: the golden test's lists
// in every language check ListFormat's.
var chromiumMissingSources = map[gointl.Marker]struct {
	tree, path string
}{
	gointl.MarkerRelativeTime: {"", "fields"},
	gointl.MarkerUnits:        {"unit", ""},
}

// TestChromiumData derives chromiumMissing from Chromium's ICU data: the
// locales of the languages Chromium has for any service along whose chain
// Chromium's data has none of a data set, so that ICU takes root's, or
// whose language has no bundle in the data set's tree, so that ICU takes
// the default locale's, where go-intl has data of the locale's own.
func TestChromiumData(t *testing.T) {
	f, err := os.Open(filepath.Join(goldenDir, "icu.json.gz"))
	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()

	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}

	var icu chromiumICU
	if err := json.NewDecoder(z).Decode(&icu); err != nil {
		t.Fatal(err)
	}

	cj := chromiumJSON()
	for m, src := range chromiumMissingSources {
		want := map[string]gointl.DataLocale{}
		for _, tag := range cj.Tags {
			lang, _, _ := strings.Cut(tag, "-")
			if tag == "root" || src.path == "" && !chromiumHasLanguage(lang) {
				continue
			}

			lacks, rootParent := icu.lacks(tag, src.tree, src.path)
			if !lacks {
				continue
			}

			l, err := gointl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := gointl.Embedded.Open(m, l.Data()); err == nil {
				want[tag] = gointl.DataLocale{}
				if src.path == "" && !rootParent {
					want[tag] = english
				}
			}
		}
		if got := chromiumMissing[m]; !maps.Equal(got, want) {
			t.Errorf("%s: chromiumMissing has %v, Chromium's ICU data says %v", m, got, want)
		}
	}
	if !maps.Equal(chromiumMissing[gointl.MarkerLists], chromiumMissing[gointl.MarkerRelativeTime]) {
		t.Errorf("chromiumMissing has lists for other locales than relative times")
	}
}

// chromiumICU is the summary of Chromium's ICU data: the paths of the
// resources, and by tree and locale, the bundles with the paths each has.
type chromiumICU struct {
	Paths   []string
	Bundles map[string]struct {
		Alias, Parent string
		Has           []int
	}
}

// lacks reports whether no bundle along tag's chain in tree has a resource
// whose path starts with path, short of root, or, for an empty path,
// whether there is no bundle in tree along it at all; and whether the chain
// ends at a parent of root's that the bundle names, which ICU follows where
// it finds no bundle, rather than at the language. Aliases and parents are
// the main tree's, but that a tree without a bundle for a locale does not
// follow a parent other than root: nn, whose parent is no, takes the
// default locale's units.
func (icu *chromiumICU) lacks(tag, tree, path string) (lacks, rootParent bool) {
	key := strings.ReplaceAll(tag, "-", "_")
	for {
		full := key
		if tree != "" {
			full = tree + "/" + key
		}
		b, ok := icu.Bundles[full]
		switch {
		case ok && path == "":
			return false, false
		case ok && slices.ContainsFunc(b.Has, func(i int) bool { return strings.HasPrefix(icu.Paths[i], path) }):
			return false, false
		}
		main := icu.Bundles[key]
		switch {
		case main.Alias != "":
			key = main.Alias
			continue
		case main.Parent == "root":
			return true, true
		case main.Parent != "" && path != "":
			key = main.Parent
			continue
		}
		i := strings.LastIndexByte(key, '_')
		if i < 0 {
			return true, false
		}

		key = key[:i]
	}
}

// chromiumHasLanguage reports whether some service of Chromium's has tag,
// which it then resolves as itself.
func chromiumHasLanguage(tag string) bool {
	for _, resolves := range chromiumJSON().Services {
		if r, ok := resolves[tag]; !ok || r == tag {
			return true
		}
	}
	return false
}
