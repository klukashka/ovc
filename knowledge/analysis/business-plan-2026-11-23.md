# Business plan

Written 2026-11-23. Agent analysis: it informs the owner's decisions and decides nothing; it is not financial or tax advice. Figures come from the [budget, revenue model and tax regimes](budget-and-revenue-model-2026-11-23.md), competitors from the [tutor tools market research of 2026-11-23](../research/tutor-tools-market-2026-11-23.md), the [Weekify research](../research/weekify-2026-11-23.md) and the [competitors research](../research/competitors-2026-10-22.md), and the product direction from the [early-stage product strategy](early-stage-product-strategy-2026-10-25.md), [proposal 0007](../../docs/proposals/0007-tutor-crm-scope.md) and the [interface rework plan](../plans/ui-rework.md). Every market figure is a survey or a press report and shows direction, not size.

## Summary

- **Who:** independent tutors in Russia who teach online, one to one and in small groups; first tutors of school students (the pilot's segment, chosen by the owner on 2026-10-25), then tutors of adults.
- **What:** one workspace for a tutor's whole week: video lessons with a whiteboard, chat, schedule, homework that checks itself, and a tutor CRM with finances, online booking and data export, with all data in Russia.
- **Price:** about 490 ₽ a month per tutor, students free (owner, 2026-11-23); 990 ₽ and 1,490 ₽ are the comparison points.
- **Money:** about 81,000 ₽ once before the pilot, about 9,600 ₽ a month during it, about 16,000–19,000 ₽ a month from about 20 tutors. At 490 ₽ OVC covers its cash costs from about 45–60 paying tutors and the owner's time from about 270; the base scenario needs a reserve of about 260,000–390,000 ₽ and turns cash-positive in December 2028 to April 2029, depending on the tax regime.
- **Biggest risks:** the price sits against Finch and Weekify while OVC alone runs its own video; growth through free channels only; call quality on Russian networks; the legal footing for students aged 14–17; and a pilot date close to the remaining roadmap.

## Segment

| | Tutors of school students (first) | Tutors of adults (next) |
|---|---|---|
| Subjects | mathematics, Russian, physics, computer science, social studies, languages | foreign languages, professional subjects |
| Lesson price, 2026 (**third-party**) | ОГЭ about 1,500 ₽, ЕГЭ 2,000–2,500 ₽ an hour | about 1,000–1,500 ₽ an hour on average |
| Rhythm | September to June, peak in spring; pauses in summer | all year, evenings and weekends |
| Who pays the tutor | a parent | the student |
| Fit today | needs the decision on students aged 14–17 ([proposal 0003](../../docs/proposals/0003-students-aged-14-and-over.md)) and the lawyer's review | designed for it |

A tutor with 10–20 regular students earns roughly 40,000–150,000 ₽ a month (**estimate**), so 490 ₽ is about 0.3–1.2% of that income. The number of tutors in Russia is unknown; Russia had about 17 million self-employed in August 2026 (**third-party**), and tutoring is described as one of their common activities by a bank's publication cited in the [cost levers and market research](../research/cost-levers-and-market-2026-10-25.md#tutoring-market) (**third-party**), while the published breakdown of the self-employed does not list education or tutoring.

## Value

The weekly loop in one place: schedule a series, hold the lesson in the room with a whiteboard, issue homework from the library, let tests check themselves, review the rest, see who paid and who owes, and give the student one place to find everything. A tutor drops a call app, a messenger thread, shared documents and a spreadsheet or CRM. Three promises the market lacks together:

1. **Video, homework and money in one product.** No competitor researched combines a lesson room, self-checking homework and finances: Weekify has finances without video or real homework, Finch has video and finances with no self-checking homework found (**unconfirmed**), ProgressMe and Edvibe's Standard plan have rooms and exercises without finances (Edvibe's Pro plan adds payment acceptance).
2. **One student, several tutors, one account,** which also brings tutors in through their students.
3. **Data in Russia and a self-hosted room** that a messenger restriction or a foreign service's exit does not switch off; every user can download their data (FR-18).

## Competitors

| Product | Cheapest paid plan | Video | Homework | CRM and finances | Students |
|---|---|---|---|---|---|
| Weekify | 350 ₽ per 30 days | no | a text field | yes, with booking | not stated |
| ЯРепетитор | 269 ₽, or 2,390 ₽ a year (tier and period **unconfirmed**) | no | top tier only | yes | tier-dependent |
| **Finch** (finch.study) | 597 ₽ for all modules, ≈408 ₽ on a yearly plan; free up to 3 students without video | yes, with a whiteboard, groups up to 100 | a homework widget, no self-checking found | yes: payments, debts, reminders, booking | unlimited on the paid module |
| ProgressMe | about 380–990 ₽ (**third-party**) | yes | language exercises | no | plan-dependent |
| Edvibe | ≈919–1,225 ₽ | yes, no screen sharing | self-checking | no | 50 |
| Antitreningi, individual plan | 1,490 ₽ | one to one, whiteboard | AI checking | no | 20 |
| AlfaCRM, Hollihop | 1,750–2,600 ₽ | no | no | yes, built for schools | many |
| Telemost, VK Calls, Max | free | yes | no | no | — |

**Finch is the closest competitor:** an independent-tutor platform with video, whiteboard, schedule, booking and payment tracking at about OVC's planned price. OVC's answer is the homework loop (self-checking tests, a review screen, a library and builder), several tutors per student, recordings after the MVP and the export. Whether Finch adds self-checking homework is worth checking each quarter.

## Pricing

- **Base:** one plan at about 490 ₽ a month, students free, with a 30-day trial; founding tutors of 2027 keep their price for life.
- **Annual plan** at about ten months' price, which evens out the summer pauses of tutors of school students.
- **What the model shows about the price,** with the same demand assumed at every price: at 490 ₽ OVC needs about twice as many paying tutors as at 990 ₽ to cover its cash costs (about 45–60 against 21–26) and its break-even comes 13–15 months later in the base scenario. A second plan at 990 ₽, once recordings exist, tests whether part of the base pays more without moving the entry price.
- **No money moves through OVC** for tutors' lessons ([proposal 0007](../../docs/proposals/0007-tutor-crm-scope.md)); OVC's own subscription is paid through an acquirer whose fee includes fiscal receipts.

## Channels

Free channels only (owner, 2026-11-23):

1. **The pilot tutors' network** and referrals from founding tutors, with the founding offer as the reason to join early.
2. **Students with several tutors,** who add a tutor by code; the share of tutors whose first student came this way is measured (H6 of the strategy).
3. **Tutor communities** in Russian messengers and social networks and methodology associations, with the messages the strategy compares ("your whole practice in one place", "homework that checks itself", "your students' data stays in Russia").
4. **The export and the import path** as a reason to switch: tutors leaving another tool keep their data (the import itself is deferred in proposal 0007).

Paid advertising is out of the budget. For reference, education leads cost about 1,500–5,000 ₽ in VK Ads and about 3,100 ₽ in one Yandex Direct case (**third-party**), which a 490 ₽ plan repays only after many months of retention.

## Milestones

| When | Milestone | Proves | Exit criterion |
|---|---|---|---|
| Nov 2026 – Jan 2027 | Slice 10 stages in order: the interface rework, finance (FIN), booking (BOOK), task tracker (TASK), phone (PHONE), 10.4, 10.5 | the CRM scope of proposal 0007 | each stage's slice review resolved |
| before the pilot | decision record on students aged 14–17, the lawyer's review of the three documents, the pilot deployment items of ADR-0013, the conferencing probe | the legal footing and call quality | the roadmap's "before the pilot" list done |
| Feb – Apr 2027 | pilot: two tutors of school students, free | H1 (one tool), H2 (call quality) | both tutors run all OVC students' lessons and homework in OVC for four consecutive weeks |
| May – Aug 2027 | founding tutors, up to 20, free; the conferencing host on a daily window | activation and retention; H4 (willingness to pay) | 80% activation; a dated start of billing accepted by at least 60% |
| Sep 2027 | billing starts at the school year | revenue | at least 12 paying tutors (the base case) |
| Dec 2027 | first checkpoint | the scenario the business is in | under 20 paying tutors means the pessimistic path: revisit price, segment or scope |
| 2028 | after the MVP, in the order the owner chose on 2026-10-25: recordings with homework analytics, then parent access; finances already shipped in FIN | retention of paying tutors | monthly churn at most 5% |
| Dec 2028 | cash break-even at 490 ₽ under НПД (base) | the model | about 56 paying tutors |

The pilot date of January–February 2027 leaves about ten weeks for seven slice 10 stages and the pre-pilot items; if the stages slip, the founding period and billing slip with them, and billing should still start at the beginning of a school year or of a calendar term rather than mid-season.

## Risks

| Risk | Effect | Response |
|---|---|---|
| Price against Finch and Weekify | at 490 ₽ OVC carries a video host that Weekify does not, at Finch's price | differentiate on the homework loop and several tutors per student; test a 990 ₽ plan with recordings |
| Growth through free channels only | the pessimistic scenario: about ten paying tutors, never break-even | the December 2027 checkpoint; referral by students; measure each channel |
| Call quality on Russian networks (H2) | tutors return to free call tools | the probe before the pilot, TURN on TCP 443, a lesson quality rating |
| Students aged 14–17 | the pilot segment depends on the lawyer's answer | the decision record and the lawyer before the pilot; fall back to tutors of adults |
| НПД fit and its end in 2028 | higher tax or a forced move to УСН | the accountant's review before switching; plan the return to УСН |
| Seasonality | revenue dips in July and August | the annual plan; access for planning over the summer |
| Cloud incidents and prices | the October 2026 data-centre incident at Yandex Cloud lost part of customers' data (**third-party**); hardware prices doubled | backups held outside the hosting provider before paying tutors depend on OVC; recheck prices before each stage |
| The owner's time | 10 hours a week caps support and development | keep onboarding concierge-style only up to 20 tutors; write help pages from the founding tutors' questions |
| Regulatory fines | under 152-FZ, failing to notify the authority costs 100,000–300,000 ₽, a leak 3–5 million ₽ and more (**third-party**) | notify before the first deployment; the threat model's controls |

## Key metrics

From the strategy's measurement table, plus money: activation within 7 days, lessons held in OVC, lesson quality, join success, homework loop speed, paying tutors, conversion from trial, monthly churn, revenue per month, cash reserve against the model's cash need, and the share of new tutors from referrals.

## Not verified

- Every market figure and competitor price beyond the research files' checks; whether finch.study is the product intended as the comparison.
- Every scenario input of the budget file.
- Whether the slice 10 stages fit before a pilot in January–February 2027.
