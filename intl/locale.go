// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

const defaultLocale = "en-US"

func canonicalLocale(tag string) (string, error) {
	c, err := goCanonicalizer()
	if err != nil {
		return "", err
	}
	l, err := c.Canonicalize(tag)
	if err != nil {
		return "", rangeError("Incorrect locale information provided")
	}
	return l.String(), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
