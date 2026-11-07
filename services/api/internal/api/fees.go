package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// Handlers for fee setup. Same shape as every handler: check the permission, call the service,
// map the result. No money is posted from here (ADR-0014).

// CreateFeeType adds a fee type.
func (s *Server) CreateFeeType(ctx context.Context, req gen.CreateFeeTypeRequestObject) (gen.CreateFeeTypeResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeSettings)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	created, err := s.fees.CreateType(ctx, p, fees.FeeTypeInput{Code: req.Body.Code, Name: req.Body.Name, Kind: string(req.Body.Kind)})
	if err != nil {
		return nil, err
	}
	out, err := toAPIFeeType(created)
	if err != nil {
		return nil, err
	}
	return gen.CreateFeeType201JSONResponse(out), nil
}

// ListFeeTypes lists the school's fee types, creating the suggested set the first time.
func (s *Server) ListFeeTypes(ctx context.Context, _ gen.ListFeeTypesRequestObject) (gen.ListFeeTypesResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	list, err := s.fees.ListTypes(ctx, p)
	if err != nil {
		return nil, err
	}
	items := make([]gen.FeeType, 0, len(list))
	for _, t := range list {
		out, err := toAPIFeeType(t)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListFeeTypes200JSONResponse{Items: items}, nil
}

// ArchiveFeeType archives a fee type.
func (s *Server) ArchiveFeeType(ctx context.Context, req gen.ArchiveFeeTypeRequestObject) (gen.ArchiveFeeTypeResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeSettings)
	if err != nil {
		return nil, err
	}
	archived, err := s.fees.ArchiveType(ctx, p, req.FeeTypeId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIFeeType(archived)
	if err != nil {
		return nil, err
	}
	return gen.ArchiveFeeType200JSONResponse(out), nil
}

// SetFeePlanLine adds a version to a class plan.
func (s *Server) SetFeePlanLine(ctx context.Context, req gen.SetFeePlanLineRequestObject) (gen.SetFeePlanLineResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeePlanManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	amount, err := fromAPIMoney(req.Body.Amount)
	if err != nil {
		return nil, err
	}
	in := fees.PlanLineInput{
		FeeTypeID:     req.Body.FeeTypeId.String(),
		EffectiveFrom: req.Body.EffectiveFrom.Time,
		Remove:        req.Body.Remove != nil && *req.Body.Remove,
		Amount:        amount,
	}
	if req.Body.Frequency != nil {
		in.Frequency = string(*req.Body.Frequency)
	}
	set, err := s.fees.SetPlanLine(ctx, p, req.ClassId.String(), in)
	if err != nil {
		return nil, err
	}
	out, err := toAPIPlanLine(set)
	if err != nil {
		return nil, err
	}
	return gen.SetFeePlanLine201JSONResponse(out), nil
}

// GetFeePlan returns a class plan as it stands in a month.
func (s *Server) GetFeePlan(ctx context.Context, req gen.GetFeePlanRequestObject) (gen.GetFeePlanResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	plan, err := s.fees.GetPlan(ctx, p, req.ClassId.String(), monthParam(req.Params.Month))
	if err != nil {
		return nil, err
	}
	lines := make([]gen.PlanLine, 0, len(plan.Lines))
	for _, l := range plan.Lines {
		out, err := toAPIPlanLine(l)
		if err != nil {
			return nil, err
		}
		lines = append(lines, out)
	}
	return gen.GetFeePlan200JSONResponse{
		ClassId: req.ClassId,
		Month:   openapitypes.Date{Time: plan.Month},
		Lines:   lines,
	}, nil
}

// GetFeePlanHistory returns every version ever set on a class plan.
func (s *Server) GetFeePlanHistory(ctx context.Context, req gen.GetFeePlanHistoryRequestObject) (gen.GetFeePlanHistoryResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	list, err := s.fees.PlanHistory(ctx, p, req.ClassId.String())
	if err != nil {
		return nil, err
	}
	items := make([]gen.PlanLine, 0, len(list))
	for _, l := range list {
		out, err := toAPIPlanLine(l)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.GetFeePlanHistory200JSONResponse{Items: items}, nil
}

// SetFeeOverride adds a version to a student's overrides.
func (s *Server) SetFeeOverride(ctx context.Context, req gen.SetFeeOverrideRequestObject) (gen.SetFeeOverrideResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeePlanManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	amount, err := fromAPIMoney(req.Body.Amount)
	if err != nil {
		return nil, err
	}
	set, err := s.fees.SetOverride(ctx, p, req.StudentId.String(), fees.OverrideInput{
		FeeTypeID:     req.Body.FeeTypeId.String(),
		EffectiveFrom: req.Body.EffectiveFrom.Time,
		Reason:        req.Body.Reason,
		Clear:         req.Body.Clear != nil && *req.Body.Clear,
		Amount:        amount,
	})
	if err != nil {
		return nil, err
	}
	out, err := toAPIOverride(set)
	if err != nil {
		return nil, err
	}
	return gen.SetFeeOverride201JSONResponse(out), nil
}

// ListFeeOverrides returns every override version of a student.
func (s *Server) ListFeeOverrides(ctx context.Context, req gen.ListFeeOverridesRequestObject) (gen.ListFeeOverridesResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	list, err := s.fees.ListOverrides(ctx, p, req.StudentId.String())
	if err != nil {
		return nil, err
	}
	items := make([]gen.FeeOverride, 0, len(list))
	for _, o := range list {
		out, err := toAPIOverride(o)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListFeeOverrides200JSONResponse{Items: items}, nil
}

// GetFeeStructure returns what a student is charged for a month.
func (s *Server) GetFeeStructure(ctx context.Context, req gen.GetFeeStructureRequestObject) (gen.GetFeeStructureResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	st, err := s.fees.FeeStructure(ctx, p, req.StudentId.String(), monthParam(req.Params.Month))
	if err != nil {
		return nil, err
	}
	classID, err := uuid.Parse(st.ClassID)
	if err != nil {
		return nil, err
	}
	lines := make([]gen.FeeLine, 0, len(st.Lines))
	for _, l := range st.Lines {
		id, err := uuid.Parse(l.FeeTypeID)
		if err != nil {
			return nil, err
		}
		lines = append(lines, gen.FeeLine{
			FeeTypeId: id, FeeTypeName: l.Name, Kind: gen.FeeKind(l.Kind),
			Amount: toAPIMoney(l.Amount), PlanAmount: toAPIMoney(l.PlanAmount),
			Frequency: gen.FeeFrequency(l.Frequency), Source: gen.FeeLineSource(l.Source),
		})
	}
	return gen.GetFeeStructure200JSONResponse{
		StudentId: req.StudentId, ClassId: classID, Month: openapitypes.Date{Time: st.Month}, Lines: lines,
	}, nil
}

// ---- mapping --------------------------------------------------------------------------

// fromAPIMoney reads an incoming {amount, currency}. Only PKR exists in 1.0; any other currency is
// refused rather than silently treated as rupees. A missing amount stays nil so the rules can say so.
func fromAPIMoney(m *gen.Money) (*money.Money, error) {
	if m == nil {
		return nil, nil
	}
	if m.Currency != gen.MoneyCurrency(money.Currency) {
		return nil, apperr.Validation("money.currency_invalid", "Only PKR amounts are supported.",
			apperr.Field{Field: "amount.currency", Message: "Use PKR."})
	}
	v := money.Money(m.Amount)
	return &v, nil
}

// monthParam returns the zero time when the optional month is absent; the service then uses the
// current month in Pakistan.
func monthParam(d *openapitypes.Date) time.Time {
	if d == nil {
		return time.Time{}
	}
	return d.Time
}

func toAPIFeeType(t fees.FeeType) (gen.FeeType, error) {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return gen.FeeType{}, err
	}
	return gen.FeeType{
		Id: id, Code: t.Code, Name: t.Name, Kind: gen.FeeKind(t.Kind),
		IsDeposit: t.IsDeposit, Status: gen.FeeTypeStatus(t.Status),
	}, nil
}

// toAPIPlanLine omits amount, frequency and kind for a removal version, which has no amount.
func toAPIPlanLine(l fees.PlanLine) (gen.PlanLine, error) {
	id, err := uuid.Parse(l.FeeTypeID)
	if err != nil {
		return gen.PlanLine{}, err
	}
	out := gen.PlanLine{
		FeeTypeId: id, FeeTypeName: l.FeeTypeName,
		EffectiveFrom: openapitypes.Date{Time: l.EffectiveFrom}, Removed: l.Removed,
	}
	if l.Kind != "" {
		kind := gen.FeeKind(l.Kind)
		out.Kind = &kind
	}
	if !l.Removed {
		amount, freq := toAPIMoney(l.Amount), gen.FeeFrequency(l.Frequency)
		out.Amount, out.Frequency = &amount, &freq
	}
	return out, nil
}

func toAPIOverride(o fees.Override) (gen.FeeOverride, error) {
	id, err := uuid.Parse(o.FeeTypeID)
	if err != nil {
		return gen.FeeOverride{}, err
	}
	out := gen.FeeOverride{
		FeeTypeId: id, FeeTypeName: o.FeeTypeName, EffectiveFrom: openapitypes.Date{Time: o.EffectiveFrom},
		Cleared: o.Cleared, Reason: o.Reason, CreatedAt: o.CreatedAt,
	}
	if !o.Cleared {
		amount := toAPIMoney(o.Amount)
		out.Amount = &amount
	}
	return out, nil
}
