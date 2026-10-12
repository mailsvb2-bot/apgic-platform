\set ON_ERROR_STOP on

-- Persona R3 SQL evidence. All identities and connectors are dedicated CI
-- fixtures, never production or donor application data.
INSERT INTO identities (id) VALUES ('00000000-0000-0000-0000-00000000d201')
ON CONFLICT (id) DO NOTHING;
INSERT INTO connector_instances (id,capability_class,provider_kind,status,config_ref)
VALUES ('00000000-0000-0000-0000-00000000d211','PERSONA_PROVIDER',
'CI_PERSONA_FAKE','ACTIVE','secret://ci/persona-contract');

INSERT INTO persona_model_deployments (
 policy_version,connector_instance_id,provider_kind,model_id,model_version,
 config_version,eval_suite_version,eval_evidence_ref,safety_policy_version,
 budget_policy_version,starts_per_identity_hour,max_concurrent_per_identity,
 state,approved_at
) VALUES (
 'ci-persona-policy-v1','00000000-0000-0000-0000-00000000d211',
 'CI_PERSONA_FAKE','test-only-model','ci-1',
 'config-ci-v1','eval-ci-v1','ci://not-a-production-approval',
 'safety-ci-v1','budget-ci-v1',3,1,'ACTIVE',now()
);
INSERT INTO persona_catalog (
 id,label,language,voice_profile,visual_profile,supports_video,supports_voice,
 supports_text,rights_evidence_ref,state
) VALUES (
 '00000000-0000-0000-0000-00000000d221',
 'CI synthetic avatar','ru','voice-ci','visual-ci',true,true,true,
 'ci://synthetic-avatar-rights','PUBLISHED'
);

-- No raw media or conversations may silently grow into business truth.
DO $$
BEGIN
 IF EXISTS(
  SELECT 1 FROM information_schema.columns
  WHERE table_schema='public'
  AND table_name IN('avatar_sessions','avatar_session_facts','persona_model_deployments')
  AND column_name IN ('transcript','raw_transcript','audio','raw_audio','video','raw_video',
   'messages','conversation','diagnosis','prompt','secret','provider_token')
 ) THEN RAISE EXCEPTION 'raw persona content or secrets in canonical business schema'; END IF;
END $$;

-- An arbitrary policy string cannot authorize a provider/model.
DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  INSERT INTO avatar_sessions (
   id,user_id,persona_id,provider_instance_id,idempotency_key,session_purpose,
   privacy_mode,mode,style,language,voice_profile,visual_profile,state,policy_version
  ) VALUES (
   '00000000-0000-0000-0000-00000000d229',
   '00000000-0000-0000-0000-00000000d201',
   '00000000-0000-0000-0000-00000000d221',
   '00000000-0000-0000-0000-00000000d211','ci-unregistered',
   'LISTEN','PRIVATE_EPHEMERAL','VOICE','CALM','ru','voice-ci','visual-ci',
   'PENDING','unreviewed-model'
  );
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'unreviewed persona model was admitted'; END IF;
END $$;

-- The governed start is valid and preserves complete immutable context.
INSERT INTO avatar_sessions (
 id,user_id,persona_id,provider_instance_id,idempotency_key,session_purpose,
 privacy_mode,mode,style,language,voice_profile,visual_profile,state,policy_version
) VALUES (
 '00000000-0000-0000-0000-00000000d231',
 '00000000-0000-0000-0000-00000000d201',
 '00000000-0000-0000-0000-00000000d221',
 '00000000-0000-0000-0000-00000000d211','ci-session-one',
 'LISTEN','PRIVATE_EPHEMERAL','VOICE','CALM','ru','voice-ci','visual-ci',
 'PENDING','ci-persona-policy-v1'
);
UPDATE avatar_sessions SET state='ACTIVE',started_at=now(),
 provider_session_ref='ci-only/provider-session' WHERE id='00000000-0000-0000-0000-00000000d231';
UPDATE avatar_sessions SET safety_state='ESCALATED'
 WHERE id='00000000-0000-0000-0000-00000000d231';

DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  UPDATE avatar_sessions SET safety_state='NORMAL'
  WHERE id='00000000-0000-0000-0000-00000000d231';
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'safety escalation was reversible'; END IF;
END $$;

DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  UPDATE avatar_sessions SET style='BUSINESS'
  WHERE id='00000000-0000-0000-0000-00000000d231';
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'AI style/policy snapshot was mutable'; END IF;
END $$;

-- Client-created safety/handoff operational facts are append-only evidence.
INSERT INTO avatar_session_facts (id,session_id,actor_id,event_type,reason_code,policy_version)
VALUES ('00000000-0000-0000-0000-00000000d241',
 '00000000-0000-0000-0000-00000000d231',
 '00000000-0000-0000-0000-00000000d201',
 'SAFETY_ESCALATED','CI_SAFETY_TRIGGER','ci-persona-policy-v1');

DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  DELETE FROM avatar_session_facts WHERE id='00000000-0000-0000-0000-00000000d241';
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'persona audit evidence could be deleted'; END IF;
END $$;

UPDATE avatar_sessions SET state='ENDED',ended_at=now()
WHERE id='00000000-0000-0000-0000-00000000d231';

DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  UPDATE avatar_sessions SET state='ACTIVE'
  WHERE id='00000000-0000-0000-0000-00000000d231';
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'ended session was resurrected'; END IF;
END $$;

-- A safety/operator kill-switch is permanent for this reviewed policy.
INSERT INTO persona_model_disable_decisions (
 id,policy_version,reason_code,reviewer_ref
) VALUES (
 '00000000-0000-0000-0000-00000000d251',
 'ci-persona-policy-v1','CI_OPERATOR_PAUSED','ci://operator'
);
DO $$
DECLARE blocked boolean := false;
BEGIN
 BEGIN
  INSERT INTO avatar_sessions (
   id,user_id,persona_id,provider_instance_id,idempotency_key,session_purpose,
   privacy_mode,mode,style,language,voice_profile,visual_profile,state,policy_version
  ) VALUES (
   '00000000-0000-0000-0000-00000000d232',
   '00000000-0000-0000-0000-00000000d201',
   '00000000-0000-0000-0000-00000000d221',
   '00000000-0000-0000-0000-00000000d211','ci-post-kill-switch',
   'LISTEN','PRIVATE_EPHEMERAL','VOICE','CALM','ru','voice-ci','visual-ci',
   'PENDING','ci-persona-policy-v1'
  );
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'disabled AI policy started a new persona session'; END IF;
END $$;

SELECT 'PERSONA_COMPANION_DB_INVARIANTS_PASS' AS result;
