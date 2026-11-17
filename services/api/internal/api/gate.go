package api

import (
	"context"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/attendance"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// Handlers for student ID cards and the gate scan (ADR-0023). Same shape as every handler: check the
// permission, call the service, map the result.

// IssueStudentCard issues a card, disabling the student's previous one.
func (s *Server) IssueStudentCard(ctx context.Context, req gen.IssueStudentCardRequestObject) (gen.IssueStudentCardResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentEdit)
	if err != nil {
		return nil, err
	}
	c, err := s.attendance.IssueCard(ctx, p, req.StudentId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPICard(c)
	if err != nil {
		return nil, err
	}
	return gen.IssueStudentCard201JSONResponse(out), nil
}

// ListStudentCards lists a student's cards.
func (s *Server) ListStudentCards(ctx context.Context, req gen.ListStudentCardsRequestObject) (gen.ListStudentCardsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentEdit)
	if err != nil {
		return nil, err
	}
	list, err := s.attendance.ListCards(ctx, p, req.StudentId.String())
	if err != nil {
		return nil, err
	}
	items := make([]gen.StudentCard, 0, len(list))
	for _, c := range list {
		item, err := toAPICard(c)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return gen.ListStudentCards200JSONResponse{Items: items}, nil
}

// DisableStudentCard disables a card.
func (s *Server) DisableStudentCard(ctx context.Context, req gen.DisableStudentCardRequestObject) (gen.DisableStudentCardResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentEdit)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	c, err := s.attendance.DisableCard(ctx, p, req.CardId.String(), req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out, err := toAPICard(c)
	if err != nil {
		return nil, err
	}
	return gen.DisableStudentCard200JSONResponse(out), nil
}

// ScanCard records an arrival from a scanned card.
func (s *Server) ScanCard(ctx context.Context, req gen.ScanCardRequestObject) (gen.ScanCardResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceScan)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	in := attendance.ScanInput{Code: req.Body.Code, ScannedAt: req.Body.ScannedAt}
	if req.Body.ClientScanId != nil {
		in.ClientScanID = req.Body.ClientScanId.String()
	}
	r, err := s.attendance.Scan(ctx, p, in)
	if err != nil {
		return nil, err
	}
	stu, err := uuid.Parse(r.Student.ID)
	if err != nil {
		return nil, err
	}
	return gen.ScanCard200JSONResponse{
		Outcome: gen.ScanResultOutcome(r.Outcome), Status: gen.ScanResultStatus(r.Status), ScannedAt: r.ScannedAt, ArrivalAt: r.ArrivalAt,
		Student: gen.ScanStudent{Id: stu, RegistrationNo: r.Student.RegistrationNo, FullName: r.Student.FullName, SectionName: r.Student.SectionName},
	}, nil
}

// ListGateScans lists a day's scans.
func (s *Server) ListGateScans(ctx context.Context, req gen.ListGateScansRequestObject) (gen.ListGateScansResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	date := dateParam(req.Params.Date)
	list, err := s.attendance.GateScans(ctx, p, date)
	if err != nil {
		return nil, err
	}
	items := make([]gen.GateScan, 0, len(list))
	for _, g := range list {
		id, err := uuid.Parse(g.ID)
		if err != nil {
			return nil, err
		}
		stu, err := uuid.Parse(g.StudentID)
		if err != nil {
			return nil, err
		}
		sec, err := uuid.Parse(g.SectionID)
		if err != nil {
			return nil, err
		}
		items = append(items, gen.GateScan{
			Id: id, StudentId: stu, RegistrationNo: g.RegistrationNo, FullName: g.FullName, SectionId: sec, SectionName: g.SectionName,
			ScannedAt: g.ScannedAt, Status: gen.GateScanStatus(g.Status), IsArrival: g.IsArrival,
		})
	}
	return gen.ListGateScans200JSONResponse{Date: openapitypes.Date{Time: date}, Items: items}, nil
}

// ListAttendanceMismatches lists where the gate and the class register disagree.
func (s *Server) ListAttendanceMismatches(ctx context.Context, req gen.ListAttendanceMismatchesRequestObject) (gen.ListAttendanceMismatchesResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	date := dateParam(req.Params.Date)
	list, err := s.attendance.Mismatches(ctx, p, date)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Mismatch, 0, len(list))
	for _, m := range list {
		stu, err := uuid.Parse(m.StudentID)
		if err != nil {
			return nil, err
		}
		sec, err := uuid.Parse(m.SectionID)
		if err != nil {
			return nil, err
		}
		items = append(items, gen.Mismatch{
			StudentId: stu, RegistrationNo: m.RegistrationNo, FullName: m.FullName, SectionId: sec, SectionName: m.SectionName,
			Kind: gen.MismatchKind(m.Kind), RegisterStatus: gen.AttendanceStatus(m.RegisterStatus), GateArrivedAt: m.GateArrivedAt,
		})
	}
	return gen.ListAttendanceMismatches200JSONResponse{Date: openapitypes.Date{Time: date}, Items: items}, nil
}

func toAPICard(c attendance.Card) (gen.StudentCard, error) {
	id, err := uuid.Parse(c.ID)
	if err != nil {
		return gen.StudentCard{}, err
	}
	stu, err := uuid.Parse(c.StudentID)
	if err != nil {
		return gen.StudentCard{}, err
	}
	return gen.StudentCard{
		Id: id, StudentId: stu, Status: gen.StudentCardStatus(c.Status), Code: c.Code, IssuedAt: c.IssuedAt,
		DisabledAt: c.DisabledAt, DisabledReason: c.DisabledReason,
	}, nil
}
