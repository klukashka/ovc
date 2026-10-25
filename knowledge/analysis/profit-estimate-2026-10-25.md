# Profit estimate

Written 2026-10-25. Agent analysis: it informs the owner's decisions and decides nothing. Hosting costs come from the [hosting cost and load analysis](hosting-cost-and-load-2026-10-25.md), the price range and market from the [early-stage product strategy](early-stage-product-strategy-2026-10-25.md), the post-pilot development effort from the [post-pilot plan](post-pilot-plan-and-mobile-app-2026-10-25.md), and tax and acquiring facts from the [taxes and acquiring research](../research/taxes-and-acquiring-2026-10-25.md). Every figure is per month in roubles unless stated, and every result is an **estimate**: no tutor has paid for OVC yet.

## Inputs

- **Price:** a flat monthly plan per tutor, students free, between 990 ₽ and 1,490 ₽; the pilot is free. The project owner stated this pricing model on 2026-10-25; it is not yet recorded in a decision record, and the price itself is still a hypothesis (H4 of the strategy).
- **Hosting, under the owner's statements of 2026-10-25, not yet recorded** (the conferencing host always on during the pilot and on a daily window afterwards; PostgreSQL on the application VM until tutors pay, which conflicts, until a decision record amends it, with ADR-0013's move to the managed service once the pilot passes two tutors; managed PostgreSQL at the larger size):

  | Size | Hosting | Basis |
  |---|---|---|
  | Pilot, 2 tutors | ≈9,500 ₽ | ADR-0013 as decided |
  | ≈20 tutors | ≈14,000–17,000 ₽ | application VM with PostgreSQL ≈6,400 ₽; an 8 vCPU conferencing host on a 15-hour window, ≈7,200 ₽ with standard CPU as in the hosting analysis, or ≈9,550 ₽ with high-frequency CPU (**estimate** extending that analysis: the high-frequency host's 13,944 ₽ of CPU and RAM × 15/24, plus 836 ₽ of disk and address billed always); storage and email ≈300–1,000 ₽ |
  | ≈200 tutors | ≈42,000 ₽ | the optimised growth setup of the hosting analysis |

- **Payment fees:** 3% of revenue, a conservative assumption above the highest sourced rate (the research file finds rates from 0.4% for some methods to a 2.6% calculator example for cards, actual rates set by contract).
- **Tax:** while annual revenue stays under the 2.4 million ₽ limit of the professional income tax, 4% on payments from individuals (tutors paying as private persons) or 6% from companies or entrepreneurs; above it, an individual entrepreneur on the simplified system at 6% of revenue, with the fixed contributions (57,390 ₽ a year in 2026) and the extra 1% above 300,000 ₽ assumed to be offset against that tax. Net of fees and tax the operator keeps about 93% of revenue under the professional income tax at 4%, and about 91% as an entrepreneur.
- **Not included:** the owner's and operator's time, paid support, legal advice (the 152-FZ assessment for minors among it), customer acquisition, and hired development. These are discussed under "What the estimate leaves out".

## Scenarios

| Size | Paying tutors | Price | Revenue | Kept after fees and tax | Hosting | **Profit before people costs** |
|---|---|---|---|---|---|---|
| Pilot | 0 of 2 | free | 0 ₽ | 0 ₽ | ≈9,500 ₽ | **≈−9,500 ₽** |
| ≈20 tutors | 20 | 990 ₽ | 19,800 ₽ | ≈18,400 ₽ | 14,000–17,000 ₽ | **≈+1,400 to +4,400 ₽** |
| ≈20 tutors | 20 | 1,490 ₽ | 29,800 ₽ | ≈27,700 ₽ | 14,000–17,000 ₽ | **≈+10,700 to +13,700 ₽** |
| ≈200 tutors | 200 | 990 ₽ | 198,000 ₽ | ≈180,000–184,000 ₽ | ≈42,000 ₽ | **≈+138,000 to +142,000 ₽** |
| ≈200 tutors | 200 | 1,490 ₽ | 298,000 ₽ | ≈271,000 ₽ (entrepreneur) | ≈42,000 ₽ | **≈+229,000 ₽** |

At 200 tutors and 990 ₽ annual revenue is about 2.38 million ₽, at the edge of the professional income tax limit, so the range covers both regimes; at 1,490 ₽ it is about 3.58 million ₽ a year and needs an entrepreneur. A year at 200 tutors therefore yields about 1.7 million ₽ at 990 ₽ and about 2.7–2.8 million ₽ at 1,490 ₽, before people costs.

### Break-even

With the early-production hosting of 14,000–17,000 ₽, which stays about the same from a handful of tutors up to about 20:

| Price | Kept per paying tutor | Paying tutors to cover hosting |
|---|---|---|
| 990 ₽ | ≈920 ₽ | **16–19** |
| 1,490 ₽ | ≈1,390 ₽ | **11–13** |
| 350 ₽ (the compared tutor CRM's price) | ≈325 ₽ | about 44–53, more than the 20-tutor setup carries |

The early stage therefore needs nearly every active tutor to pay at 990 ₽ just to cover hosting; at 1,490 ₽ about 55–65% is enough. OVC cannot compete on the compared CRM's price while it runs its own video: video is what costs money, and it is also what the CRM does not offer.

### Per tutor at scale

At 200 tutors hosting is about 210 ₽ per tutor, so each paying tutor contributes about 700 ₽ at 990 ₽ and about 1,140 ₽ at 1,490 ₽ toward people costs and profit.

### Seasonality

If the project owner's statement of 2026-10-25 that students can be anyone is recorded (ADR-0005 and the product overview still say adults), many tutors on OVC will teach school students, who study from September to June. If they pause their subscriptions in July and August, annual revenue falls by about a sixth, while hosting falls only partly (the conferencing host's window saves most in summer). A year at 200 tutors of school students would then yield roughly 20% less than the figures above (**estimate**). An annual plan with a discount, or keeping access over the summer for planning and materials, softens this.

## What the estimate leaves out

- **Development after the pilot.** The post-pilot slices 12–15 take 10–14 developer-weeks for one hired developer. Assuming 130,000–200,000 ₽ a month, the upper part of the vacancy ranges in the [tutor CRM and mobile distribution research](../research/tutor-crm-and-mobile-distribution-2026-10-25.md#mobile-developer-pay-in-russia) (which also shows 70,000–110,000 ₽), that is about 30,000–46,000 ₽ a developer-week and about 0.3–0.65 million ₽ in pay alone, before payroll contributions (**estimate**): one to five months of profit at 200 tutors, and about 2 to 39 years of profit at 20. Hiring also ends the professional income tax regime, which allows no employees, so the operator would become an entrepreneur or a company. The estimate suggests keeping development with the owner and agents until OVC has well over a hundred paying tutors; that is the owner's choice.
- **Support and operations.** At 200 tutors support questions, onboarding and incidents could take a quarter to half of one person's time; whether that is the owner or a paid person decides most of the real profit.
- **Legal costs.** The decision record on minors needs a lawyer's assessment of 152-FZ consent; the privacy policy and processing instructions are owner obligations (ADR-0008). Their price was not researched.
- **Customer acquisition.** Recruiting tutors through communities and referrals costs time more than money at the early stage; paid acquisition would need its own measurement of cost per paying tutor.
- **A store app** later: about 8,100 ₽ a year for the Apple membership if it can be paid, and 2–4 days a month of upkeep (post-pilot plan).

## Conclusions

1. **The pilot is an investment of about 9,500 ₽ a month** in hosting, plus the owner's time and legal advice; it earns nothing by design.
2. **The early stage is roughly break-even.** At about 20 tutors OVC covers its hosting only if 16–19 pay at 990 ₽ or 11–13 at 1,490 ₽; the profit is at most about 14,000 ₽ a month, which does not pay for anyone's time.
3. **Meaningful profit starts around 100–200 paying tutors**: about 140,000–230,000 ₽ a month at 200 before people costs, enough for part-time support and occasional hired development.
4. **The upper price is worth testing first.** 1,490 ₽ needs about a third fewer paying tutors to break even, and the strategy's willingness-to-pay test (H4) should offer it alongside 990 ₽.
5. **Costs scale slower than revenue.** Hosting per tutor falls from about 4,750 ₽ in the pilot to about 700–850 ₽ at 20 tutors and about 210 ₽ at 200, so growth, not further optimisation, is what makes OVC profitable.

## Not verified

- Every figure: no tutor has paid; price, conversion, churn and seasonality are assumptions.
- Acquiring rates, the tax regime's fit for a subscription service, and the 2026 thresholds (research file).
- Hosting at 200 tutors depends on the slice 7 probe and the measurements in the hosting analysis.
