// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Shadow-stack pass for the new engine generator. It runs after -O2, when the
// only allocas left are the locals whose address is taken and the dynamic
// ones. It moves every one of them onto the VM's shadow stack, a region the
// host allocates outside the Go heap, so no C address ever points into the
// movable Go stack.
//
// Contract of every engine function: argument 0 is the shadow-stack pointer,
// the first free byte of the shadow stack. A function lays its fixed locals
// out from there and hands each callee the first byte above everything it has
// live. Returning frees the frame for free, since the caller's pointer never
// moved. Dynamic allocas keep the running top in the frame's first word.
//
// The region is 2^shadow-size-log2 bytes and aligned to its size, so the
// space left is computed from the pointer alone. __shadow_remaining() gives it
// to C, which checks it at the places that can raise RangeError. A dynamic
// alloca that would cross the end traps: the C checks are meant to make that
// unreachable.
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/IntrinsicInst.h"
#include "llvm/IR/Intrinsics.h"
#include "llvm/IR/PassManager.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Plugins/PassPlugin.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Transforms/Utils/BasicBlockUtils.h"
#include <set>
#include <string>
using namespace llvm;

static cl::opt<unsigned> SizeLog2("shadow-size-log2", cl::init(20), cl::desc("log2 of the shadow-stack region size"));
static cl::opt<unsigned> Guard("shadow-guard", cl::init(16), cl::desc("bytes kept free at the region end"));
static cl::list<std::string> Leaves("shadow-leaf", cl::CommaSeparated, cl::desc("register-entry leaves: no shadow pointer"));

static void dropMemoryFacts(Function &F) {
  // The pass adds loads and stores through argument 0, so anything -O2
  // inferred about the function's memory use or about that argument is stale.
  F.removeFnAttr(Attribute::Memory);
  F.removeFnAttr(Attribute::NoSync);
  for (auto K : {Attribute::ReadNone, Attribute::ReadOnly, Attribute::WriteOnly,
                 Attribute::Captures, Attribute::NoAlias, Attribute::Dereferenceable,
                 Attribute::DereferenceableOrNull, Attribute::NonNull})
    F.removeParamAttr(0, K);
}

struct ShadowStack : PassInfoMixin<ShadowStack> {
  PreservedAnalyses run(Module &M, ModuleAnalysisManager &) {
    std::set<std::string> leaves(Leaves.begin(), Leaves.end());
    const DataLayout &DL = M.getDataLayout();
    LLVMContext &C = M.getContext();
    Type *I64 = Type::getInt64Ty(C), *I8 = Type::getInt8Ty(C);
    PointerType *P = PointerType::getUnqual(C);
    const uint64_t S = 1ull << SizeLog2;
    for (Function &F : M) {
      if (F.isDeclaration())
        continue;
      if (leaves.count(F.getName().str()))
        report_fatal_error(Twine("register-entry leaf defined in the engine unit: ") + F.getName());
      if (F.arg_size() == 0 || !F.getArg(0)->getType()->isPointerTy())
        report_fatal_error(Twine("function without a shadow-stack pointer: ") + F.getName());
      dropMemoryFacts(F);
      // The Go frame cannot be realigned: LLVM must use the stack alignment
      // the target is told (8 on amd64) and spill with unaligned moves.
      F.addFnAttr("no-realign-stack");
      Argument *ss = F.getArg(0);

      SmallVector<AllocaInst *> fixed, dynamic;
      SmallVector<IntrinsicInst *> lifetimes, saves, restores;
      SmallVector<CallBase *> calls;
      for (BasicBlock &B : F)
        for (Instruction &I : B) {
          if (auto *A = dyn_cast<AllocaInst>(&I)) {
            if (A->isStaticAlloca())
              fixed.push_back(A);
            else
              dynamic.push_back(A);
          } else if (auto *II = dyn_cast<IntrinsicInst>(&I)) {
            switch (II->getIntrinsicID()) {
            case Intrinsic::lifetime_start:
            case Intrinsic::lifetime_end:
              lifetimes.push_back(II);
              break;
            case Intrinsic::stacksave:
              saves.push_back(II);
              break;
            case Intrinsic::stackrestore:
              restores.push_back(II);
              break;
            default:
              break;
            }
          } else if (auto *CB = dyn_cast<CallBase>(&I)) {
            if (CB->isInlineAsm())
              report_fatal_error(Twine("inline assembly in ") + F.getName());
            calls.push_back(CB);
          }
        }
      for (auto *II : lifetimes)
        II->eraseFromParent();

      const bool dyn = !dynamic.empty() || !saves.empty();
      uint64_t off = dyn ? 16 : 0; // word 0 holds the running top
      SmallVector<std::pair<AllocaInst *, uint64_t>> placed;
      for (auto *A : fixed) {
        uint64_t al = A->getAlign().value();
        if (al > 16)
          report_fatal_error(Twine("local aligned beyond 16 in ") + F.getName());
        off = alignTo(off, al);
        placed.push_back({A, off});
        off += A->getAllocationSize(DL)->getFixedValue();
      }
      const uint64_t frame = alignTo(off, 16);

      IRBuilder<> E(&*F.getEntryBlock().getFirstInsertionPt());
      if (dyn)
        E.CreateStore(E.CreateConstGEP1_64(I8, ss, frame), ss);
      // The first free byte above everything live at a point.
      auto top = [&](IRBuilder<> &B) -> Value * {
        if (dyn)
          return B.CreateLoad(P, ss, "shadow.top");
        return frame ? B.CreateConstGEP1_64(I8, ss, frame) : ss;
      };

      // Each use computes the address from argument 0 where it needs it, so
      // no derived pointer has to survive a call in a register.
      for (auto [A, o] : placed) {
        for (Use &U : make_early_inc_range(A->uses())) {
          auto *I = cast<Instruction>(U.getUser());
          Instruction *at = I;
          if (auto *Phi = dyn_cast<PHINode>(I))
            at = Phi->getIncomingBlock(U)->getTerminator();
          IRBuilder<> B(at);
          U.set(B.CreateConstGEP1_64(I8, ss, o));
        }
        A->eraseFromParent();
      }

      for (auto *A : dynamic) {
        IRBuilder<> B(A);
        Value *cur = B.CreateLoad(P, ss, "shadow.top");
        Value *ci = B.CreatePtrToInt(cur, I64);
        Value *start = B.CreateAnd(B.CreateAdd(ci, B.getInt64(15)), B.getInt64(~15ull));
        Value *n = B.CreateMul(B.CreateZExtOrTrunc(A->getArraySize(), I64),
                               B.getInt64(DL.getTypeAllocSize(A->getAllocatedType()).getFixedValue()));
        n = B.CreateAnd(B.CreateAdd(n, B.getInt64(15)), B.getInt64(~15ull));
        Value *end = B.CreateAdd(start, n);
        Value *base = B.CreateAnd(B.CreatePtrToInt(ss, I64), B.getInt64(~(S - 1)));
        Value *limit = B.CreateAdd(base, B.getInt64(S - Guard));
        // end < start catches a size that wrapped around.
        Value *bad = B.CreateOr(B.CreateICmpUGT(end, limit), B.CreateICmpULT(end, start));
        Instruction *trap = SplitBlockAndInsertIfThen(bad, A, true);
        IRBuilder<> T(trap);
        T.CreateIntrinsic(Intrinsic::trap, {});
        B.SetInsertPoint(A);
        Value *p = B.CreateGEP(I8, cur, B.CreateSub(start, ci));
        B.CreateStore(B.CreateGEP(I8, cur, B.CreateSub(end, ci)), ss);
        A->replaceAllUsesWith(p);
        A->eraseFromParent();
      }
      for (auto *II : saves) {
        IRBuilder<> B(II);
        II->replaceAllUsesWith(top(B));
        II->eraseFromParent();
      }
      for (auto *II : restores) {
        IRBuilder<> B(II);
        B.CreateStore(II->getArgOperand(0), ss);
        II->eraseFromParent();
      }

      for (auto *CB : calls) {
        Function *callee = CB->getCalledFunction();
        if (callee && callee->isIntrinsic())
          continue;
        IRBuilder<> B(CB);
        if (callee && callee->getName() == "__shadow_remaining") {
          Value *used = B.CreateAnd(B.CreatePtrToInt(top(B), I64), B.getInt64(S - 1));
          CB->replaceAllUsesWith(B.CreateSub(B.getInt64(S - Guard), used));
          CB->eraseFromParent();
          continue;
        }
        if (callee && leaves.count(callee->getName().str()))
          continue;
        if (CB->arg_size() == 0 || !CB->getArgOperand(0)->getType()->isPointerTy())
          report_fatal_error(Twine("call without a shadow-stack pointer in ") + F.getName());
        CB->removeFnAttr(Attribute::Memory);
        for (auto K : {Attribute::ReadNone, Attribute::ReadOnly, Attribute::WriteOnly,
                       Attribute::Captures, Attribute::NoAlias})
          CB->removeParamAttr(0, K);
        CB->setArgOperand(0, top(B));
      }
      errs() << "SHADOW " << F.getName() << " " << frame << (dyn ? " dynamic" : "") << "\n";
    }
    for (Function &F : M)
      for (BasicBlock &B : F)
        for (Instruction &I : B)
          if (isa<AllocaInst>(&I))
            report_fatal_error(Twine("alloca left on the Go stack in ") + F.getName());
    if (Function *R = M.getFunction("__shadow_remaining"))
      if (R->use_empty())
        R->eraseFromParent();
    return PreservedAnalyses::none();
  }
};

extern "C" LLVM_ATTRIBUTE_WEAK PassPluginLibraryInfo llvmGetPassPluginInfo() {
  return {LLVM_PLUGIN_API_VERSION, "goasm-shadow-stack", "22.1.8", [](PassBuilder &PB) {
            PB.registerPipelineParsingCallback(
                [](StringRef name, ModulePassManager &PM, ArrayRef<PassBuilder::PipelineElement>) {
                  if (name != "goasm-shadow-stack")
                    return false;
                  PM.addPass(ShadowStack());
                  return true;
                });
          }};
}
