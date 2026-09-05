# Ledger

An in-memory, append-only account ledger core built for a fixed six-day
event-replay exercise. No web layer, no database, no persistence — a single
Go program replays a CSV event stream against two in-memory accounts and
prints what happened, one day at a time.

## Accounts

| Account | Currency | Precision  | Opening balance |
| ------- | -------- | ---------- | ---------------- |
| ACC-001 | AED      | 2 (cents)  | 0.00              |
| ACC-002 | BHD      | 3 (fils)   | 0.000             |

## How to run the suite

There's no separate test binary to invoke — the "suite" here is the replay
script itself, run directly against the fixed event stream:

```bash
go run main.go
```

No arguments, no flags, no setup beyond having Go installed. `main.go`:

1. creates the two accounts (ACC-001 AED, ACC-002 BHD) at their spec'd
   opening balances,
2. parses `input.csv` — the ten-event stream E1–E10, exactly as given in
   the spec — via `ParseCSVRecords`,
3. replays those events through the engine in order,
4. prints a report for every account, for every day, Day 1 through Day 6,
5. prints a final per-account summary covering the whole six-day window.

Everything goes to stdout — pipe it to a file if you want to keep a copy
to diff against later:

```bash
go run main.go > output.txt
```

`input.csv`'s header row must have one column per field, including the
trailing instalment-count column (`id,day,type,account,currency,amount,
value_date,auth_id,ref_event,instalment_count`) — Go's `csv.Reader` locks
the expected field count to whatever the first row it reads has, so a
header short one column will panic on the very first data row with
`wrong number of fields`.

## How to read the output

For each account, each day's block prints, in order:

- **closing ledger balance** — the sum of every ledger entry with
  `value_date <= that day`, always recomputed, never cached
- **available balance** — closing balance minus any currently active holds
- **authorization / hold states** — every hold that existed by that day,
  and whether it's active, settled, or released
- **fees assessed** that day, with a human-readable reason
- **settlements** that day
- **errors** — any event rejected that day (unknown account, unknown or
  already-resolved authorization, over-limit settlement, etc.), with why

followed, once at the very end, by a per-account **summary**: final
closing/available balance, total fees assessed, the capitalized interest
for the six-day window, and — if the daily accruals didn't sum exactly to
the capitalized total — the rounding-reconciliation entry that makes up
the difference (see NUMBERS.md #6).

Amounts print in the account's minor units (e.g. `25000` = AED 250.00,
`10000` = BHD 10.000) unless explicitly formatted for display.

A day's report reflects the ledger **as of that point in the replay** —
once a day is processed, its fee/interest decisions are never silently
revised later, even if a later event arrives with an earlier `value_date`.
See AMBIGUITIES.md #1–#2 for why, and REJECTED.md (criterion 2) for how
this plays out concretely with E7.

## Repo layout

- `main.go` — entry point: builds the two accounts, parses `input.csv`,
  replays it, prints the reports
- `ledger/` — the ledger core: event/account/engine types, replay logic,
  reporting
- `input.csv` — the fixed ten-event stream from the spec
- `NUMBERS.md` — every constant chosen, and why that value and not another
- `AMBIGUITIES.md` — every ambiguity found in the spec and how it was
  resolved
- `REJECTED.md` — which acceptance criteria were refused and why, plus
  approaches abandoned mid-build
- `WORKLOG.md` — a real, timestamped log of the build

## Design notes

The three decisions most worth reading before the code:

1. **Money is always `int64` minor units, never a float**, even
   internally. See NUMBERS.md #1.
2. **A day is closed once reached in the replay loop.** A backdated entry
   (E7) is evaluated against the day it's actually replayed on, not
   retroactively inserted into a day that's already closed. See
   AMBIGUITIES.md #1–#2.
3. **Rounding remainders are never discarded.** Any residual — from an
   equal split (E10's BHD instalments) or from independently-rounded daily
   interest accruals vs. the capitalized total — is posted as its own
   explicit, traceable ledger entry, never silently absorbed. See
   NUMBERS.md #5–#6 and REJECTED.md (criterion 8).

## Known scope limits

- E10's third BHD instalment is dated Day 7 (parts land one calendar day
  apart, starting from the event's own `value_date` — see NUMBERS.md #7 /
  AMBIGUITIES.md #12). It's posted to the ledger — the money is real — but
  the report window is Day 1–6 only, so it never appears in a printed day
  report or in the six-day summary. This is a deliberate scope decision,
  not an oversight.
