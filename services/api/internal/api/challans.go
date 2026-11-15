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

// Handlers for challans. Same shape as every handler: check the permission, call the service, map
// the result. The posting to the ledger happens inside fees.Release, not here (ADR-0014, ADR-0017).

// GenerateChallans drafts the month's challans, or previews them.
func (s *Server) GenerateChallans(ctx context.Context, req gen.GenerateChallansRequestObject) (gen.GenerateChallansResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeGenerate)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	in := fees.GenerateInput{Month: req.Body.Month.Time, DryRun: req.Body.DryRun != nil && *req.Body.DryRun}
	if req.Body.DueDate != nil {
		d := req.Body.DueDate.Time
		in.DueDate = &d
	}
	res, err := s.fees.Generate(ctx, p, in)
	if err != nil {
		return nil, err
	}
	items := make([]gen.GenerateItem, 0, len(res.Items))
	for _, it := range res.Items {
		studentID, err := uuid.Parse(it.StudentID)
		if err != nil {
			return nil, err
		}
		item := gen.GenerateItem{
			StudentId: studentID, StudentName: it.StudentName, RegistrationNo: it.RegistrationNo,
			Outcome: gen.GenerateOutcome(it.Outcome), NewAdmission: it.NewAdmission, Total: toAPIMoney(it.Total),
		}
		if it.ChallanID != "" {
			cid, err := uuid.Parse(it.ChallanID)
			if err != nil {
				return nil, err
			}
			item.ChallanId = &cid
		}
		items = append(items, item)
	}
	return gen.GenerateChallans200JSONResponse{
		Month: openapitypes.Date{Time: res.Month}, DueDate: openapitypes.Date{Time: res.DueDate},
		DryRun: res.DryRun, Generated: res.Generated, Items: items,
	}, nil
}

// ReleaseChallans releases drafts in bulk.
func (s *Server) ReleaseChallans(ctx context.Context, req gen.ReleaseChallansRequestObject) (gen.ReleaseChallansResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeRelease)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	ids := make([]string, 0, len(req.Body.ChallanIds))
	for _, id := range req.Body.ChallanIds {
		ids = append(ids, id.String())
	}
	released, err := s.fees.Release(ctx, p, ids)
	if err != nil {
		return nil, err
	}
	items, err := toAPIChallans(released)
	if err != nil {
		return nil, err
	}
	return gen.ReleaseChallans200JSONResponse{Items: items}, nil
}

// ListChallans lists challans, newest first.
func (s *Server) ListChallans(ctx context.Context, req gen.ListChallansRequestObject) (gen.ListChallansResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	f := fees.ChallanFilter{Month: monthParam(req.Params.Month)}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.StudentId != nil {
		f.StudentID = req.Params.StudentId.String()
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		f.Cursor = *req.Params.Cursor
	}
	page, err := s.fees.ListChallans(ctx, p, f)
	if err != nil {
		return nil, err
	}
	items, err := toAPIChallans(page.Items)
	if err != nil {
		return nil, err
	}
	out := gen.ListChallans200JSONResponse{Items: items}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

// GetChallan returns one challan.
func (s *Server) GetChallan(ctx context.Context, req gen.GetChallanRequestObject) (gen.GetChallanResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeView)
	if err != nil {
		return nil, err
	}
	c, err := s.fees.GetChallan(ctx, p, req.ChallanId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIChallan(c)
	if err != nil {
		return nil, err
	}
	return gen.GetChallan200JSONResponse(out), nil
}

// VoidChallan voids a draft.
func (s *Server) VoidChallan(ctx context.Context, req gen.VoidChallanRequestObject) (gen.VoidChallanResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermFeeGenerate)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	c, err := s.fees.Void(ctx, p, req.ChallanId.String(), req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out, err := toAPIChallan(c)
	if err != nil {
		return nil, err
	}
	return gen.VoidChallan200JSONResponse(out), nil
}

func toAPIChallans(list []fees.Challan) ([]gen.Challan, error) {
	out := make([]gen.Challan, 0, len(list))
	for _, c := range list {
		item, err := toAPIChallan(c)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func toAPIChallan(c fees.Challan) (gen.Challan, error) {
	id, err := uuid.Parse(c.ID)
	if err != nil {
		return gen.Challan{}, err
	}
	studentID, err := uuid.Parse(c.StudentID)
	if err != nil {
		return gen.Challan{}, err
	}
	familyID, err := uuid.Parse(c.FamilyID)
	if err != nil {
		return gen.Challan{}, err
	}
	lines := make([]gen.ChallanLine, 0, len(c.Lines))
	for _, l := range c.Lines {
		feeTypeID, err := uuid.Parse(l.FeeTypeID)
		if err != nil {
			return gen.Challan{}, err
		}
		lines = append(lines, gen.ChallanLine{
			FeeTypeId: feeTypeID, FeeTypeName: l.FeeTypeName, Kind: gen.FeeKind(l.Kind),
			Frequency: gen.FeeFrequency(l.Frequency), Amount: toAPIMoney(l.Amount),
		})
	}
	return gen.Challan{
		Id: id, StudentId: studentID, FamilyId: familyID,
		Month: openapitypes.Date{Time: c.Month}, DueDate: openapitypes.Date{Time: c.DueDate},
		Status: gen.ChallanStatus(c.Status), Total: toAPIMoney(c.Total),
		ChallanNo: c.ChallanNo, ConsumerNo: c.ConsumerNo, Lines: lines,
		ReleasedAt: c.ReleasedAt, VoidReason: c.VoidReason, CreatedAt: c.CreatedAt,
	}, nil
}
