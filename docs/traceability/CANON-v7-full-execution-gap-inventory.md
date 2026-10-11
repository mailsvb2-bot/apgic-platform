# Canon v7 — full execution gap inventory

## Purpose and invariant

**The approved v7 DOCX owns meaning.** The immutable
`canon/requirements/coverage.json` names all 345 sections and their explicit
requirement links. `canon/requirements/registry.yaml` contains the 109
execution requirements and their states. Neither source may be silently
changed by this analysis.

Prior CI validated known requirements but did not publish **all section-owner
only gaps** in a single audit artifact. Green CI could be mistaken for
completion of the entire v7 scope. This inventory makes that impossible.

## Automatic output

Run:

```bash
python tools/canon_execution_gap_inventory.py --output /tmp/apgic-v7-gaps.json
python tools/canon_execution_gap_inventory.py --strict-complete --output /tmp/apgic-v7-final-check.json
```

The first command always inventories every section and requirement,
records statuses and implementation/test/evidence references, and orders
remaining requirements with dependencies first. It does **not** claim that
an unmapped section has no code. The second command is the **final whole-TZ**
gate, which intentionally fails while any section has only owner-level
coverage, any requirement is not RELEASED, traceability inputs disagree, or
production evidence is empty.

All CI runs now publish a downloadable `canon-v7-gap-inventory-*` JSON
artifact and a human-readable GitHub Actions job summary. CI inventory
generation **must remain green even with incomplete requirements**, or no
one could use the inventory to guide incremental closure.

## Baseline findings: 2026-10-12 (main 357edf239f2c)

- 345 frozen canon sections.
- 151 explicitly linked to a named requirement.
- 194 have only an owner domain and need requirement-to-section analysis.
- 109 executable requirements in the registry: 27 VERIFIED and 82 IN_PROGRESS
  at analysis time, but **zero RELEASED**.
- One concrete linkage discrepancy to reconcile with an approved RFC:
  section 344 lists PAY-015 through PAY-020 in the frozen coverage file,
  whereas the current registry lists section 344 as a source only for
  PAY-015. Do **not** delete these links or alter v7 without governance.
- The production evidence index currently has zero records.

This is **traceability debt, not a measured 194 missing features**.
There are meaningful sections describing product mission, illustrative
examples and architectural constraints. Whether each should receive a
separate requirement, an existing requirement link or a governed exemption
requires reading the original DOCX section.

## Ordering work against total canon

1. Audit each OWNER_ONLY section with the original DOCX and choose the
   existing governing requirement or submit a canon RFC for any new,
   testable requirement/exemption.
2. Close missing canonical product journeys according to dependency order,
   prioritizing R0–R4 P0 and production blockers, not incidental green tests.
3. Attach code, tests, stage/production evidence, and accurate requirement
   status. Preserve a single owner of each state transition and readback.
4. Final strict gate only after release evidence, store/provider
   certification, and the *whole* canon are demonstrably complete.

## Next identified production blocker (not closed by this PR)

`demand.NewLiveServiceWithStoresAndSpecialists` starts with an empty
live catalog, while `Matches` and `Slots` still consult its
in-memory `catalog.candidates` and `catalog.slots`. Public discovery
reads real reviewed specialists from PostgreSQL, but discovery must not
fabricate prices or bookability. A genuine published offer, price,
availability, and durable hold must be connected to these flows before a
paying client can select a real specialist.

The inventory deliberately does not mark R1/R2 requirements complete
until this gap and the actual PSP/media journeys are closed.
