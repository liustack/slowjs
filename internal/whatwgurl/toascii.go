// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import "github.com/liustack/quickjs/internal/whatwgurl/idna"

// toASCII runs UTS #46 ToASCII as domain to ASCII asks for it, for a domain
// of at most MaxDomainUnits once mapped and normalized.
func toASCII(domain string, beStrict bool) (string, error) {
	return idna.ToASCII(domain, beStrict, MaxDomainUnits)
}

// Warm builds, once, what the first host with a character that is not ASCII
// builds in the process: the tables of the normalization. It is the same for
// every VM and every script. Package url calls it when it registers on a VM, so
// that no call of a script is charged for it.
func Warm() { idna.Warm() }
