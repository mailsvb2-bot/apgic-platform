# R0 CANON-001 — Launch Cut freeze evidence

APGIC-CANON-001 requires useful new ideas to stay outside the frozen R0–R4 Launch Cut unless a governed Requirement Change / RFC changes the Canon for an allowed blocker class.

## Executable proof

`tools/tests/test_canon_launch_freeze_e2e.py` exercises the real governance path rather than a helper in isolation:

1. creates an isolated copy of the repository;
2. injects a synthetic, unapproved R0 requirement into the working Requirement Registry;
3. runs the repository's real `tools/canon_lint.py`;
4. requires a non-zero exit and the frozen-set diagnostic `working registry requirement set differs from immutable baseline`.

The test therefore proves that ordinary repository changes cannot silently expand Launch Cut. The existing Requirement Change schema/template and `approved-rfcs.yaml` remain the governed mechanism for allowed semantic deltas.

## CI gate

The proof runs under the `Canon / architecture conformance` job through `python -m unittest discover -s tools/tests -p 'test_*.py'`.

APGIC-CANON-001 remains `IN_PROGRESS`: this evidence closes the repository-side E2E governance proof without asserting that every future organizational approval process has occurred.
