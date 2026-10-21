// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build quickjs_intl_nocjdict

package intl

// Without the CJK dictionary, Intl.Segmenter does not break a run of
// Chinese or Japanese into words: the run is one word, as in an ICU built
// without cjdict.dict.
var qjsCJDict string

const qjsNoCJDict = true
