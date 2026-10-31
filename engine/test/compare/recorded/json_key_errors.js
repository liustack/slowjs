// The errors JSON.parse throws around keys, the colon after them and plain
// strings, with the engine's own messages and positions, and when objects
// and arrays nest too deep. json_key_errors.txt is the transcript of the
// engine before keys and plain strings were read straight from the text
// (v0.29.6). Node words these differently.
var deep = 2100;
var bad = ['{"a" 1}', '{"a"::1}', '{"a":}', '{"a":1,}', '{"abc', '{"abc"', '{"abc":',
  '{"abc" ', '{"abc" :', '{"a\u0001":1}', '{"a\tb":1}', '{"a\\x":1}', '{"a\\u00":1}',
  '{a:1}', "{'a':1}", '{1:1}', '{"1":1 "b":2}', '{"a":1}x', '{"a":1,\u000b"b":2}',
  '{"a":1,"b"}', '{"a":1,"b":2', '{"id":"x","name":"y"},', '[{"id":"x"},{"id" "y"}]',
  '[{"id":"x"},{"id":"y",}]', '{"' + 'k'.repeat(70000) + '" 1}', '{"\u00e9":1,"a" 1}',
  '{"a\u0080":1}', '["abcdefghij', '["abcdefghij\u001fk"]', '["abcdefgh\\', '{"a":"b\u0000"}',
  '{\n  "a": 1,\n  "b" 2\n}', '{\r\n"a"\r\n:\r\n1\r\n,\r\n"b"\r\n}',
  '['.repeat(deep) + ']'.repeat(deep), '{"a":'.repeat(deep) + '1' + '}'.repeat(deep),
  '[{"a":'.repeat(deep / 2) + '1' + '}]'.repeat(deep / 2)];
var out = [];
for (var i = 0; i < bad.length; i++) {
  try {
    JSON.parse(bad[i]);
    out.push(i + ': no error');
  } catch (e) {
    out.push(i + ': ' + e.name + ': ' + e.message + ' at ' + e.lineNumber + ':' + e.columnNumber + ' ' + JSON.stringify(String(e.stack).split('\n')[0]));
  }
}
// the deepest nesting that parses
var ok = 0;
for (var d = 1990; d <= 2010; d++) {
  try { JSON.parse('{"a":'.repeat(d) + '1' + '}'.repeat(d)); ok = d } catch (e) {}
}
out.push('deepest: ' + ok);
out.join('\n');
