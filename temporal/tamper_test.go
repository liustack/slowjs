// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// probes are what TestTamper asks Temporal, each an expression whose value
// is a string or a number.
var probes = []string{
	`Temporal.PlainDate.from('2024-01-05').add({months: 1, days: 3}).toString()`,
	`new Temporal.PlainDate(2024, 1, 31).with({month: 2}).toString()`,
	`Temporal.PlainDate.from({year: 2024, month: 2, day: 30}).toString()`,
	`Temporal.PlainDate.from('2024-01-05').until('2025-03-01', {largestUnit: 'years'}).toString()`,
	`Temporal.PlainDate.from('2024-01-05').dayOfYear`,
	`Temporal.PlainDate.from('2024-01-05[u-ca=japanese]').eraYear`,
	`Temporal.PlainTime.from('13:05:07.123456789').round({smallestUnit: 'millisecond'}).toString()`,
	`Temporal.PlainDateTime.from('2024-01-05T13:05').toZonedDateTime('Asia/Tokyo').toString()`,
	`Temporal.PlainYearMonth.from('2024-02').add({months: 13}).toString()`,
	`Temporal.PlainMonthDay.from({monthCode: 'M02', day: 29}).toString()`,
	`Temporal.Instant.from('2024-01-05T13:05:00.5Z').round('second').toString()`,
	`String(Temporal.Instant.fromEpochMilliseconds(-1).epochNanoseconds)`,
	`String(new Temporal.Instant(123456789n).epochMilliseconds)`,
	`Temporal.ZonedDateTime.from('2024-11-03T01:30-05:00[America/New_York]').toString()`,
	`Temporal.ZonedDateTime.from('2024-11-03T12:00[America/New_York]').hoursInDay`,
	`Temporal.ZonedDateTime.from('2024-11-03T12:00[America/New_York]').getTimeZoneTransition({direction: 'previous'}).toString()`,
	`Temporal.ZonedDateTime.from('2024-11-03T12:00[America/New_York]').since('2024-01-01T00:00[UTC]', {largestUnit: 'month', smallestUnit: 'hour'}).toString()`,
	`Temporal.Duration.from({hours: 49, minutes: 75}).round({largestUnit: 'days'}).toString()`,
	`Temporal.Duration.from('P1Y').total({unit: 'days', relativeTo: '2024-01-01'})`,
	`Temporal.Duration.compare('P1M', 'P30D', {relativeTo: Temporal.PlainDate.from('2024-02-01')})`,
	`Temporal.Duration.from({seconds: 1.5e3}).toString({fractionalSecondDigits: 3})`,
	`new (globalThis.OriginalDate || Date)(1704460800123).toTemporalInstant().toString()`,
	`Temporal.PlainDate.from('2024-01-05').toLocaleString('en-US', {dateStyle: 'full'})`,
	`Temporal.ZonedDateTime.from('2024-01-05T13:05[Europe/Paris]').toLocaleString(['de'])`,
	`Temporal.PlainTime.from('13:05').toLocaleString('en-GB')`,
	`Temporal.PlainDate.from('2024-02-30', {overflow: 'reject'})`,
	`Temporal.PlainDate.from({})`,
	`Temporal.Duration.from({years: 1, months: -1})`,
	`Temporal.PlainDate.from('2024-01-05').valueOf()`,
	`Temporal.Now.timeZoneId()`,
	`typeof Temporal.Now.instant().epochMilliseconds`,
	`Temporal.Now.zonedDateTimeISO('UTC')[Symbol.toStringTag]`,
}

// probeScript evaluates probes with nothing but string concatenation, so
// that a page changing built-ins changes no more than Temporal's answers.
func probeScript() string {
	var b strings.Builder
	b.WriteString("(() => { let r = '';\n")
	for _, p := range probes {
		b.WriteString("try { r += (" + p + "); } catch (e) { r += 'throws ' + (e instanceof RangeError ? 'RangeError' : e instanceof TypeError ? 'TypeError' : 'other'); }\nr += '\\n';\n")
	}
	b.WriteString("return r; })()")
	return b.String()
}

// tampers are pages changing what Temporal's code could use. Option and
// field names are left alone: Temporal reads them from the objects a page
// passes, prototypes included, as the spec has it.
var tampers = map[string]string{
	"Array.prototype setters": `for (let i = 0; i < 32; i++) Object.defineProperty(Array.prototype, i, {__proto__: null, set() { throw new Error('set ' + i); }, get() { throw new Error('get ' + i); }});`,
	"Object.prototype setters": `for (const k of ['kind', 'y', 'm', 'd', 'h', 'cal', 'tz', 'info', 'f', 'ems', 'sub', 'off', 'value', 'get', 'set', 'writable', 'enumerable', 'configurable', 'names', 'kinds', 'Temporal', 'toTemporalInstant', 'length', '0', '1'])
		Object.defineProperty(Object.prototype, k, {__proto__: null, set() { throw new Error('set ' + k); }, get() { throw new Error('get ' + k); }, configurable: true});`,
	"Array.prototype methods": `for (const k of Object.getOwnPropertyNames(Array.prototype)) if (k !== 'length' && k !== 'constructor') Array.prototype[k] = function () { throw new Error(k); };
		Array.prototype[Symbol.iterator] = function () { throw new Error('iterator'); };`,
	"Function.prototype": `Function.prototype.call = Function.prototype.apply = Function.prototype.bind = function () { throw new Error('call'); };`,
	"Object and Reflect": `const names = Object.getOwnPropertyNames, reflect = names(Reflect);
		for (const k of names(Object)) if (typeof Object[k] === 'function') Object[k] = function () { throw new Error(k); };
		for (const k of reflect) Reflect[k] = function () { throw new Error(k); };`,
	"globals": `JSON.parse = () => { throw new Error('parse'); }; Math.floor = () => 7; globalThis.BigInt = function () { throw new Error('BigInt'); };
		globalThis.Number = function () { return 7; }; globalThis.Object = function () { return {}; }; globalThis.OriginalDate = Date; globalThis.Date = function () {};
		globalThis.RangeError = function () {}; globalThis.TypeError = function () {}; globalThis.Error = function () {};`,
	"String and Symbol": `for (const k of Object.getOwnPropertyNames(String.prototype)) if (k !== 'length' && k !== 'constructor') String.prototype[k] = function () { throw new Error(k); };
		Symbol.prototype.toString = () => 'x'; Object.defineProperty(Symbol.prototype, 'description', {get() { throw new Error('description'); }});`,
	"Date.prototype": `Date.prototype.valueOf = Date.prototype.getTime = () => 0;`,
	"Intl": `Intl.DateTimeFormat = function () { throw new Error('dtf'); }; Intl.DateTimeFormat.prototype.format = () => 'x';
		Intl.getCanonicalLocales = () => { throw new Error('locales'); };`,
}

// TestTamper runs probeScript on a VM a page has changed built-ins of,
// before Temporal's first use and after it, and compares with the answers
// of a VM nobody changed. Temporal's own code must not depend on anything a
// page can change.
func TestTamper(t *testing.T) {
	probe := probeScript()
	want := eval(t, newVM(t), probe).(string)
	if strings.Contains(want, "throws other") {
		t.Fatalf("probes:\n%s", want)
	}
	for name, src := range tampers {
		for _, after := range []bool{false, true} {
			name := name
			if after {
				name += " after first use"
			}
			t.Run(name, func(t *testing.T) {
				vm := newVM(t)
				if after {
					eval(t, vm, `Temporal.PlainDate.from('2024-01-05').toString(); new Date(0).toTemporalInstant(); Temporal.PlainDate.from('2024-01-05').toLocaleString()`)
				}
				if _, err := vm.Eval(src+";undefined", quickjs.EvalGlobal); err != nil {
					t.Fatal(err)
				}
				// The probes compare errors by constructor, which a page
				// replacing the global ones changes: they are taken first.
				got := eval(t, vm, probe).(string)
				if strings.Contains(src, "globalThis.RangeError") {
					got = strings.ReplaceAll(got, "throws other", "throws ?")
					want := want
					want = strings.ReplaceAll(want, "throws RangeError", "throws ?")
					want = strings.ReplaceAll(want, "throws TypeError", "throws ?")
					diff(t, got, want)
					return
				}
				diff(t, got, want)
			})
		}
	}
}

// diff reports the probes whose answers differ.
func diff(t *testing.T, got, want string) {
	t.Helper()
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range w {
		if i >= len(g) || g[i] != w[i] {
			t.Errorf("%s\n\tgot  %q\n\twant %q", probes[i], g[min(i, len(g)-1)], w[i])
		}
	}
}
