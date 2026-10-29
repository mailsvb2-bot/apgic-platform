-- Dedicated client deletion reauthentication, isolated from finance-operator MFA.
-- Enrolment through a session cannot immediately authorize irreversible erasure.
CREATE TABLE client_deletion_mfa_factors (
  identity_id uuid PRIMARY KEY REFERENCES identities(id),
  encrypted_secret bytea NOT NULL CHECK (octet_length(encrypted_secret) >= 40),
  enrolled_at timestamptz NOT NULL,
  ready_at timestamptz NOT NULL,
  last_accepted_step bigint NOT NULL DEFAULT -1,
  failure_count integer NOT NULL DEFAULT 0 CHECK (failure_count BETWEEN 0 AND 5),
  locked_until timestamptz,
  revoked_at timestamptz,
  CHECK (ready_at >= enrolled_at + interval '24 hours'),
  CHECK (last_accepted_step >= -1)
);

-- Factor secrets are encrypted with a separately managed key. Never persist
-- OTP plaintext, never allow a public API to change existing provisioned seeds.
CREATE INDEX client_deletion_mfa_ready_idx
ON client_deletion_mfa_factors(ready_at) WHERE revoked_at IS NULL;
