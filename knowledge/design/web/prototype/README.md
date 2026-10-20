# Clickable prototype

A low-fidelity, clickable prototype of five screens in the visual language decided in [ADR-0010](../../../../docs/decisions/0010-quiet-visual-language.md). It is a visual reference for the [tokens](../tokens.md), [navigation](../navigation.md) and [pages](../pages.md); those specifications stay normative, and where the prototype differs from them the specifications win. The data on the screens is invented.

| File | Screen |
|---|---|
| `today.html` | Tutor: Today |
| `builder.html` | Tutor: assignment builder, with the student preview |
| `review.html` | Tutor: two-pane review |
| `student-home.html` | Student: Home, with the tutor filter |
| `student-lesson.html` | Student: lesson page, with every join state |

## Opening it

The pages load `app.css` and `prototype.js` from this folder and the Golos Text and Literata fonts from Google Fonts, so they need a local web server and an internet connection (the product itself self-hosts its fonts). From the repository root:

```bash
python3 -m http.server 8765 --bind 127.0.0.1 --directory knowledge/design/web/prototype
```

Then open <http://127.0.0.1:8765/today.html>. The sidebar switch "Смотреть как" moves between the tutor and the student; grey sidebar items are screens the prototype does not have.

## How it is built

Each page is plain HTML with its markup inside an `<x-page>` element and a small `Component` class (extending `PageLogic`) in a `<script type="text/x-page">` block. `prototype.js` builds the shared frame from the page's `data-role` and `data-page` attributes and renders the markup: `{{ name }}` holes take values from the component's `renderVals()`, `<x-if value="{{ flag }}">` shows its content when the flag is true, and `onClick="{{ handler }}"` attaches a handler. `setState` re-renders the page and keeps keyboard focus on the same control. There is no build step and no dependency.

The frame follows [navigation](../navigation.md#frames): a hideable sidebar from 1024 px, and below that the interim top bar with an overlay sidebar (`Esc`, a click outside or choosing an item closes it). Deliberate departures from the specifications: grey sidebar items stand for screens the prototype does not have (the product never shows unavailable items), the "Смотреть как" switch exists only here, only the light theme and Russian copy are drawn, fonts come from Google Fonts instead of self-hosted files, the live dot does not pulse, and the join state's live region is re-created on each change, so screen readers may not announce it as the product must. Also: sidebar items are full page loads that do not move focus to the new heading, "Задания" opens the builder rather than the library, the review status change has no live region, Home does not mark new feedback, the builder preview shows a fixed title, and focus after a re-render is matched by element position.
