#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the native test host (host.c) on its own, with stubs in place of the
# engine: a runtime that does not start must leave no VM and no chunk behind,
# two threads that make and free runtimes must not race, and a stop asked by
# another thread or by the watchdog (after 1 s here) must come through
# host_poll, with no other thread writing the interrupt flag. Run it in tsan
# mode for the races.
#
#   engine/test/host-test.sh [MODE]
#
# MODE is a mode of build.sh (default plain).
set -eu

here=$(cd "$(dirname "$0")" && pwd)
mode=${1:-plain}

"$here/build.sh" "$mode" > /dev/null
"$here/out/$mode/host-test"
