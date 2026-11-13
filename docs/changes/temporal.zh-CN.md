---
summary: 'temporal 包，基于 go-intl 为 VM 提供的 Temporal：它覆盖什么，Temporal.Now 的时钟和时区，通过 intl 包实现的 toLocaleString，它带的时区数据（tz 2026e）以及程序怎样只链接这部分，它的 test262 和 Chromium 结果，以及在体积、内存和时间上的代价'
read_when:
  - 改 temporal 包、temporal.js、stub.js 或 internal/intlhook
  - 想知道 Temporal 在程序体积、VM 内存和时间上的代价
  - 页面在服务器上显示的 Temporal 值和在浏览器里的不一样，或者 toString 和 toLocaleString 不一致
  - 把 Temporal 的时区换到新的 tz 版本，或者改 internal/gointl 的 tz 包
  - 给 Temporal.Now 一个时钟（temporal.Clock），或者想知道为什么它不改变 Date.now
---

# Temporal

[English](temporal.md) · 简体中文

QuickJS 没有 Temporal。`temporal` 包为调用了 `temporal.Register(vm)` 的 VM 加上它，方式和 `intl` 包加上 `Intl` 一样。

## 基于 go-intl 的 Temporal

发布版本：v0.27.0。

`temporal.Register(vm)` 安装 `Temporal.Now`、`Instant`、`ZonedDateTime`、`PlainDate`、`PlainTime`、`PlainDateTime`、`PlainYearMonth`、`PlainMonthDay` 和 `Duration`，带 V8 提供的每个方法，以及 `Date.prototype.toTemporalInstant`。计算由 go-intl 的 temporal 包完成（`internal/gointl/temporal`，是 temporal_rs 的 Go 移植，V8 的 Temporal 就建立在 temporal_rs 之上）。`temporal.js` 按 V8 读取的顺序读参数、选项和属性包，用 V8 的转换和错误信息，并把每个对象的内部槽放在页面够不到的私有字段里。

它的安装方式和 `Intl` 一样（见 [time-and-intl.md](time-and-intl.zh-CN.md#intl-通过-builtin-调用-go)）。`Register` 在任何页面运行之前，在 VM 上放好 `Temporal` 和 `toTemporalInstant` 的替身，并取走 Temporal 的代码要用的内置对象。第一次使用时安装其余部分，字节码每个进程编译一次，通过一个 `Builtin`，它的第一个参数指明操作。第一次使用只在替身还在、而且可以被替换时才替换它：页面放在原处的东西、被它删掉的属性、连同它的对象一起被冻结的替身，都会保持原样，就像 V8 的内置对象那样，而留在原处的替身会转发给真正的实现。页面在 `Register` 之后改动内置对象，不会改变 Temporal 的行为。

### 时钟和时区

`Temporal.Now` 读 `VM.Now`，即 `VM.SetClock` 设置的 VM 时钟，`Date.now`、`new Date()` 和 `Intl` 也读它（见 [time-and-intl.md](time-and-intl.zh-CN.md#每个-vm-一个墙上时钟)）。默认是实时的 `time.Now`。非 nil 的 `temporal.Clock` 只对 `Temporal.Now` 覆盖 VM 时钟，`Clock(nil)` 回到 VM 时钟。时钟超出 `Instant` 的范围（1970 年前后各 1e8 天）时，`Temporal.Now` 抛出 `RangeError`。时钟不能阻塞：VM 会等它，eval 超时也没法打断它，因为引擎只在自己的指令之间检查超时。`temporal.Clock` 回调里的 panic 会传到 `Eval` 的调用者那里，之后 VM 仍可使用。`Close` 会放掉 VM 的时钟（`VM.OnClose`），所以引用着自己 VM 的时钟不会把它留住。

`Temporal.Now.timeZoneId()` 是 `intl` 包格式化时用的默认时区，即 `intl.TimeZone`，没有设置时是 `UTC`，和 ECMA-402 的 DefaultTimeZone 一样，不带时区调用的 `Now` 方法也用它。引擎的 `Date` 不跟随它：每个 VM 里它的本地时间都是 UTC（见 [time-and-intl.md](time-and-intl.zh-CN.md#每个-vm-里本地时间都是-utc)），所以设了 `intl.TimeZone("Asia/Tokyo")` 之后，`new Date().getHours()` 和 `Temporal.Now.plainTimeISO().hour` 相差 9 小时。

### toLocaleString

`toLocaleString` 通过 `internal/intlhook` 使用 `intl` 包，不管 VM 有没有注册 `intl`。日期类型使用和 `Intl.DateTimeFormat.format`、`formatToParts`、`formatRange` 和 `formatRangeToParts` 相同的、按类型区分的格式。每种类型有自己的默认字段和日历检查。Plain 值忽略 `timeZone`，包括被跳过的本地日期和时间。`Instant` 使用格式化器的时区。`ZonedDateTime` 在 `toLocaleString` 里使用它自己的时区，但把它传给 `DateTimeFormat` 会抛出 `TypeError`。范围的两个端点必须是同一种 Temporal 类型。

支持 ISO 日历。带这个日历的 `PlainYearMonth` 或 `PlainMonthDay`，需要有匹配的 `calendar: "iso8601"` 选项或 locale 扩展，和 Chromium 一样。Chromium 列出的 18 种日历都支持。ISO 日期或日期时间可以使用解析出的日历。其他日期、日期时间、年月和月日的值要求日历匹配，否则抛出 `RangeError`。

`Duration.prototype.toLocaleString` 使用 `Intl.DurationFormat`，包括样式、单位和小数位数。两条路径都读私有槽和已捕获的内置对象，所以页面对 getter、构造函数或原型的改动，无法改变它们的输出。`Intl.Locale` 也是从它的槽里读取的。

日期格式化现在读 go-intl 的 ICU 时区数据，和 Temporal 用的是同一个数据包。如果两者偶尔不一致，`ZonedDateTime.toLocaleString` 还会保留 Temporal 提供的偏移，并改为使用该偏移的名称。具名时区也通过内置的数据包验证。不需要主机的 zoneinfo。

测试：`temporal/formatting_test.go` 拿 3,216 个日期表达式和 219 个 duration 表达式与 Chromium 156.0.8078.4 做对比。它还检查 GC 之后的私有槽、两种安装顺序、缓存淘汰和被改动的内置对象。[Time and Intl](time-and-intl.zh-CN.md#datetimeformat-接受-temporal-值) 描述了这座桥和数据的代价。

### 时区数据

时区和日历的表是 go-intl 的，编进了二进制：ICU 的 `zoneinfo64`（来自 ICU 针对 tz 2026e 的时区更新），以及中国、韩国和 Umm al-Qura 日历的表。它们是 92 KB 的数据包 `internal/gointl/qjsdata/tz.bin`，Temporal 通过 `gointl.EmbeddedTemporalSource` 读取它。`intl` 包读的 `gointl.Embedded` 提供同一个数据包和其余部分，通过任一方读取的块只解压一次。因为 Temporal 为 `toLocaleString` 导入了 `intl`，所以用它的程序也会链接 `intl` 链接的那些数据包（`temporal/link_test.go` 里的 `TestLinksOnlyReadData`）。从不读主机的 zoneinfo，所以 Temporal 在每台机器上给出相同的答案。

go-intl v0.6.1 带的是 tz 2026c。数据包改为从 `internal/gointl/qjstz` 指明的更新取时区，由 go-intl 自己的 `tzgen` 生成（见 [upgrading.md](../upgrading.zh-CN.md#更新-intl-的数据internalgointl)）：相对 2026c，2026d 让西北地区从 2026-08-21 起保持在 -06，2026e 让曼尼托巴从 2026-10-31 起保持在 -05。`tz.bin` 里带有 `tzversion.bin`，即它的时区所属的版本，`TestQJSTZVersion` 会检查它。

### 结果

最初的 Temporal 实现，在 test262 提交 `2e0a5676` 上，通过了 `built-ins/Temporal` 的 9210 个运行中的 9206 个，以及 `Date/prototype/toTemporalInstant` 的全部 16 个运行。针对格式化的这次改动，这些套件没有重跑。已知的四个内置对象失败，是 ISO `PlainYearMonth` 和 `PlainMonthDay` 的默认 locale 格式化，Chromium 156 也拒绝它们。

针对性的格式化对比记录在 [Time and Intl](time-and-intl.zh-CN.md#datetimeformat-接受-temporal-值)。现有的 723 个 Temporal 语料表达式现在全部和 Chromium 一致，包括日语日历和错误信息。之前只有纪元、农历月和 duration 的差异，现在都没有了。

在 v0.27.0 里，上面的格式化改动之前，在 AMD EPYC 7B13 的一个核上，linux/amd64，不开 PGO：tifa 的示例商店按 `tifa build` 的方式用 `-ldflags=-w` 构建，增大 0.84 MB（27.09 MB 到 27.93 MB），全部是代码：时区和日历在 `tz.bin` 里，`intl` 包已经链接了它。`Register` 用 0.06 ms，没有可测的内存。进程里第一次使用用 13.4 ms，其中 8.7 ms 用来编译 JavaScript，剩下的大部分用来解压时区和日历，为这个进程留下 236 KB 堆内存。之后的 VM 里第一次使用用 1.1 ms 和 358 KB。`Temporal.PlainDate.from('2024-01-05')` 用 3.5 µs，`add({months: 1})` 用 14 µs，带 `largestUnit: 'years'` 的 `until` 用 11 µs，`ZonedDateTime.from` 用 7.3 µs，它的 `add({days: 1})` 用 17 µs，`toString` 对日期用 1.4 µs，对带时区的日期时间用 5.5 µs，`toLocaleString` 对日期用 3.8 µs，对带时区的日期时间用 4.9 µs。

之后的 [Intl 数据迁移](time-and-intl.zh-CN.md#所有-intl-格式化器共用-go-intl) 用 Chromium 选出的资源替换了这些数据包，并去掉了 `cldr.bin`。那一节记录了当前商店的体积和 Intl 的计时。日期和 duration 的格式化也复用了 [编译好的 Intl 计划](time-and-intl.zh-CN.md#编译好的-intl-格式化计划)。它们记录下来的答案没有变。[History](../history.zh-CN.md#intl-格式化) 记录了 Intl 的调用和十个页面的商店对比。

测试：`temporal/all_test.go`、`temporal/shape_test.go`、`temporal/tamper_test.go`、`temporal/chromium_test.go`、`temporal/concurrent_test.go`、`temporal/link_test.go`、`intl/temporal_test.go`、`internal/gointl/qjs_source_test.go`。改动之后要跑的检查见 [testing.md](../testing.zh-CN.md#提交前)。
