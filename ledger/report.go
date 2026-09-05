package ledger

import (
	"fmt"
	"sort"
)

// ---------------------------------------------------------------------------
// DayReport printing
// ---------------------------------------------------------------------------

func PrintDayReport(r DayReport) error {
	fmt.Println("=== Day", r.Day, "— Account", r.Account, "===")
	closingBalance, err := MinorUnitsToFloat(r.ClosingBalance, r.Currency)
	if err != nil {
		return err
	}
	fmt.Println("Closing balance:  ", closingBalance)
	availableBalance, err := MinorUnitsToFloat(r.AvailableBalance, r.Currency)
	if err != nil {
		return err
	}
	fmt.Println("Available balance:", availableBalance)

	fmt.Println("Holds:")
	if len(r.Holds) == 0 {
		fmt.Println("  (none)")
	}
	for _, h := range r.Holds {
		amount, err := MinorUnitsToFloat(h.Amount, r.Currency)
		if err != nil {
			return err
		}
		fmt.Printf("Amount %s Active: %v\n", amount, h.IsActive)
	}

	fmt.Println("Fees assessed:")
	if len(r.FeesAssessed) == 0 {
		fmt.Println("  (none)")
	}
	for _, f := range r.FeesAssessed {
		fmt.Println(" ", f.EventID, f.Amount, f.Reason)
	}

	fmt.Println("Errors:")
	if len(r.Errors) == 0 {
		fmt.Println("  (none)")
	}
	for _, e := range r.Errors {
		fmt.Println(" ", e.EventID, e.Reason)
	}
	fmt.Println()
	return nil
}

// PrintDayReports prints a map of DayReport (as generateDayReport returns)
// in a deterministic account order, since ranging over a map directly would
// print accounts in random order on every run.
func PrintDayReports(reports map[string]DayReport) error {
	ids := make([]string, 0, len(reports))
	for id := range reports {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		err := PrintDayReport(reports[id])
		if err != nil {
			return err
		}
	}
	return nil
}

// generateSummaryReport builds one SummaryReport per account. Call it AFTER
// e.Replay(events) has returned — capitalizeInterest only runs at the very
// end of Replay, so calling this mid-replay will always show
// InterestCapitalized as 0.
func (e *Engine) generateSummaryReport(day int) map[string]SummaryReport {
	result := make(map[string]SummaryReport)

	for id, account := range e.Accounts {
		s := SummaryReport{
			Account:               account.ID,
			Currency:              account.Currency,
			FinalClosingBalance:   account.ClosingBalance(day),
			FinalAvailableBalance: account.AvailableBalance(day),
		}

		for _, entry := range account.Ledger {
			switch entry.Type {
			case TypeInterestCapitalization:
				s.InterestCapitalized = entry.Amount
			case TypeInterestRoundingAdjust:
				s.InterestRoundingAdjust = entry.Amount
			}
		}

		for _, h := range account.Holds {
			if h.Active {
				s.HoldsStillActive++
			}
		}

		result[id] = s
	}

	return result
}

func PrintSummaryReport(s SummaryReport) error {
	fmt.Println("=== Summary — Account", s.Account, "===")

	closingBalance, err := MinorUnitsToFloat(s.FinalClosingBalance, s.Currency)
	if err != nil {
		return err
	}
	fmt.Println("Final closing balance:  ", closingBalance)

	finalAvailableBalance, err := MinorUnitsToFloat(s.FinalAvailableBalance, s.Currency)
	if err != nil {
		return err
	}
	fmt.Println("Final available balance:", finalAvailableBalance)

	interestCapitalized, err := MinorUnitsToFloat(s.InterestCapitalized, s.Currency)
	if err != nil {
		return err
	}
	fmt.Println("Interest capitalized:   ", interestCapitalized)
	if s.InterestRoundingAdjust != 0 {
		interestRoundingAdjust, err := MinorUnitsToFloat(s.InterestRoundingAdjust, s.Currency)
		if err != nil {
			return err
		}
		fmt.Println("Interest rounding adj.: ", interestRoundingAdjust)
	}
	fmt.Println("Holds still active:     ", s.HoldsStillActive)
	fmt.Println()
	return nil
}

// PrintSummaryReports prints every account's summary in deterministic order.
func PrintSummaryReports(summaries map[string]SummaryReport) error {
	ids := make([]string, 0, len(summaries))
	for id := range summaries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		err := PrintSummaryReport(summaries[id])
		if err != nil {
			return err
		}
	}
	return nil
}
