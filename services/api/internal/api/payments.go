package api

import (
	"context"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// Handlers for payments at the counter. Same shape as every handler: check the permission, call the
// service, map the result. The posting to the ledger happens inside fees (ADR-0014, ADR-0020).

// RecordPayment takes a payment from a family. It needs an Idempotency-Key: a retry must never take
// the money twice. The key is checked after the permission, so callers without the permission still get 403.
func (s *Server) RecordPayment(ctx context.Context, req gen.RecordPaymentRequestObject) (gen.RecordPaymentResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeCollect)
	if err != nil {
		return nil, err
	}
	if req.Params.IdempotencyKey == nil {
		return nil, apperr.Validation("idempotency.key_required", "Send an Idempotency-Key header: a retry must never take the money twice.",
			apperr.Field{Field: "Idempotency-Key", Message: "Required for this request."})
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	amount, err := fromAPIMoney(&req.Body.Amount)
	if err != nil {
		return nil, err
	}
	in := fees.PaymentInput{
		FamilyID: req.Body.FamilyId.String(), Amount: *amount, Method: string(req.Body.Method), AccountCode: string(req.Body.AccountCode),
	}
	if req.Body.ReceivedOn != nil {
		in.ReceivedOn = req.Body.ReceivedOn.Time
	}
	if req.Body.Reference != nil {
		in.Reference = *req.Body.Reference
	}
	in.HoldExcessAsAdvance = req.Body.HoldExcessAsAdvance != nil && *req.Body.HoldExcessAsAdvance
	if req.Body.Allocations != nil {
		for _, a := range *req.Body.Allocations {
			m, err := fromAPIMoney(&a.Amount)
			if err != nil {
				return nil, err
			}
			in.Allocations = append(in.Allocations, fees.Allocation{ChallanID: a.ChallanId.String(), Amount: *m})
		}
	}
	created, err := s.fees.RecordPayment(ctx, p, in)
	if err != nil {
		return nil, err
	}
	out, err := toAPIPayment(created)
	if err != nil {
		return nil, err
	}
	return gen.RecordPayment201JSONResponse(out), nil
}

// ListPayments lists payments, newest first.
func (s *Server) ListPayments(ctx context.Context, req gen.ListPaymentsRequestObject) (gen.ListPaymentsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	f := fees.PaymentFilter{}
	if req.Params.FamilyId != nil {
		f.FamilyID = req.Params.FamilyId.String()
	}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		f.Cursor = *req.Params.Cursor
	}
	page, err := s.fees.ListPayments(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Payment, 0, len(page.Items))
	for _, it := range page.Items {
		out, err := toAPIPayment(it)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	out := gen.ListPayments200JSONResponse{Items: items}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

// GetPayment returns one payment.
func (s *Server) GetPayment(ctx context.Context, req gen.GetPaymentRequestObject) (gen.GetPaymentResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	pay, err := s.fees.GetPayment(ctx, p, req.PaymentId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIPayment(pay)
	if err != nil {
		return nil, err
	}
	return gen.GetPayment200JSONResponse(out), nil
}

// VoidPayment voids a payment.
func (s *Server) VoidPayment(ctx context.Context, req gen.VoidPaymentRequestObject) (gen.VoidPaymentResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermPaymentVoid)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	pay, err := s.fees.VoidPayment(ctx, p, req.PaymentId.String(), req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out, err := toAPIPayment(pay)
	if err != nil {
		return nil, err
	}
	return gen.VoidPayment200JSONResponse(out), nil
}

// GetFamilyDues lists what a family owes.
func (s *Server) GetFamilyDues(ctx context.Context, req gen.GetFamilyDuesRequestObject) (gen.GetFamilyDuesResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	d, err := s.fees.GetFamilyDues(ctx, p, req.FamilyId.String())
	if err != nil {
		return nil, err
	}
	items := make([]gen.DueItem, 0, len(d.Items))
	for _, it := range d.Items {
		challan, err := uuid.Parse(it.ChallanID)
		if err != nil {
			return nil, err
		}
		student, err := uuid.Parse(it.StudentID)
		if err != nil {
			return nil, err
		}
		items = append(items, gen.DueItem{
			ChallanId: challan, StudentId: student, ChallanNo: it.ChallanNo,
			Month: openapitypes.Date{Time: it.Month}, DueDate: openapitypes.Date{Time: it.DueDate},
			Total: toAPIMoney(it.Total), Paid: toAPIMoney(it.Paid), Balance: toAPIMoney(it.Balance),
		})
	}
	return gen.GetFamilyDues200JSONResponse{FamilyId: req.FamilyId, TotalDue: toAPIMoney(d.TotalDue), AdvanceAvailable: toAPIMoney(d.AdvanceAvailable), Items: items}, nil
}

func toAPIPayment(p fees.Payment) (gen.Payment, error) {
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return gen.Payment{}, err
	}
	family, err := uuid.Parse(p.FamilyID)
	if err != nil {
		return gen.Payment{}, err
	}
	entry, err := uuid.Parse(p.EntryID)
	if err != nil {
		return gen.Payment{}, err
	}
	allocs := make([]gen.PaymentAllocation, 0, len(p.Allocations))
	for _, a := range p.Allocations {
		challan, err := uuid.Parse(a.ChallanID)
		if err != nil {
			return gen.Payment{}, err
		}
		student, err := uuid.Parse(a.StudentID)
		if err != nil {
			return gen.Payment{}, err
		}
		allocs = append(allocs, gen.PaymentAllocation{ChallanId: challan, StudentId: student, ChallanNo: a.ChallanNo, Amount: toAPIMoney(a.Amount)})
	}
	refunds := make([]gen.AdvanceRefund, 0, len(p.Refunds))
	for _, r := range p.Refunds {
		rid, err := uuid.Parse(r.ID)
		if err != nil {
			return gen.Payment{}, err
		}
		eid, err := uuid.Parse(r.EntryID)
		if err != nil {
			return gen.Payment{}, err
		}
		refunds = append(refunds, gen.AdvanceRefund{Id: rid, Amount: toAPIMoney(r.Amount), AccountCode: gen.PaymentAccount(r.AccountCode), Reason: r.Reason, EntryId: eid, CreatedAt: r.CreatedAt})
	}
	return gen.Payment{
		Advance: toAPIMoney(p.Advance), AdvanceApplied: toAPIMoney(p.AdvanceApplied), AdvanceRefunded: toAPIMoney(p.AdvanceRefunded),
		AdvanceAvailable: toAPIMoney(p.AdvanceAvailable), Refunds: refunds,
		Id: id, ReceiptNo: p.ReceiptNo, FiscalYear: p.FiscalYear, FamilyId: family, Amount: toAPIMoney(p.Amount),
		Method: gen.PaymentMethod(p.Method), AccountCode: gen.PaymentAccount(p.AccountCode), Reference: p.Reference,
		ReceivedOn: openapitypes.Date{Time: p.ReceivedOn}, Status: gen.PaymentStatus(p.Status), Allocations: allocs,
		EntryId: entry, VoidReason: p.VoidReason, VoidedAt: p.VoidedAt, CreatedAt: p.CreatedAt,
	}, nil
}

// RefundAdvance pays back advance that has not been applied.
func (s *Server) RefundAdvance(ctx context.Context, req gen.RefundAdvanceRequestObject) (gen.RefundAdvanceResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeRefund)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	amount, err := fromAPIMoney(&req.Body.Amount)
	if err != nil {
		return nil, err
	}
	pay, err := s.fees.RefundAdvance(ctx, p, req.PaymentId.String(), fees.RefundInput{
		Amount: *amount, AccountCode: string(req.Body.AccountCode), Reason: req.Body.Reason,
	})
	if err != nil {
		return nil, err
	}
	out, err := toAPIPayment(pay)
	if err != nil {
		return nil, err
	}
	return gen.RefundAdvance200JSONResponse(out), nil
}

// ListDefaulters lists the families that are overdue by the window, most overdue first.
func (s *Server) ListDefaulters(ctx context.Context, req gen.ListDefaultersRequestObject) (gen.ListDefaultersResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	f := fees.DefaulterFilter{MinDays: req.Params.MinDays}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		f.Cursor = *req.Params.Cursor
	}
	page, err := s.fees.ListDefaulters(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Defaulter, 0, len(page.Items))
	for _, d := range page.Items {
		family, err := uuid.Parse(d.FamilyID)
		if err != nil {
			return nil, err
		}
		challans := make([]gen.DueItem, 0, len(d.Challans))
		for _, it := range d.Challans {
			challan, err := uuid.Parse(it.ChallanID)
			if err != nil {
				return nil, err
			}
			student, err := uuid.Parse(it.StudentID)
			if err != nil {
				return nil, err
			}
			challans = append(challans, gen.DueItem{
				ChallanId: challan, StudentId: student, ChallanNo: it.ChallanNo,
				Month: openapitypes.Date{Time: it.Month}, DueDate: openapitypes.Date{Time: it.DueDate},
				Total: toAPIMoney(it.Total), Paid: toAPIMoney(it.Paid), Balance: toAPIMoney(it.Balance),
			})
		}
		item := gen.Defaulter{
			FamilyId: family, OverdueTotal: toAPIMoney(d.OverdueTotal), OldestDueDate: openapitypes.Date{Time: d.OldestDue},
			DaysOverdue: d.DaysOverdue, Challans: challans,
		}
		if d.FeePayer != nil {
			item.FeePayer = &gen.FeePayer{Name: d.FeePayer.Name, Mobile: d.FeePayer.Mobile, Relationship: d.FeePayer.Relationship}
		}
		items = append(items, item)
	}
	out := gen.ListDefaulters200JSONResponse{Items: items}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}
