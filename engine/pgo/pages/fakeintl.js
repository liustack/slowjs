// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A small Intl for the native test host, which has none (Intl comes from the
// Go packages intl and temporal). The pages of engine/pgo/pages need an Intl
// to load. It formats numbers with toFixed, dates as ISO dates and plurals as
// one or other, so the HTML differs from tifa's real pages in those places.
(function () {
  function NF(locale, o) { this.o = o || {}; }
  NF.prototype.format = function (n) { var d = this.o.style === 'currency' ? 2 : (this.o.maximumFractionDigits === undefined ? 3 : this.o.maximumFractionDigits); return (this.o.style === 'currency' ? '$' : '') + Number(n).toFixed(Math.min(d, 2)); };
  NF.prototype.formatToParts = function (n) { return [{ type: 'integer', value: this.format(n) }]; };
  NF.prototype.resolvedOptions = function () { return { locale: 'en-US', numberingSystem: 'latn' }; };
  function DTF(locale, o) { this.o = o || {}; }
  DTF.prototype.format = function (d) { return new Date(d).toISOString().slice(0, 10); };
  DTF.prototype.formatToParts = function (d) { return [{ type: 'literal', value: this.format(d) }]; };
  DTF.prototype.resolvedOptions = function () { return { locale: 'en-US', timeZone: 'UTC', calendar: 'gregory', numberingSystem: 'latn' }; };
  function PR() {} PR.prototype.select = function (n) { return n === 1 ? 'one' : 'other'; };
  function Col() {} Col.prototype.compare = function (a, b) { return a < b ? -1 : a > b ? 1 : 0; };
  function RTF() {} RTF.prototype.format = function (v, u) { return v + ' ' + u; };
  function LF() {} LF.prototype.format = function (l) { return Array.prototype.join.call(l, ', '); };
  function Seg() {} Seg.prototype.segment = function (s) { return Array.from(String(s), function (c, i) { return { segment: c, index: i, input: s }; }); };
  function DN() {} DN.prototype.of = function (c) { return c; };
  function Loc(t) { this.language = String(t).split('-')[0]; this.baseName = String(t); }
  Loc.prototype.toString = function () { return this.baseName; };
  globalThis.Intl = { NumberFormat: NF, DateTimeFormat: DTF, PluralRules: PR, Collator: Col, RelativeTimeFormat: RTF, ListFormat: LF, Segmenter: Seg, DisplayNames: DN, Locale: Loc,
    getCanonicalLocales: function (l) { return [].concat(l || []); }, supportedValuesOf: function () { return []; } };
  Number.prototype.toLocaleString = function (l, o) { return new NF(l, o).format(this); };
  Date.prototype.toLocaleString = Date.prototype.toLocaleDateString = Date.prototype.toLocaleTimeString = function (l, o) { return new DTF(l, o).format(this); };
  String.prototype.localeCompare = function (b) { return this < b ? -1 : this > b ? 1 : 0; };
})();
