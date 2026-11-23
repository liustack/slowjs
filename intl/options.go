// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/tifajs/intl"

import (
	"errors"
	"fmt"
	"math"

	gointl "github.com/liustack/tifajs/internal/gointl"
)

// Reading the options of a request. intl.js has already read every option
// of a constructor and converted it to a string, a number or a boolean, so
// these only check the values, in the order ECMA-402 reads the options.

// A jsError is an error javascript gets as an error of its constructor,
// such as a RangeError, with msg as its message (see jsErr in bridge.go).
type jsError struct {
	ctor, msg string
}

func (e *jsError) Error() string { return e.msg }

// rangeError is a RangeError with a formatted message.
func rangeError(format string, args ...any) error {
	return &jsError{"RangeError", fmt.Sprintf(format, args...)}
}

// typeError is a TypeError with a formatted message.
func typeError(format string, args ...any) error {
	return &jsError{"TypeError", fmt.Sprintf(format, args...)}
}

// badOption is the RangeError of option key given the value v, which the
// option does not take.
func badOption(key string, v any) error {
	return rangeError("value %v out of range for Intl options property %s", v, key)
}

// optString returns the string option key of o, "" when it is not there.
// With allowed, the value has to be one of them.
func optString(o map[string]any, key string, allowed ...string) (string, error) {
	v, ok := o[key]
	if !ok {
		return "", nil
	}

	s, ok := v.(string)
	if !ok {
		return "", badOption(key, v)
	}

	if len(allowed) == 0 {
		return s, nil
	}

	for _, a := range allowed {
		if s == a {
			return s, nil
		}
	}
	return "", badOption(key, s)
}

// optBool returns the boolean option key of o, and whether it was given as
// one.
func optBool(o map[string]any, key string) (bool, bool) {
	b, ok := o[key].(bool)
	return b, ok
}

// optInt is ECMA-402's DefaultNumberOption for option key of o: a number
// from min to max, floored once it is known to be in that range, so that
// 3.9 is out of 1 to 3. It is false when the option was not given.
func optInt(o map[string]any, key string, min, max int) (int, bool, error) {
	v, ok := o[key]
	if !ok {
		return 0, false, nil
	}

	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || f < float64(min) || f > float64(max) {
		return 0, false, badOption(key, v)
	}

	return int(math.Floor(f)), true, nil
}

// enumOption returns the option key of o as the value from first to last
// whose String it is, the zero value when it was not given.
func enumOption[T interface {
	~int
	String() string
}](o map[string]any, key string, first, last T) (T, error) {
	var zero T
	v, ok := o[key]
	if !ok {
		return zero, nil
	}

	if s, ok := v.(string); ok {
		for n := first; n <= last; n++ {
			if n.String() == s {
				return n, nil
			}
		}
	}
	return zero, badOption(key, v)
}

// formatOptionError is err as javascript is to see it: go-intl refusing a
// combination of options is a RangeError.
func formatOptionError(err error) error {
	if errors.Is(err, gointl.ErrOption) {
		return rangeError("%s", err.Error())
	}
	return err
}

// isAlpha, isDigit and isAlnum report whether every byte of s is an ASCII
// letter, a digit, or either. They are true for "".
func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i] | 0x20; c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c|0x20 < 'a' || c|0x20 > 'z') {
			return false
		}
	}
	return true
}
