// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// FormatJS polyfills, forced in (polyfill-force), with zh and en data and
// the golden time zone set. Order follows the FormatJS docs: each polyfill
// needs the ones before it.
import "@formatjs/intl-getcanonicallocales/polyfill-force.js";
import "@formatjs/intl-locale/polyfill-force.js";
import "@formatjs/intl-pluralrules/polyfill-force.js";
import "@formatjs/intl-pluralrules/locale-data/en.js";
import "@formatjs/intl-pluralrules/locale-data/zh.js";
import "@formatjs/intl-numberformat/polyfill-force.js";
import "@formatjs/intl-numberformat/locale-data/en.js";
import "@formatjs/intl-numberformat/locale-data/zh.js";
import "@formatjs/intl-numberformat/locale-data/zh-Hans.js";
import "@formatjs/intl-datetimeformat/polyfill-force.js";
import "@formatjs/intl-datetimeformat/locale-data/en.js";
import "@formatjs/intl-datetimeformat/locale-data/zh.js";
import "@formatjs/intl-datetimeformat/locale-data/zh-Hans.js";
import "@formatjs/intl-datetimeformat/add-golden-tz.js";
import "@formatjs/intl-listformat/polyfill-force.js";
import "@formatjs/intl-listformat/locale-data/en.js";
import "@formatjs/intl-listformat/locale-data/zh.js";
import "@formatjs/intl-relativetimeformat/polyfill-force.js";
import "@formatjs/intl-relativetimeformat/locale-data/en.js";
import "@formatjs/intl-relativetimeformat/locale-data/zh.js";
