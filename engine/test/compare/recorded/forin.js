// for in over objects whose prototype chain has enumerable properties or
// not, of every kind of object, with the chain and the object changed while
// the loop runs. Each line is one loop: the keys it gives, in order.
var out = [];
function keys(o) {
  var r = [];
  for (var k in o) r.push(k);
  return r.join();
}
function line(name, o) { out.push(name + ': ' + keys(o)); }

line('plain', { a: 1, b: 2, 3: 3, c: 4 });
line('empty', {});
line('null proto', Object.create(null, { x: { value: 1, enumerable: true } }));
line('proto enumerable', Object.create({ p: 1, q: 2 }, { own: { value: 1, enumerable: true } }));
line('proto shadowed', Object.assign(Object.create({ a: 1, b: 2 }), { b: 3, c: 4 }));
line('proto not enumerable', Object.create(Object.defineProperty({}, 'hidden', { value: 1 }), { own: { value: 1, enumerable: true } }));
var sym = {};
sym[Symbol('s')] = 1;
line('proto enumerable symbol', Object.assign(Object.create(sym), { own: 1 }));
line('proto integer key', Object.assign(Object.create({ 7: 'x', 2: 'y' }), { 1: 'z', own: 1 }));
line('proto getter', Object.assign(Object.create({ get g() { return 1; } }), { own: 1 }));
line('proto hidden getter', Object.assign(Object.create(Object.defineProperty({}, 'g', { get: function () { return 1; } })), { own: 1 }));
class A { m() {} static s() {} }
class B extends A { constructor() { super(); this.x = 1; this.y = 2; } n() {} }
line('class', new B());
function F() { this.own = 1; }
F.prototype.shared = 2;
line('function prototype', new F());
line('deep', Object.create(Object.create(Object.create({ deep: 1 }, { mid: { value: 1, enumerable: true } }))));
line('deep quiet', Object.create(Object.create(Object.create({}))));
line('array', [1, 2, 3]);
var holes = [1, , 3];
holes.extra = 1;
line('array extra', holes);
line('proto array', Object.create([7, 8]));
line('proto empty array', Object.assign(Object.create([]), { own: 1 }));
line('string', 'abc');
line('string object', new String('ab'));
line('proto string object', Object.assign(Object.create(new String('xy')), { own: 1 }));
line('typed array', new Uint8Array(3));
line('arguments', (function () { return arguments; })(1, 2));
line('number', 5);
line('null', null);
line('undefined', undefined);
line('function', function f() {});
line('date', new Date(0));
line('regexp', /a/g);
line('map', new Map([[1, 2]]));
line('frozen', Object.freeze({ a: 1, b: 2 }));
line('error', new Error('e'));

var log = [];
var proxyProto = new Proxy({ t: 1 }, {
  ownKeys: function (t) { log.push('ownKeys'); return Reflect.ownKeys(t); },
  getOwnPropertyDescriptor: function (t, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); },
  getPrototypeOf: function (t) { log.push('getPrototypeOf'); return Reflect.getPrototypeOf(t); },
});
line('proxy proto', Object.assign(Object.create(proxyProto), { own: 1 }));
out.push('proxy log: ' + log.join());
log = [];
line('proxy', new Proxy({ a: 1, b: 2 }, {
  ownKeys: function (t) { log.push('ownKeys'); return Reflect.ownKeys(t); },
  getOwnPropertyDescriptor: function (t, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); },
}));
out.push('proxy log: ' + log.join());

// The chain or the object changes while the loop runs.
var proto = {};
var o = Object.assign(Object.create(proto), { a: 1, b: 2, c: 3 });
var seen = [];
for (var k in o) { seen.push(k); if (k === 'a') proto.late = 1; }
out.push('proto grows: ' + seen.join());
proto = { early: 1 };
o = Object.assign(Object.create(proto), { a: 1, b: 2 });
seen = [];
for (var k in o) { seen.push(k); if (k === 'a') delete proto.early; }
out.push('proto shrinks: ' + seen.join());
o = { a: 1, b: 2, c: 3 };
seen = [];
for (var k in o) { seen.push(k); if (k === 'a') { delete o.b; o.d = 4; } }
out.push('object changes: ' + seen.join());
var p1 = { one: 1 }, p2 = { two: 2 };
o = Object.assign(Object.create(p1), { a: 1 });
seen = [];
for (var k in o) { seen.push(k); Object.setPrototypeOf(o, p2); }
out.push('prototype swapped: ' + seen.join());
o = Object.assign(Object.create(Object.prototype), { a: 1 });
seen = [];
Object.prototype.polluted = 1;
for (var k in o) seen.push(k);
delete Object.prototype.polluted;
for (var k in o) seen.push(k);
out.push('Object.prototype polluted: ' + seen.join());
var counts = [];
for (var n = 0; n < 3; n++) {
  var c = 0;
  for (var k in { a: 1, b: 2, c: 3 }) c++;
  counts.push(c);
}
out.push('again: ' + counts.join());
out.join('\n');
