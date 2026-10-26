/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* abort is a request to the host. The host does not give control back: it
   ends the process, or in Go it panics and the VM can only be closed. The trap
   after it is for a host that does. */
#include <stdlib.h>

#include "host.h"

void abort(void)
{
	host_call(0, NULL, 0, NULL, HOST_CALL_ABORT);
	__builtin_trap();
}
