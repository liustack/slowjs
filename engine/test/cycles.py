# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""The cycles of the engine's call graph that no counted depth cuts.

    python3 engine/test/cycles.py ENGINE.ll

ENGINE.ll is the engine as one module of LLVM IR, built with -O0 and
-fsanitize=kcfi (see cycles.sh). A function calls the functions it names, and
an indirect call reaches every function whose address is taken and whose kcfi
type is that of the call, as the generator joins them. A function that calls
one of COUNTS (js_call_enter, js_internal_enter and the like, see
docs/fork-changes/c-engine.md) is a cut: a cycle through it is bounded by its
count. It prints each strongly connected component left once the cuts are taken
out, with more than one function or a function that calls itself: its size and
its functions, sorted, one per line, largest first.

It then prints each function of a NO_REACH pair that the other reaches, with
a shortest path to it: "reach F G via F A B G". None is expected. The bound
of a nesting rests on these: JS_ThrowError2 nests twice at most because making
an error runs no JavaScript and turns no interrupt into an exception (see
js_nest_enter in quickjs.c).
"""
import collections
import re
import sys

COUNTS = {"js_call_enter", "js_internal_enter", "js_internal_enter_rt",
          "lre_depth_enter", "js_nest_enter"}

# (F, G): F must not reach G by any path of calls, counted or not.
NO_REACH = [
    ("JS_ThrowError2", "JS_CallInternal"),
    ("JS_ThrowError2", "JS_ThrowInterrupted"),
]

DEFINE = re.compile(r'^define\b.*?@("[^"]+"|[-\w.$]+)\(')
CALL = re.compile(r'(?:^\s+|=\s+)(?:(?:tail|musttail|notail)\s+)?(?:call|invoke)\s'
                  r'.*?\s([@%])("[^"]+"|[-\w.$]+)\(')
KCFI_CALL = re.compile(r'"kcfi"\(i32 (-?\d+)\)')
KCFI_TYPE = re.compile(r'!kcfi_type !(\d+)')
NAME = re.compile(r'@("[^"]+"|[-\w.$]+)')
METADATA = re.compile(r'^!(\d+) = .*?i32 (-?\d+)')


def graph(text):
    """Return (adjacency, cuts) of the module text."""
    types = {}
    for line in text.split("\n"):
        m = METADATA.match(line)
        if m:
            types[m.group(1)] = int(m.group(2))
    funcs, calls, icalls, taken = {}, {}, {}, set()
    cur = None
    for line in text.split("\n"):
        if line.startswith("define"):
            cur = DEFINE.match(line).group(1).strip('"')
            k = KCFI_TYPE.search(line)
            funcs[cur] = types[k.group(1)] if k else None
            calls[cur], icalls[cur] = set(), []
            continue
        if line.startswith("}"):
            cur = None
            continue
        if cur is None:
            if line.startswith("@") and "=" in line:
                taken.update(n.strip('"') for n in NAME.findall(line.split("=", 1)[1]))
            continue
        rest = line
        m = CALL.search(line)
        if m and m.group(1) == "@":
            calls[cur].add(m.group(2).strip('"'))
            rest = line[:m.start(2)] + line[m.end(2):]
        elif m:
            k = KCFI_CALL.search(line)
            if not k:
                raise SystemExit("cycles.py: an indirect call with no kcfi type in " + cur)
            icalls[cur].append(int(k.group(1)))
        taken.update(n.strip('"') for n in NAME.findall(rest))
    by_type = {}
    for f, k in funcs.items():
        if f in taken and k is not None:
            by_type.setdefault(k, set()).add(f)
    adj = {}
    for f in funcs:
        s = {c for c in calls[f] if c in funcs}
        for k in icalls[f]:
            s |= by_type.get(k, set())
        adj[f] = s
    cuts = {f for f in funcs if calls[f] & COUNTS}
    return adj, cuts


def components(adj, nodes):
    """Tarjan's strongly connected components of nodes, without recursion."""
    index, low, on, stack, out, n = {}, {}, set(), [], [], 0
    for root in sorted(nodes):
        if root in index:
            continue
        work = [(root, 0)]
        while work:
            v, i = work.pop()
            if i == 0:
                index[v] = low[v] = n
                n += 1
                stack.append(v)
                on.add(v)
            succ = sorted(w for w in adj[v] if w in nodes)
            descended = False
            while i < len(succ):
                w = succ[i]
                i += 1
                if w not in index:
                    work.append((v, i))
                    work.append((w, 0))
                    descended = True
                    break
                if w in on:
                    low[v] = min(low[v], index[w])
            if descended:
                continue
            if low[v] == index[v]:
                comp = []
                while True:
                    w = stack.pop()
                    on.discard(w)
                    comp.append(w)
                    if w == v:
                        break
                out.append(comp)
            if work:
                u = work[-1][0]
                low[u] = min(low[u], low[v])
    return out


def cycles(text):
    adj, cuts = graph(text)
    nodes = set(adj) - cuts
    found = [sorted(c) for c in components(adj, nodes)
             if len(c) > 1 or c[0] in adj[c[0]]]
    found.sort(key=lambda c: (-len(c), c))
    return found


def path(adj, src, dst):
    """A shortest path of calls from src to dst, or None."""
    if src not in adj:
        return None
    parent = {src: None}
    work = collections.deque([src])
    while work:
        f = work.popleft()
        if f == dst:
            out = []
            while f is not None:
                out.append(f)
                f = parent[f]
            return out[::-1]
        for g in sorted(adj[f]):
            if g not in parent:
                parent[g] = f
                work.append(g)
    return None


def reaches(text, pairs=NO_REACH):
    """The pairs (F, G) of pairs where F reaches G, each with its path."""
    adj, _ = graph(text)
    found = []
    for f, g in pairs:
        p = path(adj, f, g)
        if p:
            found.append((f, g, p))
    return found


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: cycles.py ENGINE.ll")
    with open(sys.argv[1]) as f:
        text = f.read()
    for c in cycles(text):
        print(len(c), " ".join(c))
    for f, g, p in reaches(text):
        print("reach", f, g, "via", " ".join(p))


if __name__ == "__main__":
    main()
