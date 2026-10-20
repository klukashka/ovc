# ADR-0010: Quiet visual language, designed for computers first

- Status: accepted, 2026-10-20
- Reviewed: direction chosen by the project owner on 2026-10-20 from a clickable prototype; wording pending line-by-line owner review
- Deciders: project owner
- Related: NFR-5.2, FR-15.3, [product overview](../product.md#visual-language), [design tokens](../../knowledge/design/web/tokens.md), [navigation](../../knowledge/design/web/navigation.md), [ADR-0008](0008-hosting-and-data-residency.md)

## Context

The first page design described a "warm and friendly" style: a rounded geometric typeface (Nunito) in heavy weights, a terracotta accent, every block in a white card with a shadow, filled status pills, and student screens laid out for a phone first with bottom navigation. A clickable prototype built on those tokens read as generic and machine-made to the owner: everything had equal weight, the typeface felt artificial rather than warm, and the phone-width student screens left most of a computer screen empty, although students often study at a computer.

Three alternative directions were compared on the prototype: a notebook look with ruled lines and a red-pen accent, a quiet editorial look, and a tactile workshop look with earthy colours and illustrations. The owner chose the quiet editorial direction, with calm text-first interfaces and book typography as references.

## Decision

- **Quiet, text-first interface.** Hierarchy comes from typography, alignment and space. Lists are rows divided by hairline rules; a bordered panel is reserved for the one element per page that must stand out; page content has no shadows.
- **Typefaces:** Golos Text for the interface, Literata only for page titles and quoted feedback, JetBrains Mono for codes; all self-hosted.
- **Colour:** warm paper background, near-black ink text, one muted olive accent whose filled form marks only the primary action of a view (its other uses are the tab underline, selected radios and checkboxes and the focus ring; its darker text shade colours links, the news dot and the states that invite action); statuses are words with a small dot, never filled pills; tutors are told apart by an 8 px dot from a muted earthy palette. Light and dark themes both follow this language.
- **Navigation:** both roles use a collapsible left sidebar in the manner of document tools: the user's name at the top, sections with outline icons and counts, notifications and settings in the same list.
- **Student Home uses the computer width.** Besides the next lesson, work to do and recently checked work (now with the opening of the feedback and a corrected-file note), Home gains a side column with the lessons of the next seven days and the student's tutors; FR-15.3 and the `/home` response are amended accordingly.
- **Computers first for both roles.** Every page, including the student's, is designed for a computer screen first. Phone layouts are an adaptation designed in a later slice; until then pages degrade to one column and stay usable from 360 px.

## Alternatives considered

- **Keep the warm, friendly card style:** rejected by the owner after seeing it in the prototype.
- **Notebook look** (ruled paper, red-pen accent): characterful, but the paper metaphor competes with real content such as homework files and corrected scans.
- **Workshop look** (earthy palette, hand-drawn illustrations, tutor photos): the warmest option, but hard to keep consistent across many screens and dependent on illustration work.
- **Phone first for students:** fits short sessions, but students also do homework and join lessons from computers, where a phone layout wastes the screen.

## Consequences

- [Design tokens](../../knowledge/design/web/tokens.md) are rewritten: typefaces, type scale, radii, elevation, light and dark colours with measured contrast, tutor colours and component rules.
- The [product overview](../product.md#visual-language), [navigation](../../knowledge/design/web/navigation.md), [states](../../knowledge/design/web/states.md) and [pages](../../knowledge/design/web/pages.md) describe the sidebar, dots instead of pills and computer-first student pages.
- The frontend scaffold self-hosts Golos Text, Literata and JetBrains Mono instead of Nunito.
- FR-15.3 and the student `/home` response in the [API specification](../../knowledge/specs/api.md) gain the next seven days' lessons, the tutors, feedback openings and the corrected-file note.
- NFR-5.2 is amended from "student pages are designed for phones first" to computers first for every page.
- Until dedicated phone layouts land in slice 10 of the [roadmap](../roadmap.md), narrow screens use a top bar with an overlay sidebar and one-column pages; a student bottom bar is a candidate for that slice, not a decision.
- A [clickable prototype](../../knowledge/design/web/prototype/README.md) of five screens is kept as a visual reference; the specifications remain normative.

## Where it is applied

- [Design tokens](../../knowledge/design/web/tokens.md)
- [Navigation and addresses](../../knowledge/design/web/navigation.md#frames)
- [Pages](../../knowledge/design/web/pages.md)
- [States](../../knowledge/design/web/states.md#tutor-labels)
- [Stack table](../../knowledge/architecture/detailed.md#stack-per-component)
