package main

import (
	"Banking/ledger"
	"strconv"
)

func main() {

	accounts := map[string]*ledger.Account{
		"ACC-001": ledger.NewAccount("ACC-001", "AED", 2, 0),
		"ACC-002": ledger.NewAccount("ACC-002", "BHD", 3, 0),
	}
	ledgerEngine := ledger.NewEngine(accounts)
	events, err := ledger.ParseCSVRecords("input.csv", func(record []string) (ledger.Event, error) {
		event := ledger.Event{}
		day, err := strconv.Atoi(record[1])
		if err != nil {
			return event, err
		}
		amount, err := ledger.ParseAmountToMinorUnits(record[5], record[4])
		if err != nil {
			return event, err
		}
		valueDate, err := strconv.Atoi(record[6])
		if err != nil {
			return event, err
		}
		installmentCount, err := strconv.Atoi(record[9])
		if err != nil {
			return event, err
		}
		return ledger.Event{
			ID:              record[0],
			Day:             day,
			Type:            ledger.EntryType(record[2]),
			Account:         record[3],
			Currency:        record[4],
			Amount:          amount,
			ValueDate:       valueDate,
			AuthID:          record[7],
			RefEvent:        record[8],
			InstalmentCount: installmentCount,
		}, nil
	})
	if err != nil {
		panic(err)
	}
	ledgerEngine.Replay(events)

}
