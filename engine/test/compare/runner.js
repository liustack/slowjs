// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// What the two runners of compare.sh print: a function given as the
// completion value runs after the jobs, and a lone surrogate is written out.
var log = [];
Promise.resolve().then(function () { log.push('job'); });
log.push('script');
(function () { return log.join(' ') + ' a\ud800b \u{1f600} ' + [1, 2].map(String); });
