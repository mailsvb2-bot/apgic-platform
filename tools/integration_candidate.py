#!/usr/bin/env python3
"""Real isolated Git assembly of APGIC lanes. Never pushes, deploys or approves."""
from __future__ import annotations
import argparse
import json
import re
import subprocess
import sys
import tempfile
from collections import defaultdict
from pathlib import Path

SHA = re.compile(r"^[0-9a-f]{40}$")
MIG = re.compile(r"^(\d{6})_[^/]+\.sql$")

def git(repo, *args, check=True):
    return subprocess.run(["git", "-C", str(repo), *args],
                          text=True, capture_output=True, check=check)

def versions(root):
    grouped = defaultdict(list)
    for f in (root / "backend/migrations").glob("*.sql"):
        m = MIG.fullmatch(f.name)
        if m:
            grouped[m.group(1)].append(f.relative_to(root).as_posix())
    return [{"version": v, "files": sorted(paths)}
            for v, paths in sorted(grouped.items()) if len(paths) > 1]

def conflicts(root):
    out = git(root, "diff", "--name-only", "--diff-filter=U", check=False)
    if out.stdout.strip():
        return sorted(set(out.stdout.splitlines()))
    raw = git(root, "ls-files", "-u", check=False)
    return sorted({line.split("\t", 1)[-1] for line in raw.stdout.splitlines() if "\t" in line})

def refs(repo, plan):
    result = {}
    for name in [plan["baseline"]] + [x["branch"] for x in plan["lanes"]]:
        full = "refs/remotes/origin/" + name
        proc = git(repo, "rev-parse", "--verify", full, check=False)
        sha = proc.stdout.strip()
        if proc.returncode or not SHA.fullmatch(sha):
            raise ValueError("Unpinned or missing ref: " + full)
        result[name] = sha
    return result

def probe_pairwise(repository, tmp, plan, heads, control):
    dest = Path(tmp) / "pairwise"
    git(repository, "worktree", "add", "--detach", str(dest), control)
    probes = []
    try:
        git(dest, "config", "user.name", "APGIC Isolated Assembly")
        git(dest, "config", "user.email", "integration@invalid.example")
        for lane in plan["lanes"]:
            name = lane["branch"]
            if name == plan["control_branch"]:
                continue
            git(dest, "reset", "--hard", control)
            git(dest, "clean", "-fd", check=False)
            proc = git(dest, "merge", "--no-ff", "--no-edit", heads[name], check=False)
            if proc.returncode:
                probes.append({"branch": name, "sha": heads[name],
                               "owner": lane["owner"], "status": "CONFLICT_WITH_CONTROL",
                               "conflicting_paths": conflicts(dest)})
                git(dest, "merge", "--abort", check=False)
            else:
                collisions = versions(dest)
                probes.append({"branch": name, "sha": heads[name],
                               "owner": lane["owner"],
                               "status": "MIGRATION_COLLISION" if collisions else "MERGES_WITH_CONTROL_ALONE",
                               "migration_collisions": collisions})
    finally:
        git(repository, "worktree", "remove", "--force", str(dest), check=False)
    return probes

def assemble(plan, repository, scope="launch", retain=None):
    repository = Path(repository).resolve()
    if scope not in ("launch", "all"):
        raise ValueError("Unrecognized integration scope")
    heads = refs(repository, plan)
    base = heads[plan["baseline"]]
    control = heads[plan["control_branch"]]
    if git(repository, "merge-base", "--is-ancestor", base, control, check=False).returncode:
        raise ValueError("Control is not descendant of pinned main baseline")
    stages, completed = [], set()
    with tempfile.TemporaryDirectory(prefix="apgic-assembly-") as tmp:
        dest = Path(retain).resolve() if retain else Path(tmp) / "candidate"
        if dest.exists():
            raise ValueError("Candidate destination already exists: " + str(dest))
        git(repository, "worktree", "add", "--detach", str(dest), base)
        try:
            git(dest, "config", "user.name", "APGIC Isolated Assembly")
            git(dest, "config", "user.email", "integration@invalid.example")
            stopped = None
            for lane in plan["lanes"]:
                if scope == "launch" and not lane["launch_dependency"]:
                    continue
                name = lane["branch"]
                row = {"branch": name, "owner": lane["owner"], "profile": lane["profile"],
                       "stage": lane["order"], "source_sha": heads[name],
                       "candidate_sha_before": git(dest, "rev-parse", "HEAD").stdout.strip(),
                       "scope": "LAUNCH" if lane["launch_dependency"] else "POST_LAUNCH"}
                missing = [dep for dep in lane.get("dependencies", []) if dep not in completed]
                if stopped or missing:
                    row.update(status="NOT_EXECUTED", blockers=[{
                        "code": "UPSTREAM_INTEGRATION_BLOCKED",
                        "reason": "Upstream integration not complete: " + (stopped or ",".join(missing)),
                        "files": []}])
                else:
                    proc = git(dest, "merge", "--no-ff", "--no-edit", heads[name], check=False)
                    if proc.returncode:
                        row.update(status="MERGE_CONFLICT", blockers=[{
                            "code": "GIT_MERGE_CONFLICT", "files": conflicts(dest),
                            "reason": proc.stderr.strip()[-1200:] or "Git 3-way merge rejected"}])
                        git(dest, "merge", "--abort", check=False)
                        stopped = name
                    else:
                        row["candidate_sha_after"] = git(dest, "rev-parse", "HEAD").stdout.strip()
                        duplicates = versions(dest)
                        if duplicates:
                            row.update(status="POSTGRES_MIGRATION_COLLISION", blockers=[{
                                "code": "POSTGRES_MIGRATION_COLLISION",
                                "reason": "Different migrations share one sequential version",
                                "version": x["version"], "files": x["files"]} for x in duplicates])
                            stopped = name
                        else:
                            row.update(status="MERGED_FOR_TESTING", blockers=[])
                            completed.add(name)
                stages.append(row)
            temporary_sha = git(dest, "rev-parse", "HEAD").stdout.strip()
            independent = probe_pairwise(repository, tmp, plan, heads, control)
            return {"schema_version": "apgic-isolated-assembly-v1", "scope": scope,
                    "baseline_sha": base, "control_sha": control,
                    "temporary_candidate_sha": temporary_sha, "ephemeral": True,
                    "persisted_remote_branch": False, "production_release": False,
                    "retained_worktree_for_testing": str(dest) if retain else None,
                    "stages": stages, "pairwise_probes": independent,
                    "composition_complete": bool(stages) and all(s["status"] == "MERGED_FOR_TESTING" for s in stages),
                    "journey_runtime_verification": "REQUIRES_SEPARATE_TEST_EXECUTION",
                    "note": "This tree is not a proof of provider payment, deployed user journey or RELEASED status."}
        finally:
            if not retain:
                git(repository, "worktree", "remove", "--force", str(dest), check=False)

def summary(report):
    lines = ["## APGIC actual isolated Git assembly", "",
             "Launch composition: " + ("MERGED (NOT RELEASED)" if report["composition_complete"] else "BLOCKED"),
             "Baseline " + report["baseline_sha"] + "; control " + report["control_sha"]]
    for stage in report["stages"]:
        paths = sorted({f for blocker in stage["blockers"] for f in blocker.get("files", [])})
        lines.append("- " + stage["branch"] + " (" + stage["owner"] + "): " +
                     stage["status"] + "; conflicting paths=" + (", ".join(paths) or "none"))
    lines.extend(["", "### Independently attempted all ten heads against same control"])
    for probe in report["pairwise_probes"]:
        lines.append("- " + probe["branch"] + ": " + probe["status"] + " " +
                     ", ".join(probe.get("conflicting_paths", [])))
    lines.append("No external PSP/media/store provider was exercised. No production authorization.")
    return "\n".join(lines) + "\n"

def main(argv=None):
    p = argparse.ArgumentParser()
    p.add_argument("--plan", default="config/release/integration-plan-v1.json")
    p.add_argument("--repository", default=".")
    p.add_argument("--scope", choices=["launch", "all"], default="launch")
    p.add_argument("--retain-worktree")
    p.add_argument("--output", required=True)
    p.add_argument("--summary")
    args = p.parse_args(argv)
    try:
        plan = json.loads(Path(args.plan).read_text(encoding="utf-8"))
        result = assemble(plan, args.repository, args.scope, args.retain_worktree)
        status = 0 if result["composition_complete"] else 2
        msg = summary(result)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
        status = 2
        result = {"schema_version": "apgic-isolated-assembly-v1", "scope": args.scope,
                  "composition_complete": False, "production_release": False,
                  "error": "ASSEMBLY_OBSERVATION_UNAVAILABLE: " + str(exc), "stages": []}
        msg = "## Isolated assembly BLOCKED\n\n" + result["error"] + "\n"
    target = Path(args.output)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    if args.summary:
        with Path(args.summary).open("a", encoding="utf-8") as f:
            f.write(msg)
    print(msg)
    return status

if __name__ == "__main__":
    sys.exit(main())
