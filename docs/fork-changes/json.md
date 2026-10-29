---
summary: 'The fork change to JSON: JSON.stringify and JSON.parse copy runs of plain characters, with what it gains, why it gives the same strings and errors'
read_when:
  - JSON.stringify or JSON.parse gives another string or error than before
---

# JSON

## JSON copies runs of plain characters

Release: v0.26.17.

`JSON.stringify` and `JSON.parse` copy the plain characters of a string in runs. Both went through a string one character at a time: `JSON.stringify` read each with `string_getc` and wrote it with `string_buffer_putc`, and `JSON.parse` put each byte of its text into the new string the same way, decoding UTF-8 with `unicode_from_utf8`.

The new string grows and becomes 16-bit as with `string_buffer_write8` and `string_buffer_write16`, so it is the same string, and running out of memory in a run fails the call as before. A run of `JSON.parse` stops at the length of the longest string, so a text of `JS_ParseJSON` with a longer string still throws "string too long". Reviver, replacer, `toJSON`, `space` and the other paths are not touched.

On linux/amd64 without PGO (AMD EPYC 7B13, one core), against 4a4103b (v0.26.16 and 9 later commits), quoting the JSON text of each route's data of tifa's example shop as one string again, as tifa does, takes 41% less time, and `JSON.parse` of its render data 9.5% less, while a document of many short strings and numbers parses in the same time and stringifies in 2% less. JSON is about 1% of a render, so a render can gain at most about 0.3%. The 8 rounds measured 0.2% to 1.7% less, inside the spread of a round. The HTML and the memory are the same.

The change is in C (`engine/quickjs.c`): `JS_ToQuotedString` calls `json_quote_run` and `json_parse_string` calls `json_string_run` at the start of each turn of their loops. They write with `string_buffer_write8` and `string_buffer_write16`, and `json_string_run` decodes with `unicode_from_utf8`, as the loop does. `engine/test/compare.sh` runs `testdata/jsonstr.js` and `testdata/jsonstr_errors.js`.

