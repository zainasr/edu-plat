package fees

import (
	"fmt"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/ledger"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// Challan rules: which lines fall due, the numbers, and the receivable entry. Pure functions, no
// database and no clock, so the money rules are easy to test (ADR-0017).

const (
	maxVoidReason  = 300
	defaultDueDay  = 10
	consumerDigits = 13 // digits of the challan number in a consumer number; the 14th is the check digit

	accountFeeReceivable = "1200"
	accountFeeIncome     = "4000"
)

// GenerateInput is a request to draft the month's challans.
type GenerateInput struct {
	Month   time.Time
	DueDate *time.Time
	DryRun  bool
}

// validateGenerate normalises the month to its first day and picks the due date (the 10th by default).
// Past months are allowed (backfill); a month more than one ahead of now is refused (owner decision,
// 2026-11-15). Pass now in Pakistan time, since "this month" is decided there.
func validateGenerate(in GenerateInput, now time.Time) (month, due time.Time, err error) {
	var f fieldErrors
	if in.Month.IsZero() {
		f.add("month", "Required.")
		return time.Time{}, time.Time{}, f.err("fees.generate_invalid")
	}
	month = monthStart(in.Month)
	if month.After(monthStart(now).AddDate(0, 1, 0)) {
		f.add("month", "Cannot be more than one month ahead.")
	}
	due = time.Date(month.Year(), month.Month(), defaultDueDay, 0, 0, 0, 0, time.UTC)
	if in.DueDate != nil {
		due = time.Date(in.DueDate.Year(), in.DueDate.Month(), in.DueDate.Day(), 0, 0, 0, 0, time.UTC)
		if due.Before(month) {
			f.add("due_date", "Must not be before the month starts.")
		}
	}
	return month, due, f.err("fees.generate_invalid")
}

// dueLines picks the lines of a fee structure that belong on the challan for a month (REQ-FEE-002):
//   - monthly lines, every month;
//   - one-time lines, only in the month of the student's first enrolment;
//   - annual lines, only in the month of the year in which the plan line first took effect (the anchor);
//   - ad hoc lines never (they come from the Additional Fees page, REQ-FEE-060).
//
// A line with a zero amount (a full concession) is left off: a challan line is always positive.
func dueLines(month time.Time, lines []Line, anchors map[string]time.Time, firstEnrolled time.Time) []Line {
	month = monthStart(month)
	var out []Line
	for _, l := range lines {
		if !l.Amount.IsPositive() {
			continue
		}
		switch l.Frequency {
		case "monthly":
		case "one_time":
			if !monthStart(firstEnrolled).Equal(month) {
				continue
			}
		case "annual":
			anchor, ok := anchors[l.FeeTypeID]
			if !ok || anchor.Month() != month.Month() {
				continue
			}
		default:
			continue
		}
		out = append(out, l)
	}
	return out
}

// challanTotal adds the lines with overflow checking.
func challanTotal(lines []Line) (money.Money, error) {
	var total money.Money
	for _, l := range lines {
		next, err := total.Add(l.Amount)
		if err != nil {
			return 0, fmt.Errorf("challan total: %w", err)
		}
		total = next
	}
	return total, nil
}

// consumerNumber is the 14-digit number a bank app uses to find a challan: the challan number padded
// to 13 digits, then a Luhn check digit so a mistyped number is caught before it reaches the books.
// The aggregator's real scheme is still open; this one is unique per school and replaceable (ADR-0017).
func consumerNumber(challanNo int64) string {
	payload := fmt.Sprintf("%0*d", consumerDigits, challanNo)
	return payload + string(luhnCheckDigit(payload))
}

// luhnCheckDigit returns the digit that makes payload+digit pass the Luhn check.
func luhnCheckDigit(payload string) byte {
	sum := 0
	double := true // the rightmost payload digit is doubled: the check digit will sit to its right
	for i := len(payload) - 1; i >= 0; i-- {
		d := int(payload[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return byte('0' + (10-sum%10)%10)
}

// luhnValid reports whether s ends in a correct Luhn check digit.
func luhnValid(s string) bool {
	if len(s) < 2 {
		return false
	}
	return luhnCheckDigit(s[:len(s)-1]) == s[len(s)-1]
}

// receivableEntry is the journal entry a release posts (BR-FEE-008): the school is now owed the total
// (debit Fee Receivable) and has earned it (credit Fee Income).
func receivableEntry(challanID string, challanNo int64, total money.Money, on time.Time) ledger.Entry {
	return ledger.Entry{
		OccurredOn: on,
		Memo:       fmt.Sprintf("Challan %d released", challanNo),
		SourceType: "challan.released",
		SourceID:   challanID,
		Lines: []ledger.Line{
			{AccountCode: accountFeeReceivable, Debit: total},
			{AccountCode: accountFeeIncome, Credit: total},
		},
	}
}

func validateVoidReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > maxVoidReason {
		var f fieldErrors
		f.add("reason", "Required, up to 300 characters.")
		return "", f.err("fees.void_invalid")
	}
	return reason, nil
}
