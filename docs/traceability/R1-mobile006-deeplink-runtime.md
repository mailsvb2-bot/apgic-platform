# R1 evidence — APGIC-MOBILE-006 safe universal/app links

Requirement: `APGIC-MOBILE-006` — universal links and safe deep-link routing.

## Canonical security boundary

The link is not an authorization grant.

- The server derives booking/notification ownership and tenant scope from canonical PostgreSQL state.
- Public specialist links are available only for specialists with an ACTIVE publication.
- Protected link issuance requires the signed client-session Identity to own the current resource.
- Every resolve decrypts and authenticates the short-lived token, re-reads current canonical resource truth, and re-authorizes the signed Identity before returning ALLOW.
- Ownership/scope changes invalidate an otherwise cryptographically valid old link.
- Claims are sealed with AES-GCM. Identity/tenant/resource claims are not a readable base64 payload in URL/history/logs.
- Expired, tampered, malformed and cross-user links fail closed.

## Cross-surface routing

- Universal URL: `https://apgic.ru/l/<opaque-token>`.
- Android declares an `autoVerify` HTTPS App Link for `apgic.ru/l/*`.
- iOS declares `applinks:apgic.ru` and forwards universal-link user activities through `RCTLinkingManager`.
- The web `/l/[token]` fallback calls the same canonical resolve endpoint.
- Canonical web routes for specialist, booking and notification revalidate the token and require the server-approved canonical path to match the current URL before rendering the resource shell.
- Association endpoints fail closed when real Android signing fingerprints / iOS application identifiers are not configured; the repository does not fabricate production signing identity.

## Contract and security proof

Automated coverage includes:

- domain path/fallback validation and authorization denial tests;
- AES-GCM token tamper, expiry, bounded lifetime and ownership-change invalidation;
- authenticated HTTP issuance/resolve tests including cross-user denial;
- generated OpenAPI contract drift guard;
- hostile origin/query/fragment URL parsing tests in the native client;
- verified-domain/native-manifest contract guard;
- web Playwright fallback + canonical-route revalidation;
- installed Android/iOS application E2E scripts that consume a server-issued canonical specialist link.

## Verification boundary

The implementation and test path is wired, but this document intentionally does not claim `VERIFIED` yet.

Before status advancement, the final candidate SHA must have green contract/security tests plus installed-app Android and iOS E2E, and exact `surface://WEB/`, `surface://IOS/`, and `surface://ANDROID/` evidence references must be bound into the Requirement Registry.

Production store/signing publication is a separate release gate and is not claimed here.
