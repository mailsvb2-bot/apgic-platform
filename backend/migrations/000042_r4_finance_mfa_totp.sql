-- R4 MFA: one provisioned TOTP factor per canonical APGIC Identity.
-- Enrollment is controlled by an out-of-band finance operator admin, never
-- by a public session. Secrets are AES-GCM encrypted with a distinct runtime
-- key. Attempts and monotonic steps are mutable operational security state,
-- while enrollment/verification decisions are appended to canonical audit.
CREATE TABLE finance_operator_mfa_factors (
  identity_id uuid PRIMARY KEY REFERENCES identities(id),
  encrypted_secret bytea NOT NULL CHECK (octet_length(encrypted_secret) >= 40),
  last_accepted_step bigint NOT NULL DEFAULT -1,
  failure_count integer NOT NULL DEFAULT 0 CHECK (failure_count BETWEEN 0 AND 5),
  locked_until timestamptz,
  provisioned_by text NOT NULL CHECK (btrim(provisioned_by) <> ''),
  approval_ref text NOT NULL CHECK (btrim(approval_ref) <> ''),
  provisioned_at timestamptz NOT NULL,
  revoked_at timestamptz,
  CHECK (last_accepted_step >= -1)
);
CREATE INDEX finance_operator_mfa_active_idx
  ON finance_operator_mfa_factors(identity_id)
  WHERE revoked_at IS NULL;
