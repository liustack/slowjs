# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Check shell orchestration with stand-ins, without running any engine."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent


class ScriptTest(unittest.TestCase):
    def test_caller_paths_and_check_environment(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            scripts = root / 'scripts'
            scripts.mkdir()
            shutil.copy(HERE / 'run.sh', scripts)
            for app in ['bench', 'mui', 'shop']:
                inputs = scripts / 'in' / app
                inputs.mkdir(parents=True)
                (inputs / 'one.json').write_text('{}')
            (scripts / 'pagerun.sh').write_text('''python3 - "$@" <<'END'
import json, os, sys
with open(os.environ['CALL_LOG'], 'a') as f:
    f.write(json.dumps([os.environ['TZ'], os.environ['GOMAXPROCS'], sys.argv[1:]]) + '\\n')
END
''')
            log = root / 'calls.jsonl'
            subprocess.run(['bash', str(scripts / 'run.sh'), 'new output', 'engine bins', 'native bins'],
                           cwd=root, env={**os.environ, 'CALL_LOG': str(log)}, check=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            checks = [args for _, _, args in calls if '/check/' in args[0]]
            self.assertEqual(len(checks), 9)
            for tz, procs, args in calls:
                self.assertEqual((tz, procs), ('UTC', '1'))
                self.assertTrue(args[0].startswith(str(root / 'new output') + '/'))
                self.assertTrue(args[1].startswith(str(root / 'engine bins') + '/') or
                                args[1].startswith(str(root / 'native bins') + '/'))
            self.assertTrue(any(args[1].endswith('/bin/node') for _, _, args in calls))

    def test_cap_and_three_disturbed_tries(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            log = root / 'calls'
            scripts = {
                'awk': '#!/bin/sh\ncase "$1" in -v) echo "1 0 1" ;; *) echo 0 ;; esac\n',
                'sleep': '#!/bin/sh\nexit 0\n',
                'systemd-run': '#!/bin/sh\nprintf "%s\\n" "$*" >> "$CALL_LOG"\necho "{}"\n',
            }
            for name, content in scripts.items():
                p = root / name
                p.write_text(content)
                p.chmod(0o755)
            stem = root / 'output with spaces'
            subprocess.run(['bash', str(HERE / 'pagerun.sh'), str(stem), 'fake-engine'],
                           env={**os.environ, 'PATH': str(root) + ':' + os.environ['PATH'], 'CALL_LOG': str(log)},
                           check=True)
            calls = log.read_text().splitlines()
            self.assertEqual(len(calls), 3)
            for call in calls:
                self.assertIn('-p MemoryMax=4G -p MemorySwapMax=0 taskset -c 1 fake-engine', call)
            tries = [json.loads(line) for line in stem.with_suffix('.meta').read_text().splitlines()]
            self.assertEqual([t['try'] for t in tries], [0, 1, 2])


if __name__ == '__main__':
    unittest.main()
