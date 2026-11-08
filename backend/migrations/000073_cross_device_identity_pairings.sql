-- One canonical identity may be independently authenticated on several devices.
-- The pairing handshake never copies an existing browser cookie or merges identities.
-- The target device keeps a secret proof; the source device approves only its code.
CREATE TABLE account_device_pairings (
    id uuid PRIMARY KEY,
    code_digest text NOT NULL UNIQUE CHECK (length(code_digest) = 64),
    proof_digest text NOT NULL CHECK (length(proof_digest) = 64),
    identity_id uuid REFERENCES identities(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    approved_at timestamptz,
    consumed_at timestamptz,
    CHECK (expires_at > created_at),
    CHECK ((identity_id IS NULL) = (approved_at IS NULL)),
    CHECK (consumed_at IS NULL OR approved_at IS NOT NULL)
);
CREATE INDEX account_device_pairings_expiry_idx ON account_device_pairings(expires_at);
REVOKE ALL ON account_device_pairings FROM PUBLIC;

-- Only sessions minted by explicit device pairing are managed here.
-- Legacy client sessions retain their current behavior during migration.
CREATE TABLE account_paired_sessions (
    id uuid PRIMARY KEY,
    identity_id uuid NOT NULL REFERENCES identities(id),
    cookie_digest text NOT NULL UNIQUE CHECK (length(cookie_digest) = 64),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CHECK (expires_at > created_at)
);
CREATE INDEX account_paired_sessions_identity_idx ON account_paired_sessions(identity_id, created_at DESC);
REVOKE ALL ON account_paired_sessions FROM PUBLIC;
