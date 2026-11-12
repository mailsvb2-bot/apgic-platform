#!/usr/bin/env python3
"""Read-only inventory of remote branches against a pinned integration baseline.

An unmerged commit is NOT proof of missing behavior; similarly a same-named
file is NOT proof that a feature was integrated. This tool reports evidence
for human code review and never merges, deletes, or rewrites repository refs.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path

MIGRATION = re.compile(r"^backend/migrations/(?P<version>[0-9]{6})_[^/]+\.sql$")
REMOTE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]*$")


def git(*args: str) -> str:
    result = subprocess.run(
        ["git", *args], check=True, text=True, capture_output=True
    )
    return result.stdout.strip()


def files(ref: str) -> set[str]:
    output = git("ls-tree", "-r", "--name-only", ref)
    return set(output.splitlines()) if output else set()


def migration_index(paths: set[str]) -> dict[str, list[str]]:
    result: dict[str, list[str]] = {}
    for path in sorted(paths):
        match = MIGRATION.fullmatch(path)
        if match:
            result.setdefault(match.group("version"), []).append(path)
    return result


def inventory(base: str = "origin/main", remote: str = "origin") -> dict:
    if not REMOTE_NAME.fullmatch(remote):
        raise ValueError("invalid remote name")
    baseline = git("rev-parse", "--verify", base + "^{commit}")
    base_paths = files(baseline)
    migrations = migration_index(base_paths)
    baseline_collisions = [
        {"version": version, "paths": paths}
        for version, paths in sorted(migrations.items()) if len(paths) > 1
    ]
    names = sorted(
        name for name in git("for-each-ref", "--format=%(refname:short)", "refs/remotes/" + remote + "/").splitlines()
        if name != remote and name != remote + "/HEAD" and name != remote + "/main"
    )
    branches = []
    for name in names:
        head = git("rev-parse", "--verify", name + "^{commit}")
        ahead = int(git("rev-list", "--count", baseline + ".." + head))
        behind = int(git("rev-list", "--count", head + ".." + baseline))
        if ahead == 0:
            branches.append({"branch": name, "sha": head, "ahead": 0,
                             "behind": behind, "classification": "ANCESTOR",
                             "introduced_paths_absent_from_main": [],
                             "migration_number_conflicts": []})
            continue
        merge_base = git("merge-base", baseline, head)
        introduced = git("diff", "--diff-filter=A", "--name-only", merge_base, head)
        absent = sorted(set(introduced.splitlines()) - base_paths)
        conflicting = []
        for path in absent:
            match = MIGRATION.fullmatch(path)
            if match and match.group("version") in migrations:
                for main_path in migrations[match.group("version")]:
                    conflicting.append({
                        "branch_path": path,
                        "main_path": main_path,
                        "version": match.group("version"),
                    })
        branches.append({
            "branch": name, "sha": head, "ahead": ahead, "behind": behind,
            "classification": "DIVERGENT_REQUIRES_SEMANTIC_REVIEW",
            "introduced_paths_absent_from_main": absent,
            "migration_number_conflicts": conflicting,
        })
    return {
        "schema_version": "apgic-branch-functional-inventory-v1",
        "baseline_sha": baseline,
        "remote": remote,
        "total_non_main": len(branches),
        "ancestor_branches": sum(b["classification"] == "ANCESTOR" for b in branches),
        "divergent_branches": sum(b["classification"] != "ANCESTOR" for b in branches),
        "baseline_migration_number_collisions": baseline_collisions,
        "unresolved_migration_number_conflicts": sum(
            len(b["migration_number_conflicts"]) for b in branches
        ),
        "interpretation": "File/path/ancestry evidence only; code semantics and merged equivalent implementations require manual inspection.",
        "branches": branches,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="origin/main")
    parser.add_argument("--remote", default="origin")
    parser.add_argument("--output", type=Path, help="Optional JSON output file")
    args = parser.parse_args()
    report = inventory(args.base, args.remote)
    payload = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")


if __name__ == "__main__":
    main()
