// JSON.stringify and JSON.parse of strings made of pieces that each take
// another path: plain ASCII, the characters JSON escapes, Latin-1, wide
// characters, surrogate pairs and lone surrogates, at lengths around the
// chunks the engine copies them in. Strings come as built, as ropes, as
// substrings and as keys. Each line is written so that Node gives the same
// text (testdata/jsonstr.txt was written by Node 24): a string as the hex
// of its code units, an error as its name only.
var pieces = ['a', 'abc', 'xyz0123456789', ' ', '"', '\\', '/', "'", '\n', '\r', '\t', '\b', '\f',
  '\u0000', '\u0001', '\u001f', '\u007f', '\u0080', '\u00e9', '\u00ff', '\u0100', '\u4e2d\u6587',
  '\u6f22', '\uffff', '\ud83d\ude00', '\ud800', '\udc00', '\udbff', '\udfff', '\u2028', '\u2029'];
var seed = 1;
function rnd(n) {
  seed = (seed * 1103515245 + 12345) % 2147483648;
  return seed % n;
}
function make(count, from) {
  var s = '';
  for (var i = 0; i < count; i++) s += pieces[from === undefined ? rnd(pieces.length) : from];
  return s;
}
function hex(s) {
  var out = [];
  for (var i = 0; i < s.length; i++) out.push(s.charCodeAt(i).toString(16));
  return out.join(' ');
}
function show(v) {
  if (typeof v === 'string') return 'string ' + v.length + ': ' + hex(v);
  return typeof v + ' ' + hex(JSON.stringify(v));
}
function tryParse(text) {
  try {
    return show(JSON.parse(text));
  } catch (e) {
    return e.name;
  }
}
// The JSON text of s with every character above 0x7f, and every
// character JSON escapes, written as \uXXXX.
function escaped(s) {
  var out = '"';
  for (var i = 0; i < s.length; i++) {
    var c = s.charCodeAt(i);
    if (c < 0x20 || c > 0x7e || c === 0x22 || c === 0x5c) out += '\\u' + ('000' + c.toString(16)).slice(-4);
    else out += s[i];
  }
  return out + '"';
}
// The JSON text of s with its characters as they are, which only a string
// without '"', '\\' and characters below 0x20 has.
function raw(s) {
  return /["\\\u0000-\u001f]/.test(s) ? null : '"' + s + '"';
}

var strings = [''];
for (var p = 0; p < pieces.length; p++) {
  strings.push(pieces[p], make(3, p), 'ab' + pieces[p] + 'cd', make(70, p));
}
var lengths = [1, 2, 5, 31, 32, 33, 62, 63, 64, 65, 66, 127, 128, 200, 1000];
for (var r = 0; r < 300; r++) strings.push(make(lengths[r % lengths.length]));
// Ropes, substrings and strings that are 8-bit or 16-bit for the same text.
strings.push('x'.repeat(300) + '"y"', ('ab"c' + '\u6f22'.repeat(40)).slice(3), 'abc\u00e9'.repeat(50),
  ('\u0100abc').slice(1).repeat(30), ['p', 'q', '\\', 'r'].join('').repeat(70));

var out = [];
for (var i = 0; i < strings.length; i++) {
  var s = strings[i];
  var q = JSON.stringify(s);
  out.push('stringify ' + i + ': ' + hex(q));
  out.push('back ' + i + ': ' + tryParse(q));
  out.push('escaped ' + i + ': ' + tryParse(escaped(s)));
  var t = raw(s);
  if (t !== null) out.push('raw ' + i + ': ' + tryParse(t));
  var o = {};
  o[s] = s;
  o['k' + i] = [s, i];
  var text = JSON.stringify(o, null, i % 3 === 0 ? 2 : undefined);
  out.push('object ' + i + ': ' + hex(text));
  out.push('object back ' + i + ': ' + tryParse(text));
}

// Duplicate keys, __proto__ and keys with the characters above.
out.push('dup: ' + tryParse('{"a\\u00e9":1,"a\u00e9":2,"\u6f22":3,"\\u6f22":4}'));
out.push('proto: ' + tryParse('{"__proto__":{"x":"\u6f22"},"y":"\\"q\\""}'));
out.push('proto own: ' + Object.prototype.hasOwnProperty.call(JSON.parse('{"__proto__":1}'), '__proto__'));
out.push('reviver: ' + show(JSON.parse('{"a":"x\u6f22y","b":["\\n\u00e9",2]}', function (k, v) {
  return typeof v === 'string' ? v + '|' + k : v;
})));
out.push('replacer: ' + hex(JSON.stringify({ a: 'x"\u6f22', b: ['\ud800', 'y'] }, function (k, v) {
  return typeof v === 'string' ? v + '\\' : v;
}, '\t')));
out.push('toJSON: ' + hex(JSON.stringify({ toJSON: function () { return 'a"b\u00e9\ud83d\ude00'; } })));
out.push('String object: ' + hex(JSON.stringify([new String('a"\u6f22'), Object('b\\')])));

// Errors inside and around strings: the name only, Node words them
// differently (testdata/jsonstr_errors.txt has the engine's messages).
var bad = ['"abc', '"ab\u0001c"', '"ab\nc"', '"\u6f22\u0000"', '"\u00e9\u001f', '"a\\x"', '"a\\u12"',
  '"a\\u12G4"', '"\u6f22\\', '"\u6f22\\u', '"' + 'a'.repeat(100), '"' + '\u6f22'.repeat(100),
  '"' + '\u6f22'.repeat(70) + '\t"', "'abc'", '{"a\u00e9:1}', '{"a":"b\u0002"}', '["\u6f22",'];
for (var b = 0; b < bad.length; b++) out.push('bad ' + b + ': ' + tryParse(bad[b]));
out.join('\n');
