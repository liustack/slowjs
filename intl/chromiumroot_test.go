// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	gointl "github.com/liustack/quickjs/internal/gointl"
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
	gointl.MarkerNumbers:      {"", "NumberElements/latn/"},
	gointl.MarkerUnits:        {"unit", ""},
}

// TestChromiumData derives chromiumMissing from Chromium's ICU data: the
// languages Chromium has for any service whose bundle has none of a data
// set, so that ICU takes root's, or that have no bundle in the data set's
// tree, so that ICU takes the default locale's, where go-intl has the
// language's own.
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

	var icu struct {
		Paths   []string
		Bundles map[string]struct {
			Alias, Parent string
			Has           []int
		}
	}
	if err := json.NewDecoder(z).Decode(&icu); err != nil {
		t.Fatal(err)
	}

	cj := chromiumJSON()
	for m, src := range chromiumMissingSources {
		var want []string
		for _, tag := range cj.Tags {
			if strings.Contains(tag, "-") || tag == "root" {
				continue
			}

			key := tag
			if src.tree != "" {
				key = src.tree + "/" + tag
			}
			bundle, ok := icu.Bundles[key]
			as := gointl.DataLocale{}
			switch {
			case src.path == "":
				if ok || icu.Bundles[tag].Alias != "" || !chromiumHasLanguage(tag) {
					continue
				}
				as = english
			case !ok || bundle.Alias != "" || (bundle.Parent != "" && bundle.Parent != "root"):
				continue
			case slices.ContainsFunc(bundle.Has, func(i int) bool { return strings.HasPrefix(icu.Paths[i], src.path) }):
				continue
			}
			l, err := gointl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := gointl.Embedded.Open(m, l.Data()); err == nil {
				want = append(want, tag)
			}
			if got := chromiumMissing[m].as; got != as {
				t.Errorf("%s: chromiumMissing has %s's data in place, Chromium's ICU data %s's", m, got, as)
			}
		}
		sort.Strings(want)
		if got := chromiumMissing[m]; !slices.Equal(got.langs, want) {
			t.Errorf("%s: chromiumMissing has %v, Chromium's ICU data says %v", m, got.langs, want)
		}
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
