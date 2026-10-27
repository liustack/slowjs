# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Move the code of generated Go assembly, to measure how much a speed result
depends on where the code sits (see "Code positions" in docs/performance.md).

    python3 xform.py DIR OP...

Edits the *_amd64.s files of DIR in place, so run it on a copy, never on the
committed files. OPs are applied in order:

  shift:N                  N bytes of NOP after the first function of
                           cutils_amd64.s (after its RET, where they never run),
                           so every function laid out after it moves by N bytes.
                           Go puts amd64 functions on 32 bytes, so N below 32
                           moves nothing: use 0 or 32.
  front:NAME:N             N bytes of multi-byte NOP at the start of the body of
                           function NAME, right after its PCALIGN $16. Moves the
                           code of that function only. Use 0, 8, 16 or 24 on
                           the interpreter (JS_CallInternal in the C engine,
                           goasmRun in lib/goasm_run_linux_amd64.s).
  endalign:A[:f1,f2,..]    PCALIGN $A after the RET that ends a function (all
                           functions, or only the named): the linker then places
                           the next function at a multiple of A.
  body:A:f1,f2             PCALIGN $A in place of the PCALIGN $16 at the start of
                           the body of the named functions.

Example, the position "moved 32 bytes, 16 NOP bytes in front of the interpreter":

    python3 xform.py copy-of-generated-dir shift:32 front:JS_CallInternal:16
"""
import sys, os, re, glob

NOPS = {1: [0x90], 2: [0x66, 0x90], 3: [0x0f, 0x1f, 0x00], 4: [0x0f, 0x1f, 0x40, 0x00], 5: [0x0f, 0x1f, 0x44, 0x00, 0x00],
        6: [0x66, 0x0f, 0x1f, 0x44, 0x00, 0x00], 7: [0x0f, 0x1f, 0x80, 0, 0, 0, 0], 8: [0x0f, 0x1f, 0x84, 0, 0, 0, 0, 0]}


def nopbytes(n):
    out = []
    while n > 0:
        k = min(n, 8)
        out += NOPS[k]
        n -= k
    return "".join("\tBYTE $0x%02x\n" % b for b in out)


def read(p):
    with open(p) as f:
        return f.read()


def write(p, s):
    with open(p, "w") as f:
        f.write(s)


def apply(d, ops):
    files = sorted(glob.glob(os.path.join(d, "*_amd64.s")))
    for op in ops:
        kind, *rest = op.split(":")
        if kind == "shift":
            n = int(rest[0])
            p = os.path.join(d, "cutils_amd64.s")
            s = read(p)
            i = s.index("\tRET\n") + len("\tRET\n")
            write(p, s[:i] + "".join("\tBYTE $0x90\n" for _ in range(n)) + s[i:])
        elif kind == "endalign":
            a = int(rest[0])
            only = set(rest[1].split(",")) if len(rest) > 1 else None
            cnt = 0
            for p in files:
                if os.path.basename(p) == "engine_rodata_amd64.s":
                    continue
                s = read(p)
                parts = re.split(r"(?m)^(?=TEXT ·)", s)
                out = [parts[0]]
                for part in parts[1:]:
                    name = re.match(r"TEXT ·([A-Za-z0-9_$.]+)\(SB\)", part).group(1)
                    if only is None or name in only:
                        k = part.rindex("\tRET\n") + len("\tRET\n")
                        part = part[:k] + "\tPCALIGN $%d\n" % a + part[k:]
                        cnt += 1
                    out.append(part)
                write(p, "".join(out))
            print("endalign", a, "on", cnt, "functions")
        elif kind == "front":
            name, n = rest[0], int(rest[1])
            hit = 0
            for p in files:
                s = read(p)
                m = re.search(r"(?m)^TEXT ·%s\(SB\).*\n(?:[ \t]*NO_LOCAL_POINTERS\n)?[ \t]*PCALIGN \$16\n" % re.escape(name), s)
                if m:
                    write(p, s[:m.end()] + nopbytes(n) + s[m.end():])
                    hit += 1
            assert hit == 1, (name, hit)
        elif kind == "body":
            a = int(rest[0])
            names = set(rest[1].split(","))
            hit = 0
            for p in files:
                s = read(p)
                changed = False
                for name in names:
                    m = re.search(r"(?m)^(TEXT ·%s\(SB\).*\n(?:[ \t]*NO_LOCAL_POINTERS\n)?)[ \t]*PCALIGN \$16\n" % re.escape(name), s)
                    if m:
                        s = s[:m.start()] + m.group(1) + "\tPCALIGN $%d\n" % a + s[m.end():]
                        hit += 1
                        changed = True
                if changed:
                    write(p, s)
            assert hit == len(names), (names, hit)
        else:
            raise SystemExit("unknown op " + op)


if __name__ == "__main__":
    if len(sys.argv) < 3:
        raise SystemExit(__doc__)
    apply(sys.argv[1], sys.argv[2:])
