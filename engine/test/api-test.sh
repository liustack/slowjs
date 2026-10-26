#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the API the C tree changes, on real runtimes: JS_DumpMemoryUsage, with
# objects, with no buffer, and cut at every length, and JS_NewClassID with
# several runtimes. Run it in asan mode too: the first version of the report
# wrote past a buffer on the stack.
#
#   engine/test/api-test.sh [MODE]
#
# MODE is a mode of build.sh (default plain).
set -eu

here=$(cd "$(dirname "$0")" && pwd)
mode=${1:-plain}

"$here/build.sh" "$mode" > /dev/null
# LeakSanitizer is only there on Linux.
if [ "$(uname -s)" = Linux ]; then
	ASAN_OPTIONS=detect_leaks=1${ASAN_OPTIONS:+:$ASAN_OPTIONS}
	export ASAN_OPTIONS
fi
"$here/out/$mode/api-test"
