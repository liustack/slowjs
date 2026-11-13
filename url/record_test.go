// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import "testing"

// TestSettersKeepTheRecord checks that a setter changes its part of the URL
// record and nothing else, also on a record that the parser would not make
// from its href: a file URL whose host is localhost, which the parser makes
// empty, and a file URL whose first segment is a drive letter with a |, which
// the parser makes a colon. Both come from a change of protocol, and the next
// setter left them as the parser makes them while a URL was kept as its href
// (the third finding of the fourth review). The answers are the standard's.
// Chrome gives them, but for the |, which it percent-encodes in a path. Node
// keeps a URL as its href too, and has the same fault.
func TestSettersKeepTheRecord(t *testing.T) {
	vm := newVM(t)
	for js, want := range map[string]string{
		`var u = new URL("http://localhost/"); u.protocol = "file:"; u.hash = "x"; u.href + " " + u.hostname`:                   "file://localhost/#x localhost",
		`var u = new URL("http://localhost/a"); u.protocol = "file"; u.search = "q"; u.pathname = "/b"; u.href`:                 "file://localhost/b?q",
		`var u = new URL("http://localhost/"); u.protocol = "file"; u.protocol = "https"; u.href`:                               "https://localhost/",
		`var u = new URL("http://h/C|/x"); u.protocol = "file:"; u.hash = "y"; u.href + " " + u.pathname`:                       "file://h/C|/x#y /C|/x",
		`var u = new URL("http://h/C|/x"); u.protocol = "file"; u.password = "p"; u.username = "u"; u.port = "1"; u.href`:       "file://h/C|/x",
		`var u = new URL("http://h/C|/x"); u.protocol = "file"; u.host = "g"; u.href`:                                           "file://g/C|/x",
		`var u = new URL("http://h/C|/x"); u.protocol = "file"; u.searchParams.append("a", "b"); u.href`:                        "file://h/C|/x?a=b",
		`var u = new URL("http://localhost/"); u.protocol = "file"; u.searchParams.append("a", "b"); u.href + " " + u.hostname`: "file://localhost/?a=b localhost",
	} {
		if got := evalString(t, vm, js); got != want {
			t.Errorf("%s\n got  %q\n want %q", js, got, want)
		}
	}
}
