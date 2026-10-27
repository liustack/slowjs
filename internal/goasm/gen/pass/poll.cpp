// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Polls (-shadow-poll). The generated code is Go assembly, which the Go
// runtime never stops asynchronously: the garbage collector and the
// scheduler wait for a goroutine in the engine until it enters a function
// that checks the stack. A loop of C that calls no such function would keep
// them waiting for as long as it runs. So every cycle of every engine
// function counts down a counter, a tick, and when the counter runs out the
// code calls host_poll, a Go function whose stack check is where the runtime
// stops the goroutine. host_poll also sets the counter again, to the
// interval of the VM, and is where the host sees an interrupt.
//
// The counter is a 64-bit word at a fixed offset (-shadow-poll-offset) from
// the base of the shadow-stack region, which the host keeps for it: the
// region is aligned to its size, so the base is the shadow pointer with its
// low bits cleared. Each loop counts where that costs less (chooseLocals):
//
//   - A loop that keeps the count in a local loads the word into it where the
//     loop is entered and stores it back where the loop is left, and stores
//     it before and loads it after each call in the loop that may tick, so
//     its ticks are a decrement of a register, mem2reg's. Its inner loops
//     count in the same local.
//   - A loop that counts in memory loads and stores the word at each tick,
//     and its inner loops choose for themselves. A loop that ticks only at
//     the ends of its chunks counts in memory, and so does one whose hot
//     code already needs about every register.
//   - Ticks on cycles outside these count in memory.
//
// Every retreating edge of a depth-first walk gets a tick, since every cycle
// has one, but those of a loop that scalar evolution proves short
// (boundedEdges): whatever enters such a loop again ticks. An innermost loop
// that calls nothing and is not proved short runs in chunks (stripLoops),
// which makes it short, and the end of each chunk ticks for the chunk.
// Calls of memcpy, memmove and memset, which have no ticks, go through
// helpers that do at most LeafChunk bytes at a time (chunkLeafCalls).
//
// The interpreter counts its bytecode itself: the C marks where it counts
// (poll_count in engine/cutils.h), at the entry of JS_CallInternal and where
// pc does not go forward, and the compiler of the C holds the bytecode to a
// bound of instructions between two of them (check_polls in quickjs.c). Each
// mark becomes a tick of its weight (takePollMarks), and the edges into the
// dispatch of a function with an entry mark get none, so that the dispatch
// stays one short block that llc copies into the end of every handler.
//
// checkPolls verifies the result on its own: removing the blocks that tick or
// poll from each function, and the edges into a dispatch that counts its
// bytecode, must leave no cycle, each mark must come after a tick of its
// weight, each tick must take from the count as it stands, and each call that
// may tick and each return must find the count in memory (checkCounts).
#include "llvm/ADT/BitVector.h"
#include "llvm/ADT/DenseMap.h"
#include "llvm/ADT/PostOrderIterator.h"
#include "llvm/ADT/SCCIterator.h"
#include "llvm/Analysis/AssumptionCache.h"
#include "llvm/Analysis/BlockFrequencyInfo.h"
#include "llvm/Analysis/BranchProbabilityInfo.h"
#include "llvm/Analysis/LoopInfo.h"
#include "llvm/Analysis/PostDominators.h"
#include "llvm/Analysis/ScalarEvolution.h"
#include "llvm/Analysis/ScalarEvolutionExpressions.h"
#include "llvm/Analysis/TargetLibraryInfo.h"
#include "llvm/IR/CFG.h"
#include "llvm/IR/Dominators.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/Instructions.h"
#include "llvm/IR/IntrinsicInst.h"
#include "llvm/IR/MDBuilder.h"
#include "llvm/IR/Module.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/ErrorHandling.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Transforms/Utils/BasicBlockUtils.h"
#include "llvm/Transforms/Utils/Cloning.h"
#include "llvm/Transforms/Utils/LoopSimplify.h"
#include "llvm/Transforms/Utils/LoopUtils.h"
#include "llvm/Transforms/Utils/PromoteMemToReg.h"
#include "llvm/Transforms/Utils/ValueMapper.h"
#include <functional>
#include <map>
#include <set>
#include <string>
#include <vector>
using namespace llvm;

static cl::opt<bool> PollOn("shadow-poll", cl::init(false),
                            cl::desc("tick on every cycle and call host_poll when the count runs out"));
static cl::opt<unsigned> PollOffset("shadow-poll-offset", cl::init(0),
                                    cl::desc("offset of the poll counter from the base of the shadow-stack region"));
static cl::opt<std::string> PollFault("shadow-poll-fault", cl::init(""), cl::Hidden,
                                      cl::desc("for the tests of checkPolls only: 'cond' gives every tick a test "
                                               "that never holds, 'entry' takes out the entry mark, "
                                               "'mark' those of the other marks, 'callstore' and 'callload' leave "
                                               "out the store before and the load after each call that may tick in "
                                               "code that keeps the count in a register, 'exit' the store where a "
                                               "loop that keeps it in a register is left"));
static cl::opt<bool> PollTrace("shadow-poll-trace", cl::init(false), cl::Hidden,
                               cl::desc("print where each loop keeps the count, and why"));
static cl::opt<bool> PollStats("shadow-poll-stats", cl::init(false), cl::Hidden,
                               cl::desc("print the ticks, locals and syncs of each function"));

static const char *TickMD = "goasm.tick";
// MarkMD is on the marks of takePollMarks: !{weight, entry}. CountedMD is on
// the dispatch of a function with an entry mark.
static const char *MarkMD = "goasm.poll.mark";
static const char *CountedMD = "goasm.poll.counted";

// checkPollOptions stops on an offset that is not a word inside the region.
void checkPollOptions(unsigned sizeLog2) {
  if (!PollOn)
    return;
  if (PollOffset % 8 != 0 || PollOffset + 8 > (1ull << sizeLog2))
    reportFatalUsageError(Twine("-shadow-poll-offset=") + Twine(PollOffset) + " is not a word inside the region");
  if (PollFault != "" && PollFault != "cond" && PollFault != "entry" && PollFault != "mark" &&
      PollFault != "callstore" && PollFault != "callload" && PollFault != "exit")
    reportFatalUsageError(Twine("-shadow-poll-fault=") + PollFault.getValue() +
                          " is not cond, entry, mark, callstore, callload or exit");
}

// The one indirect branch of F, when F has exactly one: the dispatch of its
// computed gotos.
static IndirectBrInst *dispatchOf(Function &F) {
  IndirectBrInst *IB = nullptr;
  unsigned n = 0;
  for (BasicBlock &B : F)
    if (auto *I = dyn_cast<IndirectBrInst>(B.getTerminator()))
      IB = I, n++;
  return n == 1 ? IB : nullptr;
}

// A mark of takePollMarks: its weight, and whether it is an entry mark.
static bool pollMark(const Instruction &I, uint64_t *weight = nullptr, bool *entry = nullptr) {
  MDNode *N = I.getMetadata(MarkMD);
  if (!N)
    return false;
  if (weight)
    *weight = mdconst::extract<ConstantInt>(N->getOperand(0))->getZExtValue();
  if (entry)
    *entry = mdconst::extract<ConstantInt>(N->getOperand(1))->isOne();
  return true;
}

// takePollMarks runs first. The C marks where the interpreter counts its
// bytecode with calls of goasm_poll_count(n) and, at the entry of
// JS_CallInternal, goasm_poll_entry(n) (poll_count in engine/cutils.h). Each
// becomes a mark, a call of llvm.donothing with MarkMD, where insertPolls
// puts a tick of n. The function with an entry mark must have one indirect
// branch, its dispatch, which gets CountedMD: the bytecode counts on the
// cycles through it, so the edges into it need no tick. Without -shadow-poll
// the calls go.
void takePollMarks(Module &M) {
  LLVMContext &C = M.getContext();
  Type *I64 = Type::getInt64Ty(C), *I1 = Type::getInt1Ty(C);
  for (const char *name : {"goasm_poll_count", "goasm_poll_entry"}) {
    Function *G = M.getFunction(name);
    if (!G)
      continue;
    bool entry = StringRef(name) == "goasm_poll_entry";
    for (User *U : make_early_inc_range(G->users())) {
      auto *CB = dyn_cast<CallInst>(U);
      auto *W = CB && CB->getCalledFunction() == G && CB->arg_size() == 1 ? dyn_cast<ConstantInt>(CB->getArgOperand(0))
                                                                            : nullptr;
      if (!W || W->getSExtValue() < 1)
        reportFatalUsageError(Twine("poll: ") + name + " used other than in a call with a weight of at least 1");
      Function *F = CB->getFunction();
      if (PollOn) {
        if (entry) {
          IndirectBrInst *IB = dispatchOf(*F);
          if (!IB)
            reportFatalUsageError(Twine("poll: an entry mark in a function without exactly one indirect branch: ") +
                                  F->getName());
          IB->setMetadata(CountedMD, MDNode::get(C, {}));
        }
        IRBuilder<> B(CB);
        auto *mark = cast<CallInst>(B.CreateIntrinsic(Intrinsic::donothing, {}));
        mark->setMetadata(MarkMD, MDNode::get(C, {ConstantAsMetadata::get(ConstantInt::get(I64, W->getZExtValue())),
                                                  ConstantAsMetadata::get(ConstantInt::get(I1, entry))}));
      }
      CB->eraseFromParent();
    }
    if (G->use_empty())
      G->eraseFromParent();
  }
}

// dropPollMarks runs after checkPolls: the marks have done their work.
void dropPollMarks(Module &M) {
  for (Function &F : M)
    for (BasicBlock &B : F)
      for (Instruction &I : make_early_inc_range(B))
        if (pollMark(I))
          I.eraseFromParent();
  if (Function *D = M.getFunction("llvm.donothing"))
    if (D->use_empty())
      D->eraseFromParent();
}

// LeafChunk is the most a call of a register-entry leaf (memcpy, memmove,
// memset) copies or sets at once: the leaves have no ticks, so a longer one
// goes through a chunked helper, whose loop ticks LeafChunkWeight per chunk.
static const uint64_t LeafChunk = 256 << 10, LeafChunkWeight = 65536;
static const char *ChunkWeightMD = "goasm.poll.weight";

// chunkHelper defines goasm_chunked_<op>(dst, src or byte, n), which calls
// the leaf op on at most LeafChunk bytes at a time: forward, or for memmove
// onto an overlapping higher destination, backward from the end. It is built
// as clang builds the C (like, a function of the C that calls the leaf):
// with its target attributes and frame pointer.
static Function *chunkHelper(Module &M, StringRef op, Function *leaf, Function *like) {
  LLVMContext &C = M.getContext();
  Type *I64 = Type::getInt64Ty(C), *I32 = Type::getInt32Ty(C), *I8 = Type::getInt8Ty(C);
  PointerType *P = PointerType::getUnqual(C);
  bool set = op == "memset";
  std::string name = ("goasm_chunked_" + op).str();
  if (Function *F = M.getFunction(name))
    return F;
  Function *F = Function::Create(FunctionType::get(Type::getVoidTy(C), {P, set ? (Type *)I32 : P, I64}, false),
                                 GlobalValue::InternalLinkage, name, M);
  for (const Attribute &A : like->getAttributes().getFnAttrs())
    F->addFnAttr(A);
  for (Attribute::AttrKind K : {Attribute::AlwaysInline, Attribute::OptimizeNone, Attribute::Cold, Attribute::Hot,
                                Attribute::NoReturn, Attribute::Naked, Attribute::MinSize, Attribute::OptimizeForSize})
    F->removeFnAttr(K);
  F->addFnAttr(Attribute::NoInline);
  Argument *D = F->getArg(0), *S = F->getArg(1), *N = F->getArg(2);
  BasicBlock *entry = BasicBlock::Create(C, "entry", F);
  BasicBlock *exit = BasicBlock::Create(C, "exit", F);
  ReturnInst::Create(C, exit);
  MDNode *weight = MDNode::get(C, ConstantAsMetadata::get(ConstantInt::get(I64, LeafChunkWeight)));
  auto call = [&](IRBuilder<> &B, Value *d, Value *s, Value *n) {
    Value *a1 = s;
    if (!leaf) {
      if (set)
        B.CreateMemSet(d, B.CreateTrunc(s, I8), n, MaybeAlign());
      else if (op == "memmove")
        B.CreateMemMove(d, MaybeAlign(), s, MaybeAlign(), n);
      else
        B.CreateMemCpy(d, MaybeAlign(), s, MaybeAlign(), n);
      return;
    }
    B.CreateCall(leaf, {d, a1, n});
  };
  // One loop: at offset off (counting up from 0, or down from n), copy
  // umin(left, LeafChunk) bytes.
  auto loop = [&](BasicBlock *from, bool back) -> BasicBlock * {
    BasicBlock *L = BasicBlock::Create(C, back ? "back" : "fwd", F);
    IRBuilder<> FB(from);
    FB.CreateBr(L);
    IRBuilder<> B(L);
    PHINode *left = B.CreatePHI(I64, 2, "left");
    left->addIncoming(N, from);
    Value *c = B.CreateBinaryIntrinsic(Intrinsic::umin, left, B.getInt64(LeafChunk));
    Value *left1 = B.CreateSub(left, c);
    Value *off = back ? left1 : B.CreateSub(N, left);
    Value *d = B.CreateGEP(I8, D, off);
    Value *s = set ? (Value *)S : B.CreateGEP(I8, S, off);
    call(B, d, s, c);
    left->addIncoming(left1, L);
    BranchInst *Br = B.CreateCondBr(B.CreateICmpNE(left1, B.getInt64(0)), L, exit);
    Br->setMetadata(ChunkWeightMD, weight);
    return L;
  };
  if (op == "memmove") {
    IRBuilder<> B(entry);
    Value *d = B.CreatePtrToInt(D, I64), *s = B.CreatePtrToInt(S, I64);
    Value *over = B.CreateAnd(B.CreateICmpUGT(d, s), B.CreateICmpULT(d, B.CreateAdd(s, N)));
    BasicBlock *f = BasicBlock::Create(C, "pickfwd", F), *b = BasicBlock::Create(C, "pickback", F);
    B.CreateCondBr(over, b, f);
    loop(f, false);
    loop(b, true);
  } else {
    loop(entry, false);
  }
  return F;
}

// chunkLeafCalls sends every call of memcpy, memmove or memset (the leaves,
// or the memory intrinsics llc turns into calls of them) whose length may
// be over LeafChunk to the chunked helper when it is. It runs before the
// shadow pointer is added, so the helpers get one like any function.
void chunkLeafCalls(Module &M) {
  if (!PollOn)
    return;
  LLVMContext &C = M.getContext();
  Type *I32 = Type::getInt32Ty(C);
  MDBuilder MDB(C);
  struct Site {
    CallBase *CB;
    StringRef op;
  };
  std::vector<Site> sites;
  for (Function &F : M) {
    if (F.isDeclaration() || F.getName().starts_with("goasm_chunked_") || F.getName() == "memcpy" ||
        F.getName() == "memmove" || F.getName() == "memset")
      continue;
    for (BasicBlock &B : F)
      for (Instruction &I : B) {
        auto *CB = dyn_cast<CallBase>(&I);
        if (!CB)
          continue;
        StringRef op;
        if (auto *MI = dyn_cast<MemIntrinsic>(CB)) {
          if (MI->isVolatile() || MI->isForceInlined() || isa<MemSetPatternInst>(MI))
            continue;
          op = isa<MemSetInst>(MI) ? "memset" : isa<MemMoveInst>(MI) ? "memmove" : "memcpy";
        } else if (Function *G = CB->getCalledFunction()) {
          if (G->getName() == "memcpy" || G->getName() == "memmove" || G->getName() == "memset")
            op = G->getName();
        }
        if (op.empty())
          continue;
        Value *n = CB->getArgOperand(2);
        if (auto *K = dyn_cast<ConstantInt>(n); K && K->getZExtValue() <= LeafChunk)
          continue;
        if (!n->getType()->isIntegerTy(64))
          reportFatalUsageError(Twine("poll: a ") + op + " call whose length is not 64-bit in " + F.getName());
        if (!isa<CallInst>(CB))
          reportFatalUsageError(Twine("poll: a ") + op + " call that is not a plain call in " + F.getName());
        sites.push_back({CB, op});
      }
  }
  unsigned n = 0;
  for (Site &st : sites) {
    // The operands as they are now: an earlier site may have replaced the
    // result of its leaf call, which this one used.
    Value *d = st.CB->getArgOperand(0), *src = st.CB->getArgOperand(1), *len = st.CB->getArgOperand(2);
    Function *leaf = M.getFunction(st.op);
    if (leaf && leaf->getFunctionType()->getNumParams() != 3)
      reportFatalUsageError(Twine("poll: ") + st.op + " has an unexpected type");
    Function *H = chunkHelper(M, st.op, leaf, st.CB->getFunction());
    // A leaf's result is its destination.
    if (!st.CB->getType()->isVoidTy())
      st.CB->replaceAllUsesWith(d);
    Instruction *ThenT, *ElseT;
    IRBuilder<> B(st.CB);
    SplitBlockAndInsertIfThenElse(B.CreateICmpUGT(len, B.getInt64(LeafChunk)), st.CB, &ThenT, &ElseT,
                                  MDB.createUnlikelyBranchWeights());
    IRBuilder<> TB(ThenT);
    if (st.op == "memset")
      src = src->getType()->isIntegerTy(32) ? src : TB.CreateZExt(src, I32);
    TB.CreateCall(H, {d, src, len});
    st.CB->moveBefore(ElseT->getIterator());
    n++;
  }
  errs() << "POLL-LEAF calls " << n << "\n";
}

// Retreating edges of a depth-first walk from the entry, once each.
static std::vector<std::pair<BasicBlock *, BasicBlock *>> retreatingEdges(Function &F) {
  std::vector<std::pair<BasicBlock *, BasicBlock *>> edges;
  DenseMap<BasicBlock *, int> state; // 1 on the walk's path, 2 done
  std::vector<std::pair<BasicBlock *, succ_iterator>> st;
  std::set<std::pair<BasicBlock *, BasicBlock *>> seen;
  BasicBlock *E = &F.getEntryBlock();
  st.push_back({E, succ_begin(E)});
  state[E] = 1;
  while (!st.empty()) {
    auto &[B, it] = st.back();
    if (it == succ_end(B)) {
      state[B] = 2;
      st.pop_back();
      continue;
    }
    BasicBlock *S = *it;
    ++it;
    int x = state.lookup(S);
    if (x == 1) {
      if (seen.insert({B, S}).second)
        edges.push_back({B, S});
    } else if (x == 0) {
      state[S] = 1;
      st.push_back({S, succ_begin(S)});
    }
  }
  return edges;
}

static bool isPollCall(const Instruction &I) {
  auto *CB = dyn_cast<CallBase>(&I);
  return CB && CB->getCalledFunction() && CB->getCalledFunction()->getName() == "host_poll";
}

// mayTick tells whether a call can tick or poll before it returns, so that a
// loop keeping the count in a register must store it before the call and
// load it after: a call through a pointer, a function with a cycle or a mark
// or one that calls such a function, host_poll and host_call (which may
// enter the engine again). The other host callbacks and the register-entry
// leaves run no engine code.
static std::set<Function *> tickers(Module &M, const std::set<std::string> &leaves) {
  std::set<Function *> t;
  std::map<Function *, std::vector<Function *>> callees;
  std::vector<Function *> fs;
  for (Function &F : M) {
    if (F.isDeclaration()) {
      if (F.getName() == "host_poll" || F.getName() == "host_call")
        t.insert(&F);
      continue;
    }
    if (leaves.count(F.getName().str()))
      continue;
    fs.push_back(&F);
    bool tick = !retreatingEdges(F).empty();
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I)) {
          if (pollMark(I))
            tick = true;
          if (CB->isInlineAsm())
            continue;
          Function *G = CB->getCalledFunction();
          if (!G)
            tick = true;
          else if (!G->isIntrinsic())
            callees[&F].push_back(G);
        }
    if (tick)
      t.insert(&F);
  }
  for (bool changed = true; changed;) {
    changed = false;
    for (Function *F : fs)
      if (!t.count(F))
        for (Function *G : callees[F])
          if (t.count(G)) {
            t.insert(F);
            changed = true;
            break;
          }
  }
  return t;
}

// A loop runs a bounded stretch of code between ticks when it is innermost,
// calls nothing (callFree), and scalar evolution proves it takes its back
// edges at most MaxRun - 1 times per entry, with its runs times its
// instructions within RunBudget. Its back edges need no tick: whatever
// enters it again ticks. A tick then stands for at most RunBudget
// instructions of such a loop, so that the interval of the host bounds the
// time between two polls: 10000 ticks are a few milliseconds at most.
static const uint64_t MaxRun = 256, RunBudget = 256;

using Edge = std::pair<BasicBlock *, BasicBlock *>;

static uint64_t loopSize(const Loop *L) {
  uint64_t n = 0;
  for (BasicBlock *B : L->blocks())
    for (Instruction &I : *B)
      n += !isa<PHINode>(&I) && !I.isDebugOrPseudoInst();
  return n;
}

// callFree: no call in the loop but intrinsics llc expands in place, and a
// memory intrinsic only of a small constant length, and no mark. Its
// branches are plain ones, so its cycles are the loop's.
static bool callFree(const Loop *L) {
  for (BasicBlock *B : L->blocks()) {
    const Instruction *T = B->getTerminator();
    if (!isa<BranchInst>(T) && !isa<SwitchInst>(T) && !isa<ReturnInst>(T) && !isa<UnreachableInst>(T))
      return false;
    for (Instruction &I : *B) {
      auto *CB = dyn_cast<CallBase>(&I);
      if (!CB)
        continue;
      if (!isa<IntrinsicInst>(CB) || pollMark(*CB))
        return false;
      if (auto *MI = dyn_cast<MemIntrinsic>(CB)) {
        auto *N = dyn_cast<ConstantInt>(MI->getLength());
        if (!N || N->getZExtValue() > 256)
          return false;
      }
    }
  }
  return true;
}

// The latch-to-header edges of the loops of F that scalar evolution bounds.
static std::set<Edge> boundedEdges(Function &F, const Triple &T) {
  TargetLibraryInfoImpl TLII(T);
  TargetLibraryInfo TLI(TLII, &F);
  AssumptionCache AC(F);
  DominatorTree DT(F);
  LoopInfo LI(DT);
  ScalarEvolution SE(F, TLI, AC, DT, LI);
  std::set<Edge> edges;
  for (Loop *L : LI.getLoopsInPreorder()) {
    if (!L->isInnermost() || !callFree(L))
      continue;
    auto *BTC = dyn_cast<SCEVConstant>(SE.getConstantMaxBackedgeTakenCount(L));
    if (!BTC || BTC->getAPInt().getActiveBits() > 32)
      continue;
    uint64_t runs = BTC->getAPInt().getZExtValue() + 1;
    if (runs > MaxRun || runs * std::max<uint64_t>(1, loopSize(L)) > RunBudget)
      continue;
    SmallVector<BasicBlock *> latches;
    L->getLoopLatches(latches);
    for (BasicBlock *Lt : latches)
      edges.insert({Lt, L->getHeader()});
  }
  return edges;
}

// stripLoops splits each innermost loop that calls nothing (callFree), and
// that scalar evolution does not bound already, into chunks: the loop
// runs at most a chunk of iterations and leaves to a new block, which
// counts the whole chunk with one tick and enters it again from a new outer
// header. Scalar evolution then bounds the loop: it has no tick and keeps
// the poll out of it, so its registers stay as they were. Returns the new
// blocks with the weight of their tick.
static std::map<BasicBlock *, uint64_t> stripLoops(Function &F, unsigned &n) {
  std::map<BasicBlock *, uint64_t> chunks;
  DominatorTree DT(F);
  LoopInfo LI(DT);
  std::set<Edge> bounded = boundedEdges(F, Triple(F.getParent()->getTargetTriple()));
  struct Cand {
    Loop *L;
    uint64_t chunk;
  };
  std::vector<Cand> cands;
  for (Loop *L : LI.getLoopsInPreorder()) {
    if (!L->isInnermost() || !callFree(L) || !L->getLoopLatch() ||
        !isa<BranchInst>(L->getLoopLatch()->getTerminator()))
      continue;
    if (bounded.count({L->getLoopLatch(), L->getHeader()}))
      continue;
    // The chunk keeps a chunk's run, with the counter the split adds, in
    // the budget the check holds bounded loops to.
    uint64_t chunk = std::min<uint64_t>(MaxRun, RunBudget / (loopSize(L) + 4));
    if (chunk < 16)
      continue;
    if (!L->getLoopPreheader() && !InsertPreheaderForLoop(L, &DT, &LI, nullptr, false))
      continue;
    cands.push_back({L, chunk});
  }
  LLVMContext &C = F.getContext();
  Type *I64 = Type::getInt64Ty(C);
  MDBuilder MDB(C);
  for (auto [L, chunk] : cands) {
    BasicBlock *H = L->getHeader(), *Ph = L->getLoopPreheader(), *Lt = L->getLoopLatch();
    // Ph -> O -> H, and Lt -> Y -> H, where Y counts the chunk down and
    // leaves for X at its end, and X goes back to O.
    BasicBlock *O = SplitEdge(Ph, H);
    BasicBlock *Y = SplitEdge(Lt, H);
    O->setName("poll.chunk.outer");
    Y->setName("poll.chunk.latch");
    BasicBlock *X = BasicBlock::Create(C, "poll.chunk.end", &F);
    BranchInst::Create(O, X);
    for (PHINode &Phi : H->phis()) {
      PHINode *Po = PHINode::Create(Phi.getType(), 2, Phi.getName() + ".outer", O->begin());
      Po->addIncoming(Phi.getIncomingValueForBlock(O), Ph);
      Po->addIncoming(Phi.getIncomingValueForBlock(Y), X);
      Phi.setIncomingValueForBlock(O, Po);
    }
    PHINode *K = PHINode::Create(I64, 2, "poll.chunk.left", H->begin());
    IRBuilder<> YB(Y->getTerminator());
    Value *K1 = YB.CreateNUWSub(K, YB.getInt64(1));
    BranchInst *Br = YB.CreateCondBr(YB.CreateICmpEQ(K1, YB.getInt64(0)), X, H, MDB.createUnlikelyBranchWeights());
    Y->getTerminator()->eraseFromParent();
    (void)Br;
    K->addIncoming(ConstantInt::get(I64, chunk), O);
    K->addIncoming(K1, Y);
    chunks[X] = chunk;
    n++;
  }
  return chunks;
}

// liveness gives the integer and pointer values live where each block of F
// starts (its phis among them). A pointer that adds a constant to another is
// left out, as llc folds it into the address of its uses.
struct Liveness {
  DenseMap<Value *, unsigned> idx;
  DenseMap<BasicBlock *, BitVector> in;
};
static Liveness liveness(Function &F) {
  Liveness R;
  auto tracked = [](Value *V) {
    Type *T = V->getType();
    if (auto *G = dyn_cast<GetElementPtrInst>(V); G && G->hasAllConstantIndices())
      return false;
    return (T->isIntegerTy() && T->getIntegerBitWidth() <= 64) || T->isPointerTy();
  };
  for (Argument &A : F.args())
    if (tracked(&A))
      R.idx[&A] = R.idx.size();
  for (BasicBlock &B : F)
    for (Instruction &I : B)
      if (tracked(&I))
        R.idx[&I] = R.idx.size();
  unsigned n = R.idx.size();
  struct Sets {
    BitVector use, def, phis, in;
  };
  DenseMap<BasicBlock *, Sets> sets;
  for (BasicBlock &B : F) {
    Sets &S = sets[&B];
    S.use.resize(n), S.def.resize(n), S.phis.resize(n), S.in.resize(n);
    for (Instruction &I : B) {
      if (!isa<PHINode>(&I))
        for (Value *Op : I.operands()) {
          auto it = R.idx.find(Op);
          if (it != R.idx.end() && !S.def.test(it->second))
            S.use.set(it->second);
        }
      auto it = R.idx.find(&I);
      if (it != R.idx.end()) {
        S.def.set(it->second);
        if (isa<PHINode>(&I))
          S.phis.set(it->second);
      }
    }
  }
  std::vector<BasicBlock *> order;
  for (BasicBlock *B : post_order(&F.getEntryBlock()))
    order.push_back(B);
  for (bool changed = true; changed;) {
    changed = false;
    for (BasicBlock *B : order) {
      Sets &S = sets[B];
      BitVector in(n);
      for (BasicBlock *Sx : successors(B)) {
        Sets &T = sets[Sx];
        BitVector x = T.in;
        x.reset(T.phis);
        in |= x;
        for (PHINode &Phi : Sx->phis()) {
          auto it = R.idx.find(Phi.getIncomingValueForBlock(B));
          if (it != R.idx.end())
            in.set(it->second);
        }
      }
      in.reset(S.def);
      in |= S.use;
      in |= S.phis;
      if (in != S.in) {
        S.in = std::move(in);
        changed = true;
      }
    }
  }
  for (auto &[B, S] : sets)
    R.in[B] = std::move(S.in);
  return R;
}

// Where a loop counts. A loop that keeps the count in a local, which mem2reg
// puts in a register, loads it where the loop is entered, stores it back
// where the loop is left and around each call in the loop that may tick, and
// loads it again after such a call. Its ticks are a decrement of a register.
// A loop that counts in memory loads and stores the counter at each tick and
// holds no register for it. chooseLocals weighs the two by how often LLVM
// expects each block to run (BlockFrequencyInfo), from the outermost loops
// in: a loop that keeps a local keeps it for its inner loops too, and the
// inner loops of one that counts in memory are weighed in turn.
//
//   - A loop whose only ticks are the ends of its chunks (stripLoops) counts
//     in memory: it ticks once a chunk, and the load and the stores of a
//     local would cost each entry more than that.
//   - A loop whose hot code already needs about every register counts in
//     memory, as a register for the count would push another value out to
//     the stack in that code. The need is the most values that the loop uses
//     and that are live at once where one of its blocks starts, among the
//     blocks that run at least a quarter as often as its most frequent one.
//     A loop needs at least PollRegs of them to count in memory. The
//     estimate counts more values than llc keeps in registers (constants,
//     addresses it can work out again), so the bound is above the 15
//     registers llc has: in the engine, loops that needed up to 18 lost
//     speed in memory, and the digit loop of js_atod, at 24, lost it in a
//     register.
//   - Otherwise the loop keeps a local when that costs fewer instructions:
//     PollEntry and PollExit for each entry and exit, PollRegTick for each
//     tick and PollSync for each call that may tick, against PollMemTick for
//     each tick in memory.
static constexpr double PollEntry = 3, PollExit = 2, PollSync = 4, PollRegTick = 3, PollMemTick = 5;
static constexpr unsigned PollRegs = 20;

static std::vector<Loop *> chooseLocals(Function &F, LoopInfo &LI, DominatorTree &DT, const std::vector<Edge> &sites,
                                        const std::vector<BasicBlock *> &marks,
                                        const std::map<BasicBlock *, uint64_t> &chunks,
                                        const std::function<bool(Instruction &)> &mayTick) {
  Triple T(F.getParent()->getTargetTriple());
  TargetLibraryInfoImpl TLII(T);
  TargetLibraryInfo TLI(TLII, &F);
  PostDominatorTree PDT(F);
  BranchProbabilityInfo BPI(F, LI, &TLI, &DT, &PDT);
  BlockFrequencyInfo BFI(F, BPI, LI);
  Liveness live = liveness(F);
  auto freq = [&](BasicBlock *B) { return (double)BFI.getBlockFreq(B).getFrequency(); };
  auto edge = [&](BasicBlock *u, BasicBlock *v) {
    BranchProbability p = BPI.getEdgeProbability(u, v);
    return freq(u) * p.getNumerator() / BranchProbability::getDenominator();
  };
  std::vector<Loop *> chosen;
  std::function<void(Loop *)> weigh = [&](Loop *L) {
    double entries = 0, exits = 0, ticks = 0, calls = 0, hottest = 0;
    bool chunkOnly = true;
    unsigned nticks = 0, need = 0;
    for (BasicBlock *P : predecessors(L->getHeader()))
      if (!L->contains(P))
        entries += edge(P, L->getHeader());
    BitVector used(live.idx.size());
    for (BasicBlock *B : L->blocks()) {
      hottest = std::max(hottest, freq(B));
      for (BasicBlock *Sx : successors(B))
        if (!L->contains(Sx))
          exits += edge(B, Sx);
      for (Instruction &I : *B) {
        if (mayTick(I))
          calls += freq(B);
        for (Value *Op : I.operands())
          if (auto it = live.idx.find(Op); it != live.idx.end())
            used.set(it->second);
      }
    }
    for (BasicBlock *B : L->blocks())
      if (freq(B) * 4 >= hottest) {
        BitVector u = live.in.lookup(B);
        u &= used;
        need = std::max(need, (unsigned)u.count());
      }
    for (auto [u, v] : sites)
      if (L->contains(u) && L->contains(v)) {
        ticks += edge(u, v);
        nticks++;
        chunkOnly &= chunks.count(u) > 0;
      }
    for (BasicBlock *B : marks)
      if (L->contains(B)) {
        ticks += freq(B);
        nticks++;
        chunkOnly = false;
      }
    double reg = PollEntry * entries + PollExit * exits + PollRegTick * ticks + PollSync * calls;
    double mem = PollMemTick * ticks;
    bool keep = nticks > 0 && !chunkOnly && need < PollRegs && reg < mem;
    if (PollTrace)
      errs() << "POLL-LOOP " << F.getName() << " " << L->getHeader()->getName() << " depth " << L->getLoopDepth()
             << " blocks " << L->getNumBlocks() << " ticks " << nticks << (chunkOnly ? " chunks" : "") << " need "
             << need << " freq entry " << entries << " exit " << exits << " tick " << ticks << " call " << calls
             << " cost reg " << reg << " mem " << mem << (keep ? " -> register" : " -> memory") << "\n";
    if (keep) {
      chosen.push_back(L);
      return;
    }
    for (Loop *Sub : *L)
      weigh(Sub);
  };
  for (Loop *L : LI)
    weigh(L);
  return chosen;
}

void insertPolls(Module &M, const std::set<std::string> &leaves, unsigned sizeLog2) {
  if (!PollOn)
    return;
  LLVMContext &C = M.getContext();
  Type *I64 = Type::getInt64Ty(C);
  PointerType *P = PointerType::getUnqual(C);
  const uint64_t S = 1ull << sizeLog2;
  FunctionCallee poll = M.getOrInsertFunction("host_poll", FunctionType::get(Type::getVoidTy(C), {P}, false));
  if (cast<Function>(poll.getCallee())->getFunctionType() != poll.getFunctionType())
    reportFatalUsageError("poll: host_poll is declared with another type");
  MDBuilder MDB(C);
  MDNode *tickMD = MDNode::get(C, {});
  std::set<Function *> ticks = tickers(M, leaves);
  unsigned nticks = 0, nreg = 0, nloops = 0, ncounted = 0, nmarks = 0, nsync = 0, nstrip = 0, nbounded = 0;
  for (Function &F : M) {
    if (F.isDeclaration() || leaves.count(F.getName().str()))
      continue;
    // The marks of F, and its dispatch when it counts its bytecode.
    std::vector<Instruction *> marks;
    BasicBlock *dispatch = nullptr;
    for (BasicBlock &B : F) {
      for (Instruction &I : B)
        if (pollMark(I))
          marks.push_back(&I);
      if (B.getTerminator()->getMetadata(CountedMD))
        dispatch = &B;
    }
    if (retreatingEdges(F).empty() && marks.empty())
      continue;
    if (F.arg_size() == 0 || !F.getArg(0)->getType()->isPointerTy())
      reportFatalUsageError(Twine("poll: function without a shadow pointer: ") + F.getName());
    unsigned fticks = nticks, freg = nreg, floops = nloops, fsync = nsync, fstrip = nstrip;
    ncounted += dispatch != nullptr;
    Argument *sp = F.getArg(0);
    IRBuilder<> E(&*F.getEntryBlock().getFirstInsertionPt());
    auto counterPtr = [&](IRBuilder<> &B) -> Value * {
      Value *base = B.CreateAnd(B.CreatePtrToInt(sp, I64), B.getInt64(~(S - 1)));
      return B.CreateIntToPtr(B.CreateAdd(base, B.getInt64(PollOffset)), P, "poll.counter");
    };
    std::map<BasicBlock *, uint64_t> chunks = stripLoops(F, nstrip);
    std::set<Edge> exempt = boundedEdges(F, Triple(M.getTargetTriple()));
    nbounded += exempt.size();
    DominatorTree DT(F);
    LoopInfo LI(DT);
    // The loops that keep the count in a local, which mem2reg puts in a
    // register.
    std::map<Loop *, AllocaInst *> local;
    SmallVector<AllocaInst *> allocas;
    // A call that may tick: through a pointer, or to a function that ticks.
    std::function<bool(Instruction &)> mayTick = [&](Instruction &I) {
      auto *CB = dyn_cast<CallBase>(&I);
      if (!CB || CB->isInlineAsm() || isPollCall(I))
        return false;
      Function *G = CB->getCalledFunction();
      return !G || ticks.count(G) > 0;
    };
    // A call that may tick sees the count, and the code after it sees what
    // the call counted.
    auto syncCalls = [&](ArrayRef<BasicBlock *> blocks, AllocaInst *A) {
      SmallVector<CallBase *> cs;
      for (BasicBlock *Bk : blocks)
        for (Instruction &I : *Bk)
          if (mayTick(I))
            cs.push_back(cast<CallBase>(&I));
      for (CallBase *CB : cs) {
        if (!isa<CallInst>(CB))
          reportFatalUsageError(Twine("poll: a call that is not a plain call in ") + F.getName());
        IRBuilder<> BB(CB);
        if (PollFault != "callstore")
          BB.CreateStore(BB.CreateLoad(I64, A), counterPtr(BB));
        IRBuilder<> AB(CB->getNextNode());
        if (PollFault != "callload")
          AB.CreateStore(AB.CreateLoad(I64, counterPtr(AB)), A);
        nsync++;
      }
    };
    // The edges that tick: every retreating edge but those into a dispatch
    // that counts its bytecode and the back edges of the loops scalar
    // evolution bounds.
    auto tickEdges = [&]() {
      std::vector<Edge> es;
      for (auto [u, v] : retreatingEdges(F))
        if (!(v == dispatch || exempt.count({u, v})))
          es.push_back({u, v});
      return es;
    };
    std::vector<BasicBlock *> markBlocks;
    for (Instruction *I : marks)
      markBlocks.push_back(I->getParent());
    // Each loop chooseLocals picks keeps the count in a local from where it
    // is entered to where it is left, and stores it back and loads it again
    // around each call in it that may tick.
    for (Loop *L : chooseLocals(F, LI, DT, tickEdges(), markBlocks, chunks, mayTick)) {
      BasicBlock *ph = L->getLoopPreheader();
      if (!ph)
        ph = InsertPreheaderForLoop(L, &DT, &LI, nullptr, false);
      if (!ph)
        continue;
      formDedicatedExitBlocks(L, &DT, &LI, nullptr, false);
      if (!L->hasDedicatedExits())
        continue;
      AllocaInst *A = E.CreateAlloca(I64, nullptr, "poll.local");
      allocas.push_back(A);
      IRBuilder<> PB(ph->getTerminator());
      PB.CreateStore(PB.CreateLoad(I64, counterPtr(PB)), A);
      SmallVector<BasicBlock *> exits;
      L->getUniqueExitBlocks(exits);
      if (PollFault != "exit")
        for (BasicBlock *X : exits) {
          IRBuilder<> XB(&*X->getFirstInsertionPt());
          XB.CreateStore(XB.CreateLoad(I64, A), counterPtr(XB));
        }
      syncCalls(SmallVector<BasicBlock *>(L->blocks()), A);
      local[L] = A;
      nloops++;
    }
    // The local of an edge: that of the loop with one that holds both ends.
    auto localOf = [&](BasicBlock *u, BasicBlock *v) -> AllocaInst * {
      for (Loop *L = LI.getLoopFor(u); L; L = L->getParentLoop()) {
        auto it = local.find(L);
        if (it != local.end())
          return L->contains(v) ? it->second : nullptr;
      }
      return nullptr;
    };
    // The count of a tick: in the local, or in memory.
    auto tick = [&](IRBuilder<> &B, AllocaInst *A, uint64_t weight) -> Value * {
      Value *at = A ? (Value *)A : counterPtr(B);
      Value *c = B.CreateLoad(I64, at, "poll.c");
      auto *c1 = cast<Instruction>(B.CreateSub(c, B.getInt64(weight), "poll.c1"));
      c1->setMetadata(TickMD, tickMD);
      B.CreateStore(c1, at);
      nticks++;
      if (A)
        nreg++;
      return c1;
    };
    // After host_poll the local takes the count the host set.
    auto reload = [&](IRBuilder<> &B, AllocaInst *A) {
      if (A)
        B.CreateStore(B.CreateLoad(I64, counterPtr(B)), A);
    };
    // Where each edge's tick goes: the end of its source when the edge is
    // the only way out of it (or the way out is an indirect branch), else a
    // block of its own on the edge. All are placed before any is inserted.
    std::map<Instruction *, size_t> placed;
    std::vector<std::tuple<Instruction *, AllocaInst *, uint64_t>> at;
    auto placeTick = [&](Instruction *t, AllocaInst *A, uint64_t weight) {
      auto [it, fresh] = placed.insert({t, at.size()});
      if (fresh)
        at.push_back({t, A, weight});
      else
        std::get<2>(at[it->second]) = std::max(std::get<2>(at[it->second]), weight);
    };

    // A mark ticks where it is. -shadow-poll-fault=entry takes out the
    // entry mark, and mark leaves the others without a tick.
    for (Instruction *I : marks) {
      uint64_t weight;
      bool entry;
      pollMark(*I, &weight, &entry);
      nmarks++;
      if (entry && PollFault == "entry")
        I->eraseFromParent();
      else if (entry || PollFault != "mark")
        placeTick(I, localOf(I->getParent(), I->getParent()), weight);
    }
    for (auto [u, v] : tickEdges()) {
      auto ch = chunks.find(u);
      uint64_t weight = ch == chunks.end() ? 1 : ch->second;
      if (MDNode *W = u->getTerminator()->getMetadata(ChunkWeightMD))
        weight = mdconst::extract<ConstantInt>(W->getOperand(0))->getZExtValue();
      AllocaInst *A = localOf(u, v);
      Instruction *t = u->getTerminator();
      if (!(u->getSingleSuccessor() == v || isa<IndirectBrInst>(t) || isa<CallBrInst>(t)))
        t = SplitEdge(u, v, &DT, &LI)->getTerminator();
      placeTick(t, A, weight);
    }
    for (auto [t, A, weight] : at) {
      IRBuilder<> B(t);
      Value *c1 = tick(B, A, weight);
      // -shadow-poll-fault=cond: c1 < INT64_MIN, which never holds.
      Value *expired = B.CreateICmpSLT(c1, B.getInt64(PollFault == "cond" ? 1ull << 63 : 1));
      Instruction *T = SplitBlockAndInsertIfThen(expired, t, false, MDB.createUnlikelyBranchWeights());
      IRBuilder<> TB(T);
      TB.CreateCall(poll, {sp});
      reload(TB, A);
    }
    if (!allocas.empty()) {
      DominatorTree DT2(F);
      PromoteMemToReg(allocas, DT2);
    }
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (isa<AllocaInst>(&I))
          reportFatalUsageError(Twine("poll: a counter left in an alloca in ") + F.getName());
    if (PollStats)
      errs() << "POLLFN " << F.getName() << " ticks " << nticks - fticks << " inreg " << nreg - freg << " localloops "
             << nloops - floops << " synced " << nsync - fsync << " marks " << marks.size() << " counted "
             << (dispatch ? 1 : 0) << " chunked " << nstrip - fstrip << " bounded " << exempt.size() << "\n";
  }
  errs() << "POLL ticks " << nticks << " in registers " << nreg << " loops in registers " << nloops
         << " synced calls " << nsync << " marks " << nmarks << " counted dispatches " << ncounted
         << " chunked loops " << nstrip << " bounded edges " << nbounded << "\n";
}

// checkCounts holds F to the rule that keeps the count whole while code holds
// it in a register: each tick and each store to the word take the count as it
// stands, and each call that may tick (or poll) and each return finds the
// count in memory. It
// follows, through F, which values hold the count as it stands: the word in
// memory, a load of the word while that holds, a tick of such a value, a phi
// all of whose incoming values hold it on their edges. A tick leaves only its
// result holding the count, until a store of it puts it in memory. A call
// that may tick leaves only the word holding it (the callee keeps to the same
// rule), and so does host_poll, which sets the word. A tick or a store of
// anything else, or a call or a return while the word is behind, fails: a
// count kept in a register that a callee does not see, or that is stored over
// what a callee counted, would let the ticks between two polls run past the
// interval.
static void checkCounts(Function &F, const std::set<Function *> &ticks, uint64_t mask) {
  Argument *sp = F.getArg(0);
  // The word: inttoptr((ptrtoint(sp) & mask) + offset), as counterPtr writes it.
  auto isCounter = [&](Value *P) {
    auto *I2P = dyn_cast<IntToPtrInst>(P);
    auto *Add = I2P ? dyn_cast<BinaryOperator>(I2P->getOperand(0)) : nullptr;
    if (!Add || Add->getOpcode() != Instruction::Add)
      return false;
    auto *Off = dyn_cast<ConstantInt>(Add->getOperand(1));
    auto *And = dyn_cast<BinaryOperator>(Add->getOperand(0));
    if (!Off || Off->getZExtValue() != PollOffset || !And || And->getOpcode() != Instruction::And)
      return false;
    auto *Mask = dyn_cast<ConstantInt>(And->getOperand(1));
    auto *P2I = dyn_cast<PtrToIntInst>(And->getOperand(0));
    return Mask && Mask->getZExtValue() == mask && P2I && P2I->getOperand(0) == sp;
  };
  bool any = false;
  for (BasicBlock &B : F)
    for (Instruction &I : B)
      if (I.getMetadata(TickMD) || (isa<LoadInst>(&I) && isCounter(cast<LoadInst>(&I)->getPointerOperand())) ||
          (isa<StoreInst>(&I) && isCounter(cast<StoreInst>(&I)->getPointerOperand())))
        any = true;
  if (!any)
    return;
  auto mayTick = [&](Instruction &I) {
    auto *CB = dyn_cast<CallBase>(&I);
    if (!CB || CB->isInlineAsm() || isPollCall(I))
      return false;
    Function *G = CB->getCalledFunction();
    return !G || ticks.count(G) > 0;
  };
  // What holds the count: the word (mem), and the values in cur. A block not
  // reached yet (top) takes whatever its other predecessors give.
  struct State {
    bool top = true, mem = false;
    SmallVector<Value *, 4> cur;
    bool holds(Value *V) const { return top || is_contained(cur, V); }
    bool operator==(const State &o) const { return top == o.top && mem == o.mem && cur == o.cur; }
  };
  auto fail = [&](const Twine &what, Instruction &I) {
    reportFatalUsageError(Twine("poll check: ") + what + " in " + F.getName() + " (block " +
                          I.getParent()->getName() + ")");
  };
  // Every block has its state from the start, so that no lookup adds one
  // while a reference to another is held.
  DenseMap<BasicBlock *, State> out;
  for (BasicBlock &B : F)
    out[&B];
  ReversePostOrderTraversal<Function *> RPO(&F);
  for (bool changed = true; changed;) {
    changed = false;
    for (BasicBlock *B : RPO) {
      State s;
      if (B->isEntryBlock()) {
        s.top = false, s.mem = true;
      } else {
        for (BasicBlock *Pd : predecessors(B)) {
          const State &o = out[Pd];
          if (o.top)
            continue;
          if (s.top) {
            s = o;
            s.cur.clear();
            for (Value *V : o.cur)
              if (all_of(predecessors(B), [&](BasicBlock *Q) { return out[Q].holds(V); }))
                s.cur.push_back(V);
          }
          s.mem &= o.mem;
        }
        for (PHINode &Phi : B->phis())
          if (!s.top && all_of(predecessors(B), [&](BasicBlock *Q) {
                return out[Q].holds(Phi.getIncomingValueForBlock(Q));
              }))
            s.cur.push_back(&Phi);
      }
      if (!s.top)
        for (Instruction &I : *B) {
          if (auto *L = dyn_cast<LoadInst>(&I)) {
            if (isCounter(L->getPointerOperand()) && s.mem)
              s.cur.push_back(L);
          } else if (auto *St = dyn_cast<StoreInst>(&I)) {
            if (isCounter(St->getPointerOperand())) {
              if (!is_contained(s.cur, St->getValueOperand()))
                fail("a store of a value that is not the count as it stands", I);
              s.mem = true;
            }
          } else if (I.getMetadata(TickMD)) {
            if (!is_contained(s.cur, I.getOperand(0)))
              fail("a tick of a value that is not the count as it stands", I);
            s.cur.assign({&I});
            s.mem = false;
          } else if (isPollCall(I)) {
            s.cur.clear();
            s.mem = true;
          } else if (mayTick(I)) {
            if (!s.mem)
              fail("a call that may tick while the count is not in memory", I);
            s.cur.clear();
          } else if (isa<ReturnInst>(&I) && !s.mem) {
            fail("a return while the count is not in memory", I);
          }
        }
      State &o = out[B];
      if (!(o == s)) {
        o = std::move(s);
        changed = true;
      }
    }
  }
}

// expiredTest tells whether Cond holds whenever the count Tick leaves has run
// out (is 0 or less): Tick < K with K at least 1, or Tick <= K with K at least
// 0.
static bool expiredTest(Value *Cond, Instruction *Tick) {
  auto *Cmp = dyn_cast<ICmpInst>(Cond);
  auto *K = Cmp ? dyn_cast<ConstantInt>(Cmp->getOperand(1)) : nullptr;
  if (!K || Cmp->getOperand(0) != Tick || K->getBitWidth() > 64)
    return false;
  if (Cmp->getPredicate() == ICmpInst::ICMP_SLT)
    return K->getSExtValue() >= 1;
  if (Cmp->getPredicate() == ICmpInst::ICMP_SLE)
    return K->getSExtValue() >= 0;
  return false;
}

// tickBefore tells whether a mark at the start of its block B comes right
// after a tick of at least weight: one predecessor of B ticks and branches
// to B or to a block of its own that polls and goes on to B, and B has no
// other predecessor.
static bool tickBefore(Instruction *Mark, uint64_t weight) {
  BasicBlock *B = Mark->getParent();
  if (&B->front() != Mark)
    return false;
  BasicBlock *H = nullptr;
  for (BasicBlock *P : predecessors(B)) {
    auto *Br = dyn_cast<BranchInst>(P->getTerminator());
    if (!Br || !Br->isConditional() || Br->getSuccessor(1) != B)
      continue;
    auto *Cmp = dyn_cast<ICmpInst>(Br->getCondition());
    auto *Tick = Cmp ? dyn_cast<BinaryOperator>(Cmp->getOperand(0)) : nullptr;
    auto *W = Tick && Tick->getMetadata(TickMD) ? dyn_cast<ConstantInt>(Tick->getOperand(1)) : nullptr;
    if (W && Tick->getParent() == P && expiredTest(Cmp, Tick) && W->getZExtValue() >= weight)
      H = P;
  }
  if (!H)
    return false;
  for (BasicBlock *P : predecessors(B))
    if (P != H && !(P->getSinglePredecessor() == H && any_of(*P, [](Instruction &I) { return isPollCall(I); })))
      return false;
  return true;
}

// checkPolls holds every function to the rule, apart from how the ticks were
// placed: with the blocks that tick (a decrement tagged goasm.tick) or call
// host_poll taken out, and the back edges of the loops scalar evolution
// bounds (boundedEdges, worked out again here), the control flow graph has
// no cycle left. In a function whose dispatch counts its bytecode
// (CountedMD), the edges into the dispatch are taken out too: the entry block
// must tick, and each mark must come right after a tick of its weight
// (tickBefore). A tick must take at least 1 from the count, and its block's
// way out must poll whenever the count it leaves has run out: a conditional
// branch on a test of the tick that holds then (expiredTest), whose taken
// side calls host_poll. Each function is held to checkCounts first.
void checkPolls(Module &M, const std::set<std::string> &leaves, unsigned sizeLog2) {
  if (!PollOn)
    return;
  std::set<Function *> ticks = tickers(M, leaves);
  for (Function &F : M) {
    if (F.isDeclaration() || leaves.count(F.getName().str()))
      continue;
    checkCounts(F, ticks, ~((1ull << sizeLog2) - 1));
    std::set<Edge> bounded = boundedEdges(F, Triple(M.getTargetTriple()));
    std::set<BasicBlock *> cut, counted;
    bool entryTicks = false;
    for (BasicBlock &B : F)
      for (Instruction &I : B) {
        uint64_t weight;
        if (pollMark(I, &weight) && !tickBefore(&I, weight))
          reportFatalUsageError(Twine("poll check: a mark without a tick of its weight before it in ") + F.getName());
        if (isPollCall(I))
          cut.insert(&B);
        if (I.isTerminator() && I.getMetadata(CountedMD))
          counted.insert(&B);
        if (!I.getMetadata(TickMD))
          continue;
        entryTicks |= B.isEntryBlock();
        cut.insert(&B);
        auto *Sub = dyn_cast<BinaryOperator>(&I);
        auto *W = Sub && Sub->getOpcode() == Instruction::Sub ? dyn_cast<ConstantInt>(Sub->getOperand(1)) : nullptr;
        if (!W || W->getBitWidth() > 64 || W->getSExtValue() < 1)
          reportFatalUsageError(Twine("poll check: a tick that does not take at least 1 from the count in ") +
                                F.getName());
        Instruction *T = B.getTerminator();
        bool ok = false;
        if (auto *Br = dyn_cast<BranchInst>(T))
          if (Br->isConditional() && expiredTest(Br->getCondition(), &I))
            for (Instruction &J : *Br->getSuccessor(0))
              ok |= isPollCall(J);
        if (!ok)
          reportFatalUsageError(Twine("poll check: a tick whose branch does not poll when the count runs out in ") +
                                F.getName());
      }
    if (!counted.empty() && !entryTicks)
      reportFatalUsageError(Twine("poll check: a dispatch that counts its bytecode in a function that does not tick "
                                  "at its entry: ") +
                            F.getName());
    // The edges of the cycle search: not those of the bounded loops, nor
    // those into a dispatch that counts its bytecode.
    auto follows = [&](BasicBlock *u, BasicBlock *v) { return !bounded.count({u, v}) && !counted.count(v); };
    for (scc_iterator<Function *> I = scc_begin(&F); !I.isAtEnd(); ++I) {
      if (!I.hasCycle())
        continue;
      // A cycle of the SCC that avoids the cut blocks: search the SCC with
      // them taken out.
      std::set<BasicBlock *> in;
      for (BasicBlock *B : *I)
        if (!cut.count(B))
          in.insert(B);
      // Kahn's algorithm on the SCC minus the cut: whatever is left has a
      // cycle.
      std::map<BasicBlock *, int> indeg;
      for (BasicBlock *B : in)
        indeg[B];
      for (BasicBlock *B : in)
        for (BasicBlock *Sx : successors(B))
          if (in.count(Sx) && follows(B, Sx))
            indeg[Sx]++;
      std::vector<BasicBlock *> q;
      for (auto &[B, d] : indeg)
        if (d == 0)
          q.push_back(B);
      size_t done = 0;
      while (!q.empty()) {
        BasicBlock *B = q.back();
        q.pop_back();
        done++;
        for (BasicBlock *Sx : successors(B))
          if (in.count(Sx) && follows(B, Sx) && --indeg[Sx] == 0)
            q.push_back(Sx);
      }
      if (done != in.size()) {
        std::string names;
        for (auto &[B, d] : indeg)
          if (d > 0 && names.size() < 200)
            names += " " + B->getName().str();
        reportFatalUsageError(Twine("poll check: a cycle without a tick or a poll in ") + F.getName() + ":" + names);
      }
    }
  }
}
