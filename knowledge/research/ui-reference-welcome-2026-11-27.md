# Welcome page: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R10 `feat/redesign-welcome` of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The screen is the public page a person without a session sees at `/`: a hero with one primary call to action, sections describing implemented capabilities, a short "how to start", questions and answers, and a footer with the legal documents. Sign in moves from `/` to `/login`. The study looks at structure only: the hero, the number of calls to action, where sign-in sits, the feature blocks, social proof, the questions section and the footer. It does not look at the compared products' prices, trials or data practices.

OVC today:

- `/` is Sign in, and signed-in people are redirected to their home page ([public addresses](../design/web/navigation.md#public-addresses), [Sign in](../design/web/pages.md#sign-in)).
- The public frame holds the logo, the language switch and sign-in or sign-up links; on phones they share one row that wraps, with no bottom bar ([frames](../design/web/navigation.md#frames)).
- The legal documents live at `/privacy`, `/consent` and `/terms`, reachable signed in or out, rendered from `frontend/src/legal/{ru,en}/` by the restricted Markdown renderer, loading nothing from a third party ([legal documents](../design/web/pages.md#legal-documents)). The interface carries no legal wording beyond the consent label and the acceptance line ([public pages](../design/web/pages.md#public-pages)).
- [ADR-0024](../../docs/decisions/0024-legal-documents-and-student-data.md) decision 4 makes the user agreement a public offer of the free service, and no legal text is published before a lawyer's review.
- Route and redirect code: `frontend/src/routes/router.tsx`, `frontend/src/routes/access.ts` (anonymous redirect builder, `safeNext`, post-sign-in redirect), `frontend/src/features/auth/SignInPage.tsx` (the `sessionEnded` and `accountDeleted` notices), `PublicFrame.tsx`.

Owner's decisions that bind the screen (plan section 3): item 11 (no price, trial duration, unlimited entitlement or recordings claims until separately approved; residency or legal assurances only in approved legal text; `/` becomes welcome and `/login` Sign in; signed-in `/` still redirects home), item 16 (one action is primary; legal and navigation anchors without underline), item 6 (no underlined action links), item 4 (one flat page background), items 2 and 3 (Warm paper palettes), item 18 (this study, named tests and an owner-approved task plan before any branch). The plan's section 4 makes welcome a lazy public route so marketing texts stay out of the entry bundle.

Mockup: `knowledge/design/web/prototype/redesign/welcome.html` at `8b3a8b0`. It draws a header with Features, Pricing and Questions anchors, "Войти" and "Начать бесплатно"; a hero with "Начать бесплатно" and a secondary "Как это работает", a "[N] days free, then 490 ₽ a month" line and a "data stored in Russia" line; a synthetic Today card standing in for a screenshot; six feature cards (video lessons "with recording", chats, schedule with reminders, homework, students and groups, data export and residency); three "how to start" steps; a one-tariff pricing block; five questions; a closing call to action; and a footer with "[operator]", "[support contact]" and the three legal links. Section 3 item 11 removes the pricing block, the trial and price lines, the "any number of students" entitlement and the recording claim, and the residency lines until legal text is approved.

## How the compared products solve it

### TutorBird (tutorbird.com)

- The header holds Features (a dropdown), Pricing, Contact, Support, then "Join Free", "Book a Demo" and Login as the last item; Login points to a separate application host, app.tutorbird.com, and a language switch (English / Français) sits beside it [1].
- The hero has one button, "Get started today!", under a claim of long standing and category position [1].
- The same call to action repeats four times in the body (the hero, two trial bands and a closing "Ready to get started?" block), all to one sign-up address; the header adds "Join Free" and "Book a Demo" [1].
- First comes an audience block ("Designed for tutors of all kinds": private tutoring, centres, travelling, online), then one feature band per capability: lessons, students, payments, website, student portal, consultations [1].
- Social proof is a band of review-site ratings (three sites at 4.8, one with 265 reviews) and named testimonials, placed between the feature bands [1].
- There is no questions section on the home page [1].
- The footer has a Features column, a Useful Links column that holds Privacy Policy and Terms of Service, a contact column with an email and phone numbers, social links and the copyright of the legal entity [1].

### Teachworks (teachworks.com)

- The header has Features, Solutions, Testimonials, Pricing and Support menus, a top-level Login link, and two buttons, "Try it for Free" and "Book a Demo" [2].
- The hero headline rotates through audiences ("Simplify & Automate Your" tutoring business, music school, driving school…), with the subheadline "Teach more, manage less." and three calls to action: "Try it for Free", "Book a Demo" and "Or Take a Quick Tour" [2].
- Sections follow: audience cards, a six-item capability grid ("Everything you Need To Run Your Teaching Business": scheduling, students, billing, employee hours, communication, reports), integrations, worldwide use and scaling [2].
- Social proof is two numbers (languages, countries) and two named testimonials with a "See More Reviews" link [2].
- There is no questions section on the home page; "FAQs" is only a Support menu item [2].
- The closing block pairs "Start Your Free Trial" with "Take a Quick Tour" [2].
- The footer has Navigation, Resources, Solutions, Our Company and a separate Legal column (Privacy Policy, Terms & Conditions, Cookie Policy, AI Policy) [2]. The fetched page also showed leftover website-template content in the closing block and footer [2].

### Weekify (weekify.ru)

- The home page HTML returned on 2026-11-27 holds only the title "Викифай - CRM для репетитора" and the brand name; the landing content is rendered by client-side script, so its sections, buttons and footer could not be read from the page itself [3].
- `robots.txt` reads `Disallow: /` and `Allow: /$` [4], so this study fetched only the home page and `robots.txt`, and no script bundle or other path.
- Earlier research of 2026-11-23 recorded a built-in question-and-answer assistant on the landing page whose entries ship in the public script bundle [5]. That observation was not rechecked today because rechecking it would mean fetching files the robots rule covers; it is cited as prior repository research, not as a 2026-11-27 finding.

### Preply, tutor page (preply.com/en/teach)

- The header has a language selector, a currency selector and "Log In" at the far right, apart from the main navigation [6].
- The hero is a single headline followed by three numbered steps ("Sign up", "Get approved", "Start earning") [6].
- One primary button, "Create a tutor profile now", repeats four times down the page; the secondary actions are links ("Log In", "Check our Help center") [6].
- Three benefit blocks follow, then a six-item benefits list, one named testimonial, and a "Frequently asked questions" section with eight questions on requirements, earnings, commission and equipment [6].
- The footer holds a legal group (Legal Center, Privacy Policy, Cookie Policy, Legal Notice), the copyright and audience columns [6].

## Comparison

| Design question | TutorBird [1] | Teachworks [2] | Weekify [3, 4] | Preply tutors [6] | OVC decision/target |
|---|---|---|---|---|---|
| Sign-in placement | Header, last item, separate app host | Header, top-level Login link | Not readable (script-rendered) | Header, far right, apart from main nav | Header text link to `/login`, secondary to the one primary action |
| Primary calls to action in the hero | 1 | 3 (trial, demo, tour) | Not readable | 1 (after the steps) | 1 (plan item 16); any second hero action is secondary |
| Repetition of the primary action | 4 in the body, plus 2 header buttons | Header, hero, closing block | Not readable | 4 | Task plan to settle under "one primary in each active screen state" |
| Feature blocks | One band per capability | Six-item grid | Not readable | Three blocks plus six-item list | One block per implemented capability only (plan item 11) |
| Audience segmentation | Yes, four kinds of tutor | Yes, six kinds of school | Not readable | No (one audience) | No: one audience, private tutors of adults |
| Social proof | Review-site ratings, testimonials | Two numbers, two testimonials | Not readable | One testimonial, reach claims | None until real, approved material exists |
| "How it works" steps | No | "Quick Tour" link | Not readable | Three steps under the hero | Three steps as drawn, wording checked against real flows |
| Questions section | None | None (menu link only) | Assistant recorded 2026-11-23 [5], not rechecked | Eight questions | A short section, answers limited to implemented behavior and approved legal text |
| Price on the page | Trial only; prices on a separate page | No; separate page | Not readable | Earnings and commission in questions | None (plan item 11; ADR-0024 decision 4) |
| Footer legal links | Privacy, Terms in a links column | Separate Legal column, four documents | Not readable | Legal group, four documents | `/privacy`, `/consent`, `/terms`; operator and contact only as approved |
| Demo or tour | "Book a Demo" | "Book a Demo", "Quick Tour" | Not readable | No | No demo booking (no sales channel, no third-party scheduler) |

## What OVC adopts

- **One primary action in the hero**, with sign-in as a quieter header link: TutorBird's single hero button [1] and Preply's single repeated button with secondary links [6]. Maps to plan item 16 and the R10 acceptance criterion "only one primary CTA in each active screen state".
- **Sign-in as a separate, always visible header item** on its own address: all three readable products put Login in the header, Preply and TutorBird at the far end [1, 2, 6]. Maps to item 11 (`/login`) and keeps the public frame's existing sign-in link ([frames](../design/web/navigation.md#frames)).
- **A short numbered "how to start" sequence**: Preply's three steps under the hero [6]. The mockup already draws three steps; each must describe a flow that exists (sign up, invite by link or code, schedule and hold a lesson).
- **Feature blocks per capability**: TutorBird's bands and Teachworks' grid [1, 2]. OVC keeps the mockup's grid but writes each block only from implemented behavior (item 11).
- **A questions section on the page**: Preply's tutor page [6]; TutorBird and Teachworks have none [1, 2], so this follows the minority but suits OVC, which has no help centre to link to. Each answer stays inside implemented behavior and approved legal text.
- **Legal documents in the footer**: all readable products link privacy and terms from the footer [1, 2, 6]; OVC links its three existing pages.

## Where OVC differs and why

- **No price, trial or entitlement claims**: TutorBird advertises a trial [1]; OVC's user agreement is a public offer of a free service (ADR-0024 decision 4) and plan item 11 forbids price, trial-duration and unlimited-entitlement claims until approved. Whether a label such as "Start free" counts as such a claim is an open point.
- **No recordings promise**: the mockup's "with recording" is removed (item 11) until separately approved.
- **No residency or legal assurances in interface copy**: the public pages carry no legal wording beyond the consent label and acceptance line ([public pages](../design/web/pages.md#public-pages)); any "data stays in Russia" line needs approved legal text (item 11, ADR-0008).
- **No social proof**: OVC has no published reviews or usage numbers; inventing them would be a fabricated claim. Review-site badges would also load third-party content, which the content security policy and ADR-0008 rule out.
- **No demo booking, tour video or chat assistant**: TutorBird and Teachworks offer demos [1, 2] and Weekify was recorded with an assistant backed by its own API [5]; OVC has no sales channel, and an assistant or embedded video would add an outbound service or untrusted content.
- **One audience**: TutorBird and Teachworks segment by school type [1, 2]; OVC serves private tutors of adults and has no school mode.
- **No separate application host**: TutorBird's Login leaves for another host [1]; OVC keeps welcome and `/login` in one application, with the welcome texts in a lazy chunk.
- **No screenshot of real data**: the mockup's "real screenshot" placeholder becomes a synthetic illustration with invented names (the R10 checks name "synthetic illustration contents"), never a capture of an account.
- **Accessibility**: one `h1`, landmark regions, a skip link as on other frames, 44 px targets on phones, and anchors without underline outside message text only where they stay distinguishable (WCAG 2.2, plan items 6 and 16).

## Open points for the task plan

- Whether a header button and a closing-section button with the same label as the hero action count as one primary action per active screen state, or the header keeps only text links.
- The wording of the primary action ("Create an account", "Start", "Start free") and whether "free" is acceptable under ADR-0024 decision 4 without separate approval.
- The list of implemented capabilities each feature block may name, checked against the specifications: reminders to students, group lessons, the tutor's and the student's data export, "one student with several tutors".
- Whether the footer shows an operator name and a support contact, and from which approved source, or omits them until the legal texts are reviewed.
- Whether the questions section has a fixed list, and how answers that touch data handling defer to `/privacy` instead of paraphrasing it.
- Whether a signed-in person sees welcome at any address (for example via a "About OVC" link) or only the redirect, and whether the language switch on welcome behaves as on other public pages.
- What the synthetic illustration shows and how a test proves it holds no real data and loads nothing from a third party.
- How `?next=` arriving at `/` from old links or emails is handled after the move to `/login`.

## Unverified

- The content of Weekify's landing page (hero, buttons, sections, footer) on 2026-11-27: not readable from the home page HTML; the bundle was deliberately not fetched because of the robots rule.
- Weekify's landing assistant: recorded on 2026-11-23 [5], not rechecked.
- TutorBird's full hero headline: the fetched text showed only "Helping you…", likely an animated rotating phrase; the rest was not confirmed.
- Phone layouts of all compared pages (menu collapse, sticky header buttons): not checked.

## Sources

1. TutorBird — home page — <https://www.tutorbird.com/> — FP — checked 2026-11-27 — hero headline only partly captured.
2. Teachworks — home page — <https://teachworks.com/> — FP — checked 2026-11-27 — page contained leftover template content.
3. Weekify — home page — <https://weekify.ru/> — FP — checked 2026-11-27 — app shell only; title and brand name readable.
4. Weekify — robots.txt — <https://weekify.ru/robots.txt> — FP — checked 2026-11-27.
5. OVC repository — Weekify public facts — [weekify-2026-11-23.md](weekify-2026-11-23.md) — prior repository research of 2026-11-23 (FP bundle observation) — not rechecked 2026-11-27.
6. Preply — "Become a tutor" page — <https://preply.com/en/teach> — FP — checked 2026-11-27.
