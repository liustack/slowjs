---
summary: 'How to measure whether a change makes renders faster or slower, down to a fraction of a percent: the protocol (layouts, balanced blocks, reruns, paired log ratios, the verdict against a stated delta, callgrind next to it), its defaults and why, the false positive rate and the smallest effect it was shown to see, how to read a result and when not to trust one'
read_when:
  - Timing a change to the engine or to tifa against the code before it
  - Deciding whether a change of less than a few percent is real
  - Reading the output of sjdev bench judge, or a result someone else measured
  - Changing sjdev bench, the layouts it builds or the defaults of the protocol
  - Using callgrind to explain a timing
---

# Measuring a change

A change for speed is judged by timing tifa's pages, the same way every time. This page is the protocol, its defaults, what it was shown to detect and what it cannot. The tool is `go tool sjdev bench` (commands in [testing.md](testing.md#time-one-build-against-another)).

Results hold for the machine they were measured on. A result on linux/amd64 says nothing about darwin/arm64, and the layouts below exist only on amd64.

## The protocol

1. **Fix the question first.** Write down the machine, the pages, the metric and the regression that matters, delta. The default is delta = 1% on the sum of the P50 of every page of the app, on the GCP machine (AMD EPYC 7B13, Zen 3). Timing decides. Instruction counts explain.
2. **Build every side at the 16 protocol layouts** (`bench build`, `-positions protocol`, the default). A layout moves the code without changing it:
   - front: 0, 8, ..., 56 bytes of NOP at the start of the interpreter body, so the body takes each 8-byte place in its 64-byte block,
   - shift: bytes of NOP after the first engine function the binary keeps, so every engine function moves to other blocks and another place in its 4 KiB page.

   The set is the 8 fronts at shift 0, and the 8 fronts again at 8 shifts spread over a page (32, 576, 1120, ..., 3840), paired so that front and shift do not grow together. Layout 0.0 is the formal binary, the one that ships. `bench build` writes where the interpreter body and 10 hot handlers landed in each binary to `layouts.tsv`, and fails when a shift did not move the interpreter.
3. **Time in balanced blocks** (`bench run`). A block is every side at one layout, one after the other. Blocks come in a random order, the formal layout 3 times per round. Within a block the sides run in a random order in an even round and in the reverse order in the next round, so any two sides meet as often in each order. The default is 2 rounds: 2 pairs at each layout and 6 at the formal one. Before each process the tool drops the cached pages of its binary (`-fresh-pages`, on by default on linux), so the process maps the binary on new physical pages. Without it, a binary keeps the physical pages it got when it was first read for as long as it stays cached, and two copies of the same bytes timed up to 2.5% apart at the same layout, every time.
4. **Run a disturbed block again, whole.** A block runs again, up to 3 tries, when any of its processes ran while the sibling hyperthread was over 5% busy, other work took over 5% of the timing core, or the host stole over 1% of it. A block still disturbed after 3 tries is left out of the analysis and counted. Every record keeps its start time, slot, order, attempt, session, steal time and the load of the other cores (`machine_pct`, 100 is one core), since builds on other cores share the L3 cache and are not a reason to run again (see below).
5. **Analyse paired log ratios** (`bench judge`). Each block gives y = ln(new/base) of the metric. The judge reports:
   - the formal layout alone, from its own pairs, with a t interval,
   - the summary: the mean of the layout means, each layout weighted the same, with the t interval of the spread of the layout means (L-1 degrees of freedom). It treats the layouts as a sample of the layouts a change can land on, so the spread between layouts is part of its error. **The summary decides.**
   - the same mean with the run-to-run error only ("fixed layouts only"), which says how well these 16 layouts were measured,
   - the run-to-run error of one pair (pooled within layouts), the spread of the true effect between layouts after taking out the run-to-run error, and the order effect (new first against base first).
6. **Read the verdict against delta.** With the 95% interval [lo, hi] of the change:
   - faster: hi below 0,
   - slower: lo above 0 ("slower, by less than delta" when hi is also below delta),
   - no regression beyond delta: the interval holds 0 and hi is below delta,
   - inconclusive: the interval reaches past delta.

   "No regression beyond delta" is the only verdict that says a change does not cost what matters. An interval that holds 0 and reaches past delta does not show that a change is neutral: it shows the measurement could not tell.
7. **Count with callgrind next to it, never instead** (`bench cg -small 0 -big 3 -dumps -sim`). One process per binary warms every page, then counts only the window the benchmark marks with client requests, 3 renders of every page (see [testing.md](testing.md#time-one-build-against-another)). The table shows the whole program and the interpreter's own code: instructions, branches, mispredicted branches and cache misses, and the raw files are kept. Callgrind's predictor and caches are models of old hardware, not of Zen 3, so a rise in its misses is a reason to look closer, not a verdict. When the counts and the timing disagree, keep measuring. Neither overrules the other.
8. **Confirm a winner on new data.** When several variants were tried and one is kept, time it again in a new session with fresh blocks before quoting it. When one change is judged on 4 apps, a single app with a verdict out of line is weak evidence: with 4 apps and 95% intervals, up to about 1 change in 5 (1 - 0.95^4) can show a false "slower" or "faster" on some app by chance. Use Holm's correction when the question is "is any app slower".

## The defaults and why

| setting | default | why |
| --- | --- | --- |
| layouts | 16 (`-positions protocol`) | The spread between layouts is most of the error, and only more layouts shrink it. 8 fronts cover every 8-byte place of the body in its block, the 8 shifts move every other function to other blocks and page offsets. |
| rounds | 2 (`-rounds 2`) | Two rounds put each layout in both orders. Going to 4 rounds shrinks only the run-to-run part of the error: the smallest change it can show drops from about 0.5% to 0.35% for a change that is the same at every layout, but only from about 1% to 0.9% for a real change to the engine (the table below). For a real change, more layouts pay more than more rounds. |
| formal blocks per round | 3 (`-formal-reps 3`) | The formal layout gets its own interval, which needs more than 2 pairs. |
| renders per page | 8 warm-up, then 20 timed (`TIFA_WARM=8`, `-test.benchtime 20x`), the P50 | The sum of the P50s of one process differs from the next by about 1% (the run SD below), small next to the 4% to 5% between layouts. |
| delta | 1% (`-delta 0.01`) | About the smallest real change to the engine that 2 rounds can show (0.9% to 1.1% on bench, mui and the storefront, 1.2% on the shop). Against 0.5%, 7 of 15 A/A runs came out inconclusive. Against 1%, all 15 said "no regression beyond delta". Ask for 0.5% only with more layouts or for a change that cannot move code (the Go host, a build flag). |
| confidence | 95% (`-alpha 0.05`) | One comparison at a time. Use Holm's correction over apps when the question is "is any app slower". |
| disturbance | sibling or other work over 5% of the timing core, steal over 1%, 3 tries | Pairs where the sibling hyperthread was over 2% busy spread 5 times as much as quiet ones. 5% with at least 3 ticks keeps the rerun count bearable on a machine that also builds. |
| one process | one VM (`GOMAXPROCS=1`), on one core with its sibling idle (`taskset`, `-cpu`, `-sib`), at `nice 19` under `flock` | The pair of hyperthreads is the unit the timing owns. |

## What the protocol was shown to see

Measured on 2026-11-01 on the GCP machine, timing on cpu 2 with cpu 6 idle, the sum of the P50 of every page. For most of the day other work ran on the other cores (1.5 to 4 cores busy, builds and callgrind), and for the last runs it was quieter. Each run is 2 rounds over the 16 layouts.

**Runs with no change (A/A).** The same binary copied under a second name, three copies of it, and the same commit built again from a second clone with an empty Go cache (different bytes, every function at the same address).

| app | runs | comparisons | fresh pages | summary said faster or slower | formal layout said faster or slower |
| --- | ---: | ---: | --- | ---: | ---: |
| bench | 3 | 13 | no | 0 | 9 |
| shop | 1 | 1 | no | 0 | 1 |
| bench | 3 | 13 | yes | 0 | 0 |
| mui | 1 | 1 | yes | 0 | 0 |
| storefront | 1 | 1 | yes | 0 | 0 |

Over 29 A/A comparisons the summary never said "faster" or "slower", and its interval held 0 every time. Taking each round alone (one pair per layout, 58 comparisons), it said "slower" twice (3%). The comparisons in one run share sides, so they are not independent, and the rate is a rough one.

The formal layout alone said "faster" or "slower" in 10 of 14 A/A comparisons without fresh pages, by up to 2.7%, and in none of 15 with them. The cause is the physical pages: a copy of a binary keeps the pages it got when it was first read, and those placed it steadily faster or slower than another copy of the same bytes. Within a session the gap was the same in every process, so the pairs of the formal layout agreed with each other and the interval was narrow and wrong. Dropping the pages before each process turns that gap into run-to-run error: the run SD rose (about 0.7% to 1.4%), the A/A layout SD fell (about 1.1% to 0.2%), and the formal intervals became honest. The copies test ran twice back to back on a quiet machine to tell this from the load of the other cores. A binary built again from a second clone acted like one more copy.

**Known slowdowns.** A spin on the clock after each timed render, in the benchmark itself, of 0.5% or 1% of the median warm render (sides `s5` and `s10`), so the engine code and its layout are the same as the base's.

| true change | runs | comparisons | summary "slower" | summary interval held the truth |
| --- | ---: | ---: | ---: | ---: |
| +0.5% | 3 (bench twice, shop) | 8 | 3 | 8 |
| +1% | 2 (bench twice) | 4 | 4 | 3 |

A slowdown of 1% was found every time, one of 0.5% about a third of the time (2 of 3 with fresh pages on a quiet machine, 1 of 5 without them on a busy one). Of 41 summary intervals with a known truth, A/A runs included, 1 missed it (2%). Extra work in a hot handler (an increment of a field of the context in `OP_get_loc0`, +0.05% instructions on the shop) gave +2.1% at the formal layout and -0.25% over the layouts on bench: the formal layout alone would have called it slower.

**The smallest change each app can show.** From the run SD and the layout SD of the A/A runs, the change the summary finds 80% of the time at 95% (the t quantiles of 15 degrees of freedom). The first column is for a change that is the same at every layout. A real change to the engine also moves code, and the second column takes the largest layout SD seen in a real change on that app.

| app | run SD | A/A layout SD | layout SD of a real change | 2 rounds: same at every layout | 2 rounds: real change | 4 rounds: same at every layout | 4 rounds: real change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| bench | 1.42% | 0.50% | 0.62% (xw) | 0.83% | 0.87% | 0.64% | 0.70% |
| shop | 0.93% | 0 (assumed) | 1.50% (drop_get_arg0) | 0.48% | 1.22% | 0.34% | 1.17% |
| mui | 1.04% | 0.43% | 1.27% (C) | 0.63% | 1.09% | 0.50% | 1.03% |
| storefront | 0.92% | 0 | 1.02% (C) | 0.48% | 0.90% | 0.34% | 0.83% |

All with fresh pages. The shop had no A/A run with fresh pages: its run SD is that of drop_get_arg0 on a quiet machine, and the A/A run without fresh pages on a busy machine gave a run SD of 1.56% (0.81% and 1.73% at 2 rounds). With 32 layouts instead of 16 (give `-positions` 16 more shifts by hand) a real change of about 0.6% to 0.85% can be seen at 2 rounds. That follows from the same numbers and was not run.

**Callgrind.** The window of client requests gave the same counts for the same binary to 0.002% on bench. On the shop the whole program moved by 0.06% (the Go side: the collector and the scheduler) and the interpreter's own code by under 0.01%. Toggling on the Go function of the window instead lost 0.8% of the count of one binary and 7% of the other, which turned extra work in a hot handler (+0.05% instructions) into -6.7%.

## Reading a result

`bench judge -in FILE -kind APP -base base -new new` prints one row for the sum of the pages, which decides, and one row per page, which explains. `-detail` adds every layout with each of its pairs.

- **Read the summary of the sum.** A page row with its own verdict is one of several tests. Use it to say where a change acts, not whether it is real.
- **The formal layout against the summary.** The formal layout is the binary that ships today, but the next change to any engine function moves the code again. When the formal layout says "slower" and the summary does not, the change met a bad place, not a bad idea. In the validation above, extra work in a hot handler was +2.1% at the formal layout and -0.25% over the layouts.
- **Layout SD against run SD.** A layout SD near 0 means the change does about the same at every layout. A layout SD well over the run SD means the change acts through where the code lands (it moves a hot loop over a block boundary, or makes the body larger). Then the summary is the honest number and the formal one is luck.
- **Order effect.** The mean of y when the new side ran first, less the mean when the base ran first. Balanced blocks take it out of the mean, so a nonzero order effect does not bias the verdict, but one far from 0 says something carries over from one process to the next (the page cache, the clock speed).
- **Reruns.** The line under the table says how many blocks ran again and how many were left out. Many reruns mean the machine was busy. The verdict still holds, but the run took longer than it should.
- **Callgrind next to it.** A change in instructions of the whole program and of the interpreter's own code says what the change did. A drop in instructions with no drop in time says the cost moved into waiting (misses, mispredicted branches). Callgrind does not model Zen 3, so read its misses as a hint.

## When not to trust a result

## When not to trust a result

- **The judge reports a problem.** It stops on a failed process, a process recorded twice, a block that lacks a side, or sides that report different pages. Fix the run, do not edit the records.
- **The sides were not timed in the same blocks.** Compare only sides of one file, timed in one session. Two files from two days, or from two machines, differ by more than the changes this protocol is for.
- **The effect is under the smallest the app can show** (the table above). An inconclusive verdict there is the expected answer, not a failure. Run more rounds only when the run SD, not the layout SD, is what makes the interval wide (more rounds shrink only the run part).
- **Other cores were busy.** Builds and callgrind on other cores share the L3 cache and the memory bus with the timing core, and the disturbance check cannot see that (it reads the timing core and its sibling only). Each record keeps `machine_pct`. When the base and the new side ran under very different loads, or the load came and went during the run, time again on a quiet machine.
- **The winner of many tries.** The variant that came out best of several is, on average, luckier than it is good. Time it again in a new session before quoting it.
- **Another machine or another arch.** These numbers are for the GCP machine (Zen 3, 2 hyperthreads per core). They say nothing about darwin/arm64, which has no layouts, or about another x86 machine. Measure there with an A/A run first.
- **After a change to the tool, the harness or the machine.** Run an A/A first (the same binary copied under a second name) and check that it says "no regression beyond delta" and that its interval holds 0.
