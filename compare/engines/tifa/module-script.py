# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""module-script.py <ssr.js> <out.js>: a tifa SSR bundle as a script.

tifa builds its SSR bundle as an ES module with no imports, whose only export
is the `export{};` at its end. Engines without a module loader of the same
kind (qjs --std, Node's vm, the Go engines' script entry) run it as a script:
this drops that export and the source map comment and wraps the rest in a
strict function, so its top-level names stay local and strict mode holds as
in a module. A top-level await, import or other export does not compile in
the function, so such a bundle fails at load rather than running differently.
"""
import re
import sys

src = open(sys.argv[1], encoding="utf-8").read()
body = re.sub(r"\n//# sourceMappingURL=\S+\s*$", "\n", src).rstrip()
if not body.endswith("export{};"):
    sys.exit(f"{sys.argv[1]}: does not end with export{{}};, not a tifa ES module bundle")
body = body[: -len("export{};")]
with open(sys.argv[2], "w", encoding="utf-8") as f:
    f.write('(function(){"use strict";\n' + body + "\n})();\n")
