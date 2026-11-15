package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// Handlers for staff and roles. Same shape as every handler: check the permission, call the service,
// map the result. The owner-only and last-owner rules live in iam, where the data is (ADR-0019).

// ListRoles lists the built-in roles.
func (s *Server) ListRoles(ctx context.Context, _ gen.ListRolesRequestObject) (gen.ListRolesResponseObject, error) {
	if _, err := iam.Require(ctx, iam.PermUserView); err != nil {
		return nil, err
	}
	items := make([]gen.RoleInfo, 0, 3)
	for _, r := range iam.RoleCatalogue() {
		items = append(items, gen.RoleInfo{Name: gen.RoleName(r.Name), Permissions: r.Permissions})
	}
	return gen.ListRoles200JSONResponse{Items: items}, nil
}

// CreateStaff adds a staff member.
func (s *Server) CreateStaff(ctx context.Context, req gen.CreateStaffRequestObject) (gen.CreateStaffResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermUserManage)
	if err != nil {
		return nil, err
	}
	// Adding staff gives them roles, so the caller also needs the permission to give roles.
	if _, err := iam.Require(ctx, iam.PermRoleManage); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	roles := make([]string, 0, len(req.Body.Roles))
	for _, r := range req.Body.Roles {
		roles = append(roles, string(r))
	}
	created, err := s.iam.CreateStaff(ctx, p, iam.StaffInput{Mobile: req.Body.Mobile, FullName: req.Body.FullName, Roles: roles})
	if err != nil {
		return nil, err
	}
	out, err := toAPIStaff(created)
	if err != nil {
		return nil, err
	}
	return gen.CreateStaff201JSONResponse(out), nil
}

// ListStaff lists the school's staff.
func (s *Server) ListStaff(ctx context.Context, req gen.ListStaffRequestObject) (gen.ListStaffResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermUserView)
	if err != nil {
		return nil, err
	}
	status := ""
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	list, err := s.iam.ListStaff(ctx, p, status)
	if err != nil {
		return nil, err
	}
	items := make([]gen.StaffMember, 0, len(list))
	for _, m := range list {
		out, err := toAPIStaff(m)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	return gen.ListStaff200JSONResponse{Items: items}, nil
}

// GetStaff returns one staff member.
func (s *Server) GetStaff(ctx context.Context, req gen.GetStaffRequestObject) (gen.GetStaffResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermUserView)
	if err != nil {
		return nil, err
	}
	m, err := s.iam.GetStaff(ctx, p, req.StaffId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIStaff(m)
	if err != nil {
		return nil, err
	}
	return gen.GetStaff200JSONResponse(out), nil
}

// AssignRole gives a role.
func (s *Server) AssignRole(ctx context.Context, req gen.AssignRoleRequestObject) (gen.AssignRoleResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermRoleManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	m, err := s.iam.AssignRole(ctx, p, req.StaffId.String(), string(req.Body.Role))
	if err != nil {
		return nil, err
	}
	out, err := toAPIStaff(m)
	if err != nil {
		return nil, err
	}
	return gen.AssignRole200JSONResponse(out), nil
}

// RevokeRole takes a role away.
func (s *Server) RevokeRole(ctx context.Context, req gen.RevokeRoleRequestObject) (gen.RevokeRoleResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermRoleManage)
	if err != nil {
		return nil, err
	}
	m, err := s.iam.RevokeRole(ctx, p, req.StaffId.String(), string(req.Role))
	if err != nil {
		return nil, err
	}
	out, err := toAPIStaff(m)
	if err != nil {
		return nil, err
	}
	return gen.RevokeRole200JSONResponse(out), nil
}

// DeactivateStaff switches a person off.
func (s *Server) DeactivateStaff(ctx context.Context, req gen.DeactivateStaffRequestObject) (gen.DeactivateStaffResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermUserManage)
	if err != nil {
		return nil, err
	}
	m, err := s.iam.DeactivateStaff(ctx, p, req.StaffId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIStaff(m)
	if err != nil {
		return nil, err
	}
	return gen.DeactivateStaff200JSONResponse(out), nil
}

// ReactivateStaff switches a person back on.
func (s *Server) ReactivateStaff(ctx context.Context, req gen.ReactivateStaffRequestObject) (gen.ReactivateStaffResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermUserManage)
	if err != nil {
		return nil, err
	}
	m, err := s.iam.ReactivateStaff(ctx, p, req.StaffId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIStaff(m)
	if err != nil {
		return nil, err
	}
	return gen.ReactivateStaff200JSONResponse(out), nil
}

func toAPIStaff(m iam.StaffMember) (gen.StaffMember, error) {
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return gen.StaffMember{}, err
	}
	roles := make([]gen.RoleName, 0, len(m.Roles))
	for _, r := range m.Roles {
		roles = append(roles, gen.RoleName(r))
	}
	return gen.StaffMember{
		Id: id, Mobile: m.Mobile, FullName: m.FullName,
		Status: gen.StaffStatus(m.Status), Roles: roles, CreatedAt: m.CreatedAt,
	}, nil
}
