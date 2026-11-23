// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package main

import (
	"encoding/json"
	"testing"
)

func TestSelfRowReadsOldResultsFiles(t *testing.T) {
	for _, c := range []struct {
		engines []string
		want    string
	}{
		{[]string{"liustack-tifajs", "moejs"}, "liustack-tifajs"},
		{[]string{"liustack-quickjs", "moejs"}, "liustack-quickjs"},
		{[]string{"liustack-slowjs", "moejs"}, "liustack-slowjs"},
		{[]string{"slowjs", "moejs"}, "slowjs"},
		{[]string{"moejs"}, "liustack-tifajs"},
	} {
		m := map[string][]json.RawMessage{}
		for _, e := range c.engines {
			m[e] = nil
		}
		if got := selfRow(m); got != c.want {
			t.Errorf("selfRow(%v) = %q, want %q", c.engines, got, c.want)
		}
	}
}
