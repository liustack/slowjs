---
summary: 'JSON 的改动：JSON.stringify 和 JSON.parse 成段复制普通字符，收益是什么，为什么得到的字符串和错误不变'
read_when:
  - JSON.stringify 或 JSON.parse 给出的字符串或错误和以前不一样
---

# JSON

[English](json.md) · 简体中文

## JSON 成段复制普通字符

发布版本：v0.26.17。

`JSON.stringify` 和 `JSON.parse` 把字符串里的普通字符成段复制。原来两者都是一次处理一个字符：`JSON.stringify` 用 `string_getc` 读每个字符，再用 `string_buffer_putc` 写出，`JSON.parse` 也用同样的办法把文本的每个字节放进新字符串，用 `unicode_from_utf8` 解码 UTF-8。

新字符串的增长和变成 16 位的方式和 `string_buffer_write8`、`string_buffer_write16` 一致，所以得到的是同一个字符串，成段复制时内存耗尽仍然让调用失败。`JSON.parse` 的一段在最长字符串的长度处停下，所以带更长字符串的 `JS_ParseJSON` 文本仍然抛出 "string too long"。reviver、replacer、`toJSON`、`space` 和其他路径都没动。

在 linux/amd64、不开 PGO（AMD EPYC 7B13，单核）上，对比 4a4103b（v0.26.16 加之后 9 个提交），像 tifa 那样把 tifa 示例商店每条路由数据的 JSON 文本再当作一个字符串加引号，耗时少 41%，对其渲染数据做 `JSON.parse` 少 9.5%。由许多短字符串和数字组成的文档，解析用时不变，序列化少 2%。JSON 约占一次渲染的 1%，所以一次渲染最多只能快约 0.3%。8 轮实测少 0.2% 到 1.7%，在一轮的波动范围内。HTML 和内存不变。

改动在 C 里（`engine/quickjs.c`）：`JS_ToQuotedString` 在循环每一轮开头调用 `json_quote_run`，`json_parse_string` 同样调用 `json_string_run`。它们用 `string_buffer_write8` 和 `string_buffer_write16` 写入，`json_string_run` 和原来的循环一样用 `unicode_from_utf8` 解码。`engine/test/compare.sh` 会运行 `testdata/jsonstr.js` 和 `testdata/jsonstr_errors.js`。
