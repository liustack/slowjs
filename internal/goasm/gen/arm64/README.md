# arm64 backend (phase-0 prototype)

The arm64 side of the new C-engine generator. `conv` turns LLVM aarch64 output
into Go arm64 assembly following the plan's frame and register rules. It is the
converter the arm64 prototype proved on real hardware, brought into the
generator tree.

## Rules it enforces

- Plain AAPCS inside C. x18, x26, x27, x28 are fixed by clang flags; x29 is the
  reserved frame pointer; x30 (LR) is reserved for the return address and may
  appear only in loads and stores with an sp base. conv rejects any use of a
  reserved register and any bare LR.
- Go's frame: 0(RSP) is the saved LR, 8(RSP) is padding, the LLVM frame of N
  bytes sits at 16(RSP), and the TEXT declares `$(N+8)`. conv shifts every
  `[sp, #k]` to `[sp, #k+16]` and rejects any other stack use.
- Every `ret` becomes a branch to one Go `RET`. svc, mrs, msr, blr, br, adr and
  adrp are rejected, as are tail jumps and relocations other than
  `R_AARCH64_CALL26`.
- A leaf (no stack, no call) gets a register entry (`TEXT name, NOSPLIT|NOFRAME`)
  and a unified entry (`c_name`). A non-leaf and every Go callback are reached
  through a NOSPLIT stub (`call_name`) that saves and restores R19-R25 and
  F8-F15, because Go and morestack clobber them.
- Shrink-wrapping is off so the prologue is always at the entry.

## Running

`build.sh` (from this dir) needs LLVM 22.1.8 whose clang and llvm-mc can target
aarch64 (native, or an x86 host with the aarch64 target built in, via
`--target=aarch64-linux-gnu`). It compiles `testdata/engine.c`, lowers it,
assembles it with llvm-mc, and emits the runnable package under
`internal/mini`, which is plain Go plus arm64 assembly: it builds and runs on
any arm64 Go, darwin or linux, with no toolchain. `CONV_NO_SAVES=1` builds the
stub without the R19-R25/F8-F15 saves, for timing that cost in isolation (the
benchmark checks the result is still correct first).

`testdata/*.go.txt` and `*.s.txt` are the behavior tests, the host callbacks,
the Go reference and the call-overhead benchmark, kept as text so this dir does
not compile them. The driver copies them next to the generated files.
