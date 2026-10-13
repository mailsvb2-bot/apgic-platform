\set ON_ERROR_STOP on
-- Positive full R6 transaction proof using canonical specialization,
-- published organization-owned Product, qualification, offer and signed
-- intent ownership semantics. Roll back synthetic proof identities and money.
BEGIN;
INSERT INTO identities (id) VALUES
 ('00000000-0000-0000-0000-00000000a901'),
 ('00000000-0000-0000-0000-00000000a902'),
 ('00000000-0000-0000-0000-00000000a903'),
 ('00000000-0000-0000-0000-00000000a904'),
 ('00000000-0000-0000-0000-00000000a905');

INSERT INTO organizations (id,name,status)
VALUES ('00000000-0000-0000-0000-00000000a911','R6 eligibility fixture','ACTIVE');
INSERT INTO organization_memberships (organization_id,identity_id,status)
VALUES ('00000000-0000-0000-0000-00000000a911','00000000-0000-0000-0000-00000000a901','ACTIVE');
INSERT INTO organization_ownerships (organization_id,identity_id,status)
VALUES ('00000000-0000-0000-0000-00000000a911','00000000-0000-0000-0000-00000000a901','ACTIVE');
INSERT INTO organization_directions (id,organization_id,name,status)
VALUES ('00000000-0000-0000-0000-00000000a912','00000000-0000-0000-0000-00000000a911','R6 verified consultation','ACTIVE');

INSERT INTO products (
 id,owner_type,owner_id,commercial_owner_ref,revenue_beneficiary_ref,
 organization_direction_id,author_refs,name,status,published_at
) VALUES (
 '00000000-0000-0000-0000-00000000a913','ORGANIZATION',
 '00000000-0000-0000-0000-00000000a911',
 'organization/00000000-0000-0000-0000-00000000a911',
 'identity/00000000-0000-0000-0000-00000000a901',
 '00000000-0000-0000-0000-00000000a912',
 ARRAY['identity/00000000-0000-0000-0000-00000000a901'],
 'Evidence backed counselling','PUBLISHED',now()
);

INSERT INTO specialist_profiles
 (id,identity_id,display_name,profession_code,profile_complete,review_state)
VALUES (
 '00000000-0000-0000-0000-00000000a914',
 '00000000-0000-0000-0000-00000000a901',
 'R6 verified qualified practitioner','PSYCHOLOGIST',true,'APPROVED'
);
INSERT INTO specialist_capabilities
 (specialist_id,topic_id,evidence_state,verification_state,evidence_refs,verified_at)
VALUES ('00000000-0000-0000-0000-00000000a914','anxiety',
 'APGIC_VERIFIED','ACTIVE',ARRAY['evidence:r6'],now());
INSERT INTO qualification_evaluations (
 id,specialist_id,topic_id,jurisdiction,service_format,age_group,
 decision,policy_version,reason_codes,evidence_refs,
 capability_evidence_state,capability_verification_state,evaluated_at
) VALUES (
 '00000000-0000-0000-0000-00000000a915',
 '00000000-0000-0000-0000-00000000a914',
 'anxiety','RU','ONLINE','ADULT','ELIGIBLE','qualification-v1',
 ARRAY['QUAL_ELIGIBLE'],ARRAY['evidence:r6'],
 'APGIC_VERIFIED','ACTIVE',now()
);
INSERT INTO specialist_publications
 (id,specialist_id,topic_id,qualification_evaluation_id,state,reason_code,published_at)
VALUES (
 '00000000-0000-0000-0000-00000000a916',
 '00000000-0000-0000-0000-00000000a914',
 'anxiety','00000000-0000-0000-0000-00000000a915',
 'ACTIVE','PUBLISH_ALLOWED',now()
);
INSERT INTO specialist_bookable_offers
 (id,specialist_id,specialist_identity_id,product_id,topic_id,price_minor,currency,state)
VALUES (
 '00000000-0000-0000-0000-00000000a917',
 '00000000-0000-0000-0000-00000000a914',
 '00000000-0000-0000-0000-00000000a901',
 '00000000-0000-0000-0000-00000000a913','anxiety',190000,'RUB','ACTIVE'
);
INSERT INTO help_intents
 (id,identity_id,free_text,topics,goals,context,state,confirmed_at)
VALUES
 ('00000000-0000-0000-0000-00000000a921',
  '00000000-0000-0000-0000-00000000a902','Anxiety help',
  ARRAY['anxiety'],ARRAY[]::text[],
  '{"format":"ONLINE","language":"ru","jurisdiction":"RU","age_group":"ADULT"}'::jsonb,
  'CONFIRMED',now()),
 ('00000000-0000-0000-0000-00000000a922',
  '00000000-0000-0000-0000-00000000a903','Second client anxiety help',
  ARRAY['anxiety'],ARRAY[]::text[],
  '{"format":"ONLINE","language":"ru","jurisdiction":"RU","age_group":"ADULT"}'::jsonb,
  'CONFIRMED',now()),
 ('00000000-0000-0000-0000-00000000a923',
  '00000000-0000-0000-0000-00000000a904','Third client anxiety help',
  ARRAY['anxiety'],ARRAY[]::text[],
  '{"format":"ONLINE","language":"ru","jurisdiction":"RU","age_group":"ADULT"}'::jsonb,
  'CONFIRMED',now()),
 ('00000000-0000-0000-0000-00000000a924',
  '00000000-0000-0000-0000-00000000a905','Fourth client requalification race',
  ARRAY['anxiety'],ARRAY[]::text[],
  '{"format":"ONLINE","language":"ru","jurisdiction":"RU","age_group":"ADULT"}'::jsonb,
  'CONFIRMED',now());

DO $$
DECLARE outcome text;
BEGIN
  IF NOT apgic_live_offer_valid('00000000-0000-0000-0000-00000000a917') THEN
    RAISE EXCEPTION 'R6 fixture did not pass real canonical Product/Qualification gate';
  END IF;
  outcome := apgic_instant_enable(
   '00000000-0000-0000-0000-00000000a901',
   '00000000-0000-0000-0000-00000000a917',true,now());
  IF outcome <> 'DUTY_AVAILABLE_NOW' THEN
    RAISE EXCEPTION 'R6 available specialist could not activate: %',outcome;
  END IF;
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a941',
   '00000000-0000-0000-0000-00000000a902',
   '00000000-0000-0000-0000-00000000a921',now());
  IF outcome <> 'DUTY_OFFERED' THEN
    RAISE EXCEPTION 'R6 verified client matching failed: %',outcome;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM instant_dispatches
      WHERE id='00000000-0000-0000-0000-00000000a931'
      AND state='OFFERED' AND price_minor=190000 AND currency='RUB'
      AND specialist_identity_id='00000000-0000-0000-0000-00000000a901') THEN
    RAISE EXCEPTION 'R6 economic proposal lost specialist ownership or canonical price';
  END IF;
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a932',
   '00000000-0000-0000-0000-00000000a941',
   '00000000-0000-0000-0000-00000000a902',
   '00000000-0000-0000-0000-00000000a921',now());
  IF outcome <> 'DUTY_IDEMPOTENT_REPLAY' THEN
    RAISE EXCEPTION 'R6 duplicate request not idempotent: %',outcome;
  END IF;
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a933',
   '00000000-0000-0000-0000-00000000a943',
   '00000000-0000-0000-0000-00000000a903',
   '00000000-0000-0000-0000-00000000a922',now());
  IF outcome <> 'DUTY_NO_SUPPLY' THEN
    RAISE EXCEPTION 'R6 double-dispatched busy specialist: %',outcome;
  END IF;
  outcome := apgic_instant_answer(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a903',true,now());
  IF outcome <> 'DUTY_NOT_FOUND' THEN
    RAISE EXCEPTION 'R6 unauthorised specialist answered: %',outcome;
  END IF;
  outcome := apgic_instant_answer(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a901',true,now());
  IF outcome <> 'DUTY_ACCEPTED_PAYMENT_REQUIRED' THEN
    RAISE EXCEPTION 'R6 accepted offer skipped real-payment gate: %',outcome;
  END IF;
  IF EXISTS (SELECT 1 FROM bookings WHERE client_identity_id='00000000-0000-0000-0000-00000000a902') THEN
    RAISE EXCEPTION 'R6 created an unpaid booking from specialist acceptance';
  END IF;
  outcome := apgic_instant_cancel(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a903',now());
  IF outcome <> 'DUTY_NOT_FOUND' THEN
    RAISE EXCEPTION 'R6 cross-owner cancellation authorized: %',outcome;
  END IF;
  outcome := apgic_instant_cancel(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a902',now());
  IF outcome <> 'DUTY_CANCELLED' THEN
    RAISE EXCEPTION 'R6 owner cannot cancel pending payment: %',outcome;
  END IF;
  outcome := apgic_instant_cancel(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a902',now());
  IF outcome <> 'DUTY_ALREADY_CANCELLED' THEN
    RAISE EXCEPTION 'R6 duplicate cancel not idempotent: %',outcome;
  END IF;
  outcome := apgic_instant_answer(
   '00000000-0000-0000-0000-00000000a931',
   '00000000-0000-0000-0000-00000000a901',true,now());
  IF outcome <> 'DUTY_OFFER_NOT_ACTIVE' THEN
    RAISE EXCEPTION 'R6 late answer resurrected cancelled offer: %',outcome;
  END IF;
  -- Explicit retry after a client cancellation is a new consented request,
  -- and must not blacklist the only qualified specialist forever.
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a937',
   '00000000-0000-0000-0000-00000000a947',
   '00000000-0000-0000-0000-00000000a902',
   '00000000-0000-0000-0000-00000000a921',now());
  IF outcome <> 'DUTY_OFFERED' THEN
    RAISE EXCEPTION 'R6 client cannot explicitly retry same qualified specialist after own cancellation: %',outcome;
  END IF;
  outcome := apgic_instant_cancel(
   '00000000-0000-0000-0000-00000000a937',
   '00000000-0000-0000-0000-00000000a902',now());
  IF outcome <> 'DUTY_CANCELLED' THEN
    RAISE EXCEPTION 'R6 second voluntary cancellation failed: %',outcome;
  END IF;
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a934',
   '00000000-0000-0000-0000-00000000a944',
   '00000000-0000-0000-0000-00000000a903',
   '00000000-0000-0000-0000-00000000a922',now());
  IF outcome <> 'DUTY_OFFERED' THEN
    RAISE EXCEPTION 'R6 cancellation did not return specialist to supply: %',outcome;
  END IF;
  outcome := apgic_instant_answer(
   '00000000-0000-0000-0000-00000000a934',
   '00000000-0000-0000-0000-00000000a901',false,now());
  IF outcome <> 'DUTY_DECLINED' THEN
    RAISE EXCEPTION 'R6 specialist decline invalid: %',outcome;
  END IF;
  outcome := apgic_instant_enable(
   '00000000-0000-0000-0000-00000000a901',
   '00000000-0000-0000-0000-00000000a917',true,now());
  IF outcome <> 'DUTY_AVAILABLE_NOW' THEN
    RAISE EXCEPTION 'R6 declined specialist could not renew duty: %',outcome;
  END IF;
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a935',
   '00000000-0000-0000-0000-00000000a945',
   '00000000-0000-0000-0000-00000000a904',
   '00000000-0000-0000-0000-00000000a923',now());
  IF outcome <> 'DUTY_OFFERED' THEN
    RAISE EXCEPTION 'R6 renewed duty did not match third real confirmed client: %',outcome;
  END IF;
  PERFORM apgic_instant_expire(now()+interval '50 seconds');
  IF (SELECT state FROM instant_dispatches WHERE id='00000000-0000-0000-0000-00000000a935') <> 'EXPIRED' THEN
    RAISE EXCEPTION 'R6 unanswered offer was not expired';
  END IF;
  IF (SELECT state FROM instant_presence WHERE specialist_identity_id='00000000-0000-0000-0000-00000000a901') <> 'AVAILABLE_NOW' THEN
    RAISE EXCEPTION 'R6 expiry failed to release still-on-duty specialist';
  END IF;
  -- A new latest qualification can downgrade the specialist after OFFERED
  -- while old publication remains ACTIVE. Acceptance must recheck this truth.
  outcome := apgic_instant_request(
   '00000000-0000-0000-0000-00000000a936',
   '00000000-0000-0000-0000-00000000a946',
   '00000000-0000-0000-0000-00000000a905',
   '00000000-0000-0000-0000-00000000a924',now());
  IF outcome <> 'DUTY_OFFERED' THEN
    RAISE EXCEPTION 'R6 fourth qualified user could not request requalification-race offer: %',outcome;
  END IF;
  INSERT INTO qualification_evaluations (
   id,specialist_id,topic_id,jurisdiction,service_format,age_group,
   decision,policy_version,reason_codes,evidence_refs,
   capability_evidence_state,capability_verification_state,evaluated_at
  ) VALUES (
   '00000000-0000-0000-0000-00000000a918',
   '00000000-0000-0000-0000-00000000a914',
   'anxiety','RU','ONLINE','ADULT','MANUAL_REVIEW','qualification-v1',
   ARRAY['QUAL_MANUAL_REVIEW_REQUIRED'],ARRAY['evidence:r6'],
   'APGIC_VERIFIED','ACTIVE',now()+interval '1 second'
  );
  outcome := apgic_instant_answer(
   '00000000-0000-0000-0000-00000000a936',
   '00000000-0000-0000-0000-00000000a901',true,now());
  IF outcome <> 'DUTY_OFFER_NOT_ACTIVE' THEN
    RAISE EXCEPTION 'R6 stale qualification allowed acceptance after new MANUAL_REVIEW: %',outcome;
  END IF;
  IF (SELECT state FROM instant_dispatches
       WHERE id='00000000-0000-0000-0000-00000000a936') <> 'INVALIDATED' THEN
    RAISE EXCEPTION 'R6 previously offered consultation remained payable after MANUAL_REVIEW';
  END IF;
  IF (SELECT state FROM instant_presence
       WHERE specialist_identity_id='00000000-0000-0000-0000-00000000a901') <> 'OFFLINE' THEN
    RAISE EXCEPTION 'R6 revoked qualification left specialist marked available now';
  END IF;
  outcome := apgic_instant_cancel(
   '00000000-0000-0000-0000-00000000a936',
   '00000000-0000-0000-0000-00000000a905',now());
  IF outcome <> 'DUTY_NOT_CANCELLABLE' THEN
    RAISE EXCEPTION 'R6 terminal invalidation was incorrectly changed by cancellation: %',outcome;
  END IF;
  outcome := apgic_instant_enable(
   '00000000-0000-0000-0000-00000000a901',
   '00000000-0000-0000-0000-00000000a917',true,now());
  IF outcome <> 'DUTY_NOT_ELIGIBLE' THEN
    RAISE EXCEPTION 'R6 downgraded latest qualification allowed new duty: %',outcome;
  END IF;
  -- A later qualification revocation must defeat the earlier green badge.
  UPDATE specialist_capabilities
     SET verification_state='REVOKED',updated_at=now()
   WHERE specialist_id='00000000-0000-0000-0000-00000000a914'
     AND topic_id='anxiety';
  IF apgic_live_offer_valid('00000000-0000-0000-0000-00000000a917') THEN
    RAISE EXCEPTION 'R6 revoked qualification still offers paid duty';
  END IF;
  outcome := apgic_instant_enable(
   '00000000-0000-0000-0000-00000000a901',
   '00000000-0000-0000-0000-00000000a917',true,now());
  IF outcome <> 'DUTY_NOT_ELIGIBLE' THEN
    RAISE EXCEPTION 'R6 allowed duty after capability revocation: %',outcome;
  END IF;
END;
$$;
ROLLBACK;
