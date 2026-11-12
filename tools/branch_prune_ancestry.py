#!/usr/bin/env python3
"""Delete only verified remote branches already ancestors of an exact main SHA.

Dry run is the default. For deletion, use an authenticated Git remote and
explicit --apply --confirm-main <40-hex-SHA>. Divergent branches are NEVER
deleted by this tool, even when their file names appear in main.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

HEX = re.compile(r"^[0-9a-f]{40}$")
REF = re.compile(r"^(?!/)(?!.*(?:\\.\\.|//|@\\{|\\\\|[ :~^?*\\[\\]]))[a-zA-Z0-9._/-]+$")


def git(*argv: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *argv], text=True, capture_output=True, check=check)


def remote_sha(remote: str, branch: str) -> str | None:
    p = git("ls-remote", "--heads", remote, f"refs/heads/{branch}")
    lines = p.stdout.splitlines()
    if not lines:
        return None
    if len(lines) != 1:
        raise ValueError(f"ambiguous remote ref for {branch}")
    sha, ref = lines[0].split("\t", 1)
    if not HEX.fullmatch(sha) or ref != f"refs/heads/{branch}":
        raise ValueError(f"unexpected remote ref for {branch}")
    return sha


def prune(report: dict, remote: str, apply: bool, confirm_main: str | None) -> dict:
    if not REF.fullmatch(remote):
        raise ValueError("invalid remote name")
    base = report["baseline_sha"]
    if not HEX.fullmatch(base):
        raise ValueError("invalid report baseline")
    actual_main = remote_sha(remote, "main")
    if actual_main != base:
        raise ValueError(f"remote main changed: report={base} remote={actual_main}")
    if apply and confirm_main != base:
        raise ValueError("deletion requires --confirm-main matching exact 40-hex SHA")
    if not apply and confirm_main:
        raise ValueError("--confirm-main is only valid with --apply")
    # Snapshot remote heads once. Per-branch ls-remote would issue hundreds of
    # network requests; an exact --force-with-lease still fences each deletion
    # against advances after this snapshot.
    heads: dict[str, str] = {}
    for line in git("ls-remote", "--heads", remote).stdout.splitlines():
        sha, ref = line.split("\t", 1)
        if not HEX.fullmatch(sha) or not ref.startswith("refs/heads/"):
            raise ValueError("malformed remote branch listing")
        heads[ref[len("refs/heads/"):]] = sha
    rows = report["branches"]
    result = {"baseline_sha": base, "remote": remote, "dry_run": not apply,
              "eligible": 0, "deleted": [], "skipped": [], "failed": []}
    seen: set[str] = set()
    for row in rows:
        full = row["branch"]
        prefix = remote + "/"
        if not isinstance(full, str) or not full.startswith(prefix):
            raise ValueError("report contains a branch outside target remote")
        name = full[len(prefix):]
        if name == "main" or name == "HEAD" or not REF.fullmatch(name) or name.startswith("-"):
            raise ValueError("invalid or protected branch in report")
        if name in seen:
            raise ValueError("duplicate branch in report")
        seen.add(name)
        expected = row["sha"]
        if not isinstance(expected, str) or not HEX.fullmatch(expected):
            raise ValueError("invalid pinned branch SHA")
        if row["classification"] != "ANCESTOR" or row["ahead"] != 0:
            result["skipped"].append({"branch": name, "reason": "divergent_or_unreviewed"})
            continue
        # Verify current branch contents, not merely old inventory classification.
        current = heads.get(name)
        if current != expected:
            result["skipped"].append({"branch": name, "reason": "changed_or_missing"})
            continue
        if git("merge-base", "--is-ancestor", expected, base, check=False).returncode != 0:
            result["skipped"].append({"branch": name, "reason": "not_ancestor_of_baseline"})
            continue
        result["eligible"] += 1
        if not apply:
            continue
        # This lease fences a concurrent branch advance. No force rewrite occurs:
        # it is a deletion guarded by the exact last observed branch SHA.
        deletion = git("push", "--porcelain",
                       f"--force-with-lease=refs/heads/{name}:{expected}",
                       remote, f":refs/heads/{name}", check=False)
        if deletion.returncode:
            result["failed"].append({"branch": name, "reason": "delete_rejected_or_ref_changed",
                                     "stderr": deletion.stderr[-500:]})
        else:
            result["deleted"].append(name)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--remote", default="origin")
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--confirm-main", default=None)
    parser.add_argument("--result", type=Path)
    args = parser.parse_args()
    try:
        report = json.loads(args.report.read_text(encoding="utf-8"))
        result = prune(report, args.remote, args.apply, args.confirm_main)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print(f"branch cleanup denied: {error}", file=sys.stderr)
        return 2
    body = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.result:
        args.result.write_text(body, encoding="utf-8")
    else:
        print(body, end="")
    return 1 if result["failed"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
