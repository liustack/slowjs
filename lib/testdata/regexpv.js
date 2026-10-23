// Escapes and punctuators in regexps with the v flag (unicodeSets), and the
// same patterns with other flags. Each line is a pattern, its flags and
// either the name of the error it throws or what it matches: every
// printable ASCII character it matches on its own, then the result of
// exec on a few more strings. Error messages differ between engines, so
// only the name of the error is printed. testdata/regexpv.txt is what Node
// prints for it (see TestRegexpVSemantics).
var punct = '&-!#%,:;<=>@`~';         // ClassSetReservedPunctuator
var syntax = '^$\\.*+?()[]{}|/';       // SyntaxCharacter and /
var double = '&!#$%*+,.:;<=>?@^`~';    // ClassSetReservedDoublePunctuator, by halves
var cases = [];
function add(src, flags, strs) { cases.push([src, flags, strs || []]); }
var allFlags = ['v', 'vi', 'u', ''];

// One escaped character in a class, alone, after a character and
// before one, and negated.
for (var c of punct + syntax + 'bdsw0' + '\'"_ aAzZ') {
  for (var f of allFlags) {
    add('[\\' + c + ']', f);
    add('[x\\' + c + 'y]', f);
    add('[^\\' + c + ']', f);
  }
}
// Each escaped character outside a class.
for (var c of punct + syntax + '\'"_ ') {
  for (var f of allFlags) add('\\' + c, f, ['\\' + c, c, 'a' + c + 'b']);
}
// Each character unescaped in a class, alone and doubled.
for (var c of punct + syntax.replace('\\', '').replace(']', '')) {
  for (var f of allFlags) {
    add('[' + c + ']', f);
    add('[a' + c + c + 'b]', f);
    add('[a\\' + c + c + 'b]', f);
    add('[a' + c + '\\' + c + 'b]', f);
    add('[a\\' + c + '\\' + c + 'b]', f);
  }
}
for (var c of double) {
  for (var f of allFlags) add('[' + c + c + ']', f);
}
// Each character outside a class, alone, doubled and around a class.
for (var c of punct + '/') {
  for (var f of allFlags) {
    add(c, f, [c, c + c, 'a' + c]);
    add(c + c, f, [c + c, c]);
    add(c + '[[a-z]--[c]]', f, [c + 'a', c + 'c']);
    add('[a]' + c, f, ['a' + c]);
  }
}
add('-&&', 'v', ['-&&', '-&']);
add(']', 'v'); add('}', 'v'); add('{', 'v'); add(')', 'v');
// Ranges whose ends are escaped punctuators.
for (var f of allFlags) {
  add('[\\!-\\#]', f); add('[\\#-\\&]', f); add('[\\<-\\@]', f); add('[\\--\\:]', f);
  add('[ -\\&]', f); add('[\\&-z]', f); add('[\\@-\\`]', f); add('[\\#-\\!]', f);
  add('[\\--0]', f); add('[\\---]', f); add('[!-\\-]', f);
}
// Escaped punctuators in set operations, nested classes and strings.
var ops = [
  '[\\w--\\-]', '[\\w--[\\-\\_]]', '[\\w&&\\&]', '[\\&&&\\&]', '[\\&&&[\\&\\!]]', '[[\\&\\!]&&[\\!\\#]]',
  '[[\\&\\!]--[\\!]]', '[\\p{ASCII}--[\\w\\s]--\\&--\\-]', '[\\&--\\&]', '[\\-\\-]', '[\\&\\&]', '[\\&&]',
  '[&\\&]', '[\\!\\!\\!]', '[\\~\\~]', '[\\&--&]', '[\\-&&\\-]', '[[\\-]&&[\\-]]', '[\\&&&&]',
  '[\\q{\\&|\\-|a\\!b}]', '[\\q{\\(|\\)|\\{|\\}|\\/|\\||\\\\}]', '[\\q{(}]', '[\\q{-}]', '[\\q{&&}]',
  '[\\q{\\&\\&}]', '[\\q{a&b}]', '[\\q{!}]', '[\\q{[}]', '[\\q{]}]', '[\\q{\\b}]', '[\\q{\\d}]',
  '[\\q{\\q{a}}]', '[\\q{\\p{L}}]', '[\\q{\\x41\\u0042\\u{43}}]', '[\\q{\\cA}]', '[\\q{\\0}]',
  '[\\q{\\@\\`\\~}]', '[\\q{\\\'}]', '[\\q{\\_}]', '[\\q{\\.\\*\\+\\?\\^\\$}]',
  '[\\&\\;\\p{space}]', '[\\-0-9A-Z_a-z]', '[\\!\\|]', '[^\\&\\;\\p{space}]',
  '(?:[\\&\\;]+)', '^[\\-\\w]+$', '[\\-\\w]+(?=\\:)', '[[\\p{L}--\\p{Lu}]\\&]',
];
var opStrs = ['&', '-', '!', '#', 'a', 'a!b', '(', ')', '/', '|', '\\', '&&', '\b', '\x01', 'ABC', '\0', '@`~',
  '@', "'", '_', '.*+?^$', 'é', 'É', ' ', ';', 'foo-bar:', 'x&y'];
for (var src of ops) for (var f of allFlags) add(src, f, opStrs);

var printable = '';
for (var i = 32; i < 127; i++) printable += String.fromCharCode(i);
var out = [];
for (var k of cases) {
  var src = k[0], flags = k[1], r;
  try {
    r = new RegExp(src, flags);
  } catch (e) {
    out.push(JSON.stringify(src) + ' /' + flags + ' ' + e.name);
    continue;
  }
  var one = new RegExp('^(?:' + src + ')$', flags), hit = '';
  for (var ch of printable + '\b\0\x01\x1fé') if (one.test(ch)) hit += ch;
  var res = k[2].map(function (s) { var m = r.exec(s); return m && [m.index, Array.from(m)]; });
  out.push(JSON.stringify(src) + ' /' + flags + ' ' + JSON.stringify(hit) + ' ' + JSON.stringify(res));
}
out.length + ' lines\n' + out.join('\n');
