// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// llc plugin, built into the same shared object as the shadow-stack pass and
// loaded with llc -load. The pass gives every indirect call a "kcfi" operand
// bundle whose type id encodes the call's signature (-shadow-icall-sigs).
// Without the "kcfi" module flag llc emits no check for it, but it keeps the
// id on the machine call as its CFI type, through instruction selection,
// tail merging (which does not merge calls of different ids) and register
// allocation. This pass runs after the machine passes and names a symbol at
// each such call, goasm.icall.<function>.<n>.<id>, which llc writes as a
// label right before the call in its text and as a local symbol at the
// call's address in its object. The generator reads the signature of each
// indirect call from it, and the post-link check reads it again from the
// object. It changes no instruction. An indirect call without an id stops
// llc.
#include "llvm/CodeGen/MachineFunction.h"
#include "llvm/CodeGen/MachineFunctionPass.h"
#include "llvm/CodeGen/MachineInstr.h"
#include "llvm/CodeGen/Passes.h"
#include "llvm/CodeGen/TargetPassConfig.h"
#include "llvm/MC/MCContext.h"
#include "llvm/Support/ErrorHandling.h"
#include "llvm/Target/RegisterTargetPassConfigCallback.h"
using namespace llvm;

namespace {

struct ICallSymbols : MachineFunctionPass {
  static char ID;
  ICallSymbols() : MachineFunctionPass(ID) {}
  StringRef getPassName() const override { return "goasm indirect-call symbols"; }

  bool runOnMachineFunction(MachineFunction &MF) override {
    unsigned n = 0;
    bool changed = false;
    for (MachineBasicBlock &MBB : MF)
      for (MachineInstr &MI : MBB) {
        if (!MI.isCall())
          continue;
        const MachineOperand &T = MI.getOperand(0);
        bool direct = T.isGlobal() || T.isSymbol() || T.isMCSymbol();
        if (!MI.getCFIType()) {
          if (!direct)
            reportFatalUsageError("indirect call without a signature id in " + MF.getName());
          continue;
        }
        if (direct)
          reportFatalUsageError("direct call with a signature id in " + MF.getName());
        if (MI.getPreInstrSymbol())
          reportFatalUsageError("indirect call that already has a symbol in " + MF.getName());
        MI.setPreInstrSymbol(MF, MF.getContext().getOrCreateSymbol("goasm.icall." + MF.getName() + "." + Twine(n++) +
                                                                   "." + Twine(MI.getCFIType())));
        changed = true;
      }
    return changed;
  }
};

char ICallSymbols::ID = 0;

// Inserted after MachineSanitizerBinaryMetadata. That is the last machine pass
// that moves, merges or duplicates a call in the llc run the generator makes
// (llc -O2, -enable-shrink-wrap=false, x86-64 with the engine's C flags), which
// the tests cover. It is not the last machine pass LLVM 22.1.8 has: the
// optional MachineOutliner, function splitting and the pre-emit passes run
// after it, and none of them is on in that run. Check this insertion point
// again when an llc option, the LLVM version or the target (arm64 lowering)
// changes. A symbol that ends up missing, in the wrong function or on
// something that is not an indirect call still fails the generator and the
// post-link check, so a wrong place cannot go unseen.
RegisterTargetPassConfigCallback Register([](TargetMachine &, PassManagerBase &, TargetPassConfig *PC) {
  PC->insertPass(&MachineSanitizerBinaryMetadataID, new ICallSymbols());
});

} // namespace
