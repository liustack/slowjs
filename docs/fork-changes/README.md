---
summary: 'Index of every change the fork makes to upstream quickjs and libquickjs: one line per change with the release it shipped in and the doc that has what it does, why it is safe, what it gains and what to do when upstream changes it'
read_when:
  - Asking what this fork changes compared with modernc.org/quickjs and libquickjs
  - Looking for the doc of a fork change before changing it, or after fixlib stopped on it
  - Adding a new fork change, to see where it belongs
---

# Fork changes

The fork starts from quickjs v0.25.0 and libquickjs v0.13.1 (QuickJS release 2026-06-04, commit `3d5e064`). Each doc below takes one area. For each change it says what the change does, why it is safe, what it gains or costs and how that was measured, which tests check it, and, under "When upstream changes it", what to do when `fixlib` stops on it after an upstream release. [upgrading.md](../upgrading.md) has the procedure of an upgrade, and [CHANGELOG.md](../../CHANGELOG.md) what each release changed for users.

Changes to the generated Go are made by `internal/fixlib` (see [upgrading.md](../upgrading.md)). A change that also has a C form is in `lib/internal/patch/quickjs.diff`, so a regeneration of `lib/` keeps it. The changes made for speed have no C form.

| Change | Release | Doc |
| --- | --- | --- |
| No leak when converting a string runs out of memory | v0.26.0 | [uncatchable.md](uncatchable.md#no-leak-when-converting-a-string-runs-out-of-memory) |
| Call reports the exception of the function expression | v0.26.0 | [go-api.md](go-api.md#call-reports-the-exception-of-the-function-expression) |
| Error stacks formatted when first read | v0.26.1 | [errors-and-stacks.md](errors-and-stacks.md#error-stacks-formatted-when-first-read) |
| Intl in every locale Chromium has | v0.26.2 | [time-and-intl.md](time-and-intl.md#intl-in-every-locale-chromium-has) |
| Nested calls keep the outer deadline | v0.26.2 | [uncatchable.md](uncatchable.md#nested-calls-keep-the-outer-deadline) |
| Local time is UTC in every VM | v0.26.3 | [time-and-intl.md](time-and-intl.md#local-time-is-utc-in-every-vm) |
| Fraction digits as ECMA-402 reads them since 2021 | v0.26.4 | [time-and-intl.md](time-and-intl.md#fraction-digits-as-ecma-402-reads-them-since-2021) |
| A string Go cannot convert comes back as the error | v0.26.5 | [go-api.md](go-api.md#a-string-go-cannot-convert-comes-back-as-the-error) |
| Interrupts cannot be caught through promises or iterators | v0.26.6 | [uncatchable.md](uncatchable.md#interrupts-cannot-be-caught-through-promises-or-iterators) |
| Out of memory is uncatchable | v0.26.6 | [uncatchable.md](uncatchable.md#out-of-memory-is-uncatchable) |
| Interrupts and out of memory inside Go functions | v0.26.6 | [uncatchable.md](uncatchable.md#interrupts-and-out-of-memory-inside-go-functions) |
| A panic in a Go function | v0.26.6 | [uncatchable.md](uncatchable.md#a-panic-in-a-go-function) |
| Compiling with little memory left | v0.26.6 | [uncatchable.md](uncatchable.md#compiling-with-little-memory-left) |
| Endless recursion throws RangeError | v0.26.6 | [errors-and-stacks.md](errors-and-stacks.md#endless-recursion-throws-rangeerror) |
| The eval timeout runs on the monotonic clock | v0.26.7 | [uncatchable.md](uncatchable.md#the-eval-timeout-runs-on-the-monotonic-clock) |
| `*Error` says why a call ended | v0.26.7 | [go-api.md](go-api.md#error-says-why-a-call-ended) |
| Valid UTF-8 for lone surrogates | v0.26.8 | [go-api.md](go-api.md#valid-utf-8-for-lone-surrogates) |
| Daylight saving time named as ICU does | v0.26.9 | [time-and-intl.md](time-and-intl.md#daylight-saving-time-named-as-icu-does) |
| Interrupt is safe during and after Close | v0.26.10 | [uncatchable.md](uncatchable.md#interrupt-is-safe-during-and-after-close) |
| Invalid UTF-8 from Go becomes one U+FFFD per byte | v0.26.10 | [go-api.md](go-api.md#invalid-utf-8-from-go-becomes-one-ufffd-per-byte) |
| RegisterFunc no longer aborts on an existing name | v0.26.11 | [go-api.md](go-api.md#registerfunc-no-longer-aborts-on-an-existing-name) |
| Calls allocate no Go memory | v0.26.12 | [calls-and-tls-frames.md](calls-and-tls-frames.md#calls-allocate-no-go-memory) |
| Less stack work in the interpreter loop | v0.26.13 | [interpreter-loop.md](interpreter-loop.md#less-stack-work-in-the-interpreter-loop) |
| New properties of plain objects skip the generic path | v0.26.13 | [objects-and-shapes.md](objects-and-shapes.md#new-properties-of-plain-objects-skip-the-generic-path) |
| NewVM leaks nothing when it fails | v0.26.14 | [go-api.md](go-api.md#newvm-leaks-nothing-when-it-fails) |
| Frames freed without defer | v0.26.15 | [calls-and-tls-frames.md](calls-and-tls-frames.md#frames-freed-without-defer) |
| Skip to where a regexp can match | v0.26.15 | [regexp.md](regexp.md#skip-to-where-a-regexp-can-match) |
| Cold paths of === and key names | v0.26.15 | [calls-and-tls-frames.md](calls-and-tls-frames.md#cold-paths-of--and-key-names) |
| One block of TLS stack per VM outside linux | v0.26.15 | [memory-and-allocation.md](memory-and-allocation.md#one-block-of-tls-stack-per-vm-outside-linux) |
| TLS frames taken per statement | v0.26.16 | [calls-and-tls-frames.md](calls-and-tls-frames.md#tls-frames-taken-per-statement) |
| set_value written out in the interpreter | v0.26.16 | [interpreter-loop.md](interpreter-loop.md#set_value-written-out-in-the-interpreter) |
| The interrupt countdown inside the interpreter | v0.26.16 | [interpreter-loop.md](interpreter-loop.md#the-interrupt-countdown-inside-the-interpreter) |
| A front cache for small blocks | v0.26.16 | [memory-and-allocation.md](memory-and-allocation.md#a-front-cache-for-small-blocks) |
| Kept shapes | v0.26.16 | [objects-and-shapes.md](objects-and-shapes.md#kept-shapes) |
| SetShapeCache and DefaultShapeCache | v0.26.16 | [go-api.md](go-api.md#setshapecache-and-defaultshapecache) |
| for in skips prototypes with no enumerable property | Unreleased | [objects-and-shapes.md](objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) |
| Bitwise operators on floats skip the slow path | Unreleased | [operators.md](operators.md#bitwise-operators-on-floats-skip-the-slow-path) |
| The usual comparisons answered in the interpreter | Unreleased | [operators.md](operators.md#the-usual-comparisons-answered-in-the-interpreter) |
| One TLS frame per call on linux, and 34 more functions with fewer frames | Unreleased | [calls-and-tls-frames.md](calls-and-tls-frames.md#one-tls-frame-per-call-on-linux) |

v0.26.14 also made `fixlib` stop wherever an upstream change could make it fix the code wrong: most of the checks under "When upstream changes it" come from that release.

## Adding a change

- Put it in the doc of its area, or in a new doc here when no area fits, and add a row to the table above.
- Say what it does, why it is safe, what it gains or costs with the machine and how it was measured, which tests check it, whether it has a C form, and what to do when `fixlib` stops on it.
- Add it to [CHANGELOG.md](../../CHANGELOG.md) and [CHANGELOG.zh-CN.md](../../CHANGELOG.zh-CN.md), and a change for speed to [performance.md](../performance.md).
