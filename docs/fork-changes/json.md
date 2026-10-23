---
summary: 'The fork change to JSON: JSON.stringify and JSON.parse copy runs of plain characters in one Go copy, with what it gains, why it gives the same strings and errors, and what to do when upstream changes it'
read_when:
  - Changing lib/jsonstr.go or fixJSONStrings in internal/fixlib
  - fixlib stops on jsonSites after an upstream release
  - JSON.stringify or JSON.parse gives another string or error than before
---

# JSON

## JSON copies runs of plain characters

Release: v0.26.17.

`JSON.stringify` and `JSON.parse` copy the plain characters of a string in runs. Both went through a string one character at a time: `JSON.stringify` read each with `string_getc` and wrote it with `string_buffer_putc`, and `JSON.parse` put each byte of its text into the new string the same way, decoding UTF-8 with `unicode_from_utf8`. Now, at the start of each turn of those loops, `jsonQuoteRun` and `jsonStringRun` (`lib/jsonstr.go`) copy the run of characters the loop would put as they are, in one Go copy, and leave the loop the character that ended the run: one to escape, the end of the string, a surrogate (`JSON.stringify` decides how to write it) or a byte that is not valid UTF-8 (`JSON.parse` throws the same error at the same place).

The new string grows and becomes 16-bit as with `string_buffer_write8` and `string_buffer_write16`, so it is the same string, and running out of memory in a run fails the call as before. A run of `JSON.parse` stops at the length of the longest string, so a text of `JS_ParseJSON` with a longer string still throws "string too long". Reviver, replacer, `toJSON`, `space` and the other paths are not touched.

On linux/amd64 without PGO (AMD EPYC 7B13, one core), against 4a4103b (v0.26.16 and 9 later commits), quoting the JSON text of each route's data of tifa's example shop as one string again, as tifa does, takes 41% less time, and `JSON.parse` of its render data 9.5% less, while a document of many short strings and numbers parses in the same time and stringifies in 2% less. JSON is about 1% of a render, so a render can gain at most about 0.3%. The 8 rounds measured 0.2% to 1.7% less, inside the spread of a round. The HTML and the memory are the same.

`fixlib` puts the two calls into all 5 files and takes the length, the 16-bit flag and the offset of the characters of a `JSString` from each file's `JS_ToQuotedString` and `string_get`. Generated Go only, no C hunk (`internal/fixlib/jsonstr.go`).

Tests: `lib/jsonstr_test.go`, whose `testdata/jsonstr.txt` is what Node 24 prints for `testdata/jsonstr.js` (strings of every kind of character at the lengths around the runs, as ropes, substrings and keys, through `stringify`, `parse`, reviver, replacer and `toJSON`), the errors and their positions and the invalid UTF-8 cases against the engine before the change, both runs called directly on buffers of every room, 8-bit and 16-bit (the characters, where the run stops, and a buffer never filled past its room), each way a run grows or widens a buffer failing for lack of memory, and a run past the longest string, `lib/jsonstr_memgrind_test.go` (no C memory left behind) and `internal/fixlib/jsonstr_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If it reports that a function of `jsonFuncs` changed (`internal/fixlib/jsonsites.go`), read them against `lib/jsonstr.go`: `JS_ToQuotedString` must still write every character that is not `"`, `\`, below 0x20 or a surrogate as it is, `json_parse_string` must still put every byte from 0x20 to 0x7f that is not the quote or `\` as it is and decode the others with `unicode_from_utf8`, which must still take the forms `jsonUTF8` takes, and `string_buffer_write8` and `string_buffer_write16` must still grow and widen the buffer as `jsonWrite8` and `jsonWrite16` do. Then run `-json`. If it reports that a loop or `string_get` changed shape, update `reQuoteLoop`, `reParseLoop` or `reJSONWide` in `internal/fixlib/jsonstr.go`, and run `go test -run JSONString ./lib`.
