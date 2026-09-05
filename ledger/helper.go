package ledger

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// ParseCSVRecords reads a CSV line-by-line and applies a converter function
// that returns a *T and an error. It collects and returns all parsed results.
func ParseCSVRecords[T any](filePath string, parseFn func(record []string) (T, error)) ([]T, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open file %s: %w", filePath, err)
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Printf("failed to close file %s: %v", filePath, err)
		}
	}(file)

	reader := csv.NewReader(file)
	var result []T
	rowIndex := 0

	for {
		record, err := reader.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("error reading CSV at row %d: %w", rowIndex, err)
		}

		item, err := parseFn(record)
		if err != nil {
			return nil, fmt.Errorf("error parsing row %d: %w", rowIndex, err)
		}

		result = append(result, item)
		rowIndex++
	}

	return result, nil
}

// SplitEqual divides `total` into `n` as-equal-as-possible minor-unit parts,
// applying the remainder-distribution rule (NUMBERS.md #6): the first part
// absorbs whatever residual is left after integer division, so the parts
// always sum exactly back to `total`, regardless of divisibility.
func SplitEqual(total int64, n int) []int64 {
	base := total / int64(n)
	remainder := total - base*int64(n)
	parts := make([]int64, n)
	for i := range parts {
		parts[i] = base
	}
	parts[0] += remainder
	return parts
}

// RoundMicrosToMinorUnits performs the single, deliberate rounding step from
// high-precision micro-units down to real currency minor units, using
// round-half-to-even (banker's rounding) per NUMBERS.md #5.
func RoundMicrosToMinorUnits(micros int64) int64 {
	quotient := micros / MicroUnitsPerMinorUnit
	remainder := micros % MicroUnitsPerMinorUnit
	half := MicroUnitsPerMinorUnit / 2

	switch {
	case remainder > half || (remainder == half && quotient%2 != 0):
		quotient++
	case remainder < -half || (remainder == -half && quotient%2 != 0):
		quotient--
	}
	return quotient
}

// currencyPrecision maps a currency code to its decimal precision.
// Mirrors NUMBERS.md #2 — kept as a lookup here so the parser doesn't need
// an *Account in scope, just the currency code from the CSV row.
var currencyPrecision = map[string]int{
	"AED": PrecisionAED,
	"BHD": PrecisionBHD,
}

// ParseAmountToMinorUnits converts a decimal amount string (e.g. "1200.00",
// "10.000") into an int64 in the currency's minor units (e.g. 120000,
// 10000), using pure string/integer arithmetic — never through a float
// intermediate (see NUMBERS.md #1: float rounding is not acceptable for
// ledger money).
//
// The input must have exactly the currency's expected number of decimal
// places (2 for AED, 3 for BHD) — this is intentionally strict so a
// malformed input.csv row (wrong precision for its currency) fails loudly
// at load time rather than silently truncating or padding a real number.
func ParseAmountToMinorUnits(amountStr, currency string) (int64, error) {
	precision, ok := currencyPrecision[currency]
	if !ok {
		return 0, fmt.Errorf("unknown currency %q", currency)
	}

	amountStr = strings.TrimSpace(amountStr)
	if amountStr == "" {
		return 0, fmt.Errorf("empty amount")
	}

	negative := false
	if strings.HasPrefix(amountStr, "-") {
		negative = true
		amountStr = amountStr[1:]
	}

	parts := strings.SplitN(amountStr, ".", 2)
	wholePart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}

	if wholePart == "" {
		return 0, fmt.Errorf("amount %q has no whole-number part", amountStr)
	}
	if !isAllDigits(wholePart) || (fracPart != "" && !isAllDigits(fracPart)) {
		return 0, fmt.Errorf("amount %q is not a valid decimal number", amountStr)
	}
	if len(fracPart) != precision {
		return 0, fmt.Errorf(
			"amount %q has %d decimal place(s), currency %s requires exactly %d",
			amountStr, len(fracPart), currency, precision,
		)
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q: whole part invalid: %w", amountStr, err)
	}
	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q: fractional part invalid: %w", amountStr, err)
	}

	scale := int64(1)
	for i := 0; i < precision; i++ {
		scale *= 10
	}

	minorUnits := whole*scale + frac
	if negative {
		minorUnits = -minorUnits
	}
	return minorUnits, nil
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// MinorUnitsToFloat converts a minor-unit amount into a major-unit with currency attached
// amount for DISPLAY ONLY — e.g. 14545 AED -> 145.45 AED. Never feed this back
// into ledger arithmetic or persist it as a balance (NUMBERS.md #1); use
// FormatAmount if you just need a string.
func MinorUnitsToFloat(amount int64, currency string) (string, error) {
	precision, ok := currencyPrecision[currency]
	if !ok {
		return "", fmt.Errorf("unknown currency %q", currency)
	}
	scale := int64(1)
	for i := 0; i < precision; i++ {
		scale *= 10
	}
	return fmt.Sprintf("%.*f %s", precision, float64(amount)/float64(scale), currency), nil
}
