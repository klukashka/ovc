# Budget, revenue model and tax regimes

Written 2026-11-23. Agent analysis: it informs the owner's decisions and decides nothing. It is not financial or tax advice; the final choice of a tax regime belongs with an accountant. It replaces the figures of the [profit estimate](profit-estimate-2026-10-25.md) and extends the [hosting cost and load analysis](hosting-cost-and-load-2026-10-25.md), whose levers it reuses. Unit prices come from the [hosting and service prices of 2026-11-23](../research/hosting-and-services-prices-2026-11-23.md), tax, payment and legal facts from the [taxes, payments and legal research of 2026-11-23](../research/taxes-payments-legal-2026-11-23.md), and competitor prices from the [tutor tools market research of 2026-11-23](../research/tutor-tools-market-2026-11-23.md). The [business plan](business-plan-2026-11-23.md) builds on this file. Every figure is in roubles, per month and including VAT unless stated, and every result is an **estimate**: no tutor has paid for OVC yet.

## Inputs stated by the owner on 2026-11-23

- **Price:** a flat plan per tutor, students free; the base case is 490 ₽ a month, and 990 ₽ and 1,490 ₽ are modelled for comparison.
- **Timing:** the pilot with two tutors of school students starts in January–February 2027 (modelled from February).
- **Operator:** the owner's individual entrepreneur (ИП) on the simplified system (УСН), which has no income other than OVC ([ADR-0024](../../docs/decisions/0024-legal-documents-and-student-data.md) makes it the operator).
- **Owner's time:** counted as a separate line, not in the cash break-even: 10 hours a week at 1,500 ₽ an hour, about 65,000 ₽ a month.
- **Not counted:** subscriptions to AI development tools (a personal expense) and paid acquisition (free channels only).
- **Earlier owner decisions still applied** (2026-10-25): the conferencing host always on during the pilot and on a fixed daily window afterwards; PostgreSQL on the application VM until tutors pay, then the managed service. They are recorded today only as owner statements in the [hosting cost and load analysis](hosting-cost-and-load-2026-10-25.md) and the [profit estimate](profit-estimate-2026-10-25.md), not in a decision record; the price of about 490 ₽ and the rule of free channels only likewise live only in `knowledge/` until a decision record adopts them.

## Assumptions added by this analysis

- **Lawyer:** one review of the privacy policy, consent text and user agreement plus a written opinion on students aged 14–17, as [ADR-0024](../../docs/decisions/0024-legal-documents-and-student-data.md) and [proposal 0003](../../docs/proposals/0003-students-aged-14-and-over.md) require before the first deployment: 45,000–160,000 ₽, base 80,000 ₽, paid in January 2027. The research found document packages from about 11,500 ₽, a user agreement from 15,000 ₽, opinions at 15,000–25,000 ₽ and estimated a specialised opinion on minors at 30,000–100,000 ₽ (all **third-party** or **unconfirmed**).
- **Payments:** Robokassa as the reference acquirer, because its fees include fiscal receipts, so no separate online cash register is needed: about 3.5% blended at small turnover (cards 2.9–3.9%, the Faster Payments System 1.8–2.7% for an ИП; 3.4% and 3.0% for the self-employed). A provider that charges VAT on its fee costs 1.22 times its quoted rate from 2026.
- **Contributions of the ИП:** 61,154 ₽ for 2027 (published), assumed 65,000 ₽ for 2028 and 69,000 ₽ for 2029 (**estimate**, indexation).
- **Bank account and accounting:** a zero-fee account and the owner keeping the books of a one-person ИП; an accountant's subscription (about 500–6,000 ₽ a month, **third-party**) is optional and not in the totals.

## One-time costs before the pilot

| Item | Base | Range | Note |
|---|---|---|---|
| Lawyer: three documents and the opinion on minors | 80,000 ₽ | 45,000–160,000 ₽ | before the first deployment |
| Notification of the data-protection authority | 0 ₽ | 0–4,900 ₽ | no state fee found; an agency charges about 4,900 ₽ |
| Domain, first year | 200 ₽ | 169–747 ₽ | renewal about 400–1,200 ₽ a year |
| VM hours for the restore drill and the conferencing probe | 1,000 ₽ | 500–3,000 ₽ | hourly billing |
| **Total** | **≈81,000 ₽** | **≈46,000–169,000 ₽** | |

## Monthly costs per stage

### The configurations

| Component | Pilot, 2 tutors (ADR-0013) | ≈20 tutors, before billing | ≈20 tutors, paying | ≈200 tutors |
|---|---|---|---|---|
| Application VM | 2 vCPU / 4 GB, with PostgreSQL: 3,499 ₽ | 4 vCPU / 8 GB, with PostgreSQL: 6,808 ₽ | 2 vCPU / 4 GB: 3,499 ₽ | API 4/8 6,808 ₽ + worker 2/4 3,499 ₽ |
| PostgreSQL | on the VM | on the VM | managed, 2 vCPU / 8 GB: 5,941 ₽ | managed with a replica: ≈12,830 ₽ (October figure) |
| Conferencing host | 4 vCPU / 8 GB, always on: 5,901 ₽ | 8 vCPU / 16 GB standard, 15-hour window: 7,167 ₽ | same: 7,167 ₽ | two high-frequency hosts, one on the window and one at peaks: 9,552 + ≈5,020 ₽ |
| TURN relay on TCP 443 | — | 1,360 ₽ | 1,360 ₽ | 1,360 ₽ |
| File storage and backups | ≈60 ₽ | 100 GB: ≈240 ₽ | ≈240 ₽ | 1 TB: ≈2,400 ₽ |
| Email | Postbox free tier: 0 ₽ | 10,000 a month: 643 ₽ | 643 ₽ | 50,000 a month: 2,750–3,450 ₽ |
| DNS and domain | ≈140 ₽ | ≈140 ₽ | ≈140 ₽ | ≈140 ₽ |
| **Total** | **≈9,600 ₽** | **≈16,400 ₽** | **≈19,000 ₽** | **≈44,400–45,100 ₽** |
| Per active tutor | ≈4,800 ₽ | ≈820 ₽ | ≈950 ₽ | ≈225 ₽ |

The scenario model below uses 16,300 ₽, 18,900 ₽ and 45,300 ₽ for the last three columns; the difference moves no break-even count by more than one tutor. The pilot's optimised variant, with the conferencing host up about 30% of the month, is about 6,000 ₽. Between 40 and 200 active tutors the model interpolates linearly between the last two columns, and above 200 adds about 150 ₽ per tutor.

### Conferencing options

The conferencing host is the largest line at every stage. Prices are VK Cloud's unless stated; disk and the public address stay billed while a host is stopped.

| Option | Pilot | ≈20 tutors | ≈200 tutors | What it costs besides money |
|---|---|---|---|---|
| Always on | 4/8 standard: 5,901 ₽ | 8/16 standard 10,965 ₽, high-frequency 14,781 ₽ | two high-frequency: 29,562 ₽ | nothing; ADR-0013 as decided |
| On a daily window or the schedule | 30% of hours: ≈2,356 ₽ | 62.5% window: 7,167 ₽ standard, 9,552 ₽ high-frequency | window + peak host: ≈14,570 ₽ | a cloud credential that can stop the host, boot time, start capacity; amends ADR-0013 (owner chose it for after the pilot) |
| Dedicated server | 8 cores / 32 GB at Selectel ≈11,000 ₽ (**unconfirmed**; tables were being updated) | same, or Timeweb 10 cores ≈15,900 ₽ | two servers ≈22,000–32,000 ₽ | cannot be stopped to save money; real cores for audio quality; ADR-0004 names this shape |
| Peer-to-peer for one-to-one lessons, the host only for groups | TURN ≈1,360 ₽ + host on demand | TURN 1,360–2,720 ₽ + small host on demand ≈2,400 ₽ | 2–3 TURN VMs + window host ≈9,000–14,000 ₽ (**estimate**) | **new scope**: a media path of OVC's own, without BigBlueButton's whiteboard, presentation and recording in one-to-one lessons, which the product promises; changes ADR-0004 and the threat model |

The peer-to-peer option is the cheapest on paper and the most expensive in development: it would rebuild what BigBlueButton gives for free in a lesson. The cheapest option that keeps the product as designed is the daily window, which the owner already chose for after the pilot; the dedicated server becomes interesting only if the probe shows that shared vCPUs degrade audio.

### Fixed costs outside hosting

| Item | ИП on УСН | ИП on the professional income tax (НПД) |
|---|---|---|
| Contributions of the ИП | 61,154 ₽ in 2027, due without income | none (voluntary only) |
| Tax | 6% of revenue, reduced by the contributions | 4% of payments from individuals, 6% from companies and ИП; this analysis assumes every tutor pays as an individual, which is **unconfirmed** for a self-employed tutor paying by card and false for a tutor paying from an ИП account |
| Online cash register | not needed with Robokassa's receipts; otherwise about 800–3,000 ₽ a month plus a fiscal drive | not needed (receipts in «Мой налог») |

## Tax regimes

### The rule that matters for a one-person ИП on УСН

An ИП without employees reduces the 6% tax by all contributions paid. The total of tax and contributions is therefore the larger of the contributions (61,154 ₽ plus 1% of income above 300,000 ₽ in 2027) and 6% of revenue. Below about 1.16 million ₽ of revenue a year (with the 2027 contributions; 0.05 × revenue = 58,154 ₽) the ИП pays exactly its contributions, a fixed cost of about 5,100 ₽ a month; above it, effectively 6% of revenue. Since OVC is the ИП's only income, the contributions are entirely OVC's cost.

### Comparison by annual revenue (2027 rates)

| Annual revenue | Paying tutors at 490 / 990 / 1,490 ₽ | УСН 6%, with contributions | НПД, first year (10,000 ₽ deduction) | Difference |
|---|---|---|---|---|
| 0 ₽ | — | 61,154 ₽ | 0 ₽ | 61,154 ₽ |
| 300,000 ₽ | 51 / 25 / 17 | 61,154 ₽ | 9,000 ₽ | 52,154 ₽ |
| 600,000 ₽ | 102 / 51 / 34 | 64,154 ₽ | 18,000 ₽ | 46,154 ₽ |
| 1,200,000 ₽ | 204 / 101 / 67 | 72,000 ₽ | 38,000 ₽ | 34,000 ₽ |
| 2,400,000 ₽ (the НПД limit) | 408 / 202 / 134 | 144,000 ₽ | 86,000 ₽ | 58,000 ₽ |
| 3,600,000 ₽ | 612 / 303 / 201 | 216,000 ₽ | not available | — |

The automated simplified system (АУСН, 8% of income, replacing the contributions) is cheaper than УСН 6% below about 830,000 ₽ a year (0.08 × revenue = 61,154 ₽ + 1% of revenue above 300,000 ₽), where every scenario sits through 2027, the regime's last year, and dearer above it; at every revenue it costs twice НПД's 4%. It is left out of the recommendation because the regime runs only until 2027-12-31 and only in regions that adopted it, but it is the fallback to check if a subscription service does not fit НПД.

### What the regimes mean for OVC

- **НПД is cheaper at every stage the scenarios reach before 2029,** by about 35,000–60,000 ₽ a year, and needs no online cash register. In the base scenario at 490 ₽ OVC's revenue stays under 600,000 ₽ a year until 2029, so the ИП on УСН pays about 61,000–69,000 ₽ a year of contributions for revenue that would owe 4% under НПД.
- **An ИП can apply НПД and stays an ИП,** so ADR-0024's operator does not change. The procedure and deadlines for moving an ИП from УСН to НПД, including any notice that the simplified system ends, are not in the research and are **unconfirmed**; missing such a deadline can leave both regimes applied, so the accountant confirms them before the switch. The research covers only the opposite move: 20 days to notify a move to УСН after the НПД limit is exceeded.
- **The limits of НПД:**
  - The 2.4 million ₽ annual limit is about 408 paying tutors at 490 ₽, 202 at 990 ₽ and 134 at 1,490 ₽; beyond it, the ИП has 20 days to notify a move to УСН, otherwise it falls to the general regime until the year ends.
  - No employees.
  - The regime is an experiment until 2028-12-31; its extension is not known.
  - Years on НПД earn no pension rights unless contributions are paid voluntarily.
  - Whether subscriptions to one's own online service fit НПД is **unconfirmed**: Ministry of Finance letters allow selling rights to one's own software and forbid reselling software, but none names a subscription service. A written answer from the tax office or an accountant's opinion settles it before relying on it.
- **УСН 6% wins** once revenue passes the НПД limit, when the ИП hires anyone, or if НПД is not extended beyond 2028. The VAT threshold of the simplified system stays at 20 million ₽ until 2029, far above every scenario.

**Recommendation for the accountant's review:** move the ИП to НПД before the pilot, while revenue is zero, and return to УСН 6% when the annual revenue forecast approaches about 2 million ₽, when the first employee is hired, or at the end of 2028 if the experiment is not extended. Confirm first that a subscription service fits НПД and the exact switch procedure both ways.

## Revenue model

### Scenarios

| Assumption | Pessimistic | Base | Optimistic |
|---|---|---|---|
| Pilot, February–April 2027 | 2 tutors, free | same | same |
| Founding tutors, May–August 2027 | grow to 20, free | same | same |
| Share of founding tutors paying from September 2027 | 40% | 60% | 75% |
| New trial tutors a month from September 2027 | 4, flat | 10, growing 4% a month | 20, growing 6% a month |
| Trial tutors who start paying | 25% | 35% | 45% |
| Monthly churn of paying tutors | 8% | 5% | 3% |
| Seasonality (tutors of school students) | trials at 30% in June–August and 150% in September; 30% of paying tutors pause in July and August | same | same |

Billing starts in September 2027, at the start of the school year, when tutors of school students set up their year. The 4-month founding period is the willingness-to-pay test of the [early-stage strategy](early-stage-product-strategy-2026-10-25.md) (H4), whose rejection criterion is stated at 990 ₽; at a base price of 490 ₽ that hypothesis needs restating for the price the founding offer names. Trial tutors cost hosting without paying; the model counts them as active.

**The same tutor counts are used at every price.** Trial inflow, conversion and churn do not depend on the price in this model, because no measurement of how demand responds to price exists yet. In reality a higher price likely converts fewer trial tutors; the comparisons between prices below show what each price needs, not what each price would get.

### Break-even

Paying tutors needed for monthly revenue, after fees and tax, to cover hosting at the paying-stage configuration, with every active tutor paying:

| Price | УСН, cash | НПД, cash | УСН, with the owner's 65,000 ₽ | НПД, with the owner's 65,000 ₽ |
|---|---|---|---|---|
| 490 ₽ | 57 | 44 | ≈274 | ≈266 |
| 990 ₽ | 26 | 21 | ≈106 | ≈104 |
| 1,490 ₽ | 17 | 14 | ≈66 | ≈64 |

At 20 paying tutors under УСН the month ends at about −14,600 ₽ at 490 ₽, −5,000 ₽ at 990 ₽ and +4,600 ₽ at 1,490 ₽; at 200 paying tutors at about +43,000 ₽, +134,000 ₽ and +224,000 ₽, before the owner's time. At 490 ₽ the ≈20-tutor stage cannot cover its own hosting; OVC needs about 45–60 paying tutors just to stop losing money.

### Results, February 2027 to December 2029

"Cash need" is the deepest point of cumulative cash flow, including the one-time costs and the contributions or tax, excluding the owner's time; "first positive month" is the first month after billing starts whose cash flow is positive, with paying tutors in brackets.

| Price | Scenario | Paying, Dec 2028 / Dec 2029 | УСН: cash need, first positive month | НПД: cash need, first positive month |
|---|---|---|---|---|
| 490 ₽ | pessimistic | 10 / 11 | ≈770,000 ₽ and still growing; never | ≈580,000 ₽ and still growing; never |
| 490 ₽ | base | 56 / 98 | ≈390,000 ₽; April 2029 (71) | ≈260,000 ₽; December 2028 (56) |
| 490 ₽ | optimistic | 184 / 420 | ≈250,000 ₽; February 2028 (75) | ≈190,000 ₽; January 2028 (65) |
| 990 ₽ | pessimistic | 10 / 11 | ≈640,000 ₽ and growing; never | ≈460,000 ₽ and growing; never |
| 990 ₽ | base | 56 / 98 | ≈230,000 ₽; January 2028 (28) | ≈180,000 ₽; November 2027 (22) |
| 990 ₽ | optimistic | 184 / 420 | ≈215,000 ₽; November 2027 (46) | ≈175,000 ₽; November 2027 (46), passes the НПД limit in 2029 |
| 1,490 ₽ | pessimistic | 10 / 11 | ≈510,000 ₽ and growing; never | ≈330,000 ₽ and growing; never |
| 1,490 ₽ | base | 56 / 98 | ≈215,000 ₽; November 2027 (22) | ≈175,000 ₽; November 2027 (22) |
| 1,490 ₽ | optimistic | 184 / 420 | ≈215,000 ₽; November 2027 (46) | ≈175,000 ₽; November 2027 (46), passes the НПД limit in 2029 |

About 175,000 ₽ of every cash need is spent before the first payment: the lawyer, three months of the pilot and four months of the ≈20-tutor setup. The owner's time adds about 65,000 ₽ a month on top, about 2.3 million ₽ over the 35 months.

### What the model says

1. **For the same demand, the price decides more than the tax regime.** At 490 ₽ the base scenario breaks even about 15–19 months after billing starts and needs the deepest cash reserve; at 990 ₽ and 1,490 ₽ it breaks even within a few months of billing.
2. **The pessimistic scenario never breaks even at any price:** about ten paying tutors stay below the cost of the ≈20-tutor setup. Its warning sign is visible by December 2027: under 20 paying tutors after the founding period.
3. **НПД lowers the cash need by about 40,000–130,000 ₽** in the scenarios that reach break-even, and by about 180,000–190,000 ₽ in the pessimistic ones by the end of 2029, mostly the contributions that УСН charges while revenue is small.
4. **At 490 ₽ OVC sits between Weekify (350 ₽) and Finch (597 ₽, or about 408 ₽ yearly),** which offer CRM features without self-hosted video (Weekify) or with video and a whiteboard (Finch). OVC carries a conferencing host that Weekify does not; at that price, growth, not optimisation, has to pay for it.
5. **Covering the owner's time needs about 270 paying tutors at 490 ₽, about 105 at 990 ₽ and about 65 at 1,490 ₽.** Only the optimistic scenario reaches 270 at 490 ₽ within the period, in 2029.

## Levers on the budget

- **Price:** an annual plan at about ten months' price evens out the summer pauses and brings cash forward; a second plan at 990 ₽ with recordings once they exist tests whether part of the base pays more.
- **Hosting before billing:** keep the pilot's application VM and PostgreSQL on it until billing starts, and start the conferencing window only when more than two tutors teach; this moves about 30,000 ₽ out of the period before the first payment.
- **The tax regime:** НПД while revenue is small, as above, after the accountant's review.
- **The lawyer:** a single engagement for the three documents and the opinion on minors, agreed at a fixed price.

## Not verified

- Every scenario input: conversion, churn, trial inflow and seasonality are assumptions until the founding period measures them.
- The fit of a subscription service under НПД, the switch procedure between НПД and УСН, and whether НПД is extended beyond 2028.
- The dedicated server price at Selectel, the managed PostgreSQL replica price (October figure), Robokassa's rates at OVC's real turnover, and the lawyer's price for OVC's documents.
- Contributions for 2028 and 2029, and cloud prices beyond November 2026; analysts expect rises after server hardware prices roughly doubled, and no provider has announced one.
