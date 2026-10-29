// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pytext

import (
	"slices"
	"testing"
)

func TestSplitLines(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"a\nb\r\nc\rd\x0be f\n", []string{"a", "b", "c", "d", "e", "f"}},
		{"", nil},
		{"\n", []string{""}},
		{"a\n\nb", []string{"a", "", "b"}},
		{"a\r\n\r\n", []string{"a", ""}},
	} {
		if got := SplitLines(c.in); !slices.Equal(got, c.want) {
			t.Errorf("SplitLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsSpace(t *testing.T) {
	for _, r := range []rune{' ', '\t', '\x1f', ' ', ' ', '　'} {
		if !IsSpace(r) {
			t.Errorf("IsSpace(%U) = false", r)
		}
	}
	if IsSpace('a') || IsSpace('\x1b') {
		t.Error("IsSpace is true for a letter or ESC")
	}
}
