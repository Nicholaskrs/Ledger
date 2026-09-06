package ledger

// ---------------------------------------------------------------------------
// Money is always stored as int64 in the currency's minor unit
// (AED -> hundredths, BHD -> thousandths). See NUMBERS.md #1.
// ---------------------------------------------------------------------------

// EntrySource distinguishes ledger entries that came from the replayed
// input event stream from entries the engine generated as a side effect
// of applying the rules (fees, interest, rounding adjustments).
type EntrySource string

const (
	SourceInput  EntrySource = "input"  // e.g. E1..E10, taken verbatim from input.csv
	SourceSystem EntrySource = "system" // e.g. fees, interest, rounding adjustments
)

// EntryType enumerates every kind of ledger entry, both input-driven and
// system-generated.
type EntryType string

const (
	TypeCredit                 EntryType = "CREDIT"
	TypeDebit                  EntryType = "DEBIT"
	TypeAuthorization          EntryType = "AUTHORIZATION"
	TypeSettlement             EntryType = "SETTLEMENT"
	TypeReversal               EntryType = "REVERSAL"
	TypeOverdraftFee           EntryType = "OVERDRAFT_FEE"
	TypeInterestCapitalization EntryType = "INTEREST_CAPITALIZATION"
	TypeInterestRoundingAdjust EntryType = "INTEREST_ROUNDING_ADJUSTMENT"
)

// Event is a single row parsed from input.csv — a replayed external event.
// Not all fields apply to every type; unused fields are left zero-valued.
type Event struct {
	ID              string    // "E1".."E10", unique
	Day             int       // day the event is replayed/entered
	Type            EntryType // CREDIT, DEBIT, AUTHORIZATION, SETTLEMENT, REVERSAL
	Account         string    // "ACC-001", "ACC-002"
	Currency        string    // "AED", "BHD" (may be blank for REVERSAL)
	Amount          int64     // minor units; zero/unused for REVERSAL
	ValueDate       int       // day the event is effective for (may differ from Day)
	AuthID          string    // used by AUTHORIZATION (new hold) and SETTLEMENT (which hold)
	RefEvent        string    // used by REVERSAL — the event ID being reversed
	InstalmentCount int       // used if user want to pay by installment.
}

// LedgerEntry is an immutable, append-only record in an account's ledger.
// Both input-derived and system-generated entries share this shape;
// Source distinguishes which.
type LedgerEntry struct {
	EventID   string // own ID, e.g. "E1" or "SYS-FEE-ACC-001-D2"
	Type      EntryType
	Source    EntrySource
	Account   string
	Amount    int64 // minor units, signed: +credit-like, -debit-like
	ValueDate int
	RelatesTo []string // other EventIDs this entry references/reconciles (fees, reversals, rounding adjustments)
	Reason    string   // human-readable justification, for traceability

	// Instalment tracking — populated only for entries that are part of a
	// split/instalment posting (e.g. E10). Zero-valued for ordinary entries.
	// See NUMBERS.md #8 and AMBIGUITIES.md #12.
	InstalmentGroup string // shared batch ID: the originating event's ID (e.g. "E10")
	InstalmentSeq   int    // 1-based position within the group (1, 2, 3...)
	InstalmentTotal int    // total instalments in the group
}

// Hold represents an active or resolved authorization against an account.
type Hold struct {
	AuthID    string
	Amount    int64 // minor units, the held amount
	CreatedOn int   // day the hold was created (value_date of the AUTHORIZATION)
	Active    bool  // true until settled or released
}

// AuthState is what gets reported per authorization in the per-day output.
type AuthState struct {
	Amount   int64
	IsActive bool // "ACTIVE", "SETTLED", "RELEASED", "REJECTED_NO_MATCH"
}

// ReplayError records a rejected event and why, for the error log /
// per-day report. Rejected events never produce a LedgerEntry.
type ReplayError struct {
	EventID string
	Day     int
	Reason  string
}

// Account holds an account's full append-only ledger and current holds.
// Precision is the number of decimal places for its currency (2 for AED,
// 3 for BHD, etc...) — see NUMBERS.md #2.
type Account struct {
	ID                  string
	Currency            string
	Precision           int
	Ledger              []LedgerEntry         // append-only, never mutated or truncated
	Holds               map[string]*Hold      // keyed by AuthID
	Errors              map[int][]ReplayError // rejected events referencing this account
	DailyRawMicros      map[int]int64         // Total Daily in Raw Micros
	FeesAssessed        map[int][]LedgerEntry
	Balance             int64         // running ledger balance — kept in sync on every insert
	ActiveHoldsTotal    int64         // running sum of active holds — kept in sync on create/resolve
	PendingBalanceByDay map[int]int64 // value_date -> amount not yet folded into Balance
}

// NewAccount creates an account with an explicit opening balance entry,
// so the opening balance itself is a traceable, append-only ledger entry —
// not an implicit zero-value struct field.
// NewAccount creates an account with a given opening balance.
func NewAccount(id, currency string, precision int, openingBalanceMinorUnits int64) *Account {
	return &Account{
		ID:                  id,
		Currency:            currency,
		Precision:           precision,
		Ledger:              make([]LedgerEntry, 0),
		Holds:               make(map[string]*Hold),
		Errors:              make(map[int][]ReplayError),
		DailyRawMicros:      make(map[int]int64),
		FeesAssessed:        make(map[int][]LedgerEntry),
		Balance:             openingBalanceMinorUnits,
		PendingBalanceByDay: make(map[int]int64),
	}
}

// AvailableBalance returns ledger balance minus all currently active holds.
func (a *Account) AvailableBalance() int64 {
	total := a.Balance
	for _, h := range a.Holds {
		if h.Active {
			total -= h.Amount
		}
	}
	return total
}

func (a *Account) HasLedgerEntry(id string) bool {
	for _, e := range a.Ledger {
		if e.EventID == id {
			return true
		}
	}
	return false
}

func (a *Account) GetAuthState() []AuthState {
	authState := make([]AuthState, 0)
	for _, e := range a.Holds {
		authState = append(authState, AuthState{
			Amount:   e.Amount,
			IsActive: e.Active,
		})
	}
	return authState
}

// DayReport is the structure printed per account per day.
type DayReport struct {
	Day              int
	Account          string
	ClosingBalance   int64
	Currency         string
	AvailableBalance int64
	Holds            []AuthState
	FeesAssessed     []LedgerEntry
	Errors           []ReplayError
}

type SummaryReport struct {
	Account               string
	Currency              string
	FinalClosingBalance   int64
	FinalAvailableBalance int64

	InterestCapitalized    int64 // amount from SYS-INTEREST-CAP-<acct>
	InterestRoundingAdjust int64 // amount from SYS-INTEREST-ADJ-<acct>, 0 if no remainder

	HoldsStillActive int
}
