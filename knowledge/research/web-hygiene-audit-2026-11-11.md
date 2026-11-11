# Public web hygiene checks for the pilot deployment

Checked 2026-11-11 against a public survey of 284 large Russian websites (Habr, "Мошенникам даже не нужно ничего взламывать: я проверил 284 крупных сайта Рунета", https://habr.com/ru/articles/1105240/). The survey lists weaknesses that need no break-in: mail spoofing, old libraries, dangling subdomains, missing transport protections, weak content security policies, blocked addresses and undisclosed transfers to third parties. This note maps each item to OVC's current decisions and code, and lists what the pilot deployment must still prove. It is agent research for the owner's decision, not a decision.

## Mapping

| Survey item | OVC today | Gap and where it belongs |
|---|---|---|
| Mail spoofing: no DMARC, or `p=none` | [ADR-0006](../../docs/decisions/0006-background-worker-and-email.md) and the [notifications specification](../specs/notifications.md) require SPF, DKIM and DMARC on the OVC domain, with no policy level | The pilot should reach `p=reject` with `sp=reject`. It starts at `p=none`, with aggregate reports sent to a mailbox OVC controls in Russia, and moves through `quarantine`. A check reads the published records. Slice 10. |
| Old JavaScript libraries with known CVEs | No CDN scripts. The frontend is built from `pnpm-lock.yaml`, scanned by OSV-Scanner in every merge gate ([development guide](../../docs/development.md#security-scanning)), under the release-age rule of [ADR-0017](../../docs/decisions/0017-dependency-release-age.md) | Covered. |
| Dangling subdomains open to takeover | The conferencing server runs on a subdomain of the OVC domain ([realtime and video](../specs/realtime-and-video.md)); DNS is at a Russian provider ([ADR-0013](../../docs/decisions/0013-pilot-deployment.md)) | There is no inventory or check of DNS records. The conferencing subdomain matters most, because it is the one origin OVC trusts: the only framed origin (`frame-src`, T-16), same-site for cookies (T-24), and handled at the WebSocket origin check (T-45). The pilot needs a list of DNS records in the repository and a deployment check that every record points to a live resource OVC controls. Slice 10. |
| Missing HSTS | Not specified in any specification or in the threat model | Production responses should send `Strict-Transport-Security` with a long max-age and `includeSubDomains`. Preloading the domain is slow to undo, so it stays a separate owner decision. Slice 10, with the reverse proxy of ADR-0013. |
| TLS 1.0 and 1.1 still accepted | Outbound calls to the conferencing server require TLS 1.2 or newer ([realtime and video](../specs/realtime-and-video.md)); the inbound policy is not specified | The reverse proxy should accept only TLS 1.2 and 1.3, checked after deployment. Slice 10. |
| Certificates close to expiry | ADR-0013 uses Let's Encrypt, whose certificate lifetime is shrinking ([hosting research](hosting-2026-10-22.md)) | There is no renewal check or expiry alert. The pilot needs automatic renewal and an expiry check that alerts well before expiry. Slice 10. |
| Weak or missing content security policy and other protective headers | Pages send `default-src 'self'`, `script-src 'self'`, `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`, and `frame-src 'none'` today, which becomes the conferencing subdomain only once the embedded room is enabled ([API conventions](../specs/api.md#conventions), T-16, T-37). Every response sends `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`. Downloads send `default-src 'none'; sandbox; frame-ancestors 'none'` ([files](../specs/files.md)) | Covered in design. `frontend/security-headers.ts` sets these headers for the preview server, but the reverse proxy serves production, so a deployment check must confirm that the proxy sends the same policy and headers. Slice 10. |
| Visitor data sent to third parties | No analytics and no third-party scripts. [ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md) keeps user data in Russia, and `script-src 'self'` blocks injected trackers | Web push ([ADR-0015](../../docs/decisions/0015-web-push.md), T-46) is planned for slice 10 and stays off until its legal assessment. It sends metadata to the browser vendors' push services outside Russia, so enabling it adds those vendors as recipients and needs a cross-border transfer notice. |
| Privacy policy that contradicts real data flows | ADR-0008 makes the privacy policy, consent at sign-up and registration with the data-protection authority owner obligations outside this repository | The deployment checklist should track that the policy names the actual processors: hosting, storage, mail, conferencing, and the push vendors once push is enabled. Owner obligation, tracked in slice 10. |
| Dependence on foreign providers | Hosting, storage, mail and conferencing are Russian (ADR-0008); certificates come from Let's Encrypt, a foreign authority | Residual: if that authority stopped issuing certificates, HTTPS would break. Whether to keep a fallback authority is an open question for slice 10. |
| Addresses in the blocking registry through shared hosting | The application runs on a cloud VM at a Russian provider (ADR-0013) | Residual: a shared provider address could be listed because of another tenant. The pilot should check its public address against the registry before launch. Slice 10. |
| Embedded resources blocked in Russia | The only framed origin is OVC's own conferencing subdomain; nothing else external is embedded | Covered. |
| Unflagged links to designated organisations | No such links | Not applicable. |

## Summary

The application design already avoids the content-side weaknesses: third-party scripts and trackers, old libraries and weak content security. The open items belong to deployment and operations, which slice 10 prepares for the pilot (ADR-0013):

- the DMARC policy level;
- HSTS;
- the inbound TLS versions;
- a DNS inventory against takeover, especially of the conferencing subdomain;
- certificate expiry monitoring;
- a check that the reverse proxy sends the specified headers;
- a check of the public address against the blocking registry;
- a privacy policy that matches the real processors, including the push vendors once web push is enabled;
- a fallback certificate authority.

Each should become a checkable acceptance criterion of the pilot deployment, with the threat model and the specifications updated in the same change.
