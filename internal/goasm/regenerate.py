# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Regenerate the goasm interpreter from the repository, or check that the
committed files are what the generator makes.

    python3 internal/goasm/regenerate.py --llvm DIR [--zip FILE] [--check]

DIR holds clang-22, clang++-22, opt-22 and llc-22 of LLVM 22.1.8. The pinned
QuickJS archive named in lib/Makefile is downloaded unless --zip gives a copy,
and its SHA-256 must match. CXXFLAGS adds flags to the build of the LLVM pass
plugin, for headers outside the compiler's default paths. With --check the
committed lib/goasm_* files it makes are left alone, and the script
exits with status 1 and names each file that differs.
"""
import argparse,hashlib,os,pathlib,shlex,shutil,subprocess,sys,tempfile,urllib.request,zipfile

# The files build.py writes, all committed under lib/.
GENERATED=['goasm_bridges_linux_amd64.go','goasm_bridges_linux_amd64.s','goasm_helpers.txt','goasm_run_linux_amd64.s']
# SHA-256 of each QuickJS archive lib/Makefile may pin.
SOURCE_SHA256={'3d5e064e9dd67c70f7962836505a7fa067bf0a4e.zip':'dcc3a698209915262bf5c49b458d01e60ee0f3651591bfcc9d32c1c7cee5ac17'}

def pinned_source(root):
 """The archive name and URL of the QuickJS source that lib/Makefile pins."""
 for line in (root/'lib/Makefile').read_text().splitlines():
  if line.startswith('QJS_ZIP = '):
   name=line.split('=',1)[1].strip()
   return name,'https://github.com/bellard/quickjs/archive/'+name
 raise RuntimeError('lib/Makefile names no QJS_ZIP')

def drift(out,lib):
 """The generated files that differ between out and lib, or are in only one."""
 names=sorted(set(GENERATED)|{f.name for f in out.iterdir() if f.name.startswith('goasm_')})
 return [n for n in names if not (out/n).is_file() or not (lib/n).is_file() or (out/n).read_bytes()!=(lib/n).read_bytes()]

def run(cmd,**kw):subprocess.run([str(x) for x in cmd],check=True,**kw)

def source_tree(root,zip_path,work):
 name,url=pinned_source(root)
 if name not in SOURCE_SHA256:raise RuntimeError(f'no SHA-256 for {name}: add it to SOURCE_SHA256 after checking the archive')
 if zip_path is None:
  zip_path=work/name
  with urllib.request.urlopen(url) as r,open(zip_path,'wb') as f:shutil.copyfileobj(r,f)
 if hashlib.sha256(zip_path.read_bytes()).hexdigest()!=SOURCE_SHA256[name]:raise RuntimeError(f'{zip_path}: SHA-256 differs from the pinned {name}')
 with zipfile.ZipFile(zip_path) as z:z.extractall(work/'src')
 src=work/'src'/('quickjs-'+name.removesuffix('.zip'))
 with open(root/'lib/internal/patch/quickjs.diff','rb') as f:run(['patch','-s','-p1','-d',src],stdin=f)
 return src

def main():
 p=argparse.ArgumentParser(description=__doc__,formatter_class=argparse.RawDescriptionHelpFormatter)
 p.add_argument('--llvm',required=True,type=pathlib.Path,help='directory of the LLVM 22.1.8 tools')
 p.add_argument('--zip',type=pathlib.Path,help='a local copy of the pinned QuickJS archive')
 p.add_argument('--work',type=pathlib.Path,help='keep the intermediate files here')
 p.add_argument('--check',action='store_true',help='compare with the committed files instead of writing them')
 a=p.parse_args()
 here=pathlib.Path(__file__).resolve().parent;root=here.parent.parent;llvm=a.llvm.resolve()
 tmp=None
 if a.work is None:tmp=tempfile.TemporaryDirectory();work=pathlib.Path(tmp.name)
 else:work=a.work.resolve();shutil.rmtree(work,ignore_errors=True);work.mkdir(parents=True)
 try:
  src=source_tree(root,a.zip.resolve() if a.zip else None,work)
  include=subprocess.check_output([str(llvm/'llvm-config'),'--includedir'],text=True).strip() if (llvm/'llvm-config').exists() else None
  run([llvm/'clang++-22','-std=c++17','-O2','-fPIC','-shared','-fno-rtti',*(['-I',include] if include else []),*shlex.split(os.environ.get('CXXFLAGS','')),here/'allocas.cpp','-o',work/'allocas.so'])
  env=dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0',GOFLAGS='-mod=mod')
  listing=subprocess.run(['go','list','-tags','goasm','-json','./lib','modernc.org/libc'],cwd=root,env=env,check=True,capture_output=True).stdout
  with open(work/'signatures.json','wb') as f:subprocess.run(['go','run','./internal/goasm/cmd/signatures'],cwd=root,input=listing,stdout=f,check=True)
  out=work/'out';out.mkdir()
  run([sys.executable,here/'build.py','--source',src,'--work',work/'build','--llvm',llvm,'--plugin',work/'allocas.so','--signatures',work/'signatures.json','--out',out])
  if a.check:
   bad=drift(out,root/'lib')
   for n in bad:print(f'lib/{n} differs from what the generator makes',file=sys.stderr)
   if bad:return 1
   print('the committed goasm files match the generator')
  else:
   for n in GENERATED:shutil.copyfile(out/n,root/'lib'/n)
  return 0
 finally:
  if tmp:tmp.cleanup()

if __name__=='__main__':sys.exit(main())
