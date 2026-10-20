---
summary: 'The fork change to regular expressions: skipping to where a regexp can start to match before running the backtracking machine, what it gains, which regexps take it, how timeouts still stop it, and what to do when upstream changes the regexp engine'
read_when:
  - Changing lib/regexskip.go or fixRegexSkip
  - fixlib stops on regexSites after an upstream release
  - A regexp gives another result than before, or a timeout does not stop a regexp
---

# Regular expressions



## Skip to where a regexp can match

Release: v0.26.15.

A regexp that is not sticky no longer tries every position of the string in the backtracking machine. Its bytecode starts with a loop that tries the match at one position, steps over one character and tries again. `lre_exec` now first calls `lreSkip` (`lib/regexskip.go`): when the regexp starts with one character or one class of characters, such as `/["&<>]/` or `/\d+/`, it finds the first position that holds one in a plain Go loop and starts there, and when there is none it answers no match without running the bytecode. A regexp that starts with `^` outside multiline mode is run at position 0 only. Unicode regexps (`u`, `v`), regexps that ignore case or are multiline (`i`, `m`), and every other first op take the old path. `lreSkip` checks for an interrupt after every 65,536 characters it looks at, as the backtracking machine does every 10,000 steps, so the eval timeout and `Interrupt` stop a regexp that runs again and again on a long string as before. A server render of a page of tifa's example shop takes about 3% less time on linux/amd64 (AMD EPYC), with the same HTML. `fixlib` puts the call into all 13 files, and checks the text of `lre_compile`, `lre_exec` and `lre_exec_backtrack`, whose bytecode and ops `lreSkip` reads, against fingerprints. This changes only the generated Go, with no C hunk (`internal/fixlib/regexskip.go`).

Tests: `lib/regexskip_test.go`, whose `testdata/regex.txt` holds what 61 regexps give on 47 strings through every API that runs one, from the engine before the change, `internal/fixlib/regexskip_test.go`, and `TestRegexScanStops` in `timeout_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If it reports that `Xlre_compile`, `Xlre_exec` or `_lre_exec_backtrack` changed (a fingerprint other than the one in `internal/fixlib/regexsites.go`), read the new code against `lib/regexskip.go`: the loop and `save_start 0` at the start of the bytecode, the header fields it reads, and what `char`, `range` and `line_start` match at a position must still be what `lreSkip` assumes, and `lre_exec` must still return `LRE_RET_TIMEOUT` when `lre_check_timeout` asks it to stop. Then put the new fingerprints in (`go run ./internal/fixlib -regex` prints them) and run `go test -run Regex ./lib`.
