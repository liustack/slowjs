// The regexps shiki's JavaScript engine makes for its grammars of bash, css,
// go, html, json, typescript and yaml, run on the code they were made for.
// shikiRegexps (testdata/shiki/regexps.json) and shikiInput (the text of
// testdata/shiki/input.md) are set before this script runs. Each line is
// the index and flags of a regexp, then the error it throws or how many
// matches it found and a hash of them: the regexp is run from every
// position of every line of the input, as a tokenizer of shiki runs it.
// A regexp with more capture groups than the engine allows (254, where
// Node allows 32767) is left out. testdata/shikire.txt is what Node prints
// for it (see TestRegexpShiki).
var lines = shikiInput.split('\n');
// groups counts the capture groups of the pattern src, with the v flag or
// the u flag.
function groups(src, v) {
  var n = 0, depth = 0;
  for (var i = 0; i < src.length; i++) {
    var c = src[i];
    if (c === '\\') i++;
    else if (c === '[' && (depth === 0 || v)) depth++;
    else if (c === ']' && depth > 0) depth--;
    else if (c === '(' && depth === 0 && (src[i + 1] !== '?' || (src[i + 2] === '<' && src[i + 3] !== '=' && src[i + 3] !== '!'))) n++;
  }
  return n;
}
function hash(s, h) {
  for (var i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return h;
}
var out = [], bad = 0, skipped = 0, total = 0;
for (var i = 0; i < shikiRegexps.length; i++) {
  var x = shikiRegexps[i], r, g = groups(x.source, x.flags.indexOf('v') >= 0);
  if (g > 254) {
    skipped++;
    out.push(i + ' /' + x.flags + ' ' + g + ' groups');
    continue;
  }
  try {
    r = new RegExp(x.source, x.flags);
  } catch (e) {
    bad++;
    out.push(i + ' /' + x.flags + ' ' + e.name);
    continue;
  }
  var n = 0, h = 0;
  for (var li = 0; li < lines.length; li++) {
    var s = lines[li] + '\n';
    for (var pos = 0; pos < s.length; pos++) {
      r.lastIndex = pos;
      var m = r.exec(s);
      if (!m) continue;
      n++;
      h = hash(JSON.stringify([li, pos, m.index, r.lastIndex, Array.from(m), m.indices && Array.from(m.indices), m.groups]), h);
      if (!r.global && !r.sticky) break;
    }
  }
  total += n;
  out.push(i + ' /' + x.flags + ' ' + n + ' ' + h);
}
shikiRegexps.length + ' regexps, ' + skipped + ' with too many groups, ' + bad + ' rejected, ' + total + ' matches\n' + out.join('\n');
