package ledger

// ---------------------------------------------------------------------------
// All numeric constants used across the ledger engine.
// Every constant here should have a corresponding explanation in NUMBERS.md —
// treat this file and NUMBERS.md as a matched pair.
// ---------------------------------------------------------------------------

// --- Currency precision (NUMBERS.md #2) ---
const (
	PrecisionAED = 2
	PrecisionBHD = 3
)

// --- Overdraft fee (NUMBERS.md #3) ---
const (
	OverdraftFeeAED int64 = 2500 // 25.00 AED, in minor units (cents)
)

// --- Daily interest rate (NUMBERS.md #4) ---
const (
	DailyInterestRateNumerator   int64 = 4 // 0.04% == 4 / 10000
	DailyInterestRateDenominator int64 = 10000
)

// --- Internal calculation precision (NUMBERS.md #5) ---
const (
	// MicroUnitsPerMinorUnit scales one minor unit (cent/fils) up to give
	// 3 extra internal decimal digits before the single rounding step.
	MicroUnitsPerMinorUnit int64 = 1000
)

// --- Instalment schedule (NUMBERS.md #8, AMBIGUITIES.md #12) ---

// InstalmentIncrementDays is the number of days between each successive
// instalment part, starting from the originating event's value_date.
// e.g. with value_date=5 and 3 instalments: parts land on Day 5, Day 6, Day 7
// (5, 5+1*1, 5+2*1). Unit: days.
const InstalmentIncrementDays = 1
