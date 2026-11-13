# Sources of internal/whatwgurl

The URL parser of package `url`. Nothing here keeps state between calls.

## Derived and written here

- Derived from github.com/nlnwa/whatwg-url v0.6.2: `parser.go`, `host.go`, `url.go`, `setters.go` and `codesets.go`. Their structure is the library's: the states of the basic URL parser, the host parser with its IPv4 and IPv6 steps, the serializer and the percent-encode sets. The code was rewritten (it reads UTF-8 in place, keeps no options, follows the standard of 2026 and takes and makes no string longer than `MaxUnits`), so little of its text remains. These files carry the Apache-2.0 header of the library, with a note that the changes are offered under MIT as well, as AGENTS.md asks for a file made from an upstream file.
- Written here, under MIT: `input.go`, `form.go`, `toascii.go`, `idna/encode.go` (the Punycode encoder, see below), and every test and benchmark but `idna/encode_rfc_test.go`, which keeps the encoder of x/net as the reference the tests compare with. The web-platform-tests data is theirs, see [testdata/SOURCES.md](testdata/SOURCES.md).
- Taken from golang.org/x/net and golang.org/x/text, under BSD-3-Clause: `idna/` and `bidi/`, as the table below says.

| Path | From | License |
| --- | --- | --- |
| `parser.go`, `host.go`, `url.go`, `setters.go`, `codesets.go` | The state machine, the host parser and the serializer started from `url/` of github.com/nlnwa/whatwg-url v0.6.2 (commit `605b909`, 2025-03-28), and were rewritten: they read the input as UTF-8 in place, keep no options, no encoding override and no list of validation errors, follow the standard of 2026 and take and make no string longer than `MaxUnits` | Apache-2.0, [LICENSE-WHATWG-URL](LICENSE-WHATWG-URL), for what remains of it. The changes are offered under MIT as well |
| `input.go`, `form.go`, `toascii.go`, `idna/encode.go`, the tests | liustack/quickjs | MIT |
| `idna/` | `idna/` of golang.org/x/net v0.62.0: `idna.go` (one profile, no options, no ToUnicode, NFC from `internal/gointl`, the algorithm of UTS #46 revision 35 only), `punycode.go` (the decoder, without the encoder), `idna/encode_rfc_test.go` (the encoder, as the reference of the tests), `trie.go`, `trieval.go` and `tables17.0.0.go` (as `tables.go`, Unicode 17, without the `go1.27` build tag) | BSD-3-Clause, [LICENSE-GO](LICENSE-GO) |
| `bidi/` | `unicode/bidi/` of golang.org/x/text v0.44.0: `prop.go`, `trieval.go` and `tables17.0.0.go` (without the build tag), and `secure/bidirule/bidirule.go` without `transform` (the check of the Bidi Rule on a string, as `rule.go`) | BSD-3-Clause, [LICENSE-GO](LICENSE-GO) |
| `testdata/` | web-platform-tests, see [testdata/SOURCES.md](testdata/SOURCES.md) | BSD-3-Clause, [testdata/WPT-LICENSE.md](testdata/WPT-LICENSE.md) |

The standard changes of 2024 to 2025 that nlnwa/whatwg-url v0.6.2 lacks are in
the code here: an opaque path keeps its trailing spaces as `%20` when `?` or
`#` follows, `^` is in the path percent-encode set, and an ASCII domain does not
go through IDNA when it is not strict (so `xn--` labels with invalid punycode
are kept as written).

## Differences from the originals

- The Unicode 17 tables are used on every Go release. x/net uses Unicode 15 on a Go before 1.27, which the IdnaTestV2 data of the standard does not match.
- `idna.ToASCII` has one profile: CheckBidi and CheckJoiners on, Transitional off, and CheckHyphens, UseSTD3ASCIIRules and VerifyDnsLength as the caller says (strict or not).
- The punycode decoder keeps x/net's limit of 1024 code points in a label. The standard has none, and a label with more is a failure here.
- An input is an `Input`, which only `NewInput` makes, of at most `MaxUnits` (65,536) UTF-16 units. A URL whose serialization would be longer is refused with `ErrTooLong`, and a setter that would make one leaves the URL as it was. With that, the work of a call is in proportion to its input, but for IDNA. The standard has no such limit.
- A domain that is not ASCII is refused when it is longer than `MaxDomainUnits` (1,024) UTF-16 units after percent-decoding, and `idna.ToASCII` refuses it when it is longer once mapped (as soon as the mapping passes the limit) or once normalized, so that the NFC of `internal/gointl`, the sort of the marks and the Punycode of a label are bounded whatever the mapping makes of it: a code point maps to as many as 18 units (`idna.Widest`, written here, finds the code points that grow the most). The standard has no such limit.
- A URL is kept by package url as its parts (`Parts`, `FromParts`, written here), the record and not its href: a setter can make a record that the parser does not make from its href.
- The Punycode encoder is written here (`idna/encode.go`). The one of RFC 3492 and x/net goes over the label once for each code point that is not ASCII, the square of the label's length when they all differ. This one gives the same output, counting with a Fenwick tree over the positions, in time n log n. `TestEncodeRandom` and `TestEncodeWPT` compare the two.
- The path is kept as its serialization in one buffer (`Path`), not as a list of strings.

## Updating

Take a new IdnaTestV2 or a new Unicode in as a commit of its own: copy `tables17.0.0.go` (the new one) of x/net and of x/text, check the case lists in `testdata/known-failures.txt`, and run `go test ./internal/whatwgurl/...`. A case that passes while it is listed fails the test, so the list only shrinks.
