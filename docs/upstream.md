---
summary: 'The README of upstream modernc.org/quickjs as this fork carries it, with where it comes from, what the fork changed in it, and the notes on Intl and time zone data it gives users'
read_when:
  - Looking for what upstream says about the package, or for its badges and logo
  - Checking what the fork changed in the upstream README
---

# The upstream README

This is the README of `modernc.org/quickjs` by the quickjs-go authors (Jan Mercl and others), from https://gitlab.com/cznic/quickjs at v0.25.0, the release the fork starts from. The fork changed the module path in the badge and the links, and the paragraph on `Intl`, which upstream wrote for its en-US only package. Upstream's own engine module, `modernc.org/libquickjs`, keeps its README in [lib/README.md](../lib/README.md).

---

[![Go Reference](https://pkg.go.dev/badge/github.com/liustack/quickjs.svg)](https://pkg.go.dev/github.com/liustack/quickjs)
[![LiberaPay](https://liberapay.com/assets/widgets/donate.svg)](https://liberapay.com/jnml/donate)
[![receives](https://img.shields.io/liberapay/receives/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)
[![patrons](https://img.shields.io/liberapay/patrons/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)

![logo_png](../logo.png)

## quickjs

Package quickjs is a pure Go embeddable Javascript engine. It supports the ECMA script 14 ([ES2023]) specification including modules, asynchronous generators, proxies and BigInt.

See also the original [C QuickJS] library.

The engine has no `Intl`: ECMA-402 is not implemented by the C library this package wraps. Package [quickjs/intl] adds `Intl.DateTimeFormat`, `Intl.NumberFormat` and `Intl.PluralRules`, in every locale Chromium has and over Go's time zone database, for a VM that opts in by calling its `Register`. Call it once per VM, including one created to replace another, because `Intl` lives on that VM's global object.

Resolving IANA time zone names needs a zoneinfo database. Go reads the operating system's, or the copy `time/tzdata` embeds in the binary. That import belongs in a program's main package rather than in a library, so neither package makes it for you.

[C QuickJS]: https://bellard.org/quickjs
[quickjs/intl]: https://pkg.go.dev/github.com/liustack/quickjs/intl
[ES2023]: https://tc39.es/ecma262/2023/
