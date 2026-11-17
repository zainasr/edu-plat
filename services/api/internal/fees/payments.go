package fees

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/fees/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// Payments at the counter (ADR-0020). One payment is one receipt: a family pays, the money is split
// across its released challans, the payment posts to the ledger in the same transaction, and nothing is
// ever edited afterwards: a mistake is voided, which posts a reversal and keeps the receipt number.

// PaymentAllocation is the part of a payment applied to one challan.
type PaymentAllocation struct {
	ChallanID string
	StudentID string
	ChallanNo int64
	Amount    money.Money
}

// Payment is a payment and its receipt.
type Payment struct {
	ID         string
	ReceiptNo  string
	FiscalYear int
	FamilyID   string
	Amount     money.Money
	// Advance is the part of the payment held for the family's next challans; Applied and Refunded are how
	// much of it has been used, and Available what is left. Worked out when read, never stored.
	Advance          money.Money
	AdvanceApplied   money.Money
	AdvanceRefunded  money.Money
	AdvanceAvailable money.Money
	Method           string
	AccountCode      string
	Reference        *string
	ReceivedOn       time.Time
	Status           string
	Allocations      []PaymentAllocation
	Refunds          []AdvanceRefund
	EntryID          string
	VoidReason       *string
	VoidedAt         *time.Time
	CreatedAt        time.Time
}

// AdvanceRefund is advance paid back to a family.
type AdvanceRefund struct {
	ID          string
	Amount      money.Money
	AccountCode string
	Reason      string
	EntryID     string
	CreatedAt   time.Time
}

// PaymentPage is one page of payments, newest first.
type PaymentPage struct {
	Items      []Payment
	NextCursor string
}

// PaymentFilter narrows a list.
type PaymentFilter struct {
	FamilyID string
	Status   string
	Limit    int
	Cursor   string
}

// DueItem is a released challan with something left to pay.
type DueItem struct {
	ChallanID string
	StudentID string
	ChallanNo int64
	Month     time.Time
	DueDate   time.Time
	Total     money.Money
	Paid      money.Money
	Balance   money.Money
}

// FamilyDues is what a family owes now, in the order a payment would pay it.
type FamilyDues struct {
	FamilyID string
	TotalDue money.Money
	// AdvanceAvailable is the advance the family holds, which the next challans will use.
	AdvanceAvailable money.Money
	Items            []DueItem
}

var (
	errPaymentNotFound = apperr.NotFound("payments.not_found", "Payment not found.")
	errPaymentCursor   = apperr.Validation("payments.cursor_invalid", "The cursor is not valid. Start again without a cursor.")
)

// RecordPayment takes a payment from a family. Everything happens in one transaction: lock the family's
// open challans, work out the split, take the next receipt number, post debit Cash or Bank and credit
// Fee Receivable, then write the payment and its allocations. The database re-checks at commit that the
// allocations add up and that no challan is over-paid.
func (s *Service) RecordPayment(ctx context.Context, p iam.Principal, in PaymentInput) (Payment, error) {
	in, err := validatePayment(in, s.now().In(pakistanZone))
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
		if err := s.students.RequireFamily(ctx, tx, p.TenantID, in.FamilyID); err != nil {
			return err
		}
		family, err := db.ParseUUID(in.FamilyID)
		if err != nil {
			return err
		}
		// Lock the family's released challans, then read balances for exactly the challans locked. A
		// challan released a moment later is simply not part of this payment, so no challan is ever
		// allocated to without its lock held.
		locked, err := q.LockFamilyChallans(ctx, gen.LockFamilyChallansParams{TenantID: tenant, FamilyID: family})
		if err != nil {
			return fmt.Errorf("lock family challans: %w", err)
		}
		balances, err := familyBalances(ctx, q, tenant, family, locked)
		if err != nil {
			return err
		}
		allocs, advance, err := allocate(in.Amount, balances, in.Allocations, in.HoldExcessAsAdvance)
		if err != nil {
			return err
		}

		fy := fiscalYear(in.ReceivedOn)
		no, err := q.NextReceiptNo(ctx, gen.NextReceiptNoParams{TenantID: tenant, Name: fmt.Sprintf("receipt-%d", fy)})
		if err != nil {
			return fmt.Errorf("next receipt number: %w", err)
		}
		receipt := receiptNumber(fy, no)
		id := uuid.New()
		entry, err := s.ledger.Post(ctx, tx, p, paymentEntry(id.String(), receipt, in.AccountCode, in.Amount, advance, in.ReceivedOn))
		if err != nil {
			return fmt.Errorf("post payment: %w", err)
		}
		entryID, err := db.ParseUUID(entry.ID)
		if err != nil {
			return err
		}
		paymentID := pgtype.UUID{Bytes: id, Valid: true}
		params := gen.InsertPaymentParams{
			ID: paymentID, TenantID: tenant, FamilyID: family, FiscalYear: int32(fy), ReceiptNo: no,
			Amount: int64(in.Amount), Advance: int64(advance), Method: in.Method, AccountCode: in.AccountCode,
			ReceivedOn: pgtype.Date{Time: in.ReceivedOn, Valid: true}, JournalEntryID: entryID, CreatedBy: actor,
		}
		if in.Reference != "" {
			params.Reference = pgtype.Text{String: in.Reference, Valid: true}
		}
		if err := q.InsertPayment(ctx, params); err != nil {
			return fmt.Errorf("insert payment: %w", err)
		}
		for _, a := range allocs {
			challan, err := db.ParseUUID(a.ChallanID)
			if err != nil {
				return err
			}
			if err := q.InsertAllocation(ctx, gen.InsertAllocationParams{TenantID: tenant, PaymentID: paymentID, ChallanID: challan, Amount: int64(a.Amount)}); err != nil {
				return fmt.Errorf("insert allocation: %w", err)
			}
		}
		if err := s.audit(ctx, tx, p, "payment.recorded", "payment", id.String(), map[string]any{
			"receipt_no": receipt, "amount": int64(in.Amount), "method": in.Method, "account": in.AccountCode,
			"family_id": in.FamilyID, "challans": len(allocs), "advance": int64(advance),
		}); err != nil {
			return err
		}
		out, err = readPayment(ctx, q, tenant, paymentID)
		return err
	})
	return out, retryable(err)
}

// VoidPayment cancels a payment: the receipt number stays (marked voided), a reversing journal entry
// is posted, and the challans it paid have their balances back, because what is paid is worked out
// from the payments that stand (BR-FEE-002, BR-FEE-003). It can happen once.
func (s *Service) VoidPayment(ctx context.Context, p iam.Principal, id, reason string) (Payment, error) {
	reason, err := validatePaymentVoidReason(reason)
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
		uid, err := db.ParseUUID(id)
		if err != nil {
			return errPaymentNotFound
		}
		row, err := q.LockPayment(ctx, gen.LockPaymentParams{TenantID: tenant, ID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("lock payment: %w", err)
		}
		if row.Status != "recorded" {
			return apperr.Conflict("payments.already_voided", "This payment has already been voided.")
		}
		held, err := q.GetPayment(ctx, gen.GetPaymentParams{TenantID: tenant, ID: uid})
		if err != nil {
			return fmt.Errorf("read payment: %w", err)
		}
		if held.AdvanceApplied+held.AdvanceRefunded > 0 {
			return apperr.Conflict("payments.advance_in_use",
				"Part of this payment's advance has already been applied to a challan or refunded, so it can no longer be voided.")
		}
		reversal, err := s.ledger.Reverse(ctx, tx, p, db.UUIDString(row.JournalEntryID), reason, s.now().In(pakistanZone))
		if err != nil {
			return fmt.Errorf("reverse payment: %w", err)
		}
		reversalID, err := db.ParseUUID(reversal.ID)
		if err != nil {
			return err
		}
		n, err := q.VoidPayment(ctx, gen.VoidPaymentParams{
			TenantID: tenant, ID: uid, Reason: pgtype.Text{String: reason, Valid: true}, VoidedBy: actor, VoidEntryID: reversalID,
		})
		if err != nil {
			return fmt.Errorf("void payment: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("void payment: %d rows changed, want 1", n)
		}
		if err := s.audit(ctx, tx, p, "payment.voided", "payment", id, map[string]any{"reason": reason}); err != nil {
			return err
		}
		out, err = readPayment(ctx, q, tenant, uid)
		return err
	})
	return out, err
}

// GetPayment returns one payment with its allocations.
func (s *Service) GetPayment(ctx context.Context, p iam.Principal, id string) (Payment, error) {
	var out Payment
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, _, err := paymentIDs(p)
		if err != nil {
			return err
		}
		uid, err := db.ParseUUID(id)
		if err != nil {
			return errPaymentNotFound
		}
		out, err = readPayment(ctx, gen.New(tx), tenant, uid)
		return err
	})
	return out, err
}

// ListPayments returns a page, newest first.
func (s *Service) ListPayments(ctx context.Context, p iam.Principal, f PaymentFilter) (PaymentPage, error) {
	limit := f.Limit
	if limit == 0 {
		limit = defaultPage
	}
	if limit < 1 || limit > maxPage {
		return PaymentPage{}, apperr.Validation("payments.limit_invalid", "limit must be between 1 and 200.")
	}
	if f.Status != "" && f.Status != "recorded" && f.Status != "voided" {
		return PaymentPage{}, apperr.Validation("payments.status_invalid", "status must be recorded or voided.")
	}
	after, err := decodeKeyset(f.Cursor, errPaymentCursor)
	if err != nil {
		return PaymentPage{}, err
	}
	var page PaymentPage
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := paymentIDs(p)
		if err != nil {
			return err
		}
		params := gen.ListPaymentsPageParams{TenantID: tenant, PageLimit: int32(limit + 1)}
		if f.FamilyID != "" {
			fam, err := db.ParseUUID(f.FamilyID)
			if err != nil {
				return apperr.Validation("payments.family_invalid", "family_id must be an id.")
			}
			params.FamilyID = fam
		}
		if f.Status != "" {
			params.Status = pgtype.Text{String: f.Status, Valid: true}
		}
		if after != nil {
			params.AfterCreatedAt = pgtype.Timestamptz{Time: after.createdAt, Valid: true}
			params.AfterID = after.id
		}
		rows, err := q.ListPaymentsPage(ctx, params)
		if err != nil {
			return fmt.Errorf("list payments: %w", err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		ids := make([]pgtype.UUID, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		allocs, err := loadAllocations(ctx, q, tenant, ids)
		if err != nil {
			return err
		}
		refunds, err := loadRefunds(ctx, q, tenant, ids)
		if err != nil {
			return err
		}
		page.Items = make([]Payment, 0, len(rows))
		for _, r := range rows {
			item := toPayment(gen.GetPaymentRow(r), allocs[db.UUIDString(r.ID)])
			if rs := refunds[db.UUIDString(r.ID)]; rs != nil {
				item.Refunds = rs
			}
			page.Items = append(page.Items, item)
		}
		if more && len(rows) > 0 {
			last := rows[len(rows)-1]
			page.NextCursor = encodeKeyset(last.CreatedAt.Time, db.UUIDString(last.ID))
		}
		return nil
	})
	return page, err
}

// GetFamilyDues lists what a family owes, challan by challan, in the order a payment would pay them.
func (s *Service) GetFamilyDues(ctx context.Context, p iam.Principal, familyID string) (FamilyDues, error) {
	out := FamilyDues{FamilyID: familyID, Items: []DueItem{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := paymentIDs(p)
		if err != nil {
			return err
		}
		if err := s.students.RequireFamily(ctx, tx, p.TenantID, familyID); err != nil {
			return err
		}
		family, err := db.ParseUUID(familyID)
		if err != nil {
			return err
		}
		held, err := familyAdvance(ctx, q, tenant, family)
		if err != nil {
			return err
		}
		for _, h := range held {
			out.AdvanceAvailable += h.Remaining
		}
		rows, err := q.FamilyChallanBalances(ctx, gen.FamilyChallanBalancesParams{TenantID: tenant, FamilyID: family})
		if err != nil {
			return fmt.Errorf("family balances: %w", err)
		}
		for _, r := range rows {
			owed := money.Money(r.Total - r.Paid)
			if owed <= 0 {
				continue
			}
			out.Items = append(out.Items, DueItem{
				ChallanID: db.UUIDString(r.ID), StudentID: db.UUIDString(r.StudentID), ChallanNo: r.ChallanNo.Int64,
				Month: r.Month.Time, DueDate: r.DueDate.Time, Total: money.Money(r.Total), Paid: money.Money(r.Paid), Balance: owed,
			})
			out.TotalDue += owed
		}
		return nil
	})
	return out, err
}

// ---- helpers --------------------------------------------------------------------------

// retryable turns a refusal that only the database could see (a commit-time constraint or a deadlock)
// into a 409 that tells the client to try again, instead of a 500. Nothing was recorded.
func retryable(err error) error {
	if err != nil && (db.IsCheckViolation(err) || db.IsRetryable(err)) {
		return apperr.Conflict("payments.conflict", "The balances changed while this payment was being recorded. Nothing was recorded; try again.")
	}
	return err
}

func paymentIDs(p iam.Principal) (tenant, actor pgtype.UUID, err error) {
	if tenant, err = db.ParseUUID(p.TenantID); err != nil {
		return
	}
	actor, err = db.ParseUUID(p.UserID)
	return
}

// familyBalances reads the released challans of a family with what each has paid. With only set, just
// those challans (the ones the caller holds locks on); with nil, all of them.
func familyBalances(ctx context.Context, q *gen.Queries, tenant, family pgtype.UUID, only []pgtype.UUID) ([]Balance, error) {
	rows, err := q.FamilyChallanBalances(ctx, gen.FamilyChallanBalancesParams{TenantID: tenant, FamilyID: family, OnlyIds: only})
	if err != nil {
		return nil, fmt.Errorf("family balances: %w", err)
	}
	out := make([]Balance, 0, len(rows))
	for _, r := range rows {
		out = append(out, Balance{
			ChallanID: db.UUIDString(r.ID), StudentID: db.UUIDString(r.StudentID), DueDate: r.DueDate.Time,
			ChallanNo: r.ChallanNo.Int64, Month: r.Month.Time, Total: money.Money(r.Total), Paid: money.Money(r.Paid),
		})
	}
	return out, nil
}

// familyAdvance is what each of a family's payments still holds as advance, oldest first. Spent ones are left out.
func familyAdvance(ctx context.Context, q *gen.Queries, tenant, family pgtype.UUID) ([]AdvanceBalance, error) {
	rows, err := q.FamilyAdvanceBalances(ctx, gen.FamilyAdvanceBalancesParams{TenantID: tenant, FamilyID: family})
	if err != nil {
		return nil, fmt.Errorf("family advance: %w", err)
	}
	out := make([]AdvanceBalance, 0, len(rows))
	for _, r := range rows {
		if r.Remaining > 0 {
			out = append(out, AdvanceBalance{PaymentID: db.UUIDString(r.ID), Remaining: money.Money(r.Remaining)})
		}
	}
	return out, nil
}

func readPayment(ctx context.Context, q *gen.Queries, tenant, id pgtype.UUID) (Payment, error) {
	row, err := q.GetPayment(ctx, gen.GetPaymentParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, errPaymentNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("get payment: %w", err)
	}
	allocs, err := loadAllocations(ctx, q, tenant, []pgtype.UUID{id})
	if err != nil {
		return Payment{}, err
	}
	refunds, err := loadRefunds(ctx, q, tenant, []pgtype.UUID{id})
	if err != nil {
		return Payment{}, err
	}
	out := toPayment(row, allocs[db.UUIDString(id)])
	if r := refunds[db.UUIDString(id)]; r != nil {
		out.Refunds = r
	}
	return out, nil
}

func loadRefunds(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, ids []pgtype.UUID) (map[string][]AdvanceRefund, error) {
	out := map[string][]AdvanceRefund{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListRefundsForPayments(ctx, gen.ListRefundsForPaymentsParams{TenantID: tenant, PaymentIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list refunds: %w", err)
	}
	for _, r := range rows {
		k := db.UUIDString(r.PaymentID)
		out[k] = append(out[k], AdvanceRefund{
			ID: db.UUIDString(r.ID), Amount: money.Money(r.Amount), AccountCode: r.AccountCode, Reason: r.Reason,
			EntryID: db.UUIDString(r.JournalEntryID), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func loadAllocations(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, ids []pgtype.UUID) (map[string][]PaymentAllocation, error) {
	out := map[string][]PaymentAllocation{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListAllocationsForPayments(ctx, gen.ListAllocationsForPaymentsParams{TenantID: tenant, PaymentIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list allocations: %w", err)
	}
	for _, r := range rows {
		k := db.UUIDString(r.PaymentID)
		out[k] = append(out[k], PaymentAllocation{
			ChallanID: db.UUIDString(r.ChallanID), StudentID: db.UUIDString(r.StudentID), ChallanNo: r.ChallanNo.Int64, Amount: money.Money(r.Amount),
		})
	}
	return out, nil
}

func toPayment(r gen.GetPaymentRow, allocs []PaymentAllocation) Payment {
	p := Payment{
		ID: db.UUIDString(r.ID), ReceiptNo: receiptNumber(int(r.FiscalYear), r.ReceiptNo), FiscalYear: int(r.FiscalYear),
		FamilyID: db.UUIDString(r.FamilyID), Amount: money.Money(r.Amount), Method: r.Method, AccountCode: r.AccountCode,
		Advance: money.Money(r.Advance), AdvanceApplied: money.Money(r.AdvanceApplied), AdvanceRefunded: money.Money(r.AdvanceRefunded),
		ReceivedOn: r.ReceivedOn.Time, Status: r.Status, Allocations: allocs, EntryID: db.UUIDString(r.JournalEntryID),
		CreatedAt: r.CreatedAt.Time,
	}
	if p.Allocations == nil {
		p.Allocations = []PaymentAllocation{}
	}
	if r.Status == "recorded" {
		p.AdvanceAvailable = p.Advance - p.AdvanceApplied - p.AdvanceRefunded
	}
	p.Refunds = []AdvanceRefund{}
	if r.Reference.Valid {
		p.Reference = &r.Reference.String
	}
	if r.VoidReason.Valid {
		p.VoidReason = &r.VoidReason.String
	}
	if r.VoidedAt.Valid {
		t := r.VoidedAt.Time
		p.VoidedAt = &t
	}
	return p
}
