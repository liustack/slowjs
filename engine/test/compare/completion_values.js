// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The completion value of try statements, as eval gives it: a try or catch
// block with no value of its own gives undefined, not the value of the
// statement before it (upstream QuickJS 4984249). The file is what Node 24
// prints.
const out = []
const srcs = [
  `1; try { } catch (e) { }`,
  `1; try { 2 } catch (e) { 3 }`,
  `1; try { 2; throw 0 } catch (e) { }`,
  `1; try { throw 0 } catch (e) { }`,
  `1; try { throw 0 } catch (e) { 4 }`,
  `1; try { throw 0 } catch (e) { 4; }`,
  `1; try { } finally { }`,
  `1; try { 2 } finally { 3 }`,
  `1; try { 2 } finally { }`,
  `1; try { } finally { 3 }`,
  `1; try { throw 0 } catch (e) { } finally { 5 }`,
  `1; try { throw 0 } catch (e) { 4 } finally { }`,
  `1; try { throw 0 } catch (e) { } finally { }`,
  `1; try { 2 } catch (e) { 3 } finally { 5 }`,
  `1; try { 2 } catch { 3 }`,
  `1; try { throw 0 } catch { }`,
  `1; try { throw 0 } catch { 6 }`,
  `1; do { try { 2; break } catch (e) { } } while (false)`,
  `1; do { try { throw 0 } catch (e) { break } } while (false)`,
  `1; do { try { throw 0 } catch (e) { 7; break } } while (false)`,
  `1; do { try { 8; continue } finally { } } while (false)`,
  `1; for (var i = 0; i < 2; i++) { try { throw 0 } catch (e) { } }`,
  `1; for (var i = 0; i < 2; i++) { try { throw 0 } catch (e) { i } }`,
  `1; for (var i = 0; i < 2; i++) { try { i } catch (e) { } }`,
  `1; if (true) { try { } catch (e) { } }`,
  `1; { try { throw 0 } catch (e) { } }`,
  `1; l: try { 2; break l } catch (e) { }`,
  `1; try { try { throw 0 } catch (e) { } } catch (e) { 9 }`,
  `1; try { try { throw 0 } finally { } } catch (e) { }`,
  `1; try { throw 0 } catch (e) { try { } catch (e2) { } }`,
  `1; switch (0) { case 0: try { throw 0 } catch (e) { } }`,
  `1; switch (0) { case 0: 2; try { throw 0 } catch (e) { } }`,
  `1; try { throw 0 } catch ({ a }) { }`,
  `1; try { throw {} } catch ({ a }) { a }`,
  `1; try { throw 0 } catch (e) { var x = 1 }`,
  `1; try { throw 0 } catch (e) { ; }`,
  `1; try { throw 0 } catch (e) { function f() {} }`,
  `1; try { throw 0 } catch (e) { let y = 1 }`,
  `1; with ({}) { try { throw 0 } catch (e) { } }`,
]
for (const s of srcs) {
  let r
  try { r = (0, eval)(s) } catch (e) { r = 'throws ' + e.name }
  out.push(s + ' => ' + String(r))
}
out.join('\n')
