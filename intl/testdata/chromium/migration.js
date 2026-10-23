// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Recorded by record.cjs probes migration.
var cases = [];
for (const locale of ['en','zh-CN','ar']) {
    for (const options of [
        '{minimumFractionDigits:2,maximumFractionDigits:2,roundingIncrement:5}',
        '{minimumFractionDigits:1,maximumFractionDigits:1,roundingIncrement:2,roundingMode:"halfEven"}',
        '{maximumFractionDigits:2,maximumSignificantDigits:3,roundingPriority:"morePrecision"}',
        '{maximumFractionDigits:2,maximumSignificantDigits:3,roundingPriority:"lessPrecision"}',
        '{minimumFractionDigits:2,minimumSignificantDigits:2,roundingPriority:"morePrecision"}',
        '{notation:"compact"}', '{roundingIncrement:5}', '{maximumSignificantDigits:3,roundingIncrement:5}',
        '{minimumFractionDigits:1,maximumFractionDigits:2,roundingIncrement:5}',
        '{minimumFractionDigits:2,maximumFractionDigits:2,roundingIncrement:5,roundingPriority:"morePrecision"}'
    ]) {
        for (const kind of ['NumberFormat','PluralRules']) {
            const f='new Intl.'+kind+'('+JSON.stringify(locale)+','+options+')';
            cases.push('JSON.stringify('+f+'.resolvedOptions())');
            for (const n of ['1.125','-1.125','0.0099','1234.567','1','0','NaN','Infinity']) {
                cases.push('String('+f+'.'+(kind==='NumberFormat'?'format':'select')+'('+n+'))');
                if (kind==='NumberFormat') cases.push('JSON.stringify('+f+'.formatToParts('+n+'))');
            }
        }
    }
}
for (const zone of ['America/Winnipeg','America/Inuvik','Europe/Dublin','Africa/Casablanca','Antarctica/Casey', 'america/new_york','Asia/Calcutta','Europe/Kyiv','+00:00','-00','Local','US/Pacific-New']) {
    for (const date of ['2026-11-15T12:00Z','2026-07-15T12:00Z','1925-01-01T00:00Z']) {
        const o='{timeZone:'+JSON.stringify(zone)+',year:"numeric",month:"long",day:"numeric",hour:"numeric",minute:"numeric",timeZoneName:"long"}';
        cases.push('new Intl.DateTimeFormat("en",'+o+').format(new Date("'+date+'"))');
        cases.push('JSON.stringify(new Intl.DateTimeFormat("en",'+o+').formatToParts(new Date("'+date+'")))');
        cases.push('new Date("'+date+'").toLocaleString("en",'+o+')');
    }
}
for (const calendar of ['iso8601','buddhist','japanese','chinese','dangi','hebrew','islamic','persian']) {
    for (const locale of ['en','ja','zh-CN']) {
        const f='new Intl.DateTimeFormat('+JSON.stringify(locale)+',{calendar:'+JSON.stringify(calendar)+',dateStyle:"full"})';
        cases.push(f+'.format(new Date("2024-09-19T13:05Z"))');
        cases.push('JSON.stringify('+f+'.resolvedOptions())');
    }
}
for (const options of ['{maximumFractionDigits:2,maximumSignificantDigits:3,roundingPriority:"morePrecision"}', '{minimumFractionDigits:2,maximumFractionDigits:2,roundingIncrement:5}'])
    cases.push('(1234.567).toLocaleString("zh-CN",'+options+')');
cases.push('JSON.stringify(Intl.supportedValuesOf("calendar"))');
