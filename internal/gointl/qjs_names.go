// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

// QJSNoNames reports that Embedded has no display names: the build tag
// quickjs_intl_nonames leaves them out, and NewDisplayNames then fails.
func QJSNoNames() bool { return qjsNoNames }

// QJSDisplayNamesLocale is the locale a DisplayNames made for loc resolves
// to, for a build that has no names to make one with.
func QJSDisplayNamesLocale(loc Locale) string { return loc.onlyKeywords().String() }
