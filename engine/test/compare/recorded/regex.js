// Every result of a set of regexps on a set of strings, through every API
// that runs a regexp, from every start position the APIs take. The last
// value is the transcript: its line count and hash, then one line for each
// result.
var pats = [/["'&<>]/, /["'&<>]/g, /["'&<>]/y, /["'&<>]/u, /["'&<>]/i, /["'&<>]/m, /a/, /a/g, /a/y, /a/i, /é/, /中/, /中/g,
  /^on[A-Z]/, /^on[A-Z]/g, /^on[A-Z]/m, /^a/gm, /^/g, /^$/, /[\x00-\x07\x0b\x0e-\x1f<\u2028\u2029\ud800-\udfff]/, /[\ud800-\udfff]/,
  /[^a]/, /[^a]/g, /[\s\S]/, /[Ā-￿]/, /[ÿ-￿]/, /[￿]/, /[a-z]+/g, /a|b/, /(a)/, /(?:a)b/, /a(?=b)/, /(?<=a)b/g,
  /[%\\\x00-\x1f\x7f]/, /[\s\u0080-￿]/, /\d/, /\d+/g, /[ab]c/g, /aa/g, /x*/g, /<\/?[a-z]/g, /[&"<>]/, /é/i, /[é]/i, /[A-Z]/i, /k/i,
  /[a]/, /[a]/gi, /\u{1F600}/u, /[\u{1F600}-\u{1F64F}]/u, /[😀]/u, /./g, /^\w*$/, /[<>]/g, /[a-c]x/g, /\n/g, /[\n\r]/, /^\d/, /中文/, /[一-龥]/g, /[+\-]?(?:0|[1-9]\d*)(?:\.\d*)?(?:[eE][+\-]?\d+)?/g];
var strs = ['', 'a', 'b', 'ab', 'ba', 'aab', 'bbbbba', 'hello world', 'onClick', 'xonClick', 'on', 'onc', 'a\nonClick', 'x\nab',
  'Tom & "Jerry" <b>', "it's", 'no special chars here at all, just text', 'café é É', '中文字符串', '中', 'abc中def', '😀 smile',
  '\ud800', 'x\udc00y', '￿', 'ÿ', 'kK', 'K', 'Kelvin K', 'x'.repeat(300) + '<', '<'.repeat(5), 'a\u2028b',
  'tab\there', 'ctl\x01', '%41', '12.5e3 -4 +7', 'αβγ', 'mixé<中>"', 'aaaa', 'AbC', 'ß', 'ﬀ', '中'.repeat(40) + '<', '中'.repeat(40) + 'ax', 'Ā', 'axbxcx', 'a\r\nb\n'];
var out = [];
function rec(x) { out.push(typeof x === 'string' ? x : JSON.stringify(x)); }
function safe(f) { try { return f(); } catch (e) { return 'ERR ' + e.name + ': ' + e.message; } }
for (var p of pats) {
  for (var s of strs) {
    var r = new RegExp(p.source, p.flags);
    rec([p.toString(), s.length, safe(() => r.test(s)), r.lastIndex]);
    r.lastIndex = 0;
    rec(safe(() => r.exec(s)));
    rec(r.lastIndex);
    r.lastIndex = 3; rec(safe(() => r.exec(s))); rec(r.lastIndex);
    r.lastIndex = s.length + 1; rec(safe(() => r.exec(s))); rec(r.lastIndex);
    rec(safe(() => s.search(r)));
    rec(safe(() => s.match(r)));
    rec(safe(() => s.replace(r, '[$&]')));
    rec(safe(() => s.split(r)));
    if (r.global) rec(safe(() => Array.from(s.matchAll(r), m => [m[0], m.index])));
    if (r.global) rec(safe(() => s.replaceAll(r, '_')));
  }
}
var h = 0;
for (var line of out) for (var i = 0; i < line.length; i++) h = (h * 31 + line.charCodeAt(i)) | 0;
out.length + ' lines, hash ' + h + '\n' + out.join('\n');
