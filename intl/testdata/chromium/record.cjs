// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Records what a Chromium build answers, for the golden test and for the
// generator. It is a maintainer tool: go test and the generator only read
// the files it writes, and need neither node nor a browser.
//
//   node record.cjs golden [name ...]
//       evaluates corpus.json and each corpus-<name>.json (go test -run
//       TestGolden -golden.corpus writes them) and writes golden.json.gz and
//       golden-<name>.json.gz. With names, only those sets: "" is the first.
//   node record.cjs temporal <name>
//       evaluates temporal/testdata/<name>.js and writes <name>.json.
//   node record.cjs probes <name>
//       evaluates testdata/chromium/<name>.js and writes <name>.json.
//   node record.cjs shape
//       evaluates shape.js and writes shape.json: the names, lengths and
//       property attributes of Intl's built-ins (TestBuiltinShape).
//   node record.cjs tamper
//       runs each script of tamper.js in a realm of its own, an iframe, and
//       writes their answers to tamper.json (TestTamper).
//   node record.cjs icudtl
//       prints the path of the browser's ICU data file, for the generator's
//       -icudtl.
//
// PLAYWRIGHT names the playwright or playwright-core package to load and
// CHROMIUM the browser executable; both default to what `npx playwright
// install chromium` leaves behind. The page runs with time zone UTC and
// locale en-US, the defaults intl.Register uses.

'use strict';

const fs = require('fs');
const path = require('path');
const zlib = require('zlib');

const dir = __dirname;

function playwright() {
	return require(process.env.PLAYWRIGHT || 'playwright-core');
}

async function withPage(fn) {
	const { chromium } = playwright();
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

async function golden(names) {
	if (names.length === 0) {
		names = fs.readdirSync(dir).filter((f) => /^corpus(-[\w-]+)?\.json$/.test(f))
			.map((f) => f.replace(/^corpus-?/, '').replace(/\.json$/, ''));
	}
	await withPage(async (page, version) => {
		const ua = await page.evaluate(() => navigator.userAgent);
		for (const name of names) {
			const suffix = name === '' ? '' : '-' + name;
			const corpus = JSON.parse(fs.readFileSync(path.join(dir, 'corpus' + suffix + '.json'), 'utf8'));
			const results = [];
			for (let i = 0; i < corpus.length; i += 2000) {
				results.push(...await evaluate(page, corpus.slice(i, i + 2000)));
			}
			const out = { browser: 'Chromium ' + version, userAgent: ua, cases: corpus.map((e, i) => [e, results[i]]) };
			fs.writeFileSync(path.join(dir, 'golden' + suffix + '.json.gz'), zlib.gzipSync(JSON.stringify(out), { level: 9 }));
			console.log(`${out.browser}: golden${suffix}, ${corpus.length} expressions`);
		}
	});
}

async function probes(target, name) {
    const src = fs.readFileSync(path.join(target, name + '.js'), 'utf8');
    await withPage(async (page, version) => {
        if (!version.startsWith('156.')) throw new Error('expected Chromium 156, got ' + version);
        const corpus = await page.evaluate(src => (0, eval)(src + '\ncases'), src);
        const answers = await evaluate(page, corpus);
        const out = { browser: 'Chromium ' + version, cases: corpus.map((e, i) => [e, answers[i]]) };
        fs.writeFileSync(path.join(target, name + '.json'), JSON.stringify(out, null, '\t') + '\n');
        console.log(out.browser + ': ' + name + ', ' + corpus.length + ' expressions');
    });
}

async function shape() {
	const src = fs.readFileSync(path.join(dir, 'shape.js'), 'utf8');
	await withPage(async (page, version) => {
		const out = await page.evaluate((src) => (0, eval)(src + '\nshape()'), src);
		fs.writeFileSync(path.join(dir, 'shape.json'), out + '\n');
		console.log(`Chromium ${version}: shape.json`);
	});
}

// tamper runs the scripts in iframes, so that what they change stays out of
// the realm that collects the answers.
async function tamper() {
	const src = fs.readFileSync(path.join(dir, 'tamper.js'), 'utf8');
	await withPage(async (page, version) => {
		const out = await page.evaluate((src) => {
			const scripts = (0, eval)(src + '\nscripts');
			const r = {};
			for (const [name, script] of scripts) {
				const frame = document.createElement('iframe');
				document.body.appendChild(frame);
				r[name] = String(frame.contentWindow.eval(script));
				frame.remove();
			}
			return r;
		}, src);
		fs.writeFileSync(path.join(dir, 'tamper.json'), JSON.stringify(out, null, '\t') + '\n');
		console.log(`Chromium ${version}: ${Object.keys(out).length} scripts`);
	});
}

// icudtl prints where the browser keeps icudtl.dat: next to the executable on
// Linux and Windows, in the framework's Resources directory on macOS.
function icudtl() {
	const exe = process.env.CHROMIUM || playwright().chromium.executablePath();
	if (!fs.existsSync(exe)) {
		throw new Error(`no browser at ${exe}: run npx playwright install chromium, or set CHROMIUM`);
	}
	const find = (d, depth) => {
		for (const e of fs.readdirSync(d, { withFileTypes: true })) {
			const p = path.join(d, e.name);
			if (e.name === 'icudtl.dat') {
				return p;
			}
			if (depth > 0 && e.isDirectory() && !e.isSymbolicLink()) {
				const found = find(p, depth - 1);
				if (found) {
					return found;
				}
			}
		}
		return '';
	};
	for (let d = path.dirname(exe), up = 0; up < 3; d = path.dirname(d), up++) {
		const found = find(d, 6);
		if (found) {
			console.log(found);
			return;
		}
	}
	throw new Error(`no icudtl.dat near ${exe}`);
}

(async () => {
	switch (process.argv[2]) {
	case 'icudtl':
		icudtl();
		break;
	case 'golden':
		await golden(process.argv.slice(3));
		break;
	case 'temporal':
		await probes(path.resolve(dir, '../../../temporal/testdata'), process.argv[3]);
		break;
	case 'probes':
		await probes(dir, process.argv[3]);
		break;
	case 'shape':
		await shape();
		break;
	case 'tamper':
		await tamper();
		break;
	default:
		console.error('usage: node record.cjs golden [set] | temporal <name> | probes <name> | shape | tamper | icudtl');
		process.exit(2);
	}
})().catch((err) => {
	console.error(err);
	process.exit(1);
});
