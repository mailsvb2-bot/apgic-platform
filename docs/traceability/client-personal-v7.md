# APGIC Personal — client history and settings (feat/client-personal)

## Canon and ownership
Canon v7 sections 4–5, 34–45, 210–211, 226–249.
APGIC-ID-001, APGIC-AUTH-001, APGIC-DATA-001, APGIC-PRIV-001,
APGIC-CONSULT-002, APGIC-MOBILE-001/003/010, APGIC-MOBILE-020.

One signed Identity owns the full client read model. Bookings, consultations,
orders and requests are read from their existing canonical PostgreSQL tables.
No duplicated consultation transcript, media or financial truth. Editable
personal display settings are isolated from legal consent and notification truth.
An empty response means there are no facts yet, not a made-up history.

## API
- GET /v1/client/personal?history_cursor=<opaque>&limit=20 (max 50)
- PUT /v1/client/personal/preferences with full display_name/locale/time_zone.
- Both require signed client session; caller identity IDs rejected/ignored.
- Cache-Control: private, no-store. History paginated by (starts_at,booking_id)
  in a read-only repeatable-read transaction.
- For complete booking/consultation details, navigate to existing canonical
  resource endpoints. The UI never marks a consultation completed locally.

## User journeys to verify in staging/production
1. Existing client session -> personal -> sees own future booking and no foreign booking.
2. Signed consultation completion -> personal history -> persisted completed
   state -> API restart -> same state, without exposing raw content.
3. Native and Web obtain equivalent server projection, never share locally
   cached sensitive content or independent domain stores.
4. Update name/timezone -> reload in a new browser session for same Identity ->
   durable settings; other Identity retains its own settings.
5. Expired/tampered cookie -> 401, store not called; tampered cursor -> 400.
6. Two bookings with same start -> keyset pagination does not skip or repeat.
7. DB outage -> 503, no synthetic fallback.

## Deliberately not claimed
AI concierge, entitlement delivery, recommendations, subscription accounting,
rematch, notification preferences and recorded notes require their canonical
service owners and policies; no UI placeholders are evidence of production readiness.

## Signed-session navigation and visual regression
The anonymous landing page retains its approved visual baseline: no false
"logged in" badge/link or untrusted local account state. A signed client
session receives the "Мой кабинет" navigation entry after the owner-scoped
GET succeeds. After creating the first request, the journey emits a
presentation-only refresh event; the server always remains the sole authority.
Playwright tests cover anonymous and authenticated cross-page navigation.
Visual baselines are never weakened, skipped or automatically overwritten.
