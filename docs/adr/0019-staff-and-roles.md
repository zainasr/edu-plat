# ADR-0019: Staff and roles management

| | |
|---|---|
| Status | Accepted (choices made by the product owner on 2026-11-15) |
| Date | 2026-11-15 |
| Spec links | REQ-IAM-001, REQ-IAM-002, REQ-IAM-005, REQ-IAM-006, US-IAM-001, US-IAM-005, ADR-0011, ADR-0012 |

## Context
Until now the only way to give someone access was a database edit. An owner needs to add staff, give and take roles, and switch people off, safely: no school may lock itself out, nobody may promote themselves, and a person who leaves must lose access at once. Users are one global identity per mobile number (REQ-IAM-001), and the API database role cannot write the shared `users` table.

## Decision
1. **Adding a person** goes through one narrow SECURITY DEFINER function, `ensure_user(mobile, name)` (same pattern as ADR-0012): it adds the person if the number is new, otherwise returns the existing id and changes nothing. The API role gets no write access to `users`.
2. **The response never reveals another school.** A school stores its own name for a person (`memberships.display_name`) and shows only that, so adding a number that exists elsewhere neither shows the other school's name nor says the person existed.
3. **Roles are revoked, not deleted** (DEC-011, REQ-IAM-006): `role_assignments` gains `revoked_at/revoked_by`; uniqueness applies to live assignments only, so a role can be given again later. A trigger makes a revoked row immutable and lets a live row change only by being revoked; the API role may update only those two columns.
4. **Deactivation** sets the membership to `deactivated` and revokes every token of that membership. Because `Authenticate` already checks the membership on every request, access ends on the very next request, with no sweep job (owner choice). Reactivation restores the roles; old tokens stay dead and the person signs in again.
5. **Owner rules**, enforced in the service where the data is:
   - only an owner can give or take the `owner` role;
   - the last active owner can never be demoted or deactivated (`staff.last_owner`);
   - nobody can deactivate themselves (`staff.self_deactivate`).
   Operations that can remove an owner take one per-school advisory lock, then count, so two owners removing each other at the same moment cannot both succeed.
6. **Permissions** `user.view`, `user.manage` and `role.manage` belong to the owner role only. The Accountant and Receptionist roles hold none, until an Admin role exists.
7. `POST /staff` and `POST /staff/{id}/roles` honour the Idempotency-Key (ADR-0018).

## Found in review
- **A caller's roles are read at sign-in, before the transaction.** For anything that touches the owner role, the service takes the per-school owner lock first and then re-reads the caller's live roles and status inside the transaction (`rolesForOwnerRule`). Someone demoted or switched off a moment ago cannot promote anyone. This covers adding an owner, assigning or revoking `owner`, and deactivating or reactivating an owner (that last pair needs an owner too).
- **The response has no global user id.** The first draft returned `users.id`, which let two schools see they had added the same person. It is gone; the person is identified only inside the school by the membership id.
- **Adding staff needs `user.manage` and `role.manage`**, because it gives roles. Proved with a test-only principal helper (`iam/export_test.go`), since only the owner holds both today.
- **REQ-IAM-006 followed to the letter:** revoking a person's last role switches them off at once, with tokens revoked, as deactivation does. You cannot do that to yourself (`staff.self_deactivate`), and the last owner rule still applies first.
- `ensure_user` can add unlimited `users` rows with chosen names if the API role is compromised; it cannot change or read an existing one. Accepted under ADR-0012. A timing difference between "new number" and "existing number" remains and is accepted.

## Consequences
- A school can be fully administered through the API for the first time.
- Roles are still code (three built-ins). Custom roles (US-IAM-002), scoped assignments (REQ-IAM-003), maker-checker and parent/student accounts are later slices.
- A person who holds no live role can still sign in but can do nothing; the audit log keeps who changed what (`staff.created`, `role.assigned`, `role.revoked`, `staff.deactivated`, `staff.reactivated`).
- The mobile number of a staff member is visible to anyone with `user.view`. Only owners hold it today.
