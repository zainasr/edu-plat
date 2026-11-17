// Package iam covers identity for the skeleton: OTP sign-in, sessions, personal access
// tokens, and named permissions (IAM spec §3, §6; ADR-0011).
package iam

import (
	"context"
	"slices"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// Named permissions (IAM spec §3). Handlers check these, never role names.
const (
	PermStudentView   = "student.view"
	PermStudentCreate = "student.create"
	PermStudentEdit   = "student.edit"
	PermTokenManage   = "token.manage"
	PermOrgView       = "org.view"
	PermOrgManage     = "org.manage"
	PermSessionManage = "session.manage"

	PermLedgerView        = "ledger.view"
	PermReportFinanceView = "report.finance.view"

	PermFeeView       = "fee.view"
	PermFeePlanManage = "fee.plan.manage"
	PermFeeSettings   = "fee.settings"
	PermFeeGenerate   = "fee.generate"
	PermFeeRelease    = "fee.release"
	PermFeeCollect    = "fee.collect"
	PermPaymentVoid   = "payment.void"
	PermFeeRefund     = "fee.refund"

	PermCalendarManage       = "calendar.manage"
	PermAttendanceView       = "attendance.view"
	PermAttendanceMark       = "attendance.mark"
	PermAttendanceEditWindow = "attendance.edit_within_window"
	PermAttendanceEditAny    = "attendance.edit_any"
	PermAttendanceSettings   = "attendance.settings"
	PermAttendanceContacts   = "attendance.contacts"

	PermUserView   = "user.view"
	PermUserManage = "user.manage"
	PermRoleManage = "role.manage"
)

// rolePermissions maps each built-in role to its permissions. Roles are code in the skeleton;
// assignments are data (role_assignments). Permissions are the union over a membership's roles.
var rolePermissions = map[string][]string{
	"owner": {
		PermStudentView, PermStudentCreate, PermStudentEdit, PermTokenManage,
		PermOrgView, PermOrgManage, PermSessionManage,
		PermLedgerView, PermReportFinanceView,
		PermFeeView, PermFeePlanManage, PermFeeSettings, PermFeeGenerate, PermFeeRelease,
		PermFeeCollect, PermPaymentVoid, PermFeeRefund,
		PermCalendarManage, PermAttendanceView, PermAttendanceMark, PermAttendanceEditWindow, PermAttendanceEditAny, PermAttendanceSettings, PermAttendanceContacts,
		PermUserView, PermUserManage, PermRoleManage,
	},
	// Accountant: fee.* except settings, and the accounting reads (spec 02-identity §3). Payments,
	// payroll and the rest are added with the slices that build them.
	"accountant": {
		PermFeeView, PermFeePlanManage, PermFeeGenerate, PermFeeRelease, PermFeeCollect, PermPaymentVoid, PermFeeRefund,
		PermLedgerView, PermReportFinanceView,
	},
	// Receptionist: sees students and fees and takes payments (fee.collect). The cash session arrives later.
	"receptionist": {
		PermStudentView, PermFeeView, PermFeeCollect, PermOrgView,
		PermAttendanceView, PermAttendanceMark, PermAttendanceContacts,
	},
	// Attendance clerk: the attendance desk enters absentees from teachers' paper slips (US-ATT-007). Teachers
	// marking their own sections come with the staff module's section assignments.
	// The clerk can also correct a submitted register within the correction window (every correction keeps
	// the old value, who and why); anything older needs attendance.edit_any, held by the owner.
	"attendance_clerk": {
		PermStudentView, PermOrgView, PermAttendanceView, PermAttendanceMark, PermAttendanceEditWindow, PermAttendanceContacts,
	},
}

// Principal is who is calling, in which tenant, with which token (REQ-IAM-002, REQ-API-017).
type Principal struct {
	TenantID     string
	UserID       string
	MembershipID string
	TokenID      string
	TokenKind    string // "session" or "pat"
	roles        []string
	permissions  map[string]struct{}
}

// HasRole reports whether the principal holds the named role (a live assignment).
func (p Principal) HasRole(role string) bool { return slices.Contains(p.roles, role) }

// Can reports whether the principal holds the named permission.
func (p Principal) Can(permission string) bool {
	_, ok := p.permissions[permission]
	return ok
}

// permissionsFor returns the union of the permissions of the given roles.
func permissionsFor(roles []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, role := range roles {
		for _, perm := range rolePermissions[role] {
			out[perm] = struct{}{}
		}
	}
	return out
}

type principalKey struct{}

// WithPrincipal stores the principal in the context. Only the authentication middleware calls it.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// From returns the principal, if the request was authenticated.
func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Require returns the principal when it holds the permission. Missing credentials are 401;
// missing permission is 403 (API conventions §3).
func Require(ctx context.Context, permission string) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return Principal{}, apperr.Unauthenticated("auth.required", "Sign in or send a token to use this endpoint.")
	}
	if !p.Can(permission) {
		return Principal{}, apperr.Forbidden("auth.permission_denied", "Your token does not allow this action ("+permission+").")
	}
	return p, nil
}

// RequireAny is like Require for an endpoint that two permissions can open (for example correcting
// attendance within a window, or at any age). The service then decides what the caller may do.
func RequireAny(ctx context.Context, permissions ...string) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return Principal{}, apperr.Unauthenticated("auth.required", "Sign in or send a token to use this endpoint.")
	}
	for _, perm := range permissions {
		if p.Can(perm) {
			return p, nil
		}
	}
	return Principal{}, apperr.Forbidden("auth.permission_denied", "Your token does not allow this action ("+permissions[0]+").")
}
