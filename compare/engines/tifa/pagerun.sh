#!/bin/bash
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
# pagerun.sh <out stem> <command...>: runs one engine process on CPU 1 under
# a 4 GiB cap and writes its stdout to <stem>.json. Like cmd/compare it
# repeats a run (up to 3 times) when the sibling CPU 5 was busy for at least
# 3 ticks and more than 5% of the run, or when the process got less than 90%
# of the wall time as CPU. Each try is a line in <stem>.meta.
stem=$1; shift
ticks() { awk '$1=="cpu5"{print $2+$3+$4+$7+$8+$9}' /proc/stat; }
TIMEFORMAT='%R %U %S'
: > "$stem.meta"
for try in 0 1 2; do
  t0=$(ticks)
  { time systemd-run --user --scope -q -p MemoryMax=4G -p MemorySwapMax=0 taskset -c 1 "$@" > "$stem.json" 2> "$stem.err"; } 2> "$stem.time"
  t1=$(ticks)
  read -r wall cpu bad < <(awk -v t=$((t1-t0)) '{w=$1; c=$2+$3; bad=((t>=3 && t>0.05*w*100) || (w-c>=0.05 && c<0.9*w)); print w, c, bad}' "$stem.time")
  echo "{\"try\":$try,\"sibling_ticks\":$((t1-t0)),\"wall_s\":$wall,\"cpu_s\":$cpu,\"disturbed\":$bad}" >> "$stem.meta"
  [ "$bad" = 0 ] && break
  [ "$try" = 2 ] || sleep 10
done
