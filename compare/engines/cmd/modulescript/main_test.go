// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import "testing"

func TestScript(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"export and map dropped", "var a=1;\nexport{};\n//# sourceMappingURL=ssr.js.map\n",
			"(function(){\"use strict\";\nvar a=1;\n\n})();\n"},
		{"no map comment", "var a=1;export{};\n", "(function(){\"use strict\";\nvar a=1;\n})();\n"},
		{"trailing blank lines", "var a=1;export{};\n\n\n", "(function(){\"use strict\";\nvar a=1;\n})();\n"},
		{"map comment with trailing spaces", "x();export{};\n//# sourceMappingURL=a.map  \n\n", "(function(){\"use strict\";\nx();\n})();\n"},
		{"a map comment in the middle stays", "//# sourceMappingURL=a.map\nx();export{};", "(function(){\"use strict\";\n//# sourceMappingURL=a.map\nx();\n})();\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := script(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("script(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A bundle that does not end with the empty export, as a module with
// imports, other exports or top-level await would not, is refused.
func TestScriptRefusesOtherEnds(t *testing.T) {
	for _, in := range []string{"var a=1;", "export{a};", "export{};x();", ""} {
		if got, err := script(in); err == nil {
			t.Errorf("script(%q) = %q, want an error", in, got)
		}
	}
}
