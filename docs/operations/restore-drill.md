# Restore drill evidence contract

APGIC-DR-001 is satisfied only by a completed, isolated restore drill with integrity/business probes and measured RPO/RTO evidence.

A backup job or successful snapshot alone is insufficient. Future production CI/release tooling must attach immutable evidence identifying source backup, target environment, start/end timestamps, measured RPO/RTO, schema version and probe results.


## Staging scheduled restore proof

The staging host runs a separate scheduled restore verification against the latest protected backup. The verifier must restore into an isolated temporary database; it must never apply a backup over the live staging database.

Historical `staging-restore-evidence-v1` files remain immutable and prove backup identity, measured backup age/RPO, restore RTO, table-count parity and required-table presence.

New drills emit `staging-restore-evidence-v2`. In addition to the v1 measurements, v2 fails closed unless:

- restored `identities` contains non-zero business truth;
- restored `organizations` contains non-zero business truth;
- the restored database contains exactly one canonical append-only audit trigger;
- the restored database contains exactly one canonical append-only ledger trigger;
- Organization direction hard-delete protection survives restore;
- Product owner integrity protection survives restore;
- Booking transition protection survives restore;
- Order append-only protection survives restore.

The evidence records restored business counts, individual integrity probe counts, and explicit `business_probes_passed=true` / `integrity_probes_passed=true` flags.

Staging evidence remains `production_evidence=false`. It improves pre-production disaster-recovery proof but must not be relabeled as a production restore drill.
