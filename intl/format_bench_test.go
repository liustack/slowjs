// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"testing"
)

// JS construction measures the shared registry's normal warm path. Resolve
// separately measures constructing a new data formatter without that cache.
func BenchmarkFormat(b *testing.B) {
	for _, locale := range []string{"en-US", "zh-CN"} {
		for _, service := range []string{"NumberFormat", "DateTimeFormat"} {
			opts := `{style:'currency',currency:'USD'}`
			value := `1234567.891`
			options := map[string]any{"style": "currency", "currency": "USD"}
			if service == "DateTimeFormat" {
				opts = `{dateStyle:'medium',timeStyle:'short',timeZone:'UTC'}`
				value = `1755324309123`
				options = map[string]any{"dateStyle": "medium", "timeStyle": "short", "timeZone": "UTC"}
			}
			constructor := fmt.Sprintf("new Intl.%s(%q,%s)", service, locale, opts)
			b.Run(service+"/"+locale+"/New", func(b *testing.B) { benchmarkLoop(b, constructor, constructor) })
			b.Run(service+"/"+locale+"/Format", func(b *testing.B) { benchmarkLoop(b, "const f="+constructor, "f.format("+value+")") })
			b.Run(service+"/"+locale+"/Resolve", func(b *testing.B) {
				req := request{Locales: []string{locale}, Options: options}
				run := func() {
					var f formatter
					var err error
					if service == "NumberFormat" {
						f, err = resolveNumberFormat(req)
					} else {
						f, err = resolveDateTimeFormat(req, "UTC")
					}
					if err != nil {
						b.Fatal(err)
					}
					if err = f.init(); err != nil {
						b.Fatal(err)
					}
				}
				run()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					run()
				}
			})
		}
	}
}
