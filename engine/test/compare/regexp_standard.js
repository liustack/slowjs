// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// replace, replaceAll, match and split with a regexp, when the regexp or
// what its methods read was changed by the script: each flag getter and flags
// replaced (they are read through flags, once a call, the sticky one too), exec
// replaced on RegExp.prototype or on the regexp or deleted, a regexp of another
// realm, a subclass with and without its own exec, Symbol.species and the
// constructor, Symbol.match as a getter, lastIndex as an object with valueOf or
// a number that is not an index, lastIndex that cannot be written, a frozen
// regexp, a regexp with a property of its own, a proxy, and a replacement
// function, a string argument or a limit that changes RegExp.prototype or the
// regexp while the call runs. Each line gives the result and the calls the
// script saw. The file is what Node 24 prints, but for replace: Node reads
// global and unicode, where RegExp.prototype[Symbol.replace] reads flags (ES2025
// 22.2.6.11 step 7, which test262 checks), and so reads every flag getter. The
// lines of a flag getter replaced, of flags that throw and of a string argument
// that replaces flags are what the specification gives, as the engine prints
// them before the fast paths for the calls that replace with a function, and
// the call that replaces with a string reads the getter too.
var out = [];
var log = [];
var P = RegExp.prototype;
var flagNames = ['flags', 'hasIndices', 'global', 'ignoreCase', 'multiline', 'dotAll', 'unicode', 'unicodeSets', 'sticky'];
var saved = {};
flagNames.concat(['exec', 'constructor', Symbol.match]).forEach(function (k) { saved[String(k)] = [k, Object.getOwnPropertyDescriptor(P, k)]; });
var savedSpecies = Object.getOwnPropertyDescriptor(RegExp, Symbol.species);
var realExec = P.exec;

function restore() {
  for (var k in saved) Object.defineProperty(P, saved[k][0], saved[k][1]);
  Object.defineProperty(RegExp, Symbol.species, savedSpecies);
}
function show(v) {
  if (v === null || v === undefined) return String(v);
  if (typeof v === 'string') return JSON.stringify(v);
  if (Array.isArray(v)) {
    var s = '[' + v.map(show).join(',') + ']';
    if ('index' in v) s += '@' + v.index;
    if (v.groups) s += ' groups ' + JSON.stringify(v.groups);
    if (v.indices) s += ' indices ' + JSON.stringify(v.indices);
    return s;
  }
  return String(v);
}
function run(name, f) {
  var r;
  log = [];
  try { r = show(f()); } catch (e) { r = 'throws ' + e.name + (e.name === 'Error' ? ': ' + e.message : ''); }
  restore();
  out.push(name + ': ' + r + (log.length ? ' [' + log.join(' ') + ']' : ''));
}
function fn(m) {
  var a = Array.prototype.slice.call(arguments);
  log.push('fn(' + a.map(function (x) { return typeof x === 'object' && x ? JSON.stringify(x) : String(x); }).join(',') + ')');
  return '<' + m + '>';
}
// the calls on a regexp: every one this file makes, on a fresh regexp each
var ops = [
  ['replace', function (re) { return 'xaby ab a'.replace(re, '[$&$1]'); }],
  ['replace fn', function (re) { return 'xaby ab a'.replace(re, fn); }],
  ['match', function (re) { return 'xaby ab a'.match(re); }],
  ['split', function (re) { return 'xaby ab a'.split(re); }],
];
function each(name, make, before) {
  for (var i = 0; i < ops.length; i++) {
    var op = ops[i];
    run(name + ' ' + op[0], function () {
      var re = make();
      if (before) before(re);
      var r = op[1](re);
      log.push('lastIndex ' + show(re.lastIndex));
      return r;
    });
  }
}

each('plain', function () { return /a(b)?/; });
each('plain g', function () { return /a(b)?/g; });
each('plain y', function () { return /a(b)?/y; }, function (re) { re.lastIndex = 1; });
each('plain gy', function () { return /a(b)?/gy; });
each('plain d', function () { return /a(b)?/d; });

// Every flag getter is read once a call, through flags, also the ones the
// call does not use.
flagNames.forEach(function (k) {
  var get = saved[k][1].get;
  each('getter ' + k, function () { return /a(b)?/g; }, function () {
    Object.defineProperty(P, k, { get: function () { log.push('get ' + k); return get.call(this); }, configurable: true });
  });
});
// A getter that gives another value changes what the call does.
each('global says true', function () { return /a(b)?/y; }, function () {
  Object.defineProperty(P, 'global', { get: function () { return true; }, configurable: true });
});
each('global says false', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(P, 'global', { get: function () { return false; }, configurable: true });
});
each('unicode says true', function () { return /(?:)/g; }, function () {
  Object.defineProperty(P, 'unicode', { get: function () { return true; }, configurable: true });
});
each('flags of another', function () { return /a(b)?/y; }, function () {
  Object.defineProperty(P, 'flags', { get: function () { return 'gy'; }, configurable: true });
});
each('flags throws', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(P, 'flags', { get: function () { throw new Error('no flags'); }, configurable: true });
});
// The sticky getter of another realm's RegExp.prototype, or the getter of
// another flag put in its place.
each('getter of another realm', function () { return /a(b)?/g; }, function () {
  var g = newRealm('Object.getOwnPropertyDescriptor(RegExp.prototype, "sticky").get');
  Object.defineProperty(P, 'sticky', { get: g, configurable: true });
});
each('getter of another flag', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(P, 'sticky', { get: saved.global[1].get, configurable: true });
});

// exec
each('exec replaced', function () { return /a(b)?/g; }, function () {
  P.exec = function (s) { log.push('exec ' + this.lastIndex); return realExec.call(this, s); };
});
each('exec own', function () { return /a(b)?/g; }, function (re) {
  re.exec = function (s) { log.push('own exec'); return realExec.call(this, s); };
});
each('exec deleted', function () { return /a(b)?/g; }, function () { delete P.exec; });
each('exec of another realm', function () { return /a(b)?/; }, function () {
  P.exec = newRealm('RegExp.prototype.exec');
});
each('exec gives a made up result', function () { return /a(b)?/g; }, function () {
  var n = 0;
  P.exec = function () {
    if (n++ > 1) return null;
    this.lastIndex = 4 + n;
    return { length: 2, 0: 'ab', 1: 7, index: 1 + n, groups: undefined };
  };
});
run('exec of another realm, its result', function () {
  P.exec = newRealm('RegExp.prototype.exec');
  var m = 'xab'.match(/a(b)/);
  return Object.getPrototypeOf(m) === Array.prototype;
});

// a regexp of another realm, through this realm's String methods and through
// this realm's RegExp.prototype methods
each('another realm', function () { return newRealm('/a(b)?/g'); });
run('another realm, match result', function () {
  var m = 'xab'.match(newRealm('/a(b)/'));
  return Object.getPrototypeOf(m) === Array.prototype;
});
run('another realm, this realm\'s match', function () {
  var re = newRealm('/a(b)/');
  var m = P[Symbol.match].call(re, 'xab');
  var mg = P[Symbol.match].call(newRealm('/a(b)/g'), 'xab ab');
  return [Object.getPrototypeOf(m) === Array.prototype, Object.getPrototypeOf(mg) === Array.prototype, show(m), show(mg)];
});
run('another realm, this realm\'s split', function () {
  var re = newRealm('var R = RegExp; /,/');
  var a = P[Symbol.split].call(re, 'a,b');
  return [Object.getPrototypeOf(a) === Array.prototype, show(a)];
});
run('another realm, this realm\'s replace', function () {
  return P[Symbol.replace].call(newRealm('/a(b)?/g'), 'xaby ab a', fn);
});

// subclasses
class Sub extends RegExp {}
class SubExec extends RegExp {
  exec(s) { log.push('sub exec ' + this.lastIndex); return super.exec(s); }
}
class SubNew extends RegExp {
  constructor(p, f) { log.push('new ' + f); super(p, f); }
}
each('subclass', function () { return new Sub('a(b)?', 'g'); });
each('subclass with exec', function () { return new SubExec('a(b)?', 'g'); });
each('subclass with a constructor', function () { return new SubNew('a(b)?', 'g'); });

// species and constructor
each('species', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(RegExp, Symbol.species, { get: function () { log.push('species'); return RegExp; }, configurable: true });
});
each('species of a subclass', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(RegExp, Symbol.species, { get: function () { return SubExec; }, configurable: true });
});
each('species of another realm', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(RegExp, Symbol.species, { get: function () { return newRealm('RegExp'); }, configurable: true });
});
each('constructor', function () { return /a(b)?/g; }, function () {
  Object.defineProperty(P, 'constructor', { get: function () { log.push('constructor'); return RegExp; }, configurable: true });
});
each('constructor of a subclass', function () { return /a(b)?/g; }, function () { P.constructor = SubNew; });
each('constructor own', function () { return /a(b)?/g; }, function (re) { re.constructor = SubNew; });
each('constructor undefined', function () { return /a(b)?/g; }, function () { P.constructor = undefined; });
each('constructor not an object', function () { return /a(b)?/g; }, function () { P.constructor = 1; });
each('Symbol.match getter', function () { return /a(b)?/g; }, function () {
  var m = saved['Symbol(Symbol.match)'][1].value;
  Object.defineProperty(P, Symbol.match, { get: function () { log.push('get match'); return m; }, configurable: true });
});
each('Symbol.match false', function () { return /a(b)?/g; }, function () { P[Symbol.match] = false; });

// lastIndex
each('lastIndex valueOf', function () { return /a(b)?/; }, function (re) {
  re.lastIndex = { valueOf: function () { log.push('valueOf'); return 0; } };
});
each('lastIndex valueOf, g', function () { return /a(b)?/g; }, function (re) {
  re.lastIndex = { valueOf: function () { log.push('valueOf'); return 0; } };
});
each('lastIndex valueOf, y', function () { return /a(b)?/y; }, function (re) {
  re.lastIndex = { valueOf: function () { log.push('valueOf'); return 1; } };
});
[-5, 1.5, '1', 2 ** 53, -0, NaN, Infinity, 1e10].forEach(function (v) {
  each('lastIndex ' + show(v) + ', y', function () { return /a(b)?/y; }, function (re) { re.lastIndex = v; });
  each('lastIndex ' + show(v), function () { return /a(b)?/; }, function (re) { re.lastIndex = v; });
});
each('lastIndex not writable', function () { return /a(b)?/; }, function (re) {
  Object.defineProperty(re, 'lastIndex', { writable: false });
});
each('lastIndex not writable, g', function () { return /a(b)?/g; }, function (re) {
  Object.defineProperty(re, 'lastIndex', { writable: false });
});
each('lastIndex not writable, y', function () { return /a(b)?/y; }, function (re) {
  Object.defineProperty(re, 'lastIndex', { value: 1, writable: false });
});
each('frozen', function () { return Object.freeze(/a(b)?/); });
each('frozen g', function () { return Object.freeze(/a(b)?/g); });
each('own property', function () { return /a(b)?/g; }, function (re) { re.x = 1; });
each('prototype with a new property', function () { return /a(b)?/g; }, function () { P.zz = 1; });
each('prototype changed', function () { return /a(b)?/g; }, function (re) { Object.setPrototypeOf(re, Object.create(P)); });
each('proxy', function () {
  return new Proxy(/a(b)?/g, {
    get: function (t, k) { log.push('get ' + String(k)); var v = t[k]; return typeof v === 'function' ? v.bind(t) : v; },
    set: function (t, k, v) { log.push('set ' + String(k) + ' ' + show(v)); t[k] = v; return true; },
  });
});
run('after all', function () { return 'xaby ab a'.replace(/a(b)?/g, fn) + ' ' + show('ab'.split(/(?:)/)); });

// What runs while the call runs.
run('fn compiles the regexp', function () {
  var re = /a/g;
  return 'aaa'.replace(re, function (m, i) { re.compile('b', 'g'); log.push(i + ' ' + re.lastIndex); return 'x'; }) + ' ' + re.source;
});
run('fn reads lastIndex, g', function () {
  var re = /a/g;
  return 'aaa'.replace(re, function (m, i) { log.push(i + ' ' + re.lastIndex); re.lastIndex = 7; return 'x'; }) + ' ' + re.lastIndex;
});
run('fn reads lastIndex, y', function () {
  var re = /a/y;
  re.lastIndex = 1;
  return 'aaa'.replace(re, function (m, i) { log.push(i + ' ' + re.lastIndex); return 'x'; }) + ' ' + re.lastIndex;
});
run('fn replaces exec', function () {
  var re = /a/g, n = 0;
  var r = 'aaa'.replace(re, function () { P.exec = function () { log.push('new exec'); return null; }; return 'x' + n++; });
  return r + ' ' + 'aaa'.replace(re, 'y');
});
run('fn throws', function () {
  var re = /a/g;
  try { 'aaa'.replace(re, function (m, i) { if (i === 1) throw new Error('stop'); return 'x'; }); } catch (e) { log.push(e.message); }
  return re.lastIndex;
});
run('fn gives an object', function () {
  return 'aXa'.replace(/a/g, function (m, i) { return { toString: function () { log.push('toString ' + i); return '$&' + i; } }; });
});
run('string argument replaces flags', function () {
  var re = /a/y;
  return re[Symbol.replace]({ toString: function () {
    Object.defineProperty(P, 'flags', { get: function () { log.push('new flags'); return 'g'; }, configurable: true });
    return 'aaa';
  } }, 'x');
});
run('string argument replaces exec', function () {
  var re = /a/g;
  return re[Symbol.match]({ toString: function () {
    P.exec = function (s) { log.push('new exec ' + this.lastIndex); return realExec.call(this, s); };
    return 'aa';
  } });
});
run('string argument of split replaces exec', function () {
  return /,/[Symbol.split]({ toString: function () {
    P.exec = function (s) { log.push('new exec ' + this.lastIndex); return realExec.call(this, s); };
    return 'a,b';
  } });
});
run('limit replaces exec', function () {
  return 'a,b,c'.split(/,/, { valueOf: function () {
    P.exec = function (s) { log.push('new exec ' + this.lastIndex); return realExec.call(this, s); };
    return 2;
  } });
});
run('limit replaces flags', function () {
  return 'a,b,c'.split(/,/, { valueOf: function () {
    Object.defineProperty(P, 'flags', { get: function () { log.push('new flags'); return 'y'; }, configurable: true });
    return 5;
  } });
});
run('limit compiles the regexp', function () {
  var re = /,/;
  return 'a,b;c'.split(re, { valueOf: function () { re.compile(';'); return 5; } });
});
run('limit throws', function () {
  return 'a,b'.split(/,/, { valueOf: function () { throw new Error('limit'); } });
});
run('the replacement of replaceAll', function () {
  return 'xaby ab a'.replaceAll(/a(b)?/g, fn);
});
run('replaceAll without g', function () {
  return 'xaby ab a'.replaceAll(/a(b)?/, fn);
});
out.push(out.length + ' lines');
out.join('\n');
