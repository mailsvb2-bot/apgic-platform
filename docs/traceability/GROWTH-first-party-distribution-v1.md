# Growth & Distribution — verified first-party specialist acquisition v1

Normative source: APGIC v7 FINAL sections 163–165, 169–172, 197–200, 250–253 and 271–272. This is a production-shaped first vertical slice, **not** a declaration of completion of every growth domain.

## Actual user journey

1. A specialist owns a published product with a priced offer passing the canonical live qualification gate.
2. Their signed-in professional workspace requests a link for the offer. PostgreSQL validates the subject, the owner and current publication/qualification.
3. The API issues a 29-day first-party signed opaque share path. Its token contains only a link ID and expiry. No browser/partner can set the economic source or tenant.
4. The visitor opens /g/{token}; API checks HMAC, expiry, revocation, self-referral and current qualification, then binds a touch to an APGIC-signed HttpOnly Identity session.
5. Repeated hits for the same person/link do not multiply touches. The real HelpIntent INSERT atomically captures the earliest eligible first-party touch within 30 days.
6. The specialist sees an aggregate, privacy-minimal funnel and can revoke the link without destroying historical evidence.

## Invariants

- Source is explicitly SPECIALIST_SOURCED; other source types need separate owner verification.
- Sensitive HelpIntent free text, consultation/avatar content, identities and cross-tenant data do not leave into growth reports.
- Link source/owner/offer, touch and intent attribution are immutable in SQL; link revocation is one-way.
- Growth API fails closed when 32-byte APGIC_GROWTH_SIGNING_KEY or PostgreSQL is missing. Never commit secrets.
- No platform money movement, provider custody, synthetic payments or automatic advertising spend.
- **Verified conversion bridge:** a confirmed HelpIntent may link to a real hold only in the same database transaction and only when the clicked link and held slot point to the same qualified live specialist offer. Booking → Order → non-custodial Ledger can then produce provider-paid order counts. This does not enable payment processing or attest checkout, revenue, net refunds, CAC, royalty, commission or contribution margin. Without external provider evidence the paid-order count stays zero.

## Evidence gate

Go signing tests, HTTP auth/signed capture tests, web origin tests, DB transaction/immutability integration, staging from reviewed offer to first-party captured and confirmed client request; restart/replay and revocation checks. Until staging and release evidence exist, no affected requirement is VERIFIED/RELEASED.
