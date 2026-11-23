// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build go1.28 && !tifajs_anygo

package goversion

// The engine is checked with Go 1.26 and 1.27. A newer Go stops here, with
// this name in the error. Moving the line above to the next version is the
// last step of docs/internals/upgrading.md.
var _ = tifajs_is_checked_up_to_go1_27_see_docs_upgrading_md_or_build_with_tags_tifajs_anygo
