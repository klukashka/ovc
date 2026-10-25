# Hosting cost and load distribution for the pilot and early production

Written 2026-10-25. Agent analysis: it informs the owner's decisions and decides nothing. The pilot deployment is decided in [ADR-0013](../../docs/decisions/0013-pilot-deployment.md), the residency constraint in [ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md), the conferencing provider in [ADR-0004](../../docs/decisions/0004-bigbluebutton-primary-provider.md) and the targets in the [non-functional requirements](../../docs/requirements/non-functional.md). Unit prices come from the [hosting research](../research/hosting-2026-10-22.md); billing rules, storage classes and capacity reports checked for this analysis are in the [cost levers and market research](../research/cost-levers-and-market-2026-10-25.md). Every figure is per month and includes VAT unless stated. Figures marked **estimate** rest on the stated assumptions and are replaced by measurements as soon as the slice 7 probe and the pilot produce them.

It answers two questions: whether performance work can lower the cost of rented hardware, and how load is best distributed so that users stay satisfied.

## Summary

- **The conferencing host is the cost.** BigBlueButton is about 62% of the two-tutor pilot's bill and about 53–54% of the larger setups in the hosting research. Everything else together (application VM, PostgreSQL, storage, email) is small and grows slowly.
- **The conferencing host idles most of the month.** Lessons occupy a few afternoon, evening and weekend hours, and fewer in summer, but the host is billed for 720 hours. On VK Cloud a stopped machine is not billed for CPU and RAM, only for its disk and address, so running the host only around scheduled lessons is the largest lever: about 3,300–3,800 ₽ of the pilot's 9,500 ₽, and more at every larger size.
- **The second lever is sizing by measurement,** not by the official production minimum: the slice 7 probe and the pilot's own peak concurrency decide whether a standard rather than a high-frequency host suffices.
- **Application-tier tuning saves little money but buys time:** the API used at most 33 MB under load in the slice 1 baselines and the hot queries answer in milliseconds in the slice 2 and 3 baselines, so one small VM carries far more than the pilot. Tuning keeps that single VM longer instead of shrinking it.
- **Load follows the schedule, and OVC owns the schedule.** Lessons start on the hour and half hour: adults' in evenings and at weekends, school students' in the afternoons after school, and exam preparation peaks in spring and falls in summer; join requests, room creation, reminders and live updates all burst at those moments. Capacity is planned from the forecast of concurrent lessons, maintenance runs in the quietest slot of the week, and the worker's background work yields to request handling at peaks.
- **Two bounds bite before the hardware does:** the email provider's default daily quota, approached at about 20 tutors, and its default send rate, which breaks NFR-4.5's two minutes at about 200 tutors; and the absence of any measured join-latency or availability target. Both are cheap to fix and are listed below.

## Cost model

### Components and what drives each

| Component | Pilot (ADR-0013) | Drives the cost | Scales with |
|---|---|---|---|
| BigBlueButton 4.0 host | VK Cloud, 4 vCPU, 8 GB, 50 GB SSD, always running: ≈5,900 ₽ | CPU for audio mixing and media forwarding; bandwidth (unmetered on VK Cloud); disk only once recordings land | concurrent lessons × participants with cameras, at the evening peak |
| Application VM: reverse proxy, API, worker, PostgreSQL 18 | Yandex Cloud, 2 vCPU, 4 GB, 40 GB SSD: ≈3,500 ₽ | a reference-size VM; the load is a tiny fraction of NFR-3.1 | active tutors and students, connected sockets, email volume |
| Database backups | WAL-G to Yandex Object Storage: ≈30 ₽ | WAL volume over a 30-day window | write volume |
| File storage | Yandex Object Storage, 2 × 5 GB quota: ≈25 ₽ | stored gigabytes; egress inside the free allowance | tutors × used quota |
| Email | Yandex Postbox: ≈0 ₽ inside the free 2,000 a month | messages sent | lessons (reminders), messages (digests), homework |
| Domain, TLS, DNS, monitoring | ≈0 ₽ a month | — | — |
| **Total** | **≈9,500 ₽** | | |

The larger setups of the hosting research, without optimisation: about **27,850 ₽** for about 20 tutors and 200 students (BigBlueButton high-frequency 8 vCPU host ≈14,780 ₽, managed PostgreSQL ≈5,940 ₽, application VM ≈6,090 ₽) and about **60,300 ₽** for 200 tutors and 2,000 students (two high-frequency hosts and a Scalelite VM ≈32,540 ₽, managed PostgreSQL with a replica ≈12,830 ₽).

### Sizing by load

Assumptions (**estimate**): an average active tutor teaches about 10 lessons a week (the two pilot tutors about 15 each, about 60 lesson-hours a month), one-to-one lessons dominate, and about 60% of a tutor's lessons fall into the 20 weekday hour-slots between 15:00 and 22:00 that adults' evenings and school students' afternoons share (five days, four busiest hours each), so in an average peak hour about 6 of 20 slots are taken, roughly 30% of active tutors teach at once, and the 18:00–21:00 hours where both segments overlap may run higher; spreading the tutor base over several timezones lowers the national peak to about 20–30%. The slice 5 schedule replaces this with the real forecast.

| Size | Active tutors / students | Peak concurrent lessons (estimate) | Connected users at peak (estimate) | Conferencing host | Application tier |
|---|---|---|---|---|---|
| Pilot | 2 / up to about 30 (**estimate**) | 2 (ADR-0013's bound) | under 50 | 4 vCPU / 8 GB; the probe confirms two lessons with webcams | one 2 vCPU / 4 GB VM with PostgreSQL |
| Early production | about 20 / 200 | 4–6 | about 100–200 | one 8 vCPU / 16 GB host; standard or high-frequency CPU decided by the probe and pilot measurements | one 4 vCPU / 8 GB VM; PostgreSQL on it or managed (lever 2) |
| Growth | about 200 / 2,000 | 40–60 (the hosting research's planning figure is about 50) | about 600–1,000 | two 8 vCPU hosts, the second needed only at peaks | API and worker on one or two VMs, managed PostgreSQL with a replica |
| NFR-3.1 scale | 500 / 5,000 | 100 (NFR-3.1) | 2,000 sockets (NFR-2.2) | planned separately (NFR-3.1); several hosts | the reference deployment of NFR-2 |

Per-lesson resource figures are still planning numbers from third-party sources: about 0.6–1.3 Mbit/s of server egress for a one-to-one lesson with both cameras on, and 0.3–0.6 GB per lesson-hour ([hosting research](../research/hosting-2026-10-22.md#traffic-for-about-10-concurrent-lessons-of-25-participants)). Reported operator capacity (107 simultaneous users across seven meetings on 8 dedicated cores at about half the CPU) suggests that one 8-core host carries the early-production peak with room to spare, but it is not a measurement of OVC's profile and decides nothing.

**Unit cost.** The pilot carries about 120 lesson-hours a month, so it costs about 79 ₽ per lesson-hour in total and about 49 ₽ for the conferencing host alone (**estimate**). Unit cost falls with utilisation; the levers below lower it at every size.

## Optimisation levers

Ranked by expected monthly saving at the early-production size (about 20 tutors), the next step after the pilot. Savings use the confirmed unit prices of the hosting research: on VK Cloud 820 ₽ per standard vCPU, 1,297 ₽ per high-frequency vCPU and 223 ₽ per GB of RAM a month, with disk (13 ₽ per GB) and the public address (187 ₽) billed whether the machine runs or not.

| # | Lever | Pilot | ≈20 tutors | ≈200 tutors | Cost or risk |
|---|---|---|---|---|---|
| 1 | Run the conferencing host only around scheduled lessons | −3,300 to −3,800 ₽ (of 9,500) | −5,200 ₽ | −14,000 to −15,000 ₽ (with lever 4) | a cloud API credential, a start job tied to the schedule, boot time, start capacity |
| 2 | Keep PostgreSQL on the application VM until paying tutors depend on it | already so | −5,600 ₽ | not recommended | single point of failure, operator-owned backups; amends ADR-0013's growth rule |
| 3 | Size the conferencing host by measurement: standard CPU if the probe allows | already standard | −3,800 ₽ always on, −2,400 ₽ with lever 1 | −7,600 ₽ for two hosts, if quality holds | audio quality at peak; gated on measurements; amends ADR-0013's growth rule and ADR-0004's dedicated server |
| 4 | Place meetings on hosts in OVC instead of a Scalelite VM, while there are no recordings | — | — | −3,000 ₽, plus the second host only at peak (inside lever 1) | adapter work: the host per meeting is stored and chosen at `create` |
| 5 | Recordings after the MVP: cold storage class for recordings older than a few days | — | up to −1,100 ₽ of +2,400 ₽ | up to −11,000 ₽ of +23,800 ₽ | colder class costs more per request; playback from it to be checked |
| 6 | One-year committed use for always-on Yandex Cloud VMs | −400 ₽ | −800 ₽ | −1,300 ₽ | one-year commitment to the size and provider |
| 7 | Keep the API and the worker on one VM longer, by tuning | — | — | −3,200 ₽ | needs the measurements listed under load distribution |

**Avoided costs, to keep:** the conferencing host on unmetered traffic (metered egress would add about 1,300–2,800 ₽ at 20 tutors and 14,000–31,000 ₽ at 200, by the hosting research's traffic estimates), and no CDN (below). The quota and retention figures of lever 5 are ceilings; FR-5.6 already sets a 90-day default retention, so real recording storage will likely stay well below them.

**Rejected:**

- **Preemptible or interruptible machines** for the conferencing host or the application: about 73–75% cheaper on Yandex Cloud, but stopped at any moment and always after 24 hours, without an SLA; a lesson cut off mid-way costs more trust than the saving is worth. Suitable only for the slice 7 probe and load tests.
- **The ice storage class** for recordings: its 12-month minimum storage period charges for recordings deleted earlier, and OVC deletes recordings on expiry (at most 365 days), on account deletion and on purge.
- **A CDN or separate static hosting** for the frontend: the student bundle is about 185 KB gzip (slice 3 review) and cached by hash, so even 2,000 students reloading cold 30 times a month download about 15 GB, inside the free egress allowance at early-production size and about 25 ₽ a month at 200 tutors, where file downloads already exceed it. A CDN saves nothing measurable and adds a processor that ADR-0008 constrains to Russia. The roadmap's slice 10 caching of hashed assets at the reverse proxy is the useful part.

### Lever 1: run the conferencing host around the schedule

- **Mechanism.** The worker reads the schedule (slice 5) and starts the host a fixed lead time before the first join window of a day (a tutor may join 10 minutes before the start, FR-5.1) and stops it once no lesson is live or due within a gap longer than the boot time; a lesson ends at the latest 30 minutes after its scheduled end (FR-4.6). Before stopping, it confirms with the provider that no meeting is running. The disk and the public address stay, so the host keeps its name, certificate and configuration.
- **Saving.** On the pilot host, CPU and RAM are 5,064 ₽ of 5,900 ₽. Two tutors teaching mostly in evenings and at weekends keep the host up about 25–35% of the month: 1,270–1,770 ₽ of compute instead of 5,064 ₽ (**estimate**). At 20 tutors, a daily window of about 15 hours (08:00–23:00 in the main timezone) is 62.5% of the month: the high-frequency host's 13,944 ₽ of compute becomes about 8,700 ₽.
- **A simpler first step.** A fixed daily window (for example 08:00–23:00) instead of the schedule-driven one saves less but needs no coupling to the schedule; a lesson scheduled outside the window starts the host on demand.
- **Cost and risk.**
  - A cloud API credential that can start and stop the host: a new `Secret` setting and a threat-model entry (T-19 covers secrets in general). Whether VK Cloud can scope a credential to one machine and to start and stop only is **unconfirmed**. A stored key that can stop the host is also an availability threat, and ADR-0013's rule that no process on a VM can fetch cloud tokens has the same intent; whether a scoped key held by the worker is acceptable, or the start job runs elsewhere, is a question for the owner, not a conclusion of this analysis.
  - Boot time, unmeasured: if the host takes several minutes to accept `create`, an unplanned lesson created at the last moment waits. Adding the boot time to the slice 7 probe settles it.
  - Start capacity: a stopped machine may fail to start when the zone lacks resources (**unconfirmed** for VK Cloud). The start job alerts the operator and retries well before the first lesson; the lead time covers retries.
  - ADR-0013 considered and did not choose this for the pilot; adopting it needs the owner's decision and an amendment of ADR-0013.

### Lever 2: PostgreSQL on the application VM for longer

ADR-0013 moves PostgreSQL to the managed service when the pilot passes two tutors. Keeping it on a 4 vCPU / 8 GB application VM with WAL-G until tutors pay saves the managed node (about 5,940 ₽, minus about 300 ₽ of extra disk and backup storage). The cost is the same as in the pilot: one VM is a single point of failure for the application and the database, the operator owns backups and upgrades, and losing the disk loses up to about a minute of writes. The saving is real only while the operator's time is cheap and no paying user depends on availability.

### Lever 3: size the conferencing host by measurement

The official minimum asks for 8 dedicated cores with high single-thread performance, and ADR-0013 moves the host to the production minimum, dedicated CPU included, as the pilot grows; ADR-0004 also names one dedicated server. A standard shared-CPU host beyond the pilot therefore needs both decision records amended. For one-to-one lessons the probe and the pilot decide: CPU at the measured peak, audio mixer load, packet loss and the share of lessons with complaints. If a standard 8 vCPU host stays below about 60% CPU at the forecast peak with clean audio, it costs about 10,960 ₽ instead of 14,780 ₽. A Selectel dedicated server with real cores at about 12,000 ₽ (**unconfirmed** in the hosting research) remains the alternative when dedicated cores matter more than hourly billing; a dedicated server cannot be stopped to save money, so it competes with levers 1 and 3 together.

### Lever 4: OVC places meetings while there are no recordings

OVC creates every meeting itself, so with two hosts the adapter can choose the host at `create` (the least loaded by its own count of live meetings, or by the schedule), store it with the meeting and send every later call for that meeting to the same host. Scalelite then becomes necessary only for protected recordings after the MVP, if the recordings probe needs it ([ADR-0004](../../docs/decisions/0004-bigbluebutton-primary-provider.md)). The second host runs only during the peak window, about 30% of the month (**estimate**), which is where most of lever 1's saving at 200 tutors comes from.

### Lever 5: recordings, after the MVP

At full quota recordings add 50 GB per tutor. The cold class costs about 1.27 ₽ per GB-month against 2.38 ₽ for the standard class, so moving recordings older than a few days to it roughly halves their storage bill. Retention is already short by default: FR-5.6 sets 90 days, with 365 days as the longest choice. Quotas are ceilings: real usage, measured from the pilot, will likely be far lower than the full-quota figures. Whether the cold class serves playback without a restore step, and its request prices at the expected playback volume, are to be checked when recordings are planned.

### Levers 6 and 7: commitments and tuning

A one-year commitment is about 15% cheaper on Yandex Cloud compute, six months about 10%; it fits only VMs that are always on and whose size has stabilised, so after the pilot's first months. Tuning (below) is what keeps the API and the worker on one VM up to about 200 tutors; it is worth about one 2 vCPU / 4 GB VM.

### Optimised totals (estimate)

| Size | Unoptimised | Optimised | How |
|---|---|---|---|
| Pilot | ≈9,500 ₽ | ≈5,300–5,800 ₽ | lever 1 on the 4 vCPU host, lever 6 on the application VM |
| ≈20 tutors | ≈27,850 ₽ | ≈14,000–14,600 ₽ | PostgreSQL on a 4 vCPU / 8 GB application VM (≈6,400 ₽), a standard 8 vCPU host with a 15-hour daily window (≈7,200 ₽), storage and email (≈300–1,000 ₽) |
| ≈200 tutors | ≈60,300 ₽ | ≈42,000 ₽ | managed PostgreSQL kept, two high-frequency hosts (one on a 15-hour window, one at peaks only) without a Scalelite VM (≈14,600 ₽), the API and the worker on separate VMs as planned |

Per tutor this moves early production from about 1,390 ₽ to about 700–730 ₽ a month, and growth from about 300 ₽ to about 210 ₽ (hosting only, excluding the operator's time, support and payment fees).

## Load distribution

### What users feel, and which requirement covers it

| Moment | What the user feels | Requirement | Gap |
|---|---|---|---|
| Joining a lesson | time from pressing Join to seeing and hearing the other side | FR-5.1 sets the window; NFR-2.1 covers only the API's join route (under 500 ms at p95 for writes) | no end-to-end join target; proposed below |
| During a lesson | audio without drops, video without freezes | none beyond the host's sizing (NFR-3.1 plans video separately) | no quality target; proposed below |
| Live updates | a notification or a changed lesson appears without reload | NFR-2.2: within 1 s at p95 with 2,000 sockets | to be measured at the slice 4 review |
| Reminders | the 1-hour reminder arrives on time | NFR-4.5: within 2 minutes of due time with 1,000 other emails queued | the email provider's rate and quota |
| Pages | the next lesson is visible quickly on a phone | NFR-2.4, NFR-2.5 | met in slice 3 measurements (0.7 s on a throttled phone profile) |
| Availability | the platform and the room work when a lesson is due | NFR-4.1 to NFR-4.4 cover loss and restarts | no availability target; proposed below |

Proposed targets for the owner to accept, reject or change (they are proposals here, and enter the non-functional requirements only through an approved change; the lesson rating, the client join beacon and the forecast job they rely on are **new scope**, not in the functional requirements):

- **Join:** 95% of joins show the room within 10 seconds of pressing Join, measured from the client; 99% of join attempts inside the join window succeed.
- **Lesson quality:** fewer than 2% of lessons end with a reported audio or video problem or move to another tool, measured by a one-tap rating after the lesson and the pilot tutors' log.
- **Availability during lesson windows:** 99.5% of scheduled lesson minutes have both the platform and the conferencing host reachable, measured by an external check every minute during the windows; maintenance outside lesson windows does not count against it.

### Peak hours

Two segments shape the week (**estimate**; the slice 5 schedule data replaces it):

- **Adults** take lessons after work on weekdays (roughly 18:00–22:00 in their timezone), in late morning and afternoon at weekends, and to a lesser degree before work and at lunchtime.
- **School students preparing for exams** take lessons after school, roughly 15:00–21:00 on weekdays, and in the daytime at weekends. Their demand is seasonal: it grows from September, peaks in the spring before the exams and falls sharply in July and August.

The two daily peaks overlap between about 18:00 and 21:00, which is the hour the host is sized for; the seasonal trough in summer is where lever 1 saves most, and a summer downsizing of the host (a smaller flavour from July to August) is a further option once a season's data exists. Lessons start on the hour or half hour, so within the peak the load arrives in bursts of a few minutes at :00 and :30. Russia spans eleven timezones, which spreads a national tutor base's peak over several hours of Moscow time, but each tutor's peak sits in their own evening.

### The plan

1. **Plan video capacity from the schedule.** From slice 5 on, a daily job (**new scope**) computes the forecast of concurrent lessons and participants per 15-minute slot for the next 14 days. The operator sizes and starts hosts against the forecast peak with headroom: the forecast stays below about 70% of the measured per-host capacity. A forecast above that alerts the operator days ahead; the proposal is that OVC never refuses a scheduled lesson for capacity.
2. **Keep one meeting on one host.** With more than one host, OVC chooses the host at `create` and keeps every call for that meeting there (lever 4). Lessons are never moved between hosts while running.
3. **Absorb the :00 burst.** Joins, meeting creation and presentation upload all hit at the start of a slot. The join route stays within NFR-2.1; the probe measures `create` latency and the conversion load of a pre-uploaded presentation on the host, since conversion at :00 for many lessons competes with audio for CPU. If it does, the lever is bounding presentation size or converting it before the lesson starts.
4. **Requests before background work.** On the shared VM the API gets a higher CPU weight than the worker and PostgreSQL's background maintenance (container CPU shares or cgroup weights), so a burst of email rendering or a purge batch never slows the join route. Within the worker the per-lane loops of slice 4 already keep high-priority reminders ahead of other email.
5. **Move heavy maintenance off-peak.** The purge, cleanup and reconciliation jobs have deadlines measured in days; running their large batches in the quietest slot (by the forecast, likely the small hours of Moscow time) keeps peaks free. Deploys and host maintenance use the same slot, and a deploy checks that no lesson is live or due within the next hour before restarting anything; whether a running BigBlueButton lesson survives a restart of the application VM is a probe item.
6. **Raise the email limits before they bite.** Postbox's default is one email a second and 200 a day. If both participants of each lesson receive the 24-hour and the 1-hour reminder, 20 tutors with about 10 lessons a week (about 29 lessons a day together) send about 115 reminders a day; with confirmations, homework emails and message digests the total approaches the 200-a-day quota at about 20 tutors and passes it soon after; at 200 tutors one start slot of the peak (30–50 lessons) produces 60–100 one-hour reminders due at the same minute as 60–100 twenty-four-hour reminders for the next day's lessons in that slot, so 120–200 emails take two to three minutes at one a second, beyond NFR-4.5's two minutes (**estimate**). Request a quota and rate raise before the pilot grows past two tutors, or move to a provider with parallel connections, as ADR-0013 already anticipates; the hosting research's larger setups already use Unisender Go, so this argument applies only while Postbox is kept.
7. **Keep the database small and close.** PostgreSQL stays on loopback with the API (ADR-0013); a bounded connection pool per process, statement timeouts, and the indexes the slice 2 and 3 reviews verified (every hot query under 10 ms at tens of thousands of rows) keep it far from saturation at these sizes.
8. **Make the media path robust before making it bigger.** A TURN relay on TCP 443 next to the host serves students behind restrictive networks and keeps media flowing when the host's network is blocked or throttled; the competitors research records that blocking and throttling of a Russian lesson platform's media subnets in 2022 raised its technically cancelled lessons to up to 16%, and that a relay next to each media server was one of the fixes; the conferencing host sits in a Moscow-region data centre close to most users.
9. **One API instance until a measurement asks for more.** The slice 4 broadcaster already allows several instances; at these sizes one instance holds every socket (NFR-2.2's 2,000), and splitting adds a load balancer and cost without a user-visible gain.

### What to measure

| Signal | Where | Threshold that triggers action (proposed) |
|---|---|---|
| Forecast concurrent lessons per 15-minute slot | worker job from the schedule (slice 5, **new scope**) | forecast peak above 70% of measured host capacity |
| Host CPU, audio mixer CPU, packet loss, bandwidth | conferencing host, per minute during lessons | CPU above 60% at peak, or packet loss above 2% |
| Join funnel: Join pressed → join link issued → room loaded → media connected | API timing and a client beacon to OVC's own API (**new scope**) | p95 above 10 s, or success below 99% |
| Lesson quality rating and moves to another tool | lesson page after the lesson (**new scope**); pilot tutors' log | above 2% of lessons |
| API latency by route, p95 | access records | NFR-2.1 exceeded in the peak hour |
| Socket count and frame delivery p95 | gateway | NFR-2.2 exceeded |
| Outbox lag per lane and the provider's send rate | worker | high-priority lag above 60 s |
| PostgreSQL connections, CPU share, slow statements | database | pool waits or statements above 100 ms in the peak hour |
| Application VM CPU and memory in the peak hour | host | CPU above 60% at peak for a week |
| Host running hours against lesson hours | cloud billing and the start job | running hours more than twice the lesson-window hours |
| External availability during lesson windows | an availability check in Russia | below 99.5% of lesson minutes in a month |

Each signal follows the [observability specification](../specs/observability.md): identifiers and timings only, never names, join links or message contents.

## Proposed probe additions for slice 7

- Boot time of the stopped host until it accepts `create`, and whether a lesson that is running survives a restart of the application VM.
- CPU, audio quality and packet loss for two, four and eight simultaneous one-to-one lessons with webcams on the 4 vCPU host, and on an 8 vCPU standard and high-frequency host if one is rented for an hour, so the early-production size is chosen from data.
- `create` latency with and without a pre-uploaded presentation, and the conversion's CPU cost.
- A TURN relay on TCP 443 for a student behind a network that blocks UDP.

## Not verified

- Every per-lesson resource figure and the reported capacity of an 8-core host; peak-hour shares; email volume per lesson.
- Boot time and start capacity of a stopped VK Cloud machine, and whether a VK Cloud credential can be scoped to starting and stopping one machine.
- Playback from the cold storage class without a restore step, and its request costs at playback volume.
- Whether a running lesson survives an application restart.
