package iam_test

import (
	"context"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// Adding a staff member gives them roles, so it needs role.manage as well as user.manage. Only the
// owner holds both today, so this needs a principal that holds one and not the other. The server has
// no services: a request that gets past the permission check would fail loudly instead of passing.
func TestUS_IAM_001_AddingStaffNeedsBothPermissions(t *testing.T) {
	srv := api.NewServer(nil, nil, nil, nil, nil)
	const id = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	body := &gen.CreateStaffJSONRequestBody{Mobile: "03001234567", FullName: "X", Roles: []gen.RoleName{"accountant"}}

	for name, perms := range map[string][]string{
		"user.manage only": {iam.PermUserManage},
		"role.manage only": {iam.PermRoleManage},
		"neither":          {iam.PermUserView},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := iam.WithPrincipal(context.Background(), iam.PrincipalWith(id, id, id, nil, perms...))
			_, err := srv.CreateStaff(ctx, gen.CreateStaffRequestObject{Body: body})
			e, ok := apperr.As(err)
			if !ok || e.Code != "auth.permission_denied" {
				t.Fatalf("must be refused with 403, got %v", err)
			}
		})
	}
}
