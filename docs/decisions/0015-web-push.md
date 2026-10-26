# ADR-0015: Web push from an installable web app

- Status: accepted, 2026-10-26; not enabled for any user until the legal assessment and the decision record on minors below are recorded
- Reviewed: decided by the project owner on 2026-10-25 (web push in slice 10, a messenger bot later) and on 2026-10-26 (the legal assessment before the pilot, the functional requirement written with the slice 10 plan); the pushed kinds, the minors gate and the payload, lifecycle, outbound, key and service worker rules are drafted for the owner's review; wording pending owner review, line by line
- Deciders: project owner
- Amends: [ADR-0008](0008-hosting-and-data-residency.md), with one exception to its rule that third-party services outside Russia receive no personal data; [ADR-0006](0006-background-worker-and-email.md), in its rule that email is the only external notification channel in the MVP
- Related: FR-13, NFR-1.3, NFR-1.4, [ADR-0006](0006-background-worker-and-email.md), [ADR-0013](0013-pilot-deployment.md), [notifications specification](../../knowledge/specs/notifications.md), [threat model](../threat-model.md) T-18, T-19, T-20, T-21, T-28, T-29, T-34, T-36, T-37, T-46, [research](../../knowledge/research/tutor-crm-and-mobile-distribution-2026-10-25.md), [roadmap](../roadmap.md)

## Context

The owner plans the pilot with tutors of school students, a change of audience tracked as an [open question](../../knowledge/architecture/detailed.md#open-implementation-questions) until its own decision record lands. Email reaches such students and their families poorly, and missed lessons are the cost of a reminder nobody reads. A phone notification is the channel such students notice. The [post-pilot analysis](../../knowledge/analysis/post-pilot-plan-and-mobile-app-2026-10-25.md) compared an installable web app with web push, a store app and a bot in a Russian messenger; the owner chose web push in the MVP and a messenger bot later.

Web push does not stay inside Russia. A browser receives push messages only through the push service its vendor operates, and the services of the major browsers are operated by their vendors, as far as known outside Russia ([research](../../knowledge/research/tutor-crm-and-mobile-distribution-2026-10-25.md)). The Web Push protocol encrypts the payload end to end between the sending server and the browser, so the push service cannot read it, but even a push without any content tells the push service:

- the subscription endpoint, an address the push service itself issued to the user's browser;
- that this browser, and the network address it registers from, subscribed to OVC's sender key, which means that its user uses OVC;
- the network address of OVC's sending server;
- when each message is sent and how large it is, which follows the user's lesson schedule and activity.

[ADR-0008](0008-hosting-and-data-residency.md) rejected services abroad because each becomes a cross-border transfer to assess. Web push is exactly such a case, so it is decided here as an explicit exception, gated on a legal assessment, rather than as a reading of ADR-0008.

On iOS and iPadOS a web app receives push only after the user adds it to the Home Screen and grants permission, and reports describe subscriptions there that stop working after a while ([research](../../knowledge/research/tutor-crm-and-mobile-distribution-2026-10-25.md)).

## Decision

- **Scope.** In roadmap slice 10 the frontend becomes an installable web app (a web app manifest, icons and a service worker), and the worker sends web push through the outbox. Email and in-app notifications stay the primary channels; push is an addition that never replaces a required email. A bot in a Russian messenger comes after the MVP under its own decision record.
- **Gate.** Push is off unless a setting enables it, and the setting defaults to off in every deployment. It stays off in every production deployment until the legal assessment that the roadmap requires before the pilot is recorded, and until the decision record on minors is recorded; once both are recorded, push is enabled only for the whole deployment, unless the minors record gives OVC a way to tell which accounts belong to minors and decides for them. That assessment answers, as questions and not as conclusions taken here, whether the metadata listed above is personal data under Federal Law 152-FZ, whether sending it to foreign push services is a cross-border transfer and what it requires (including any notification of the data-protection authority), and what the privacy policy must say.
- **Exception to ADR-0008.** Once the gate opens, the vendors' push services receive only the metadata listed in the context and an encrypted payload that follows the payload rules below. No other data reaches them, and no other foreign service is admitted by this record.
- **Payload rules.**
  - The payload holds only the notification kind, a value of one enum, and one opaque identifier: the in-app notification row's, or for a kind without an in-app row, such as the 24-hour lesson reminder, the lesson's, which the REST API resolves within the caller's scope. It holds no names, lesson times, subjects, message text, addresses, counts or scores.
  - The service worker shows a fixed text per kind in the user's language ("A lesson reminder from OVC"), never text from the payload, so a locked screen on a shared phone shows nothing personal.
  - The notification's `tag` and `data` and the push headers (`Topic`, `Urgency`) carry no identifiers or data beyond the payload's kind and identifier.
  - On a click the service worker opens only an OVC route on the same origin built from the kind and the identifier, never an address taken from the payload ([T-28](../threat-model.md#threats-and-controls)).
  - Push carries only kinds whose timing reveals little: lesson reminders and lesson changes in the MVP. The slice 10 plan may add kinds under the same rules; chat messages are never pushed one by one.
- **Subscription lifecycle.**
  - A user subscribes only by an explicit action in OVC, per device; nothing subscribes by default.
  - A subscription belongs to the session that created it. Every path that revokes a session or ends an account deletes the subscriptions of the sessions it ends in its own transaction, enforced at the data level (by the revocation triggers or a cascade from the session row) rather than path by path: sign-out, sign out everywhere, refresh token reuse, the same-browser confirmation that revokes other sessions, password reset and change, reclaim, blocking, account deletion and purge ([T-29](../threat-model.md#threats-and-controls)). An expired session's subscriptions are deleted by the session cleanup.
  - An endpoint is unique across accounts: registering it again moves it to the new session and deletes the earlier row, so a shared browser never keeps pushing an earlier account's notifications. The client also unsubscribes the browser on sign-out and on an account change.
  - A `404` or `410` answer from the push service deletes the subscription.
  - The worker decides the recipient again when it sends, as for email: the subscription's session is neither revoked nor expired, the account is active and not blocked, the relationship or group still holds ([T-21](../threat-model.md#threats-and-controls)), the lesson is still due, and the preference allows the kind.
- **Outbound requests.** The endpoint is client input. The worker sends only to push-service hosts fixed in code, matched exactly, or as a suffix at a dot boundary only for a vendor that issues a host per endpoint, which no production setting can extend; only over `https` on port 443, without user information in the address and within a bounded length. The API checks this when a browser subscribes and the worker again when it sends. When dialling, the worker refuses resolved loopback, private, link-local and metadata addresses and ignores proxy environment variables. Redirects are refused, response bodies are bounded and never logged, and every request has a timeout. A fake push service for tests exists only under the same conditions as the fake meeting provider and the test control surface ([T-36](../threat-model.md#threats-and-controls)).
- **Limits.** The number of subscriptions per session and per account is capped, subscribing is throttled, and a push service's `429` or `5xx` answer backs off with the bounded outbox retries and the `dead` state of [ADR-0006](0006-background-worker-and-email.md).
- **Keys and logs.** The sender's private signing key (VAPID) is a `Secret` setting held only by the worker; the API holds only the public key and refuses to start when the private key is set. The key's contact claim is OVC's address or a generic operator address, never a person's email. The endpoint and the subscription's keys (`p256dh` and `auth`) are never returned by any API response and never logged, DEBUG included; a log names the push service only as an enum value and never contains a push service's error text.
- **Service worker.** It never intercepts or caches API routes, the WebSocket, signed storage addresses or any authenticated response: it has no fetch handling beyond, at most, precaching hashed static assets, and its caches are cleared on sign-out. It is served from the built output on the OVC origin with `Cache-Control: no-cache`, with scope `/` and no `Service-Worker-Allowed` header. The Content Security Policy gains `worker-src 'self'` and `manifest-src 'self'` ([T-37](../threat-model.md#threats-and-controls)).
- **Installation guide.** Slice 10 writes a short guide for pilot students: installing OVC to the Home Screen on iOS and iPadOS, enabling notifications, and what a notification shows.

## Alternatives considered

- **Email and in-app notifications only:** no foreign service, but reminders reach school students poorly, which is the problem the pilot most needs solved.
- **A bot in a Russian messenger first:** keeps the channel in Russia but needs each student's messenger account linked to OVC and a decision record on the messenger operator; planned after the MVP.
- **A store app:** native push goes through Apple's or Google's push services, or on Android through RuStore's own transport, adds store reviews and, for iOS, a paid developer membership with no lawful way to pay it known today; decided after the pilot.
- **Push with content:** would put names and times on locked screens and, if the encryption ever failed, in the push service's hands; refused.

## Consequences

- The vendors' push services learn that a device's user uses OVC and when their notifications arrive. The owner chose web push on 2026-10-25 knowing this metadata; the risk stays subject to the legal assessment and recorded under [T-46](../threat-model.md#threats-and-controls).
- On iOS and iPadOS push depends on installation and may stop silently, so reminders that matter stay in email and in the notifications panel.
- New secret and new stored data: the private signing key, and subscription rows whose keys are treated like credentials. The rows also persist in the 30-day database backups after deletion, and anyone who reads the database or a backup and also holds the private signing key can push to every subscribed device, limited by the fixed text per kind and the same-origin click; whether the slice 10 plan also encrypts the subscription keys with the field encryption key is left to it. Rotating the signing key invalidates every subscription.
- Slice 10 grows by about half a slice, and its functional requirement is written with the slice 10 plan.

## Where it is applied

[Roadmap, slice 10](../roadmap.md#slice-10-results-home-pages-and-hardening), [non-functional requirements](../requirements/non-functional.md) NFR-1.4, [threat model](../threat-model.md) boundary 10 and T-46, [open implementation questions](../../knowledge/architecture/detailed.md#open-implementation-questions).
