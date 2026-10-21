# Upstream sources

Every input go-intl generates from, pinned. Changing any pin is its own commit
with the gates re-measured, never folded into a behavior change.

`go run ./internal/regen` fetches every source below, checks it against the
sha256 given here, and regenerates the data from them; its source list and
this file change together.

## The anchor: ICU 78.3

go-intl targets **ICU 78.3**, because that is the ICU that produced the golden
corpus go-intl is held against. Reported by the oracle itself:

```console
$ node -e "console.log(process.versions.icu, process.versions.unicode, process.versions.cldr, process.versions.tz)"
78.3 17.0 48.0 2026c
```

Generating from a CLDR the oracle was not built with turns upstream drift into
corpus differences that look like bugs. Both move together, deliberately, or
neither moves.

Note this differs from ICU4X's own pins (CLDR `49.0.0-ALPHA2`, icuexport
`icu4x/2026-08-31/79.x`). ICU4X is an architectural reference, not a version
authority.

## Pins

| Source | Pin | Feeds | Confirmed |
|---|---|---|---|
| ICU | 78.3 | the anchor; the golden corpus oracle | yes, from node |
| CLDR (`cldr-json`) | **48.2.0** | numbers, dates, units, names, plurals, zones | yes, by the corpus |
| Unicode (UCD) | 17.0.0 | normalization | yes, from node |
| IANA tzdb | 2026c, as ICU's `zoneinfo64` | zone rules (`tzgen`) | yes, from node |
| `icuexportdata` | `icu4x-icuexportdata-78.3.zip` | UCA and tailorings (`collgen`); later properties, case, dictionaries | yes |
| ICU data sources | `icu4c-78.3-data.zip` | the collation tree and defaults (`collgen`); numbering-system entries and en_US_POSIX's number patterns (`numbergen`); calendar glue and interval patterns (`dategen`); the rules of the algorithmic numbering systems (`rbnfgen`); zone names, region names for zones, and the zone metadata (`zonegen`); the locale aliases and extension types, from `misc/metadata.txt`, `keyTypeData.txt` and `timezoneTypes.txt` (`aliasgen`); each service's available locales, from the locale and collation trees, their `LOCALE_DEPS.json` and `misc/plurals.txt`, and the collations, currencies and time zones `supportedValuesOf` lists, from the collation and currency trees, `keyTypeData.txt`, `zoneinfo64.txt` and `timezoneTypes.txt` (`availgen`) | yes |
| ICU's compiled data | `icudt78l.dat` in `icu4c-78.3-sources.tgz` | the break rules and dictionaries (`segmentgen`); the collation types that tailor conjoining jamo, which the export leaves incomplete (`collgen`) | yes: Node carries this data |
| ICU's sources | `icu4c-78.3-sources.tgz` | the root collation's unsafe-backward set, `collunsafe.h` (`collgen`); the host abbreviations' zones, `putil.cpp` (`tzgen`) | yes: Node builds this ICU |
| Unicode (UCD) properties | 17.0.0 `Scripts.txt`, `LineBreak.txt`, `extracted/DerivedGeneralCategory.txt`, `PropList.txt` | the break engines' sets and the Script property (`segmentgen`); the general categories, default ignorables and Hebrew script that currency spacing, number parts and Hebrew lists read (`propgen`) | yes, from node |
| Temporal's crates | `temporal_rs` 0.2.3, `temporal_capi` 0.2.3, `ixdtf` 0.6.4, `timezone_provider` 0.2.3, `zoneinfo64` 0.3.0, `icu_calendar` 2.2.1, `calendrical_calculations` 0.2.4, `icu_calendar_data` 2.2.0, `icu_locale_core` 2.2.0 | the `temporal` package: Temporal's calendars, arithmetic, parsing and zones; Temporal's zone names (`tzidgen`) | yes, from node's `deps/crates/Cargo.lock` at v26.10.0 |
| V8 | as Node v26.10.0 vendors it, `deps/v8` | the `date` package: Date's offset cache, parser and strings, ported; no data | yes, from node |

CLDR is fetched **per component from npm**, not as the 79 MB `json-full.zip`
the release page offers. `cldr-core` is 200 KB and carries all of the
supplemental data, so a generator takes only the package it reads:

```sh
curl -sLO https://registry.npmjs.org/cldr-core/-/cldr-core-48.2.0.tgz
```

| Package | Version | sha256 | Used by |
|---|---|---|---|
| `cldr-core` | 48.2.0 | `5310e0c7a06c1feb83dc8e54c8584bbe0b9c2ea8320172a18d541986b124585d` | `localegen`, `numbergen`, `pluralgen` (plurals, ordinals and plural ranges, vendored), `dategen` (time data and week data, vendored) |
| `cldr-numbers-full` | 48.2.0 | `2d17a1453c559a62112caeed52e0bcfe3cb8539c99d239ae7b7ed4d0827679d9` | `numbergen` |
| `cldr-misc-full` | 48.2.0 | `c6ba8384d7ea8701cf86935db0461379231ddd970cc41c249f4a33b9857ded3c` | `listgen` |
| `cldr-units-full` | 48.2.0 | `754d55f183570c53029a77493302f432fb3e905df35a715f9cb022e2ebcb093c` | `unitgen` |
| `cldr-dates-full` | 48.2.0 | `0256f1cefeca14f7d515be4872dda48fcdd7e75381430f25aafd0143eae5b430` | `reltimegen`, `namegen`, `dategen` |
| `cldr-localenames-full` | 48.2.0 | `7f2ac7fd3b5d90f56ad127f9ed5ae67b58b3e556818f55530b9f47bb81bcde57` | `namegen` |
| `cldr-cal-buddhist-full` | 48.2.0 | `3429cb832bef99a978863f11a6afd8b36f5ce981ab18c51481648cef479400ff` | `dategen` |
| `cldr-cal-persian-full` | 48.2.0 | `937af05a2e84a3a38e9fd6880888c622ff14db6577fc254d71a91d2e0442f41f` | `dategen` |
| `cldr-cal-coptic-full` | 48.2.0 | `abc83085e46f6d3ee1b39f886f2c81da0f6a7aec11f6988f1644f7638c22ea5c` | `dategen` |
| `cldr-cal-ethiopic-full` | 48.2.0 | `0c31d3c862f91bc7495a757e6e845fa0077b29f942d7e19d6715ce694c3a3126` | `dategen` (Ethiopic and Amete Alem) |
| `cldr-cal-indian-full` | 48.2.0 | `13dce446625868d858199117d88cbeee3e15fb520994dbf33f94dbf28cb3ea03` | `dategen` |
| `cldr-cal-islamic-full` | 48.2.0 | `ea139d42f68105a135d72e49eed1eb84d6e6970173b35e83924412c12022b885` | `dategen` (civil and tabular so far) |
| `cldr-cal-roc-full` | 48.2.0 | `c90f6e4b718d1384f11a1b005101f26e6b6573e08274584ce84a51f3e4530983` | `dategen` |
| `cldr-cal-hebrew-full` | 48.2.0 | `b06b8f2834564e843a3bda0410b9cc6152b010a54e08fda34d3530892a0d3e7f` | `dategen` |
| `cldr-cal-japanese-full` | 48.2.0 | `05d2e6709e87349ee8dfbe733b93b2e8a8da375164d838b9aa01fc5d759ec49b` | `dategen`; the eras' start dates come from ICU's `misc/supplementalData.txt` |
| `cldr-cal-chinese-full` | 48.2.0 | `cf6acfa7725a6cbd4fe9504169d2d978a204731730db8b367dacff89e77fcbf2` | `dategen` |
| `cldr-cal-dangi-full` | 48.2.0 | `67171aaa6fe075c0ba3c86c67788a26c8b6f5f595d5fb76276a0fbaddc722e73` | `dategen` |

`dategen` finds each package by its directory's name, `cldr-cal-<name>-full`,
so the packages are unpacked under those names and given in any order.

**What is vendored and what is not.** A supplemental file of a few tens of
kilobytes is vendored beside the generator that reads it, so that generator
runs with nothing to fetch. A per-locale package is not: `cldr-numbers-full` is
37 MB unpacked, so `numbergen` takes the path to an unpacked copy and this file
is what pins which copy.

Other URLs:

- `https://github.com/unicode-org/cldr-json/releases/download/{tag}/cldr-{tag}-json-full.zip`
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4x-icuexportdata-78.3.zip`
  — sha256 `eb63a12439f3fd9199886808275900229a5638fb0ee88d3c3c528eca7b811e60`, 5.6 MB
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-data.zip`
  — sha256 `9d8b3899096aeb83e4e21ef8a40fec9e03b28db18c48452efac882ce25a91e27`, 20 MB.
  `collgen` checks both checksums before reading either archive.
- ICU's time zone update 2026c, `https://raw.githubusercontent.com/unicode-org/icu-data/main/tzdata/icunew/2026c/44/<file>`
  for `zoneinfo64.txt` (sha256 `9e4ac14d6217865fd3d94d288063a214c6dc1e53012f6624aeb70a8a517b30a3`),
  `metaZones.txt` (`55ba858327677222e9526acce34db10bee4fed14ccefc9df5b90452ab76f8c61`),
  `timezoneTypes.txt` (`38b441f390473e353502a3fbd98f46a479fe3aef77d78fb4eef502623db46039`)
  and `windowsZones.txt` (`7addd9b95977b860d540d29796a655b8fe7247a0fdf9641f6f759b5442041312`).
  Node 26 runs tz 2026c (`process.versions.tz`) over ICU 78.3, whose data
  archive carries 2026a; `zonegen`, `aliasgen`, `availgen` and `tzgen` take the
  directory these were downloaded to and read them in place of the
  archive's copies, checking each checksum. Only the metazones differ in
  what go-intl carries: Casablanca and El Aaiun join the Western European
  metazone from 2026-09-20. `tzgen` writes each zone's offsets from
  `zoneinfo64.txt`, with its canonical name from `timezoneTypes.txt`, and
  the Windows zone mapping from `windowsZones.txt`; and, from
  `icu4c-78.3-sources.tgz` (checksum below), `source/common/putil.cpp`'s
  `OFFSET_ZONE_MAPPINGS`, the zones ICU guesses from a host's abbreviations
  where nothing names its zone (`HostAbbreviations`).
- `collgen` reads the same `icudt78l.dat`, through `internal/icudat`, for
  each collation type whose compiled trie tailors a conjoining jamo -- the
  search collations and Korean's `searchjl` -- and takes its table whole:
  `coll/<locale>.res`, `collations/<type>/%%CollationBin`, its UTrie2
  rebuilt as a code point trie and checked against it at every code point.
  `numbergen` reads `locales/en_US_POSIX.txt` from the data archive for
  en_US_POSIX's number patterns and marks, which cldr-json does not carry.
- `segmentgen` reads `icu/source/data/in/icudt78l.dat` from the sources
  archive (checksum below), the prebuilt ICU data Node's full-icu carries,
  for the compiled break rules (`brkitr/*.brk`) and dictionaries
  (`brkitr/*.dict`). The data has no LSTM models, so neither Node nor
  go-intl uses them. It reads the brkitr bundles of the data archive for
  which rules each names, and from the Unicode Character Database 17.0.0
  `https://www.unicode.org/Public/17.0.0/ucd/Scripts.txt` (sha256
  `9f5e50d3abaee7d6ce09480f325c706f485ae3240912527e651954d2d6b035bf`),
  `LineBreak.txt` (`e6a18fa91f8f6a6f8e534b1d3f128c21ada45bfe152eb6b1bcc5e15fd8ac92e6`)
  and `extracted/DerivedGeneralCategory.txt`
  (`d62e5bab70ca74f099343f71224fa051cb1fdd61a1ab45c0488c44cfc0b6102e`).
- `propgen` reads `Scripts.txt` and `extracted/DerivedGeneralCategory.txt`,
  as above, and `https://www.unicode.org/Public/17.0.0/ucd/PropList.txt`
  (sha256 `130dcddcaadaf071008bdfce1e7743e04fdfbc910886f017d9f9ac931d8c64dd`),
  so that what formatting asks of Unicode is Unicode 17, as ICU's, rather
  than the Go toolchain's.
- Temporal's crates, as Node 26.10.0 pins them in `deps/crates/Cargo.lock`
  (which carries no checksums; these are crates.io's), from
  `https://static.crates.io/crates/<name>/<name>-<version>.crate`:
  `icu_calendar` 2.2.1 (sha256 `a2b2acc6263f494f1df50685b53ff8e57869e47d5c6fe39c23d518ae9a4f3e45`),
  `icu_calendar_data` 2.2.0 (`118577bcf3a0fa7c6ac0a7d6e951814da84ee56b9b1f68fb4d8d10b08cefaf4d`),
  `calendrical_calculations` 0.2.4 (`5abbd6eeda6885048d357edc66748eea6e0268e3dd11f326fff5bd248d779c26`)
  `temporal_rs` 0.2.3 (`9a902a45282e5175186b21d355efc92564601efe6e2d92818dc9e333d50bd4de`),
  `temporal_capi` 0.2.3 (`8a2a1f001e756a9f5f2d175a9965c4c0b3a054f09f30de3a75ab49765f2deb36`),
  `ixdtf` 0.6.4 (`84de9d95a6d2547d9b77ee3f25fa0ee32e3c3a6484d47a55adebc0439c077992`),
  `timezone_provider` 0.2.3 (`c48f9b04628a2b813051e4dfe97c65281e49625eabd09ec343190e31e399a8c2`),
  `zoneinfo64` 0.3.0 (`ed6eb2607e906160c457fd573e9297e65029669906b9ac8fb1b5cd5e055f0705`)
  and `icu_locale_core` 2.2.0 (`92219b62b3e2b4d88ac5119f8904c10f8f61bf7e95b640d25ba3075e6cac2c29`).
  They are Temporal's implementation in Node, and the `temporal` package is
  ported from them at these versions, not from the ICU4X checkout, which is
  newer: temporal_rs's types and operations, temporal_capi's conversions
  (built, as Node builds it, with `float64_representable_durations`),
  ixdtf's parser, zoneinfo64's reading of ICU's zone data, and
  icu_locale_core's parsing of a calendar name. `temporalgen` reads
  `icu_calendar`'s crate, after checking its checksum, for the four source
  files that are data: the Chinese, Korean and Qing years and the Umm
  al-Qura years. `tzidgen` reads `timezone_provider`'s the same way, for
  `src/data/iana_normalizer.rs.data`, the zone names Temporal takes and
  their links, built from tz 2025c, into `data/temporalzones.bin`.
- V8 as Node v26.10.0 vendors it, `https://github.com/nodejs/node/tree/v26.10.0/deps/v8`:
  `src/date/date.{h,cc}` (the offset cache, `ToDateString`, the day and
  year arithmetic), `src/date/dateparser*` (`Date.parse`),
  `src/builtins/builtins-date.cc` (the constructor and setters),
  `src/objects/js-objects.cc` (`JSDate`, which reads local time whenever
  its value is set) and `src/objects/intl-objects.cc`
  (`ICUTimezoneCache`). The `date` package is ported from these and reads
  no data of its own: zones and zone names are go-intl's. For Temporal,
  `src/objects/js-temporal-objects.cc` and `src/builtins/builtins-temporal.cc`
  (how V8 reads JavaScript's values and options before it calls
  temporal_rs, and the messages it throws itself), which the `temporal`
  package's Node replay ports and the package leaves to the engine.
- `https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-sources.tgz`
  — sha256 `3a2e7a47604ba702f345878308e6fefeca612ee895cf4a5f222e7955fabfe0c0`, 28 MB.
  Six files are read, and all are vendored in `internal/icusrc`:
  `source/common/localefallback_data.h`, ICU's tables of parent locales and
  default scripts, which ICU's resource fallback consults;
  `source/i18n/islamcal.cpp`, as `islamcal.cpp.txt` since Go refuses C++
  files in a package without cgo, for the Umm al-Qura calendar's tables of
  month lengths and year-start corrections; and `source/common/uloc_tag.cpp`,
  as `uloc_tag.cpp.txt`, for the legacy and redundant tags ICU's parser
  rewrites before anything else; and `source/common/ucurr.cpp`, as
  `ucurr.cpp.txt`, for the list of ISO currencies with their flags; and
  `source/common/uscript_props.cpp`, as `uscript_props.cpp.txt`, for which
  scripts run right to left; and `source/common/loclikely.cpp`, as
  `loclikely.cpp.txt`, for the regions a `-u-rg-` or `-u-sd-` keyword may
  name (`gValidRegionMap`). None is in the data archive.
- `https://www.unicode.org/Public/17.0.0/ucd/UnicodeData.txt` - sha256
  `2e1efc1dcb59c575...`, vendored in `internal/normgen`
- `https://www.unicode.org/Public/17.0.0/ucd/CompositionExclusions.txt` -
  vendored likewise
- `https://github.com/unicode-org/lstm_word_segmentation/releases`

## Which CLDR 48, settled

Node reports ICU 78.3's CLDR as "48.0", and that turned out to be a coarser
label than a version. The pin started at 48.0.0 on the strength of it, and
**the corpus moved it to 48.2.0**.

The evidence was one locale and its spaces. Traditional Chinese joins a date
to a time:

| | `zh-Hant` glue |
|---|---|
| CLDR 48.0.0 | `{1}{0}` - nothing between them |
| CLDR 48.2.0 | a plain space, except a thin space U+2009 after a short date |
| ICU 78.3 | the same as 48.2.0 |

Nothing in 48.0 could produce a space there. Regenerating every table at
48.2.0 took DateTimeFormat from 1,221 of 1,230 to **1,230 of 1,230** and moved
no other service by a single case, which is as clean a confirmation as this
offers.

An earlier version of this section said ICU writes a plain space where CLDR
has the thin one, and `dategen` flattened U+2009 accordingly. The corpus
cases it rested on all used a longer date, whose glue is a plain space in CLDR
already. The legacy `toLocaleString` cases use a short date, and ICU writes
the thin space there: `2024/1/5`, U+2009, `下午3:04:05`.

The version string was never the authority. The corpus was.

One thing that looks like a contradiction and is not: CLDR 48 reports
`_unicodeVersion: "16.0.0"` while ICU 78.3 reports Unicode 17.0. Those describe
different things — CLDR's is the Unicode release its own data was built
against, ICU's is the Unicode release ICU integrated. The authority for
character properties is `uprops` in the 78.3 export, not CLDR's metadata.

## The 78.3 export

ICU publishes most `icuexportdata` artifacts under `icu4x/{date}/{major}.x`
tags, and **there is no 78.x among them** — they run 71, 72, 73, 75, 76, 77, 79.
Looking only there would have concluded the anchor could not be matched.

It is attached to the ICU release itself instead: `release-78.3` carries
`icu4x-icuexportdata-78.3.zip`. So the export is available at exactly the
version that produced the golden corpus, and no compromise between anchor and
export is needed.

Contents, 931 entries:

| Directory | Holds |
|---|---|
| `collation/` | 342 files each under `implicithan/` and `unihan/` |
| `norm/` | `nfd`, `nfkd`, `compositions`, `decompositionex`, `uts46d`, in `fast/` and `small/` |
| `uprops/` | 108 property files, in `fast/` and `small/` |
| `ucase/` | case mapping, in `fast/` and `small/` |
| `segmenter/dictionary/` | `cjdict`, `thaidict`, `khmerdict`, `laodict`, `burmesedict` |

The tailorings go-quickjs carries are all present: `zh_pinyin`, `zh_stroke`,
`zh_unihan`, `ja_standard`, `ja_unihan`, `ko_standard`, `ko_search`,
`de_phonebook`, `es_traditional`, plus `root_standard`, `root_emoji`,
`root_eor` and `root_search`.

**One pinned 5.6 MB artifact replaces three separate ad-hoc inputs**: the
node-scraped collation data, the Perl-derived normalizer tables, and the break
dictionaries currently vendored as loose `.txt` copies from the ICU source tree.

Two choices it seemed to force, both settled:

- **`implicithan` or `unihan`** for how Han characters order: **`unihan`**,
  by evidence. The two flavors differ in the root, and Node sorts U+3400
  before U+9FA0, which only radical-and-stroke order does; implicit order puts
  the URO block, U+9FA0 included, before Extension A. ICU4X ships
  `implicithan` because it is smaller; ICU4C does not.
- **`fast` or `small`** for norm, uprops and ucase: **not a choice after
  all** for anything built so far. The collation tries carry their own type,
  and the normalizer is generated from the Unicode Character Database. It will
  come back if the segmenter reads the export's property tables.

**What the export leaves out of collation**, and where it comes from instead:

- The combining diacritics U+0300-U+034E and the conjoining jamo are not in
  any trie. The root's are in `root_standard_dia` and `root_standard_jamo`,
  and a tailoring that changes the diacritics (Vietnamese, Ewe) has a `dia`
  file of its own.
- A tailoring's jamo are dropped altogether. A collation type whose
  compiled trie tailors any conjoining jamo -- the search collations, Korean
  `searchjl` among them -- is taken whole from ICU's compiled data
  (`icudt78l.dat`, read by `internal/icudat`); its settings, reordering and
  diacritics still come from the export.
- The collation tree - aliases, parents, default types, the installed
  locales - is not in the export. It is in the data archive's `data/coll`:
  `LOCALE_DEPS.json` and each locale's `default`. It is not the ordinary
  tree: CLDR's `parentLocales` has a `collations` section of its own, and ICU
  folds that into these files.

**tzdb is 2026a in ICU 78.3's data archive, but Node 26 runs 2026c**, ICU's
own time zone update. go-intl pins that update, not the archive's copy.

The zone data format matters as well as the version. The tz database gives
Ireland a negative daylight saving in winter; ICU builds `zoneinfo64` from
the rearguard form, where Irish summer is daylight time. go-intl reads
ICU's `zoneinfo64` from the 2026c update Node runs, not Go's zone data or
go-quickjs's bundle, and so agrees with Node about Dublin.

## Normalization

The normalizer is generated from the Unicode Character Database at 17.0.0 by
`internal/normgen`, from its vendored `UnicodeData.txt` and
`CompositionExclusions.txt`. ICU exports its normalizer in `icuexportdata`
too, but in ICU4X's trie encoding, which would have to be implemented to
read; the database is line-based text and says the same thing. Every
character up to U+2FFFF was put through all four forms and compared with
Node: 779,392 normalizations, no differences.

## Where cldr-json is not all of CLDR

CLDR's root uses locale-relative aliases: this value is whatever the *asking*
locale has at that other path. cldr-json resolves such an alias as if it
pointed at the root's own value, or leaves the path out. ICU's resource
bundle sources keep the aliases, and ICU resolves them as CLDR means. Every
place go-intl has met this is read from those sources, from the pinned data
archive, through `internal/icusrc`:

| What | cldr-json | ICU, and so go-intl |
|---|---|---|
| a numbering system a locale does not use | missing | the locale's own few fields, then the root's entry, then the locale's Latin data |
| a calendar's date-time atTime glue | the root's for other calendars; the locale's own plain glue where it overrides only that | the first bundle up the chain that has one, else the Gregorian one |
| zone names | each locale resolved by CLDR's inheritance | merged field by field along ICU's chain; a locale with no zone bundle falls back by ICU's resource rules and ICU's own default scripts, so `sr-Cyrl-ME` reads the Latin `sr_Latn_ME` |

## The space before AM and PM

CLDR separates a time from its day period with U+202F, a narrow no-break
space, in 440 locales, and ICU writes it. **Node writes a plain space**, and
the golden corpus has plain spaces, but that is V8's doing and not ICU's: V8
replaces U+202F in everything it formats (`Replace202F` in
`js-date-time-format.cc`), reverting ICU 72's change for the web's sake.

So the date tables keep CLDR's character, and the NodeICU profile makes V8's
replacement, as a named divergence (`compat.go`). An earlier version of
`dategen` replaced the character in the data itself, on the belief that ICU
did; reading V8 showed otherwise.

CLDR offers "-alt-ascii" patterns that look like they say the same thing and do
not: `en-GB`'s medium time is `HH:mm:ss` and its ascii alternate is
`h:mm:ss a`, which is a different clock rather than a different space.
Preferring those alternates put 75 locales on the wrong clock before the corpus
caught it. Only the character is taken.
