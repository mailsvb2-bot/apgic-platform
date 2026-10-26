# R5 session lifecycle hardening

Existing canonical R5 personal view, bookings and check-ins are retained.
This change addresses client-state leaks and stale writes, not payment/media.

- Native invalidates old GET history and page responses on reload/unmount.
- Signed PUT preferences and POST check-in are guarded by one in-flight
  mutation lock. Requests interrupted by identity/session changes are not
  allowed to restore old view state or old-user messages.
- Native session expiration during pagination or mutation triggers an
  authoritative reload; it never merges an old client's page into a new
  client's cabinet.
- Web treats 401/403 from pagination or preferences as a session boundary:
  clear personal state and request a new signed session.
- Backend continues to be the owner of all identity checks and check-ins.
  Neither frontend path generates bookings, provider evidence or medical facts.

Verify with full Web E2E, Go/PG, native TS and Android/iOS builds.
Additional real-device tests of logout/login races remain necessary.
