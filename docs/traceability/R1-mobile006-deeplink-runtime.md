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

Repository-side verification is complete for implementation candidate `c8fc3964a895517457955b0918fc85f8bbb2d2ab` in CI run `36880963294` (CI #1857).

Required evidence:

- CONTRACT_TEST: Canon / architecture conformance job `110433285717` passed, including `mobile_deeplink_contract_guard.py`, generated OpenAPI drift checks, `multisurface_guard.py`, `multisurface_verification_e2e.py` and `r1_multisurface_guard.py`.
- SECURITY_TEST: Go format/vet/race job `110433285660` passed the deep-link unit/HTTP suites covering AES-GCM tamper rejection, expiry, bounded input, stale ownership/scope invalidation, cross-user denial, authentication-before-resource-lookup and malformed/trailing/oversized request rejection.
- NATIVE_E2E / IOS: job `110433285950` passed simulator build plus installed-app capability, installation and canonical deep-link runtime proof.
- NATIVE_E2E / ANDROID: job `110433285987` passed debug build plus installed-app capability, installation and canonical deep-link runtime proof.
- WEB: job `110433285677` passed the production Next.js build and Playwright fallback/resource revalidation suite across the configured browser profiles.

Exact multi-surface bindings:

- Web: `surface://WEB/mobile006-safe-deeplink/run-36880963294/job-110433285677`
- iOS: `surface://IOS/mobile006-safe-deeplink/run-36880963294/job-110433285950`
- Android: `surface://ANDROID/mobile006-safe-deeplink/run-36880963294/job-110433285987`

Run artifacts include the iOS native simulator build (artifact `11171339563`, SHA-256 digest `6cff36a33dc1623e9c90e8f256e87979c993be7cb1871e06c781b76ac91544be`) and Android native debug build (artifact `11170589984`, SHA-256 digest `efde196069844d867ea10ded6d7f14426b476c6400337a03016a58654daf51ce`).

The Requirement Registry is advanced to `VERIFIED`. This does **not** claim `RELEASED` or production verified-domain publication. Real organization-owned Apple Team / bundle identity, Android package/signing fingerprint, signed store artifacts and live-domain real-device evidence remain the external release boundary tracked in issue #3 and must not be fabricated from simulator/debug evidence.
