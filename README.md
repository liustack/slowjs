[![Go Reference](https://pkg.go.dev/badge/github.com/liustack/quickjs.svg)](https://pkg.go.dev/github.com/liustack/quickjs)
[![LiberaPay](https://liberapay.com/assets/widgets/donate.svg)](https://liberapay.com/jnml/donate)
[![receives](https://img.shields.io/liberapay/receives/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)
[![patrons](https://img.shields.io/liberapay/patrons/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)

![logo_png](logo.png)

# quickjs

Package quickjs is a pure Go embeddable Javascript engine. It supports the
ECMA script 14 ([ES2023]) specification including modules, asynchronous
generators, proxies and BigInt.

See also the original [C QuickJS] library.

The engine has no `Intl`: ECMA-402 is not implemented by the C library this
package wraps. Package [quickjs/intl] adds `Intl.DateTimeFormat` and
`Intl.NumberFormat`, en-US over Go's time zone database, for a VM that opts in
by calling its `Register` — once per VM, including one created to replace
another, because `Intl` lives on that VM's global object.

Resolving IANA time zone names needs a zoneinfo database. Go reads the
operating system's, or the copy `time/tzdata` embeds in the binary. That import
belongs in a program's main package rather than in a library, so neither
package makes it for you.

[C QuickJS]: https://bellard.org/quickjs
[quickjs/intl]: https://pkg.go.dev/github.com/liustack/quickjs/intl
[ES2023]: https://tc39.es/ecma262/2023/
