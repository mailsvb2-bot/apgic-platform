#!/usr/bin/env python3
"""Functional source inventory for all retired APGIC branch heads.

This is source traceability, NOT proof of production readiness. Branch heads
are preserved in main ancestry; file presence alone is not feature acceptance.
No live API, provider, migration, deployment, remote branch or runtime mutation.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ARCHIVE = "docs/operations/branch-history-consolidation-20261113.json"
SHA = re.compile(r"^[0-9a-f]{40}$")

# Old paths must never be reintroduced with competing migration sequence IDs.
RENAMED_SQL = {
    "backend/migrations/000042_persona_companion.sql": "backend/migrations/000079_persona_companion.sql",
    "backend/migrations/000043_persona_rights_review.sql": "backend/migrations/000080_persona_rights_review.sql",
    "backend/migrations/000044_r9_live_events.sql": "backend/migrations/000074_r9_live_events.sql",
    "backend/migrations/000045_growth_first_party_attribution.sql": "backend/migrations/000075_growth_first_party_attribution.sql",
    "backend/migrations/000046_growth_integrity_guards.sql": "backend/migrations/000076_growth_integrity_guards.sql",
    "backend/migrations/000047_growth_verified_conversion_bridge.sql": "backend/migrations/000077_growth_verified_conversion_bridge.sql",
    "backend/migrations/000048_growth_owned_link_lifecycle.sql": "backend/migrations/000078_growth_owned_link_lifecycle.sql",
    "backend/migrations/000039_r6_instant_duty_dispatch.sql": "backend/migrations/000045_r6_instant_duty_dispatch.sql",
    "backend/migrations/000049_r2_r3_signed_payment_admission.sql": "backend/migrations/000054_r2_r3_signed_payment_admission.sql",
}
# The old R5 worker was replaced by one canonical execution path, not moved.
SUPERSEDED_R5 = {
    "backend/cmd/deletion-provider-receipt/main.go": "backend/internal/runtimepostgres/account_deletion_provider.go",
    "backend/cmd/deletion-worker/main.go": "backend/internal/runtimepostgres/account_deletion_runtime.go",
    "backend/internal/privacy/deletion_evidence_test.go": "backend/internal/privacy/deletion_policy_test.go",
    "backend/internal/privacy/deletion_provider_receipt.go": "backend/internal/privacy/deletion_provider_attestation.go",
    "backend/internal/runtimepostgres/account_deletion_executor.go": "backend/internal/runtimepostgres/account_deletion_execution.go",
    "backend/internal/runtimepostgres/account_deletion_executor_integration_test.go": "backend/internal/runtimepostgres/account_deletion_end_to_end_integration_test.go",
    "backend/internal/runtimepostgres/account_deletion_revocation.go": "backend/internal/runtimepostgres/account_deletion_provider.go",
    "backend/migrations/000049_r5_deletion_reconfirmation.sql": "backend/migrations/000049_r5_deletion_reconfirmation_evidence.sql",
    "backend/migrations/000050_r5_deletion_erasure_evidence.sql": "backend/migrations/000050_r5_durable_deletion_execution.sql",
}
LEGACY_DOCS = {
    "docs/traceability/R9-live-events-slice.md",
    "docs/operations/integration-control.md",
    "docs/operations/integration-user-journeys.md",
    "docs/operations/launch-v1-assurance.md",
    "docs/r2-r3-signed-payment-admission.md",
}
LEGACY_AUTH = "tools/auth001_web_tenant_isolation_e2e.sh"
# Explicitly verified canonical replacement entrypoints for risky old R5.
REPLACEMENT_GUARDS = {
    "backend/internal/runtimepostgres/account_deletion_execution.go": "func (c *Checker) AdvanceApprovedDeletion(",
    "backend/internal/runtimepostgres/account_deletion_runtime.go": "type DeletionRuntime struct",
    "backend/internal/runtimepostgres/account_deletion_provider.go": "deletion_provider_verified_receipts",
    "backend/internal/runtimepostgres/account_deletion_reconfirm.go": "ReconfirmAccountDeletionRequest(",
    "backend/internal/privacy/deletion_provider_attestation.go": "Verify",
    "backend/migrations/000049_r5_deletion_reconfirmation_evidence.sql": "apgic_delete_reconfirmation_guard",
    "backend/migrations/000050_r5_durable_deletion_execution.sql": "deletion_provider_verified_receipts",
}
# Historical API names removed with the abandoned, weaker R5 executor.
INTENTIONAL_R5_REPLACEMENTS = {
    ("backend/internal/privacy/deletion_policy.go", "Digest"),
    ("backend/internal/privacy/deletion_policy.go", "KeyFingerprint"),
    ("backend/internal/privacy/deletion_policy.go", "Policy"),
    ("backend/internal/privacy/deletion_policy.go", "RawJSON"),
    ("backend/internal/privacy/deletion_policy.go", "Signature"),
    ("backend/internal/runtimepostgres/account_deletion_reconfirm.go", "ReconfirmAccountDeletion"),
}
FUNCTION = re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\(", re.M)


def git(*arguments: str, repo: Path = ROOT, check: bool = True) -> str:
    result = subprocess.run(["git", "-C", str(repo), *arguments],
                            capture_output=True, text=True, encoding="latin1")
    if check and result.returncode:
        raise RuntimeError("git " + arguments[0] + ": " + result.stderr[-350:])
    return result.stdout.strip()


def blob(revision: str, path: str, *, repo: Path = ROOT) -> str | None:
    p = subprocess.run(["git", "-C", str(repo), "show", f"{revision}:{path}"],
                       capture_output=True, text=True, encoding="latin1")
    return p.stdout if p.returncode == 0 else None


def classify_absent(path: str, old_content: str, main_sha: str, *, repo: Path) -> dict:
    if path in RENAMED_SQL:
        target = RENAMED_SQL[path]
        current = blob(main_sha, target, repo=repo)
        if current is None:
            raise ValueError("Missing renumbered canonical SQL: " + target)
        if old_content.strip() == current.strip():
            status = "RENUMBERED_SAME_SQL"
        elif path.endswith("r2_r3_signed_payment_admission.sql") and all(
            anchor in current for anchor in (
                "apgic_live_consultation_payment_authorized",
                "apgic_live_join_signed_capture_insert_guard",
                "payment_webhook_receipts",
            )
        ):
            status = "RENUMBERED_STRONGER_SIGNED_PAYMENT_GUARD"
        else:
            raise ValueError("Renumbered SQL changed without approved semantic mapping: " + path)
        return {"path": path, "disposition": status, "canonical_path": target}
    if path in SUPERSEDED_R5:
        target = SUPERSEDED_R5[path]
        if blob(main_sha, target, repo=repo) is None:
            raise ValueError("Missing replacement R5 runtime: " + target)
        return {"path": path, "disposition": "REPLACED_BY_SINGLE_CANONICAL_R5",
                "canonical_path": target}
    if path == LEGACY_AUTH:
        workflow = blob(main_sha, ".github/workflows/ci.yml", repo=repo)
        spec = blob(main_sha, "apps/web/e2e/auth001-postgres.spec.ts", repo=repo)
        if workflow is None or spec is None or not all(
            s in workflow for s in (
                "npx playwright test e2e/auth001-postgres.spec.ts",
                "AUTH_CROSS_TENANT_DENY", "AUTH_TENANT_CONTEXT_DENIED",
                'test "$cross_count" -ge 1', 'test "$forged_count" -ge 1',
            )
        ) or "private-profile" not in spec:
            raise ValueError("Missing live browser/PostgreSQL replacement for AUTH-001")
        return {"path": path, "disposition": "REPLACED_BY_BROWSER_POSTGRES_AUTH001",
                "canonical_path": "apps/web/e2e/auth001-postgres.spec.ts"}
    if path in LEGACY_DOCS:
        return {"path": path, "disposition": "RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME",
                "canonical_path": None}
    raise ValueError("Unclassified absent archived file (manual review required): " + path)


def reconcile(*, repo: Path = ROOT, manifest: str = ARCHIVE,
              candidate_sha: str | None = None) -> dict:
    repo = repo.resolve()
    head = git("rev-parse", "HEAD", repo=repo)
    if candidate_sha is not None and candidate_sha != head:
        raise ValueError("Exact candidate SHA mismatch")
    if not SHA.fullmatch(head):
        raise ValueError("Cannot pin exact canonical commit")
    archive = json.loads((repo / manifest).read_text(encoding="utf-8"))
    branches = archive.get("branches")
    if not isinstance(branches, list) or len(branches) != 47 or archive.get("branch_count") != 47:
        raise ValueError("Expected all 47 archived source heads")
    baseline = archive.get("baseline_sha")
    if not isinstance(baseline, str) or not SHA.fullmatch(baseline):
        raise ValueError("Missing exact reconciliation baseline")

    for path, marker in REPLACEMENT_GUARDS.items():
        now = blob(head, path, repo=repo)
        if now is None or marker not in now:
            raise ValueError("R5 canonical replacement is absent: " + path)
    absent: dict[str, dict] = {}
    branches_report = []
    totals: Counter[str] = Counter()
    removed_exports = []
    for entry in branches:
        name, sha = entry.get("name"), entry.get("sha")
        if not isinstance(name, str) or not isinstance(sha, str) or not SHA.fullmatch(sha):
            raise ValueError("Invalid archived head")
        if subprocess.run(["git", "-C", str(repo), "merge-base", "--is-ancestor", sha, head],
                          capture_output=True).returncode:
            raise ValueError("Archived head not reachable from exact candidate: " + name)
        ancestor = git("merge-base", baseline, sha, repo=repo)
        paths = git("diff", "--name-only", ancestor, sha, repo=repo).splitlines()
        row = {"name": name, "head_sha": sha, "branch_base_sha": ancestor,
               "changed_paths": len(paths), "exact": 0, "revised": 0, "obsolete": 0}
        for path in paths:
            historical = blob(sha, path, repo=repo)
            if historical is None:  # Deleted on this branch, not a new feature.
                continue
            current = blob(head, path, repo=repo)
            if current is None:
                row["obsolete"] += 1
                # Classify once; a path may be shared between historical heads.
                absent.setdefault(path, classify_absent(path, historical, head, repo=repo))
            elif current == historical:
                row["exact"] += 1
            else:
                row["revised"] += 1
                if path.endswith(".go") and not path.endswith("_test.go"):
                    dropped = FUNCTION.findall(historical)
                    current_functions = set(FUNCTION.findall(current))
                    for symbol in dropped:
                        if symbol[:1].isupper() and symbol not in current_functions:
                            if (path, symbol) not in INTENTIONAL_R5_REPLACEMENTS:
                                removed_exports.append({"branch": name, "path": path,
                                                        "symbol": symbol})
        totals.update({"changed": len(paths), "exact": row["exact"], "revised": row["revised"],
                       "obsolete": row["obsolete"]})
        branches_report.append(row)
    if removed_exports:
        raise ValueError("Unexplained vanished Go APIs: " + str(removed_exports))
    return {
        "schema_version": "apgic-archived-functional-reconciliation-v1",
        "candidate_sha": head,
        "archive_baseline_sha": baseline,
        "archived_heads": len(branches_report),
        "files_examined_per_branch": totals["changed"],
        "identical_file_versions": totals["exact"],
        "subsequently_revised_file_versions": totals["revised"],
        "absent_historical_file_versions": totals["obsolete"],
        "distinct_absent_paths": len(absent),
        "absence_dispositions": sorted(absent.values(), key=lambda x: x["path"]),
        "unexplained_removed_go_apis": removed_exports,
        "branches": branches_report,
        "evidence_class": "SOURCE_RECONCILIATION_NOT_USER_JOURNEY_OR_PRODUCTION_PROOF",
        "production_release": False,
        "note": "Revised source files require regression and E2E verification; reachability or matching symbols alone never proves behavior.",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    report = reconcile(candidate_sha=args.candidate_sha)
    path = Path(args.output)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("ARCHIVED SOURCE RECONCILIATION: PASS",
          report["archived_heads"], "heads",
          report["files_examined_per_branch"], "changed versions",
          report["distinct_absent_paths"], "classified historic paths")
    print("NO RELEASE CLAIM; revised files still need user-journey and provider evidence.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
