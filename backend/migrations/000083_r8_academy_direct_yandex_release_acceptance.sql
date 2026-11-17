-- R8 direct Yandex delivery release admission. The first-party APGIC gateway
-- is not a CDN_PROVIDER. Acceptance never grants an entitlement or completes
-- a lesson; it only authorizes publication after independently audited proof.
CREATE TABLE academy_video_direct_yandex_acceptances (
  id uuid PRIMARY KEY,
  tested_source_sha text NOT NULL CHECK (tested_source_sha ~ '^[0-9a-f]{40}$'),
  acceptance_sha256 text NOT NULL CHECK (acceptance_sha256 ~ '^[0-9a-f]{64}$'),
  acceptance_ref text NOT NULL CHECK (
    length(btrim(acceptance_ref)) BETWEEN 1 AND 2048
    AND position('://' in acceptance_ref)=0
  ),
  accepted_by uuid NOT NULL REFERENCES identities(id),
  step_up_audit_ref uuid NOT NULL UNIQUE REFERENCES audit_records(id),
  accepted_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  CHECK (expires_at > accepted_at)
);
CREATE UNIQUE INDEX academy_video_direct_yandex_acceptances_source_unique
  ON academy_video_direct_yandex_acceptances(tested_source_sha,acceptance_sha256);

CREATE FUNCTION academy_video_direct_yandex_acceptance_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
  auth_actor text;
  auth_action text;
  auth_scope text;
  auth_resource_ref text;
  auth_state jsonb;
  auth_reason text;
  auth_occurred_at timestamptz;
BEGIN
  SELECT actor_id,action,scope,resource_ref,new_state,reason,occurred_at
    INTO auth_actor,auth_action,auth_scope,auth_resource_ref,auth_state,auth_reason,auth_occurred_at
  FROM audit_records WHERE id=NEW.step_up_audit_ref;
  IF NOT FOUND
    OR auth_actor IS DISTINCT FROM NEW.accepted_by::text
    OR auth_action IS DISTINCT FROM 'authorization.decision'
    OR auth_scope IS DISTINCT FROM 'academy.video.direct.certify'
    OR auth_resource_ref IS DISTINCT FROM 'academy/direct-yandex'
    OR auth_state->>'decision' IS DISTINCT FROM 'ALLOW'
    OR auth_state->>'risk' IS DISTINCT FROM 'HIGH_RISK'
    OR auth_state->>'action' IS DISTINCT FROM 'academy.video.direct.certify'
    OR auth_state->>'tested_source_sha' IS DISTINCT FROM NEW.tested_source_sha
    OR auth_state->>'acceptance_sha256' IS DISTINCT FROM NEW.acceptance_sha256
    OR auth_state->>'acceptance_ref' IS DISTINCT FROM NEW.acceptance_ref
    OR auth_state->>'real_yandex_range' IS DISTINCT FROM 'PASS'
    OR auth_state->>'signed_learner_entitlement' IS DISTINCT FROM 'PASS'
    OR auth_state->>'committed_revocation' IS DISTINCT FROM 'PASS'
    OR auth_state->>'video_lesson_completion' IS DISTINCT FROM 'PASS'
    OR auth_state->>'payment_reconciliation' IS DISTINCT FROM 'PASS'
    OR btrim(COALESCE(auth_reason,''))=''
    OR NEW.accepted_at<auth_occurred_at
    OR NEW.accepted_at>auth_occurred_at+interval '5 minutes' THEN
    RAISE EXCEPTION 'direct Yandex publication requires fresh independent high-risk release acceptance';
  END IF;
  IF NEW.expires_at<=now() OR NEW.accepted_at>now() THEN
    RAISE EXCEPTION 'direct Yandex acceptance timestamps are invalid';
  END IF;
  RETURN NEW;
END;
$fn$;

CREATE TRIGGER academy_video_direct_yandex_acceptance_insert_guard
  BEFORE INSERT ON academy_video_direct_yandex_acceptances
  FOR EACH ROW EXECUTE FUNCTION academy_video_direct_yandex_acceptance_guard();
CREATE TRIGGER academy_video_direct_yandex_acceptance_immutable
  BEFORE UPDATE OR DELETE ON academy_video_direct_yandex_acceptances
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE FUNCTION academy_video_direct_yandex_acceptance_available(p_source_sha text)
RETURNS boolean LANGUAGE sql STABLE AS $fn$
  SELECT COALESCE(p_source_sha ~ '^[0-9a-f]{40}$',false) AND EXISTS (
    SELECT 1 FROM academy_video_direct_yandex_acceptances cert
    WHERE cert.tested_source_sha=p_source_sha AND cert.expires_at>now()
  );
$fn$;
