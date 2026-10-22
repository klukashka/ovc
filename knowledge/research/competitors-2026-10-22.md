# Competitors: what a Russian private tutor uses instead of OVC

Checked 2026-10-22. Prices, limits, availability in Russia and company statements are volatile; recheck them on each vendor's page before relying on them. This file is agent research: it informs product and pricing choices and decides nothing. OVC's own scope is set by the [product overview](../../docs/product.md), the [functional requirements](../../docs/requirements/functional.md) and the [roadmap](../../docs/roadmap.md); its hosting by [ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md), [ADR-0013](../../docs/decisions/0013-pilot-deployment.md) and the [hosting research](hosting-2026-10-22.md).

## Method and conventions

- Every fact below was checked on 2026-10-22 at the linked page, unless the text gives another date for the source itself. Official vendor pages and help centres are preferred. Sources that are not the vendor (review sites, news, aggregators, competitors) are marked **third-party**.
- Facts that no reachable source confirmed are marked **unconfirmed**. Several vendor sites render prices only with JavaScript, and one (ProgressMe) answered automated requests with a CAPTCHA, which was not bypassed; their figures come from help centres or third-party pages and are marked as such.
- Order-of-magnitude figures that are not published anywhere are marked **estimate** with their basis.
- Foreign prices are converted at the Bank of Russia official rate for 2026-10-22: 1 USD = 81.9739 ₽, 1 EUR = 91.9337 ₽ ([Bank of Russia daily rates](https://www.cbr.ru/currency_base/daily/)).
- **Usage profile** for "10 students" and "50 students": each student has four one-hour lessons a month, so 40 or 200 lesson-hours. Products that charge per lesson or per hour are computed from that profile; products that charge per active student or per seat are not affected by it.
- OVC's MVP functions are those of the [functional requirements](../../docs/requirements/functional.md): a video room with whiteboard, presentation and screen sharing, embedded in the lesson page once confirmed (FR-5.1 to FR-5.3), recordings after the MVP (FR-5.4 to FR-5.6); private and group chats with files (FR-12); a schedule with weekly series in the tutor's timezone (FR-4); homework with automatically checked questions and tutor review (FR-6 to FR-11.1); students and groups (FR-2, FR-3); one student with several tutors (FR-2.4); in-app and email notifications (FR-13); every page usable on a phone, with dedicated phone layouts in slice 10 (NFR-5.2).

## Market context in 2026

- **Tutors teach online and charge about 1,000 ₽ an hour.** A survey by Avito Services and ProgressMe in September 2025 found that almost half of the surveyed tutors charge more than 1,000 ₽ for an hour, only 1.5% charge less than 500 ₽, and 97% teach online fully or partly; foreign languages lead demand at 31%, then mathematics at 23% ([Skillbox Media summary](https://skillbox.ru/media/education/skolko-stoit-zanyatie-s-repetitorom-i-ot-chego-zavisit-cena/), **third-party**; the original report was not found).
- **Personal data of Russian citizens must be collected into databases in Russia.** Part 5 of article 18 of 152-FZ, in the wording of Federal Law 23-FZ of 28 February 2025, says that when personal data is collected, including over the internet, the recording, systematisation, accumulation, storage, clarification (updating, changing) and retrieval of personal data of Russian citizens with databases outside Russia is not allowed, except in the cases of items 2, 3, 4 and 8 of part 1 of article 6 ([article 18 at ConsultantPlus](https://www.consultant.ru/document/cons_doc_LAW_61801/cbf4e15b7c330f9372e876cdf2bc928bad7950ef/), which marks the part as amended by 23-FZ). The Ministry of Digital Development's letter of 12 May 2025 No. П25-44929 quotes this wording, states that it enters into force on 1 July 2025, and adds that it does not restrict cross-border transfer of data first collected into databases in Russia, which still follows article 12, including the prior notification to Roskomnadzor ([letter text](https://pravo.ppt.ru/pismo/mincifry-rossii/n-p25-44929-316881), **third-party** publication of the letter; the text of 23-FZ itself was not read). Analysis, not legal advice: a tutor who keeps students' data in a foreign service is likely exposed as an operator; whether a private tutor is an operator at all, including the exclusion for processing by an individual for personal and family needs in item 1 of part 2 of article 1 (not checked against the text here), is a question for a lawyer.
- **Foreign messenger calls are restricted.** Roskomnadzor announced on 13 August 2025 that it partly restricts voice calls in WhatsApp and Telegram ([Rozetked](https://rozetked.me/news/40906), **third-party**). Tutors who taught through those calls moved to other tools; how many is **unconfirmed**.
- **The national messenger Max has group video calls** with screen sharing and, as VK announced in July 2025, no limit on duration or participants ([Mobile-review](https://mobile-review.com/all/news/v-messendzhere-max-poyavilsya-funkczional-gruppovyh-videozvonkov/), **third-party**). It is a free substitute for the video and chat parts of OVC, without a whiteboard, schedule or homework.
- **Foreign payments are mostly closed.** Visa and Mastercard suspended operations in Russia in March 2022 ([Neowin](https://www.neowin.net/news/mastercard-paypal-and-visa-suspend-service-in-russia-amid-the-ukraine-conflict/), **third-party**), and Stripe does not list Russia among its countries ([Stripe global](https://stripe.com/global)). Foreign subscriptions below are usually unpayable with a Russian card.
- The school messaging platform Sferum, integrated into Max, verifies users through their school ([Mel](https://mel.fm/vospitaniye/eksperty/7928504-shkola-obyazuyet-perevesti-vse-chaty-na-rossyskiye-servisy-kakoy-vybrat), **third-party**), so it is not a tool for private tutors of adults and is not covered further.

## Russian platforms built for tutors

### ProgressMe

- **Target user:** private language tutors, language schools and course authors; founded in 2017, a Skolkovo resident since 2022, and about 50,000 tutors and 200,000 users in July 2021 ([CycloWiki](https://cyclowiki.org/wiki/ProgressMe), **third-party**).
- **Video:** built into the lesson window next to the interactive textbook and whiteboard, with breakout rooms, attendance and in-lesson chat ([PickTech product card, updated 2026-08-25](https://picktech.ru/product/progressme/), **third-party**). The same source says lessons cannot be recorded. Participant cap and media technology are **unconfirmed**.
- **Homework:** more than 30 exercise types checked automatically, AI lesson generation, built-in dictionary ([PickTech, 2026-09-07](https://picktech.ru/blog/expert-opinion/platforma-dlya-repetitora/), **third-party**). The platform is strong on ready-made language course content, which OVC does not have.
- **Schedule and notifications:** a schedule with reminders and group lessons ([CycloWiki](https://cyclowiki.org/wiki/ProgressMe), **third-party**); weekly series, chat with files and one student with several tutors are **unconfirmed**.
- **Mobile:** no native apps, an adaptive web interface only ([PickTech blog](https://picktech.ru/blog/expert-opinion/platforma-dlya-repetitora/), **third-party**).
- **Pricing:** the official page says online lessons cost "from 380 ₽ a month", free for students, with a 7-day trial ([pricing page](https://progressme.ru/Home/Pricing), read through a search snippet because of the CAPTCHA). A third-party list gives a per-student scale from 380 ₽ for 2 students to 3,750 ₽ for 50 ([Startpack](https://startpack.ru/application/progressme), **third-party**); another gives a Standard plan at 990 ₽ a month (743 ₽ billed yearly) for up to 50 students ([PickTech product card](https://picktech.ru/product/progressme/), **third-party**). A tutor with 10 students pays **about 990 ₽**, with 50 students **990 ₽ to 3,750 ₽** depending on which model is current (**unconfirmed**).
- **Reliability:** no status page, SLA or public incident history found.
- **Data residency and hosting:** the privacy policy could not be read (**unconfirmed**). The site's anti-bot page is Yandex SmartCaptcha, a Yandex Cloud service, which is a weak hint that Yandex Cloud is in use, not evidence of where data is stored.

### Edvibe

- **Target user:** independent language teachers (Standard plan) and schools (PRO plan) ([Edvibe 3.0 release notes, 2024-11-25](https://intercom.help/edvibe/ru/articles/10056973-edvibe-3-0-полный-список-обновлений)). Third-party sources link it to the ProgressMe team ([CycloWiki](https://cyclowiki.org/wiki/ProgressMe), **third-party**); the companies do not state this themselves (**unconfirmed**). Its Trustpilot company profile gives an address in Wilmington, USA ([Trustpilot](https://www.trustpilot.com/review/edvibe.com), **third-party**).
- **Video:** virtual classes with video and whiteboards, group classes of up to 25 people, breakout rooms ([group work help](https://intercom.help/edvibe/ru/articles/8401502-работа-с-группами-в-виртуальном-классе)). Recordings and media technology are **unconfirmed**.
- **Payments and lesson balances:** a balance system tracks lesson packages, payments and group attendance; payments through the platform cost a 0.9% commission and run through Stripe or WayForPay ([balance system help](https://intercom.help/edvibe/ru/articles/8125201-балансовая-система-edvibe)). Neither payment provider serves Russian cards, so the feature targets tutors outside Russia (**estimate** from the providers' coverage).
- **Pricing:** no official 2026 price list could be read. Third-party figures: from 3 USD a month for 2 students in February 2025 ([eLearning Industry](https://elearningindustry.com/directory/elearning-software/edvibe/pricing), **third-party**); 2,800–3,700 ₽ for 50 students, undated ([Irecommend](https://irecommend.ru/content/vse-eshche-otsylaete-uchenikam-otskanirovannye-uchebniki-pora-perekhodit-v-tsifru), **third-party**). 50 students: **about 2,800–3,700 ₽**; 10 students **unconfirmed**.
- **Reliability:** Trustpilot shows 2.4 out of 5 from 23 reviews; reviews dated 2026 report outages on 7 October 2026 that forced lesson cancellations, a database connection error on 11 October 2026, features broken after updates (30 March 2026) and videos failing to load (13 July 2026) ([Trustpilot](https://www.trustpilot.com/review/edvibe.com), **third-party**). No status page or SLA found.
- **Data residency and hosting:** **unconfirmed**.

### Vimbox (Skyeng), a reference only

- **Target user:** Skyeng's own teachers; Vimbox is not offered to independent tutors ([Skyteach](https://skyteach.ru/help/vimbox-samaya-udobnaya-platforma-dlya-urokov-anglijskogo/)). It is included because Skyeng published the most detailed engineering account of a Russian online-lesson video stack.
- **Video infrastructure:** own WebRTC media servers on Janus Gateway, Twilio STUN/TURN when needed, VP8 and later VP9. Skyeng's cost per lesson fell from about 113 ₽ on Tokbox and 20 ₽ on Voximplant (audio only) to about 2 ₽ on its own Janus servers, plus development and support. Quality degraded beyond about 420 rooms per server although CPU, memory and disk were far from their limits, so the per-server bandwidth limit was lowered from 300 to 200 Mbit/s; teacher complaints about call quality fell from 7.1% in January 2018 to 2.5% in January 2019 ([Skyeng on Habr, 2019-04-03](https://habr.com/ru/post/446444)).
- **Public outage report:** from 26 February 2022 the subnets of Skyeng's Janus servers were affected by blocking and throttling, which Skyeng attributes to Roskomnadzor; lessons cancelled for technical reasons rose from about 1–1.5% to up to 16% and settled at 3–4%. Fixes were a coturn relay next to each Janus server, proxying through other countries, moving users to Firefox and a fallback to Voximplant, which loses recordings ([Skyeng on Habr, 2022-04-28](https://habr.com/en/companies/skyeng/articles/663318)).
- **Hosting:** Skyeng moved its test environment from AWS to Yandex Cloud and saved about 30% ([Yandex Cloud case](https://yandex.cloud/ru/cases/skyeng)); its production hosting is **unconfirmed**.
- **Pricing:** none for tutors; students pay from about 950 ₽ per lesson ([Engblog review](https://engblog.ru/skyeng-review), **third-party**).

### AlfaCRM, the CRM for lessons and payments

- **Target user:** training centres, language and music schools; schedule, subscriptions, finance and a client portal ([home page](https://alfacrm.pro/)). No video room found; tutors pair it with a separate video tool.
- **Functions:** notification add-ons through Telegram, Max, push, SMS and email, internal chats, free online booking ([pricing](https://alfacrm.pro/pricing)); an iOS client app ([App Store](https://apps.apple.com/ru/app/id6478144118)). Homework and tests are not offered (**unconfirmed**, none mentioned).
- **Pricing** ([pricing](https://alfacrm.pro/pricing)): Start 2,600 ₽ a month for up to 100 active clients (a client who attended a lesson in the last 30 days), Business 3,900 ₽ for 200, Personal from 5,400 ₽; add-ons Telegram 800 ₽, client portal 400 ₽, chats 500 ₽; up to 20% off yearly. A tutor with 10 or 50 students pays **2,600 ₽**, or **3,800 ₽** with Telegram and the portal.
- **Data residency:** the [privacy policy](https://alfacrm.pro/legal/privacy_policy) states that personal data of Russian citizens is stored in databases in Russia under part 5 of article 18 of 152-FZ; the operator is a Novosibirsk company. The hosting provider is not named (**unconfirmed**).
- **Reliability:** no status page or SLA found.

## Russian online-school platforms

### GetCourse

- **Target user:** online schools and course sellers, from single experts to producers ([home page](https://getcourse.ru/)).
- **Video:** its own browser webinar room with screen sharing and automatic recording, up to 20,000 viewers by default, with larger events agreed with support at least three days ahead ([broadcast mode help](https://getcourse.ru/help/translationmode)). A whiteboard is **unconfirmed**. The room is a broadcast webinar, not a small interactive classroom.
- **Learning and messaging:** tests and homework checking, CRM messaging by email, SMS and messengers, payments, funnels and a site builder ([home page](https://getcourse.ru/)). A lesson calendar with weekly series is **unconfirmed** and appears absent.
- **Several schools per student:** a GetCourse ID lets one student move between the schools they study at, and the free student app covers all of them ([GetCourse blog](https://getcourse.ru/blog/1019460)).
- **Pricing** ([price page](https://getcourse.ru/prices)): billed by users who sign in during the month; the smallest plan, Старт, allows 1,000 users and 20 GB for 5,900 ₽ a month, Любитель 2,000 users for 10,100 ₽; 3 and 6-month prepayment gives up to 20% and 35% off. A tutor with 10 or 50 students pays **5,900 ₽**.
- **Reliability:** a [status page](https://status.getcourse.ru/) shows subsystem states without uptime figures or incident history; no SLA found. Reviews from 2023 complain of slowness ([CRM Index](https://crmindex.ru/services/getcourse), **third-party**).
- **Data residency:** the [privacy policy](https://getcourse.ru/privacy_policy) states in clause 7.1.1 that all servers that process and store personal data are physically in Russia, and clause 7.1 names Selectel as the hosting provider. The school is the operator and GetCourse processes data on its instruction.
- **Scale and hosting:** GetCourse claims 70% of Russian online schools, 50,000 clients since 2013 and 158 billion ₽ of schools' turnover in 2025 ([home page](https://getcourse.ru/), [blog](https://getcourse.ru/blog/1198607)). No engineering posts, CDN or video infrastructure details were found.

### Skillspace

- **Target user:** experts, online schools, corporate training and universities ([home page](https://skillspace.ru/)).
- **Video:** built-in webinars "without Zoom, YouTube and OBS", one-click recording into a library, polls during the broadcast ([webinars page](https://skillspace.ru/features/webinars)). Participant caps, screen sharing and whiteboard are **unconfirmed**.
- **Learning:** homework and test builders with checking, chat with students, certificates and attendance ([home page](https://skillspace.ru/)). A lesson calendar with weekly series, notifications and several schools per student are **unconfirmed**.
- **Mobile:** an iOS app listed as designed for iPad ([App Store](https://apps.apple.com/ru/app/skillspace/id1578915606)); Android is **unconfirmed**.
- **Pricing** ([pricing page](https://skillspace.ru/pricing)): Старт 3,490 ₽ a month for 50 active students and 20 GB, Эксперт 7,990 ₽ for 500; paid modules on top, whose prices are **unconfirmed**. A tutor with 10 or 50 students pays **3,490 ₽** plus any modules; a 51st student moves them to 7,990 ₽.
- **Reliability, data residency, hosting:** no status page, SLA, storage-location statement or hosting provider found (**unconfirmed**).

### Antitreningi

- **Target user:** experts, online schools, producers and training centres ([home page](https://antitreningi.ru/)).
- **Video:** webinars "up to 100,000 viewers", and online meetings with whiteboard and screen sharing: one-to-one on the cheapest plan, group meetings from Старт ([pricing](https://antitreningi.ru/lp/pricing)). Meeting caps and the media technology are **unconfirmed**.
- **Learning:** homework review with AI-assisted checking, a test builder with nine question types, free iOS and Android apps with push notifications ([home page](https://antitreningi.ru/)). Weekly lesson series, chat with files and several schools per student are **unconfirmed**.
- **Pricing** ([pricing](https://antitreningi.ru/lp/pricing)), monthly, by active students (a student who has not opened the course for 30 days stops counting): Индивидуальный 1,490 ₽ for 20 students, Старт 3,100 ₽ for 30, Любитель 6,100 ₽ for 100, Профи 12,600 ₽ for 300. A tutor with 10 students pays **1,490 ₽** if every lesson is one-to-one, otherwise **3,100 ₽**; with 50 students **6,100 ₽**.
- **Data residency:** the [privacy policy](https://antitreningi.ru/info/privacy-policy/) cites 152-FZ but states neither the storage location nor the hosting provider (**unconfirmed**).

## Russian video and classroom tools

### Yandex Telemost

- **Target user:** everyone for free; businesses through Yandex 360 plans.
- **Functions:** group calls of up to 40 people, no time limit in the browser, desktop or mobile clients ([call types help](https://yandex.ru/support/yandex-360/customers/telemost/web/ru/types.md)); scheduled and repeating meetings through Yandex Calendar, a waiting room, files in Telemost chats stored on Yandex Disk ([Telemost help, full text](https://yandex.ru/support/yandex-360/customers/telemost/web/ru/llms-full.txt)); screen sharing. Recordings are saved only to the recording participant's own computer, from Yandex Browser, Chrome or the desktop app, not from Firefox, and stop when that person leaves ([recording help](https://yandex.ru/support/yandex-360/customers/telemost/web/ru/call/record.md)). A whiteboard is not mentioned in the help (**unconfirmed**, probably absent). No homework, no student management, no embedding API found.
- **Pricing:** free for a tutor with 10 or 50 students while no call exceeds 40 people. Business plans with 100 to 300 participants cost about 309–549 ₽ per user a month by reseller pages ([Zerobit](https://zerobit.ru/yandex360-telemost), **third-party**; official prices **unconfirmed**).
- **Reliability and scale:** more than 100,000 meetings a day in August 2024, when Yandex replaced an external open-source media server with its own WebRTC platform, Goloom ([Yandex on Habr](https://habr.com/ru/articles/834512/)), so Yandex runs its own media servers. No status page or SLA found. A Yandex outage on 17 September 2026 included Telemost, with more than 250 complaints by the afternoon ([Techora](https://techora.ru/news/sboy-yandeksa-17-sentyabrya-ne-rabotayut-2026-09-17), **third-party**); a wider Yandex and Yandex Cloud outage on 12 October 2026 was reported without naming Telemost ([Esports.ru](https://esports.ru/news/yandeks-stolknulsya-s-masshtabnym-sboem/), **third-party**).
- **Data residency:** only a review site states that servers are in Russia ([Comss](https://www.comss.ru/page.php?id=7898), **third-party**); an official statement and registry status are **unconfirmed**.

### VK Calls and VK Tutoria

- **VK Calls:** free calls with no limit on time or participants, screen sharing, in-call chat with files for signed-in users, one-off and recurring scheduled calls, a waiting room, anonymous joining by link, recording delivered as a file in the call's VK chat, apps for iOS, Android, Windows, macOS and Linux ([VK business guide](https://m.vk.ru/@business-kak-biznesu-ispolzovat-vk-zvonki)). The participant limit was removed in November 2021, when group calls had 20 million monthly users, on VK's own horizontally scaled servers with server-side transcoding ([VK press release](https://m.vk.ru/press/calls-no-limit)). An API creates and ends calls on a schedule and switches recording on, and an SDK embeds calls in other products ([VK API announcement, 2022-07-08](https://m.vk.ru/press/calls-api)). No whiteboard in Calls itself (**unconfirmed**).
- **VK Tutoria:** a free platform for tutors and students launched on 13 August 2024, with a calendar, notes, Mail.ru Cloud storage, a built-in whiteboard, VK Messenger and VK Calls, sign-in through VK ID, and no limit on students or lessons; monetisation through subscriptions was planned ([Habr news, 2024-08-13](https://habr.com/ru/news/835708/), **third-party**). Group lessons with up to 100 participants per group arrived on 26 September 2024 ([Habr news, 2024-09-26](https://habr.com/ru/news/846262), **third-party**). Homework with auto-checked tests is not mentioned. Its official site could not be fetched, so whether Tutoria is still offered and free in October 2026 is **unconfirmed**. It is the closest Russian analogue of OVC's scope, backed by a large company and free.
- **Pricing:** 0 ₽ for 10 or 50 students.
- **Reliability:** no status page or SLA found; VK's site and apps were unavailable for about 30 minutes on 3 July 2025 ([3DNews](https://3dnews.ru/1125361/vkontakte-perestala-otkrivatsya-v-rossii-i-za-granitsey), **third-party**).
- **Data residency:** a 152-FZ statement was found only for VK WorkSpace, not for consumer VK Calls or Tutoria (**unconfirmed**).

### MTS Link (formerly Webinar.ru)

- **Target user:** businesses and education: meetings, webinars, boards, courses and forms ([home page](https://mts-link.ru/)).
- **Plans** ([tariffs](https://mts-link.ru/tariffs/), prices on request): Free with 30 participants, 60-minute meetings and three boards; Standard and Pro with 200 participants. A third-party review gives meetings from 15,600 ₽ per user a year (about 1,300 ₽ a month) and webinars Optimum 30 at 8,275 ₽ a month ([Vakas, prices checked by its author 2026-09-29](https://vakas.ru/articles/mts-link-obzor/), **third-party**). A tutor with one-to-one lessons fits the free tier only if each lesson ends within 60 minutes; otherwise about **1,300 ₽** a month.
- **Reliability and scale:** a [status page](https://status.mts-link.ru/) exists; on 2026-10-22 it showed an error page and a maintenance notice dated 21 October 2026 that new-account registration was temporarily unavailable. No SLA. 5 million events and 30 million participants in 2023 ([Mobile-review press release](https://mobile-review.com/all/press_releases/za-god-na-platforme-mts-link-proshlo-5-millionov-onlajn-meropriyatij/), **third-party**).
- **Data residency:** the home page states that data is stored on servers in Russia and processed under 152-FZ; data centres are not named; an [on-premises version](https://mts-link.ru/products/on-premise/) exists.

### Pruffme

- Webinars with up to 30 speakers, automatic 1080p recording, whiteboards with more than 50 tools, polls and quizzes, an open API for events on one's own site, an on-premises option, a listing in the Russian software registry, and claims of more than 4,000 companies and about 200,000 broadcasts a month ([home page](https://pruffme.com/)). Courses with single-choice, multiple-choice and ordering questions and essays; prices of 2023 were 3,910–8,165 ₽ a month for webinars and 2,500 ₽ for the courses group plan ([HF, 2023-05-31](https://hf.ru/services/pruffme), **third-party**, possibly outdated). Current prices, participant caps, 152-FZ statement, status page and SLA are **unconfirmed**.

### Mirapolis Virtual Room

- A browser virtual room inside the Mirapolis HCM suite, aimed at corporate training, with an on-premises option; aggregators claim up to 10,000 participants and prices from 1,990 ₽ or on request ([Soware](https://soware.ru/products/virtualroom), **third-party**). Not a realistic choice for a private tutor; listed for completeness.

### Hosted BigBlueButton in Russia

No Russian provider of managed, multi-tenant BigBlueButton rooms was found. What exists is a virtual machine with BigBlueButton preinstalled, which the customer then runs:

- **NetAngels:** a VM with BigBlueButton deployed automatically, paid as a VM: 4 cores, 8 GB, 30 GB for 1,771 ₽ a month on yearly billing (2.46 ₽ an hour), quoted as about 10 participants on video or 100 in webinar mode; 8 cores, 16 GB for 5,210 ₽ (about 20 on video); 12 cores, 32 GB for 10,271 ₽ (about 30 on video); the page says no plan limits users and the figures are for smooth operation; data-centre location and post-setup administration are not stated ([NetAngels BigBlueButton](https://www.netangels.ru/landings/bbb/)).
- **bigbluebutton.ru (BEGET):** VPS from 1,650 ₽ a month (4 cores, 6 GB) to 5,100 ₽, self-install or a template, not endorsed by the BigBlueButton project ([bigbluebutton.ru](https://bigbluebutton.ru/)).
- **SpaceWeb:** VPS with BigBlueButton in Saint Petersburg and Moscow from about 2,080 ₽ a month ([SpaceWeb](https://sweb.ru/vds/bigbluebutton/), read through a search snippet).
- Foreign hosts such as WebHostingZone sell managed BigBlueButton on European servers from 59 USD a month ([WebHostingZone](https://www.webhostingzone.org/solutions/bigbluebutton-hosting.html)), which [ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md) rules out for OVC.

These offers are a hosting option for OVC's own BigBlueButton host rather than a competitor to a tutor. NetAngels' 4-core, 8 GB VM at 1,771 ₽ matches the pilot's VK Cloud host at about 5,900 ₽ in CPU and RAM but not in disk (30 GB against 50 GB) ([hosting research](hosting-2026-10-22.md#small-pilot-owners-figures-2026-10-22-mvp-per-month)); whether its cores are dedicated, its traffic is unmetered and its data centre is in Russia is **unconfirmed** and decides whether it is comparable.

## International references

### Preply

- **Target user:** a tutor marketplace where students find, book and pay tutors; Preply's classroom has video, whiteboard, screen sharing and chat with files, and works in its app ([Preply classroom help](https://help.preply.com/en/articles/4182374-preply-classroom)). Video runs on Agora ([Agora case study](https://www.agora.io/en/success-stories/preply)). Recordings, group lessons and auto-checked homework are **unconfirmed** and appear absent.
- **Commission:** trial lessons 100%; then from 33%, falling with hours taught to a floor of 18% ([commission model](https://help.preply.com/en/articles/4171383-preply-commission-model)). At 20 USD an hour, 40 hours cost a new tutor about 264 USD (21,641 ₽) at a flat 33%, and 200 hours at the 18% floor 720 USD (59,021 ₽) (derived approximations: the real commission falls as hours accumulate, the intermediate tiers are not published in text, and trial lessons with each new student are kept at 100% on top).
- **Russia:** since 4 March 2022 tutors from Russia cannot earn on Preply ([DOU](https://dou.ua/lenta/news/preply-stands-with-ukraine/), **third-party**), so it is not an option for a tutor in Russia, only a reference for the classroom and the marketplace model.
- **Hosting:** AWS with Kubernetes and Kafka, by a job posting ([Techstars jobs](https://jobs.techstars.com/companies/preply/jobs/64445274-senior-platform-cloud-engineer), **third-party**).

### Zoom Workplace

- **Functions:** video with whiteboard, screen sharing and chat; Pro allows 100 participants, 30-hour meetings and 10 GB of cloud recording per licence, and Basic limits meetings to 40 minutes ([pricing](https://zoom.us/pricing)). No homework, schedule beyond meetings, or student management.
- **Pricing:** Pro 16.99 USD a month billed monthly (1,393 ₽), 14.16 USD billed yearly (1,161 ₽) ([pricing](https://zoom.us/pricing)); the same for 10 or 50 students because participants need no licence.
- **Reliability:** a [status page](https://status.zoom.us/); a 99.9% uptime commitment appears in a public-sector listing of its terms ([UK Digital Marketplace](https://www.applytosupply.digitalmarketplace.service.gov.uk/g-cloud/services/571841732555380), **third-party**); on 16 April 2025 the `zoom.us` domain was unavailable for about two hours after a registry block ([Tom's Guide](https://www.tomsguide.com/news/live/zoom-down-outage-apr-16-25), **third-party**).
- **Russia:** in July 2025 a Moscow court fined Zoom 79.6 million ₽ for having no Russian office, no Roskomnadzor account and no data localisation, while the service stayed accessible ([Moskvich Mag](https://moskvichmag.ru/gorod/taganskij-sud-oshtrafoval-zoom-na-80-mln-rublej-za-otkaz-sotrudnichat-s-roskomnadzorom/), **third-party**). Paying with a Russian card is not possible (see market context); whether Zoom sells to Russian residents at all is **unconfirmed**. Data is stored outside Russia.
- **Hosting:** its own colocation data centres plus AWS and other clouds ([Data Center Frontier](https://datacenterfrontier.com/inside-zooms-infrastructure-scaling-up-massively-with-colo-and-cloud/), **third-party**), 29 colocation sites by a 10-K summary ([DCD](https://www.datacenterdynamics.com/en/news/zoom-decreasing-dependence-on-cloud-services-boosts-colo-data-center-portfolio-to-29), **third-party**).

### Google Classroom with Google Meet

- **Functions:** classes, assignments with manual grading and auto-graded Forms quizzes, Meet links per class. A personal account limits Meet group calls to 60 minutes and a class to 250 members; Workspace for Education lifts that to 24 hours and 1,000 ([Classroom help](https://support.google.com/edu/classroom/answer/10942472?hl=en)). Workspace for Education is only for qualifying institutions ([editions](https://edu.google.com/workspace-for-education/editions/compare-editions/)).
- **Pricing:** free with a personal account; Business Starter 6.80 EUR (625 ₽) and Business Standard with Meet recording 13.60 EUR (1,250 ₽) per user a month ([Workspace pricing](https://workspace.google.com/pricing)).
- **Reliability:** a 99.9% monthly uptime SLA with service credits for paid Workspace ([SLA](https://workspace.google.com/terms/sla/)).
- **Russia:** Google restricted the creation of new accounts by Russian users in September 2024 ([Novaya Gazeta Europe](https://novayagazeta.eu/en/articles/2024/09/26/google-restricts-creation-of-new-accounts-in-russia-en-news), **third-party**), so new students may be unable to register; whether this still applies in 2026 is **unconfirmed**. Data is stored outside Russia.

### TutorBird and Teachworks

- **TutorBird:** scheduling, invoicing, online payments through Stripe and PayPal, a student portal with files; video through Zoom or Lessonspace integrations; 16.95 USD a month (1,389 ₽) plus 4.95 USD per extra tutor, unlimited students ([pricing](https://tutorbird.com/pricing)). Data is processed in Canada, the United States and other jurisdictions ([Port 443 privacy policy](https://www.port443.io/privacy-policy/)).
- **Teachworks:** repeating lessons, billing, online lessons through Lessonspace, Pencil Spaces, Zoom or Meet links, no native app ([pricing](https://teachworks.com/pricing), [repeating lessons](https://intercom.help/teachworks-e2d272c6e669/en/articles/15362050-adding-a-repeating-lesson), [online tutoring](https://teachworks.com/addons/online-tutoring-software)). Starter is 16.49 USD plus 0.32 USD per student lesson: 29.29 USD (2,401 ₽) for 40 lessons and 80.49 USD (6,598 ₽) for 200 (derived; the page also shows an older price set, so the figures are to be rechecked).
- Neither has homework with auto-checked tests, and neither can be paid from Russia without a foreign card. Status pages and hosting are **unconfirmed**.

### Lessonspace

- **Functions:** a browser classroom with video, chat, screen sharing, an infinite whiteboard for up to 10 students, document annotation and recordings kept 90 days; an API creates spaces and embeds them by iframe; no schedule or homework ([home page](https://www.thelessonspace.com/), [pricing](https://www.thelessonspace.com/pricing)). It is the closest international analogue of OVC's embedded room, sold as a component to tutor CRMs.
- **Pricing:** billed per second while at least two people are connected; Starter 29 USD for 30 hours, then 1.80 USD an hour; Advanced 199 USD for 200 hours; each participant beyond four costs 0.25 USD an hour ([pricing](https://www.thelessonspace.com/pricing)). 40 hours: 47 USD (3,853 ₽); 200 hours: 199 USD (16,313 ₽) (derived).
- **Reliability:** a [status page](https://status.thelessonspace.com) and a claimed 99.99% uptime, with a custom SLA only on Enterprise ([pricing](https://www.thelessonspace.com/pricing)). Cloud and media vendors are **unconfirmed**. Data is outside Russia.

### Miro

- A whiteboard only, often paired with Zoom or Meet. Since 15 October 2026 Miro blocks users in Russia by IP address on every plan, including free accounts, guests and members of foreign teams, in the browser, desktop and mobile clients ([SecurityLab, 2026-10-16](https://www.securitylab.ru/news/578823.php), **third-party**; Miro's own statement was not found). Miro runs on AWS EKS ([AWS case study](https://aws.amazon.com/solutions/case-studies/miro-eks-case-study/)).

## Comparison with OVC's MVP

Legend: **yes** documented by the vendor; **partly** documented with a limit that matters; **no** documented as absent or plainly outside the product; **?** not confirmed. Prices are per month for the usage profile in the method section.

| Product | Video room with whiteboard | Recordings | Chat with files | Weekly series | Auto-checked tests and manual review | Groups | One student, several tutors | Data in Russia stated | 10 students | 50 students |
|---|---|---|---|---|---|---|---|---|---|---|
| **OVC MVP** | yes, BigBlueButton, embedded | after the MVP | yes | yes | yes | yes | yes | yes (ADR-0008) | not priced | not priced |
| ProgressMe | yes | no (third-party) | ? | ? | yes | yes | ? | ? | ≈990 ₽ | 990–3,750 ₽ |
| Edvibe | yes, up to 25 | ? | ? | ? | ? | yes | ? | ? | ? | ≈2,800–3,700 ₽ |
| AlfaCRM | no | no | partly (add-on) | yes | no | yes | ? | yes | 2,600 ₽ | 2,600 ₽ |
| GetCourse | partly (webinar) | yes | ? | no | yes | yes | yes (GetCourse ID) | yes, Selectel | 5,900 ₽ | 5,900 ₽ |
| Skillspace | partly (webinar) | yes | ? | ? | yes | ? | ? | ? | 3,490 ₽ | 3,490 ₽ |
| Antitreningi | yes | ? | ? | ? | yes | yes | ? | ? | 1,490–3,100 ₽ | 6,100 ₽ |
| Yandex Telemost | partly (no whiteboard) | partly (local only) | yes | yes | no | no | no | ? | 0 ₽ | 0 ₽ |
| VK Calls | partly (no whiteboard) | yes | yes | yes | no | no | no | ? | 0 ₽ | 0 ₽ |
| VK Tutoria | yes | ? | yes (VK Messenger) | ? | no | yes, up to 100 | ? | ? | 0 ₽ (2024) | 0 ₽ (2024) |
| MTS Link | yes | yes | ? | ? | partly (webinar tests) | ? | no | yes | 0–1,300 ₽ | ≈1,300 ₽ |
| Pruffme | yes | yes | ? | ? | partly (course tests) | ? | no | ? | ? | ? |
| Zoom Pro | yes | yes | yes | yes | no | no | no | no | 1,393 ₽ | 1,393 ₽ |
| Google Classroom and Meet | partly | paid plans | yes | yes | yes | yes | no | no | 0–1,250 ₽ | 0–1,250 ₽ |
| Lessonspace with TutorBird | yes | yes | partly | ? | no | ? | no | no | ≈5,240 ₽ | ≈17,700 ₽ |
| Preply | yes | ? | yes | yes | no | ? | no | no | closed to tutors in Russia | closed to tutors in Russia |

The Lessonspace with TutorBird row adds Lessonspace's 3,853 ₽ or 16,313 ₽ to TutorBird's 1,389 ₽ (derived). Mirapolis and Miro are left out of the table: the first is a corporate tool, the second unavailable in Russia since 15 October 2026.

## What OVC does better, worse or the same

**Better:**

- **One student with several tutors** (FR-2.4) is modelled directly. Among the products checked, only GetCourse documents a cross-school student identity, and it is built for course sellers, not tutors.
- **Everything a tutor needs in one place, built for tutors:** none of the Russian products checked documents all of an embedded room with whiteboard, weekly series in the tutor's timezone, chats with files, homework with auto-checked tests and tutor review, and groups together. Tutors today likely combine a free call tool (Telemost, VK Calls, Max) with a CRM (AlfaCRM) or an exercise platform (ProgressMe, Edvibe) (**estimate** from the product scopes above; no usage survey was found).
- **A stated data-residency position.** Only GetCourse, AlfaCRM and MTS Link were found to state that data is stored in Russia, and only GetCourse names its provider. Every foreign product stores data abroad. Analysis, not legal advice: a tutor who collects students' data into such a product is likely exposed under part 5 of article 18 of 152-FZ as an operator (see the market context for the wording and its exceptions); whether a private tutor is an operator at all, including the personal and family needs exclusion of item 1 of part 2 of article 1, is a question for a lawyer.
- **Scores hidden from students, answer keys server-side,** and relationship isolation between tutors and between students: no competitor documents these as product rules.

**The same:**

- The video room with whiteboard, presentation and screen sharing matches ProgressMe, Edvibe, Antitreningi and VK Tutoria; the BigBlueButton client also includes breakout rooms and polls, which several competitors advertise, but OVC's requirements (FR-5.2) do not list them and OVC may disable them, so they are not counted as an OVC function.
- Email reminders and in-app notifications match the common baseline.

**Worse:**

- **Price of video alone:** Telemost, VK Calls and Max are free and unlimited for the lessons a tutor runs; OVC cannot win on video and must win on integration.
- **Recordings** are missing from the MVP while VK Calls, GetCourse, Skillspace, MTS Link, Pruffme and Zoom have them, and Telemost records locally.
- **Ready-made content and AI:** ProgressMe and Antitreningi sell libraries of interactive exercises and AI generation of lessons and tests; OVC's library starts empty.
- **Native mobile apps and push:** Antitreningi, VK and Yandex have native apps with push; OVC is a web application with phone layouts only in slice 10 and no push in the MVP.
- **Messenger notifications:** AlfaCRM sells Telegram and Max notifications and GetCourse messages through messengers; OVC sends email and in-app notifications (FR-13.1, FR-13.2) and nothing to messengers.
- **Track record:** competitors have years of operation and, in GetCourse's and MTS Link's case, public status pages; OVC starts as a two-tutor pilot on one application VM without failover ([ADR-0013](../../docs/decisions/0013-pilot-deployment.md)).

## Gaps in OVC's MVP that matter to tutors

In the order a tutor is likely to notice them; each is already listed as after the MVP or not in the first release in the [roadmap](../../docs/roadmap.md#after-the-mvp) or the [product overview](../../docs/product.md#not-in-the-first-release), unless marked as new.

1. **Payments and lesson balances** ([proposal 0001](../../docs/proposals/0001-payment-ledger.md)): AlfaCRM and Edvibe are bought for this; a tutor who pays for AlfaCRM today keeps paying for it alongside OVC until OVC has it.
2. **Lesson recordings** (FR-5.4 to FR-5.6): common among competitors and asked for by adult learners who rewatch lessons (the demand is an **estimate**, not measured).
3. **Notifications in a messenger:** Telegram or Max notifications are not in OVC's requirements or roadmap at all (**new**); email-only reminders are weaker than what AlfaCRM sells.
4. **Push notifications and calendar export,** both after the MVP.
5. **Content to start with:** ready-made exercise sets or import of existing materials (**new**); ProgressMe's main selling point is content, not the room.
6. **Student self-booking of slots,** not in the first release; AlfaCRM offers free online booking.

## Pricing implications

- **What a tutor pays today:** 0 ₽ for video alone (Telemost, VK Calls, Max); about 1,000 ₽ for a tutor exercise platform with a room (ProgressMe); 1,500–6,100 ₽ for an online-school platform (Antitreningi, Skillspace, GetCourse); 2,600–3,800 ₽ for a CRM without video (AlfaCRM). The sums are the same for 10 and 50 students on most Russian products, because their smallest plans already cover 50 students; only Antitreningi and the per-student ProgressMe scale grow with the count.
- **What a tutor earns:** at about 1,000 ₽ an hour ([survey summary](https://skillbox.ru/media/education/skolko-stoit-zanyatie-s-repetitorom-i-ot-chego-zavisit-cena/), **third-party**) and the usage profile above, 10 students bring about 40,000 ₽ a month and 50 students about 200,000 ₽ (**estimate**). Today's tools therefore take 0–15% of a small tutor's income and 0–3% of a busy tutor's.
- **What OVC costs to run:** the pilot costs about 9,500 ₽ a month for at most two tutors ([ADR-0013](../../docs/decisions/0013-pilot-deployment.md)), about 4,750 ₽ per tutor. The larger setups in the [hosting research](hosting-2026-10-22.md#pilot-about-20-tutors-200-students-mvp-per-month) cost about 27,850 ₽ for 20 tutors (about 1,390 ₽ per tutor) and about 60,300 ₽ for 200 tutors (about 300 ₽ per tutor) (derived).
- **Consequence:** at pilot size OVC's cost per tutor is above what most tutors pay for any single tool, so the pilot cannot be priced to cover its costs and should be free to its tutors. At 20 tutors the cost per tutor approaches ProgressMe's price (about 1,390 ₽ against ≈990 ₽, about 1.4 times); at 200 tutors a price of 1,000–2,000 ₽ a month, between ProgressMe and AlfaCRM, would cover hosting several times over (**estimate**, hosting only, excluding the operator's time, support and payment fees). Pricing flat per tutor rather than per student matches how Skillspace, GetCourse and AlfaCRM price for the sizes in question.

## Hosting, reliability and scale: how the others do it

- **Own media servers are cheap per lesson but need a team.** Skyeng cut its video cost from about 113 ₽ per lesson on a SaaS to about 2 ₽ on its own Janus servers ([Skyeng on Habr, 2019](https://habr.com/ru/post/446444)); Yandex replaced an external media server with its own Goloom in 2024 ([Yandex on Habr](https://habr.com/ru/articles/834512/)); VK runs its own scaled call servers ([VK press release](https://m.vk.ru/press/calls-no-limit)); Preply buys video from Agora. For OVC the self-hosted BigBlueButton host is the same choice as Skyeng's, without Skyeng's team.
- **Utilisation drives unit cost.** At the pilot's size the BigBlueButton host costs about 5,900 ₽ a month for two tutors; if each teaches about 60 lesson-hours a month, that is about 49 ₽ per lesson-hour (**estimate**: 5,900 ₽ divided by 120 lesson-hours), against Skyeng's 2 ₽ at scale. The cost falls only as more tutors share the host, which is why starting the host only for lessons or a cheaper preinstalled VM (NetAngels, about 1,771 ₽ for the same CPU and RAM with a smaller disk, comparability **unconfirmed**) is worth rechecking before the pilot grows.
- **Bandwidth and network, not CPU, were Skyeng's limit.** Quality degraded beyond about 420 rooms per server while CPU and memory were far from their limits, and was fixed by capping bandwidth per server ([Skyeng on Habr, 2019](https://habr.com/ru/post/446444)). OVC's slice 7 probe already measures the host's bandwidth against BigBlueButton's 250 Mbit/s ([ADR-0013](../../docs/decisions/0013-pilot-deployment.md)); packet loss under load is worth measuring with it.
- **WebRTC blocking is a real risk in Russia.** In 2022 Skyeng's lesson cancellations for technical reasons rose from about 1–1.5% to up to 16% when its media subnets were blocked or throttled; relays (coturn) next to each media server and a TCP fallback were the fix ([Skyeng on Habr, 2022](https://habr.com/en/companies/skyeng/articles/663318)). The [BigBlueButton research](bigbluebutton-2026-10-20.md#open-probe-items-for-slice-7) does not yet cover a TURN relay on TCP 443 for students behind restrictive networks; adding it to the slice 7 probe is the lesson for OVC.
- **Large providers fail too, and visibly.** Yandex services including Telemost failed on 17 September 2026 and Yandex Cloud on 12 October 2026 (see the Telemost section, **third-party** reports); Edvibe's users report outages in October 2026 with lessons cancelled ([Trustpilot](https://www.trustpilot.com/review/edvibe.com), **third-party**). OVC's pilot puts the application on Yandex Cloud and BigBlueButton on VK Cloud, and the two failures are not symmetric. A VK Cloud outage stops the video room while the platform, chats and homework stay up; the new-tab fallback (FR-5.3) does not help then. A Yandex Cloud outage stops the platform and also new joins to lessons, because join links come from OVC, and email, because Postbox is in Yandex Cloud; lessons already running on BigBlueButton may continue (**unconfirmed**, depends on how BigBlueButton treats an unreachable callback host).
- **Public status pages are rare and minimal.** Only GetCourse and MTS Link were found with status pages, neither with uptime history; no Russian competitor publishes an SLA. An external availability check with a simple public status page would already match the market.
- **Data-residency statements are a selling point.** GetCourse's policy names the provider (Selectel) and states that every server holding personal data is in Russia ([privacy policy](https://getcourse.ru/privacy_policy)); AlfaCRM cites part 5 of article 18 of 152-FZ explicitly ([privacy policy](https://alfacrm.pro/legal/privacy_policy)). OVC's privacy policy, an owner obligation under ADR-0008, can do the same with Yandex Cloud and VK Cloud.
- **Hosting cost of competitors is not public.** No competitor publishes its hosting bill. As an **estimate**, a tutor platform the size ProgressMe reported in 2021 (50,000 tutors) running one-to-one lessons on its own media servers at Skyeng's unit cost of about 2 ₽ per lesson would spend about 300,000–600,000 ₽ a month on video (basis: an assumed 5–10% of tutors, 2,500–5,000, teaching about 60 lessons a month each, so 150,000–300,000 lessons at 2 ₽; Skyeng's 2019 unit cost is not adjusted for later price changes), plus servers for the application. If those 2,500–5,000 tutors each paid 1,000 ₽ a month, revenue would be 2.5–5 million ₽, so video would take about an eighth of it. Webinar platforms such as GetCourse pay mainly for egress to large audiences, which at one 720p stream of about 1.5 Mbit/s is about 0.7 GB per viewer-hour (**estimate** from the bitrate); their totals cannot be estimated without audience figures.

## Not confirmed

- Current official prices of ProgressMe, Edvibe, Pruffme and Yandex 360 business plans; whether VK Tutoria is still offered and free in October 2026.
- Storage location and hosting provider of ProgressMe, Edvibe, Skillspace, Antitreningi, Yandex Telemost, VK Calls and Pruffme.
- Media technology behind the rooms of ProgressMe, Edvibe, Antitreningi and Skillspace.
- Whether any competitor other than GetCourse supports one student account with several tutors.
- Whether NetAngels' BigBlueButton VMs have dedicated cores, unmetered traffic and a data centre in Russia.
- Uptime figures or SLAs for any Russian product; none was found.
