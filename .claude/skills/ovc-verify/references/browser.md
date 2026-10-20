# Browser review

Act as an independent reviewer. Read `AGENTS.md`, the affected API contracts and the frontend sections of `knowledge/architecture/detailed.md` linked from `knowledge/README.md`. Review only the assigned component/profile without source edits. You may delegate narrower work in fresh contexts with inherited scope and permissions; own the combined conclusion and report descendant assignments. Return findings, scenario evidence and missing coverage using the parent skill's evidence format.

Before implementation, examine changed interactions, state transitions, loading, empty, error and stale states, role-dependent views and accessibility against the requirements and specifications. A design verdict is separate from browser execution.

For every frontend or backend change with a runnable frontend, run Playwright common smoke plus the affected journeys against the real backend and database, with the fake meeting provider configured. Do not stub routes or HTTP transports. If no visible journey changes, explain why common smoke plus component checks covers the effect. Use isolated local data.

- Install listeners before navigation for console warnings and errors, uncaught exceptions, failed requests and HTTP error responses; `requestfailed` alone misses HTTP failures. Happy paths allow zero unexpected diagnostics. Negative scenarios match the exact expected status, message and context; no broad ignores or warning allowlists.
- Keep retries at zero and retain the trace, screenshot and diagnostics on the first failure. Wait on explicit state with a deadline; do not synchronize with sleeps or `networkidle`. Fix flakiness instead of retrying to green.
- Exercise changed actions as both a teacher and a student, authorization denials, partial and stale data, error recovery, and WebSocket disconnect and reconnect for chat and notifications. Verify that untrusted content (messages, names, file names, feedback) renders as text.
- For video, verify that joining mounts the embed with the room and display name the backend returned, that leaving or navigating away disposes of it, and that a user without access sees the refusal rather than an embed. Real media is outside browser review; the fake meeting provider stands in for the conferencing service.
- Check keyboard navigation and focus order, focus handling in dialogs, automated accessibility (axe once adopted), and responsive layouts at phone, tablet and desktop widths.
- Use a production build for end-to-end runs and a small development-mode smoke for React development-only warnings.

The first runnable frontend shell activates boot, diagnostics and accessibility checks; journeys for features not yet built stay `pending implementation` with their roadmap slice. Browser testing does not substitute for separate performance or security review.
