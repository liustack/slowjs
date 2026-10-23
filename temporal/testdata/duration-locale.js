// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Recorded by intl/testdata/chromium/record.cjs temporal duration-locale.
var cases = [];
for (const value of ['PT0S', 'P1Y2M3W4DT5H6M7.008009001S', '-P1DT2H3M', 'PT0.000000001S', 'PT25H90M']) {
    for (const locale of ['en-US', 'de', 'fr', 'zh-CN', 'ar', 'ja', 'en-u-nu-arab']) {
        for (const options of ['undefined', '{style:"long"}', '{style:"narrow"}', '{style:"digital"}', '{style:"digital",fractionalDigits:9}', '{seconds:"numeric", fractionalDigits:3}']) {
            cases.push('Temporal.Duration.from(' + JSON.stringify(value) + ').toLocaleString(' + JSON.stringify(locale) + ',' + options + ')');
        }
    }
}
for (const args of ['"en", null', '"en", 1', 'null', '"bad_tag"', '"en", {style:"bad"}', '"en", {fractionalDigits:10}', '"en", {hours:"2-digit", minutes:"long"}']) {
    cases.push('Temporal.Duration.from("P1DT2H").toLocaleString(' + args + ')');
}
cases.push(`(() => {
    const d = Temporal.Duration.from('P1DT2H');
    for (const k of ['years','months','weeks','days','hours','minutes','seconds','milliseconds','microseconds','nanoseconds','toString'])
        Object.defineProperty(d, k, {get() { throw new Error('read ' + k); }});
    return d.toLocaleString('en', {style:'long'});
})()`);
cases.push(`(() => {
    const reads = [];
    const o = new Proxy({}, {get(t, p) { reads.push(p); }});
    Temporal.Duration.from('P1DT2H').toLocaleString('en', o);
    return reads.join(',');
})()`);
