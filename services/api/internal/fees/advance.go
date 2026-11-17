package fees

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/fees/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// Advance, refunds and defaulters (ADR-0021). Advance is money a family paid above what it owed, held as
// a liability and spent only on that family's next challans (DEC-010). It is applied automatically when a
// challan is released; what cannot be applied is refunded explicitly. Nothing is edited: applications and
// refunds are rows, and what is left is worked out.

// RefundAdvance pays back advance that has not been applied. One step by someone with fee.refund,
// audited (owner decision 2026-11-17). It posts debit Advance Fees Received, credit Cash or Bank, and
// can never exceed what the payment still holds.
func (s *Service) RefundAdvance(ctx context.Context, p iam.Principal, paymentID string, in RefundInput) (Payment, error) {
	in, err := validateRefund(in)
	if err != nil {
		return Payment{}, err
	}
	var out Payment
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := paymentIDs(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(paymentID)
		if err != nil {
			return errPaymentNotFound
		}
		// The payment row lock serialises this against applying the same advance at release, another
		// refund, and a void.
		locked, err := q.LockPayment(ctx, gen.LockPaymentParams{TenantID: tenant, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("lock payment: %w", err)
		}
		if locked.Status != "recorded" {
			return apperr.Conflict("payments.already_voided", "A voided payment holds no advance.")
		}
		row, err := q.GetPayment(ctx, gen.GetPaymentParams{TenantID: tenant, ID: id})
		if err != nil {
			return fmt.Errorf("read payment: %w", err)
		}
		remaining := money.Money(row.Advance - row.AdvanceApplied - row.AdvanceRefunded)
		if err := refundAllowed(remaining, in.Amount); err != nil {
			return err
		}
		receipt := receiptNumber(int(row.FiscalYear), row.ReceiptNo)
		entry, err := s.ledger.Post(ctx, tx, p, advanceRefundEntry(paymentID, receipt, in.AccountCode, in.Amount, s.now().In(pakistanZone)))
		if err != nil {
			return fmt.Errorf("post refund: %w", err)
		}
		entryID, err := db.ParseUUID(entry.ID)
		if err != nil {
			return err
		}
		refundID, err := q.InsertAdvanceRefund(ctx, gen.InsertAdvanceRefundParams{
			TenantID: tenant, PaymentID: id, Amount: int64(in.Amount), AccountCode: in.AccountCode, Reason: in.Reason,
			JournalEntryID: entryID, CreatedBy: actor,
		})
		if err != nil {
			return fmt.Errorf("insert refund: %w", err)
		}
		if err := s.audit(ctx, tx, p, "advance.refunded", "payment", paymentID, map[string]any{
			"refund_id": db.UUIDString(refundID), "amount": int64(in.Amount), "account": in.AccountCode, "reason": in.Reason, "receipt_no": receipt,
		}); err != nil {
			return err
		}
		out, err = readPayment(ctx, q, tenant, id)
		return err
	})
	return out, retryable(err)
}

// applyAdvanceToChallan spends the family's advance on a challan that was just released, oldest payment
// first, inside the release transaction. The caller has already locked the family's advance payments.
func (s *Service) applyAdvanceToChallan(ctx context.Context, tx pgx.Tx, q *gen.Queries, p iam.Principal, tenant, actor, challan, family pgtype.UUID, challanNo int64, total money.Money, on time.Time) error {
	held, err := familyAdvance(ctx, q, tenant, family)
	if err != nil {
		return err
	}
	uses := applyAdvance(total, held)
	if len(uses) == 0 {
		return nil
	}
	var applied money.Money
	for _, u := range uses {
		payment, err := db.ParseUUID(u.PaymentID)
		if err != nil {
			return err
		}
		if err := q.InsertAdvanceApplication(ctx, gen.InsertAdvanceApplicationParams{
			TenantID: tenant, PaymentID: payment, ChallanID: challan, Amount: int64(u.Amount), CreatedBy: actor,
		}); err != nil {
			return fmt.Errorf("apply advance: %w", err)
		}
		applied += u.Amount
	}
	if _, err := s.ledger.Post(ctx, tx, p, advanceAppliedEntry(db.UUIDString(challan), challanNo, applied, on)); err != nil {
		return fmt.Errorf("post advance applied: %w", err)
	}
	return s.audit(ctx, tx, p, "advance.applied", "challan", db.UUIDString(challan), map[string]any{
		"challan_no": challanNo, "amount": int64(applied), "payments": len(uses),
	})
}

// ---- defaulters -----------------------------------------------------------------------

// FeePayerContact is who to call about a family's fees.
type FeePayerContact struct {
	Name         string
	Mobile       string
	Relationship string
}

// Defaulter is a family with challans overdue by at least the window.
type Defaulter struct {
	FamilyID     string
	OverdueTotal money.Money
	OldestDue    time.Time
	DaysOverdue  int
	FeePayer     *FeePayerContact
	Challans     []DueItem
}

// DefaulterPage is one page of defaulters, most overdue first.
type DefaulterPage struct {
	Items      []Defaulter
	NextCursor string
}

// DefaulterFilter narrows the list. MinDays nil means the default of 30 days.
type DefaulterFilter struct {
	MinDays *int
	Limit   int
	Cursor  string
}

var errDefaulterCursor = apperr.Validation("defaulters.cursor_invalid", "The cursor is not valid. Start again without a cursor.")

// ListDefaulters finds families whose released challans are overdue by at least the window and still have
// a balance (REQ-FEE-041), with the fee payer to call (REQ-FEE-042). Nothing is stored: it is worked out
// from the challans and payments as they stand, so a payment removes a family from the list at once.
func (s *Service) ListDefaulters(ctx context.Context, p iam.Principal, f DefaulterFilter) (DefaulterPage, error) {
	minDays := 0
	if f.MinDays != nil {
		minDays = *f.MinDays
	}
	minDays, err := validateMinDays(minDays, f.MinDays == nil)
	if err != nil {
		return DefaulterPage{}, err
	}
	limit := f.Limit
	if limit == 0 {
		limit = defaultPage
	}
	if limit < 1 || limit > maxPage {
		return DefaulterPage{}, apperr.Validation("defaulters.limit_invalid", "limit must be between 1 and 200.")
	}
	afterDue, afterFamily, err := decodeDefaulterCursor(f.Cursor)
	if err != nil {
		return DefaulterPage{}, err
	}
	today := s.now().In(pakistanZone)
	cutoff := overdueCutoff(today, minDays)

	var page DefaulterPage
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := paymentIDs(p)
		if err != nil {
			return err
		}
		params := gen.DefaulterFamiliesParams{TenantID: tenant, Cutoff: pgtype.Date{Time: cutoff, Valid: true}, PageLimit: int32(limit + 1)}
		if !afterDue.IsZero() {
			params.AfterDue = pgtype.Date{Time: afterDue, Valid: true}
			params.AfterFamily = afterFamily
		}
		rows, err := q.DefaulterFamilies(ctx, params)
		if err != nil {
			return fmt.Errorf("defaulter families: %w", err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		if len(rows) == 0 {
			page.Items = []Defaulter{}
			return nil
		}
		famIDs := make([]pgtype.UUID, 0, len(rows))
		famStrs := make([]string, 0, len(rows))
		for _, r := range rows {
			famIDs = append(famIDs, r.FamilyID)
			famStrs = append(famStrs, db.UUIDString(r.FamilyID))
		}
		challanRows, err := q.OverdueChallansForFamilies(ctx, gen.OverdueChallansForFamiliesParams{TenantID: tenant, FamilyIds: famIDs, Cutoff: pgtype.Date{Time: cutoff, Valid: true}})
		if err != nil {
			return fmt.Errorf("overdue challans: %w", err)
		}
		byFamily := map[string][]DueItem{}
		for _, c := range challanRows {
			owed := money.Money(c.Total - c.Paid)
			if owed <= 0 {
				continue
			}
			k := db.UUIDString(c.FamilyID)
			byFamily[k] = append(byFamily[k], DueItem{
				ChallanID: db.UUIDString(c.ID), StudentID: db.UUIDString(c.StudentID), ChallanNo: c.ChallanNo.Int64,
				Month: c.Month.Time, DueDate: c.DueDate.Time, Total: money.Money(c.Total), Paid: money.Money(c.Paid), Balance: owed,
			})
		}
		payers, err := s.students.FamilyFeePayers(ctx, tx, p.TenantID, famStrs)
		if err != nil {
			return err
		}
		page.Items = make([]Defaulter, 0, len(rows))
		for _, r := range rows {
			k := db.UUIDString(r.FamilyID)
			d := Defaulter{
				FamilyID: k, OverdueTotal: money.Money(r.OverdueTotal), OldestDue: r.OldestDue.Time,
				DaysOverdue: daysOverdue(today, r.OldestDue.Time), Challans: byFamily[k],
			}
			if d.Challans == nil {
				d.Challans = []DueItem{}
			}
			if fp, ok := payers[k]; ok {
				d.FeePayer = &FeePayerContact{Name: fp.Name, Mobile: fp.Mobile, Relationship: fp.Relationship}
			}
			page.Items = append(page.Items, d)
		}
		if more {
			last := rows[len(rows)-1]
			page.NextCursor = encodeDefaulterCursor(last.OldestDue.Time, db.UUIDString(last.FamilyID))
		}
		return nil
	})
	return page, err
}

func encodeDefaulterCursor(oldestDue time.Time, familyID string) string {
	return encodeKeyset(oldestDue, familyID)
}

func decodeDefaulterCursor(s string) (time.Time, pgtype.UUID, error) {
	k, err := decodeKeyset(s, errDefaulterCursor)
	if err != nil || k == nil {
		return time.Time{}, pgtype.UUID{}, err
	}
	return k.createdAt, k.id, nil
}

// sortedUnique returns the distinct ids in order, so locks are always taken in the same order.
func sortedUnique(ids []string) []string {
	out := slices.Clone(ids)
	sort.Strings(out)
	return slices.Compact(out)
}
