# Proposal 0001: Lesson and payment ledger

- Status: deferred
- Reviewed: pending owner review
- Author: project owner
- Date: 2026-10-20
- Trigger: the MVP is in use by tutors, and at least one asks to track what their students owe; or the payment-ledger slice is scheduled on the roadmap
- Related: [ADR-0005](../decisions/0005-tutor-centric-domain.md), [schedule specification](../../knowledge/specs/schedule.md)

## Motivation

Private tutors are paid per lesson or in packages, usually outside any platform (bank transfer, cash). They track who has paid for how many lessons in notebooks or spreadsheets. OVC already knows which lessons took place, so it could keep that record for them. No money moves through OVC.

## Proposal

- A price per lesson on each relationship (and optionally per group), in a currency chosen by the tutor.
- Packages: the tutor records "Anna paid for 8 lessons" or "paid 12,000"; the balance increases.
- Each ended lesson charges its participants by its price; a cancelled lesson charges nothing; the tutor can mark a lesson as "charged anyway" (late cancellation) or "free".
- Balances and debts on the student card and a monthly report for the tutor; the student sees their own balance with that tutor.
- Attendance comes from the conferencing provider's join events, so an ended lesson shows who actually attended.

What does not change: no online payments, no invoices, no tax documents.

## Alternatives

- **Online payments through a payment provider:** a regulated flow with acquiring, refunds and receipts; out of scope until tutors ask for it.
- **Export to a spreadsheet only:** cheaper, but the balance is the useful part.

## Plan

1. Attendance from provider join events, stored on lesson participants.
2. Prices and the ledger with charges and payments, as entries; balances are derived.
3. Student card, student view and monthly report.

Each slice is tested with the fake meeting provider and a fake clock.

## Risks and open questions

- Currency and rounding rules; multi-currency tutors.
- Whether a missed lesson charges by default.
- Ledger entries must be append-only, with corrections as new entries.

## Decision

Pending.
