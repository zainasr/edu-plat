package iam

// PrincipalWith builds a principal holding exactly the given permissions. It exists only for tests (this
// file is compiled only into test binaries), so a rule that depends on one permission among several can be
// proved with someone who has it and not the others. Production code cannot build a principal.
func PrincipalWith(tenantID, userID, membershipID string, roles []string, permissions ...string) Principal {
	perms := make(map[string]struct{}, len(permissions))
	for _, p := range permissions {
		perms[p] = struct{}{}
	}
	return Principal{TenantID: tenantID, UserID: userID, MembershipID: membershipID, roles: roles, permissions: perms}
}
