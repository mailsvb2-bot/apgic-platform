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


## 2026-10-07 deployed v2 proof and backup-gap finding

Candidate `3ebce6a86142bf051e6d22163177142e925ae797` was deployed on the authorized APGIC staging host `92.51.23.254`.

The current maintenance units match the repository and include `RestrictAddressFamilies=AF_UNIX AF_NETLINK`. A fresh backup executed through `apgic-staging-backup.service` completed with `status=0/SUCCESS`, then `apgic-staging-restore-verify.service` restored that backup into an isolated temporary database and completed with `status=0/SUCCESS`.

Recorded proof: `canon/evidence/staging-restore-20261007T181635Z.json`.

Observed measurements and probes:

- measured backup RPO observation: 11 seconds;
- measured restore RTO: 1449 ms;
- source/restored table count: 60 / 60;
- restored identities: 36;
- restored organizations: 28;
- audit, ledger, direction-delete, product-owner, booking-transition and order-append-only guards: exactly 1 each;
- `business_probes_passed=true`;
- `integrity_probes_passed=true`;
- `production_evidence=false`.

The operational journal also showed two real scheduled backup failures on 2026-10-05 and 2026-10-06. The old installed systemd sandbox did not allow the netlink access required by `assert-authorized-host.sh`, so the correct host failed closed as if it were unauthorized. Repository commit `579af6c46ad19bfc5bed00f3956f9be7050c8e0f` added `AF_NETLINK` and deploy-time reconciliation of the maintenance units. The scheduled 2026-10-07 backup succeeded with the reconciled unit, and the fresh backup/restore proof above confirms the corrected path end-to-end.

APGIC-DR-001 remains `IN_PROGRESS`: this evidence proves the deployed staging DR path, but the acceptance starts from a production backup and the evidence contract deliberately records `production_evidence=false`.
