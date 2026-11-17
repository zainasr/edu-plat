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

// Handlers for attendance registers (ADR-0022). Same shape as every handler: check the permission, call the
// service, map the result.

// GetAttendanceSettings returns the school's attendance settings.
func (s *Server) GetAttendanceSettings(ctx context.Context, _ gen.GetAttendanceSettingsRequestObject) (gen.GetAttendanceSettingsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	cfg, err := s.attendance.GetSettings(ctx, p)
	if err != nil {
		return nil, err
	}
	return gen.GetAttendanceSettings200JSONResponse(toAPISettings(cfg)), nil
}

// SetAttendanceSettings changes them.
func (s *Server) SetAttendanceSettings(ctx context.Context, req gen.SetAttendanceSettingsRequestObject) (gen.SetAttendanceSettingsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceSettings)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	cfg, err := s.attendance.SetSettings(ctx, p, attendance.SettingsInput{
		RegisterDueTime: req.Body.RegisterDueTime, EditWindowDays: req.Body.EditWindowDays, LowAttendancePercent: req.Body.LowAttendancePercent,
	})
	if err != nil {
		return nil, err
	}
	return gen.SetAttendanceSettings200JSONResponse(toAPISettings(cfg)), nil
}

// OpenRegister opens (or returns) a section's register for a date.
func (s *Server) OpenRegister(ctx context.Context, req gen.OpenRegisterRequestObject) (gen.OpenRegisterResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceMark)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	reg, err := s.attendance.OpenRegister(ctx, p, req.Body.SectionId.String(), req.Body.Date.Time)
	if err != nil {
		return nil, err
	}
	out, err := toAPIRegister(reg)
	if err != nil {
		return nil, err
	}
	return gen.OpenRegister200JSONResponse(out), nil
}

// ListRegisters lists the registers of a date.
func (s *Server) ListRegisters(ctx context.Context, req gen.ListRegistersRequestObject) (gen.ListRegistersResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	list, err := s.attendance.ListRegisters(ctx, p, dateParam(req.Params.Date))
	if err != nil {
		return nil, err
	}
	items := make([]gen.RegisterBrief, 0, len(list))
	for _, r := range list {
		b, err := toAPIRegisterBrief(r)
		if err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return gen.ListRegisters200JSONResponse{Items: items}, nil
}

// GetRegister returns one register.
func (s *Server) GetRegister(ctx context.Context, req gen.GetRegisterRequestObject) (gen.GetRegisterResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	reg, err := s.attendance.GetRegister(ctx, p, req.RegisterId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIRegister(reg)
	if err != nil {
		return nil, err
	}
	return gen.GetRegister200JSONResponse(out), nil
}

// MarkRecords marks students in an open register.
func (s *Server) MarkRecords(ctx context.Context, req gen.MarkRecordsRequestObject) (gen.MarkRecordsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceMark)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	marks := make([]attendance.Mark, 0, len(req.Body.Marks))
	for _, m := range req.Body.Marks {
		marks = append(marks, attendance.Mark{StudentID: m.StudentId.String(), Status: string(m.Status)})
	}
	reg, err := s.attendance.MarkRecords(ctx, p, req.RegisterId.String(), marks)
	if err != nil {
		return nil, err
	}
	out, err := toAPIRegister(reg)
	if err != nil {
		return nil, err
	}
	return gen.MarkRecords200JSONResponse(out), nil
}

// SubmitRegister submits a register.
func (s *Server) SubmitRegister(ctx context.Context, req gen.SubmitRegisterRequestObject) (gen.SubmitRegisterResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceMark)
	if err != nil {
		return nil, err
	}
	reg, err := s.attendance.SubmitRegister(ctx, p, req.RegisterId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIRegister(reg)
	if err != nil {
		return nil, err
	}
	return gen.SubmitRegister200JSONResponse(out), nil
}

// CorrectRecord corrects a submitted register. Either correction permission opens the endpoint; the
// service decides how far back the caller may go.
func (s *Server) CorrectRecord(ctx context.Context, req gen.CorrectRecordRequestObject) (gen.CorrectRecordResponseObject, error) {
	p, err := iam.RequireAny(ctx, iam.PermAttendanceEditWindow, iam.PermAttendanceEditAny)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	reg, err := s.attendance.CorrectRecord(ctx, p, req.RegisterId.String(), attendance.Correction{
		StudentID: req.Body.StudentId.String(), Status: string(req.Body.Status), Reason: req.Body.Reason,
	})
	if err != nil {
		return nil, err
	}
	out, err := toAPIRegister(reg)
	if err != nil {
		return nil, err
	}
	return gen.CorrectRecord200JSONResponse(out), nil
}

// ListMissingRegisters lists sections with no submitted register once the due time has passed.
func (s *Server) ListMissingRegisters(ctx context.Context, req gen.ListMissingRegistersRequestObject) (gen.ListMissingRegistersResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	m, err := s.attendance.MissingRegisters(ctx, p, dateParam(req.Params.Date))
	if err != nil {
		return nil, err
	}
	items := make([]gen.MissingRegister, 0, len(m.Items))
	for _, it := range m.Items {
		sec, err := uuid.Parse(it.SectionID)
		if err != nil {
			return nil, err
		}
		open := it.HasOpenRegister
		items = append(items, gen.MissingRegister{SectionId: sec, SectionName: it.SectionName, HasOpenRegister: &open})
	}
	return gen.ListMissingRegisters200JSONResponse{
		Date: openapitypes.Date{Time: m.Date}, SchoolDay: m.SchoolDay, DueTime: m.DueTime, Overdue: m.Overdue, Items: items,
	}, nil
}

// GetAttendanceSummary adds up a day.
func (s *Server) GetAttendanceSummary(ctx context.Context, req gen.GetAttendanceSummaryRequestObject) (gen.GetAttendanceSummaryResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	d, err := s.attendance.DailySummary(ctx, p, dateParam(req.Params.Date))
	if err != nil {
		return nil, err
	}
	secs := make([]gen.SectionDay, 0, len(d.Sections))
	for _, sd := range d.Sections {
		sec, err := uuid.Parse(sd.SectionID)
		if err != nil {
			return nil, err
		}
		secs = append(secs, gen.SectionDay{SectionId: sec, SectionName: sd.SectionName, Submitted: sd.Submitted, Counts: toAPICounts(sd.Counts)})
	}
	return gen.GetAttendanceSummary200JSONResponse{Date: openapitypes.Date{Time: d.Date}, SchoolDay: d.SchoolDay, Totals: toAPICounts(d.Totals), Sections: secs}, nil
}

// ListAbsentToday lists the students absent on a date.
func (s *Server) ListAbsentToday(ctx context.Context, req gen.ListAbsentTodayRequestObject) (gen.ListAbsentTodayResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	list, err := s.attendance.AbsentOnDay(ctx, p, dateParam(req.Params.Date))
	if err != nil {
		return nil, err
	}
	items := make([]gen.AbsentStudent, 0, len(list))
	for _, a := range list {
		stu, err := uuid.Parse(a.StudentID)
		if err != nil {
			return nil, err
		}
		sec, err := uuid.Parse(a.SectionID)
		if err != nil {
			return nil, err
		}
		item := gen.AbsentStudent{StudentId: stu, RegistrationNo: a.RegistrationNo, FullName: a.FullName, SectionId: sec, SectionName: a.SectionName}
		if a.Contact != nil {
			item.Contact = &gen.AbsentContact{Name: a.Contact.Name, Mobile: a.Contact.Mobile, Relationship: a.Contact.Relationship}
		}
		items = append(items, item)
	}
	return gen.ListAbsentToday200JSONResponse{Date: openapitypes.Date{Time: dateParam(req.Params.Date)}, Items: items}, nil
}

// ListLowAttendance lists students below the threshold.
func (s *Server) ListLowAttendance(ctx context.Context, req gen.ListLowAttendanceRequestObject) (gen.ListLowAttendanceResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	threshold := 0
	if req.Params.Threshold != nil {
		threshold = *req.Params.Threshold
	}
	rep, err := s.attendance.LowAttendance(ctx, p, dateParam(req.Params.From), dateParam(req.Params.To), threshold)
	if err != nil {
		return nil, err
	}
	items := make([]gen.LowAttendanceItem, 0, len(rep.Items))
	for _, it := range rep.Items {
		stu, err := uuid.Parse(it.StudentID)
		if err != nil {
			return nil, err
		}
		items = append(items, gen.LowAttendanceItem{StudentId: stu, RegistrationNo: it.RegistrationNo, FullName: it.FullName, Totals: toAPITotals(it.Totals)})
	}
	return gen.ListLowAttendance200JSONResponse{
		From: openapitypes.Date{Time: rep.From}, To: openapitypes.Date{Time: rep.To}, Threshold: rep.Threshold, Items: items,
	}, nil
}

// GetStudentAttendance returns one student's attendance over a period.
func (s *Server) GetStudentAttendance(ctx context.Context, req gen.GetStudentAttendanceRequestObject) (gen.GetStudentAttendanceResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermAttendanceView)
	if err != nil {
		return nil, err
	}
	h, err := s.attendance.StudentHistory(ctx, p, req.StudentId.String(), dateParam(req.Params.From), dateParam(req.Params.To))
	if err != nil {
		return nil, err
	}
	days := make([]gen.StudentDay, 0, len(h.Days))
	for _, d := range h.Days {
		sec, err := uuid.Parse(d.SectionID)
		if err != nil {
			return nil, err
		}
		days = append(days, gen.StudentDay{Date: openapitypes.Date{Time: d.Date}, SectionId: sec, Status: gen.AttendanceStatus(d.Status), Method: d.Method, Corrected: d.Corrected})
	}
	return gen.GetStudentAttendance200JSONResponse{
		StudentId: req.StudentId, From: openapitypes.Date{Time: h.From}, To: openapitypes.Date{Time: h.To}, Totals: toAPITotals(h.Totals), Days: days,
	}, nil
}

func toAPISettings(c attendance.Settings) gen.AttendanceSettings {
	return gen.AttendanceSettings{RegisterDueTime: c.RegisterDueTime, EditWindowDays: c.EditWindowDays, LowAttendancePercent: c.LowAttendancePercent}
}

func toAPICounts(c attendance.Counts) gen.RegisterCounts {
	return gen.RegisterCounts{Students: c.Students, Present: c.Present, Absent: c.Absent, Late: c.Late, Leave: c.Leave}
}

func toAPITotals(t attendance.Totals) gen.AttendanceTotals {
	return gen.AttendanceTotals{Present: t.Present, Late: t.Late, Absent: t.Absent, Leave: t.Leave, ExpectedDays: t.ExpectedDays, Percent: float32(t.Percent)}
}

func toAPIRegisterBrief(r attendance.Register) (gen.RegisterBrief, error) {
	id, err := uuid.Parse(r.ID)
	if err != nil {
		return gen.RegisterBrief{}, err
	}
	sec, err := uuid.Parse(r.SectionID)
	if err != nil {
		return gen.RegisterBrief{}, err
	}
	return gen.RegisterBrief{
		Id: id, SectionId: sec, SectionName: r.SectionName, Date: openapitypes.Date{Time: r.Date},
		Status: gen.RegisterStatus(r.Status), SubmittedAt: r.SubmittedAt, Counts: toAPICounts(r.Counts),
	}, nil
}

func toAPIRegister(r attendance.Register) (gen.Register, error) {
	id, err := uuid.Parse(r.ID)
	if err != nil {
		return gen.Register{}, err
	}
	sec, err := uuid.Parse(r.SectionID)
	if err != nil {
		return gen.Register{}, err
	}
	name := r.SectionName
	records := make([]gen.AttendanceRecord, 0, len(r.Records))
	for _, rec := range r.Records {
		stu, err := uuid.Parse(rec.StudentID)
		if err != nil {
			return gen.Register{}, err
		}
		records = append(records, gen.AttendanceRecord{
			StudentId: stu, RegistrationNo: rec.RegistrationNo, FullName: rec.FullName, Status: gen.AttendanceStatus(rec.Status),
			Method: gen.AttendanceRecordMethod(rec.Method), Version: rec.Version, Corrected: rec.Corrected,
		})
	}
	return gen.Register{
		Id: id, SectionId: sec, SectionName: &name, Date: openapitypes.Date{Time: r.Date}, Status: gen.RegisterStatus(r.Status),
		SubmittedAt: r.SubmittedAt, Counts: toAPICounts(r.Counts), Records: records,
	}, nil
}
