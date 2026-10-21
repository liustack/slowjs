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

// chromiumRootDataSources are, for each service of chromiumRootData, the
// resource of Chromium's ICU data it formats with, as the summary in
// testdata/chromium/icu.json.gz names it, and the go-intl data it is.
var chromiumRootDataSources = map[gointl.Service]struct {
	tree, path string
	marker     gointl.Marker
}{
	gointl.ServiceRelativeTimeFormat: {"", "fields", gointl.MarkerRelativeTime},
}

// TestChromiumRootData derives chromiumRootData from Chromium's ICU data:
// the languages Chromium has the service for whose bundle has none of its
// resource, so that ICU takes root's, where go-intl has data for the
// language.
func TestChromiumRootData(t *testing.T) {
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

	b, err := os.ReadFile(filepath.Join("..", "internal", "gointl", "qjsdata", "chromium.json"))
	if err != nil {
		t.Fatal(err)
	}

	var cj struct {
		Tags     []string
		Services map[string]map[string]string
	}
	if err := json.Unmarshal(b, &cj); err != nil {
		t.Fatal(err)
	}

	if len(chromiumRootDataSources) != len(chromiumRootData) {
		t.Fatalf("%d sources for %d services", len(chromiumRootDataSources), len(chromiumRootData))
	}

	for svc, src := range chromiumRootDataSources {
		path := slices.Index(icu.Paths, src.path)
		if path < 0 {
			t.Fatalf("%s: the summary has no %s", svc, src.path)
		}

		resolves := cj.Services[string(svc)]
		var want []string
		for _, tag := range cj.Tags {
			if strings.Contains(tag, "-") || tag == "root" {
				continue
			}

			if r, ok := resolves[tag]; ok && r != tag {
				continue // Chromium does not have the language for the service
			}

			key := tag
			if src.tree != "" {
				key = src.tree + "/" + tag
			}
			bundle, ok := icu.Bundles[key]
			if !ok || bundle.Alias != "" || slices.Contains(bundle.Has, path) || (bundle.Parent != "" && bundle.Parent != "root") {
				continue
			}

			l, err := gointl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := gointl.Embedded.Open(src.marker, l.Data()); err == nil {
				want = append(want, tag)
			}
		}
		sort.Strings(want)
		if got := chromiumRootData[svc]; !slices.Equal(got.langs, want) {
			t.Errorf("%s: chromiumRootData has %v, Chromium's ICU data says %v", svc, got.langs, want)
		}
	}
}
