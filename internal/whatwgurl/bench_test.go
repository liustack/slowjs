// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import "testing"

// BenchmarkShopURL is what a page of a shop does per link: a relative path
// against http://localhost, then the serialization with its parts.
func BenchmarkShopURL(b *testing.B) {
	base, _ := parse("http://localhost", nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		u, err := parse("/zh-CN/products/some-product-name?page=2&sort=price", base)
		if err != nil {
			b.Fatal(err)
		}
		u.Serialize()
	}
}

// BenchmarkShopURLParseBase also parses the base every time.
func BenchmarkShopURLParseBase(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		base, _ := parse("http://localhost", nil)
		u, err := parse("/zh-CN/products/some-product-name?page=2&sort=price", base)
		if err != nil {
			b.Fatal(err)
		}
		u.Serialize()
	}
}
