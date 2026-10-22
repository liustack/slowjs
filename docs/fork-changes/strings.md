---
summary: 'The fork change to string searches: includes, startsWith, endsWith, indexOf and lastIndexOf skip to where the string is with the search of package bytes, with what it gains, why it gives the same results, and what to do when upstream changes it'
read_when:
  - Changing lib/stringsearch.go or fixStringSearch in internal/fixlib
  - fixlib stops on searchSites after an upstream release
  - String.prototype.includes, indexOf or lastIndexOf gives another result than before, or is slow on a long string
---

# Strings

## Searches skip to where the string is

Release: Unreleased.

`String.prototype.includes`, `startsWith`, `endsWith`, `indexOf` and `lastIndexOf` find the string they look for with the search of package bytes. Upstream compares the whole string with `string_cmp` at one index after the other, and `string_cmp` reads each character of both strings through `string_get`, so a search that finds nothing costs a call and a few character reads for each character of the string it looks through: about 3 ns a character. tifa looks for the marker of a Suspense boundary in the whole HTML of a page after each render, which was about 1% of a render of tifa's example shop.

`fixStringSearch` puts a call of `stringFind` (`lib/stringsearch.go`) at the start of the loops of `js_string_includes` and `js_string_indexOf`. From the index the loop is at, it returns the next index where the string is, in the direction the loop goes, and the loop compares there as before, finds it and ends. When there is none, it returns -1 and the loop ends without a match. When both strings have 8-bit characters, `stringFind` uses `bytes.Index` or `bytes.LastIndex`. When the string it looks through has 16-bit characters, it searches its bytes for the bytes of the other string widened to 16 bits, and takes only a match at an even offset. A string of 8-bit characters longer than 64 looked for in one of 16-bit characters, or the other way round, is compared one character after the other, as `string_cmp` does, with no copy. `startsWith` and `endsWith` compare at one index: there `stringFind` returns that index at once, inlined, and the loop compares.

Why it gives the same results: the loop still makes its own comparison at the index `stringFind` returns, so a match is only ever one `string_cmp` confirms, and `stringFind` returns the first index in the loop's direction where the characters are the same, which is the first one the loop would have found. A string of 16-bit characters may hold only characters below 0x100, so the widths of the two strings do not decide whether there is a match. The strings are read where `string_get` reads them, with the constants `fixJSONStrings` takes from each generated file. `string_indexof`, which `split`, `replace` and `replaceAll` use, is left as it is: it already skips to the next index of the first character, and its strings are mostly short, where a call costs more than it saves.

Looking through 100 KB of HTML that does not hold the string takes 81 µs rather than 320 µs, and 87 µs rather than 271 µs when the HTML has 16-bit characters. A miss in a string of about 20 characters takes 108 ns rather than 164 with `indexOf` and 102 rather than 119 with `includes`, `lastIndexOf` in 1 KB 1.6 µs rather than 3.1, and `startsWith` 102 ns rather than 107 (linux/amd64, AMD EPYC 7B13, one core, without PGO, each a loop in javascript of the same call). In a Go CPU profile of tifa's six shop workbench pages, `String.prototype.includes` went from 1.06% of the samples to 0.27%: its own search from about 0.75% to 0.13%, and what is left is mostly making the rope of the HTML one string, which the render then reads anyway. On the MUI pages of tifa's test app it went from 0.63% to 0.11%. A render of the six shop pages takes 0.995 of the time (0.975 to 1.032 over 6 rounds in 6 code layouts), and the MUI pages 1.003 (0.970 to 1.024), without PGO: the gain is smaller than the spread between code layouts, which alone moves a render by about 2%. No page and no micro benchmark got slower.

Generated Go only, no C hunk (`internal/fixlib/stringsearch.go`). The memory is the same: the widened string is an array of 64 characters on the Go stack.

Tests: `lib/stringsearch_test.go` checks the five methods, with every position argument, and `split`, `replace` and `replaceAll`, against a search written with `charCodeAt` alone, on strings of 8-bit and 16-bit characters, ropes and slices, found and not found, at the ends, more than once and longer than 64 characters, and `stringFind` directly on strings of each width, 16-bit strings that hold only characters below 0x100, characters whose bytes match at odd offsets, and both directions. `lib/stringsearch_memgrind_test.go` checks that no C memory is left behind, and `internal/fixlib/stringsearch_test.go` the rewrite.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If it reports that a function of `searchFuncs` changed (`internal/fixlib/stringsearch.go`), read them against `lib/stringsearch.go`: the loops of `js_string_includes` and `js_string_indexOf` must still compare the whole string with `string_cmp` at each index from `start` to `stop`, both included, by a step of 1 (or `inc`, 1 or -1, in `js_string_indexOf`), and stop at the first match, with the characters of both strings in range. `string_cmp` must still compare the characters `string_get` reads, and `string_get` must still read them as the constants of `fixJSONStrings` say. Then run `-search`. If it reports that a loop changed shape, update `reSearchLoop` in `internal/fixlib/stringsearch.go`, and run `go test -run 'StringSearch|StringFind' ./lib ./internal/fixlib`.
