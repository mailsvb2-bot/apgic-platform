# APGIC agent and infrastructure boundary

## Authoritative server

The only server authorized for APGIC runtime, deployment, maintenance, diagnostics, cleanup, database work, service restarts, logs, backups, or any other server-side APGIC operation is:

- IPv4: `92.51.23.254`
- APGIC release root: `/opt/apgic`
- Current release symlink: `/opt/apgic/current`
- Runtime configuration/secrets: `/etc/apgic/staging.env`

## Hard isolation rule

All AI agents, chats, coding sessions, operators, automation, and deployment work for this repository MUST obey the following boundary:

1. For APGIC server-side work, connect to and modify only `92.51.23.254`.
2. MUST NOT connect to, inspect, scan, clean, deploy to, restart services on, copy files to/from, or otherwise modify any other server on behalf of APGIC.
3. The presence of other reachable hosts, SSH credentials, Remote Desktop Commander devices, saved infrastructure references, or historical deployment information does not authorize their use for APGIC.
4. **Remote Desktop Commander hard rule:** for APGIC, every remote command/file/service/database/log operation MUST target only the connected device whose IPv4 is exactly `92.51.23.254`. Do not run diagnostics, cleanup, inspection, `hostname`, `df`, `docker`, `systemctl`, file reads, or any other APGIC command on another Desktop Commander device. If the authorized device cannot be positively identified as `92.51.23.254`, do not execute the remote operation.
5. Other servers belong to other projects unless the user explicitly changes this boundary in the current conversation.
6. If `92.51.23.254` is unavailable, stop server-side work and report the failure. Do not fall back to another host.
7. GitHub-only APGIC work may continue without server access.
8. A change to this server boundary requires an explicit user instruction; it must never be inferred from repository history, another project's configuration, or available infrastructure.

This boundary is an operational safety rule, not a suggestion.


## GitHub-first source of truth

GitHub is the authoritative source of APGIC code.

1. All code changes MUST exist in the `mailsvb2-bot/apgic-platform` GitHub repository before they are deployed to `92.51.23.254`.
2. Server-only code, uncommitted production/staging patches, ad-hoc binaries as the only copy of logic, and edits made directly under `/opt/apgic/current` as a source of truth are prohibited.
3. The server may contain generated build artifacts, caches, logs, backups, temporary test files, and rollback binaries, but none of these may be the only copy of application source code.
4. Deployment MUST identify a Git commit SHA and the runtime must be traceable back to that SHA.
5. If code exists only on a server or in `/tmp`, it is not considered canonical APGIC code. It must first be recovered, reviewed, committed, and pushed to GitHub before deployment.
6. Temporary worktrees used during development are disposable only after any intended changes are confirmed present in GitHub.
