// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// core-js polyfills for URL, URLSearchParams, structuredClone and the iterator
// helpers (Iterator.concat), for the engines that lack them. core-js leaves a native one that passes its own
// checks alone.
import "core-js/modules/web.url.js";
import "core-js/modules/web.url.can-parse.js";
import "core-js/modules/web.url.parse.js";
import "core-js/modules/web.url.to-json.js";
import "core-js/modules/web.url-search-params.js";
import "core-js/modules/web.url-search-params.delete.js";
import "core-js/modules/web.url-search-params.has.js";
import "core-js/modules/web.url-search-params.size.js";
import "core-js/modules/web.structured-clone.js";
import "core-js/es/iterator/index.js";
