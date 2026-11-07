// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

// goasm_room, generated in engine_room_windows_amd64.s, does nothing, but
// its stack check makes gen.RoomBelow bytes of room below the stack of its
// caller, as the room check of every generated function does on windows:
// there the system writes the state of the CPU at a fault below the stack
// pointer, and Go keeps only 4 KiB for it (docs/changes/c-engine.md, When the
// engine fails).
func goasm_room()

// makeRoom makes the room again before a callback returns into the engine:
// the stack may have shrunk while the callback ran, and the generated code
// checks the room only where a function starts.
func makeRoom() { goasm_room() }
