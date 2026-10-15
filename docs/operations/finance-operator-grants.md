# APGIC finance-operator grant administration

Finance discrepancy resolution is a high-risk operation. There is no self-service grant endpoint. Provisioning and revocation are explicit, audited operations executed with a controlled database credential and an approved change record.

## Grant

Run from a trusted operator environment against the intended APGIC database:

```sh
APGIC_DATABASE_URL='(secret from the approved environment)' go run ./cmd/finance-operator-admin grant \
  --identity-id '<APGIC Identity UUID>' \
  --approved-by '<control-plane approver / change actor>' \
  --approval-ref '<approved ticket or change record>'
```

The command requires PostgreSQL readiness, refuses a second active grant, writes the append-only grant and audit record in one transaction, and prints the generated grant UUID. Store that UUID in the change record.

## Revoke

```sh
APGIC_DATABASE_URL='(secret from the approved environment)' go run ./cmd/finance-operator-admin revoke \
  --grant-id '<grant UUID>' \
  --approved-by '<control-plane approver / change actor>' \
  --reason 'At least ten characters explaining the revocation'
```

Revocation is a separate append-only fact and an audit record; the grant itself is never updated or deleted. A repeated revocation fails closed.

## Controls and limitations

- Never put the database URL or credentials in source control, shell history, tickets, or logs.
- The tool does not authenticate the human named in `--approved-by`; the OS/operator credential and external change approval remain the trust boundary.
- Granting this capability does not satisfy step-up authentication. The resolution API still requires a fresh session-bound step-up assertion.
- The web operations page is read-only until a real WebAuthn/step-up ceremony is connected. Do not bypass this requirement.
