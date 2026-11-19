// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package main

import "testing"

// liustack-slowjs-polyfill is built with the other binaries, but the V8 suite
// does not run it unless -engines names it.
func TestPagesOnlyVariant(t *testing.T) {
	has := func(vs []variant, name string) bool {
		for _, v := range vs {
			if v.Name == name {
				return true
			}
		}
		return false
	}
	const bare = "liustack-slowjs-polyfill"
	oldEngines, oldBuildOnly := *flagEngines, *flagBuildOnl
	defer func() { *flagEngines, *flagBuildOnl = oldEngines, oldBuildOnly }()
	*flagEngines, *flagBuildOnl = "", false
	if has(selected(), bare) || !has(selected(), "liustack-slowjs") {
		t.Errorf("default run selects %+v", selected())
	}
	*flagBuildOnl = true
	if !has(selected(), bare) {
		t.Errorf("-build-only does not select %s", bare)
	}
	*flagBuildOnl, *flagEngines = false, bare
	if s := selected(); len(s) != 1 || s[0].Name != bare || s[0].Tags != "eng_liustackpolyfill" {
		t.Errorf("-engines %s selects %+v", bare, s)
	}
}
