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
one of COUNTS (js_call_enter, js_internal_enter, js_json_enter and the like, see
docs/fork-changes/c-engine.md) is a cut: a cycle through it is bounded by its
count. It prints each strongly connected component left once the cuts are taken
out, with more than one function or a function that calls itself: its size and
its functions, sorted, one per line, largest first.

It then prints each function of a NO_REACH pair that the other reaches, with
a shortest path to it: "reach F G via F A B G". None is expected. The bound
of a nesting rests on these: JS_ThrowError2 nests twice at most because making
an error runs no JavaScript and reads no interrupt flag, and
free_zero_refcount once because freeing starts no collection (see
js_nest_enter in quickjs.c). A function of a pair that the module does not
define is an error.
"""
import collections
import re
import sys

# js_json_enter is js_internal_enter with the error of JSON: it counts the same level.
COUNTS = {"js_call_enter", "js_internal_enter", "js_internal_enter_rt",
          "lre_depth_enter", "js_nest_enter", "js_json_enter"}

# (F, G): F must not reach G by any path of calls, counted or not.
NO_REACH = [
    ("JS_ThrowError2", "JS_CallInternal"),
    ("JS_ThrowError2", "JS_ThrowInterrupted"),
    # Nor does it read the interrupt flag: these are the functions that do
    # (poll_flag), or that the flag is handed to.
    ("JS_ThrowError2", "js_poll_interrupts"),
    ("JS_ThrowError2", "lre_check_timeout"),
    ("JS_ThrowError2", "lre_poll_timeout"),
    ("JS_ThrowError2", "rqsort_stop"),
    # A collection throws nothing, so it starts no error, whose object can
    # start a collection.
    ("JS_RunGC", "JS_ThrowError2"),
    ("JS_RunYoungGC", "JS_ThrowError2"),
    # Freeing starts no collection: free_zero_refcount is never entered
    # twice (JS_NEST_FREE). The finalizers of the host are not in the graph.
    ("free_zero_refcount", "JS_RunGC"),
    ("free_zero_refcount", "JS_RunYoungGC"),
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
    """The pairs (F, G) of pairs where F reaches G, each with its path. A
    function of a pair that the module does not define, which would check
    nothing, is an error."""
    adj, _ = graph(text)
    found = []
    for f, g in pairs:
        for h in (f, g):
            if h not in adj:
                raise SystemExit("cycles.py: %s of NO_REACH is not in the module" % h)
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
