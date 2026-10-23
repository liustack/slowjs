// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package zonedata

import (
	"fmt"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Walk reads all of a locale's data, the tables Decode leaves in the pool
// included, so that slowjs's pack tool learns which parts of the pool the
// locale reaches.
func Walk(data []byte, pool blob.Shared) error {
	l, err := Decode(data, pool)
	if err != nil {
		return err
	}
	for _, k := range l.metazones.Keys() {
		if _, ok := l.Metazone(k); !ok {
			return fmt.Errorf("zonedata: metazone %s does not read", k)
		}
	}
	for _, k := range l.zones.Keys() {
		if _, ok := l.Zone(k); !ok {
			return fmt.Errorf("zonedata: zone %s does not read", k)
		}
	}
	for _, k := range l.regions.Keys() {
		if _, ok := l.Region(k); !ok {
			return fmt.Errorf("zonedata: region %s does not read", k)
		}
	}
	return nil
}
