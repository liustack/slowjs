// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Records which locale a Chromium build resolves each of go-intl's available
// locales to, for every Intl constructor, into qjsdata/chromium.json. The pack
// tool keeps the locales Chromium has, and TestQJSResolveMatchesChromium
// checks the packs answer as Chromium does. It is a maintainer tool, run by
// qjs-sync.sh chromium: the build and go test only read chromium.json.
//
//   node qjs-chromium.cjs <file of tags, one a line>
//
// PLAYWRIGHT names the playwright or playwright-core package to load and
// CHROMIUM the browser executable, as for intl/testdata/chromium/record.cjs.
// The page's locale is en-US, the default a locale nothing matches resolves
// to.

'use strict';

const fs = require('fs');
const path = require('path');

const constructors = ['Collator', 'DateTimeFormat', 'DisplayNames', 'DurationFormat', 'ListFormat',
	'NumberFormat', 'PluralRules', 'RelativeTimeFormat', 'Segmenter'];

async function main(file) {
	const tags = fs.readFileSync(file, 'utf8').split('\n').filter((l) => l !== '');
	const { chromium } = require(process.env.PLAYWRIGHT || 'playwright-core');
	const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROMIUM || undefined });
	try {
		const context = await browser.newContext({ timezoneId: 'UTC', locale: 'en-US' });
		const page = await context.newPage();
		const out = await page.evaluate(([tags, constructors]) => {
			const answer = (f) => {
				try {
					return f();
				} catch (err) {
					return 'ERR:' + err.constructor.name;
				}
			};
			const canonical = {};
			for (const t of tags) {
				const c = answer(() => Intl.getCanonicalLocales(t)[0]);
				if (c !== t) {
					canonical[t] = c;
				}
			}
			// Each constructor's answer where it is not the tag's canonical
			// form: the locales it is available in are the rest.
			const services = {};
			for (const name of constructors) {
				const differ = {};
				for (const t of tags) {
					const got = answer(() => new Intl[name](t, name === 'DisplayNames' ? { type: 'language' } : undefined)
						.resolvedOptions().locale);
					if (got !== (canonical[t] || t)) {
						differ[t] = got;
					}
				}
				services[name] = differ;
			}
			return { defaultLocale: new Intl.NumberFormat().resolvedOptions().locale, canonical, services };
		}, [tags, constructors]);
		const json = {
			browser: 'Chromium ' + browser.version(),
			defaultLocale: out.defaultLocale,
			tags,
			canonical: out.canonical,
			services: out.services,
		};
		const dst = path.join(__dirname, 'qjsdata', 'chromium.json');
		fs.writeFileSync(dst, JSON.stringify(json, null, '\t') + '\n');
		const counts = constructors.map((c) => `${c} ${tags.length - Object.keys(out.services[c]).length}`);
		console.log(`${json.browser}, ${tags.length} tags, available: ${counts.join(', ')}`);
	} finally {
		await browser.close();
	}
}

main(process.argv[2]).catch((err) => {
	console.error(err);
	process.exit(1);
});
