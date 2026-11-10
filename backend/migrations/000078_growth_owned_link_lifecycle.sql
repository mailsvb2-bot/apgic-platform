-- Uniqueness and replay across browser retries, restarts and concurrent API
-- instances. Preserve all attribution evidence from historical link IDs.
-- The original v1 endpoint could create many active links for the same offer.
-- Reconcile deterministically, then enforce exactly one active link.
WITH ranked AS (
  SELECT id,ROW_NUMBER() OVER (
    PARTITION BY issuer_identity_id,specialist_offer_id
    ORDER BY expires_at DESC,created_at DESC,id DESC
  ) AS position
  FROM growth_links WHERE state='ACTIVE'
)
UPDATE growth_links l
  SET state='REVOKED',revoked_at=clock_timestamp()
 FROM ranked r
 WHERE l.id=r.id AND (r.position>1 OR l.expires_at<=clock_timestamp());

CREATE UNIQUE INDEX growth_links_one_active_per_owner_offer
  ON growth_links(issuer_identity_id,specialist_offer_id)
  WHERE state='ACTIVE';
