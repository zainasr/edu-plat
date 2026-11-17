package fees

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/ledger"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// Payment rules: pure functions, no database and no clock (ADR-0020).

const (
	accountAdvance       = "2000" // Advance Fees Received: a liability held for the family (DEC-010)
	defaultDefaulterDays = 30
	maxDefaulterDays     = 365

	receiptPrefix    = "RCP"
	maxReference     = 100
	minPayVoidReason = 3
	maxPayVoidReason = 200
	accountCash      = "1000"
	accountBank      = "1100"
)

var paymentMethods = map[string]bool{"cash": true, "bank_transfer": true, "cheque": true}

// PaymentInput is a request to take a payment from a family.
type PaymentInput struct {
	FamilyID    string
	Amount      money.Money
	Method      string
	AccountCode string
	ReceivedOn  time.Time // zero means today
	Reference   string
	Allocations []Allocation // optional; default is oldest first
	// HoldExcessAsAdvance asks for anything the challans cannot take to be held as Advance; without it a
	// payment above what the family owes is refused.
	HoldExcessAsAdvance bool
}

// Allocation is part of a payment applied to one challan.
type Allocation struct {
	ChallanID string
	Amount    money.Money
}

// Balance is a released challan and what is still owed on it.
type Balance struct {
	ChallanID string
	StudentID string
	DueDate   time.Time
	ChallanNo int64
	Month     time.Time
	Total     money.Money
	Paid      money.Money
}

// Owed is what is left to pay on the challan.
func (b Balance) Owed() money.Money { return b.Total - b.Paid }

// fiscalYear names the July to June fiscal year by the year it starts (owner decision, 2026-11-17).
func fiscalYear(on time.Time) int {
	if on.Month() >= time.July {
		return on.Year()
	}
	return on.Year() - 1
}

// receiptNumber is PREFIX-YEAR-NNNNNN, the sequence padded to six digits (REQ-FEE-024).
func receiptNumber(fiscalYear int, n int64) string {
	return fmt.Sprintf("%s-%d-%06d", receiptPrefix, fiscalYear, n)
}

// validatePayment checks the request and fills in the date (today by default, never in the future).
func validatePayment(in PaymentInput, today time.Time) (PaymentInput, error) {
	var f fieldErrors
	if _, err := db.ParseUUID(in.FamilyID); err != nil {
		f.add("family_id", "Required: the family paying.")
	}
	if !in.Amount.IsPositive() {
		f.add("amount", "Must be more than zero.")
	}
	if !paymentMethods[in.Method] {
		f.add("method", "Choose cash, bank_transfer or cheque.")
	}
	if in.AccountCode != accountCash && in.AccountCode != accountBank {
		f.add("account_code", "Choose the account the money goes to: 1000 (Cash in Hand) or 1100 (Bank).")
	}
	in.Reference = strings.TrimSpace(in.Reference)
	if len(in.Reference) > maxReference {
		f.add("reference", "Up to 100 characters.")
	}
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if in.ReceivedOn.IsZero() {
		in.ReceivedOn = today
	} else {
		in.ReceivedOn = time.Date(in.ReceivedOn.Year(), in.ReceivedOn.Month(), in.ReceivedOn.Day(), 0, 0, 0, 0, time.UTC)
		if in.ReceivedOn.After(today) {
			f.add("received_on", "A payment cannot be dated in the future.")
		}
	}
	return in, f.err("payments.invalid")
}

// allocate splits a payment across a family's challans. By default the oldest due date is paid first
// (ties by challan number); with explicit allocations the cashier's split is checked instead. Either
// way no challan is paid beyond what it owes. What the challans cannot take is Advance, held for the
// family's next challans, but only when the caller asked for it (holdExcess): otherwise a payment above
// what is owed is refused, so a typing slip cannot silently park money (owner decision 2026-11-17).
// It returns the allocations and the advance; allocations plus advance equal the amount.
func allocate(amount money.Money, balances []Balance, explicit []Allocation, holdExcess bool) ([]Allocation, money.Money, error) {
	open := make([]Balance, 0, len(balances))
	var owed money.Money
	for _, b := range balances {
		if b.Owed() > 0 {
			open = append(open, b)
			owed += b.Owed()
		}
	}

	if len(explicit) > 0 {
		byID := make(map[string]Balance, len(open))
		for _, b := range open {
			byID[b.ChallanID] = b
		}
		seen := map[string]bool{}
		var sum money.Money
		for _, a := range explicit {
			b, ok := byID[a.ChallanID]
			switch {
			case !ok:
				return nil, 0, allocationInvalid("A line names a challan that is not an open challan of this family.")
			case seen[a.ChallanID]:
				return nil, 0, allocationInvalid("Each challan may appear once.")
			case !a.Amount.IsPositive():
				return nil, 0, allocationInvalid("Each line must be more than zero.")
			case a.Amount > b.Owed():
				return nil, 0, allocationInvalid("A line is more than its challan still owes.")
			}
			seen[a.ChallanID] = true
			sum += a.Amount
		}
		switch {
		case sum > amount:
			return nil, 0, allocationInvalid("The lines add up to more than the payment.")
		case sum < amount && !holdExcess:
			return nil, 0, allocationInvalid("The lines must add up to the payment amount, or ask to hold the rest as advance.")
		}
		return append([]Allocation(nil), explicit...), amount - sum, nil
	}

	if len(open) == 0 {
		if holdExcess {
			return nil, amount, nil // nothing owed yet: the whole payment is advance against the next challan
		}
		return nil, 0, apperr.Conflict("payments.no_dues", "This family owes nothing right now, so there is nothing to pay.")
	}
	if amount > owed && !holdExcess {
		return nil, 0, apperr.Conflict("payments.exceeds_dues",
			fmt.Sprintf("The family owes %s, which is less than this payment. Take no more than what is owed, or ask to hold the extra as advance.", owed))
	}

	sort.Slice(open, func(i, j int) bool {
		if !open[i].DueDate.Equal(open[j].DueDate) {
			return open[i].DueDate.Before(open[j].DueDate)
		}
		if open[i].ChallanNo != open[j].ChallanNo {
			return open[i].ChallanNo < open[j].ChallanNo
		}
		return open[i].ChallanID < open[j].ChallanID
	})
	var out []Allocation
	left := amount
	for _, b := range open {
		if left == 0 {
			break
		}
		take := b.Owed()
		if take > left {
			take = left
		}
		out = append(out, Allocation{ChallanID: b.ChallanID, Amount: take})
		left -= take
	}
	return out, left, nil
}

func allocationInvalid(msg string) error {
	return apperr.Validation("payments.allocation_invalid", msg, apperr.Field{Field: "allocations", Message: msg})
}

// paymentEntry is the journal entry a payment posts: the money arrives in Cash or Bank (debit); the
// family's receivable falls by what pays challans (credit Fee Receivable); anything held as advance is a
// liability to the family (credit Advance Fees Received) (BR-FEE-008, DEC-010).
func paymentEntry(paymentID, receipt, account string, amount, advance money.Money, on time.Time) ledger.Entry {
	lines := []ledger.Line{{AccountCode: account, Debit: amount}}
	if paid := amount - advance; paid > 0 {
		lines = append(lines, ledger.Line{AccountCode: accountFeeReceivable, Credit: paid})
	}
	if advance > 0 {
		lines = append(lines, ledger.Line{AccountCode: accountAdvance, Credit: advance})
	}
	return ledger.Entry{
		OccurredOn: on,
		Memo:       "Payment " + receipt,
		SourceType: "payment.recorded",
		SourceID:   paymentID,
		Lines:      lines,
	}
}

// AdvanceBalance is what one payment still holds as advance.
type AdvanceBalance struct {
	PaymentID string
	Remaining money.Money
}

// AdvanceUse is part of one payment's advance applied to a challan.
type AdvanceUse struct {
	PaymentID string
	Amount    money.Money
}

// applyAdvance spends a family's advance on a newly released challan: oldest payment first, never more
// than the challan owes and never more than a payment still holds. held must be in the order to use it.
func applyAdvance(owed money.Money, held []AdvanceBalance) []AdvanceUse {
	var out []AdvanceUse
	left := owed
	for _, h := range held {
		if left <= 0 {
			break
		}
		if h.Remaining <= 0 {
			continue
		}
		take := h.Remaining
		if take > left {
			take = left
		}
		out = append(out, AdvanceUse{PaymentID: h.PaymentID, Amount: take})
		left -= take
	}
	return out
}

// advanceAppliedEntry moves advance from a liability to the receivable it pays: debit Advance Fees
// Received, credit Fee Receivable.
func advanceAppliedEntry(challanID string, challanNo int64, amount money.Money, on time.Time) ledger.Entry {
	return ledger.Entry{
		OccurredOn: on,
		Memo:       fmt.Sprintf("Advance applied to challan %d", challanNo),
		SourceType: "advance.applied",
		SourceID:   challanID,
		Lines: []ledger.Line{
			{AccountCode: accountAdvance, Debit: amount},
			{AccountCode: accountFeeReceivable, Credit: amount},
		},
	}
}

// advanceRefundEntry pays advance back to the family: debit Advance Fees Received, credit Cash or Bank.
func advanceRefundEntry(paymentID, receipt, account string, amount money.Money, on time.Time) ledger.Entry {
	return ledger.Entry{
		OccurredOn: on,
		Memo:       "Advance refund for " + receipt,
		SourceType: "advance.refunded",
		SourceID:   paymentID,
		Lines: []ledger.Line{
			{AccountCode: accountAdvance, Debit: amount},
			{AccountCode: account, Credit: amount},
		},
	}
}

// RefundInput is a request to pay back advance.
type RefundInput struct {
	Amount      money.Money
	AccountCode string
	Reason      string
}

func validateRefund(in RefundInput) (RefundInput, error) {
	var f fieldErrors
	if !in.Amount.IsPositive() {
		f.add("amount", "Must be more than zero.")
	}
	if in.AccountCode != accountCash && in.AccountCode != accountBank {
		f.add("account_code", "Choose the account the money is paid from: 1000 (Cash in Hand) or 1100 (Bank).")
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < minPayVoidReason || len(in.Reason) > maxPayVoidReason {
		f.add("reason", "3 to 200 characters.")
	}
	return in, f.err("payments.refund_invalid")
}

// refundAllowed refuses to pay back more than the payment still holds as advance.
func refundAllowed(remaining, amount money.Money) error {
	if amount > remaining {
		return apperr.Conflict("payments.refund_exceeds_advance",
			fmt.Sprintf("Only %s of advance is still held on this payment.", remaining))
	}
	return nil
}

// validateMinDays picks the defaulter window: 30 days unless given, 1 to 365 (REQ-FEE-041).
func validateMinDays(v int, absent bool) (int, error) {
	if absent {
		return defaultDefaulterDays, nil
	}
	if v < 1 || v > maxDefaulterDays {
		var f fieldErrors
		f.add("min_days", "Between 1 and 365.")
		return 0, f.err("defaulters.invalid")
	}
	return v, nil
}

// overdueCutoff is the latest due date that counts as minDays overdue today.
func overdueCutoff(today time.Time, minDays int) time.Time {
	t := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	return t.AddDate(0, 0, -minDays)
}

func daysOverdue(today, due time.Time) int {
	t := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	d := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
	return int(t.Sub(d).Hours() / 24)
}

// paymentProgress says how much of a released challan is paid. Drafts and voided challans have none.
func paymentProgress(status string, total, paid money.Money) string {
	if status != "released" {
		return ""
	}
	switch {
	case paid <= 0:
		return "unpaid"
	case paid >= total:
		return "paid"
	default:
		return "partial"
	}
}

func validatePaymentVoidReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < minPayVoidReason || len(reason) > maxPayVoidReason {
		var f fieldErrors
		f.add("reason", "3 to 200 characters.")
		return "", f.err("payments.void_invalid")
	}
	return reason, nil
}
