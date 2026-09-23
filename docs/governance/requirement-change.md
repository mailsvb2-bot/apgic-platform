# Requirement Change / RFC governance

APGIC implementation never silently changes the meaning of the active Canon.

A change to immutable requirement semantics, dependency graph, release profile, Launch Cut, canonical ownership, acceptance criteria, or source coverage requires a Requirement Change / RFC before implementation treats the new meaning as authoritative.

For R0–R4, a useful new feature remains post-launch/backlog unless the change is required to resolve a legal blocker, security blocker, financial-integrity blocker, or missing dependency that prevents the approved launch scope from being implemented safely.

The repository enforces this in two layers:

1. `canon/baseline/` preserves the supplied v7 FINAL machine-readable baseline.
2. `tools/canon_lint.py` rejects working-registry semantic drift and requirement-set drift.

The GitHub Requirement Change / RFC issue template records the reason class, exact affected Requirement IDs, impact, evidence, rollout, and acceptance changes. Approval of an issue alone does not mutate the Canon; the approved canonical artifacts must be updated through the governed change and then pass conformance CI.
