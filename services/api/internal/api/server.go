// Package api implements the HTTP contract defined in api/openapi/openapi.yaml.
// Routes and request/response types come from the generated package gen. Handlers only
// translate: check permissions, call a service, map the result. No business rules here.
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// Server implements gen.StrictServerInterface.
type Server struct {
	iam      *iam.Service
	students *students.Service
}

// NewServer wires the services into the HTTP handlers.
func NewServer(identity *iam.Service, enrolment *students.Service) *Server {
	return &Server{iam: identity, students: enrolment}
}

// GetHealth reports that the process is up. Readiness checks (database, queue)
// are added once those dependencies exist.
func (s *Server) GetHealth(_ context.Context, _ gen.GetHealthRequestObject) (gen.GetHealthResponseObject, error) {
	return gen.GetHealth200JSONResponse{Status: "ok"}, nil
}

// RequestOtp sends a code to a known number. Unknown numbers get the same answer.
func (s *Server) RequestOtp(ctx context.Context, req gen.RequestOtpRequestObject) (gen.RequestOtpResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body with a mobile number is required.")
	}
	ttl, err := s.iam.RequestOTP(ctx, string(req.Body.Mobile))
	if err != nil {
		return nil, err
	}
	return gen.RequestOtp202JSONResponse{ExpiresInSeconds: int(ttl / time.Second)}, nil
}

// VerifyOtp exchanges a code for a session in one tenant.
func (s *Server) VerifyOtp(ctx context.Context, req gen.VerifyOtpRequestObject) (gen.VerifyOtpResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	sess, err := s.iam.VerifyOTP(ctx, string(req.Body.Mobile), req.Body.Code, req.Body.TenantSlug)
	if err != nil {
		return nil, err
	}
	tenantID, err := uuid.Parse(sess.TenantID)
	if err != nil {
		return nil, err
	}
	return gen.VerifyOtp200JSONResponse{
		Token:     sess.Token,
		ExpiresAt: sess.ExpiresAt,
		Tenant:    gen.TenantRef{Id: tenantID, Slug: sess.TenantSlug},
	}, nil
}

// CreateToken issues a personal access token for the caller.
func (s *Server) CreateToken(ctx context.Context, req gen.CreateTokenRequestObject) (gen.CreateTokenResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermTokenManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body with a name is required.")
	}
	days := 0
	if req.Body.ExpiresInDays != nil {
		days = *req.Body.ExpiresInDays
	}
	issued, err := s.iam.CreatePAT(ctx, p, req.Body.Name, days)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(issued.ID)
	if err != nil {
		return nil, err
	}
	return gen.CreateToken201JSONResponse{
		Id:        id,
		Name:      issued.Name,
		Token:     issued.Plaintext,
		ExpiresAt: issued.ExpiresAt,
	}, nil
}

// CreateStudent enrols a student with its guardians.
func (s *Server) CreateStudent(ctx context.Context, req gen.CreateStudentRequestObject) (gen.CreateStudentResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentCreate)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	in := students.StudentInput{
		FullName:    req.Body.FullName,
		FatherName:  req.Body.FatherName,
		Gender:      string(req.Body.Gender),
		DateOfBirth: req.Body.DateOfBirth.Time,
	}
	for _, g := range req.Body.Guardians {
		in.Guardians = append(in.Guardians, guardianInput(g))
	}
	created, err := s.students.Create(ctx, p, in)
	if err != nil {
		return nil, err
	}
	out, err := toAPIStudent(created)
	if err != nil {
		return nil, err
	}
	return gen.CreateStudent201JSONResponse(out), nil
}

// ListStudents returns one page of students.
func (s *Server) ListStudents(ctx context.Context, req gen.ListStudentsRequestObject) (gen.ListStudentsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentView)
	if err != nil {
		return nil, err
	}
	limit := 50
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	cursor := ""
	if req.Params.Cursor != nil {
		cursor = *req.Params.Cursor
	}
	page, err := s.students.List(ctx, p, limit, cursor)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Student, 0, len(page.Items))
	for _, st := range page.Items {
		out, err := toAPIStudent(st)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	resp := gen.ListStudents200JSONResponse{Items: items}
	if page.NextCursor != "" {
		next := page.NextCursor
		resp.NextCursor = &next
	}
	return resp, nil
}

// GetStudent returns one student, or 404 when it is not in the caller's tenant.
func (s *Server) GetStudent(ctx context.Context, req gen.GetStudentRequestObject) (gen.GetStudentResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermStudentView)
	if err != nil {
		return nil, err
	}
	st, err := s.students.Get(ctx, p, req.StudentId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIStudent(st)
	if err != nil {
		return nil, err
	}
	return gen.GetStudent200JSONResponse(out), nil
}

// guardianInput maps the generated guardian body to the service input.
func guardianInput(g gen.GuardianInput) students.GuardianInput {
	in := students.GuardianInput{
		FullName:     g.FullName,
		Mobile:       string(g.Mobile),
		Relationship: string(g.Relationship),
		Primary:      g.Primary,
		FeePayer:     g.FeePayer,
	}
	in.ReceivesNotifications = g.ReceivesNotifications
	in.EmergencyContact = g.EmergencyContact
	if g.Access != nil {
		in.Access = string(*g.Access)
	}
	return in
}

// toAPIStudent maps the service read model to the generated response type.
func toAPIStudent(st students.Student) (gen.Student, error) {
	id, err := uuid.Parse(st.ID)
	if err != nil {
		return gen.Student{}, err
	}
	familyID, err := uuid.Parse(st.FamilyID)
	if err != nil {
		return gen.Student{}, err
	}
	guardians := make([]gen.GuardianView, 0, len(st.Guardians))
	for _, g := range st.Guardians {
		gid, err := uuid.Parse(g.GuardianID)
		if err != nil {
			return gen.Student{}, err
		}
		guardians = append(guardians, gen.GuardianView{
			GuardianId:            gid,
			FullName:              g.FullName,
			Mobile:                g.Mobile,
			Relationship:          gen.Relationship(g.Relationship),
			Primary:               g.Primary,
			FeePayer:              g.FeePayer,
			ReceivesNotifications: g.ReceivesNotifications,
			EmergencyContact:      g.EmergencyContact,
			Access:                gen.GuardianAccess(g.Access),
		})
	}
	return gen.Student{
		Id:             id,
		RegistrationNo: st.RegistrationNo,
		FullName:       st.FullName,
		FatherName:     st.FatherName,
		Gender:         gen.Gender(st.Gender),
		DateOfBirth:    openapitypes.Date{Time: st.DateOfBirth},
		Status:         gen.StudentStatus(st.Status),
		Family: struct {
			Id     openapitypes.UUID `json:"id"`
			Number string            `json:"number"`
		}{Id: familyID, Number: st.FamilyNumber},
		Guardians: guardians,
		CreatedAt: st.CreatedAt,
	}, nil
}
