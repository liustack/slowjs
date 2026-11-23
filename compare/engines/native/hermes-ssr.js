// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// hermes -Xhermes-internal-test-methods hermes-ssr.js -- name=value ...:
// ssr-driver.js on Hermes V1. The arguments are those of ssr-driver.js, plus
//   mode=src|hbc  src: the preludes and the bundle are read as text and run by
//                 an indirect eval, so Hermes compiles them. hbc: they are the
//                 files <hbc>/<name>.hbc that `hermesc -O` made ahead, run
//                 with hermescli.loadHBC.
//   hbc=<dir>     the bytecode files, for mode=hbc
//   driver=<path> ssr-driver.js
// The Hermes shell has no file system module and cannot write a file. The
// HTML of each page goes to stdout as a line `@html <file> <escaped>` ("\\"
// for a backslash, "\n" and "\r" for those characters), which
// native/hermes-run.sh turns into the file the other rows write. The last
// line is the JSON of the driver.
var __args = {};
hermescli.getScriptArgs().forEach(function (s) {
  var i = s.indexOf("=");
  if (i > 0) __args[s.slice(0, i)] = s.slice(i + 1);
});
var __geval = eval;
// The Hermes shell has no performance.now: the clock is Date.now, in whole milliseconds. So the
// time of a render is the time of the whole timed loop over n (host.cpu, wall time of one
// thread on a core of its own), as cpu_ms_per_op. The medians in the JSON are of whole
// milliseconds and mean nothing.
var __now = Date.now;
function __utf8(u8) {
  var parts = [], cs = [];
  for (var i = 0; i < u8.length;) {
    var c = u8[i++];
    if (c >= 0xf0) {
      c = ((c & 7) << 18) | ((u8[i] & 63) << 12) | ((u8[i + 1] & 63) << 6) | (u8[i + 2] & 63);
      i += 3;
      c -= 0x10000;
      cs.push(0xd800 + (c >> 10), 0xdc00 + (c & 1023));
    } else if (c >= 0xe0) {
      cs.push(((c & 15) << 12) | ((u8[i] & 63) << 6) | (u8[i + 1] & 63));
      i += 2;
    } else if (c >= 0xc0) {
      cs.push(((c & 31) << 6) | (u8[i] & 63));
      i += 1;
    } else cs.push(c);
    if (cs.length >= 8192) {
      parts.push(String.fromCharCode.apply(null, cs));
      cs = [];
    }
  }
  parts.push(String.fromCharCode.apply(null, cs));
  return parts.join("");
}
__geval(__utf8(hermescli.loadFile(__args.driver)));
// ssr-driver.js reads a file and then runs its text. Here a script is run by
// its path: read gives the path back and run loads it. Inputs are read as text.
var __host = {
  read: function (p) { return /\.json$/.test(p) ? __utf8(hermescli.loadFile(p)) : p; },
  run: function (path, name) {
    if (__args.mode === "hbc") hermescli.loadHBC(hermescli.loadFile(__args.hbc + "/" + path.split("/").pop() + ".hbc"));
    else __geval(__utf8(hermescli.loadFile(path)));
  },
  write: function (p, s) { print("@html " + p + " " + s.replace(/\\/g, "\\\\").replace(/\n/g, "\\n").replace(/\r/g, "\\r")); },
  now: __now,
  cpu: function () { return Date.now(); },
};
var __rest = Object.keys(__args).filter(function (k) { return k !== "mode" && k !== "driver" && k !== "hbc"; })
  .map(function (k) { return k + "=" + __args[k]; });
driver.main(__host, __rest).then(
  function (r) { print(JSON.stringify(r)); },
  function (e) { print(JSON.stringify({ err: String(e) })); });
