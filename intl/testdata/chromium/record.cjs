// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Records what a Chromium build answers, for the golden test and for the
// generator. It is a maintainer tool: go test and the generator only read
// the files it writes, and need neither node nor a browser.
//
//   node record.cjs availability <cldr-json dir>
//       writes availability.json: the locales Chromium supports for each
//       constructor, out of every locale cldr-json has.
//   node record.cjs golden
//       evaluates corpus.json (go test -run TestGolden -golden.corpus writes
//       it) and writes golden.json.gz.
//
// PLAYWRIGHT names the playwright-core package to load and CHROMIUM the
// browser executable; both default to what `npx playwright install chromium`
// leaves behind. The page runs with time zone UTC and locale en-US, the
// defaults intl.Register uses.

'use strict';

const fs = require('fs');
const path = require('path');
const zlib = require('zlib');

const dir = __dirname;

async function withPage(fn) {
	const { chromium } = require(process.env.PLAYWRIGHT || 'playwright-core');
	const browser = await chromium.launch({
		headless: true,
		executablePath: process.env.CHROMIUM || undefined,
	});
	try {
		const context = await browser.newContext({ timezoneId: 'UTC', locale: 'en-US' });
		const page = await context.newPage();
		return await fn(page, browser.version());
	} finally {
		await browser.close();
	}
}

function evaluate(page, exprs) {
	return page.evaluate((exprs) => exprs.map((e) => {
		try {
			return String((0, eval)(e));
		} catch (err) {
			return 'ERR:' + err.constructor.name;
		}
	}), exprs);
}

async function availability(cldr) {
	const read = (f) => JSON.parse(fs.readFileSync(path.join(cldr, f), 'utf8'));
	const set = new Set(read('cldr-core/availableLocales.json').availableLocales.full);
	for (const l of read('cldr-core/defaultContent.json').defaultContent) {
		set.add(l);
	}
	for (const l of [...set]) {
		// V8 also offers a locale with its script dropped: zh-Hans-CN as zh-CN.
		const p = l.split('-');
		if (p.length === 3 && p[1].length === 4) {
			set.add(p[0] + '-' + p[2]);
		}
	}
	const all = JSON.stringify([...set].sort());
	await withPage(async (page, version) => {
		const [nf, dtf, pr] = await evaluate(page, ['NumberFormat', 'DateTimeFormat', 'PluralRules']
			.map((c) => `JSON.stringify(Intl.${c}.supportedLocalesOf(${all}))`));
		const out = {
			browser: 'Chromium ' + version,
			numberFormat: JSON.parse(nf),
			dateTimeFormat: JSON.parse(dtf),
			pluralRules: JSON.parse(pr),
		};
		fs.writeFileSync(path.join(dir, 'availability.json'), JSON.stringify(out, null, '\t') + '\n');
		console.log(`${out.browser}: ${out.numberFormat.length} NumberFormat, ${out.dateTimeFormat.length} DateTimeFormat, ${out.pluralRules.length} PluralRules locales`);
	});
}

async function golden() {
	const corpus = JSON.parse(fs.readFileSync(path.join(dir, 'corpus.json'), 'utf8'));
	await withPage(async (page, version) => {
		const results = [];
		for (let i = 0; i < corpus.length; i += 2000) {
			results.push(...await evaluate(page, corpus.slice(i, i + 2000)));
		}
		const ua = await page.evaluate(() => navigator.userAgent);
		const out = { browser: 'Chromium ' + version, userAgent: ua, cases: corpus.map((e, i) => [e, results[i]]) };
		fs.writeFileSync(path.join(dir, 'golden.json.gz'), zlib.gzipSync(JSON.stringify(out), { level: 9 }));
		console.log(`${out.browser}: ${corpus.length} expressions`);
	});
}

(async () => {
	switch (process.argv[2]) {
	case 'availability':
		await availability(process.argv[3]);
		break;
	case 'golden':
		await golden();
		break;
	default:
		console.error('usage: node record.cjs availability <cldr-json dir> | golden');
		process.exit(2);
	}
})().catch((err) => {
	console.error(err);
	process.exit(1);
});
