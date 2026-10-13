// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// part is one element of an Intl formatToParts result.
type part struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// request is what the javascript shim sends when constructing a formatter.
type request struct {
	Locales []string       `json:"locales"`
	Options map[string]any `json:"options"`
}

// jsError is an error that crosses into javascript as a particular error type.
// ECMA-402 is specific about which one: a value outside a valid range is a
// RangeError, an unusable options argument a TypeError, and real code does
// catch them apart.
type jsError struct {
	ctor string
	msg  string
}

func (e *jsError) Error() string { return e.msg }

func rangeError(s string, args ...any) error {
	return &jsError{"RangeError", fmt.Sprintf(s, args...)}
}

func typeError(s string, args ...any) error {
	return &jsError{"TypeError", fmt.Sprintf(s, args...)}
}

// optString returns the string valued option key, "" if it is absent, or an
// error if its value is not one of allowed.
func optString(o map[string]any, key string, allowed ...string) (string, error) {
	v, ok := o[key]
	if !ok {
		return "", nil
	}

	s, ok := v.(string)
	if !ok {
		return "", rangeError("value %v out of range for Intl options property %s", v, key)
	}

	if slices.Contains(allowed, s) {
		return s, nil
	}

	return "", rangeError("value %s out of range for Intl options property %s", s, key)
}

// optInt returns the integer valued option key and whether it was present. The
// value must be within [min, max].
func optInt(o map[string]any, key string, min, max int) (int, bool, error) {
	v, ok := o[key]
	if !ok {
		return 0, false, nil
	}

	f, ok := v.(float64)
	if !ok || math.IsNaN(f) {
		return 0, false, rangeError("value %v out of range for Intl options property %s", v, key)
	}

	n := int(math.Floor(f))
	if n < min || n > max {
		return 0, false, rangeError("value %v out of range for Intl options property %s", v, key)
	}

	return n, true, nil
}

// optBool returns the boolean valued option key and whether it was present.
func optBool(o map[string]any, key string) (bool, bool) {
	v, ok := o[key]
	if !ok {
		return false, false
	}

	b, ok := v.(bool)
	return b, ok
}

// canonicalLocale returns tag in the canonical BCP-47 casing, or an error if it
// is not structurally valid. Only the shape is checked, no registry lookup
// happens: the result is used to answer getCanonicalLocales and to decide
// whether a locale is supported, and both are honest with a lax parser.
func canonicalLocale(tag string) (string, error) {
	if tag == "" || strings.Contains(tag, "_") {
		return "", rangeError("invalid language tag: %s", tag)
	}

	parts := strings.Split(tag, "-")
	for i, p := range parts {
		if p == "" || len(p) > 8 || !isAlnum(p) {
			return "", rangeError("invalid language tag: %s", tag)
		}

		switch {
		case i == 0:
			if len(p) < 2 || !isAlpha(p) {
				return "", rangeError("invalid language tag: %s", tag)
			}

			parts[i] = strings.ToLower(p)
		case len(p) == 4 && isAlpha(p):
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		case len(p) == 2 && isAlpha(p), len(p) == 3 && isDigit(p):
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, "-"), nil
}

// resolveLocale returns the locale the formatters will actually use. Only
// en-US data is available, so every request resolves to it - which is what
// ECMA-402's lookup matcher does with an unavailable locale anyway, and what
// resolvedOptions().locale then honestly reports.
func resolveLocale(tags []string) (string, error) {
	for _, tag := range tags {
		if _, err := canonicalLocale(tag); err != nil {
			return "", err
		}
	}
	return "en-US", nil
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

func isDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
