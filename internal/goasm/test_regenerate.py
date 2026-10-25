# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import os,pathlib,subprocess,tempfile,unittest
import regenerate

class DriftTests(unittest.TestCase):
 def setUp(self):
  self.dir=tempfile.TemporaryDirectory();d=pathlib.Path(self.dir.name)
  self.out=d/'out';self.lib=d/'lib';self.out.mkdir();self.lib.mkdir()
  for n in regenerate.GENERATED:
   (self.out/n).write_text('// '+n+'\nbody\n');(self.lib/n).write_text('// '+n+'\nbody\n')
 def tearDown(self):self.dir.cleanup()
 def test_same(self):
  self.assertEqual(regenerate.drift(self.out,self.lib),[])
 def test_one_byte(self):
  n=regenerate.GENERATED[0];(self.lib/n).write_text('// '+n+'\nbodY\n')
  self.assertEqual(regenerate.drift(self.out,self.lib),[n])
 def test_missing_and_extra(self):
  n=regenerate.GENERATED[1];(self.lib/n).unlink();(self.out/'goasm_extra_linux_amd64.s').write_text('x')
  self.assertEqual(regenerate.drift(self.out,self.lib),[n,'goasm_extra_linux_amd64.s'])

class SourceTests(unittest.TestCase):
 def test_pinned_archive(self):
  root=pathlib.Path(__file__).resolve().parent.parent.parent
  name,url=regenerate.pinned_source(root)
  self.assertRegex(name,r'^[0-9a-f]{40}\.zip$')
  self.assertEqual(url,'https://github.com/bellard/quickjs/archive/'+name)
  self.assertIn(name,regenerate.SOURCE_SHA256)

class EngineTests(unittest.TestCase):
 def setUp(self):
  self.dir=tempfile.TemporaryDirectory();d=pathlib.Path(self.dir.name)
  self.engine=d/'engine';self.legacy=d/'legacy';self.engine.mkdir();self.legacy.mkdir()
  for n in regenerate.ENGINE_FILES:
   (self.engine/n).write_text('same '+n);(self.legacy/n).write_text('same '+n)
  (self.legacy/'qjs.c').write_text('the shell')
 def tearDown(self):self.dir.cleanup()
 def test_same(self):
  self.assertEqual(regenerate.engine_drift(self.engine,self.legacy),[])
 def test_one_byte(self):
  (self.engine/'quickjs.c').write_text('same quickjs.C')
  self.assertEqual(regenerate.engine_drift(self.engine,self.legacy),['quickjs.c'])
 def test_missing_in_engine(self):
  for n in ['dtoa.c','quickjs.h','LICENSE']:(self.engine/n).unlink()
  self.assertEqual(regenerate.engine_drift(self.engine,self.legacy),['LICENSE','dtoa.c','quickjs.h'])
 def test_empty_engine(self):
  for f in self.engine.iterdir():f.unlink()
  self.assertEqual(regenerate.engine_drift(self.engine,self.legacy),regenerate.ENGINE_FILES)
 def test_missing_in_old_way(self):
  (self.engine/'extra.h').write_text('x')
  self.assertEqual(regenerate.engine_drift(self.engine,self.legacy),['extra.h'])
 def test_only_the_c_tree_is_compared(self):
  (self.engine/'README.md').write_text('notes');(self.engine/'test').mkdir();(self.engine/'test'/'host.c').write_text('x')
  self.assertEqual(regenerate.engine_names(self.engine),regenerate.ENGINE_FILES)
  self.assertEqual(regenerate.engine_drift(self.engine,self.legacy),[])
 def test_layout(self):
  ok=['engine/quickjs.c','engine/test/host.c','lib/ccgo.go','lib/testdata/tests/bjson.c','internal/goasm/cmd/signatures/main.go','internal/goasm/allocas.cpp']
  self.assertEqual(regenerate.layout_problems(None,ok),[])
  self.assertEqual(regenerate.layout_problems(None,ok+['engine/libc/wrap.go']),['engine/libc has Go files'])
  self.assertEqual(regenerate.layout_problems(None,ok+['lib/shim.c']),['lib has Go files and C files'])
 def test_repository_layout(self):
  root=pathlib.Path(__file__).resolve().parent.parent.parent
  try:files=subprocess.run(['git','ls-files'],cwd=root,check=True,capture_output=True,text=True).stdout.splitlines()
  except (OSError,subprocess.CalledProcessError):self.skipTest('not a git checkout')
  self.assertEqual(regenerate.layout_problems(root,files),[])
 @unittest.skipUnless(os.environ.get('SLOWJS_QJS_ZIP'),'set SLOWJS_QJS_ZIP to the pinned QuickJS archive')
 def test_engine_is_what_the_old_way_makes(self):
  root=pathlib.Path(__file__).resolve().parent.parent.parent
  with tempfile.TemporaryDirectory() as w:
   legacy=regenerate.legacy_tree(root,pathlib.Path(os.environ['SLOWJS_QJS_ZIP']).resolve(),pathlib.Path(w))
   self.assertEqual(regenerate.engine_drift(root/'engine',legacy),[])

if __name__=='__main__':unittest.main()
