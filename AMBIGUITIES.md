# AMBIGUITIES.md

Every ambiguity found in the spec, and how it was resolved.

---

## 1. Fee assessment: closed-day (EOD batch) vs. retroactive recomputation

**Ambiguity:** The spec says the overdraft fee is booked "with value_date equal to the
day assessed." E7 (a DEBIT with `value_date = Day 2`) is not *replayed* until Day 5.
Once E7 posts, is Day 2's fee decision:

- (a) **final and closed** — Day 2's closing balance was checked once, when Day 2 was
  reached in the replay, and that decision stands regardless of what backdated entries
  arrive later, or
- (b) **reopened retroactively** — any time a backdated entry lands, every day from its
  `value_date` forward gets its fee decision recomputed against the new information?

**Resolution:** (a). This mirrors how real core banking systems actually operate:
transactions post and settle during the business day, and at a defined cutoff an
end-of-day batch process computes that day's closing balance, assesses any overdraft
fee, and **closes the day's books**. Once a day is closed, it is not reopened —
this is the standard "period closing" principle that makes historical statements and
fee assessments final and auditable, rather than provisional and subject to silent
revision weeks later.

Practically, this means a day's fee/accrual check runs **exactly once**, at the point
that day is reached in the replay loop, using whatever entries already exist in the
ledger at that moment. A later-arriving backdated entry (like E7) is evaluated against
**the day it is actually replayed on**, not retroactively inserted into an already-closed
day. See REJECTED.md — criterion 2 is evaluated against this model, and the day the
resulting fee lands on (Day 5, not Day 2) follows directly from it.

**Why not (b):** Full retroactive recomputation was considered and rejected as
operationally incoherent, not just more work. It would require re-deriving decisions
that were correct given the information available when they were made — e.g. Auth-A's
approval on Day 2 used Day 2's real balance at the time; retroactively invalidating that
decision because a later, unrelated backdated entry changes the historical balance has
no natural stopping point, and no real bank works this way. It would also mean a
customer's already-issued statement for a closed day could silently change later, which
contradicts the append-only, audit-stable spirit of the ledger.

---

## 2. Recomputation scope — none, beyond the day being processed

**Ambiguity:** Given the resolution above, does the *current* day's fee/accrual check
need to look back at any range of prior days, or purely at itself?

**Resolution:** Purely at itself. Each day's fee and interest-accrual check runs once,
using `ClosingBalance(day)` as of the moment that day is reached in the replay loop —
which naturally includes any earlier-value-date entries that have arrived by then (since
`ClosingBalance` sums all ledger entries with `value_date <= day`, regardless of when
they were replayed), but never re-examines a day that has already been checked and
passed. No backward sweep, no re-checking of days 1 through N-1 when day N is processed.
This keeps the engine's cost linear in the number of days/events, and matches the
closed-day model in #1.

One consequence worth noting: because fee entries are themselves ledger entries with
their own `value_date`, a fee posted on one day can push a *later* day's closing balance
further negative (the Day 5 fee in this scenario counts toward Day 6's balance sum, for
example). This cascading is a natural, intended consequence of "closing balance = sum of
all entries with value_date <= day," not a special case requiring extra logic.

---

## 3. Meaning of "the capitalized total" for interest reconciliation

**Ambiguity:** The spec requires "the rounded daily accruals must sum exactly to the
capitalized total." This is only a meaningful constraint under one of two readings:

- (a) the capitalized total is *defined as* the sum of the six rounded daily accruals
  (trivially true, no reconciliation needed), or
- (b) the capitalized total is independently computed (e.g. sum of the six *raw,
  unrounded* daily interest values, rounded once at the end), and the daily roundings
  must be reconciled against it.

**Resolution:** (b), since it's the only reading that makes the constraint
non-trivial and worth stating. This requires the remainder-distribution fix described
in NUMBERS.md #6.

---

## 4. Where the rounding remainder goes

**Ambiguity:** Spec doesn't say what happens to the fractional residual when rounded
daily accruals don't sum to the capitalized total.

**Resolution:** Never discarded (see REJECTED.md, criterion 8). Posted as an explicit,
separately-typed ledger entry (`INTEREST_ROUNDING_ADJUSTMENT`) on Day 6, referencing the
six daily accrual entries it reconciles. Chosen over a separate suspense account as
out-of-scope for a 2-account, 6-day exercise (see NUMBERS.md #6).

---

## 5. Input file format

**Ambiguity:** Spec never specifies what format the event stream is provided in.

**Resolution:** CSV (`input.csv`), one row per event, with a superset of columns
covering all event types (`auth_id` for AUTHORIZATION/SETTLEMENT, `ref_event` for
REVERSAL), blank where not applicable. Amounts are parsed as strings and converted to
integer minor units directly — never passed through a float intermediate.

---

## 6. E10's "three equal instalments"

**Ambiguity:** BHD 10.000 ÷ 3 does not divide evenly at 3-decimal precision
(3.333 × 3 = 9.999). "Equal" instalments are therefore not perfectly equal once
currency precision is enforced.

**Resolution:** Split into 3.334 / 3.333 / 3.333 using the remainder-distribution rule
(NUMBERS.md #6) — first instalment absorbs the +0.001 residual. This directly
contradicts acceptance criterion 7 (see REJECTED.md).

---

## 7. Output format for the per-day report

**Ambiguity:** Spec requires printing "closing ledger balance, fee assessments,
authorization states, and errors" per day, but does not specify structure (per-account
vs. combined, plain text vs. structured).

**Resolution:** Plain-text console output, grouped per account per day, since the two
accounts (ACC-001 AED, ACC-002 BHD) never interact and combining them would need an
arbitrary shared unit. Each day's block includes: closing ledger balance, available
balance, active holds, fees assessed (with reason), settlements, interest accrual (where
applicable), and any rejected/errored events for that day.

---

## 8. Auth-Z's rejected settlement (E6) — does it "leave" a trace?

**Ambiguity:** Spec requires rejected settlements not to move funds, but doesn't say
whether the rejection itself should appear in the ledger.

**Resolution:** The rejection is **not** posted as a ledger entry (no funds moved, no
balance-affecting event occurred), but **is** recorded in the day's printed error output
and in an in-memory error log, so it's visible in the report without violating the
append-only ledger's meaning (the ledger records money movements; the error log records
replay-time events, including rejections).

Fees triggered by a later event are not automatically reversed when that event is later
reversed — they remain on the ledger as historical fact, since automatic reversal would
require re-deriving causality and risks silently erasing legitimate financial history. A
real system would require a separate, staff-approved waiver/credit rather than implicit
reversal; this is out of scope for the exercise.

---

## 9. Auth-B's unresolved authorization at window close

**Ambiguity:** Spec states Auth-B is never settled within the window, but doesn't say
whether it should still show as "active" in the Day 6 report or be treated as expired.

**Resolution:** Not applicable to the actual event stream as replayed — Auth-B (E8) is
declined at authorization time, since the account's ledger balance is already negative
(-155.00, after E7 posts earlier the same day) before the hold is even applied.
Retained here as a documented rule for the general case: had Auth-B been approved, it
would be treated as still **active** at Day 6 close, since nothing in the spec defines
an authorization expiry rule, and shown as an outstanding hold reducing available
balance with a note that it remains unresolved (not settled, not released, not expired).

---

## 10. Settling the same authorization more than once

**Ambiguity:** The spec doesn't say what should happen if a settlement event
references an authorization ID that has already been settled (or otherwise
released). A hold existing in the map isn't the same as a hold still being
usable — but the spec's settlement rule only talks about IDs that are
"not present in the ledger," not IDs that are present but already resolved.

**Resolution:** A settlement is rejected, with no funds moved, unless the
referenced hold both exists **and** is still active (`hold.Active == true`).
Once a hold is settled, it is marked inactive and can never be settled again.
This is treated the same as an unknown-authorization-ID rejection (E6/Auth-Z),
since allowing a second settlement against an already-resolved hold would let
funds move against a reservation that no longer represents a live commitment.

---

## 11. Settlement amount exceeding the held amount

**Ambiguity:** The spec never states a rule for what happens when a
settlement's amount is greater than the amount originally held by its
authorization (e.g. a hold for 1,000 settling for 10,000). Real-world card
systems sometimes allow a bounded overage (e.g. tip adjustments), but the
spec gives no tolerance threshold to base a partial-acceptance rule on.

**Resolution:** Resolved conservatively — a settlement must satisfy
`0 < settlement amount <= held amount`, or it is rejected outright with no
funds moved, using the same rejection pathway as an unknown authorization ID.
Allowing an over-settlement would let the original authorization check be
silently bypassed for the excess amount, defeating the purpose of requiring
an authorization before a settlement in the first place.

---

## 12. E10's "instalments" — same-day split vs. genuine time-staggered posting

**Ambiguity:** "Instalment," in normal banking usage, means a payment staggered
over time (an EMI plan, a scheduled bill), not several same-day postings that
happen to sum to a total. But E10 gives a single `value_date` (Day 5) for the
whole event, with no interval or future dates specified — so the literal text
supports a same-day three-way split, while the *word choice* implies something
more. There's also no way to link the resulting entries back to a single
logical transaction (E10) unless something explicitly does so.

**Considered and rejected — same-day split with no traceability:** simplest
literal reading, but "instalments" would then be a misleading label for a plain
partition, and the three resulting entries would be indistinguishable from
three unrelated credits that happened to land on the same account.

**Considered and rejected — arbitrary staggered interval:** e.g. spacing parts
a week apart, or across the full window. Rejected: the spec gives no interval
to derive a schedule from, so any spacing beyond the minimum would be inventing
data the spec never provided, not resolving an ambiguity.

**Resolution:** Treat "instalments" as genuinely implying time-staggering, using
the smallest, least-invented interval possible — one calendar day per part
(`InstalmentIncrementDays = 1`), starting from E10's own `value_date`: **today
(Day 5), tomorrow (Day 6), day after tomorrow (Day 7)** (see NUMBERS.md #7). Day 7 falls outside the 6-day report window — that part is
still posted to the ledger (the money is real and the account continues to
exist past the reporting period), it simply never appears in any of this
exercise's printed day reports, the same way a real bank statement covering
Jan 1–6 wouldn't show a transaction dated Jan 7, without that meaning the
transaction didn't happen.

Each resulting entry is tagged with `InstalmentGroup` (= the originating event
ID, "E10"), `InstalmentSeq` (1-based position), and `InstalmentTotal`, so all
three can be reconstructed as one logical 10.000 BHD transaction by any
downstream auditor — the same traceability pattern already used for the
interest-rounding adjustment entry (NUMBERS.md #6) and reversal entries.
Amounts are split via the standard remainder-distribution rule (NUMBERS.md #6):
3.334 / 3.333 / 3.333 BHD, first part absorbing the residual.