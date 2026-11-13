/*
 * Copyright 2020 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// This file started from github.com/nlnwa/whatwg-url v0.6.2 (see
// LICENSE-WHATWG-URL) and was rewritten and changed by The liustack/quickjs
// Authors, who offer their changes under the MIT license (LICENSE-MIT at the
// root of this repository) as well. SOURCES.md says what remains of it.

package whatwgurl

import "strings"

// The setters of the URL API
// (https://url.spec.whatwg.org/#api). Each changes u in place. A failure of
// the parser inside a setter leaves u as the standard says, so it is not
// reported. ErrTooLong is: the change would make the serialization of u longer
// than MaxUnits, and u is then left as it was.

// change runs the setter f on u, and undoes it when it makes the serialization
// of u longer than MaxUnits.
func (u *URL) change(f func()) error {
	old := u.Clone()
	f()
	if u.length() > MaxUnits {
		*u = *old
		return ErrTooLong
	}
	return nil
}

// SetProtocol is the setter of protocol.
func (u *URL) SetProtocol(v Input) error {
	return u.change(func() { basicParse(v.s+":", nil, u, stateSchemeStart) })
}

// SetUsername is the setter of username.
func (u *URL) SetUsername(v Input) error {
	if u.cannotHaveUsernamePasswordPort() {
		return nil
	}
	return u.change(func() { u.Username = encodeString(v.s, userinfoSet) })
}

// SetPassword is the setter of password.
func (u *URL) SetPassword(v Input) error {
	if u.cannotHaveUsernamePasswordPort() {
		return nil
	}
	return u.change(func() { u.Password = encodeString(v.s, userinfoSet) })
}

// SetHost is the setter of host.
func (u *URL) SetHost(v Input) error {
	if u.Opaque {
		return nil
	}
	return u.change(func() { basicParse(v.s, nil, u, stateHost) })
}

// SetHostname is the setter of hostname.
func (u *URL) SetHostname(v Input) error {
	if u.Opaque {
		return nil
	}
	return u.change(func() { basicParse(v.s, nil, u, stateHostname) })
}

// SetPort is the setter of port.
func (u *URL) SetPort(v Input) error {
	if u.cannotHaveUsernamePasswordPort() {
		return nil
	}
	if v.s == "" {
		u.Port = -1
		return nil
	}
	return u.change(func() { basicParse(v.s, nil, u, statePort) })
}

// SetPathname is the setter of pathname.
func (u *URL) SetPathname(v Input) error {
	if u.Opaque {
		return nil
	}
	return u.change(func() {
		u.Path = Path{}
		basicParse(v.s, nil, u, statePathStart)
	})
}

// SetSearch is the setter of search. The caller makes the list of the
// search parameters again from u.Query.
func (u *URL) SetSearch(v Input) error {
	if v.s == "" {
		u.Query, u.HasQuery = "", false
		return nil
	}
	return u.change(func() {
		u.Query, u.HasQuery = "", true
		basicParse(strings.TrimPrefix(v.s, "?"), nil, u, stateQuery)
	})
}

// SetHash is the setter of hash.
func (u *URL) SetHash(v Input) error {
	if v.s == "" {
		u.Fragment, u.HasFragment = "", false
		return nil
	}
	return u.change(func() {
		u.Fragment, u.HasFragment = "", true
		basicParse(strings.TrimPrefix(v.s, "#"), nil, u, stateFragment)
	})
}

// encodeString is the UTF-8 percent-encode of a string under a set.
func encodeString(s string, set *encodeSet) string {
	var b []byte
	for i, c := range s {
		if b == nil {
			if c < 0x80 && !set[c] {
				continue
			}
			b = append(make([]byte, 0, len(s)+8), s[:i]...)
		}
		b = appendEncoded(b, c, set)
	}
	if b == nil {
		return s
	}
	return string(b)
}
