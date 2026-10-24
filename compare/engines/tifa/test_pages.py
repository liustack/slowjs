# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


class PageArchiveTest(unittest.TestCase):
    def test_archive_keeps_raw_runs_and_html_evidence(self):
        spec = importlib.util.spec_from_file_location('pages', Path(__file__).with_name('pages.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            module.out = tmp
            (root / 'html').mkdir()
            def result(name, ok=True):
                (root / (name + '.json')).write_text(json.dumps({'pages': [
                    {'input': 'one.json', 'ok': ok, 'cpu_ms_per_op': 1},
                    {'input': 'missing.json', 'ok': False}]}))
            for name in ['slowjs-pf', 'slowjs-native', 'quickjs-ng-pf']:
                result(name)
            result('go-quickjs-interp-pf', False)
            result('go-quickjs-interp-pf-rest')
            result('probe-one')
            (root / 'probe-one.meta').write_text('{"try":0,"disturbed":1}\n{"try":1,"disturbed":0}\n')
            for name, body in [('slowjs-pf', b'<p>u:1234567890123</p>'),
                               ('slowjs-native', b'<p>u:1234567890124</p>'),
                               ('go-quickjs-interp-pf', b'<p>u:1234567890125</p>'),
                               ('quickjs-ng-pf', b'<p><!-- -->u:1234567890126</p>')]:
                (root / 'html' / (name + '-one.html')).write_bytes(body)
            archive = module.archive()
            self.assertIn('go-quickjs-interp-pf-rest', archive)
            self.assertEqual(len(archive['probe-one']['tries']), 2)
            self.assertFalse(archive['go-quickjs-interp-pf']['result']['pages'][0]['ok'])
            evidence = archive['_html']['comparisons']
            same = evidence['pf']['go-quickjs-interp']['one']
            self.assertTrue(same['same_as_liustack'])
            self.assertEqual(len(same['normalized_sha256']), 64)
            self.assertEqual(same['normalized_sha256'], same['reference_sha256'])
            self.assertFalse(evidence['pf']['quickjs-ng']['one']['same_as_liustack'])
            self.assertIsNone(evidence['pf']['quickjs-ng']['missing']['same_as_liustack'])
            self.assertTrue(evidence['deployed']['slowjs']['one']['same_as_liustack'])

    def test_tifa_row_is_compared_with_the_polyfill_row(self):
        spec = importlib.util.spec_from_file_location('pages', Path(__file__).with_name('pages.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            module.out = tmp
            (root / 'html').mkdir()
            def result(name, pages):
                (root / (name + '.json')).write_text(json.dumps({'pages': [
                    {'input': p + '.json', 'ok': True, 'cpu_ms_per_op': 1} for p in pages]}))
            result('slowjs-tifa-native', ['a', 'b'])
            result('slowjs-pf', ['a', 'b'])
            result('slowjs-tifa-mui-native', ['home', 'order'])
            result('slowjs-mui-pf', ['home'])
            for name, body in [('slowjs-tifa-native-a', b'<p>8:00 AM u:1234567890123</p>'),
                               ('slowjs-pf-a', b'<p>8:00 AM u:1234567890124</p>'),
                               ('slowjs-tifa-native-b', 'x 8:00 AM'.encode()),
                               ('slowjs-pf-b', 'x 8:00\u202fAM'.encode()),
                               ('slowjs-tifa-mui-native-home', b'<p>one</p>'),
                               ('slowjs-mui-pf-home', b'<p>two</p>')]:
                (root / 'html' / (name + '.html')).write_bytes(body)
            ev = module.tifa_row_evidence()
            self.assertTrue(ev['shop/a']['same'])
            self.assertFalse(ev['shop/b']['same'])
            self.assertTrue(ev['shop/b']['same_but_u202f'])
            self.assertEqual(ev['shop/b']['u202f_in_polyfill_page'], 1)
            self.assertFalse(ev['mui/home']['same'])
            self.assertFalse(ev['mui/home']['same_but_u202f'])
            self.assertIsNone(ev['mui/order']['same'])


if __name__ == '__main__':
    unittest.main()
