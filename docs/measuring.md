---
summary: 'How to measure whether a change makes renders faster or slower, down to a fraction of a percent: the protocol (layouts checked against the formal binary, balanced blocks, whole AB/BA units, the manifest of a run, the verdict against a stated delta, callgrind next to it), its defaults and why, what one batch of runs with known effects gave, how to read a result, its known limits and when not to trust one'
read_when:
  - Timing a change to the engine or to tifa against the code before it
  - Deciding whether a change of less than a few percent is real
  - Reading the output of sjdev bench judge, or a result someone else measured
  - Changing sjdev bench, the layouts it builds or the defaults of the protocol
  - Using callgrind to explain a timing
---

# Measuring a change

A change for speed is judged by timing tifa's pages, the same way every time. This page is the protocol, its defaults, what one batch of runs with known effects gave, and what it cannot do. The tool is `go tool sjdev bench` (commands in [testing.md](testing.md#time-one-build-against-another)).

Results hold for the machine they were measured on. A result on linux/amd64 says nothing about darwin/arm64, and the layouts below exist only on amd64.

## The protocol

1. **Fix the question first.** Write down the machine, the pages, the metric and the regression that matters, delta. The default is delta = 1% on the sum of the P50 of every page of the app, on the GCP machine (AMD EPYC 7B13, Zen 3). Timing decides. Instruction counts explain.
2. **Build every side at the 16 protocol layouts** (`bench build`, `-positions protocol`, the default). A layout moves the code without changing it:
   - front: 0, 8, ..., 56 bytes of NOP at the start of the interpreter body, so the body takes each 8-byte place in its 64-byte block,
   - shift: bytes of NOP after the first engine function the binary keeps, so every engine function moves to other blocks and another place in its 4 KiB page.

   The set is the 8 fronts at shift 0, and the 8 fronts again at 8 shifts spread over a page (32, 576, 1120, ..., 3840), paired so that front and shift do not grow together. Layout 0.0 is the formal binary, the one that ships.

   `bench build` checks every layout against the formal binary of the same side (it builds one in `.work` when 0.0 is not in the set): the interpreter moved by the shift rounded up to 64 bytes (not at all without a shift), and its body and 10 hot handlers sit `front` bytes later in it than in the formal binary, so their place modulo 64 is the formal one plus front. A layout that did not land there fails the build. It writes the addresses to `layouts.tsv`, one row per binary with the SHA-256 of the file, and a binary built again replaces its row.
3. **Time in balanced blocks** (`bench run`). A block is every side at one layout, one after the other. Blocks come in a random order, the formal layout 3 times per round. Within a block the sides run in a random order in an even round and in the reverse order in the next round. The default is 2 rounds. The orders are drawn from the experiment id of the run (see the manifest below), so a new run gets new orders and a run that goes on in the same file keeps its own.

   Before each process the tool drops the cached pages of its binary (`-fresh-pages`, on by default on linux): it syncs the file, asks the kernel to drop its pages (`fadvise DONTNEED`), then checks with `mincore` that none of them is still in memory, and stops with an error when one is. The kernel then reads the binary again into pages it picks, which may or may not be the ones it had. Each record says whether fresh pages were asked for (`fresh_pages`) and whether the check passed (`fresh_verified`). Without it, a binary keeps the physical pages it got when it was first read for as long as it stays cached, and two copies of the same bytes timed up to 2.7% apart at the formal layout, every time.
4. **Keep a manifest of the run.** `bench run -out FILE` writes `FILE.manifest.json` before the first process: a random experiment id, the plan (sides, layouts, rounds, formal blocks per round, fresh pages), the SHA-256 of every binary, and the settings that change what a process measures (the command line, the folder as an absolute path, the environment, the cores, the disturbance limits). The environment it records is the whole environment of the processes: HOME, PATH and TMPDIR of the shell, and what `-env` gives. Nothing else of the shell reaches them, and the tool names the GO* and TIFA_* variables of the shell it leaves out. A run that goes on in the same file must match it: other binaries, another environment (such as another GOMAXPROCS, GOGC or TIFA_WARM), another folder or other layouts stop it. More rounds can be added (`-round0`). A file with records but no manifest is refused. Every record carries the experiment id.
5. **Run a disturbed block again, whole.** A block runs again, up to `-tries` (3), when any of its processes ran while the sibling hyperthread was over 5% busy, other work took over 5% of the timing core, or the host stole over 1% of it. Every record keeps its start time, slot, order, attempt, session, steal time and the load of the other cores (`machine_pct`, 100 is one core), since builds on other cores share the L3 cache and are not a reason to run again (see below).
6. **Judge whole units** (`bench judge`). The unit of the analysis is the two blocks of one layout and one rep in rounds 2k and 2k+1, where the two sides ran in opposite orders. The judge reads the records against the plan of the manifest, never against what the file happens to hold:
   - A failed process, a record twice, a record of another experiment or outside the plan, or a block whose sides ran in different sessions, with other page settings or in the same order in both rounds is a problem. With any problem nothing is judged.
   - A block counts by its last attempt. It is usable when that attempt has every side of the plan and none of them was disturbed. A unit is used only when both its blocks are usable, otherwise it is lost and listed with the reason.
   - The run is complete when every layout of the plan has a usable unit and the formal layout has 2 (or all it planned, when fewer). An incomplete run gets no verdict, only "incomplete" and the list of what is missing. Run the same command again with a higher `-tries` to finish it: the lost blocks run again and the rest is kept.
7. **Read the numbers of a unit.** Each block gives y = ln(new/base) of the metric. A unit gives d, the mean of its two y, in which the order of the sides cancels, and o, the y where the new side ran first less the y where the base ran first. The judge reports, side by side:
   - **the formal layout**: the mean d of its units, with a t interval from the run error pooled over every unit,
   - **the mean of the layouts**: each layout of the plan with the same weight, with the t interval of the spread of the layout means (L-1 degrees of freedom),
   - **these layouts only**: the same mean with the run error alone, how well these very layouts were measured,
   - **the run SD**: the SD of one block's y after taking out the order effect, from the spread of o between units,
   - **the layout SD**: how much the true effect seems to differ between layouts, the spread of the layout means less what the run error explains,
   - **the order effect**: the mean of o, with its standard error.

   **The mean of the layouts decides, and the formal layout is reported on its own.** Its interval treats the 16 layouts as if they were a random sample of the places a change can land. They are not: they are a fixed set chosen to cover the 8-byte places of the body and the page offsets, so the interval is a model, not a sampling guarantee (see [Known limits](#known-limits)). The run SD and the layout SD are descriptive estimates. They assume the run errors of all blocks are independent with one variance once the order effect is out, and a layout SD of 0 only says the layout means spread no more than the run error explains, not that the change does the same at every layout.
8. **Read the verdict against delta.** With the 95% interval [lo, hi] of the change:
   - faster: hi below 0,
   - slower: lo above 0 ("slower, by less than delta" when hi is also below delta),
   - no regression beyond delta: the interval holds 0 and hi is below delta,
   - inconclusive: the interval reaches past delta,
   - incomplete: the run is not complete (step 6).

   "No regression beyond delta" is the only verdict that says a change does not cost what matters. An interval that holds 0 and reaches past delta does not show that a change is neutral: it shows the measurement could not tell.

   When the formal layout is slower, the judge says so under the table even when the mean of the layouts is not: "**The formal layout is slower by more than delta**" when its whole interval is past delta, "**The formal layout is slower**" when it is above 0. The mean of the layouts does not cancel this: the binary as it ships would be slower.
9. **Count with callgrind next to it, never instead** (`bench cg -small 0 -big 3 -dumps -sim`). One process per binary warms every page, then counts only the window the benchmark marks with client requests, 3 renders of every page (see [testing.md](testing.md#time-one-build-against-another)). The table shows the whole program and the interpreter's own code: instructions, branches, mispredicted branches and cache misses, and the raw files are kept. Callgrind's predictor and caches are models of old hardware, not of Zen 3, so a rise in its misses is a reason to look closer, not a verdict. When the counts and the timing disagree, keep measuring. Neither overrules the other.
10. **Confirm a winner on new data.** When several variants were tried and one is kept, time it again in a new run (a new file, so a new experiment id and new orders) before quoting it. The judge compares one pair of sides on one app. It does no correction over apps or over pairs: when one change is judged on 4 apps at 95% each, the chance that some app shows a false "slower" or "faster" is up to about 1 in 5 (1 - 0.95^4), so a single app out of line is weak evidence on its own.

## The defaults and why

| setting | default | why |
| --- | --- | --- |
| layouts | 16 (`-positions protocol`) | The spread between layouts is most of the error, and only more layouts shrink it. 8 fronts cover every 8-byte place of the body in its block, the 8 shifts move every other function to other blocks and page offsets. |
| rounds | 2 (`-rounds 2`) | Two rounds make one unit per layout, each side once in each order. 4 rounds shrink only the run part of the error: in the table below, the change found 80% of the time drops from about 0.8% to 0.6% when it is the same at every layout, but only from about 0.9% to 0.7% on bench for a real change to the engine. For a real change, more layouts pay more than more rounds. |
| formal blocks per round | 3 (`-formal-reps 3`) | The formal layout gets 3 units, enough for its own interval and for a run that loses one of them to stay complete. |
| renders per page | 8 warm-up, then 20 timed (`TIFA_WARM=8`, `-test.benchtime 20x`), the P50 | The sum of the P50s of one process differs from the next by about 1% (the run SD below), small next to the 4% to 5% between layouts. |
| delta | 1% (`-delta 0.01`) | About the smallest real change to the engine 2 rounds can show (0.9% to 1.1% on bench, mui and the storefront, 1.2% on the shop, in the table below). Against 0.5%, 7 of 15 A/A comparisons with fresh pages came out inconclusive. Against 1%, all 15 said "no regression beyond delta". Ask for 0.5% only with more layouts or for a change that cannot move code (the Go host, a build flag). |
| confidence | 95% (`-alpha 0.05`) | One comparison at a time. The tool does not correct over apps (step 10). |
| disturbance | sibling or other work over 5% of the timing core, steal over 1%, 3 tries | Pairs where the sibling hyperthread was over 2% busy spread 5 times as much as quiet ones. 5% with at least 3 ticks keeps the rerun count bearable on a machine that also builds. |
| one process | one VM (`GOMAXPROCS=1`), on one core with its sibling idle (`taskset`, `-cpu`, `-sib`), at `nice 19` under `flock` | The pair of hyperthreads is the unit the timing owns. |

## What one batch of runs gave

Measured on 2026-11-01 on the GCP machine, timing on cpu 2 with cpu 6 idle, the sum of the P50 of every page, read again with the judge as it is now. For most of the day other work ran on the other cores (1.5 to 4 cores busy, builds and callgrind), and for the last runs it was quieter. Each run is 2 rounds over the 16 layouts. These runs predate the `mincore` check and the manifest: their fresh pages were asked for but not checked, and their plan is given to the judge by flags (`-rounds 2 -formal-reps 3`). These numbers are one batch on one machine. They show the protocol can work, not rates it will hold to.

Four runs are incomplete under the rule of step 6, since blocks still disturbed after 3 tries left layouts without a unit: the first all-sides run on bench (8 of 18 units) and on the shop (11 of 18), and the copies test without fresh pages on the busy machine (17 of 18). They get no verdict and are left out of the counts below.

**Runs with no change (A/A).** The same binary copied under a second name, three copies of it, and the same commit built again from a second clone with an empty Go cache (different bytes, every function at the same address).

| app | complete runs | comparisons | fresh pages | mean of the layouts said faster or slower | formal layout said faster or slower |
| --- | ---: | ---: | --- | ---: | ---: |
| bench | 1 | 6 | no | 0 | 5 |
| bench | 3 | 13 | yes | 0 | 0 |
| mui | 1 | 1 | yes | 0 | 0 |
| storefront | 1 | 1 | yes | 0 | 0 |

In this batch the mean of the layouts gave 0 false positives in 21 A/A comparisons, and its interval held 0 every time. The comparisons in one run share sides, so they are not independent, and 21 of them cannot show a false positive rate.

The formal layout alone said "faster" or "slower" in 5 of 6 A/A comparisons without fresh pages, by up to 2.7%, twice past delta, and in none of 15 with them. The cause is the physical pages: a copy of a binary keeps the pages it got when it was first read, and those placed it steadily faster or slower than another copy of the same bytes. Within a session the gap was the same in every process, so the units of the formal layout agreed with each other and the interval was narrow and wrong. Dropping the pages before each process turned that gap into run-to-run error: on the copies test, back to back on a quiet machine, the run SD rose from about 0.7% to 1.35% and the A/A layout SD fell from about 1.2% to 0.4% (root mean squares over the 6 comparisons of each run). A binary built again from a second clone acted like one more copy.

**Known slowdowns.** A spin on the clock after each timed render, in the benchmark itself, aimed at 0.5% or 1% of the median warm render (sides `s5` and `s10`), so the engine code and its layout are the same as the base's. The spin is the target injected amount: it is set from the median of the warm-up renders of each process, so the true change of the P50 is close to it, not exactly it.

| target injected amount | complete runs | comparisons | mean of the layouts said "slower" | its interval held the target |
| --- | ---: | ---: | ---: | ---: |
| +0.5% | 1 (bench, fresh pages) | 3 | 2 | 3 |
| +1% | 1 (bench, fresh pages) | 2 | 2 | 2 |

Of the 26 intervals of the mean of the layouts with a known target in complete runs, A/A included, none missed it. The runs that would have shown a miss or a slowdown on the busy machine without fresh pages are the incomplete ones.

**The smallest change each app can show.** Approximate power from the model and the variance estimates: the change the mean of the layouts finds 80% of the time at 95% (the t quantiles of 15 degrees of freedom), from the run SD and the layout SD of the A/A comparisons with fresh pages. It is as good as the model of step 7 and the estimates behind it, and was not checked by simulation or by repeated runs. The first column is for a change that is the same at every layout. A real change to the engine also moves code, and the second column takes the largest layout SD seen in a real change on that app.

| app | run SD | A/A layout SD | layout SD of a real change | 2 rounds: same at every layout | 2 rounds: real change | 4 rounds: same at every layout | 4 rounds: real change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| bench | 1.41% | 0.44% | 0.66% (xw) | 0.80% | 0.88% | 0.61% | 0.71% |
| shop | 0.96% | 0 (assumed) | 1.49% (drop_get_arg0) | 0.50% | 1.22% | 0.35% | 1.17% |
| mui | 1.04% | 0.43% | 1.25% (C) | 0.63% | 1.08% | 0.50% | 1.01% |
| storefront | 0.96% | 0 | 0.99% (C) | 0.50% | 0.90% | 0.35% | 0.82% |

The shop had no complete A/A run with fresh pages: its run SD is that of drop_get_arg0 on a quiet machine. With 32 layouts instead of 16 (give `-positions` 16 more shifts by hand) a real change of about 0.6% to 0.85% could be seen at 2 rounds by the same model. That was not run.

**Callgrind.** The window of client requests gave the same counts for the same binary to 0.002% on bench. On the shop the whole program moved by 0.06% (the Go side: the collector and the scheduler) and the interpreter's own code by under 0.01%. Toggling on the Go function of the window instead lost 0.8% of the count of one binary and 7% of the other, which turned extra work in a hot handler (+0.05% instructions) into -6.7%.

## Reading a result

`bench judge -in FILE -kind APP -base base -new new` prints whether the run is complete, one row for the sum of the pages, which decides, and one row per page, which explains. `-detail` adds every layout with each of its units.

- **Complete first.** "INCOMPLETE, no verdict" means some layout has no usable unit. Raise `-tries` and run again. Do not read the numbers of an incomplete run as a result.
- **Read the mean of the layouts of the sum.** A page row with its own verdict is one of several tests. Use it to say where a change acts, not whether it is real.
- **The formal layout next to it.** The formal layout is the binary that ships today, but the next change to any engine function moves the code again. When the formal layout says "slower" and the mean of the layouts does not, the change met a bad place in this build: it ships slower, and the next build may not. The judge marks it under the table so it is not lost in the mean. In this batch, C on the storefront was +1.1% at the formal layout and +0.3% over the layouts, and drop_get_arg0 on the shop was +2.4% at the formal layout (past delta) and +1.3% over the layouts.
- **Layout SD against run SD.** A layout SD well over the run SD says the change acts through where the code lands (it moves a hot loop over a block boundary, or makes the body larger). Then the mean of the layouts is the honest number and the formal one is luck. A layout SD near 0 does not show the opposite (step 7).
- **Order effect.** The mean of o. Units take it out of their mean, so a nonzero order effect does not bias the verdict, but one far from 0 says something carries over from one process to the next (the page cache, the clock speed).
- **Reruns and lost units.** The lines under the table say how many attempts ran again and which units were lost and why. Many reruns mean the machine was busy.
- **Callgrind next to it.** A change in instructions of the whole program and of the interpreter's own code says what the change did. A drop in instructions with no drop in time says the cost moved into waiting (misses, mispredicted branches). Callgrind does not model Zen 3, so read its misses as a hint.

## Known limits

- **The layouts are a fixed set, not a random sample.** The interval of the mean of the layouts borrows the model of a random sample to say how far the effect could be over other layouts. Nothing has shown that the 16 layouts stand for the places a later change will land. Drawing layouts at random for each run would make that model hold, and the tool does not do it.
- **The run SD and the layout SD are estimates under a model.** They assume independent run errors of one variance once the order effect is out. Errors that depend on the layout, on the side or on the time of day go into one or the other without a sign.
- **The power table is from that model.** It was not checked by simulation or by many repeated runs.
- **No correction over apps or pairs.** Each verdict is one comparison at 95%.
- **Fresh pages do not pick the physical pages.** The check says the old pages left the page cache, not where the new ones land.

## When not to trust a result

- **The judge reports a problem or an incomplete run.** It stops on a failed process, a record twice, a record of another experiment, a block that lacks a side or ran in two sessions, or sides that report different pages. Fix the run, do not edit the records.
- **The sides were not timed in the same blocks.** Compare only sides of one file, timed in one run. Two files from two days, or from two machines, differ by more than the changes this protocol is for. The manifest refuses to go on with other binaries or settings in the same file.
- **The effect is under the smallest the app can show** (the table above). An inconclusive verdict there is the expected answer, not a failure. Run more rounds only when the run SD, not the layout SD, is what makes the interval wide (more rounds shrink only the run part).
- **Other cores were busy.** Builds and callgrind on other cores share the L3 cache and the memory bus with the timing core, and the disturbance check cannot see that (it reads the timing core and its sibling only). Each record keeps `machine_pct`. When the base and the new side ran under very different loads, or the load came and went during the run, time again on a quiet machine.
- **The winner of many tries.** The variant that came out best of several is, on average, luckier than it is good. Time it again in a new run before quoting it.
- **Another machine or another arch.** These numbers are for the GCP machine (Zen 3, 2 hyperthreads per core). They say nothing about darwin/arm64, which has no layouts, or about another x86 machine. Measure there with an A/A run first.
- **After a change to the tool, the harness or the machine.** Run an A/A first (the same binary copied under a second name) and check that it says "no regression beyond delta" and that its interval holds 0.
