# A tutor CRM competitor and mobile app distribution from Russia: facts checked on 2026-10-25

Checked 2026-10-25. Product features, prices, store rules and salaries are volatile; recheck them before relying on them. This file is agent research: it supplies evidence for the [post-pilot plan and mobile app analysis](../analysis/post-pilot-plan-and-mobile-app-2026-10-25.md) and decides nothing. It adds one product to the [competitors research](competitors-2026-10-22.md).

## Method and conventions

- The competitor was read from its public landing pages only; no account was created, so the inside of its application was not seen. Its features are as the vendor describes them.
- Sources that are not the vendor are marked **third-party**; facts no reachable source confirmed are marked **unconfirmed**.

## Weekify (Викифай), a CRM for tutors

Source: the [home page](https://weekify.ru/) and the [mobile page](https://weekify.ru/mobile), read on 2026-10-25.

- **Positioning:** "CRM для репетитора" ("a CRM for a tutor"), "виртуальное рабочее место современного репетитора" ("the virtual workplace of a modern tutor"); "Сделано в России" ("made in Russia").
- **Price:** 14 days free, then 350 ₽ a month; no card needed for the trial.
- **Students:** cards with whatever the tutor wants to record (the page lists parents' contacts, birthday, a pet's name, learning data and goals); students can be grouped.
- **Schedule:** regular lessons, rescheduling and cancelling (from user reviews quoted on the page).
- **Notifications:** reminders before a lesson, when a student's balance runs out, and before a student's birthday; the page shows them as chat-style messages, but the channel (messenger bot, push or email) is **unconfirmed**.
- **Finances:** recording payments, balances per student, debits for lessons, payment reminders.
- **Statistics:** "12 основных метрик" ("12 main metrics"), shown with monthly earnings and expenses compared with the previous month.
- **Students and parents:** invited students and parents receive reminders before lessons and payment reminders, and see the current schedule, homework, attendance, and the history of payments and debits.
- **Other:** online booking (the tutor sends a link and the student picks a free weekday and time), a task tracker for the tutor.
- **Mobile:** separate apps for the teacher and for the student or parent; iOS through the App Store, Android with a choice of installation methods (the methods are not listed without further clicks, **unconfirmed**).
- **Not mentioned on the page:** a video room, a whiteboard, chat, self-checking tests or a homework builder.
- **Interface, from the landing page's illustrations:** white and cool light-grey surfaces, a saturated blue accent on buttons and panels, rounded cards with shadows, multicoloured student tiles, a theme switch. The application itself was not seen.
- **Community and support:** a VK group; the operator, its legal form and where data is stored are **unconfirmed** (the privacy policy is a PDF whose text could not be extracted).

## Publishing mobile apps from Russia

- **Apple Developer Program (99 USD a year,** [Apple's program page](https://developer.apple.com/programs/)**):** reports in 2026 say developer.apple.com declines Russian cards, that paying from a mobile phone balance stopped working in April 2026, and that developers use gift cards, intermediaries, foreign cards or a change of account region, each with risk ([AppleInsider](https://appleinsider.ru/tips-tricks/oplata-apple-developer-program-kakie-sposoby-ostalis-u-razrabotchikov.html), [vc.ru](https://vc.ru/services/2968860-oplata-apple-developer-program-iz-rossii-s-pomoshchyu-bybit-card), **third-party**). No official Apple route for Russian payment was found.
- **Google Play:** Google stopped payouts to Russian developers on 2024-12-26 and does not allow new Google accounts with a Russian phone number (**third-party** reports: [ITC](https://itc.ua/en/news/google-play-will-stop-monetization-for-apps-from-russia-on-december-26), [Gigazine](https://gigazine.net/gsc_news/en/20240927-russia-block-create-google-account)).
- **Android developer verification:** Google introduces mandatory identity verification for developers of apps for Android 7 and newer, starting 2026-09-30 in four countries and worldwide within about a year; developers in Russia cannot be verified because of US sanctions, and the rule covers Google Play, other stores including RuStore, and APK files; unverified apps can still be installed after the user enables developer mode, confirms and waits 24 hours ([The Moscow Times, 2026-08-03](https://ru.themoscowtimes.com/2026/08/03/google-zapretit-rossiiskim-razrabotchikam-vikladivat-prilozheniya-dlya-drugih-stran-a202528), **third-party**, citing Google's statement to Ars Technica). How it will apply on devices sold in Russia is **unconfirmed**.
- **RuStore:** its developer tools page lists payments, updates, push notifications (sent through FCM, HMS or RuStore's own transport) and reviews as free, and says most tools are free or have free basic features; the cost of publishing is not stated ([RuStore developer tools](https://www.rustore.ru/developer/tools)).
- **Web push delivery:** the Web Push protocol encrypts message payloads end to end between the sending server and the browser ([RFC 8291](https://www.rfc-editor.org/rfc/rfc8291)), so the browser vendor's push service sees the subscription endpoint, the sender, timing and size but not the content; no browser vendor's price for push delivery was checked (**unconfirmed**, commonly free).
- **Web push on iOS:** supported for web apps added to the Home Screen since iOS and iPadOS 16.4, after the user installs the app and grants permission ([announcement summary](https://www.iclarified.com/89346/apple-announces-web-push-for-web-apps), **third-party**); developer reports describe subscriptions that stop working after one or two weeks ([webscraft](https://webscraft.org/blog/pwa-pushspovischennya-na-ios-u-2026-scho-realno-pratsyuye?lang=en), **third-party**, anecdotal).

## Mobile developer pay in Russia

Vacancies for middle React Native developers on Habr Career in October 2026 show ranges from 70,000–110,000 ₽ to 160,000–200,000 ₽ a month and more ([example](https://career.habr.com/vacancies/1000095107), [example](https://career.habr.com/vacancies/1000157771), **third-party**, individual vacancies, not a salary survey).

## Not verified

- Anything inside Weekify's application, its notification channel, its data location and its Android installation methods.
- Whether the Android verification rule will block installing an unverified app on devices sold in Russia, and when.
- The cost of publishing on RuStore.
- A lawful, reliable way to pay the Apple Developer Program from Russia.
