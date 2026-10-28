#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# remote.sh [-t TREE] [-w SECONDS] [-g PATH]... REF -- make TARGET [VAR=value]...
#
# Runs a make target on another machine over ssh, for a machine that has LLVM or
# cores to spare when this one has not. The machine fetches the commit REF from
# the git remote "origin" (it must be pushed there), or gets a git bundle when
# it is not pushed or the machine cannot reach the remote. The make target runs
# in the background in a checkout of its own, TREE (default main), under the
# limits below, and this script waits for it, prints the end of its log, saves
# the log in .build/remote/ and exits with the status of the target (124 when
# -w ran out: the job goes on, see the log on the machine).
#
# Environment (no defaults, the script stops and says so):
#   SLOWJS_REMOTE        user@host
#   SLOWJS_REMOTE_DIR    folder on the machine, such as ~/work
# Optional:
#   SLOWJS_REMOTE_CPUS   cores of the job, such as 1 or 1,5 (taskset -c)
#   SLOWJS_REMOTE_MEM    memory cap of the job, such as 4G (systemd-run, no swap)
#   SLOWJS_REMOTE_SSH    the ssh command (default ssh)
#   SLOWJS_REMOTE_SCP    the scp command (default scp)
#
# -g PATH copies PATH (relative to the checkout) to .build/remote/ afterwards.
# The checkout is a git clone that shares the objects of DIR/repo.git, never a
# git worktree, and a new REF moves only the files git tracks: the generated
# engine and .build stay.
set -eu

tree=main wait=3600 get=
while getopts t:w:g: o; do
	case $o in
	t) tree=$OPTARG ;;
	w) wait=$OPTARG ;;
	g) get="$get $OPTARG" ;;
	*) sed -n '/^# remote.sh/,/^set -eu/p' "$0" | sed 's/^# \{0,1\}//; /^set -eu/d' >&2; exit 2 ;;
	esac
done
shift $((OPTIND - 1))
[ $# -ge 3 ] && [ "$2" = -- ] || { echo "usage: remote.sh [-t TREE] [-w SECONDS] [-g PATH]... REF -- make TARGET [VAR=value]..." >&2; exit 2; }
ref=$1; shift 2
: "${SLOWJS_REMOTE:?set SLOWJS_REMOTE to user@host}"
: "${SLOWJS_REMOTE_DIR:?set SLOWJS_REMOTE_DIR to a folder on the machine, such as ~/work}"
case $tree in *[!A-Za-z0-9_.-]*|'') echo "remote.sh: tree name $tree: use letters, digits, . _ and -" >&2; exit 2 ;; esac
case $SLOWJS_REMOTE_DIR in ''|'~'|'~/'|/|.|..) echo "remote.sh: SLOWJS_REMOTE_DIR=$SLOWJS_REMOTE_DIR is not a folder to own" >&2; exit 2 ;; esac
[ -z "${SLOWJS_REMOTE_CPUS:-}" ] || case $SLOWJS_REMOTE_CPUS in *[!0-9,-]*) echo "remote.sh: SLOWJS_REMOTE_CPUS=$SLOWJS_REMOTE_CPUS: want a list such as 1,5" >&2; exit 2 ;; esac
[ -z "${SLOWJS_REMOTE_MEM:-}" ] || case $SLOWJS_REMOTE_MEM in *[!0-9KMGT]*) echo "remote.sh: SLOWJS_REMOTE_MEM=$SLOWJS_REMOTE_MEM: want a size such as 4G" >&2; exit 2 ;; esac

ssh=${SLOWJS_REMOTE_SSH:-ssh}
scp=${SLOWJS_REMOTE_SCP:-scp}
host=$SLOWJS_REMOTE
dir=$SLOWJS_REMOTE_DIR

sq() { printf "'%s'" "$(printf %s "$1" | sed "s/'/'\\\\''/g")"; }
# rd is the folder as shell text on the machine: a leading ~/ stays outside the quotes.
case $dir in "~/"*) rd="~/$(sq "${dir#\~/}")" ;; *) rd=$(sq "$dir") ;; esac
rsh() { $ssh "$host" 'bash -s'; }

sha=$(git rev-parse --verify "$ref^{commit}")
short=$(printf %s "$sha" | cut -c1-10)

rsh <<EOT
set -e
mkdir -p $rd/trees $rd/jobs
[ -d $rd/repo.git ] || git init -q --bare $rd/repo.git
EOT

if [ "$(rsh <<EOT
git -C $rd/repo.git cat-file -e $sha^{commit} 2>/dev/null && echo yes || echo no
EOT
)" = no ]; then
	fetched=
	url=$(git remote get-url origin 2>/dev/null || true)
	pushed=
	[ -n "$url" ] && pushed=$(git ls-remote "$url" 2>/dev/null | awk -v s="$sha" '$1 == s && index($2, "refs/") == 1 {print $2}' | sort | head -n 1)
	if [ -n "$pushed" ]; then
		if rsh <<EOT
set -e
. ~/.profile
git -C $rd/repo.git fetch -q --no-tags $(sq "$url") $(sq "+$pushed:refs/slowjs/$tree")
EOT
		then fetched=1; echo "the machine fetched $pushed from the remote"
		else echo "the machine could not fetch $pushed: sending a bundle"; fi
	else
		echo "the commit $short is not pushed to origin: sending a bundle"
	fi
	if [ -z "$fetched" ]; then
		tmp=$(mktemp -d)
		trap 'rm -rf "$tmp"; git update-ref -d refs/slowjs/push 2>/dev/null || true' EXIT
		git update-ref refs/slowjs/push "$sha"
		excl=
		for tip in $(rsh <<EOT
git -C $rd/repo.git for-each-ref --format='%(objectname)'
EOT
); do git cat-file -e "$tip^{commit}" 2>/dev/null && excl="$excl ^$tip"; done
		# shellcheck disable=SC2086
		git bundle create -q "$tmp/push.bundle" refs/slowjs/push $excl
		echo "sending a bundle of $(du -h "$tmp/push.bundle" | cut -f1)"
		$scp -q "$tmp/push.bundle" "$host:$dir/push.bundle"
		rsh <<EOT
set -e
git -C $rd/repo.git fetch -q --no-tags $rd/push.bundle $(sq "+refs/slowjs/push:refs/slowjs/$tree")
rm -f $rd/push.bundle
EOT
	fi
fi

# The checkout shares the objects of repo.git. Files git does not track stay.
rsh <<EOT
set -e
t=$rd/trees/$tree
[ -d \$t/.git ] || { mkdir -p \$t && git -C \$t init -q; }
echo $rd/repo.git/objects > \$t/.git/objects/info/alternates
git -C \$t checkout -q -f --detach $sha
echo "tree $tree: \$(git -C \$t log --oneline -1)"
EOT

# The job: a make target under the limits, in the background, with a done file.
wrap=nice\ -n\ 19
[ -z "${SLOWJS_REMOTE_CPUS:-}" ] || wrap="taskset -c $SLOWJS_REMOTE_CPUS $wrap"
[ -z "${SLOWJS_REMOTE_MEM:-}" ] || wrap="systemd-run --user --scope -q -p MemoryMax=$SLOWJS_REMOTE_MEM -p MemorySwapMax=0 $wrap"
words=
for a in "$@"; do words="$words $(sq "$a")"; done
job=$tree-$$
inner=". ~/.profile; cd $rd/trees/$tree; $wrap$words; echo \$? > $rd/jobs/$job.done"
rsh <<EOT
rm -f $rd/jobs/$job.done
setsid nohup bash -c $(sq "$inner") > $rd/jobs/$job.log 2>&1 < /dev/null &
EOT
echo "job $job started (log: $dir/jobs/$job.log)"

waited=0 rc=
while [ "$waited" -lt "$wait" ]; do
	rc=$(rsh <<EOT
cat $rd/jobs/$job.done 2>/dev/null || true
EOT
)
	[ -n "$rc" ] && break
	sleep 5
	waited=$((waited + 5))
done
mkdir -p .build/remote
rsh <<EOT > ".build/remote/$job.log"
cat $rd/jobs/$job.log
EOT
tail -n 40 ".build/remote/$job.log"
if [ -z "$rc" ]; then
	echo "the job is still running after ${wait}s: its log is $dir/jobs/$job.log, its done file $dir/jobs/$job.done" >&2
	exit 124
fi
for g in $get; do
	$scp -q -r "$host:$dir/trees/$tree/$g" .build/remote/ || echo "remote.sh: could not copy $g" >&2
done
echo "the job ended with status $rc (log saved in .build/remote/$job.log)"
exit "$rc"
