# Observe the full account-body abort outcome

Status: security planning assessment completed; owner-approved on 2026-11-04.
Standard tier, security trigger T-20/T-41. Production code is unchanged.

The corrected lifecycle implementation passed 85 targeted tests, 898 frontend tests,
three real-time signal checks, the observed Chromium control and reset in all three
browsers. The new shared real-transport contract failed because its first read after
abort did not reject. The trace does not establish why that first read succeeded.
A read can yield a chunk without establishing the outcome of the whole body; the
contract must examine the complete read lifecycle instead of assuming the first
read is necessarily terminal. Preserve the failed result and its uncertainty.

Change only the new account abort scenario in frontend/src/lib/api/transportContract.ts
and its evidence. Keep abort and reader acquisition in their existing order, then read until a read rejects
or the stream ends, counting streamed bytes with a 64-KiB cap and retaining the
existing ten-second signal. EOF without a read error is a failure. Overflow is an
independent failure, never an abort success. Catch only rejection from reader.read;
assertion, limit and cleanup failures must never be classified as successful abort.
Release the reader on every path and preserve awaited cancellation/cleanup errors.
Do not change application code, response data, routing, diagnostics, retry counts,
the ten-second signal, test timeouts or the 260-second merge gate.

Run the changed shared contract against the constructor-selected fake. In one isolated
copy remove abort and confirm failure specifically at the body-failure assertion,
with a finite successful body and proper reader cleanup. Perform static checks and
one real-backend execution of exactly the corrected contract. The three reset browser
cases already passed on the unchanged application and need no duplicate targeted run.
If the contract reaches EOF without error, overflows, has ambiguous failure attribution
or encounters another unexpected failure, stop without retry or merge.

The security assessment requires these distinctions and found no scope blocker.
This narrow observation correction accepts only the limited uncertainty of the earlier
first-read outcome. A passing corrected contract is not proof of its historical cause.
After success, complete remaining required checks and the already approved single full
E2E and exact-tree gate. Their allowances are unused; this approval creates no automatic
repeat entitlement. Keep device checks, sequential heavy execution and private evidence
rules unchanged. Record the plan and result in knowledge after owner approval.
