// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Recorded by intl/testdata/chromium/record.cjs temporal datetime-format.
var cases = [];
(function () {
    const values = [
        ['Temporal.PlainDate.from("2024-01-05")', 'Temporal.PlainDate.from("2024-01-07")'],
        ['Temporal.PlainTime.from("13:05:07.123456789")', 'Temporal.PlainTime.from("15:06:08.987654321")'],
        ['Temporal.PlainDateTime.from("2024-01-05T13:05:07.123456789")', 'Temporal.PlainDateTime.from("2024-01-07T15:06:08.987654321")'],
        ['new Temporal.PlainYearMonth(2024,1,"gregory")', 'new Temporal.PlainYearMonth(2024,3,"gregory")'],
        ['new Temporal.PlainMonthDay(1,5,"gregory")', 'new Temporal.PlainMonthDay(3,7,"gregory")'],
        ['Temporal.Instant.from("2024-01-05T13:05:07.123456789Z")', 'Temporal.Instant.from("2024-01-07T15:06:08.987654321Z")']
    ];
    const options = ['{}', '{year:"numeric"}', '{month:"long",day:"numeric"}', '{hour:"numeric",minute:"2-digit"}',
        '{era:"short"}', '{timeZoneName:"long"}', '{hour12:false}', '{hourCycle:"h24"}', '{fractionalSecondDigits:3}',
        '{dateStyle:"full"}', '{timeStyle:"short"}', '{dateStyle:"long",timeStyle:"medium"}',
        '{timeZone:"America/New_York"}', '{timeZone:"Asia/Tokyo",year:"numeric",month:"short",day:"numeric",hour:"numeric",minute:"2-digit"}'];
    for (const [a,b] of values) {
        for (const locale of ['en-US','de','fr','zh-CN','ja','ar']) for (const o of options) {
            const f = 'new Intl.DateTimeFormat(' + JSON.stringify(locale) + ',' + o + ')';
            cases.push(f + '.format(' + a + ')');
            cases.push('JSON.stringify(' + f + '.formatToParts(' + a + '))');
            cases.push(f + '.formatRange(' + a + ',' + b + ')');
            cases.push('JSON.stringify(' + f + '.formatRangeToParts(' + a + ',' + b + '))');
            cases.push(a + '.toLocaleString(' + JSON.stringify(locale) + ',' + o + ')');
        }
        for (const method of ['formatRange','formatRangeToParts']) {
            cases.push('JSON.stringify(new Intl.DateTimeFormat("en").' + method + '(' + a + ',' + a + '))');
            cases.push('JSON.stringify(new Intl.DateTimeFormat("en").' + method + '(' + b + ',' + a + '))');
        }
    }
    const more = values.map(v=>v[0]).concat(['new Temporal.ZonedDateTime(0n,"UTC")', 'new Temporal.Duration(0,0,0,1)', '0', 'new Date(0)', 'undefined']);
    for (const a of more) for (const b of more) for (const method of ['formatRange','formatRangeToParts'])
        cases.push('JSON.stringify(new Intl.DateTimeFormat("en").' + method + '(' + a + ',' + b + '))');
    for (const a of ['new Temporal.ZonedDateTime(0n,"UTC")','new Proxy(Temporal.PlainDate.from("2024-01-05"),{})','Object.create(Temporal.PlainDate.prototype)',
        'Temporal.Instant.fromEpochNanoseconds(-1n)', 'new Temporal.PlainYearMonth(2024,1)', 'new Temporal.PlainMonthDay(1,5)',
        'Temporal.PlainDate.from("2024-01-05[u-ca=japanese]")']) {
        for (const o of ['{}','{calendar:"iso8601"}','{calendar:"gregory"}']) {
            cases.push('new Intl.DateTimeFormat("en",' + o + ').format(' + a + ')');
            cases.push('JSON.stringify(new Intl.DateTimeFormat("en",' + o + ').formatToParts(' + a + '))');
            if (!a.startsWith('new Proxy') && !a.startsWith('Object.create')) cases.push(a + '.toLocaleString("en",' + o + ')');
        }
    }
    for (const a of ['Temporal.PlainDateTime.from("2024-03-10T02:30")','Temporal.PlainDateTime.from("2024-11-03T01:30")',
        'Temporal.PlainDate.from("2011-12-30")','new Temporal.PlainYearMonth(2024,1,"gregory",23)',
        'new Temporal.PlainMonthDay(2,29,"gregory",2000)']) {
        for (const zone of ['America/New_York','Pacific/Apia']) {
            const o = '{timeZone:' + JSON.stringify(zone) + '}';
            cases.push('new Intl.DateTimeFormat("en",' + o + ').format(' + a + ')');
            cases.push(a + '.toLocaleString("en",' + o + ')');
        }
    }
    cases.push(`(() => {
        const d = Temporal.PlainDate.from('2024-01-05');
        for (const k of ['year','month','day','calendarId','toString','valueOf',Symbol.toPrimitive])
            Object.defineProperty(d,k,{get() { throw new Error('read'); }});
        return new Intl.DateTimeFormat('en').format(d);
    })()`);
    cases.push(`(() => {
        const reads = []; const options = new Proxy({}, {get(t,p) {reads.push(p);}});
        const f = new Intl.DateTimeFormat('en', options);
        reads.length = 0;
        f.format(Temporal.PlainDate.from('2024-01-05'));
        f.formatToParts(Temporal.PlainTime.from('13:05'));
        return reads.join(',');
    })()`);
})();

// Legacy values use the same range API, with ToNumber and TimeClip.
for (const locale of ['en-US', 'de', 'zh-CN', 'ja']) {
    for (const options of ['{}', '{dateStyle:"full"}', '{month:"long",day:"numeric"}', '{timeStyle:"short"}']) {
        for (const pair of ['0,86400000', '86400000,0', '-1,0', 'NaN,0', '0,Infinity', '0,undefined', '1n,2n', '"0",new Date(86400000)']) {
            for (const method of ['formatRange','formatRangeToParts'])
                cases.push('JSON.stringify(new Intl.DateTimeFormat(' + JSON.stringify(locale) + ',' + options + ').' + method + '(' + pair + '))');
        }
    }
}
for (const method of ['formatRange','formatRangeToParts']) {
    for (const calendar of ['iso8601','gregory','japanese']) {
        cases.push('JSON.stringify(new Intl.DateTimeFormat("en").' + method + '(Temporal.PlainDate.from("2024-01-05"),Temporal.PlainDate.from("2024-01-07").withCalendar("' + calendar + '")))');
    }
}
for (const type of ['PlainDate','PlainTime','PlainDateTime','PlainYearMonth','PlainMonthDay','Instant','Duration']) {
    const values = {PlainDate:'2024-01-05', PlainTime:'13:05', PlainDateTime:'2024-01-05T13:05',
        PlainYearMonth:'2024-01-01[u-ca=gregory]', PlainMonthDay:'1972-01-05[u-ca=gregory]', Instant:'2024-01-05T13:05Z', Duration:'P1DT2H'};
    cases.push(`(() => {const locale = new Intl.Locale('de'); locale.toString = () => {throw new Error('locale');};
        return Temporal.${type}.from(${JSON.stringify(values[type])}).toLocaleString([locale]);})()`);
}
// A calendar and a kind must be checked using the original slots, even if
// the instance lies about its public properties.
cases.push(`(() => {
    const a = Temporal.PlainDate.from('2024-01-05[u-ca=japanese]');
    Object.defineProperty(a,'calendarId',{value:'gregory'});
    return new Intl.DateTimeFormat('en').format(a);
})()`);
cases.push(`(() => {
    const f = new Intl.DateTimeFormat('en');
    const value = {[Symbol.toStringTag]:'Temporal.PlainDate', valueOf(){return 0;}};
    return f.format(value);
})()`);

for (const value of ['new Temporal.PlainDate(-271821,4,19)', 'new Temporal.PlainDate(275760,9,13)',
    'new Temporal.PlainDateTime(-271821,4,19,0,0,0,0,0,1)', 'new Temporal.PlainDateTime(275760,9,13,23,59,59,999,999,999)',
    'new Temporal.Instant(-8640000000000000000000n)', 'new Temporal.Instant(8640000000000000000000n)',
    'new Temporal.PlainYearMonth(-271821,4,"gregory")','new Temporal.PlainYearMonth(275760,9,"gregory")']) {
    cases.push('new Intl.DateTimeFormat("en").format(' + value + ')');
    cases.push(value + '.toLocaleString("en")');
}

for (const method of ['formatRange','formatRangeToParts']) {
    cases.push(`(() => {let calls=0; const n={valueOf(){calls++;return 0;}};
        try { new Intl.DateTimeFormat('en').${method}(n,Temporal.PlainDate.from('2024-01-05')); } catch(e) {return e.name+':'+calls;} })()`);
    cases.push(`(() => {let calls=0; const n={valueOf(){calls++;return 0;}};
        try { new Intl.DateTimeFormat('en').${method}(Temporal.PlainDate.from('2024-01-05'),n); } catch(e) {return e.name+':'+calls;} })()`);
}
for (const locale of ['en-US','de','zh-CN']) for (const style of ['short','long']) {
    cases.push(`new Intl.DateTimeFormat('${locale}', {calendar:'iso8601',dateStyle:'${style}'}).format(Temporal.Instant.from('2024-09-19T13:05Z'))`);
    cases.push(`Temporal.PlainDate.from('2024-09-19').toLocaleString('${locale}', {calendar:'iso8601',dateStyle:'${style}'})`);
}
for (const method of ['format','formatToParts']) {
    cases.push(`(() => {const d=Temporal.Duration.from('P1DT2H3M4.005006007S');
        for (const k of ['years','months','weeks','days','hours','minutes','seconds','milliseconds','microseconds','nanoseconds'])
            Object.defineProperty(d,k,{get(){throw new Error('get '+k);}});
        return JSON.stringify(new Intl.DurationFormat('en').${method}(d));})()`);
}

// ISO styles also apply to Date values using the same formatter.
for (const locale of ['en-US','de','zh-CN']) for (const style of ['short','long']) {
    for (const method of ['format','formatToParts','resolvedOptions'])
        cases.push(`JSON.stringify(new Intl.DateTimeFormat('${locale}', {calendar:'iso8601',dateStyle:'${style}'}).${method}(new Date('2024-09-19T13:05Z')))`);
}
for (const method of ['formatRange','formatRangeToParts']) {
    for (const first of [true,false]) cases.push(`(() => {let calls=0; const n={valueOf(){calls++;return NaN;}};
        const z=Temporal.ZonedDateTime.from('2024-01-05T13:05Z[UTC]');
        try {new Intl.DateTimeFormat('en').${method}(${first?'n,z':'z,n'});} catch(e) {return e.name+':'+calls;} })()`);
}

for (const calendar of ['iso8601','gregory','japanese','buddhist']) {
    for (const locale of ['en-US','th-TH']) for (const options of ['{}','{calendar:"iso8601"}','{calendar:"gregory"}'])
        cases.push(`Temporal.ZonedDateTime.from('2024-01-05T13:05Z[UTC]').withCalendar('${calendar}').toLocaleString('${locale}',${options})`);
}
