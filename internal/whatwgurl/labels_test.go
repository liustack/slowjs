// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import "testing"

// UTS #46 revision 35 (section 4, step 4 and the validity criteria of 4.1) fails
// a label that starts with xn-- and has a character that is not ASCII, one whose
// punycode decodes to nothing, and one that decodes to a label starting with
// xn-- again. The WPT files have none of the three: toascii.json has labels with a
// character that is not ASCII after xn-- ("xn--tešla"), but the character is in
// the part that punycode decodes, which the decoder refuses; the decoder takes a
// character before the last hyphen as itself, so "xn--é-" got through. No case
// has a label that decodes to nothing or to one that starts with xn-- again, and
// the ones of IdnaTestV2 are made from the mapping table, whose labels are all
// ASCII or valid.
func TestPunycodeLabelChecks(t *testing.T) {
	for _, in := range []string{
		"http://xn--é-/",
		"http://xn--.é/",
		"http://xn--xn---epa.é/",
	} {
		if u, err := parse(in, nil); err == nil {
			t.Errorf("%q parsed as %s, want failure", in, u.Href())
		}
	}
	for in, want := range map[string]string{
		"http://xn--9ca/":           "http://xn--9ca/",
		"http://é.xn--9ca/":         "http://xn--9ca.xn--9ca/",
		"http://xn--xn---epa/":      "http://xn--xn---epa/",
		"http://xn--/":              "http://xn--/",
		"http://a.xn--bcher-kva.b/": "http://a.xn--bcher-kva.b/",
	} {
		u, err := parse(in, nil)
		if err != nil || u.Href() != want {
			t.Errorf("%q = %v %v, want %s", in, u, err, want)
		}
	}
}
