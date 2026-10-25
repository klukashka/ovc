# Cost levers and the tutoring market: facts checked on 2026-10-25

Checked 2026-10-25. Prices, billing rules, survey figures and product availability are volatile; recheck them before relying on them. This file is agent research: it supplies evidence for the [hosting cost and load analysis](../analysis/hosting-cost-and-load-2026-10-25.md) and the [early-stage product strategy](../analysis/early-stage-product-strategy-2026-10-25.md) and decides nothing.

It adds to two earlier research files and does not repeat them: unit prices of virtual machines, managed PostgreSQL, object storage, email and the BigBlueButton host candidates are in the [hosting research](hosting-2026-10-22.md), and competing products with their prices are in the [competitors research](competitors-2026-10-22.md).

## Method and conventions

- Official provider documentation is preferred. Sources that are not the provider (news, surveys published by a vendor, community posts) are marked **third-party**. Facts no reachable source confirmed are marked **unconfirmed**.
- Ruble figures for Yandex Cloud are derived from its USD rows for the Russia region (excluding VAT) as RUB including VAT = USD × 122, the factor the hosting research established; each such figure is marked **derived**.
- A month is 720 hours.

## Billing of stopped and interruptible machines

| Fact | Source |
|---|---|
| VK Cloud: "CPU (vCPU) and RAM are charged only when the virtual machine is running"; disks, backups, public IP addresses and paid licences keep being charged while a machine is stopped; resources are billed to the nearest minute. This settles the open item of the hosting research on whether a stopped VK Cloud machine is billed for CPU and RAM. | [VK Cloud virtual machine billing](https://cloud.vk.ru/docs/en/computing/iaas/tariffication) |
| Yandex Cloud: a machine is charged from the moment its status becomes `RUNNING` until it is completely stopped; the time it is stopped is not charged; disks are charged whether the machine runs or not; compute is billed per second. | [Compute Cloud pricing policy](https://yandex.cloud/en/docs/compute/pricing) |
| Yandex Cloud preemptible machines can be stopped at any moment, always after 24 hours of running, or when the zone lacks resources for regular machines, and carry no SLA. | [Preemptible VMs](https://yandex.cloud/en/docs/compute/concepts/preemptible-vm) |
| Yandex Cloud Ice Lake, Russia region, rows effective 2026-04-30: regular vCPU (100%) 0.0101639328 USD an hour, RAM 0.0027049176 USD per GB an hour; preemptible vCPU 0.0027868848 USD, RAM 0.0006803278 USD; committed volume (CVoS) for one year 0.008606556 USD per vCPU and 0.0022950816 USD per GB, for six months 0.0091803264 USD and 0.0024491799 USD. Derived: preemptible is about 73% (vCPU) and 75% (RAM) cheaper than regular; a one-year commitment about 15% cheaper, six months about 10% cheaper. | [Compute pricing, Markdown documentation](https://docs.yandex.cloud/int/en/compute/pricing.md) |

## Object storage classes at Yandex Cloud

Rows effective 2026-04-30, Russia region, per GB-hour, with the derived per-month figure (× 720) and the derived ruble figure including VAT:

| Class | USD per GB-hour | USD per GB-month (derived) | ₽ per GB-month incl. VAT (derived) | Rules |
|---|---|---|---|---|
| Standard | 0.0000270492 | ≈0.01948 | ≈2.38 | first 1 GB free |
| Cold | 0.0000144262 | ≈0.01039 | ≈1.27 | no minimum storage period stated; requests cost more than in the standard class |
| Ice | 0.0000072131 | ≈0.00519 | ≈0.63 | minimum billable storage period of 12 months: deleting an object earlier still charges the remainder |

Source: [Object Storage pricing, Markdown documentation](https://docs.yandex.cloud/int/en/storage/pricing.md). The page says its USD prices apply to the provider's foreign contracting entities; Russian customers pay in roubles, and the hosting research found that the ×122 factor reproduces the ruble figures the Russian pages state.

## BigBlueButton capacity and installation

- **Version.** The installation page documents BigBlueButton 4.0 and lists 4.1 as in development; its example output shows 4.0.0 beta packages (checked on the official page, 2026-10-25). Whether a 4.0 release beyond beta is final is **unconfirmed** from that page alone. [Install](https://docs.bigbluebutton.org/administration/install/)
- **Minimums, unchanged from the earlier research:** production 8 cores with high single-thread performance, 16 GB RAM with swap, 250 Mbit/s symmetric; a dedicated-CPU instance type is recommended on clouds; development 4 cores and 8 GB with "only a few users". The page states no number of users or meetings per server. [Install](https://docs.bigbluebutton.org/administration/install/)
- **Large deployments** use the Scalelite load balancer and several other components; the page mentions this only in passing, with community-maintained Ansible examples. [Install](https://docs.bigbluebutton.org/administration/install/)
- **Reported capacity, third-party:** an operator reported a peak of 107 simultaneous users over seven meetings on a dedicated 8-core, 16 GB server at about half the CPU, and that BigBlueButton needs at least 4 cores and 8 GB to start; the same post describes shrinking the server to a 1 GB node between uses ([Sacha Chua, 2025-01](https://sachachua.com/blog/2025/01/scaling-a-bigbluebutton-server-down-to-a-1-gb-node-between-uses/), **third-party**). A community thread reports media server and audio mixer processes as the main CPU consumers ([meet.coop forum](https://forum.meet.coop/t/bigbluebutton-bandwidth-and-hardware-usage/107), **third-party**). An early developer stress test found audio degrading near 80 voice users at about 90% CPU on 8 cores, with webcams adding little CPU ([bigbluebutton-dev group](https://groups.google.com/group/bigbluebutton-dev/browse_thread/thread/3c7ce9815a9800c3), **third-party**, old architecture). None of these is a measurement of OVC's one-to-one lesson profile; the slice 7 probe replaces them.

## Tutoring market

- **Lesson prices in 2026.** Experts quoted by Rossiyskaya Gazeta on 2026-05-13 put the average tutoring lesson at about 1,200–2,500 ₽, core exam subjects at 3,000–3,500 ₽, and the rise of the average bill in 2026 at 10–20% (25–30% in mathematics, physics and computer science); the same article cites Profi.ru for a 54% year-on-year rise in demand for exam preparation in January–April 2026 ([Rossiyskaya Gazeta](https://rg.ru/2026/05/13/cennye-uroki.html), **third-party**). Exam preparation is a school-student segment.
- **The 2025 survey** in the competitors research (almost half of tutors charge more than 1,000 ₽ an hour, 97% teach online fully or partly, languages 31% and mathematics 23% of demand) remains the most specific figure for tutors' own practice ([competitors research](competitors-2026-10-22.md#market-context-in-2026)).
- **Adult demand for online learning.** A GetCourse survey of more than 1,200 respondents aged 18 to 50, published 2026-01-16: foreign languages are the most popular subject among adults, 38% in Moscow and Saint Petersburg and also first in the regions; respondents say they are willing to spend up to 30,000 ₽ a month on online education, about 24% of income on average ([Skillbox Media summary](https://skillbox.ru/media/edtech/ocenen-spros-vzroslyh-rossiyan-na-onlayn-obrazovanie-v-2026-godu/), **third-party**, a survey by a platform vendor). A search summary also attributed to GetCourse a 2025 platform turnover of 158 billion ₽ with about 2 billion ₽ in foreign languages; the article behind it was not identified, so the figure is **unconfirmed** and not used.
- **How many tutors there are** is **unconfirmed**: no source found gives the number of tutors or of self-employed tutors in Russia. Tutoring is described as one of the most popular activities of the self-employed ([SberBusiness](https://sberbusiness.live/publications/repetitorstvo), **third-party**), and the total number of self-employed passed 15 million by the end of 2025 by the tax service's figures as reported ([Inc. Russia](https://incrussia.ru/news/fns-zafiksirovala-rost-samozanyatyh-pochti-na-15-mln-chelovek/), **third-party**, page not read in full).
- **VK Tutoria.** No source dated 2026 was found; every report is from 2024: a free service for tutors on VK ID with a calendar, board, notes, cloud storage, VK Messenger and VK Calls, group lessons of up to 100, planned monetisation by subscriptions, and a reported launch budget of 200–300 million ₽ ([Sostav](https://www.sostav.ru/publication/vk-68408.html), [Habr](https://habr.com/ru/news/846262), **third-party**). Whether it is still offered and free is **unconfirmed**.
- **ProgressMe 2026 prices** are **unconfirmed**; the competitors research holds the latest figures found.

## Payments for a tutor service

- **The Faster Payments System (СБП).** The Bank of Russia fixed its tariff grid by a separate decision from 2026-05-01 without raising fees and denied reports that transfers would become paid; an example from the grid is 2 ₽ per operation for a transfer of 3,000–6,000 ₽ ([Brobank](https://brobank.ru/sluhi-o-platnyh-perevodah-cherez-sbp/), [Nalog-nalog](https://nalog-nalog.ru/raschety/sbp-s-1-maya-2026-chto-s-komissiyami-na-samom-dele/), **third-party**).
- **Receipts of the self-employed through YooKassa.** A review reports that YooKassa stopped supporting receipts of the self-employed for payments and refunds on 2025-12-29, so income is registered in the tax service's "Мой налог" app separately, and that YooKassa's published fees are indicative and the contract fixes the rate ([Quasa](https://quasa.io/ru/media/yukassa-ili-robokassa-dlya-samozanyatogo-komissii-cheki-i-usloviya-podklyucheniya), **third-party**; YooKassa's own page was not read). Current acquiring rates are **unconfirmed**.

## Not verified

- A final (non-beta) BigBlueButton 4.0 release, and any per-server capacity figure from the project itself.
- Boot time of a BigBlueButton host from a stopped state to accepting `create` calls (a slice 7 probe item proposed by the analysis).
- Whether VK Cloud always has capacity to start a stopped machine of the chosen flavour at a given moment.
- Whether VK Cloud can issue a credential scoped to starting and stopping one machine only.
- The number of tutors and self-employed tutors in Russia, and the share teaching adults.
- VK Tutoria's status in 2026, ProgressMe's 2026 prices, acquiring rates for a service that charges tutors.
