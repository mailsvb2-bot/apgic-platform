-- APGIC business truth only. No transcript/audio/video/prompt/provider token
-- is stored in these tables. Provider runtime state remains external.
-- Approved, versioned model/eval/safety and budget evidence; zero default
-- deployments means no AI starts. No shadow model selection is allowed.
CREATE TABLE persona_model_deployments (
 policy_version text PRIMARY KEY CHECK (btrim(policy_version) <> ''),
 connector_instance_id uuid NOT NULL REFERENCES connector_instances(id),
 provider_kind text NOT NULL CHECK (btrim(provider_kind) <> ''),
 model_id text NOT NULL CHECK (btrim(model_id) <> ''),
 model_version text NOT NULL CHECK (btrim(model_version) <> ''),
 config_version text NOT NULL CHECK (btrim(config_version) <> ''),
 eval_suite_version text NOT NULL CHECK (btrim(eval_suite_version) <> ''),
 eval_evidence_ref text NOT NULL CHECK (btrim(eval_evidence_ref) <> ''),
 safety_policy_version text NOT NULL CHECK (btrim(safety_policy_version) <> ''),
 budget_policy_version text NOT NULL CHECK (btrim(budget_policy_version) <> ''),
 starts_per_identity_hour integer NOT NULL CHECK (starts_per_identity_hour BETWEEN 1 AND 60),
 max_concurrent_per_identity integer NOT NULL CHECK (max_concurrent_per_identity BETWEEN 1 AND 5),
 state text NOT NULL CHECK (state IN ('ACTIVE','PAUSED','REVOKED')),
 approved_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK (approved_at <= created_at + interval '5 minutes')
);
CREATE INDEX persona_model_deployments_connector_state_idx ON persona_model_deployments(connector_instance_id,state);
CREATE TRIGGER persona_model_deployments_append_only
 BEFORE UPDATE OR DELETE ON persona_model_deployments
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
-- Append-only kill-switch decisions. A paused/revoked model never starts new
-- sessions; only a NEW policy_version can re-enable a reviewed deployment.
CREATE TABLE persona_model_disable_decisions (
 id uuid PRIMARY KEY,
 policy_version text NOT NULL REFERENCES persona_model_deployments(policy_version),
 reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
 decided_at timestamptz NOT NULL DEFAULT now(),
 reviewer_ref text NOT NULL CHECK (btrim(reviewer_ref) <> '')
);
CREATE TRIGGER persona_model_disable_decisions_append_only
 BEFORE UPDATE OR DELETE ON persona_model_disable_decisions
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TABLE persona_catalog (
 id uuid PRIMARY KEY,
 label text NOT NULL CHECK (btrim(label) <> ''),
 language text NOT NULL CHECK (language ~ '^[a-z]{2}(-[A-Z]{2})?$'),
 voice_profile text NOT NULL CHECK (btrim(voice_profile) <> ''),
 visual_profile text NOT NULL CHECK (btrim(visual_profile) <> ''),
 supports_video boolean NOT NULL,
 supports_voice boolean NOT NULL,
 supports_text boolean NOT NULL,
 rights_evidence_ref text NOT NULL CHECK (btrim(rights_evidence_ref) <> ''),
 state text NOT NULL CHECK (state IN ('DRAFT','PUBLISHED','PAUSED','RETIRED')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (supports_video OR supports_voice OR supports_text)
);
CREATE TABLE avatar_sessions (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES identities(id),
 persona_id uuid NOT NULL REFERENCES persona_catalog(id),
 provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
 idempotency_key text NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 8 AND 128),
 session_purpose text NOT NULL CHECK (session_purpose IN ('LISTEN','ORGANIZE_THOUGHTS','PREPARE_HUMAN','WAIT_FOR_HUMAN','NAVIGATE')),
 privacy_mode text NOT NULL DEFAULT 'PRIVATE_EPHEMERAL' CHECK (privacy_mode='PRIVATE_EPHEMERAL'),
 mode text NOT NULL CHECK (mode IN ('VIDEO','VOICE','TEXT')),
 style text NOT NULL CHECK (style IN ('CALM','NEUTRAL','BUSINESS','WARM')),
 language text NOT NULL,
 voice_profile text NOT NULL,
 visual_profile text NOT NULL,
 state text NOT NULL CHECK (state IN ('PENDING','ACTIVE','FAILED','ENDED')),
 safety_state text NOT NULL DEFAULT 'NORMAL' CHECK (safety_state IN ('NORMAL','ESCALATED')),
 provider_session_ref text,
 created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 ended_at timestamptz,
 handoff_requested_at timestamptz,
 waitlist_requested_at timestamptz,
 consented_summary_id uuid,
 policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
 UNIQUE (user_id,idempotency_key),
 CHECK ((state='ACTIVE' AND started_at IS NOT NULL AND provider_session_ref IS NOT NULL) OR state<>'ACTIVE'),
 CHECK ((state='ENDED' AND ended_at IS NOT NULL) OR state<>'ENDED')
);
CREATE INDEX avatar_sessions_user_created_idx ON avatar_sessions(user_id,created_at DESC);
CREATE TABLE avatar_session_facts (
 id uuid PRIMARY KEY,
 session_id uuid NOT NULL REFERENCES avatar_sessions(id),
 actor_id uuid REFERENCES identities(id),
 event_type text NOT NULL CHECK(event_type IN ('CREATED','ACTIVATED','FAILED','CLOSED','HANDOFF_REQUESTED','SAFETY_ESCALATED')),
 reason_code text NOT NULL CHECK(btrim(reason_code) <> ''),
 policy_version text NOT NULL CHECK(btrim(policy_version) <> ''),
 occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER avatar_session_facts_append_only
 BEFORE UPDATE OR DELETE ON avatar_session_facts
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_avatar_session_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cap text; status text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'avatar session delete forbidden'; END IF;
 IF TG_OP='INSERT' THEN
  SELECT capability_class, ci.status INTO cap,status FROM connector_instances ci WHERE ci.id=NEW.provider_instance_id;
  IF cap IS DISTINCT FROM 'PERSONA_PROVIDER' OR status NOT IN ('ACTIVE','DEGRADED') THEN
   RAISE EXCEPTION 'active PersonaProvider capability required';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM persona_catalog WHERE id=NEW.persona_id AND state='PUBLISHED') THEN
   RAISE EXCEPTION 'published rights-cleared persona required';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM persona_model_deployments p WHERE p.policy_version=NEW.policy_version
   AND p.connector_instance_id=NEW.provider_instance_id AND p.state='ACTIVE'
   AND NOT EXISTS(SELECT 1 FROM persona_model_disable_decisions d WHERE d.policy_version=p.policy_version)) THEN
   RAISE EXCEPTION 'approved active persona model policy required';
  END IF;
  IF NEW.state <> 'PENDING' OR NEW.started_at IS NOT NULL OR NEW.provider_session_ref IS NOT NULL
   OR NEW.ended_at IS NOT NULL THEN RAISE EXCEPTION 'new avatar session must be pending'; END IF;
 ELSE
  IF (NEW.id,NEW.user_id,NEW.persona_id,NEW.provider_instance_id,NEW.idempotency_key,NEW.session_purpose,
   NEW.privacy_mode,NEW.style,NEW.language,NEW.voice_profile,NEW.visual_profile,NEW.created_at,NEW.policy_version)
     IS DISTINCT FROM
     (OLD.id,OLD.user_id,OLD.persona_id,OLD.provider_instance_id,OLD.idempotency_key,OLD.session_purpose,
   OLD.privacy_mode,OLD.style,OLD.language,OLD.voice_profile,OLD.visual_profile,OLD.created_at,OLD.policy_version)
  THEN RAISE EXCEPTION 'immutable avatar session identity/context';END IF;
  -- Provider downgrades are allowed only to catalog-supported, rights-cleared modes.
  IF OLD.state='PENDING' AND NEW.state='ACTIVE' AND NOT EXISTS (
   SELECT 1 FROM persona_catalog p WHERE p.id=NEW.persona_id
    AND ((NEW.mode='VIDEO' AND p.supports_video)
     OR (NEW.mode='VOICE' AND p.supports_voice)
     OR (NEW.mode='TEXT' AND p.supports_text))
  ) THEN RAISE EXCEPTION 'unsupported persona transport mode'; END IF;
  IF OLD.state='PENDING' AND NEW.state NOT IN ('ACTIVE','FAILED') OR
     OLD.state='ACTIVE' AND NEW.state <> 'ENDED' OR
     OLD.state IN ('FAILED','ENDED') AND NEW.state <> OLD.state
  THEN
   IF NEW.state IS DISTINCT FROM OLD.state THEN RAISE EXCEPTION 'invalid avatar session transition';END IF;
  END IF;
  IF OLD.handoff_requested_at IS NOT NULL AND NEW.handoff_requested_at IS DISTINCT FROM OLD.handoff_requested_at
   OR OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at
   OR OLD.ended_at IS NOT NULL AND NEW.ended_at IS DISTINCT FROM OLD.ended_at
  THEN RAISE EXCEPTION 'avatar evidence cannot be rewritten';END IF;
  IF OLD.safety_state='ESCALATED' AND NEW.safety_state<>'ESCALATED' THEN RAISE EXCEPTION 'safety escalation irreversible';END IF;
 END IF;
 RETURN NEW;
END;$$;
CREATE TRIGGER avatar_session_guard BEFORE INSERT OR UPDATE OR DELETE ON avatar_sessions
 FOR EACH ROW EXECUTE FUNCTION apgic_avatar_session_guard();

