#!/bin/sh
# Copyright (c) 2026 The liustack/tifajs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
# Builds corejs-polyfill.js (core-js: URL, URLSearchParams, structuredClone) and
# intl-polyfill.js: the FormatJS polyfills pinned in package.json,
# forced in (polyfill-force), with en and zh data and the golden time zone
# set, as one IIFE script to run before a tifa bundle.
# Package versions and license texts are kept in ../THIRD_PARTY_NOTICES.md.
# When changing the lockfile, regenerate the notices with `go run ./cmd/notices` (see ../THIRD_PARTY_NOTICES.md).
set -e
cd "$(dirname "$0")"
npm ci --no-audit --no-fund
npx esbuild entry.js --bundle --format=iife --minify --target=es2020 --legal-comments=none --outfile=intl-polyfill.js
shasum -a 256 intl-polyfill.js
npx esbuild corejs-entry.js --bundle --format=iife --minify --target=es2020 --legal-comments=none --outfile=corejs-polyfill.js
shasum -a 256 corejs-polyfill.js
