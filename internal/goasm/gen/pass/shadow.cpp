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
static cl::opt<std::string> CallConv("shadow-cc", cl::init(""),
                                     cl::desc("calling convention of unified entries and host callbacks: empty (C) or preserve_none"));
static cl::opt<unsigned> MaxIntArgs("shadow-max-int-args", cl::init(0),
                                    cl::desc("most integer argument registers a unified function may use: more go in a block (0: no limit)"));
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

// Integer argument registers a function type takes: a pointer or integer one,
// a [2 x i64] (JSValue on aarch64) two, a double none.
static unsigned intArgRegs(FunctionType *T) {
  unsigned n = 0;
  for (Type *P : T->params()) {
    if (P->isFloatingPointTy())
      continue;
    if (auto *A = dyn_cast<ArrayType>(P))
      n += A->getNumElements();
    else if (auto *S = dyn_cast<StructType>(P))
      n += S->getNumElements();
    else
      n += 1;
  }
  return n;
}

static bool unifiedFn(Function *F, const std::set<std::string> &leaves);

// Option B: every unified entry and every host callback uses preserve_none,
// whose only callee-saved registers are FP and LR (amd64: RSP and RBP). The
// register allocator then keeps across a call only what is live, in the
// frame, and no function saves registers for its caller, which matches Go's
// ABI0 where every register is clobbered. Register-entry leaves stay C.
// preserve_none places the 7th and later integer arguments in registers the
// engine reserves (x26 to x28 on aarch64), so the count is limited: packArgs
// has moved the arguments of every function over it into a block.
static void usePreserveNone(Module &M, const std::set<std::string> &leaves) {
  auto unified = [&](Function *F) { return unifiedFn(F, leaves); };
  for (Function &F : M) {
    if (!unified(&F) || F.use_empty() && F.isDeclaration())
      continue;
    if (MaxIntArgs && intArgRegs(F.getFunctionType()) > MaxIntArgs)
      report_fatal_error(Twine("preserve_none function with more than ") + Twine(MaxIntArgs) +
                         " integer argument registers: " + F.getName());
    F.setCallingConv(CallingConv::PreserveNone);
  }
  for (Function &F : M)
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I)) {
          Function *callee = CB->getCalledFunction();
          if (callee ? !unified(callee) : false)
            continue;
          if (!callee && MaxIntArgs && intArgRegs(CB->getFunctionType()) > MaxIntArgs)
            report_fatal_error(Twine("indirect preserve_none call with too many integer arguments in ") + F.getName());
          CB->setCallingConv(CallingConv::PreserveNone);
        }
}

// A unified function that needs more integer argument registers than
// preserve_none may use becomes f(sp, blk): the caller stores every argument
// after the shadow pointer in a block, 8-byte aligned in parameter order, and
// the callee loads them back at its entry. The block is an alloca in the
// caller, so the frame layout below puts it on the shadow stack, under the
// shadow pointer the callee receives. This runs before that layout.
static bool unifiedFn(Function *F, const std::set<std::string> &leaves) {
  return F && !F->isIntrinsic() && !leaves.count(F->getName().str()) && F->getName() != "__shadow_remaining";
}

static uint64_t blockLayout(FunctionType *T, const DataLayout &DL, SmallVectorImpl<uint64_t> &offs) {
  uint64_t off = 0;
  for (unsigned i = 1; i < T->getNumParams(); i++) {
    off = alignTo(off, 8);
    offs.push_back(off);
    off += DL.getTypeStoreSize(T->getParamType(i)).getFixedValue();
  }
  return alignTo(off, 16);
}

static FunctionType *packedType(FunctionType *T) {
  PointerType *P = PointerType::getUnqual(T->getContext());
  return FunctionType::get(T->getReturnType(), {P, P}, false);
}

static void packCall(CallBase *CB, Value *callee, const DataLayout &DL) {
  auto *CI = dyn_cast<CallInst>(CB);
  if (!CI)
    report_fatal_error(Twine("argument block on a call that is not a plain call in ") + CB->getFunction()->getName());
  FunctionType *T = CB->getFunctionType();
  SmallVector<uint64_t> offs;
  uint64_t size = blockLayout(T, DL, offs);
  Function *F = CB->getFunction();
  LLVMContext &C = F->getContext();
  Type *I8 = Type::getInt8Ty(C);
  IRBuilder<> E(&*F->getEntryBlock().getFirstInsertionPt());
  AllocaInst *blk = E.CreateAlloca(ArrayType::get(I8, size), nullptr, "argblock");
  blk->setAlignment(Align(16));
  IRBuilder<> B(CB);
  for (unsigned i = 1; i < CB->arg_size(); i++)
    B.CreateAlignedStore(CB->getArgOperand(i), B.CreateConstGEP1_64(I8, blk, offs[i - 1]), Align(8));
  CallInst *N = B.CreateCall(packedType(T), callee, {CB->getArgOperand(0), blk});
  N->setCallingConv(CB->getCallingConv());
  N->setAttributes(AttributeList::get(C, CB->getAttributes().getFnAttrs(), CB->getAttributes().getRetAttrs(), {}));
  N->setTailCallKind(CallInst::TCK_NoTail);
  CB->replaceAllUsesWith(N);
  CB->eraseFromParent();
}

static void packArgs(Module &M, const std::set<std::string> &leaves) {
  const DataLayout &DL = M.getDataLayout();
  LLVMContext &C = M.getContext();
  Type *I8 = Type::getInt8Ty(C);
  SmallVector<Function *> over;
  for (Function &F : M)
    if (unifiedFn(&F, leaves) && intArgRegs(F.getFunctionType()) > MaxIntArgs) {
      if (F.isDeclaration() && !F.use_empty())
        report_fatal_error(Twine("host callback with more than ") + Twine(MaxIntArgs) +
                           " integer argument registers: " + F.getName());
      if (!F.isDeclaration())
        over.push_back(&F);
    }
  for (Function *F : over) {
    FunctionType *T = F->getFunctionType();
    SmallVector<uint64_t> offs;
    blockLayout(T, DL, offs);
    Function *NF = Function::Create(packedType(T), F->getLinkage(), F->getAddressSpace(), "", &M);
    NF->copyAttributesFrom(F);
    NF->setAttributes(AttributeList::get(C, F->getAttributes().getFnAttrs(), F->getAttributes().getRetAttrs(), {}));
    NF->takeName(F);
    NF->splice(NF->begin(), F);
    IRBuilder<> B(&*NF->getEntryBlock().getFirstInsertionPt());
    F->getArg(0)->replaceAllUsesWith(NF->getArg(0));
    for (unsigned i = 1; i < T->getNumParams(); i++) {
      Value *v = B.CreateAlignedLoad(T->getParamType(i), B.CreateConstGEP1_64(I8, NF->getArg(1), offs[i - 1]), Align(8));
      F->getArg(i)->replaceAllUsesWith(v);
    }
    for (Use &U : make_early_inc_range(F->uses())) {
      auto *CB = dyn_cast<CallBase>(U.getUser());
      if (CB && CB->isCallee(&U))
        packCall(CB, NF, DL);
      else
        U.set(NF); // the address: a function table or an argument
    }
    F->eraseFromParent();
    errs() << "PACKED " << NF->getName() << "\n";
  }
  for (Function &F : M) {
    SmallVector<CallBase *> ind;
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I))
          if (!CB->getCalledFunction() && !CB->isInlineAsm() && intArgRegs(CB->getFunctionType()) > MaxIntArgs)
            ind.push_back(CB);
    for (CallBase *CB : ind)
      packCall(CB, CB->getCalledOperand(), DL);
  }
}

struct ShadowStack : PassInfoMixin<ShadowStack> {
  PreservedAnalyses run(Module &M, ModuleAnalysisManager &) {
    std::set<std::string> leaves(Leaves.begin(), Leaves.end());
    const DataLayout &DL = M.getDataLayout();
    LLVMContext &C = M.getContext();
    Type *I64 = Type::getInt64Ty(C), *I8 = Type::getInt8Ty(C);
    PointerType *P = PointerType::getUnqual(C);
    const uint64_t S = 1ull << SizeLog2;
    if (MaxIntArgs)
      packArgs(M, leaves);
    for (Function &F : M) {
      if (F.isDeclaration())
        continue;
      if (leaves.count(F.getName().str())) {
        // A register-entry leaf takes no shadow pointer and may own no
        // memory: it must not have a local whose address is taken.
        for (BasicBlock &B : F)
          for (Instruction &I : B)
            if (isa<AllocaInst>(&I))
              report_fatal_error(Twine("register-entry leaf with an addressable local: ") + F.getName());
        continue;
      }
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
    if (CallConv == "preserve_none")
      usePreserveNone(M, leaves);
    else if (!CallConv.empty())
      report_fatal_error(Twine("unknown -shadow-cc ") + CallConv);
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
