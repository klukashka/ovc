# Early-stage product strategy

Written 2026-10-25. Agent analysis: it informs the owner's decisions and decides nothing. OVC's intent is in the [product overview](../../docs/product.md), its scope in the [functional requirements](../../docs/requirements/functional.md), its build order in the [roadmap](../../docs/roadmap.md) and its domain in [ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md). Competing products and their prices are in the [competitors research](../research/competitors-2026-10-22.md); market figures checked for this analysis are in the [cost levers and market research](../research/cost-levers-and-market-2026-10-25.md); running costs are in the [hosting cost and load analysis](hosting-cost-and-load-2026-10-25.md). Market figures are surveys and press reports, most of them published by vendors, so they show direction, not size.

## Summary

- **Two segments, one core.** OVC serves private tutors; the project owner stated on 2026-10-25 that this includes tutors who prepare school students for exams, alongside tutors of adults. The repository still describes adults only (product overview, ADR-0005, the threat model), so the scope needs a decision record. The tutor's weekly practice is the same in both segments; what differs is who pays, who watches progress, how scores are shown and how personal data of minors is handled.
- **Recommended strategy: a focused, integrated workspace for a tutor's weekly practice, won by depth of the teaching loop rather than by video.** Video alone is free and good enough in Russia (Telemost, VK Calls, Max); what no Russian product documents is the whole loop in one place: schedule, lesson room with whiteboard, chat, homework that checks itself and a review screen, with one student studying with several tutors and data kept in Russia.
- **Sequence the segments.** Pilot with tutors of adults, for whom the product is already designed; meanwhile design the additions school exam preparation needs (a parent view, exam-style scores, minors' consent) as a proposal, and launch to exam-preparation tutors at the start of a school year, when they choose their tools.
- **Validate the riskiest assumptions first and cheaply:** call quality on Russian networks, that tutors move their lessons and homework into OVC, and that they would pay roughly what a tutor exercise platform costs today.
- **Get real lessons running earlier.** A lessons-only alpha for the two pilot tutors after slice 7 tests call quality and the lessons part of the weekly loop months before the roadmap's deployment after slice 10, at the price of moving the pilot deployment's hardening forward.
- **Price later, flat per tutor, students free.** The pilot is free; a paid plan around 990–1,490 ₽ a month is tested at about 20 tutors, when hosting falls to about 700 ₽ per tutor if levers 1–3 of the cost analysis are adopted.

## Market

### The tutor and the money

- Almost half of the tutors surveyed in September 2025 charged more than 1,000 ₽ an hour and 97% taught online fully or partly; foreign languages led demand at 31%, mathematics followed at 23% (competitors research, **third-party**).
- In 2026 the average lesson is quoted at about 1,200–2,500 ₽, core exam subjects at 3,000–3,500 ₽, and the average bill rose 10–20% (**third-party**). A tutor with 10–20 regular students therefore earns roughly 40,000–150,000 ₽ a month (**estimate**), so a tool at 1,000 ₽ is about 0.7–2.5% of income.
- Demand for exam preparation rose 54% year on year in January–April 2026 by one marketplace's figures (**third-party**).
- Adults name foreign languages as the subject they most want to study online, 38% in the two capitals and first in the regions, and say they would spend up to about a quarter of their income on learning (a platform vendor's survey published in January 2026, **third-party**).
- The number of tutors in Russia, and their split between adults and school students, is unknown; tutoring is one of the most common activities of the more than 15 million self-employed (**third-party**). Market size is therefore not estimated here; the pilot and the go-to-market tests measure reachable demand directly.

### Two segments

| | Tutors of adults | Tutors preparing school students for exams |
|---|---|---|
| Typical subjects | foreign languages, then professional subjects | mathematics, Russian, physics, computer science, social studies, languages |
| Who pays and decides | the student | a parent, often with the student |
| Lesson price (**third-party**) | about 1,200–2,500 ₽ | core exam subjects about 3,000–3,500 ₽ |
| Rhythm | all year, evenings and weekends | September to June, after school and at weekends, peaking in spring; most students leave after the exams |
| Format | one to one, some small groups | one to one and mini-groups, many practice tests |
| What the tutor needs beyond the core | recordings to rewatch (hypothesis), payments | a parent's view of attendance, homework and progress; exam-style scores; exam task banks |
| Fit with the current design | designed for it | needs the additions below |

**What exam preparation adds to the product.** The current design was written for adults, and three of its decisions conflict with this segment:

- **Personal data of minors.** ADR-0003 once said students may be minors; ADR-0005 replaced it with adults. Serving school students needs a lawyer's assessment of consent by a parent or other legal representative under 152-FZ, an age question at sign-up and the threat model's "personal data of adults" widened.
- **A parent.** OVC has two roles, tutor and student, and every account has exactly one (FR-1.3). A parent who sees attendance, homework and progress and who pays is a third party to the relationship, which ADR-0005's relationship scope does not cover.
- **Scores.** Points and scores never reach students ([ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md)), a choice motivated by adult students focusing on feedback. Exam preparation is measured in exam points, and students and parents expect to see them.

None of these is needed for the pilot with tutors of adults; each is a proposal-sized change, best designed once the core works.

### Competing and adjacent tools

From the competitors research, grouped by the job they do for a tutor:

| Group | Examples | Price to the tutor | Strength | Where OVC differs |
|---|---|---|---|---|
| Free video calls | Yandex Telemost, VK Calls, Max | 0 ₽ | free, familiar, native apps | no whiteboard (or a basic one), no homework, no student records |
| Free tutor workspace from a large company | VK Tutoria (status in 2026 **unconfirmed**) | 0 ₽ (2024) | calendar, board, messenger, calls, groups | no self-checking homework documented; VK ID sign-in |
| Tutor exercise platforms with a room | ProgressMe, Edvibe | about 1,000 ₽, up to 3,750 ₽ for many students | ready-made language content, many exercise types, AI generation | OVC has no content library but documents scheduling, chat, groups and several tutors per student together |
| Online-school platforms | GetCourse, Skillspace, Antitreningi | 1,500–6,100 ₽ | courses, payments, funnels, webinars | built for selling courses to many, not for a weekly one-to-one practice |
| CRM for lessons and payments | AlfaCRM | 2,600–3,800 ₽ | balances, payments, messenger notifications, booking | no video room or homework |
| Foreign tools | Zoom, Google Classroom, Miro, Lessonspace, Preply | unpayable or blocked for many | mature products | data abroad; Miro blocked in Russia since October 2026; Russian cards not accepted |

Mass online exam-preparation schools compete for the same school students and families, not for the tutor's tools; they were not researched here (**unconfirmed**). A tutor today most likely combines a free call tool, a messenger, shared documents or forms for homework and a spreadsheet or CRM for payments (**estimate**, no usage survey found).

### Pains of the target tutor

From the product principles, the competitor gaps and the market context (**estimate** until the interviews below confirm or reorder them):

1. **Tool sprawl:** a lesson link in one app, homework in another, messages in a third, payments in a spreadsheet; the student asks "where was that file?".
2. **Checking homework takes evenings:** collecting answers, scoring tests by hand, returning corrected files; heaviest in exam preparation, where practice tests are frequent.
3. **Tools disappear or degrade:** messenger calls restricted in 2025, Miro blocked in 2026, foreign payments closed; tutors want something that will not be switched off.
4. **Students who drift:** missed lessons and unfinished homework that the tutor notices late; in exam preparation, parents who ask how their child is doing.
5. **Data and legal unease:** student data in foreign services, which part 5 of article 18 of 152-FZ restricts for operators; sharper for minors' data.
6. **Payments and lesson balances:** tracking who paid for how many lessons, the reason tutors buy a CRM (after the MVP for OVC, [proposal 0001](../../docs/proposals/0001-payment-ledger.md)).

### Pricing models in the market

- **Free, funded by an ecosystem:** call tools and VK Tutoria.
- **Flat per tutor with a student ceiling:** ProgressMe, Skillspace, GetCourse's smallest plans; the common model at 10–50 students.
- **Per active student or per lesson:** ProgressMe's per-student scale, Teachworks, Lessonspace's per-hour billing.
- **Commission on payments:** marketplaces (Preply, closed to tutors in Russia) and Edvibe's payment balances.

## Positioning

**For** independent tutors who teach online, one to one and in small groups, **who** juggle a call app, a messenger, documents and a spreadsheet, **OVC is** one calm workspace where every lesson, message and assignment of a practice lives together, **unlike** free call tools, which stop at the call, and exercise platforms, which centre on their own content, **because** the weekly loop (schedule, lesson, homework that checks itself, review, feedback) takes one action at each step, a student can study with several tutors under one account, and all data stays in Russia.

The message per segment: for tutors of adults, "your whole practice in one place"; for exam-preparation tutors, "homework and practice tests that check themselves, and parents who see progress without asking", once those additions exist.

What OVC deliberately does not compete on in the early stage: free unlimited video, a content library, a marketplace that brings students, native apps.

## Go-to-market for the pilot

1. **Two pilot tutors from the owner's network** (ADR-0013), one-to-one lessons only, free. Weekly 30-minute calls with each tutor, a shared log of problems, and the measurements below. Exit criterion: both tutors run all their OVC students' lessons and homework through OVC for four consecutive weeks. Which segment the pilot tutors come from is an owner question below; tutors of adults fit the current design without changes.
2. **Then about 20 hand-picked tutors**, tutors of adults first (languages), each teaching 5–25 students online, recruited through tutor communities in Russian messengers and social networks, methodology associations and referrals from the pilot tutors. Concierge onboarding: the operator works alongside each tutor in the tutor's own signed-in session (with the tutor present), helps move the first materials into the library and schedules the first weekly series; operator actions exist only in `ovc-admin`, so the operator never acts on a tutor's data through a surface of its own.
3. **Interviews with 5–10 exam-preparation tutors** in parallel, before any design: how they work with parents, which scores they show, what they use for practice tests, when in the year they choose tools (H8).
4. **Founding-tutor offer** at the 20-tutor stage: free until the paid plan launches, then a lifetime discount for those who stay, in exchange for monthly feedback; it is also the willingness-to-pay test (H4).
5. **Exam-preparation launch** at the start of a school year (late August to September), when tutors set up their year, once the proposal for parents, scores and minors' consent has been decided and built; launching mid-season asks tutors to move students in the middle of their preparation.
6. **Messages that the tests compare:** "your whole practice in one place", "homework that checks itself", "your students' data stays in Russia", "nothing here will be switched off".
7. **Built-in spread:** a student who studies with several tutors (FR-2.4) can add another tutor by code; a tutor who receives a request from a student already on OVC is a measurable referral path (H6). School students preparing for several exams often have several tutors, which strengthens this path in that segment.

## Development plan

### Before and during the MVP: what to build

The roadmap's slices 4–10 remain the MVP. The table names, for each slice, what it proves for the strategy and what this analysis proposes in addition; every item marked **new scope** is not in the functional requirements and needs an owner decision and a requirement change before it is built.

| Slice | What it proves for the strategy | Proposed in addition |
|---|---|---|
| 4 Realtime and in-app notifications | the platform feels alive | nothing |
| 5 Schedule and reminders | the tutor's week lives in OVC | **new scope:** a forecast of concurrent lessons per 15-minute slot for capacity planning; reminder email limits checked against the provider's quota |
| 6 Chat and files | the messenger can be dropped for teaching | nothing |
| 7 Video lessons | the call tool can be dropped | the probe additions of the cost analysis (boot time, capacity at two to eight lessons, TURN on TCP 443); **new scope:** a one-tap lesson quality rating and a client join-timing beacon to OVC's own API; then a **lessons-only alpha for the two pilot tutors** (owner decision) |
| 8 Library and builder | building exercises once is fast | import of existing materials kept simple; templates for the most common exercise types (content to start with is a competitor gap) |
| 9 Issuing, completing and checking | homework stops eating evenings | the review screen's speed is the main tutor-facing metric |
| 10 Results, home pages and hardening | the student never wonders what to do | phone layouts, because students often join from a phone; the pilot deployment items, moved earlier if the alpha is approved |
| After the MVP | | **new scope**, if the owner adopts lever 1 of the cost analysis: the job that starts and stops the conferencing host around the schedule |

**The lessons-only alpha** after slice 7 can test H2 and the lessons half of H1 only: homework is issued and checked from slices 8 and 9 on. It is a first deployment before slice 10, against ADR-0013's statement that the first deployment comes after slice 10, so it needs that statement amended and ADR-0013's and the roadmap's pre-deployment items moved forward: WAL-G backups with a tested restore, the non-superuser database role and bounded logs, the host firewall and loopback-only PostgreSQL, the bounce intake from Postbox's events, Postbox's confirmation that it keeps message logs for at most 30 days, and the production-serving items of the reverse proxy. Each of those moves is an amendment of the roadmap and of ADR-0013's "before the first deployment".

### What to measure from the first lesson

| Metric | Definition | Pilot target (proposal) |
|---|---|---|
| Activation | a new tutor schedules a weekly series and holds a lesson in OVC within 7 days of sign-up | 80% of pilot tutors |
| Lessons held in OVC | share of a tutor's lessons with OVC students held in OVC's room rather than another tool | at least 80% after 4 weeks |
| Lesson quality | lessons rated with a problem or moved to another tool | under 2% |
| Join success | join attempts inside the window that reach the room | at least 99% |
| Homework loop | assignments issued per active student per week; share submitted before the deadline; median time from submission to review | issued weekly; review time falling after week 2 |
| Library reuse | share of issues made from an existing library item | rising week on week |
| Retention | tutors active in week 8 of those active in week 1; for exam-preparation tutors, measured within the school year, since students leave after the exams | both pilot tutors; 70% at 20 tutors |
| Tool replacement | tools the tutor stopped using, asked monthly | at least one per tutor by week 8 |

Measurements use OVC's own data and logs inside Russia; no foreign analytics ([ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md)). Homework analytics for tutors (FR-11.2 to FR-11.4) stays after the MVP. These are operator measurements over identifiers and counts only; since operator actions exist only in `ovc-admin`, they need a read-only `ovc-admin` report (**new scope**), not ad-hoc queries against the production database.

### What to defer, and when to revisit

| Deferred | Revisit when |
|---|---|
| Recordings (FR-5.4 to FR-5.6) | first after the MVP by the roadmap; confirm with H5 |
| Payments and lesson balances ([proposal 0001](../../docs/proposals/0001-payment-ledger.md)) | H7 shows tutors keep a CRM only for payments; in exam preparation, parents pay, which shapes the ledger |
| A parent view, exam-style scores, minors' consent | written as a proposal once the owner records the school-student scope; built before the exam-preparation launch |
| Notifications in a messenger | H9 shows email reminders are missed; only a Russian messenger fits ADR-0008 without a cross-border transfer assessment |
| Native apps and push | phone layouts of slice 10 prove insufficient in the pilot |
| Ready-made content, exam task banks and AI generation | after the library's import and templates, if H3 shows building exercises is the bottleneck |
| Self-booking of slots, calendar export | tutors ask for them in interviews more than once |
| Several API instances, a second conferencing host | the load signals of the cost analysis cross their thresholds |

## Hypotheses and tests

Each hypothesis is stated so that a measurement can reject it. The first three are the riskiest and are tested first.

| # | Hypothesis | Test | Rejected if |
|---|---|---|---|
| H1 | Tutors move both lessons and homework into one tool when it works; integration beats free video | lessons held in OVC (from the alpha on) and homework issued through OVC (from slice 9 on) per pilot tutor over 8 weeks; the monthly tool-replacement question | under 80% of lessons in OVC after 4 weeks, or homework still sent through other tools by both pilot tutors at week 8 |
| H2 | A self-hosted conferencing host gives call quality on Russian home and mobile networks good enough that tutors do not switch back | slice 7 probe (two to eight lessons, TURN on TCP 443), then the alpha's join success and quality rating | join success under 99%, or over 2% of lessons with a problem or moved to another tool |
| H3 | The homework loop saves a tutor time they notice | median time from submission to review and the share of issues reused from the library, weeks 1–8; a timed interview task "issue last week's homework again" | review time does not fall, or tutors say homework takes as long as before |
| H4 | Tutors pay about 990–1,490 ₽ a month, flat, for the whole workspace | at about 20 tutors: a price page with plans at 990 ₽ and 1,490 ₽ and the founding-tutor offer with a dated start of billing; share who agree to pay | under 30% of active tutors agree at 990 ₽ |
| H5 | Students want to rewatch lessons, so recordings are the first post-MVP feature | question in the post-lesson rating ("would you rewatch this lesson?"); tutors' reports of students asking for recordings | under 20% of ratings say yes |
| H6 | Students with several tutors bring tutors in | count of tutors whose first student arrived by a code from a student already on OVC; "how did you hear" at sign-up | no such tutor among the first 20 |
| H7 | Payments and lesson balances are what keeps tutors in a second tool | interview: which tool would you drop if OVC tracked payments; share still paying for a CRM | fewer than a third name payments as the reason |
| H8 | Exam-preparation tutors adopt OVC only with a parent view and exam-style scores | interviews with 5–10 such tutors: rank the parent view, scores, practice tests and task banks against the core; share who would start without them | more than half would start with the core alone, which would let the segment start earlier |
| H9 | Email reminders are enough | reminder open rate, missed lessons per 100 lessons, interview | missed lessons above 5% with reminders unread |
| H10 | "Data in Russia" and "nothing will be switched off" persuade tutors | message comparison in recruiting posts: replies per message | no difference from the other messages |

**Top three to test first:** H2 (call quality, the gate for everything), H1 (one tool for lessons and homework) and H4 (willingness to pay at a price that covers hosting).

## Questions for the owner

Each question offers options with a recommendation; none is decided by this file.

1. **How is the school-student scope recorded?** (a) a new decision record amending ADR-0005, the product overview and the threat model: OVC serves tutors of adults and of school students, with the additions for exam preparation designed as a proposal and scheduled after the MVP core (recommended); (b) the same, with the additions built inside the MVP before the pilot; (c) keep adults only for now.
2. **Which tutors join the pilot?** (a) tutors of adults, who fit the current design (recommended); (b) exam-preparation tutors, which needs the parent, scores and consent questions answered first; (c) one of each.
3. **When do the pilot tutors start real lessons?** (a) a lessons-only alpha after slice 7, moving the pilot deployment's pre-deployment items forward (recommended: tests H2 and the lessons half of H1 months earlier); (b) after slice 10 as the roadmap says.
4. **How are scores shown to school students?** (a) decided in the exam-preparation proposal after the H8 interviews, ADR-0009 unchanged until then (recommended); (b) exam-style scores visible to school students and parents, with ADR-0009 amended now; (c) scores stay hidden in every segment.
5. **How is OVC priced after the pilot?** (a) free pilot, then a flat per-tutor plan at 990–1,490 ₽ with students free, tested at about 20 tutors (recommended); (b) per active student; (c) free core with paid add-ons such as recordings or a parent view.
6. **Which post-MVP feature comes first?** (a) decided by pilot data from H5, H7, H8 and H9, with recordings as the default (recommended); (b) recordings, as the roadmap says, regardless of data; (c) payments and lesson balances.
7. **Is the conferencing host run only around lessons?** Covered with its options in the [hosting cost and load analysis](hosting-cost-and-load-2026-10-25.md#lever-1-run-the-conferencing-host-around-the-schedule).

## Not verified

- Every market figure (surveys and press reports, several by vendors); the number of tutors and their split between adults and school students.
- VK Tutoria's status in 2026, ProgressMe's current prices, and the mass online exam-preparation schools, which were not researched.
- The pains' order, the tool combinations tutors use today and the seasonal rhythm of exam preparation, until the pilot and the interviews.
- Legal consequences of serving minors, which need a lawyer.
