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

Repository-side verification is complete for the latest implementation candidate `8371cd15a8f65797a9ea16e9c725585e98a19663` in CI run `36890270476` (CI #1861).

Required evidence:

- CONTRACT_TEST: Canon / architecture conformance job `110463830899` passed, including the deep-link contract guard, generated-contract drift checks, multi-surface guards and the full tools test suite.
- SECURITY_TEST: Go format/vet/race job `110463831194` passed the deep-link unit/HTTP suites covering AES-GCM tamper rejection, expiry, bounded input, stale ownership/scope invalidation, cross-user denial, authentication-before-resource-lookup and malformed/trailing/oversized request rejection.
- NATIVE_E2E / IOS: job `110463830845` passed simulator build plus installed-app capability, installation lifecycle and canonical deep-link resolution. The idb dependency bootstrap also passed with bounded retry semantics, so a transient external download failure no longer creates a false product regression.
- NATIVE_E2E / ANDROID: job `110463831035` passed debug build plus installed-app capability, installation lifecycle and canonical deep-link resolution.
- WEB: job `110463830832` passed the production Next.js build and Playwright fallback/resource revalidation suite.

Exact multi-surface bindings:

- Web: `surface://WEB/mobile006-safe-deeplink/run-36890270476/job-110463830832`
- iOS: `surface://IOS/mobile006-safe-deeplink/run-36890270476/job-110463830845`
- Android: `surface://ANDROID/mobile006-safe-deeplink/run-36890270476/job-110463831035`

Run artifacts bind the candidate to:
- release evidence artifact `11177181795`, digest `sha256:735dffb3844ff9d22b58f158b50abf3d1a0b436f4928d930a57906f688edcae2`;
- iOS simulator artifact `11175879816`, digest `sha256:03f19a48f93a1e808c3e174724ae95b1ec5f4afe73c2606f2473f5412e5ff462`;
- Android debug artifact `11175773914`, digest `sha256:a745ceebdffb28c33db89d0c87d1878c27c00fedde769755e6a79c0e77cbc175`.

The Requirement Registry remains `VERIFIED`. This does **not** claim `RELEASED` or production verified-domain publication. Real organization-owned Apple Team / bundle identity, Android package/signing fingerprint, signed store artifacts and live-domain real-device evidence remain the external release boundary tracked in issue #3 and must not be fabricated from simulator/debug evidence.
