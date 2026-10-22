// Reads by name (o.x and o.x()) of strings, numbers and arrays, whose keys
// get_field and get_field2 look for on the prototype, and of the values
// next to them that still take JS_GetPropertyInternal. Each line is
// "label: value", as Node prints it too.
(function () {
	const out = []
	const show = (v) => {
		if (typeof v === 'string') return JSON.stringify(v)
		if (typeof v === 'symbol') return v.toString()
		if (typeof v === 'bigint') return v + 'n'
		if (typeof v === 'function') return 'function ' + v.name
		if (Array.isArray(v)) return '[' + v.map(show).join(',') + ']'
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
	const s = 'hello'
	const rope = 'a'.repeat(300) + 'b'.repeat(300)
	const n = 41, f = 1.25

	// Methods and missing keys of strings and numbers.
	t('string method', () => s.charCodeAt(1))
	t('string method get_field', () => { const m = s.toUpperCase; return m.call(s) })
	t('string length', () => s.length)
	t('string missing', () => s.missing)
	t('rope method', () => rope.charCodeAt(450))
	t('rope slice', () => rope.slice(298, 302))
	t('rope missing', () => rope.missing)
	t('int method', () => n.toString(2))
	t('float method', () => f.toFixed(1))
	t('NaN method', () => NaN.toString())
	t('minus zero', () => (-0).toFixed(0))
	t('int missing', () => n.missing)
	t('float missing', () => f.missing)
	t('number constructor', () => n.constructor === Number)
	t('string constructor', () => s.constructor === String)
	t('string __proto__', () => s.__proto__ === String.prototype)
	t('number __proto__', () => n.__proto__ === Number.prototype)
	t('string valueOf', () => s.valueOf())
	t('string hasOwnProperty', () => s.hasOwnProperty('length') + ',' + s.hasOwnProperty('charAt'))
	t('number toString radix', () => (255).toString(16))
	t('string NaN key', () => s.NaN)
	t('string Infinity key', () => s.Infinity)

	// Values that keep the generic path.
	t('bool method', () => true.toString())
	t('bigint method', () => (5n).toString(2))
	t('symbol description', () => Symbol('d').description)
	t('null read', () => null.x)
	t('undefined read', () => undefined.x)
	t('null call', () => null.x())

	// Keys added to the prototypes later, data and accessors.
	Object.prototype.fromObject = 'op'
	t('string from Object.prototype', () => s.fromObject)
	t('number from Object.prototype', () => n.fromObject)
	t('array from Object.prototype', () => [].fromObject)
	String.prototype.sloppyThis = function () { return typeof this }
	String.prototype.strictThis = function () { 'use strict'; return typeof this + ':' + this }
	t('sloppy method this', () => s.sloppyThis())
	t('strict method this', () => s.strictThis())
	t('rope strict method this', () => rope.strictThis().length)
	Object.defineProperty(String.prototype, 'getSloppy', { get() { return typeof this }, configurable: true })
	Object.defineProperty(String.prototype, 'getStrict', { get() { 'use strict'; return typeof this + ':' + this }, configurable: true })
	t('sloppy getter', () => s.getSloppy)
	t('strict getter', () => s.getStrict)
	Object.defineProperty(Number.prototype, 'getStrict', { get() { 'use strict'; return typeof this + ':' + this }, configurable: true })
	t('number strict getter', () => n.getStrict)
	t('float strict getter', () => f.getStrict)
	Object.defineProperty(Object.prototype, 'deepGetter', { get() { 'use strict'; return typeof this }, configurable: true })
	t('string getter on Object.prototype', () => s.deepGetter)
	t('number getter on Object.prototype', () => n.deepGetter)
	t('array getter on Object.prototype', () => [1].deepGetter)
	Object.defineProperty(String.prototype, 'thrower', { get() { throw new RangeError('x') }, configurable: true })
	t('string getter throws', () => s.thrower)
	t('string getter throws on call', () => s.thrower())
	t('string not a function', () => s.missing())

	// Shadowing a built-in method, then taking it away again.
	const charAt = String.prototype.charAt
	String.prototype.charAt = function () { return 'mine' }
	t('shadowed method', () => s.charAt(0))
	String.prototype.charAt = charAt
	t('restored method', () => s.charAt(0))
	delete String.prototype.strictThis
	t('deleted method', () => typeof s.strictThis)
	Number.prototype.toFixed2 = Number.prototype.toFixed
	t('method copied', () => f.toFixed2(3))
	delete Number.prototype.toFixed2
	t('method deleted', () => typeof f.toFixed2)

	// Read-only keys on the prototype: a read sees them, a write does not
	// change them.
	Object.defineProperty(Number.prototype, 'readOnly', { value: 'ro', writable: false, configurable: true })
	t('read-only read', () => n.readOnly)
	t('read-only sloppy write', () => { n.readOnly = 1; return n.readOnly })
	t('read-only strict write', () => { 'use strict'; n.readOnly = 1 })
	t('primitive strict write', () => { 'use strict'; s.other = 1 })

	// The prototype of String.prototype is a proxy, then a frozen object.
	const proxyLog = []
	const proxy = new Proxy(Object.prototype, {
		get(target, key, receiver) {
			if (key === 'viaProxy') { proxyLog.push(typeof receiver); return 'proxied:' + receiver }
			return Reflect.get(target, key, receiver)
		},
	})
	Object.setPrototypeOf(String.prototype, proxy)
	t('string through proxy', () => s.viaProxy)
	t('rope through proxy', () => rope.viaProxy.length)
	t('string method through proxy', () => s.charCodeAt(0))
	t('string Object.prototype through proxy', () => s.fromObject)
	t('proxy receivers', () => proxyLog.join())
	Object.setPrototypeOf(String.prototype, Object.freeze(Object.create(Object.prototype, { frozenKey: { value: 'fk' } })))
	t('string through frozen prototype', () => s.frozenKey)
	t('string past frozen prototype', () => s.fromObject)
	Object.setPrototypeOf(String.prototype, Object.prototype)
	t('string prototype back', () => typeof s.frozenKey + ',' + s.fromObject)

	// Arrays: fast, frozen, sealed, sparse, with element accessors, and
	// typed arrays with keys that are numbers.
	const a = [1, 2, 3]
	t('array method', () => a.map((x) => x * 2))
	t('array method get_field', () => typeof a.push)
	t('array length', () => a.length)
	t('array missing', () => a.missing)
	t('array symbol', () => a[Symbol.iterator] === Array.prototype.values)
	Array.prototype.last = function () { return this[this.length - 1] }
	Object.defineProperty(Array.prototype, 'first', { get() { return this[0] }, configurable: true })
	t('array method added later', () => a.last())
	t('array getter added later', () => a.first)
	t('array Infinity', () => [].Infinity)
	Object.prototype.Infinity = 'infinity'
	Object.prototype.NaN = 'nan'
	t('array Infinity on Object.prototype', () => [].Infinity)
	t('array NaN on Object.prototype', () => [].NaN)
	t('typed array Infinity', () => new Uint8Array(2).Infinity)
	t('typed array NaN', () => new Float64Array(2).NaN)
	t('typed array method', () => new Uint8Array([1, 2]).join('-'))
	t('typed array missing', () => new Int32Array(1).missing)
	t('typed array from Object.prototype', () => new Int32Array(1).fromObject)
	t('string Infinity on Object.prototype', () => s.Infinity)
	t('number NaN on Object.prototype', () => n.NaN)
	const frozen = Object.freeze([4, 5])
	t('frozen array method', () => frozen.join('+'))
	t('frozen array missing', () => frozen.missing)
	const sealed = Object.seal([6, 7])
	t('sealed array method', () => sealed.indexOf(7))
	const sparse = [1, 2]
	sparse[100] = 3
	t('sparse array method', () => sparse.filter(Boolean))
	t('sparse array length', () => sparse.length)
	const withAccessor = [1, 2]
	Object.defineProperty(withAccessor, 0, { get() { return 'g0' } })
	t('array with element accessor', () => withAccessor.join())
	const own = [1, 2]
	own.join = () => 'own join'
	t('array own method', () => own.join())
	const sub = new (class extends Array {})(3)
	t('array subclass method', () => sub.fill(0).length)
	const withProto = [1]
	Object.setPrototypeOf(withProto, { custom() { return 'custom' } })
	t('array with other prototype', () => withProto.custom() + ',' + typeof withProto.map)
	const proxied = [1]
	Object.setPrototypeOf(proxied, new Proxy(Array.prototype, { get(t2, k, r) { return k === 'p' ? 'array proxy:' + Array.isArray(r) : Reflect.get(t2, k, r) } }))
	t('array through proxy', () => proxied.p + ',' + proxied.map((x) => x + 1))

	// arguments, String objects, functions.
	t('arguments', function () { return arguments.length + ',' + arguments.fromObject + ',' + typeof arguments.callee })
	t('strict arguments callee', function () { 'use strict'; return arguments.callee })
	t('mapped arguments', function (x) { return arguments.fromObject + ',' + x })
	const so = new String('obj')
	t('String object method', () => so.charAt(1))
	t('String object length', () => so.length)
	t('String object missing', () => so.missing)
	t('String object getter', () => so.getStrict)
	t('function method', () => typeof (function f() {}).bind)
	t('function name', () => (function named() {}).name)

	// Symbols.
	const sym = Symbol('k')
	Number.prototype[sym] = 'number symbol'
	t('number symbol key', () => n[sym])
	t('string iterator', () => [...'ab'].length + ',' + (s[Symbol.iterator] === String.prototype[Symbol.iterator]))
	t('string toStringTag', () => s[Symbol.toStringTag])

	// with and eval.
	t('with String object', () => { with (new String('xy')) { return charAt(1) + length } })
	t('with array', () => { with ([1, 2]) { return join('-') + length } })
	t('eval', () => eval('"abc".toUpperCase() + (7).toFixed(1)'))
	t('indirect eval', () => (0, eval)('"abc".at(-1) + [9].at(0)'))
	t('new Function', () => new Function('s', 'return s.repeat(2) + s.length')('ab'))

	// Clean up what the lines above added.
	delete Object.prototype.fromObject
	delete Object.prototype.deepGetter
	delete Object.prototype.Infinity
	delete Object.prototype.NaN
	delete String.prototype.sloppyThis
	delete String.prototype.getSloppy
	delete String.prototype.getStrict
	delete String.prototype.thrower
	delete Number.prototype.getStrict
	delete Number.prototype.readOnly
	delete Number.prototype[sym]
	delete Array.prototype.last
	delete Array.prototype.first
	t('after clean up', () => typeof s.getStrict + ',' + typeof n.readOnly + ',' + [].Infinity + ',' + typeof [].last)
	return out.join('\n')
})()
