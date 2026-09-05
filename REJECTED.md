# REJECTED.md

## Part 1 — Rejected acceptance criteria

Four of the eight stated acceptance criteria are incorrect and are refused below,
with the arithmetic/reasoning that shows why.

---

### Criterion 2 — REJECTED
> "E7 causes exactly one overdraft fee to be assessed, on Day 2."

**Why this is wrong:** E7 (DEBIT 620.00, value_date=2) is backdated, so when it's
replayed on Day 5 it changes the closing balance of every day from Day 2 forward,
not just Day 2. Recomputing:

```
Day 2 closing (vd<=2): 1200.00 - 950.00 - 620.00 = -370.00   -> negative, fee
Day 3 closing (vd<=3): -370.00 + 400.00           =   30.00  -> positive, no fee
Day 4 closing (vd<=4):   30.00 - 185.00 (E5 settle)= -155.00 -> negative, fee
```

Day 4's closing balance was **positive (+465.00)** before E7 arrived, and only
turns negative once E7's backdated debit is included. That is a second,
independent overdraft-triggering day caused by the same event. E7 therefore
causes **two** overdraft fees (Day 2 and Day 4), not one. This holds regardless
of which interpretation of "day assessed" is used (see AMBIGUITIES.md #1) — the
count of affected days doesn't depend on which day the fee is *booked* against,
only on how many days' balances actually went negative once E7 landed.

---

### Criterion 6 — REJECTED
> "After E9, all balances and fees return to their pre-E7 values."

**Why this is wrong:** E9 reverses E7 by appending an offsetting entry
(+620.00, value_date=2) — it does not delete or mutate E7, per the append-only
rule. This restores the *balance* effect of E7 going forward. But the overdraft
fees that were triggered by E7 (Day 2 and Day 4, per criterion 2's analysis
above) were already posted as their own independent, append-only ledger entries
before E9 arrived. E9 only references E7 — it does not reference or reverse the
fee entries. Those fees remain on the ledger permanently. So after E9, the
*balance trajectory* is restored, but the account is still down two overdraft
fees (50.00 AED total) that a full "return to pre-E7 state" would require to be
gone. Balances and fees do **not** fully return to pre-E7 values.

---

### Criterion 7 — REJECTED
> "The three BHD instalments in E10 must each be BHD 3.334."

**Why this is wrong:** 3.334 x 3 = 10.002, which overshoots the actual total of
10.000 BHD by 0.002. This isn't a rounding nuance — the arithmetic is simply
incorrect as stated. At BHD's 3-decimal precision, 10.000 / 3 rounds to 3.333
per instalment (9.999 total), and the 0.001 shortfall must be assigned to
exactly one instalment via a documented remainder rule (see NUMBERS.md #6):
**3.334 / 3.333 / 3.333**, not 3.334 / 3.334 / 3.334.

---

### Criterion 8 — REJECTED
> "If the rounded daily interest accruals do not sum to the capitalized total,
> the remainder is discarded."

**Why this is wrong:** The ledger is append-only and every entry must be
traceable to real money movement — nothing is ever silently dropped. Discarding
a rounding remainder means money that was mathematically owed (per the
independently-computed "true" capitalized total) simply vanishes with no
ledger record. At scale, this pattern is the textbook shape of a rounding-skim
exploit (fractional amounts quietly absorbed rather than accounted for), which
is exactly why real banking systems reconcile residuals into an explicit,
auditable entry rather than dropping them. The correct behavior is to post the
remainder as its own typed, traceable entry
(`INTEREST_ROUNDING_ADJUSTMENT`, see structs.go / NUMBERS.md #6) — never to
discard it.

---

## Part 2 — Accepted criteria (for contrast, not rejected)

Criteria 1, 3, 4, and 5 were checked against the same event math and hold up:

- **Criterion 1** (Day 2 closing balance at end of Day 5, pre-fee, = -370.00 AED)
  — confirmed by direct calculation above.
- **Criterion 3** (Day 4 settlement of Auth-A must be accepted) — Auth-A has a
  valid preceding hold (E3) and settles for 185.00, within the 200.00 held.
  Correctly accepted.
- **Criterion 4** (settlement referencing unknown auth ID must be rejected,
  funds must not move) — correctly describes how E6 (Auth-Z) must be handled.
- **Criterion 5** (an approved hold reduces available, not ledger, balance) —
  correctly states the general mechanic. Note: in this event stream Auth-B
  (E8) is actually declined, not approved, since the account's ledger balance
  is already negative (-155.00) before the hold is even applied — but the
  criterion is phrased as a conditional ("if approved...") and that
  conditional statement about the mechanic is itself accurate.

---

## Part 3 — Approaches abandoned mid-build

- **`decimal.Decimal` (shopspring/decimal) for money representation** —
  considered for cleaner interest-rate math and readability. Abandoned in
  favor of plain `int64` minor-unit integers to keep the submission
  dependency-free and to force every rounding step to be explicit,
  hand-written code rather than a library default. See NUMBERS.md #1.

- **Dedicated suspense/rounding account for reconciliation residuals** —
  considered as the real-world production pattern for absorbing rounding
  drift across many accounts. Abandoned as out of scope: the spec defines
  exactly two accounts, and for a single account over 6 days the residual is
  fully explainable and auditable as one explicit adjustment entry within the
  affected account. Introducing a third account would solve a problem the
  spec never posed. See NUMBERS.md #6.

- **Multiple CSV rows for E10's three BHD instalments** (e.g. `E10a`, `E10b`,
  `E10c`) — considered to make the split visible directly in the input file.
  Abandoned in favor of a single input row (`E10`) with the split performed
  in code, since this keeps the input file a 1:1 match with the spec's stated
  10 events, and keeps the remainder-distribution logic testable in code
  rather than hardcoded into the input data.

- **Rounding interest to currency precision on every individual day before
  accumulating** — considered as the simplest implementation. Abandoned once
  it became clear this discards sub-minor-unit fractions with no accumulation
  point, making the required reconciliation against the capitalized total
  (criterion 8's premise) impossible to satisfy correctly. Replaced with
  6-decimal internal precision and a single rounding step at posting time.
  See NUMBERS.md #5.