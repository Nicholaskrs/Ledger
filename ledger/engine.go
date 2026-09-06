package ledger

import (
	"fmt"
	"sort"
)

type Engine struct {
	Accounts     map[string]*Account
	holdsSeenIDs map[string]bool // to detect Auth-Z-style unknown auth refs
}

func NewEngine(accounts map[string]*Account) *Engine {
	return &Engine{
		Accounts:     accounts,
		holdsSeenIDs: make(map[string]bool),
	}
}

func (e *Engine) Replay(events []Event) {
	// Map[Day][]Events.
	eventEachDay := map[int][]Event{}
	for _, ev := range events {
		eventEachDay[ev.Day] = append(eventEachDay[ev.Day], ev)
	}

	// Extract keys and sort ascending — map range order is randomized in Go,
	// so this step is mandatory, not stylistic.
	days := make([]int, 0, len(eventEachDay))
	for d := range eventEachDay {
		days = append(days, d)
	}
	sort.Ints(days)

	for _, day := range days {
		dayEvents := eventEachDay[day]
		for _, ev := range dayEvents {
			e.applyEvent(ev)
		}
		e.recomputeFees(day)
		e.recomputeAccruals(day)
		e.drainPending(day)
		dailyReports := e.generateDayReport(day)
		err := PrintDayReports(dailyReports)
		if err != nil {
			panic(err)
		}
	}
	lastDay := days[len(days)-1]
	e.capitalizeInterest(lastDay)
	summaryReport := e.generateSummaryReport()
	err := PrintSummaryReports(summaryReport)
	if err != nil {
		panic(err)
	}
}

func (e *Engine) applyEvent(ev Event) {
	acc, ok := e.Accounts[ev.Account]
	if !ok {
		panic("Account not found: " + ev.Account)

		return
	}

	switch ev.Type {
	case TypeCredit:
		e.processCredit(ev, acc)
	case TypeDebit:
		e.processDebit(ev, acc)
	case TypeAuthorization:
		e.processAuthorization(acc, ev)
	case TypeSettlement:
		e.processSettlement(acc, ev)
	case TypeReversal:
		e.processReversal(acc, ev)
	}

}

func (e *Engine) processAuthorization(acc *Account, ev Event) {
	availableAmount := acc.Balance - acc.ActiveHoldsTotal

	if availableAmount >= ev.Amount {
		acc.Holds[ev.AuthID] = &Hold{
			AuthID: ev.AuthID, Amount: ev.Amount, CreatedOn: ev.Day, Active: true,
		}
		acc.ActiveHoldsTotal += ev.Amount
		return
	}
	acc.Errors[ev.Day] = append(acc.Errors[ev.Day], ReplayError{
		EventID: ev.ID, Day: ev.Day,
		Reason: fmt.Sprintf("Authorization %s declined: Prior available balance: %d, Amount: %d", ev.AuthID, availableAmount, ev.Amount),
	})
}

func (e *Engine) processSettlement(acc *Account, ev Event) {
	hold, exists := acc.Holds[ev.AuthID]
	if !exists {
		acc.Errors[ev.Day] = append(acc.Errors[ev.Day], ReplayError{
			EventID: ev.ID, Day: ev.Day,
			Reason: fmt.Sprintf("Settlement references unknown authorization %s — rejected, no funds moved", ev.AuthID),
		})
		return
	}
	if !hold.Active {
		acc.Errors[ev.Day] = append(acc.Errors[ev.Day], ReplayError{
			EventID: ev.ID, Day: ev.Day,
			Reason: fmt.Sprintf("Settlement references already-settled/released authorization %s — rejected, no funds moved", ev.AuthID),
		})
		return
	}

	if ev.Amount > hold.Amount {
		acc.Errors[ev.Day] = append(acc.Errors[ev.Day], ReplayError{
			EventID: ev.ID, Day: ev.Day,
			Reason: fmt.Sprintf(
				"Settlement %d exceeds held amount %d for authorization %s — rejected, no funds moved",
				ev.Amount, hold.Amount, ev.AuthID,
			),
		})
		return
	}

	acc.Ledger = append(acc.Ledger, LedgerEntry{
		EventID: ev.ID, Type: TypeSettlement, Source: SourceInput,
		Account: acc.ID, Amount: -ev.Amount, ValueDate: ev.ValueDate,
		RelatesTo: []string{ev.AuthID},
	})
	acc.Balance -= ev.Amount
	acc.ActiveHoldsTotal -= hold.Amount
	hold.Active = false // release the hold regardless of settled amount vs held amount
}

func (e *Engine) processReversal(acc *Account, ev Event) {
	var original *LedgerEntry
	for i := range acc.Ledger {
		if acc.Ledger[i].EventID == ev.RefEvent {
			original = &acc.Ledger[i]
			break
		}
	}
	if original == nil {
		acc.Errors[ev.Day] = append(acc.Errors[ev.Day], ReplayError{
			EventID: ev.ID, Day: ev.Day,
			Reason: fmt.Sprintf("Reversal references unknown event %s", ev.RefEvent),
		})
		return
	}

	acc.Ledger = append(acc.Ledger, LedgerEntry{
		EventID: ev.ID, Type: TypeReversal, Source: SourceInput,
		Account: acc.ID, Amount: -original.Amount, ValueDate: ev.ValueDate,
		RelatesTo: []string{ev.RefEvent},
	})
	acc.Balance -= original.Amount
}

func (e *Engine) processInstalmentCredit(ev Event, acc *Account) {
	parts := SplitEqual(ev.Amount, ev.InstalmentCount)

	for i, amt := range parts {
		vd := ev.ValueDate + i*InstalmentIncrementDays // i=0 -> today, i=1 -> +1 day, etc.

		acc.Ledger = append(acc.Ledger, LedgerEntry{
			EventID:   fmt.Sprintf("%s-%d", ev.ID, i+1),
			Type:      TypeCredit,
			Source:    SourceInput,
			Account:   acc.ID,
			Amount:    amt,
			ValueDate: vd,
			RelatesTo: []string{ev.ID},
			Reason: fmt.Sprintf(
				"Instalment %d of %d for %s, +%d day(s) from origin",
				i+1, len(parts), ev.ID, i*InstalmentIncrementDays,
			),
			InstalmentGroup: ev.ID,
			InstalmentSeq:   i + 1,
			InstalmentTotal: len(parts),
		})
		if vd <= ev.Day {
			acc.Balance += amt // already effective as of today's replay day
			continue
		}
		acc.PendingBalanceByDay[vd] += amt

	}
}

func (e *Engine) recomputeFees(day int) {
	for _, acc := range e.Accounts {
		if acc.Balance < 0 {
			feeID := fmt.Sprintf("SYS-FEE-%s-D%d", acc.ID, day)
			if acc.HasLedgerEntry(feeID) {
				continue // already assessed for this day — append-only, don't duplicate
			}
			ledgerEntry := LedgerEntry{
				EventID: feeID,
				Type:    TypeOverdraftFee, Source: SourceSystem,
				Account: acc.ID, Amount: -OverdraftFeeAED, ValueDate: day,
				Reason: fmt.Sprintf("Closing balance negative on day %d", day),
			}
			acc.Ledger = append(acc.Ledger, ledgerEntry)
			acc.FeesAssessed[day] = append(acc.FeesAssessed[day], ledgerEntry)
			acc.Balance -= OverdraftFeeAED
		}
	}
}

func (e *Engine) recomputeAccruals(day int) {
	for _, acc := range e.Accounts {
		acc.DailyRawMicros[day] = e.rawDailyInterestMicros(acc.Balance)
	}
}

func (e *Engine) processCredit(ev Event, acc *Account) {
	if ev.InstalmentCount > 1 {
		e.processInstalmentCredit(ev, acc)
		return
	}
	acc.Ledger = append(acc.Ledger, LedgerEntry{
		EventID: ev.ID, Type: TypeCredit, Source: SourceInput,
		Account: acc.ID, Amount: ev.Amount, ValueDate: ev.ValueDate,
	})
	acc.Balance += ev.Amount
}

func (e *Engine) processDebit(ev Event, acc *Account) {
	acc.Ledger = append(acc.Ledger, LedgerEntry{
		EventID: ev.ID, Type: TypeDebit, Source: SourceInput,
		Account: acc.ID, Amount: -ev.Amount, ValueDate: ev.ValueDate,
	})
	acc.Balance -= ev.Amount
}

// rawDailyInterestMicros computes one day's interest on a positive closing
// balance, in micro-units (high internal precision). It is NEVER posted
// directly as a LedgerEntry — it exists only to avoid premature rounding.
// Returns 0 for balances <= 0 (interest applies to positive balances only).
func (e *Engine) rawDailyInterestMicros(balanceMinorUnits int64) int64 {
	if balanceMinorUnits <= 0 {
		return 0
	}
	balanceMicros := balanceMinorUnits * MicroUnitsPerMinorUnit
	return (balanceMicros * DailyInterestRateNumerator) / DailyInterestRateDenominator
}

func (e *Engine) capitalizeInterest(day int) {
	for _, acc := range e.Accounts {
		var sumRoundedMinor, sumRawMicros int64
		for _, raw := range acc.DailyRawMicros {
			sumRawMicros += raw
			sumRoundedMinor += RoundMicrosToMinorUnits(raw)

		}
		trueTotal := RoundMicrosToMinorUnits(sumRawMicros)
		remainder := trueTotal - sumRoundedMinor
		feeID := fmt.Sprintf("SYS-INTEREST-CAP-%s", acc.ID)
		acc.Ledger = append(acc.Ledger, LedgerEntry{
			EventID: feeID,
			Type:    TypeInterestCapitalization, Source: SourceSystem,
			Account: acc.ID, Amount: sumRoundedMinor, ValueDate: day,
			Reason: "Capitalization of daily interest accruals",
		})
		acc.Balance += sumRoundedMinor

		if remainder != 0 {
			acc.Ledger = append(acc.Ledger, LedgerEntry{
				EventID: fmt.Sprintf("SYS-INTEREST-ADJ-%s", acc.ID),
				Type:    TypeInterestRoundingAdjust, Source: SourceSystem,
				Account: acc.ID, Amount: remainder, ValueDate: day,
				Reason: "Reconciles rounded daily accruals to independently-rounded true total",
			})
		}
	}

}

func (e *Engine) generateDayReport(day int) map[string]DayReport {
	result := make(map[string]DayReport)
	for _, account := range e.Accounts {
		dayReport := DayReport{
			Day:              day,
			Account:          account.ID,
			Currency:         account.Currency,
			ClosingBalance:   account.Balance,
			AvailableBalance: account.AvailableBalance(),
			Holds:            account.GetAuthState(),
			FeesAssessed:     account.FeesAssessed[day],
			Errors:           account.Errors[day],
		}
		result[account.ID] = dayReport
	}
	return result
}

func (e *Engine) drainPending(day int) {
	for _, acc := range e.Accounts {
		if amt, ok := acc.PendingBalanceByDay[day]; ok {
			acc.Balance += amt
			delete(acc.PendingBalanceByDay, day)
		}
	}
}
