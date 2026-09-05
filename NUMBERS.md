# NUMBERS.md

Every constant, encoding choice, and rounding rule used in this system, and why.

---

## 1. Money representation: integer minor units

**Choice:** All amounts are stored as `int64` in the currency's smallest unit
(AED → hundredths, "fils-of-cent" scale; BHD → thousandths).

- AED 1,200.00 → stored as `120000`
- BHD 10.000 → stored as `10000`

**Why:** Floating point cannot represent decimal fractions exactly (e.g. `0.1 + 0.2 != 0.3`
in IEEE 754). A ledger cannot tolerate silent floating-point drift. Integers make every
rounding step an explicit, visible operation in code rather than an implicit float
rounding mode.

**Why not `decimal.Decimal` (e.g. shopspring/decimal):** Considered and rejected only to
keep this a dependency-free, single-binary submission. `decimal.Decimal` is arguably more
idiomatic for production Go and would have been an equally defensible choice — the
trade-off is dependency management vs. having every rounding step spelled out by hand.

---

## 2. Currency precision

| Currency | Precision | Minor unit scale |
|---|---|---|
| AED | 2 decimals | × 100 |
| BHD | 3 decimals | × 1000 |

Taken directly from the spec. Not derived — BHD's 3-decimal precision is a real-world
fact (Bahraini dinar is subdivided into 1000 fils), included here for completeness.

---

## 3. Overdraft fee: AED 25.00 (2500 minor units)

- Fixed value given by the spec, not derived.
- Assessed **once per day per account**, only when that day's closing ledger balance
  (all entries with `value_date ≤ that day`, recomputed, not cached) is negative.
- Booked with `value_date` = the day whose balance was found negative — **not** the day
  the negativity was detected during replay. See AMBIGUITIES.md #1 for why this
  matters given backdated entries (E7).

---

## 4. Daily interest: 0.04% per day

- Fixed rate given by the spec: `0.0004` as a multiplier.
- Applied only to **positive** closing ledger balances (spec: "positive balances only").
- Computed in minor units: `accrual = balance_minor_units * 4 / 10000`, using integer
  division truncation first, then a single documented rounding pass (see #5).

---

## 5. Rounding rule: round-half-to-even (banker's rounding)

**Choice:** When a computed value (interest accrual, or an equal split like the BHD
instalments) lands between two representable minor units, round to the nearest one;
on an exact tie (`.5` of a minor unit), round to the even minor unit.

**Why not round-half-up:** Round-half-up has a small systematic upward bias when
applied repeatedly across many roundings (ties always break the same direction).
At banking scale (millions of accounts, thousands of days), that bias becomes a
non-trivial, non-random transfer of value toward or away from the institution.
Round-half-to-even is the industry-standard default specifically to avoid this
directional bias (used by IEEE 754, Python's `round()`, and most banking systems).

**Applies to:** daily interest accrual rounding, and the BHD three-way instalment
split in E10.

**Internal calculation precision:** Interest is calculated internally at 6-decimal
precision (scaled integer arithmetic — no floats), and rounded to currency precision
exactly once, at the point of ledger posting. The "true" capitalized total used for
the reconciliation check (see #6 below) is computed by summing these high-precision
values *before* that single rounding step — not by summing already-rounded daily
entries. Rounding too early, on every individual day, would silently discard
sub-minor-unit fractions with no accumulation point, producing drift with no
traceable cause.

---

## 6. Remainder-distribution rule (rounding reconciliation)

**Rule:** whenever rounding several parts of a whole produces a sum that does not
exactly equal the whole (an equal split, or independently-rounded daily accruals vs.
a capitalized total), the residual is:

1. Computed as `remainder = true_total - sum(rounded_parts)` (can be positive or
   negative — same formula handles both directions, no special-casing needed).
2. Applied to a **single, designated** part — chosen as:
    - **BHD instalments (E10):** the *first* instalment absorbs the remainder.
    - **Daily interest accrual (capitalization):** *Day 6* absorbs the remainder,
      since Day 6 is already the day capitalization happens.
3. Recorded as an explicit, separately-typed ledger entry
   (`INTEREST_ROUNDING_ADJUSTMENT`), never silently folded into another entry's
   amount and never discarded. See AMBIGUITIES.md #4 and REJECTED.md (criterion 8).

**Why not a separate suspense/rounding account:** considered, and is the real-world
production pattern for reconciling rounding drift in aggregate across many accounts.
Rejected here as out of scope: the spec defines exactly two accounts, and for a
single account over 6 days the residual is fully explainable and traceable as one
explicit in-account adjustment entry — introducing a third account would be solving
a problem the spec never posed.

---

## 7. Instalment schedule (E10)

**Choice:** A split/instalment credit posts its N equal parts on consecutive days
starting from the originating event's `value_date`, spaced `InstalmentIncrementDays`
apart (unit: **days**). With the current value of `1`, N=3 parts land on: today,
tomorrow, day after tomorrow — i.e. `value_date + i * InstalmentIncrementDays` for
i = 0, 1, 2.

**Why:** "Instalment," in normal banking usage, means payments staggered over time
— not several same-day postings that happen to sum to a total. E10 as specified
gives a single `value_date` (Day 5) for the whole event, but the word "instalments"
still implies a time-staggered posting. Rather than silently ignore that implication
(same-day split) or invent an arbitrary interval, this uses the smallest, most
literal staggering possible: one calendar day apart, per part — mirroring how a
real forward-dated or instalment-style posting behaves (see AMBIGUITIES.md #12 for
the full reasoning, including why parts falling outside the 6-day report window are
still posted rather than compressed or dropped).

**Amounts:** split via the same remainder-distribution rule as any other equal
split (NUMBERS.md #6): `SplitEqual(1000000, 3)` → micro-units `[334, 333, 333]` in
BHD's minor-unit scale → **3.334 / 3.333 / 3.333 BHD**, dated Day 5 / Day 6 / Day 7
respectively.

---

## 8. Event/entry ID scheme

- Input events keep their given IDs (`E1`–`E10`) verbatim, unmodified.
- System-generated entries (fees, interest accruals, rounding adjustments) use a
  `SYS-<TYPE>-<ACCOUNT>-<DAY>` naming scheme, e.g. `SYS-FEE-ACC-001-D2`,
  `SYS-INTEREST-ADJ-ACC-001`. This keeps input-derived and system-derived entries
  visually distinguishable in the ledger dump, in addition to the `Source` field
  (see struct code).