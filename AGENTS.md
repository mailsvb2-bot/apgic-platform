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
4. Other servers belong to other projects unless the user explicitly changes this boundary in the current conversation.
5. If `92.51.23.254` is unavailable, stop server-side work and report the failure. Do not fall back to another host.
6. GitHub-only APGIC work may continue without server access.
7. A change to this server boundary requires an explicit user instruction; it must never be inferred from repository history, another project's configuration, or available infrastructure.

This boundary is an operational safety rule, not a suggestion.
