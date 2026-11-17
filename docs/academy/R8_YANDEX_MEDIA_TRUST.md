# R8 Yandex media trust configuration

The Academy media transport has two fixed Yandex downloader hosts:

- `downloader.disk.yandex.net`
- `downloader.disk.yandex.ru`

A downloader may redirect to a provider-generated storage hostname. Such a hostname is **not trusted by suffix or wildcard**. The runtime accepts storage redirects only when the exact hostname is pinned in:

`APGIC_YANDEX_STORAGE_HOSTS`

The value is a comma-separated list of exact DNS hostnames, for example as obtained from a reviewed, authorized Yandex Disk acceptance trace. Do not put schemes, ports, paths, query strings, credentials, wildcards or whitespace around entries in this setting.

Configuration is fail-closed:

- Unset or empty: no dynamic storage hostname is trusted.
- Any malformed, duplicate, wildcard or non-storage entry invalidates the entire configured list.
- A configured hostname is matched exactly after DNS case normalization.
- Nested names, trailing-dot names, IP literals, non-default ports and non-HTTPS redirects remain rejected.
- The outbound dialer independently rejects non-public and special-purpose destination IPs after DNS resolution.
- Redirects are bounded and sensitive request headers are removed before the next hop.

Do not infer or invent storage hostnames from the documented wildcard namespace. Populate this setting only from a real provider response observed during authorized acceptance, then bind the resulting host list to the deployment configuration review. A signed download URL, its query string, OAuth token, or author-owned media path must never be copied into this setting or into logs.

This setting is not a provider certification and does not enable VIDEO publication. The independent CDN playback certification, signed attestation persistence, real author/learner journey, and committed Entitlement-revocation acceptance remain separate fail-closed gates.

### Dynamic Yandex storage nodes

An operator may explicitly opt in with `APGIC_YANDEX_TRUST_STORAGE_NODES=true` after validating real provider redirect destinations and DNS/IP dial guards. Default is disabled; exact `APGIC_YANDEX_STORAGE_HOSTS` pins remain supported. Only a single DNS label directly under `storage.yandex.net` is accepted; credentials must not follow redirects, and path/query values never appear in refusal logs. This opt-in is not automatically activated by deployment.
