// tamper lists pages that change built-ins before they first use Intl, as
// [name, script] pairs. Each script runs in a realm of its own and answers a
// string. TestTamper runs them on fresh VMs and compares the answers with
// Chromium's, recorded in tamper.json by:
//
//	node record.cjs tamper
var uses = `(() => {
	let r = '';
	// show writes an object out without JSON.stringify, which in QuickJS
	// calls the setters a page puts on Array.prototype.
	function show(o) {
		return Object.keys(o).map((k) => k + ':' + o[k]).join();
	}
	function use(f) {
		let v;
		try {
			v = toString(f());
		} catch (e) {
			// Messages differ between engines, the error does not.
			v = 'error ' + (e instanceof Error ? e.name + (e.message === 'tampered' ? ' tampered' : '') : typeof e);
		}
		r += v + '\\n';
	}
	use(() => new Intl.NumberFormat(['en', 'de'], {style: 'currency', currency: 'EUR'}).format(1234.5));
	use(() => new Intl.NumberFormat('de').formatToParts(-1.5).map((p) => p.type + ':' + p.value).join());
	use(() => show(new Intl.NumberFormat('en').resolvedOptions()));
	use(() => new Intl.NumberFormat('en').format(0.5));
	use(() => new Intl.DateTimeFormat('en', {dateStyle: 'medium', timeZone: 'UTC'}).format(0));
	use(() => new Intl.DateTimeFormat('en', {timeZone: 'UTC'}).formatToParts(0).map((p) => p.type).join());
	use(() => show(new Intl.DateTimeFormat('de', {timeZone: 'UTC'}).resolvedOptions()));
	use(() => new Intl.DateTimeFormat('en', {year: 'numeric', timeZone: 'UTC'}).format() !== '1970');
	use(() => [0, 86400000].map(new Intl.DateTimeFormat('en', {timeZone: 'UTC'}).format).join());
	use(() => new Intl.PluralRules('en', {type: 'ordinal'}).select(2));
	use(() => new Intl.PluralRules('en').selectRange(1, 2));
	use(() => new Intl.PluralRules('fr').resolvedOptions().pluralCategories.join());
	use(() => Intl.getCanonicalLocales(['EN-us', 'de']).join());
	use(() => Intl.PluralRules.supportedLocalesOf(['en', 'de']).join());
	use(() => (1234.5).toLocaleString('de'));
	use(() => (1234.5).toLocaleString());
	use(() => (0.25).toLocaleString('en', {style: 'percent'}));
	use(() => new Date(0).toLocaleString('en', {timeZone: 'UTC'}));
	use(() => new Date(0).toLocaleDateString('de', {timeZone: 'UTC'}));
	use(() => new Date(0).toLocaleTimeString('en', {timeZone: 'UTC'}));
	use(() => (12n).toLocaleString('de'));
	use(() => apply(Intl.NumberFormat.prototype.resolvedOptions, {}, []));
	use(() => apply(Intl.NumberFormat.prototype.formatToParts, new Intl.DateTimeFormat('en'), [1]));
	use(() => apply(getDescriptor(Intl.NumberFormat.prototype, 'format').get, new Intl.PluralRules('en'), []));
	use(() => apply(Intl.PluralRules.prototype.select, create(Intl.PluralRules.prototype), [1]));
	use(() => typeof Intl.NumberFormat.prototype + ' ' + (getPrototypeOf(new Intl.NumberFormat('en')) === Intl.NumberFormat.prototype));
	use(() => ['b', 'a', 'ä', 'A'].sort(new Intl.Collator('de').compare).join());
	use(() => new Intl.Collator('sv', {sensitivity: 'base'}).compare('a', 'á'));
	use(() => show(new Intl.Collator('de-u-co-phonebk').resolvedOptions()));
	use(() => Intl.Collator.supportedLocalesOf(['de', 'xx']).join());
	use(() => 'ä'.localeCompare('z', 'sv') + ' ' + 'ä'.localeCompare('z', 'de'));
	use(() => 'a'.localeCompare('B', undefined, {caseFirst: 'upper'}));
	use(() => apply(getDescriptor(Intl.Collator.prototype, 'compare').get, new Intl.NumberFormat('en'), []));
	return r;
})()`;

var throws = `function () { throw new Error('tampered'); }`;

var tamper = [
	['Array.prototype.push', `Array.prototype.push = ${throws};`],
	['Array.prototype index setters', `
		for (let i = 0; i < 40; i++) {
			Object.defineProperty(Array.prototype, i, {set: ${throws}, configurable: true});
		}`],
	['Object.prototype get and set', `
		Object.prototype.get = ${throws};
		Object.prototype.set = ${throws};`],
	['Object.prototype value and writable', `
		Object.prototype.value = 1;
		Object.prototype.writable = true;`],
	['Object.prototype options', `
		for (const k of ['style', 'currency', 'localeMatcher', 'type', 'timeZone', 'hour12', 'useGrouping',
			'minimumFractionDigits', 'dateStyle', 'year', 'numberingSystem', 'notation', 'sensitivity', 'collation']) {
			Object.defineProperty(Object.prototype, k, {get: ${throws}, set: ${throws}, configurable: true});
		}`],
	['Object.prototype slots', `
		for (const k of ['kind', 'id', 'fn', 'args', 'bound', 'resolve', 'Intl', 'dateToLocaleString', 'numberToLocaleString']) {
			Object.defineProperty(Object.prototype, k, {get: ${throws}, set: ${throws}, configurable: true});
		}`],
	['Function.prototype call apply bind', `
		Function.prototype.call = ${throws};
		Function.prototype.apply = ${throws};
		Function.prototype.bind = ${throws};`],
	['Object and Reflect functions', `
		Object.defineProperty = ${throws};
		Object.getOwnPropertyDescriptor = ${throws};
		Object.create = ${throws};
		Object.setPrototypeOf = ${throws};
		Object.keys = ${throws};
		Reflect.apply = ${throws};
		Reflect.ownKeys = ${throws};
		Reflect.construct = ${throws};`],
	['JSON.parse', `JSON.parse = function () { return {tampered: true}; };`],
	['Map and WeakMap', `
		for (const C of [Map, WeakMap, Set, WeakSet]) {
			for (const k of ['get', 'set', 'has', 'delete', 'add', 'clear']) {
				C.prototype[k] = ${throws};
			}
		}
		Object.defineProperty(Map.prototype, 'size', {get: ${throws}});`],
	['WeakMap.prototype.get forging', `
		const forged = {kind: 'NumberFormat', id: 1, args: [], bound: undefined, fn() { return 1; }};
		WeakMap.prototype.get = function () { return forged; };
		WeakMap.prototype.has = function () { return true; };`],
	['valueOf', `
		Number.prototype.valueOf = function () { return 42; };
		Date.prototype.valueOf = function () { return 0; };
		BigInt.prototype.valueOf = function () { return 1n; };
		Date.prototype.getTime = function () { return 0; };`],
	['Date.now', `Date.now = function () { return 0; };`],
	['String methods', `
		for (const k of ['charAt', 'charCodeAt', 'indexOf', 'slice', 'split', 'toLowerCase', 'replace', 'startsWith']) {
			String.prototype[k] = ${throws};
		}`],
	// QuickJS compiles a template literal into a call of concat.
	['String.prototype.concat', `String.prototype.concat = ${throws};`],
	['Symbol', `
		const toStringTag = Symbol.toStringTag;
		Symbol = function () { throw new Error('tampered'); };
		Symbol.toStringTag = toStringTag;`],
	['global constructors', `
		globalThis.String = ${throws};
		globalThis.Number = ${throws};
		globalThis.Boolean = ${throws};
		globalThis.Object = ${throws};
		globalThis.Reflect = undefined;
		globalThis.WeakMap = ${throws};
		globalThis.Map = ${throws};`],
];

// The scripts. The first line keeps what uses needs.
var keep = 'const toString = String, apply = Reflect.apply, getDescriptor = Object.getOwnPropertyDescriptor, ' +
	'create = Object.create, getPrototypeOf = Object.getPrototypeOf;\n';
var scripts = tamper.map(([name, setup]) => [name, keep + setup + '\n' + uses]);
