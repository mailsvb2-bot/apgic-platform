-- Shared pricing and hold eligibility must include a current matching qualification evaluation.
CREATE OR REPLACE FUNCTION apgic_live_offer_valid(p_offer_id uuid)
RETURNS boolean LANGUAGE sql VOLATILE AS $$
  SELECT EXISTS (
    SELECT 1
      FROM specialist_bookable_offers offer
      JOIN specialist_profiles sp ON sp.id = offer.specialist_id
      JOIN specialist_capabilities cap
        ON cap.specialist_id = sp.id AND cap.topic_id = offer.topic_id
      JOIN specialist_publications pub
        ON pub.specialist_id = sp.id AND pub.topic_id = offer.topic_id
       AND pub.state = 'ACTIVE'
      JOIN qualification_evaluations evaluation
        ON evaluation.id = pub.qualification_evaluation_id
       AND evaluation.specialist_id = sp.id
       AND evaluation.topic_id = offer.topic_id
       AND evaluation.decision = 'ELIGIBLE'
       AND evaluation.policy_version = 'qualification-v1'
       AND evaluation.jurisdiction = 'RU'
       AND evaluation.service_format = 'ONLINE'
       AND evaluation.age_group = 'ADULT'
      JOIN products product ON product.id = offer.product_id
        AND product.status = 'PUBLISHED'
        AND product.owner_type = 'ORGANIZATION'
      JOIN organization_directions direction
        ON direction.id = product.organization_direction_id
       AND direction.organization_id = product.owner_id
       AND direction.status = 'ACTIVE'
      JOIN organizations org
        ON org.id = product.owner_id AND org.status = 'ACTIVE'
      JOIN organization_memberships member
        ON member.organization_id = org.id
       AND member.identity_id = sp.identity_id
       AND member.status = 'ACTIVE'
      JOIN organization_ownerships owner
        ON owner.organization_id = org.id
       AND owner.identity_id = sp.identity_id
       AND owner.status = 'ACTIVE'
     WHERE offer.id = p_offer_id
       AND offer.state = 'ACTIVE'
       AND offer.price_minor > 0
       AND offer.currency = 'RUB'
       AND offer.specialist_identity_id = sp.identity_id
       AND sp.profile_complete AND sp.review_state = 'APPROVED'
       AND cap.evidence_state = 'APGIC_VERIFIED'
       AND cap.verification_state = 'ACTIVE'
       AND (cap.expires_at IS NULL OR cap.expires_at > clock_timestamp())
  )
$$;

