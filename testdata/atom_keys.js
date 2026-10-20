// Property keys that are atoms, strings built at run time, index strings,
// strings that look like numbers and symbols, through every way a key
// reaches JS_ValueToAtom. testdata/atom_keys.golden is what Node 24 and
// the engine before atomKey both give.
(() => {
	const keys = ["a", "length", "constructor", "toString", "", "0", "-0", "1e3", "01", "1a", "9", "-1",
		"2147483647", "2147483648", "4294967294", "4294967295", "NaN", "Infinity", " 1", "١", "ключ", "1ключ"]
	const show = v => typeof v === "function" ? "fn:" + v.name : typeof v === "symbol" ? v.toString() : JSON.stringify(v) ?? String(v)
	const out = []
	for (let round = 0; round < 2; round++) {
		// round 0 takes the keys as literals (atoms), round 1 as strings built at run time
		const ks = round == 0 ? keys : keys.map(k => ("#" + k).slice(1))
		const o = {}
		ks.forEach((k, i) => { o[k] = i })
		for (const k of ks) {
			const arr = [10, 20, 30]
			const r = [show(k), show(o[k]), k in o, o.hasOwnProperty(k), show(Object.getOwnPropertyDescriptor(o, k)?.value),
				show(arr[k]), k in arr, show("hey"[k]), k in Object("hey"), show(({ [k]: 7 })[k])]
			out.push(r.join(" "))
		}
		out.push("keys " + Object.keys(o).map(show).join(","))
		const fi = []
		for (const k in o) fi.push(k)
		out.push("forin " + fi.map(show).join(","))
		for (const k of ks) delete o[k]
		out.push("after delete " + Object.keys(o).length + " " + ks.filter(k => k in o).map(show).join(","))
		const a = []
		a["1"] = "x"; a["01"] = "y"; a["-0"] = "z"; a["4294967294"] = "big"
		out.push("array " + a.length + " " + Reflect.ownKeys(a).map(show).join(","))
	}
	const s = Symbol("s"), o = { [s]: 1 }
	o[Symbol.iterator] = 2
	out.push("symbol " + o[s] + " " + (s in o) + " " + o.hasOwnProperty(s) + " " + o[Symbol.iterator] + " " + (Symbol.iterator in []) + " " + show(Reflect.ownKeys(o)))
	// the description of a symbol and the key of a registered symbol are
	// strings that share the symbol's atom: as keys they are strings
	const ds = Symbol("desc"), gs = Symbol.for("glob"), q = { [ds]: "sym", [gs]: "gsym" }
	for (const k of [ds.description, Symbol.keyFor(gs)]) q[k] = "str:" + k
	out.push("symbol text " + q[ds] + " " + q[gs] + " " + q[ds.description] + " " + q[Symbol.keyFor(gs)] + " " +
		(ds.description in q) + " " + q.hasOwnProperty(Symbol.keyFor(gs)) + " " + ({ [ds.description]: 1 }).desc + " " +
		show(Reflect.ownKeys(q).map(String)) + " " + (Symbol.for(Symbol.keyFor(gs)) === gs))
	// an atom freed and made again
	for (let i = 0; i < 3; i++) {
		const k = "re" + "built" + (i % 2)
		const p = {}
		p[k] = i
		out.push("rebuilt " + show(k) + " " + p[k] + " " + (k in p) + " " + show(Object.keys(p)))
	}
	return out.join("\n")
})()
