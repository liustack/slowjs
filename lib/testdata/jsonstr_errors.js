// The errors JSON.parse throws in and around strings, with the engine's
// own messages and positions. testdata/jsonstr_errors.txt is the transcript
// of the engine before jsonStringRun. Node words these differently.
var bad = ['"abc', '"ab\u0001c"', '"ab\nc"', '"漢\u0000"', '"é\u001f', '"a\\x"', '"a\\u12"',
  '"a\\u12G4"', '"漢\\', '"漢\\u', '"' + 'a'.repeat(100), '"' + '漢'.repeat(100),
  '"' + '漢'.repeat(70) + '\t"', "'abc'", '{"aé:1}', '{"a":"b\u0002"}', '["漢",',
  '"' + 'x'.repeat(63) + 'é\u0003"', '"' + 'é'.repeat(63) + '\u0003"', '{"漢漢":"漢\\q"}',
  '"😀\u0000"', '["ok","éé\\u00"]'];
var out = [];
for (var i = 0; i < bad.length; i++) {
  try {
    JSON.parse(bad[i]);
    out.push(i + ': no error');
  } catch (e) {
    out.push(i + ': ' + e.name + ': ' + e.message + ' at ' + e.lineNumber + ':' + e.columnNumber + ' ' + JSON.stringify(String(e.stack).split('\n')[0]));
  }
}
out.join('\n');
