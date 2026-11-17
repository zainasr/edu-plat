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
// way the parts add up to the amount and no challan is paid beyond what it owes. A payment above what
// the family owes is refused (advance is a later slice).
func allocate(amount money.Money, balances []Balance, explicit []Allocation) ([]Allocation, error) {
	open := make([]Balance, 0, len(balances))
	var owed money.Money
	for _, b := range balances {
		if b.Owed() > 0 {
			open = append(open, b)
			owed += b.Owed()
		}
	}
	if len(open) == 0 {
		return nil, apperr.Conflict("payments.no_dues", "This family owes nothing right now, so there is nothing to pay.")
	}
	if amount > owed {
		return nil, apperr.Conflict("payments.exceeds_dues",
			fmt.Sprintf("The family owes %s, which is less than this payment. Take no more than what is owed.", owed))
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
				return nil, allocationInvalid("A line names a challan that is not an open challan of this family.")
			case seen[a.ChallanID]:
				return nil, allocationInvalid("Each challan may appear once.")
			case !a.Amount.IsPositive():
				return nil, allocationInvalid("Each line must be more than zero.")
			case a.Amount > b.Owed():
				return nil, allocationInvalid("A line is more than its challan still owes.")
			}
			seen[a.ChallanID] = true
			sum += a.Amount
		}
		if sum != amount {
			return nil, allocationInvalid("The lines must add up to the payment amount.")
		}
		return append([]Allocation(nil), explicit...), nil
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
	return out, nil
}

func allocationInvalid(msg string) error {
	return apperr.Validation("payments.allocation_invalid", msg, apperr.Field{Field: "allocations", Message: msg})
}

// paymentEntry is the journal entry a payment posts: the money arrives in Cash or Bank (debit) and the
// family's receivable falls (credit Fee Receivable) (BR-FEE-008).
func paymentEntry(paymentID, receipt, account string, amount money.Money, on time.Time) ledger.Entry {
	return ledger.Entry{
		OccurredOn: on,
		Memo:       "Payment " + receipt,
		SourceType: "payment.recorded",
		SourceID:   paymentID,
		Lines: []ledger.Line{
			{AccountCode: account, Debit: amount},
			{AccountCode: accountFeeReceivable, Credit: amount},
		},
	}
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
