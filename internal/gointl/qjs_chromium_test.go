// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestQJSResolveMatchesChromium asks every service for each of go-intl's
// available locales and holds the locale it resolves to against the one the
// Chromium build of qjsdata/chromium.json answered (qjs-sync.sh chromium
// records it).
func TestQJSResolveMatchesChromium(t *testing.T) {
	b, err := os.ReadFile("qjsdata/chromium.json")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Browser       string                       `json:"browser"`
		DefaultLocale string                       `json:"defaultLocale"`
		Tags          []string                     `json:"tags"`
		Canonical     map[string]string            `json:"canonical"`
		Services      map[string]map[string]string `json:"services"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	canon, err := NewCanonicalizer(Embedded, CanonicalizeOptions{Compat: NodeICU})
	if err != nil {
		t.Fatal(err)
	}
	def := qjsLocale(c.DefaultLocale)
	services := make([]string, 0, len(c.Services))
	for s := range c.Services {
		services = append(services, s)
	}
	sort.Strings(services)
	for _, service := range services {
		m, err := NewLocaleMatcher(Embedded, Service(service))
		if err != nil {
			t.Fatal(err)
		}
		var bad []string
		for _, tag := range c.Tags {
			want, ok := c.Canonical[tag]
			if !ok {
				want = tag
			}
			l, err := canon.Canonicalize(tag)
			if strings.HasPrefix(want, "ERR:") {
				if err == nil {
					bad = append(bad, fmt.Sprintf("%s canonical %s, %s %s", tag, l, c.Browser, want))
				}
				continue
			}
			if err != nil || l.String() != want {
				bad = append(bad, fmt.Sprintf("%s canonical %s %v, %s %s", tag, l, err, c.Browser, want))
				continue
			}
			if answer, ok := c.Services[service][tag]; ok {
				want = answer
			}
			if got := m.Resolve([]Locale{l}, BestFit, def).String(); got != want {
				bad = append(bad, fmt.Sprintf("%s resolves to %s, %s %s", tag, got, c.Browser, want))
			}
		}
		if len(bad) > 0 {
			if len(bad) > 20 {
				bad = append(bad[:20], "...")
			}
			t.Errorf("%s: %d of %d tags differ:\n%s", service, len(bad), len(c.Tags), strings.Join(bad, "\n"))
		}
	}
}
