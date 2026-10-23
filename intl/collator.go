// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	gointl "github.com/liustack/slowjs/internal/gointl"
)

// collator is a resolved Intl.Collator. Its json form is the resolvedOptions()
// result, in the order Chromium reports it, which is all the go-intl
// collator depends on.
type collator struct {
	Locale            string `json:"locale"`
	Usage             string `json:"usage"`
	Sensitivity       string `json:"sensitivity"`
	IgnorePunctuation bool   `json:"ignorePunctuation"`
	Collation         string `json:"collation"`
	Numeric           bool   `json:"numeric"`
	CaseFirst         string `json:"caseFirst"`

	c *gointl.Collator
}

// resolveCollator is InitializeCollator, but for reading the options, which
// intl.js does.
func resolveCollator(req request) (*collator, error) {
	o := req.Options
	var opts gointl.CollatorOptions
	opts.Compat = goCompat
	usage, err := optString(o, "usage", "sort", "search")
	if err != nil {
		return nil, err
	}

	if usage == "search" {
		opts.Usage = gointl.UsageSearch
	}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	if s, ok := o["collation"].(string); ok {
		if !isTypeSequence(s) {
			return nil, rangeError("Invalid collation : %s", s)
		}

		opts.Collation = s
	}
	if v, ok := optBool(o, "numeric"); ok {
		opts.Numeric = gointl.Bool(v)
	}
	switch s, err := optString(o, "caseFirst", "upper", "lower", "false"); {
	case err != nil:
		return nil, err
	case s == "upper":
		opts.CaseFirst = gointl.CaseFirstUpper
	case s == "lower":
		opts.CaseFirst = gointl.CaseFirstLower
	case s == "false":
		opts.CaseFirst = gointl.CaseFirstFalse
	}
	loc, err := goResolve(gointl.ServiceCollator, req.Locales)
	if err != nil {
		return nil, err
	}

	loc = withoutLackedCollation(loc)
	if !chromiumHasCollation(loc.Language.String(), opts.Collation) {
		opts.Collation = ""
	}
	switch s, err := optString(o, "sensitivity", "base", "accent", "case", "variant"); {
	case err != nil:
		return nil, err
	case s == "base":
		opts.Sensitivity = gointl.SensitivityBase
	case s == "accent":
		opts.Sensitivity = gointl.SensitivityAccent
	case s == "case":
		opts.Sensitivity = gointl.SensitivityCase
	case s == "variant":
		opts.Sensitivity = gointl.SensitivityVariant
	}
	if v, ok := optBool(o, "ignorePunctuation"); ok {
		opts.IgnorePunctuation = gointl.Bool(v)
	}
	c, err := gointl.NewCollator(loc, opts)
	if err != nil {
		return nil, err
	}

	r := c.ResolvedOptions()
	return &collator{
		Locale:            r.Locale,
		Usage:             r.Usage.String(),
		Sensitivity:       r.Sensitivity.String(),
		IgnorePunctuation: r.IgnorePunctuation,
		Collation:         r.Collation,
		Numeric:           r.Numeric,
		CaseFirst:         r.CaseFirst.String(),
		c:                 c,
	}, nil
}

// init has nothing to do: go-intl made the collator while resolving it.
func (c *collator) init() error { return nil }
