-- Versioned, append-only legal authority for external appearances and voices.
-- An arbitrary rights_evidence_ref on persona_catalog is NOT an approval.
CREATE TABLE persona_rights_reviews (
 persona_id uuid PRIMARY KEY REFERENCES persona_catalog(id),
 rights_holder_ref text NOT NULL CHECK (btrim(rights_holder_ref) <> ''),
 evidence_sha256 text NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
 reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
 text_use boolean NOT NULL DEFAULT false,
 voice_use boolean NOT NULL DEFAULT false,
 appearance_use boolean NOT NULL DEFAULT false,
 valid_until timestamptz NOT NULL,
 reviewed_at timestamptz NOT NULL DEFAULT now(),
 CHECK (text_use OR voice_use OR appearance_use)
);
CREATE TRIGGER persona_rights_reviews_append_only
 BEFORE UPDATE OR DELETE ON persona_rights_reviews
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TABLE persona_rights_revocations (
 id uuid PRIMARY KEY,
 persona_id uuid NOT NULL REFERENCES persona_rights_reviews(persona_id),
 reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
 revoked_by text NOT NULL CHECK (btrim(revoked_by) <> ''),
 revoked_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX persona_rights_revocations_persona_idx ON persona_rights_revocations(persona_id);
CREATE TRIGGER persona_rights_revocations_append_only
 BEFORE UPDATE OR DELETE ON persona_rights_revocations
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION apgic_avatar_rights_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' OR
    (TG_OP='UPDATE' AND OLD.state='PENDING' AND NEW.state='ACTIVE') THEN
  IF NOT EXISTS (
    SELECT 1 FROM persona_rights_reviews rr
    WHERE rr.persona_id=NEW.persona_id AND rr.valid_until>now()
    AND (NEW.mode='TEXT' AND rr.text_use
      OR NEW.mode='VOICE' AND rr.voice_use
      OR NEW.mode='VIDEO' AND rr.voice_use AND rr.appearance_use)
    AND NOT EXISTS (SELECT 1 FROM persona_rights_revocations x WHERE x.persona_id=rr.persona_id)
  ) THEN
   RAISE EXCEPTION 'verified, unexpired voice and appearance usage rights required';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER avatar_rights_guard BEFORE INSERT OR UPDATE ON avatar_sessions
 FOR EACH ROW EXECUTE FUNCTION apgic_avatar_rights_guard();
