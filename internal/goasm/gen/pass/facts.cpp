// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// llc plugin, built into the same shared object as the shadow-stack pass and
// loaded with llc -load. With -goasm-facts=<file> it writes down, for every
// function llc compiles, what llc itself decided about registers, so the
// post-link check does not take it from the generator's own model of the
// calling convention (facts.go reads the file):
//
//	fn <name> args <regs> entry <regs>
//	call <name> <n> <target or *id> args <regs> defs <regs> after <regs>
//	ret <name> <n> uses <regs>
//
// args of a function are its live-in registers in argument order (only
// the arguments its code reads: a probe function of -shadow-probes reads
// all of them), entry the registers its code reads before writing them.
// Each call, in code order, lists the argument registers in order (the
// call's implicit uses), the registers it defines (its results) and the
// registers live right after it, which its code reads later before
// writing them. Each return lists the registers it returns. Registers are
// llc's names in lower case, every name of a register that is live (eax
// and al beside rax); <regs> is "-" for none. Reserved registers are left
// out (the stack and frame pointers, the control and status registers,
// the ones -ffixed takes): llc keeps no value of its own in them.
//
// It runs where icall.cpp's pass does, after the last machine pass that
// moves or duplicates a call in the generator's llc run, and changes
// nothing.
#include "llvm/CodeGen/LivePhysRegs.h"
#include "llvm/CodeGen/MachineFunction.h"
#include "llvm/CodeGen/MachineFunctionPass.h"
#include "llvm/CodeGen/MachineInstr.h"
#include "llvm/CodeGen/MachineRegisterInfo.h"
#include "llvm/CodeGen/Passes.h"
#include "llvm/CodeGen/TargetPassConfig.h"
#include "llvm/CodeGen/TargetRegisterInfo.h"
#include "llvm/CodeGen/TargetSubtargetInfo.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/ErrorHandling.h"
#include "llvm/Support/FileSystem.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Target/RegisterTargetPassConfigCallback.h"
#include <map>
#include <memory>
#include <string>
using namespace llvm;

static cl::opt<std::string> FactsOut("goasm-facts", cl::init(""),
                                     cl::desc("write the registers llc assigns and keeps live to this file"));

namespace {

std::unique_ptr<raw_fd_ostream> out;

raw_fd_ostream &facts() {
  if (!out) {
    std::error_code EC;
    out = std::make_unique<raw_fd_ostream>(FactsOut, EC, sys::fs::OF_Text);
    if (EC)
      report_fatal_error(Twine("cannot write ") + FactsOut + ": " + EC.message());
  }
  return *out;
}

struct Facts : MachineFunctionPass {
  static char ID;
  Facts() : MachineFunctionPass(ID) {}
  StringRef getPassName() const override { return "goasm register facts"; }
  void getAnalysisUsage(AnalysisUsage &AU) const override {
    AU.setPreservesAll();
    MachineFunctionPass::getAnalysisUsage(AU);
  }

  const TargetRegisterInfo *TRI = nullptr;

  std::string name(MCRegister R) const { return StringRef(TRI->getName(R)).lower(); }

  std::string list(const SmallVectorImpl<MCRegister> &rs) const {
    if (rs.empty())
      return "-";
    std::string s;
    for (MCRegister R : rs) {
      if (!s.empty())
        s += ",";
      s += name(R);
    }
    return s;
  }

  std::string live(const LivePhysRegs &L, const MachineRegisterInfo &MRI) const {
    SmallVector<MCRegister> rs;
    for (MCPhysReg R : L)
      if (!MRI.isReserved(R))
        rs.push_back(R);
    llvm::sort(rs);
    return list(rs);
  }

  bool runOnMachineFunction(MachineFunction &MF) override {
    if (FactsOut.empty())
      return false;
    TRI = MF.getSubtarget().getRegisterInfo();
    raw_fd_ostream &OS = facts();
    StringRef fn = MF.getName();
    const MachineRegisterInfo &MRI = MF.getRegInfo();

    // Liveness as llc keeps it: each block's live-outs are its successors'
    // live-ins, without the callee-saved registers the function leaves
    // alone (they are live through, never read).
    DenseMap<const MachineInstr *, std::string> after;
    std::string entry = "-";
    for (MachineBasicBlock &MBB : MF) {
      LivePhysRegs L(*TRI);
      L.addLiveOutsNoPristines(MBB);
      for (MachineInstr &MI : llvm::reverse(MBB)) {
        if (MI.isCall())
          after[&MI] = live(L, MRI);
        L.stepBackward(MI);
      }
      if (&MBB == &MF.front())
        entry = live(L, MRI);
    }

    SmallVector<MCRegister> args;
    for (const auto &LI : MRI.liveins())
      args.push_back(LI.first);
    OS << "fn " << fn << " args " << list(args) << " entry " << entry << "\n";

    unsigned calls = 0, rets = 0;
    for (MachineBasicBlock &MBB : MF)
      for (MachineInstr &MI : MBB) {
        if (MI.isCall()) {
          const MachineOperand &T = MI.getOperand(0);
          std::string target;
          if (T.isGlobal())
            target = T.getGlobal()->getName().str();
          else if (T.isSymbol())
            target = T.getSymbolName();
          else if (T.isMCSymbol())
            target = T.getMCSymbol()->getName().str();
          else
            target = "*" + std::to_string(MI.getCFIType());
          SmallVector<MCRegister> uses, defs;
          for (const MachineOperand &O : MI.implicit_operands()) {
            if (!O.isReg() || !O.getReg().isPhysical())
              continue;
            MCRegister R = O.getReg().asMCReg();
            if (MRI.isReserved(R))
              continue; // the stack pointer and the like
            (O.isDef() ? defs : uses).push_back(R);
          }
          OS << "call " << fn << " " << calls++ << " " << target << " args " << list(uses) << " defs " << list(defs)
             << " after " << after.lookup(&MI) << "\n";
        }
        if (MI.isReturn()) {
          // A return's registers are operands of its own (x86's RET64
          // takes them as variable operands), not only implicit ones.
          SmallVector<MCRegister> uses;
          for (const MachineOperand &O : MI.operands())
            if (O.isReg() && O.isUse() && O.getReg().isPhysical() && !MRI.isReserved(O.getReg()))
              uses.push_back(O.getReg().asMCReg());
          OS << "ret " << fn << " " << rets++ << " uses " << list(uses) << "\n";
        }
      }
    return false;
  }
};

char Facts::ID = 0;

RegisterTargetPassConfigCallback Register([](TargetMachine &, PassManagerBase &, TargetPassConfig *PC) {
  PC->insertPass(&MachineSanitizerBinaryMetadataID, new Facts());
});

} // namespace
