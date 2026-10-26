# Post-pilot plan and the mobile app question

Written 2026-10-25. Agent analysis: it informs the owner's decisions and decides nothing; a slice enters the [roadmap](../../docs/roadmap.md) only through an approved change, and a deferred design starts as a [proposal](../../docs/proposals/README.md). Evidence about the compared tutor CRM, app stores and push delivery is in the [tutor CRM and mobile distribution research](../research/tutor-crm-and-mobile-distribution-2026-10-25.md); the market and strategy context is in the [early-stage product strategy](early-stage-product-strategy-2026-10-25.md); running costs are in the [hosting cost and load analysis](hosting-cost-and-load-2026-10-25.md).

It answers three questions: which functions that a popular tutor CRM has and OVC lacks are worth adding after the pilot, in what order; whether OVC needs a mobile app; and what an app would take to build, host and support.

## Context

- **Already recorded:** recordings and homework analytics come first after the MVP ([roadmap](../../docs/roadmap.md#after-the-mvp), 2026-10-22); points and scores never reach students ([ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md)).
- **Stated by the project owner on 2026-10-25, not yet recorded:** students can be anyone, school students included; the pilot runs with tutors of school students after slice 10; scores stay hidden from parents too; recordings come first after the MVP regardless of pilot data. The first two conflict with [ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md) ("tutors who teach adults"), the [product overview](../../docs/product.md#who-it-is-for) and the [threat model](../../docs/threat-model.md) ("personal data of adults"); they also supersede the [early-stage strategy](early-stage-product-strategy-2026-10-25.md)'s recommendation to pilot with tutors of adults. Because school students' data then enters OVC at the pilot, the decision record on minors and a lawyer's assessment of 152-FZ consent belong before the pilot, not with the parents slice (question 1 below).
- The compared product is a tutor CRM at 350 ₽ a month with student cards, a regular schedule, reminders, payments and balances, statistics, student and parent access, online booking, a task tracker and mobile apps for both sides (research file). Its landing page mentions no video room, chat or self-checking homework. By the table below OVC covers roughly a third of its functions (**estimate**: about 3 of 9 rows fully or partly); by its landing page it covers almost none of OVC's teaching loop. The two are complementary halves of a tutor's work.

## Gap analysis

| Function of the CRM | OVC today | Worth adding? | Why |
|---|---|---|---|
| Payments, balances, debits per lesson, payment reminders | none ([proposal 0001](../../docs/proposals/0001-payment-ledger.md), deferred) | **yes, high** | the reason tutors keep a CRM next to a teaching tool; in school tutoring parents pay and ask for history |
| Attendance | lesson status only; provider attendance after the MVP | **yes, high** | needed for debits and for parents; cheap once the ledger exists |
| Parent access | none: two roles, one per account (FR-1.3) | **yes, high** for school students | parents watch schedule, homework and attendance, and pay; the pilot's tutors teach school students |
| Reminders in a messenger or as push | email and in-app only | **yes, medium** | email is weak for teenagers and parents; competitors notify through messengers |
| Tutor statistics: lessons, hours, income, debts, month over month | none (homework analytics after the MVP is a different thing) | **yes, medium** | falls out of the ledger and attendance almost for free |
| Mobile app | web, phone layouts in slice 10 | **staged**, see below | a PWA covers most of it; a store app has distribution problems from Russia |
| Online booking of free slots | none, not in the first release | later, on demand | useful for new students; most pilot students have fixed weekly slots |
| Task tracker for the tutor | none | no | not part of teaching or of the practice's records; general tools do it |
| Birthdays, pets' names and other free personal fields | none | no | conflicts with collecting only the personal data a feature needs (NFR-1.3); a free-text note on the student card would cover goals; parents' contacts are not stored on the card, because slice 13 gives a parent an account of their own |

## Proposed post-pilot slices

Effort is given in OVC slices (the roadmap's unit; slices 1–3 closed between 2026-10-21 and 2026-10-25 with agents doing the work and the owner approving) and, for comparison, in developer-weeks for one hired full-stack developer (**estimate**).

| Order | Slice | Contents | Effort | Depends on | Owner decisions and documents |
|---|---|---|---|---|---|
| 11 | Recordings | FR-5.4 to FR-5.6, as recorded in the roadmap | 1 slice; 2–3 developer-weeks | slice 7 | the recordings probe; cold storage class (cost analysis lever 5) |
| 11b | Homework analytics | FR-11.2 to FR-11.4, which the roadmap places first after the MVP together with recordings | 0.5–1 slice; 1–2 developer-weeks | slice 9 | none beyond the requirements |
| 12 | Ledger and attendance | proposal 0001: price per relationship or group, recorded payments and packages, debits for ended lessons, late-cancel and free marks, balances and debts on the student card, low-balance reminders; attendance marked by the tutor, prefilled from provider join events; no money moves through OVC | 1–1.5 slices; 3–4 developer-weeks | slice 5 lessons, slice 11 or slice 7 events | proposal 0001 moved to accepted; data model and API; threat model (financial records are personal data) |
| 13 | Parents | a guardian account linked to a student by the student's or the tutor's invitation; sees the student's schedule, homework statuses, attendance, balance and payment history with each tutor; receives reminders; never sees scores (the owner's statement of 2026-10-25); may message the tutor | 1.5 slices; 4–5 developer-weeks | slice 12 for balances; the decision record on minors (timing: question 1) | a decision record amending ADR-0005 and FR-1.3 (a third role and its scope), threat model; meets the security trigger |
| 14 | Notification channels | installable web app (PWA) with web push, and reminders through a bot in a Russian messenger; payloads carry no personal data beyond what the channel's decision record allows | 1 slice; 2–3 developer-weeks | slices 4, 5 | a decision record on push and messenger providers under ADR-0008 (browser push services and messenger platforms are third parties) |
| 15 | Tutor statistics | lessons held, hours, income, debts and attendance by month, compared with the previous month | 0.5 slice; 1–2 developer-weeks | slice 12 | none beyond requirements |
| later | Booking | a link where a new or existing student picks a free slot from the tutor's published availability | 1 slice; 2–3 developer-weeks | slice 5 | on measured demand |

Total for slices 12–15: about 4–4.5 slices, or 10–14 developer-weeks for one hired developer (**estimate**).

**The order is a proposal.** The roadmap puts recordings and homework analytics first after the MVP, and the owner confirmed recordings on 2026-10-25. Because the pilot's tutors teach school students, whose parents pay and ask about progress, the ledger and parent access are the functions most likely to keep those tutors from running a CRM alongside OVC. If the pilot shows them still keeping payments elsewhere by week 4, moving slices 12 and 13 ahead of recordings is the owner's question below.

**What to measure in the pilot to confirm the order:** whether the pilot tutors keep a separate payment record (asked weekly), how often parents ask the tutor about schedule or homework, the share of reminder emails opened by students, missed lessons per 100, and whether students ask for recordings.

## Does OVC need a mobile app?

### What an app would add

- **Reliable reminders on the phone** (push), which matter more for school students and parents than email.
- **An icon on the home screen**, and the habit of opening it.
- **Parity with competitors** that ship apps for tutors and for students or parents.

### What a web app already gives

- Every page must work from 360 px wide and gets dedicated phone layouts in slice 10 (NFR-5.2).
- The video room runs in mobile browsers, with "open in a new tab" always available (FR-5.3); a native app would show the same BigBlueButton client in a web view.
- An installable web app (PWA) adds the home-screen icon, a full-screen window and web push; on iOS, web push works only after the user adds the app to the Home Screen, and reports of subscriptions silently stopping exist (research file).

### Distribution from Russia is the hard part

- **iOS:** the App Store needs a paid Apple Developer Program membership (99 USD a year), which, by third-party reports in the research file, Russian cards cannot pay; the known workarounds are intermediaries, gift cards or another account region, each a risk for a product that depends on staying listed.
- **Android:** by third-party reports of Google's statement (research file), Google's developer verification, rolling out worldwide within about a year of 2026-09-30, excludes developers in Russia and covers Google Play, RuStore and plain APK files; unverified apps install only through a deliberate developer-mode procedure with a 24-hour wait. New Google accounts with Russian phone numbers are already refused. RuStore's own tools, push included, are free.
- **Push delivery** for any app goes through the platform's service (APNs, FCM, or RuStore's transport for Android; browser vendors' services for web push). ADR-0008 lets no foreign service receive personal data, so payloads would carry no names, times or subjects ("You have a new notification in OVC"). Even a content-free message hands the push service a device token or subscription endpoint, the recipient's IP address and the timing of messages, which may itself count as personal data; web push payloads are encrypted end to end (research file), native push payloads are visible to the platform. The choice therefore needs its own decision record that weighs this metadata, not only the payload.

### Options, effort and cost

Running costs use the hosting analysis; the API, database and storage serve an app exactly as they serve the browser, so no new server is needed.

| Option | What it is | Effort | Extra hosting a month | Fixed fees | Support |
|---|---|---|---|---|---|
| **A. PWA** | manifest, icons, a service worker for static assets, install prompt, web push sent by the worker | inside slice 14: about 0.5 slice; 1–2 developer-weeks | ≈0 ₽ (a few kilobytes per message; browser push delivery is commonly free, **unconfirmed**) | none | about half a day a month: browser changes, push subscription expiry |
| **B. Store wrapper** | the same web frontend inside a native shell (for example Capacitor) with native push, deep links and store listings | 2 slices after A; 4–6 developer-weeks, plus store review rounds | ≈0 ₽ | Apple 99 USD a year (about 8,100 ₽ at the Bank of Russia rate of 2026-10-22 in the [competitors research](../research/competitors-2026-10-22.md#method-and-conventions)) if it can be paid; RuStore publishing cost **unconfirmed**, tools free | 2–4 days a month: OS and SDK updates, yearly certificates, store reviews of every release; a Mac for iOS builds; a new dependency and a decision record amending ADR-0002 |
| **C. Native app** | separate React Native screens for both roles over the typed API | 6–10 slices; 3–5 developer-months | ≈0 ₽ | as B | 0.25–0.5 of a developer; at an assumed 130,000–200,000 ₽ a month for a hired middle developer (the upper part of the vacancy ranges in the research file, **estimate**), the build costs about 0.4–1.0 million ₽ and support about 33,000–100,000 ₽ a month in pay alone, before employer taxes and contributions |

Crash reporting and analytics for an app would also have to stay in Russia or be self-hosted (ADR-0008); none is needed for A.

### Recommendation

- **No native app for the MVP or the pilot.** Build option A, the PWA with web push, in slice 14, or earlier with the phone layouts of slice 10 if the owner wants push in the pilot.
- **Decide on B after the pilot from measurements:** share of student and parent sessions on phones, PWA install rate, push opt-in and delivery, missed lessons with unread reminders. Proceed to B when phones dominate and web push proves unreliable on iOS, starting with Android through RuStore, while the verification rule's effect in Russia is watched; publish on iOS only once a lawful, durable way to hold the Apple membership exists (question 4).
- **Option C is not justified** at this stage: it costs months, duplicates every screen and still shows the video room in a web view.

## Questions for the owner

1. **When do the decision record on minors and the consent assessment land?** (a) before the pilot, since its students are school students (recommended); (b) with the parents slice after the pilot.
2. **Which comes first after the MVP?** (a) recordings and homework analytics, then the ledger and attendance, then parents, with the order revisited if pilot tutors still keep payments elsewhere by week 4 (recommended); (b) the ledger and parent access before recordings; (c) recordings, parents, then the ledger.
3. **Which reminder channels beyond email, and when?** (a) an installable web app with web push plus a bot in a Russian messenger, in post-pilot slice 14, with a decision record under ADR-0008 that weighs push metadata (recommended); (b) the same, with web push already in slice 10 so the pilot has it; (c) email only.
4. **Is a store app planned, and how is the Apple membership held?** (a) decided after the pilot from the measurements above, Android through RuStore first, iOS only once the owner and a lawyer settle a lawful way to pay the Apple membership (recommended); (b) a wrapper app for both stores right after the pilot; (c) a fully native app.

## The owner's answers

Given by the project owner on 2026-10-25. They are decisions in intent, not yet recorded where they take effect: each lands through a decision record or an approved roadmap change in `docs/`, as the [open implementation questions](../architecture/detailed.md#open-implementation-questions) track. Answers 2 to 4, and answer 1 as a dependency before the pilot, were recorded on 2026-10-26 in [ADR-0015](../../docs/decisions/0015-web-push.md) and the [roadmap](../../docs/roadmap.md), pending owner review.

1. **Minors:** (a), the decision record on minors and the assessment of a legal guardian's consent land before the pilot.
2. **After the MVP:** (a), recordings and homework analytics first, then the ledger with attendance, then parents, with the order revisited if pilot tutors still keep payments elsewhere by week 4 of the pilot.
3. **Reminder channels:** (b), the installable web app with web push already in slice 10, so the pilot has it; the bot in a Russian messenger comes later. This grows slice 10 by about half a slice (the web app manifest and service worker, the push subscription, and sending from the worker), needs a decision record under ADR-0008 first, because even a push without content gives the browser vendors' push services the subscription address, the server's network address and the time of sending, and needs a short installation guide for pilot students, because on iOS web push works only after the student adds OVC to the Home Screen.
4. **Store app:** (a), decided after the pilot from the measurements above; Android through RuStore first, and iOS only once the owner and a lawyer settle a lawful way to pay the Apple membership.

## Not verified

- Effort figures are estimates; agent slice durations so far cover backend and frontend work without store releases.
- Every store and push fact in the research file, in particular how Android developer verification will apply in Russia.
- Demand for each function among the pilot's tutors; the measurements above decide it.
