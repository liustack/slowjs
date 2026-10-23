# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Offline generator. Consumers require only the generated Go and .s files.

Run on linux/amd64 with LLVM 22.1.8, Python 3 and matching LLVM development
headers. Input is the pinned QuickJS source after lib/internal/patch/quickjs.diff.
Every subprocess inherits CPU affinity and priority from the caller.
"""
import argparse,pathlib,subprocess,os,shutil,json,re,hashlib
p=argparse.ArgumentParser();p.add_argument('--source',required=True);p.add_argument('--work',required=True);p.add_argument('--llvm',required=True);p.add_argument('--plugin',required=True);p.add_argument('--signatures',required=True);p.add_argument('--out',required=True);a=p.parse_args()
here=pathlib.Path(__file__).resolve().parent;work=pathlib.Path(a.work).resolve();work.mkdir(parents=True,exist_ok=True)
llvm=pathlib.Path(a.llvm).resolve();source=pathlib.Path(a.source).resolve();out=pathlib.Path(a.out).resolve()
def run(cmd,**kw):subprocess.run([str(x) for x in cmd],check=True,**kw)
version=subprocess.check_output([str(llvm/'clang-22'),'--version'],text=True)
if '22.1.8' not in version:raise RuntimeError('generator requires LLVM 22.1.8')
run(['python3',here/'cpatch.py',source/'quickjs.c',work/'quickjs.c'])
run([llvm/'clang-22','-E','-P','-D_GNU_SOURCE','-DCONFIG_VERSION="2026-06-04"','-I',source,work/'quickjs.c','-o',work/'quickjs.i'])
with open(work/'ast.json','w') as f:run([llvm/'clang-22','-x','c','-Xclang','-ast-dump=json','-fsyntax-only',work/'quickjs.i'],stdout=f)
run(['python3',here/'prepare.py',work/'quickjs.i',work/'ast.json',work])
run([llvm/'clang-22','-O2','-fno-builtin','-fno-omit-frame-pointer','-mno-red-zone','-fwrapv','-S','-emit-llvm',work/'interpreter.c','-o',work/'interpreter.ll'])
definitions=re.findall(r'^define .*?@([^(]+)\(', (work/'interpreter.ll').read_text(),re.M)
if definitions!=['goasm_run']:raise RuntimeError('unexpected out-of-line C functions: '+str(definitions))
with open(work/'allocas.log','w') as f:run([llvm/'opt-22','-load-pass-plugin='+str(pathlib.Path(a.plugin).resolve()),'-passes=goasm-external-allocas','-S',work/'interpreter.ll','-o',work/'external.ll'],stderr=f)
if (work/'allocas.log').read_text().strip()!='GOASM_LOCALS_SIZE=528':raise RuntimeError('review TLS frame layout before changing local size')
run([llvm/'llc-22','-O2','-relocation-model=pic',work/'external.ll','-o',work/'external.s'])
run(['python3',here/'lower.py',work/'external.s',work/'lowered.s'])
run([llvm/'clang-22','-c',work/'lowered.s','-o',work/'lowered.o'])
run(['python3',here/'bridges.py',work/'bridges.json',a.signatures,out])
run(['python3',here/'elf2go.py',work/'lowered.o',work/'lowered.s.json',out/'goasm_run_linux_amd64.s'])
run(['gofmt','-w',out/'goasm_bridges_linux_amd64.go'])

manifest={str(f.relative_to(work)):hashlib.sha256(f.read_bytes()).hexdigest() for f in work.iterdir() if f.is_file() and f.suffix in ('.i','.ll','.s','.o','.json','.c')}
manifest['compiler']=version.strip()
(work/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
