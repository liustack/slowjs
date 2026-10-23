// The own keys of objects as every way of listing them gives them: for in,
// Object.keys, values and entries, getOwnPropertyNames and Symbols,
// Reflect.ownKeys, object spread and rest, Object.assign, JSON.stringify and
// getOwnPropertyDescriptors. Integer keys, symbols, private names,
// non-enumerable keys, accessors, deleted keys, changed flags, frozen and
// sealed objects, prototype chains, exotic objects, getters that change the
// object while it is listed, and objects that share or leave a shape. Each
// line is "label: value", as Node prints it too.
(function () {
	const out = []
	const show = (v, depth = 0) => {
		if (typeof v === 'string') return JSON.stringify(v)
		if (typeof v === 'symbol') return v.toString()
		if (typeof v === 'function') return 'function ' + v.name
		if (Array.isArray(v)) return '[' + v.map((x) => show(x, depth + 1)).join(',') + ']'
		if (v && typeof v === 'object') {
			if (depth > 2) return '{...}'
			const ks = Reflect.ownKeys(v)
			return '{' + ks.map((k) => {
				const d = Object.getOwnPropertyDescriptor(v, k)
				const f = (d.enumerable ? 'e' : '') + (d.configurable ? 'c' : '') + ('value' in d ? (d.writable ? 'w' : '') : 'a')
				return show(k) + '/' + f + ':' + ('value' in d ? show(d.value, depth + 1) : 'get')
			}).join(',') + '}'
		}
		if (Object.is(v, -0)) return '-0'
		return String(v)
	}
	const t = (label, f) => {
		let r
		try {
			r = show(f())
		} catch (e) {
			r = 'throws ' + (e && e.constructor && e.constructor.name)
		}
		out.push(label + ': ' + r)
	}
	const forIn = (o) => {
		const r = []
		for (const k in o) r.push(k)
		return r
	}
	const rest = (o) => {
		const { a, ...r } = o
		return r
	}
	const all = (label, mk) => {
		t(label + ' for in', () => forIn(mk()))
		t(label + ' keys', () => Object.keys(mk()))
		t(label + ' values', () => Object.values(mk()))
		t(label + ' entries', () => Object.entries(mk()))
		t(label + ' names', () => Object.getOwnPropertyNames(mk()))
		t(label + ' symbols', () => Object.getOwnPropertySymbols(mk()))
		t(label + ' ownKeys', () => Reflect.ownKeys(mk()))
		t(label + ' spread', () => ({ ...mk() }))
		t(label + ' spread after', () => ({ ...mk(), after: 1 }))
		t(label + ' spread before', () => ({ before: 1, ...mk() }))
		t(label + ' spread twice', () => ({ ...mk(), ...mk() }))
		t(label + ' rest', () => rest(mk()))
		t(label + ' assign', () => Object.assign({}, mk()))
		t(label + ' json', () => JSON.stringify(mk()))
		t(label + ' descriptors', () => Object.getOwnPropertyDescriptors(mk()))
	}
	const s1 = Symbol('s1'), s2 = Symbol('s2'), reg = Symbol.for('reg')

	all('plain', () => ({ a: 1, b: 'x', c: null, d: undefined, e: { f: 1 } }))
	all('empty', () => ({}))
	all('one', () => ({ a: 1 }))
	all('integer keys', () => ({ b: 1, 2: 2, a: 3, 1: 4, 10: 5, '01': 6, '4294967295': 7, '4294967294': 8, '-1': 9, '1.5': 10, 3000000000: 11, 0: 12, '2147483647': 13, '2147483648': 14 }))
	all('integer keys only', () => ({ 3: 'c', 1: 'a', 2: 'b' }))
	all('symbols', () => ({ a: 1, [s1]: 2, b: 3, [s2]: 4, 1: 5, [reg]: 6, [Symbol.iterator]: 7 }))
	all('symbols last', () => ({ a: 1, b: 2, [s1]: 3, [s2]: 4 }))
	all('symbols only', () => ({ [s2]: 1, [s1]: 2 }))
	all('non-enumerable', () => {
		const o = { a: 1, b: 2, c: 3 }
		Object.defineProperty(o, 'b', { enumerable: false })
		Object.defineProperty(o, 'h', { value: 4, enumerable: false })
		Object.defineProperty(o, s1, { value: 5, enumerable: false })
		return o
	})
	all('accessors', () => ({ a: 1, get b() { return 2 }, set c(v) {}, d: 4 }))
	all('deleted', () => {
		const o = { a: 1, b: 2, c: 3, d: 4 }
		delete o.b
		delete o.d
		o.b = 5
		return o
	})
	all('deleted all', () => {
		const o = { a: 1, b: 2 }
		delete o.a
		delete o.b
		return o
	})
	all('flags changed', () => {
		const o = { a: 1, b: 2, c: 3 }
		Object.defineProperty(o, 'a', { writable: false })
		return o
	})
	all('frozen', () => Object.freeze({ a: 1, b: 2, 1: 3 }))
	all('sealed', () => Object.seal({ a: 1, b: 2 }))
	all('not extensible', () => Object.preventExtensions({ a: 1, b: 2 }))
	all('null prototype', () => {
		const o = Object.create(null)
		o.b = 1
		o.a = 2
		o[1] = 3
		return o
	})
	all('own __proto__', () => JSON.parse('{"x":1,"__proto__":{"y":2},"z":3}'))
	all('defined __proto__', () => {
		const o = { a: 1 }
		Object.defineProperty(o, '__proto__', { value: 5, enumerable: true, writable: true, configurable: true })
		return o
	})
	all('class instance', () => {
		class A {
			#p = 1
			x = 2
			constructor() { this.y = 3 }
			m() {}
		}
		return new A()
	})
	all('private only', () => {
		class B { #p = 1; #q = 2 }
		return new B()
	})
	all('inherits', () => {
		const p = { pa: 1, shadow: 2, hidden: 3 }
		Object.defineProperty(p, 'pn', { value: 4, enumerable: false })
		const o = Object.create(p)
		o.own = 5
		o.shadow = 6
		Object.defineProperty(o, 'hidden', { value: 7, enumerable: false })
		return o
	})
	all('inherits deep', () => {
		const a = { a1: 1, 2: 1 }
		const b = Object.create(a)
		b.b1 = 2
		b[1] = 2
		const c = Object.create(b)
		c.c1 = 3
		c[0] = 3
		c.a1 = 3
		return c
	})
	all('non-enumerable shadows', () => {
		const p = { k: 1, j: 2 }
		const o = Object.create(p)
		Object.defineProperty(o, 'k', { value: 3, enumerable: false })
		return o
	})
	all('many keys', () => {
		const o = {}
		for (let i = 0; i < 70; i++) o['k' + ((i * 37) % 70)] = i
		for (let i = 0; i < 70; i += 3) delete o['k' + i]
		for (let i = 40; i > 0; i -= 7) o[i] = i
		o[s1] = 1
		return o
	})
	all('wide keys', () => ({ 'é': 1, '日本': 2, '٣': 3, '３': 4, '1e3': 5, ' 1': 6, '1 ': 7 }))
	all('array', () => {
		const a = [10, 20, , 40]
		a.x = 1
		a[s1] = 2
		return a
	})
	all('array sparse', () => {
		const a = []
		a[5] = 1
		a[2] = 2
		a.y = 3
		return a
	})
	all('arguments', function () { return (function () { return arguments })(1, 2, 3) })
	all('arguments strict', function () { return (function () { 'use strict'; return arguments })(1, 2) })
	all('string object', () => {
		const s = new String('ab')
		s.x = 1
		s[5] = 2
		return s
	})
	all('typed array', () => {
		const a = new Uint8Array([1, 2, 3])
		a.x = 1
		return a
	})
	all('function', () => {
		function f(a, b) {}
		f.x = 1
		return f
	})
	all('arrow', () => {
		const f = (a) => a
		f.y = 2
		return f
	})
	all('class', () => class C { static s = 1; static m() {} })
	all('error', () => {
		const e = new RangeError('m')
		e.code = 1
		return e
	})
	all('regexp', () => {
		const r = /a/g
		r.x = 1
		return r
	})
	all('date', () => {
		const d = new Date(0)
		d.x = 1
		return d
	})
	all('map', () => {
		const m = new Map([[1, 2]])
		m.x = 1
		return m
	})
	all('boxed number', () => {
		const n = new Number(5)
		n.x = 1
		return n
	})
	all('proxy', () => new Proxy({ a: 1, b: 2, [s1]: 3 }, {}))

	// The traps a proxy sees, in order.
	const trapLog = (label, f) => {
		const log = []
		const target = { a: 1, b: 2, c: 3, [s1]: 4 }
		Object.defineProperty(target, 'h', { value: 5, enumerable: false })
		const p = new Proxy(target, {
			ownKeys(o) { log.push('ownKeys'); return Reflect.ownKeys(o) },
			getOwnPropertyDescriptor(o, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(o, k) },
			get(o, k, r) { log.push('get ' + String(k)); return Reflect.get(o, k, r) },
			has(o, k) { log.push('has ' + String(k)); return Reflect.has(o, k) },
			getPrototypeOf(o) { log.push('proto'); return Reflect.getPrototypeOf(o) },
		})
		t(label, () => [f(p), log])
	}
	trapLog('proxy traps for in', forIn)
	trapLog('proxy traps keys', Object.keys)
	trapLog('proxy traps values', Object.values)
	trapLog('proxy traps entries', Object.entries)
	trapLog('proxy traps spread', (p) => ({ ...p }))
	trapLog('proxy traps rest', rest)
	trapLog('proxy traps assign', (p) => Object.assign({}, p))
	trapLog('proxy traps json', (p) => JSON.stringify(p))
	trapLog('proxy traps names', Object.getOwnPropertyNames)
	trapLog('proxy in chain for in', (p) => forIn(Object.create(p)))

	// Getters that change the object while it is listed.
	const changing = () => {
		const o = {
			a: 1,
			get b() { delete this.c; this.z = 9; Object.defineProperty(this, 'd', { enumerable: false }); return 2 },
			c: 3,
			d: 4,
			e: 5,
		}
		return o
	}
	t('changing spread', () => ({ ...changing() }))
	t('changing rest', () => rest(changing()))
	t('changing assign', () => Object.assign({}, changing()))
	t('changing values', () => Object.values(changing()))
	t('changing entries', () => Object.entries(changing()))
	t('changing json', () => JSON.stringify(changing()))
	t('changing descriptors keys', () => Object.keys(Object.getOwnPropertyDescriptors(changing())))
	t('getter sees target', () => {
		const seen = []
		const src = { a: 1, get b() { seen.push(Object.keys(this)); return 2 }, c: 3 }
		return [{ ...src }, seen]
	})
	t('getter adds to its object', () => {
		const src = { a: 1, get b() { this.later = 1; return 2 } }
		const copy = { ...src }
		return [copy, Object.keys(src)]
	})

	// for in while the object changes.
	t('for in delete ahead', () => {
		const o = { a: 1, b: 2, c: 3, d: 4 }
		const r = []
		for (const k in o) {
			r.push(k)
			if (k === 'a') delete o.c
		}
		return r
	})
	t('for in delete and add back', () => {
		const o = { a: 1, b: 2, c: 3 }
		const r = []
		for (const k in o) {
			r.push(k)
			if (k === 'a') { delete o.b; o.b = 5 }
		}
		return r
	})
	t('for in make non-enumerable ahead', () => {
		const o = { a: 1, b: 2, c: 3 }
		const r = []
		for (const k in o) {
			r.push(k)
			if (k === 'a') Object.defineProperty(o, 'b', { enumerable: false })
		}
		return r
	})
	t('for in proto changes', () => {
		const p = { pa: 1, pb: 2 }
		const o = Object.create(p)
		o.x = 1
		const r = []
		for (const k in o) {
			r.push(k)
			if (k === 'x') delete p.pb
		}
		return r
	})
	t('for in primitive', () => [forIn('ab'), forIn(5), forIn(null), forIn(undefined), forIn(true)])
	t('for in nested same shape', () => {
		const a = { x: 1, y: 2 }, b = { x: 3, y: 4 }
		const r = []
		for (const i in a) for (const j in b) r.push(i + j)
		return r
	})

	// Objects that share a shape, then go their own ways.
	t('shared shape', () => {
		const a = { x: 1, y: 2 }
		const b = { ...a }
		const c = { ...a }
		b.z = 3
		delete c.x
		a.w = 4
		Object.defineProperty(b, 'x', { enumerable: false })
		c.x = 5
		return [a, b, c, Object.keys(a), Object.keys(b), Object.keys(c), { ...b }, { ...c }]
	})
	t('copy of copy', () => {
		const a = { p: 1, q: [1, 2], r: { s: 1 } }
		const b = { ...a }
		const c = { ...b }
		c.r.s = 2
		c.q.push(3)
		return [a, b, c, a.q === c.q]
	})
	t('copy then freeze', () => {
		const a = { p: 1, q: 2 }
		const b = Object.freeze({ ...a })
		a.p = 3
		return [a, b, Object.isFrozen(a), Object.isFrozen(b)]
	})
	t('copy keeps values', () => {
		const o = { n: 1.5, s: 'str', b: true, u: undefined, nl: null, big: 10n ** 20n, sym: s1, f() { return 1 } }
		const c = { ...o }
		return [Object.keys(c), typeof c.big, c.big === o.big, c.sym === s1, c.f === o.f, c.f()]
	})
	t('spread then add', () => {
		const r = []
		for (let i = 0; i < 5; i++) {
			const base = { a: i, b: i }
			const o = { ...base }
			o['k' + (i % 2)] = i
			r.push(Object.keys(o).join())
		}
		return r
	})
	t('spread into setter literal', () => {
		let hit = 0
		Object.defineProperty(Object.prototype, 'trap', { set(v) { hit++ }, configurable: true })
		const o = { ...{ trap: 1, other: 2 } }
		delete Object.prototype.trap
		return [o, hit]
	})
	t('spread nullish and primitives', () => [{ ...null }, { ...undefined }, { ...'ab' }, { ...5 }, { ...[1, 2] }])
	t('rest keys', () => {
		const k = 'b'
		const { [k]: x, a, ...r } = { a: 1, b: 2, c: 3, 1: 4, [s1]: 5 }
		return [x, a, r]
	})
	t('assign setters', () => {
		const log = []
		const target = { set a(v) { log.push('a ' + v) } }
		Object.assign(target, { a: 1, b: 2 })
		return [log, Object.keys(target)]
	})
	t('assign many sources', () => Object.assign({}, { a: 1 }, null, 'xy', { b: 2, a: 3 }, [9]))
	t('fromEntries entries', () => Object.fromEntries(Object.entries({ b: 1, 1: 2, a: 3 })))

	// Many objects of a few shapes, as a render makes them.
	t('shapes in a loop', () => {
		const r = []
		const mk = [
			(i) => ({ id: i, name: 'n' + i, tags: [] }),
			(i) => ({ name: 'n' + i, id: i }),
			(i) => ({ id: i, name: 'n' + i, tags: [], [s1]: i }),
			(i) => { const o = { id: i, name: 'n', extra: 1 }; delete o.extra; return o },
			(i) => ({ 1: i, id: i }),
		]
		for (let i = 0; i < 50; i++) {
			const o = mk[i % mk.length](i)
			const c = { ...o }
			r.push(Object.keys(c).join('|') + ';' + forIn(o).join('|') + ';' + Reflect.ownKeys(c).length)
		}
		return r
	})

	// Random objects built with random steps, listed each way.
	let seed = 12345
	const rnd = (n) => {
		seed = (seed * 1103515245 + 12345) & 0x7fffffff
		return seed % n
	}
	const keys = ['a', 'b', 'c', 'd', '0', '1', '7', '42', 'x', 'children', 'className', s1, s2, reg]
	for (let i = 0; i < 120; i++) {
		const o = {}
		const steps = 1 + rnd(9)
		for (let j = 0; j < steps; j++) {
			const k = keys[rnd(keys.length)]
			switch (rnd(6)) {
				case 0:
				case 1:
				case 2:
					o[k] = j
					break
				case 3:
					delete o[k]
					break
				case 4:
					Object.defineProperty(o, k, { value: j, enumerable: rnd(2) === 0, writable: true, configurable: true })
					break
				case 5:
					Object.defineProperty(o, k, { get() { return j }, enumerable: rnd(2) === 0, configurable: true })
					break
			}
		}
		const c = { ...o }
		out.push('random ' + i + ': ' + [forIn(o), Object.keys(o), Reflect.ownKeys(o).map(String), Reflect.ownKeys(c).map(String), Object.values(c), JSON.stringify(o), Reflect.ownKeys(rest(o)).map(String)].map((x) => show(x)).join(' '))
	}
	return out.join('\n')
})()
