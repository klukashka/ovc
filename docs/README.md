# OVC documentation

Status: draft written 2026-10-20, amended the same day with the product overview; pending line-by-line owner review.

This folder is written for people. It holds what OVC is for, what it must do, how it is built and why. Every statement here is reviewed and accepted by the project owner, and each file records its review status. The reference material that an engineer or a coding agent consults section by section (normative specifications, detailed maps, research, review records) lives in [`knowledge/`](../knowledge/README.md) and is linked from here wherever a decision depends on it.

## Reading order

1. [product.md](product.md): who OVC is for, what tutors and students do, the main screens and the visual language (non-normative).
2. [technical-introduction.md](technical-introduction.md): the parts of the system, one week of tutoring through it, and why the boundaries sit where they do.
3. [requirements/functional.md](requirements/functional.md): what OVC must do, numbered `FR-n.m`.
4. [requirements/non-functional.md](requirements/non-functional.md): security, performance, scale, reliability, accessibility and verification targets, numbered `NFR-n.m`.
5. [threat-model.md](threat-model.md): assets, actors, trust boundaries and threats `T-n` with their controls.
6. [decisions/](decisions/README.md): architecture decision records and why each choice was made.
7. [roadmap.md](roadmap.md): build order, slice by slice.
8. [development.md](development.md): tool setup, checks and local review.
9. [proposals/](proposals/README.md): changes designed but not yet decided or scheduled.

Rules for contributors and coding agents are in [AGENTS.md](../AGENTS.md).
