// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_intl_nocjdict

package intl

// Without the CJK dictionary, Intl.Segmenter does not break a run of
// Chinese or Japanese into words: the run is one word, as in an ICU built
// without cjdict.dict.
var qjsCJDict string

const qjsNoCJDict = true
