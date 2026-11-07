package api

import (
	"context"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// Handlers for the academic structure and enrolment history. Same shape as every handler:
// check the permission, call the service, map the result.

// CreateSession adds an academic session.
func (s *Server) CreateSession(ctx context.Context, req gen.CreateSessionRequestObject) (gen.CreateSessionResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermSessionManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	created, err := s.org.CreateSession(ctx, p, org.SessionInput{
		Name:     req.Body.Name,
		StartsOn: req.Body.StartsOn.Time,
		EndsOn:   req.Body.EndsOn.Time,
	})
	if err != nil {
		return nil, err
	}
	out, err := toAPISession(created)
	if err != nil {
		return nil, err
	}
	return gen.CreateSession201JSONResponse(out), nil
}

// ListSessions lists the branch's sessions.
func (s *Server) ListSessions(ctx context.Context, _ gen.ListSessionsRequestObject) (gen.ListSessionsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgView)
	if err != nil {
		return nil, err
	}
	list, err := s.org.ListSessions(ctx, p)
	if err != nil {
		return nil, err
	}
	items := make([]gen.AcademicSession, 0, len(list))
	for _, x := range list {
		out, err := toAPISession(x)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListSessions200JSONResponse{Items: items}, nil
}

// MakeSessionCurrent switches the current session.
func (s *Server) MakeSessionCurrent(ctx context.Context, req gen.MakeSessionCurrentRequestObject) (gen.MakeSessionCurrentResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermSessionManage)
	if err != nil {
		return nil, err
	}
	made, err := s.org.MakeSessionCurrent(ctx, p, req.SessionId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPISession(made)
	if err != nil {
		return nil, err
	}
	return gen.MakeSessionCurrent200JSONResponse(out), nil
}

// CreateClass adds a class.
func (s *Server) CreateClass(ctx context.Context, req gen.CreateClassRequestObject) (gen.CreateClassResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	created, err := s.org.CreateClass(ctx, p, org.ClassInput{Name: req.Body.Name, GradeLevel: req.Body.GradeLevel})
	if err != nil {
		return nil, err
	}
	out, err := toAPIClass(created)
	if err != nil {
		return nil, err
	}
	return gen.CreateClass201JSONResponse(out), nil
}

// ListClasses lists the branch's classes.
func (s *Server) ListClasses(ctx context.Context, _ gen.ListClassesRequestObject) (gen.ListClassesResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgView)
	if err != nil {
		return nil, err
	}
	list, err := s.org.ListClasses(ctx, p)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Class, 0, len(list))
	for _, x := range list {
		out, err := toAPIClass(x)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListClasses200JSONResponse{Items: items}, nil
}

// ArchiveClass archives a class.
func (s *Server) ArchiveClass(ctx context.Context, req gen.ArchiveClassRequestObject) (gen.ArchiveClassResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgManage)
	if err != nil {
		return nil, err
	}
	archived, err := s.org.ArchiveClass(ctx, p, req.ClassId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIClass(archived)
	if err != nil {
		return nil, err
	}
	return gen.ArchiveClass200JSONResponse(out), nil
}

// CreateSection adds a section to a class.
func (s *Server) CreateSection(ctx context.Context, req gen.CreateSectionRequestObject) (gen.CreateSectionResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	created, err := s.org.CreateSection(ctx, p, org.SectionInput{
		ClassID: req.Body.ClassId.String(),
		Name:    req.Body.Name,
		Shift:   string(req.Body.Shift),
	})
	if err != nil {
		return nil, err
	}
	out, err := toAPISection(created)
	if err != nil {
		return nil, err
	}
	return gen.CreateSection201JSONResponse(out), nil
}

// ListSections lists sections, optionally of one class.
func (s *Server) ListSections(ctx context.Context, req gen.ListSectionsRequestObject) (gen.ListSectionsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgView)
	if err != nil {
		return nil, err
	}
	classID := ""
	if req.Params.ClassId != nil {
		classID = req.Params.ClassId.String()
	}
	list, err := s.org.ListSections(ctx, p, classID)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Section, 0, len(list))
	for _, x := range list {
		out, err := toAPISection(x)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListSections200JSONResponse{Items: items}, nil
}

// ArchiveSection archives a section.
func (s *Server) ArchiveSection(ctx context.Context, req gen.ArchiveSectionRequestObject) (gen.ArchiveSectionResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgManage)
	if err != nil {
		return nil, err
	}
	archived, err := s.org.ArchiveSection(ctx, p, req.SectionId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPISection(archived)
	if err != nil {
		return nil, err
	}
	return gen.ArchiveSection200JSONResponse(out), nil
}

// ChangeStudentSection moves a student to another section, keeping the history.
func (s *Server) ChangeStudentSection(ctx context.Context, req gen.ChangeStudentSectionRequestObject) (gen.ChangeStudentSectionResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentEdit)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	moved, err := s.students.ChangeSection(ctx, p, req.StudentId.String(), req.Body.SectionId.String(), req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out, err := toAPIStudent(moved)
	if err != nil {
		return nil, err
	}
	return gen.ChangeStudentSection200JSONResponse(out), nil
}

// ListStudentEnrolments returns the enrolment history of one student.
func (s *Server) ListStudentEnrolments(ctx context.Context, req gen.ListStudentEnrolmentsRequestObject) (gen.ListStudentEnrolmentsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentView)
	if err != nil {
		return nil, err
	}
	list, err := s.students.ListEnrolments(ctx, p, req.StudentId.String())
	if err != nil {
		return nil, err
	}
	items := make([]gen.Enrolment, 0, len(list))
	for _, e := range list {
		out, err := toAPIEnrolment(e)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListStudentEnrolments200JSONResponse{Items: items}, nil
}

// ---- mapping: service types to generated types -------------------------------------------

func toAPISession(x org.Session) (gen.AcademicSession, error) {
	id, err := uuid.Parse(x.ID)
	if err != nil {
		return gen.AcademicSession{}, err
	}
	return gen.AcademicSession{
		Id:        id,
		Name:      x.Name,
		StartsOn:  openapitypes.Date{Time: x.StartsOn},
		EndsOn:    openapitypes.Date{Time: x.EndsOn},
		IsCurrent: x.IsCurrent,
	}, nil
}

func toAPIClass(x org.Class) (gen.Class, error) {
	id, err := uuid.Parse(x.ID)
	if err != nil {
		return gen.Class{}, err
	}
	return gen.Class{Id: id, Name: x.Name, GradeLevel: x.GradeLevel, Status: gen.ClassStatus(x.Status)}, nil
}

func toAPISection(x org.Section) (gen.Section, error) {
	id, err := uuid.Parse(x.ID)
	if err != nil {
		return gen.Section{}, err
	}
	classID, err := uuid.Parse(x.ClassID)
	if err != nil {
		return gen.Section{}, err
	}
	return gen.Section{
		Id:        id,
		ClassId:   classID,
		ClassName: x.ClassName,
		Name:      x.Name,
		Shift:     gen.Shift(x.Shift),
		Status:    gen.SectionStatus(x.Status),
	}, nil
}

func toAPIEnrolment(e students.Enrolment) (gen.Enrolment, error) {
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return gen.Enrolment{}, err
	}
	sessionID, err := uuid.Parse(e.SessionID)
	if err != nil {
		return gen.Enrolment{}, err
	}
	sectionID, err := uuid.Parse(e.SectionID)
	if err != nil {
		return gen.Enrolment{}, err
	}
	out := gen.Enrolment{
		Id:          id,
		SessionId:   sessionID,
		SessionName: e.SessionName,
		SectionId:   sectionID,
		ClassName:   e.ClassName,
		SectionName: e.SectionName,
		Shift:       gen.Shift(e.Shift),
		Status:      gen.EnrolmentStatus(e.Status),
		StartedOn:   openapitypes.Date{Time: e.StartedOn},
		Reason:      e.Reason,
	}
	if e.EndedOn != nil {
		out.EndedOn = &openapitypes.Date{Time: *e.EndedOn}
	}
	return out, nil
}
