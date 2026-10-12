#!/usr/bin/env python3
"""Cross-lane APGIC integration decision; no mutations, no CI-is-release claims.

Reads live GitHub compare snapshots (or a pinned offline snapshot), groups REAL
merge conflicts and missing journey evidence by owner. A ready result requires
an integrated SHA and a separately verified canonical launch-readiness assessment.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from collections import defaultdict
from pathlib import Path

SHA = re.compile(r"^[0-9a-f]{40}$")
MIG = re.compile(r"^backend/migrations/(\d{6})_[^/]+\.sql$")
MULTI_SURFACE_FILES = {
    "apps/mobile/src/App.tsx": "native-shell-owner",
    "apps/web/app/journey.tsx": "web-journey-owner",
    "apps/web/app/page.tsx": "web-journey-owner",
    "apps/web/app/globals.css": "design-system-owner",
}


def github_json(repo: str, endpoint: str, token: str | None) -> object:
    url = f"https://api.github.com/repos/{repo}/{endpoint}"
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "apgic-integration-control/1"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def live_snapshot(plan: dict, candidate_ref: str, token: str | None) -> dict:
    repo = plan["repository"]
    base = github_json(repo, f"branches/{urllib.parse.quote(plan['baseline'], safe='')}", token)
    candidate = github_json(repo, f"branches/{urllib.parse.quote(candidate_ref, safe='')}", token)
    assert isinstance(base, dict) and isinstance(candidate, dict)
    candidate_sha = candidate["commit"]["sha"]
    branches = {}
    for lane in plan["lanes"]:
        name = lane["branch"]
        branch = github_json(repo, f"branches/{urllib.parse.quote(name, safe='')}", token)
        assert isinstance(branch, dict)
        sha = branch["commit"]["sha"]
        compare = github_json(repo, f"compare/{base['commit']['sha']}...{sha}?per_page=100", token)
        ancestry = github_json(repo, f"compare/{sha}...{candidate_sha}?per_page=1", token)
        assert isinstance(compare, dict) and isinstance(ancestry, dict)
        files = [f["filename"] for f in compare.get("files", [])]
        # GitHub returns max 300 changed files from compare endpoint. Do not
        # treat an incomplete file listing as proof of conflict-freedom.
        complete = len(files) < 300 and compare.get("too_large") is not True
        branches[name] = {
            "sha": sha,
            "files": files,
            "files_complete": complete,
            "ahead_by": compare.get("ahead_by"),
            "included_in_candidate": (ancestry.get("behind_by") == 0 and ancestry.get("status") in ("ahead", "identical")),
            "comparison_url": f"https://github.com/{repo}/compare/{base['commit']['sha']}...{sha}",
        }
    return {"schema_version": "apgic-branch-snapshot-v1", "repository": repo,
            "baseline_sha": base["commit"]["sha"], "candidate_ref": candidate_ref,
            "candidate_sha": candidate_sha, "branches": branches}


def assessment(plan: dict, snapshot: dict, launch_assessment: dict | None = None) -> dict:
    repo = plan["repository"]
    blockers = []
    checked = []
    candidate = snapshot.get("candidate_sha", "")
    baseline = snapshot.get("baseline_sha", "")

    def add(code: str, owner: str, reason: str, evidence: list[str], *, scope: str = "LAUNCH", affected: list[str] | None = None) -> None:
        blockers.append({"code": code, "owner": owner, "scope": scope, "reason": reason,
                         "affected": affected or [], "evidence": evidence,
                         "resolution": "PENDING_WITH_OWNER"})

    if snapshot.get("repository") != repo or not SHA.fullmatch(str(candidate)) or not SHA.fullmatch(str(baseline)):
        add("INVALID_INTEGRATION_IDENTITY", "platform-readiness", "A full repository, base and candidate SHA are required", [])
    lanes = {row["branch"]: row for row in plan["lanes"]}
    names_by_path: dict[str, list[str]] = defaultdict(list)
    migrations: dict[str, list[tuple[str, str]]] = defaultdict(list)
    for name, lane in lanes.items():
        value = snapshot.get("branches", {}).get(name)
        is_launch = lane["launch_dependency"]
        scope = "LAUNCH" if is_launch else "POST_LAUNCH"
        if not isinstance(value, dict) or not SHA.fullmatch(str(value.get("sha", ""))):
            add("BRANCH_SNAPSHOT_MISSING", lane["owner"], f"Cannot verify pinned branch {name}", [], scope=scope, affected=[name]); continue
        sha = value["sha"]
        evidence = [f"https://github.com/{repo}/tree/{sha}"]
        if value.get("files_complete") is not True or not isinstance(value.get("files"), list):
            add("BRANCH_CHANGESET_INCOMPLETE", lane["owner"], f"Cannot rule out unseen changes in {name}", evidence, scope=scope, affected=[name])
        for path in value.get("files", []):
            if not isinstance(path, str):
                continue
            names_by_path[path].append(name)
            m = MIG.match(path)
            if m and not path.split("/")[-1].startswith("ci_"):
                migrations[m.group(1)].append((name, path))
        if value.get("included_in_candidate") is not True:
            add("BRANCH_NOT_INTEGRATED", lane["owner"], f"Pinned head {sha} is not an ancestor of the integration candidate {candidate}", evidence,
                scope=scope, affected=[name])
        else:
            checked.append({"branch": name, "sha": sha, "included": True})

    for version, entries in sorted(migrations.items()):
        unique_paths = {path for _, path in entries}
        if len(unique_paths) <= 1:
            continue
        participating = sorted({name for name, _ in entries})
        scope = "LAUNCH" if any(lanes[n]["launch_dependency"] for n in participating) else "POST_LAUNCH"
        sources = [f"https://github.com/{repo}/blob/{snapshot['branches'][name]['sha']}/{path}" for name, path in entries]
        add("POSTGRES_MIGRATION_VERSION_COLLISION", "postgres-schema-owner",
            f"Migration {version} has {len(unique_paths)} different definitions. Allocate an immutable global order, reconcile DDL and test migrate-from-main + fresh DB + rollback/rollforward where safe; never blindly rename after deployment.", sources,
            scope=scope, affected=participating)

    critical_shared = dict(plan.get("canonical_contract_owners", {}))
    critical_shared.update(MULTI_SURFACE_FILES)
    for path, owners in sorted(names_by_path.items()):
        unique = sorted(set(owners))
        if len(unique) < 2:
            continue
        if path in critical_shared:
            code = ("OPENAPI_CONTRACT_COLLISION" if path.startswith("contracts/openapi/") else
                    "GENERATED_CLIENT_COLLISION" if "/generated/" in path else "SHARED_COMPOSITION_COLLISION")
            owner = critical_shared[path]
        elif path.startswith("backend/migrations/"):
            continue  # namespaced by migration version above
        elif path == ".github/workflows/ci.yml":
            continue  # workflow ownership is not a business contract conflict
        else:
            continue
        refs = [f"https://github.com/{repo}/blob/{snapshot['branches'][name]['sha']}/{path}" for name in unique]
        scope = "LAUNCH" if any(lanes[n]["launch_dependency"] for n in unique) else "POST_LAUNCH"
        add(code, owner, f"Multiple heads change {path}; require one semantic owner, reviewed combined diff, compatibility and real integrated runtime proof. Independent CI is insufficient.", refs,
            scope=scope, affected=unique)

    # Canon v7 launch requirements are independent of post-launch feature merging.
    if launch_assessment is None:
        add("CANON_LAUNCH_ASSESSMENT_MISSING", "platform-readiness",
            "No exact-SHA R0-R4 requirement and production journey assessment supplied; cannot mark VERIFIED as RELEASED.",
            [f"https://github.com/{repo}/blob/{candidate}/tools/launch_readiness.py"] if SHA.fullmatch(str(candidate)) else [])
    elif launch_assessment.get("candidate_sha") != candidate:
        add("CANON_LAUNCH_ASSESSMENT_SHA_MISMATCH", "platform-readiness",
            "Production launch assessment is not bound to candidate SHA", [])
    else:
        if launch_assessment.get("launch_ready") is not True or launch_assessment.get("blockers"):
            for reason in launch_assessment.get("blockers", []) or ["CANON_LAUNCH_NOT_READY"]:
                add("CANON_" + str(reason), "platform-readiness", f"Launch assessment blocked: {reason}",
                    [f"https://github.com/{repo}/blob/{candidate}/canon/requirements/registry.yaml"])
        # Check the strict gate's required evidence dimensions, not just a
        # caller-supplied launch_ready flag. Human verification still applies.
        dimensions = launch_assessment.get("evidence", {})
        if not isinstance(dimensions, dict) or not (
            isinstance(dimensions.get("production_index_records"), int) and
            dimensions["production_index_records"] > 0 and
            dimensions.get("r4_production_manifest_supplied") is True and
            dimensions.get("production_config_preflight_supplied") is True and
            dimensions.get("critical_journey_manifest_supplied") is True
        ):
            add("CANON_PRODUCTION_PROOF_INCOMPLETE", "platform-readiness",
                "Required production index, financial/provider, config and critical journey evidence are not all present", [])
        # A self-authored boolean is not certification: caller supplies the
        # strict gate's signed-off report; independent review still required.
        if launch_assessment.get("evidence_class") != "CANDIDATE_ASSESSMENT_NOT_RELEASE_EVIDENCE":
            add("CANON_ASSESSMENT_CLASS_INVALID", "platform-readiness", "Unknown gate report class", [])

    launch_blockers = [b for b in blockers if b["scope"] == "LAUNCH"]
    decision = "READY_FOR_SEPARATE_PROMOTION_REVIEW" if not launch_blockers else "BLOCKED"
    return {"schema_version": "apgic-integration-control-v1", "repository": repo,
            "candidate_ref": snapshot.get("candidate_ref"), "candidate_sha": candidate,
            "baseline_sha": baseline, "decision": decision, "promotion_authorized": False,
            "integrated_pinned_branches": checked,
            "launch_blockers": launch_blockers,
            "post_launch_blockers": [b for b in blockers if b["scope"] != "LAUNCH"],
            "integration_order": [{"branch": b["branch"], "owner": b["owner"], "profile": b["profile"], "stage": b["order"]} for b in plan["lanes"]],
            "note": "Not an authorization to deploy, collect money or change main. Verification != release."}


def markdown(report: dict) -> str:
    lines = ["## APGIC cross-lane integration: release decision", "",
             f"Candidate: `{report['candidate_ref']}` / `{report['candidate_sha']}`",
             f"Decision: **{report['decision']}**; promotion authorized: **false**.",
             f"Launch blockers: **{len(report['launch_blockers'])}**; post-launch blockers (not Launch v1 gates): **{len(report['post_launch_blockers'])}**.", ""]
    for label, entries in (("Launch R0–R4", report["launch_blockers"]), ("Post-launch / all-ten convergence", report["post_launch_blockers"])):
        lines.extend([f"### {label}", ""])
        if not entries:
            lines.append("No blockers recorded (independent evidence review still required).")
        for b in entries:
            lines.append(f"- **{b['code']}** — owner `{b['owner']}`: {b['reason']}")
            if b["evidence"]:
                lines.append("  - Evidence: " + ", ".join(f"[source]({u})" for u in b["evidence"][:8]))
        lines.append("")
    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", default="config/release/integration-plan-v1.json")
    p.add_argument("--snapshot", help="Pinned GitHub snapshot JSON; otherwise fetch live heads")
    p.add_argument("--candidate-ref", default="feat/platform-readiness")
    p.add_argument("--launch-assessment", help="Existing strict launch gate JSON, candidate-bound")
    p.add_argument("--output", required=True)
    p.add_argument("--summary", help="Append human-readable owner/blocker matrix")
    args = p.parse_args(argv)
    try:
        plan = json.loads(Path(args.plan).read_text(encoding="utf-8"))
        snapshot = (json.loads(Path(args.snapshot).read_text(encoding="utf-8")) if args.snapshot else
                    live_snapshot(plan, args.candidate_ref, os.environ.get("GITHUB_TOKEN")))
        evidence = json.loads(Path(args.launch_assessment).read_text(encoding="utf-8")) if args.launch_assessment else None
        report = assessment(plan, snapshot, evidence)
    except (OSError, ValueError, KeyError, AssertionError, urllib.error.URLError) as exc:
        # Infrastructure failure cannot silently become an all-green report.
        report = {"schema_version": "apgic-integration-control-v1", "decision": "BLOCKED",
                  "candidate_ref": args.candidate_ref, "candidate_sha": "UNVERIFIED", "promotion_authorized": False,
                  "launch_blockers": [{"code": "INTEGRATION_OBSERVATION_UNAVAILABLE", "owner": "platform-readiness",
                                       "scope": "LAUNCH", "reason": str(exc), "affected": [], "evidence": []}],
                  "post_launch_blockers": [], "integrated_pinned_branches": []}
    path = Path(args.output)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if args.summary:
        with Path(args.summary).open("a", encoding="utf-8") as fh:
            fh.write(markdown(report))
    print(markdown(report))
    return 0 if report["decision"] == "READY_FOR_SEPARATE_PROMOTION_REVIEW" else 2


if __name__ == "__main__":
    sys.exit(main())
