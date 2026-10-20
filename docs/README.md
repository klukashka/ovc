# OVC documentation

Status: draft written 2026-10-20; pending line-by-line owner review.

This folder is written for people. It holds what OVC is for, what it must do, how it is built and why. Every statement here is reviewed and accepted by the project owner, and each file records its review status. The reference material that an engineer or a coding agent consults section by section (normative specifications, detailed maps, research, review records) lives in [`knowledge/`](../knowledge/README.md) and is linked from here wherever a decision depends on it.

## Reading order

1. [technical-introduction.md](technical-introduction.md): the parts of the system, one lesson through it, and why the boundaries sit where they do.
2. [requirements/functional.md](requirements/functional.md): what OVC must do, numbered `FR-n.m`.
3. [requirements/non-functional.md](requirements/non-functional.md): security, performance, scale, reliability, accessibility and verification targets, numbered `NFR-n.m`.
4. [threat-model.md](threat-model.md): assets, actors, trust boundaries and threats `T-n` with their controls.
5. [decisions/](decisions/README.md): architecture decision records and why each choice was made.
6. [roadmap.md](roadmap.md): build order, slice by slice.
7. [development.md](development.md): tool setup, checks and local review.
8. [proposals/](proposals/README.md): changes designed but not yet decided or scheduled.

Rules for contributors and coding agents are in [AGENTS.md](../AGENTS.md).
