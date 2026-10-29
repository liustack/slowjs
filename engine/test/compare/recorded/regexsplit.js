// Every result of a set of regexps on a set of strings, through every API
// that runs a regexp, from several start positions. The regexps are the
// ones whose matching lreFirstSet and lreSplit change: alternations, groups,
// quantifiers, assertions, lookarounds, back references and every flag
// around them. Each line is the index of the regexp and of the string, and
// a hash of the results, in JSON: run the two in Node to see them. The last
// value is the transcript: its line count and hash, then the lines.
var colors = ['transparent', 'aliceblue', 'antiquewhite', 'aqua', 'aquamarine', 'azure', 'beige', 'bisque', 'black',
  'blanchedalmond', 'blue', 'blueviolet', 'brown', 'burlywood', 'burntsienna', 'cadetblue', 'chartreuse', 'chocolate',
  'coral', 'cornflowerblue', 'cornsilk', 'crimson', 'cyan', 'darkblue', 'darkcyan', 'darkgoldenrod', 'darkgray',
  'darkgreen', 'darkgrey', 'darkkhaki', 'darkmagenta', 'darkolivegreen', 'darkorange', 'darkorchid', 'darkred',
  'darksalmon', 'darkseagreen', 'darkslateblue', 'darkslategray', 'darkslategrey', 'darkturquoise', 'darkviolet',
  'deeppink', 'deepskyblue', 'dimgray', 'dimgrey', 'dodgerblue', 'firebrick', 'floralwhite', 'forestgreen', 'fuchsia',
  'gainsboro', 'ghostwhite', 'gold', 'goldenrod', 'gray', 'green', 'greenyellow', 'grey', 'honeydew', 'hotpink',
  'indianred', 'indigo', 'ivory', 'khaki', 'lavender', 'lavenderblush', 'lawngreen', 'lemonchiffon', 'lightblue',
  'lightcoral', 'lightcyan', 'lightgoldenrodyellow', 'lightgray', 'lightgreen', 'lightgrey', 'lightpink', 'lightsalmon',
  'lightseagreen', 'lightskyblue', 'lightslategray', 'lightslategrey', 'lightsteelblue', 'lightyellow', 'lime',
  'limegreen', 'linen', 'magenta', 'maroon', 'mediumaquamarine', 'mediumblue', 'mediumorchid', 'mediumpurple',
  'mediumseagreen', 'mediumslateblue', 'mediumspringgreen', 'mediumturquoise', 'mediumvioletred', 'midnightblue',
  'mintcream', 'mistyrose', 'moccasin', 'navajowhite', 'navy', 'oldlace', 'olive', 'olivedrab', 'orange', 'orangered',
  'orchid', 'palegoldenrod', 'palegreen', 'paleturquoise', 'palevioletred', 'papayawhip', 'peachpuff', 'peru', 'pink',
  'plum', 'powderblue', 'purple', 'rebeccapurple', 'red', 'rosybrown', 'royalblue', 'saddlebrown', 'salmon',
  'sandybrown', 'seagreen', 'seashell', 'sienna', 'silver', 'skyblue', 'slateblue', 'slategray', 'slategrey', 'snow',
  'springgreen', 'steelblue', 'tan', 'teal', 'thistle', 'tomato', 'turquoise', 'violet', 'wheat', 'white', 'whitesmoke',
  'yellow', 'yellowgreen'];
var number = '[+\\-]?(?:0|[1-9]\\d*)(?:\\.\\d*)?(?:[eE][+\\-]?\\d+)?';
var many = [];
for (var i = 0; i < 700; i++) many.push('w' + i + 'x');
var pats = [
  new RegExp('(' + colors.join('|') + ')(?!\\w)', 'g'), new RegExp('(' + colors.join('|') + ')(?!\\w)'),
  new RegExp('(' + colors.join('|') + ')', 'gi'), new RegExp('(?:' + colors.join('|') + ')\\b', 'y'),
  new RegExp(colors.join('|'), 'gu'), new RegExp(many.join('|'), 'g'),
  /(#(?:[0-9a-f]{2}){2,4}|(#[0-9a-f]{3})|(rgb|hsl)a?\((-?\d+%?[,\s]+){2,3}\s*[\d\.]+%?\))/gi,
  new RegExp(number, 'g'), new RegExp('(' + number + ')(%|[a-z]+)', 'i'),
  /rgba\(([0-9\.-]+), ([0-9\.-]+), ([0-9\.-]+), ([0-9\.-]+)\)/gi, /var\((--[a-zA-Z0-9-_]+),? ?([a-zA-Z0-9 ()%#.,-]+)?\)/,
  /cat|dog|bird/, /cat|dog|bird/g, /cat|dog|bird/gi, /cat|dog|bird/y, /cat|dog|bird/gm, /cat|dog|bird/u, /cat|dog|bird/v,
  /(cat)|(dog)|(bird)/g, /(?<a>cat)|(?<b>dog)/g, /(c)(a)(t)|(d)(o)(g)/g, /x(a|b)y|x(c|d)z/g, /(a|ab)(c|bcd)(d*)/,
  /(?:a|b)+c/g, /(?:ab|cd)*e/g, /(?:ab|cd)+?e/g, /a?b|c/g, /a*b|c/g, /a{2,3}|b{2}/g, /(?:a|)b/g, /(?:|a)b/g, /a||b/g,
  /|a/g, /(?:)/g, /x(?:)y|z/g, /\bfoo|bar\b/g, /^foo|bar$/g, /^foo|bar$/gm, /(?=a)ab|b/g, /(?!a)\w|a/g, /(?<=a)b|c/g,
  /(?<!a)b|c/g, /(?<=x|y)z|w/g, /(?<=(a|b)c)d/g, /(a)\1|b/g, /\1(a)|b/g, /(a)|\1b/g, /(?:(a)|b)\1/g, /k|ſ|s/gi,
  /k|S/gi, /K|s/g, /é|É/gi, /ß|ss/gi, /[A-Z]|[a-z]/gi, /[à-ÿ]|x/gi, /ÿ|y/gi, /K|k/gi, /K|k/giu,
  /[K]|q/gi, /\u{1F600}|a/gu, /\ud83d|a/g, /\ude00|a/g, /[\ud800-\udbff]|a/g, /中|文|字/g, /中文|字/g,
  /a.c|abd/g, /a.c|abd/gs, /a\sc|a\Sd/g, /[^a]x|ax/g, /\w+@|\d+\./g, /(?i:a|B)c/g, /(?i:A)|b/g, /(?-i:a)|b/gi,
  /(?m:^a)|b/g, /a(?=b|c)|d/g, /a(?!b|c)|d/g, /(?:a|b)(?:c|d)(?:e|f)/g, /(?:x|y)?z/g, /(?:x|y)*?z/g,
  /(?:aa|a)(?:aa|a)$/, /((a)|b)+/g, /(a|b)*?c/g, /(?:a+|b+)+$/, /(a)|(b)|(c)|(d)|(e)/dg,
];
var strs = ['', 'a', 'b', 'ab', 'abc', 'abcd', 'abd', 'aabb', 'aaa', 'bbb', 'xay', 'xcz', 'xdz', 'xby', 'cat', 'dog',
  'bird', 'CAT', 'Dog', 'a cat and a dog and a bird', 'concatenate', 'scattered dogma', 'red', 'darkred', 'reddish',
  'xred', 'red blue', 'darkred,navy', 'Red', 'RED and BLUE', 'transparent', 'translateY(0px)', 'translateY(-6px)',
  'rgba(0, 0, 0, 0.12)', '0px 2px 4px rgba(0,0,0,0.2)', '#fff #abcdef #12345678', 'hsl(1,2%,3%) rgb(1 , 2,3)',
  'var(--x, red)', 'var(--main-color)', '1e5 -3.5 +2 .5 0.5e-3', '12px 50% 3em', 'w0x w1xw w699x w700x w69x',
  'foo bar', 'foobar', 'barfoo', 'xfoo\nbar\nfoo', 'zab', 'yz xz wz', 'acd bcd cd', 'aa ba b', 'ab abb ba',
  'k K K s S ſ', 'é É e E', 'ß SS ss ẞ', 'ÿ Ÿ y', '中文字符', '字中文', 'a😀b', '😀\ud83d',
  'x\ude00y', 'abcabc', 'a\nc a\rc abd', 'a c a c aed', 'Bc ac bc AC', 'mail@ 12. x', 'xz yz zz z',
  'aaaa', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaab', 'ababab', 'abcde', 'ace bdf', 'x'.repeat(100) + 'cat',
  'darkslategrey'.repeat(20)];
var out = [], res;
function hash(lines) {
  var h = 0;
  for (var line of lines) for (var i = 0; i < line.length; i++) h = (h * 31 + line.charCodeAt(i)) | 0;
  return h;
}
function rec(x) { res.push(JSON.stringify(x)); }
function safe(f) { try { return f(); } catch (e) { return 'ERR ' + e.name + ': ' + e.message; } }
function arr(m) { return m && [Array.from(m), m.index, m.groups, m.indices && Array.from(m.indices)]; }
for (var pi = 0; pi < pats.length; pi++) {
  var p = pats[pi];
  for (var si = 0; si < strs.length; si++) {
    var s = strs[si], r = new RegExp(p.source, p.flags);
    res = [];
    rec([safe(() => r.test(s)), r.lastIndex]);
    r.lastIndex = 0; rec(safe(() => arr(r.exec(s)))); rec(r.lastIndex);
    r.lastIndex = 2; rec(safe(() => arr(r.exec(s)))); rec(r.lastIndex);
    r.lastIndex = 5; rec(safe(() => arr(r.exec(s)))); rec(r.lastIndex);
    r.lastIndex = 0;
    rec(safe(() => s.search(r)));
    rec(safe(() => s.match(r)));
    rec(safe(() => s.replace(r, '[$&|$1|$`]')));
    rec(safe(() => s.split(r)));
    if (r.global) rec(safe(() => Array.from(s.matchAll(r), m => [m[0], m.index, m[1]])));
    if (r.global) rec(safe(() => s.replaceAll(r, '_')));
    out.push(pi + ' ' + si + ' ' + hash(res));
  }
}
out.length + ' lines, hash ' + hash(out) + '\n' + out.join('\n');
