# Restore drill evidence contract

APGIC-DR-001 is satisfied only by a completed, isolated restore drill with integrity/business probes and measured RPO/RTO evidence.

A backup job or successful snapshot alone is insufficient. Future production CI/release tooling must attach immutable evidence identifying source backup, target environment, start/end timestamps, measured RPO/RTO, schema version and probe results.
