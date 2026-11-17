-- name: EnsureDefaultFeeTypes :exec
-- The suggested set, created the first time a school looks at its fee types. They are suggestions,
-- never locks: a school archives what it does not use. ON CONFLICT DO NOTHING means an archived
-- suggestion stays archived, and a type the school created first is not overwritten.
INSERT INTO fee_types (tenant_id, code, name, kind, is_deposit)
SELECT sqlc.arg('tenant_id')::uuid, v.code, v.name, v.kind, v.is_deposit
FROM (VALUES
    ('tuition',          'Tuition',          'tuition',          false),
    ('admission',        'Admission Fee',    'admission',        false),
    ('registration',     'Registration Fee', 'registration',     false),
    ('annual',           'Annual Fee',       'annual',           false),
    ('transport',        'Transport',        'transport',        false),
    ('security_deposit', 'Security Deposit', 'security_deposit', true)
) AS v(code, name, kind, is_deposit)
ON CONFLICT (tenant_id, code) DO NOTHING;

-- name: ListFeeTypes :many
SELECT id, code, name, kind, is_deposit, status
FROM fee_types
WHERE tenant_id = sqlc.arg('tenant_id')
ORDER BY name, id;

-- name: GetFeeType :one
SELECT id, code, name, kind, is_deposit, status
FROM fee_types
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: InsertFeeType :one
INSERT INTO fee_types (tenant_id, code, name, kind, is_deposit, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('kind'),
        sqlc.arg('is_deposit'), sqlc.arg('created_by'))
RETURNING id, code, name, kind, is_deposit, status;

-- name: ArchiveFeeType :one
UPDATE fee_types
SET status = 'archived'
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
RETURNING id, code, name, kind, is_deposit, status;

-- name: InsertPlanLine :one
INSERT INTO fee_plan_lines (tenant_id, class_id, fee_type_id, amount, frequency, effective_from, removed, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('class_id'), sqlc.arg('fee_type_id'), sqlc.arg('amount'),
        sqlc.arg('frequency'), sqlc.arg('effective_from'), sqlc.arg('removed'), sqlc.arg('created_by'))
RETURNING id;

-- name: ListPlanVersions :many
-- Every version ever set on a class plan, newest first, with the fee type's name and kind.
SELECT p.fee_type_id, t.name AS fee_type_name, t.kind, p.amount, p.frequency, p.effective_from, p.removed
FROM fee_plan_lines p
JOIN fee_types t ON t.tenant_id = p.tenant_id AND t.id = p.fee_type_id
WHERE p.tenant_id = sqlc.arg('tenant_id') AND p.class_id = sqlc.arg('class_id')
ORDER BY p.effective_from DESC, p.created_at DESC, p.id DESC;

-- name: InsertOverride :one
INSERT INTO student_fee_overrides (tenant_id, student_id, fee_type_id, amount, effective_from, cleared, reason, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('fee_type_id'), sqlc.arg('amount'),
        sqlc.arg('effective_from'), sqlc.arg('cleared'), sqlc.arg('reason'), sqlc.arg('created_by'))
RETURNING id, created_at;

-- name: ListOverridesForStudent :many
SELECT o.fee_type_id, t.name AS fee_type_name, o.amount, o.effective_from, o.cleared, o.reason, o.created_at
FROM student_fee_overrides o
JOIN fee_types t ON t.tenant_id = o.tenant_id AND t.id = o.fee_type_id
WHERE o.tenant_id = sqlc.arg('tenant_id') AND o.student_id = sqlc.arg('student_id')
ORDER BY o.effective_from DESC, o.created_at DESC, o.id DESC;

-- name: NextChallanNo :one
-- Gapless: the counter moves inside the release transaction, so a rolled-back release gives its
-- number back.
INSERT INTO tenant_counters (tenant_id, name, value)
VALUES (sqlc.arg('tenant_id'), 'challan', 1)
ON CONFLICT (tenant_id, name) DO UPDATE SET value = tenant_counters.value + 1
RETURNING value;

-- name: LiveChallanStudents :many
-- Students who already have a draft or released challan for the month.
SELECT student_id
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id') AND month = sqlc.arg('month') AND status <> 'void';

-- name: InsertChallan :one
-- DO NOTHING on the one-live-challan-per-month index: a student who already has one returns no row.
INSERT INTO challans (tenant_id, student_id, family_id, month, due_date, total, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('family_id'), sqlc.arg('month'),
        sqlc.arg('due_date'), sqlc.arg('total'), sqlc.arg('created_by'))
ON CONFLICT (tenant_id, student_id, month) WHERE status <> 'void' DO NOTHING
RETURNING id;

-- name: InsertChallanLine :exec
INSERT INTO challan_lines (tenant_id, challan_id, line_no, fee_type_id, fee_type_name, kind, frequency, amount)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('challan_id'), sqlc.arg('line_no'), sqlc.arg('fee_type_id'),
        sqlc.arg('fee_type_name'), sqlc.arg('kind'), sqlc.arg('frequency'), sqlc.arg('amount'));

-- name: GetChallan :one
SELECT id, student_id, family_id, month, due_date, status, total, challan_no, consumer_no,
       released_at, void_reason, created_at,
       (COALESCE((SELECT sum(a.amount) FROM payment_allocations a
                 JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id AND p.status = 'recorded'
                 WHERE a.tenant_id = challans.tenant_id AND a.challan_id = challans.id), 0)
        + COALESCE((SELECT sum(x.amount) FROM advance_applications x
                    WHERE x.tenant_id = challans.tenant_id AND x.challan_id = challans.id), 0))::bigint AS paid
FROM challans
WHERE challans.tenant_id = sqlc.arg('tenant_id') AND challans.id = sqlc.arg('id');

-- name: LockChallan :one
-- Takes the row lock, so two releases of the same challan run one after the other.
SELECT id, status, family_id
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: ListChallansPage :many
-- Keyset pagination on (created_at, id), newest first. A NULL cursor or filter means "no condition".
SELECT id, student_id, family_id, month, due_date, status, total, challan_no, consumer_no,
       released_at, void_reason, created_at,
       (COALESCE((SELECT sum(a.amount) FROM payment_allocations a
                 JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id AND p.status = 'recorded'
                 WHERE a.tenant_id = challans.tenant_id AND a.challan_id = challans.id), 0)
        + COALESCE((SELECT sum(x.amount) FROM advance_applications x
                    WHERE x.tenant_id = challans.tenant_id AND x.challan_id = challans.id), 0))::bigint AS paid
FROM challans
WHERE challans.tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('month')::date IS NULL OR challans.month = sqlc.narg('month')::date)
  AND (sqlc.narg('status')::text IS NULL OR challans.status = sqlc.narg('status')::text)
  AND (sqlc.narg('student_id')::uuid IS NULL OR challans.student_id = sqlc.narg('student_id')::uuid)
  AND (sqlc.narg('after_created_at')::timestamptz IS NULL
       OR (challans.created_at, challans.id) < (sqlc.narg('after_created_at')::timestamptz, sqlc.narg('after_id')::uuid))
ORDER BY challans.created_at DESC, challans.id DESC
LIMIT sqlc.arg('page_limit');

-- name: ListChallanLines :many
SELECT challan_id, line_no, fee_type_id, fee_type_name, kind, frequency, amount
FROM challan_lines
WHERE tenant_id = sqlc.arg('tenant_id') AND challan_id = ANY(sqlc.arg('challan_ids')::uuid[])
ORDER BY challan_id, line_no;

-- name: ReleaseChallan :one
-- Conditional on draft: with the row lock taken first, this cannot run twice for one challan.
UPDATE challans
SET status = 'released', challan_no = sqlc.arg('challan_no'), consumer_no = sqlc.arg('consumer_no'),
    released_at = now(), released_by = sqlc.arg('released_by')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'draft'
RETURNING id;

-- name: VoidChallan :one
UPDATE challans
SET status = 'void', void_reason = sqlc.arg('reason'), voided_at = now()
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'draft'
RETURNING id;

-- name: FirstVersionMonths :many
-- When each fee type first took effect on a class plan: it anchors annual fees to a month of the year.
SELECT fee_type_id, min(effective_from)::date AS first_from
FROM fee_plan_lines
WHERE tenant_id = sqlc.arg('tenant_id') AND class_id = sqlc.arg('class_id')
GROUP BY fee_type_id;

-- name: ChallanDepositTotal :one
-- The part of a challan that is a security deposit: a liability, never income (BR-FEE-004).
SELECT COALESCE(sum(amount) FILTER (WHERE kind = 'security_deposit'), 0)::bigint AS deposit
FROM challan_lines
WHERE tenant_id = sqlc.arg('tenant_id') AND challan_id = sqlc.arg('challan_id');

-- ---- payments (slice 5a) --------------------------------------------------------------

-- name: NextReceiptNo :one
-- Gapless per school and fiscal year: the counter moves inside the payment transaction.
INSERT INTO tenant_counters (tenant_id, name, value)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('name'), 1)
ON CONFLICT (tenant_id, name) DO UPDATE SET value = tenant_counters.value + 1
RETURNING value;

-- name: LockFamilyChallans :many
-- Locks every released challan of the family, in id order, before any balance is read, so two cashiers
-- paying the same family cannot both see the same open balance.
SELECT id
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id') AND family_id = sqlc.arg('family_id') AND status = 'released'
ORDER BY id
FOR UPDATE;

-- name: FamilyChallanBalances :many
-- Released challans of a family with what has been paid on each by payments that stand, in the order a
-- payment pays them: oldest due date first.
SELECT challans.id, student_id, due_date, challan_no, month, total,
       (COALESCE((SELECT sum(a.amount) FROM payment_allocations a
                 JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id AND p.status = 'recorded'
                 WHERE a.tenant_id = challans.tenant_id AND a.challan_id = challans.id), 0)
        + COALESCE((SELECT sum(x.amount) FROM advance_applications x
                    WHERE x.tenant_id = challans.tenant_id AND x.challan_id = challans.id), 0))::bigint AS paid
FROM challans
WHERE challans.tenant_id = sqlc.arg('tenant_id') AND challans.family_id = sqlc.arg('family_id') AND challans.status = 'released'
  AND (sqlc.narg('only_ids')::uuid[] IS NULL OR challans.id = ANY(sqlc.narg('only_ids')::uuid[]))
ORDER BY challans.due_date, challans.challan_no, challans.id;

-- name: InsertPayment :exec
INSERT INTO payments (id, tenant_id, family_id, fiscal_year, receipt_no, amount, advance, method, account_code,
                      reference, received_on, journal_entry_id, created_by)
VALUES (sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('family_id'), sqlc.arg('fiscal_year'), sqlc.arg('receipt_no'),
        sqlc.arg('amount'), sqlc.arg('advance'), sqlc.arg('method'), sqlc.arg('account_code'), sqlc.narg('reference'),
        sqlc.arg('received_on'), sqlc.arg('journal_entry_id'), sqlc.arg('created_by'));

-- name: InsertAllocation :exec
INSERT INTO payment_allocations (tenant_id, payment_id, challan_id, amount)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('payment_id'), sqlc.arg('challan_id'), sqlc.arg('amount'));

-- name: GetPayment :one
SELECT id, family_id, fiscal_year, receipt_no, amount, advance,
       COALESCE((SELECT sum(x.amount) FROM advance_applications x WHERE x.tenant_id = payments.tenant_id AND x.payment_id = payments.id), 0)::bigint AS advance_applied,
       COALESCE((SELECT sum(r.amount) FROM advance_refunds r WHERE r.tenant_id = payments.tenant_id AND r.payment_id = payments.id), 0)::bigint AS advance_refunded,
       method, account_code, reference, received_on, status,
       journal_entry_id, void_reason, voided_at, created_at
FROM payments
WHERE payments.tenant_id = sqlc.arg('tenant_id') AND payments.id = sqlc.arg('id');

-- name: LockPayment :one
SELECT id, status, journal_entry_id
FROM payments
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: ListPaymentsPage :many
-- Keyset pagination on (created_at, id), newest first.
SELECT id, family_id, fiscal_year, receipt_no, amount, advance,
       COALESCE((SELECT sum(x.amount) FROM advance_applications x WHERE x.tenant_id = payments.tenant_id AND x.payment_id = payments.id), 0)::bigint AS advance_applied,
       COALESCE((SELECT sum(r.amount) FROM advance_refunds r WHERE r.tenant_id = payments.tenant_id AND r.payment_id = payments.id), 0)::bigint AS advance_refunded,
       method, account_code, reference, received_on, status,
       journal_entry_id, void_reason, voided_at, created_at
FROM payments
WHERE payments.tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('family_id')::uuid IS NULL OR payments.family_id = sqlc.narg('family_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR payments.status = sqlc.narg('status')::text)
  AND (sqlc.narg('after_created_at')::timestamptz IS NULL
       OR (payments.created_at, payments.id) < (sqlc.narg('after_created_at')::timestamptz, sqlc.narg('after_id')::uuid))
ORDER BY payments.created_at DESC, payments.id DESC
LIMIT sqlc.arg('page_limit');

-- name: ListAllocationsForPayments :many
SELECT a.payment_id, a.challan_id, a.amount, c.student_id, c.challan_no
FROM payment_allocations a
JOIN challans c ON c.tenant_id = a.tenant_id AND c.id = a.challan_id
WHERE a.tenant_id = sqlc.arg('tenant_id') AND a.payment_id = ANY(sqlc.arg('payment_ids')::uuid[])
ORDER BY a.payment_id, c.challan_no;

-- name: VoidPayment :execrows
-- Conditional on recorded: with the row lock taken first this runs once per payment.
UPDATE payments
SET status = 'voided', void_reason = sqlc.arg('reason'), voided_at = now(), voided_by = sqlc.arg('voided_by'),
    void_entry_id = sqlc.arg('void_entry_id')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'recorded';

-- ---- advance and refunds (slice 5b) ---------------------------------------------------

-- name: LockFamilyAdvancePayments :many
-- Locks the payments of these families that hold advance, before any counter is taken, so applying
-- advance at release cannot interleave with a refund or a void of the same payment.
SELECT id
FROM payments
WHERE tenant_id = sqlc.arg('tenant_id') AND family_id = ANY(sqlc.arg('family_ids')::uuid[])
  AND status = 'recorded' AND advance > 0
ORDER BY family_id, created_at, id
FOR UPDATE;

-- name: FamilyAdvanceBalances :many
-- What each of a family's payments still holds as advance, oldest payment first.
SELECT payments.id, payments.receipt_no, payments.fiscal_year,
       (payments.advance
         - COALESCE((SELECT sum(x.amount) FROM advance_applications x WHERE x.tenant_id = payments.tenant_id AND x.payment_id = payments.id), 0)
         - COALESCE((SELECT sum(r.amount) FROM advance_refunds r WHERE r.tenant_id = payments.tenant_id AND r.payment_id = payments.id), 0))::bigint AS remaining
FROM payments
WHERE payments.tenant_id = sqlc.arg('tenant_id') AND payments.family_id = sqlc.arg('family_id')
  AND payments.status = 'recorded' AND payments.advance > 0
ORDER BY payments.created_at, payments.id;

-- name: InsertAdvanceApplication :exec
INSERT INTO advance_applications (tenant_id, payment_id, challan_id, amount, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('payment_id'), sqlc.arg('challan_id'), sqlc.arg('amount'), sqlc.arg('created_by'));

-- name: InsertAdvanceRefund :one
INSERT INTO advance_refunds (tenant_id, payment_id, amount, account_code, reason, journal_entry_id, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('payment_id'), sqlc.arg('amount'), sqlc.arg('account_code'),
        sqlc.arg('reason'), sqlc.arg('journal_entry_id'), sqlc.arg('created_by'))
RETURNING id;

-- name: ListRefundsForPayments :many
SELECT id, payment_id, amount, account_code, reason, journal_entry_id, created_at
FROM advance_refunds
WHERE tenant_id = sqlc.arg('tenant_id') AND payment_id = ANY(sqlc.arg('payment_ids')::uuid[])
ORDER BY payment_id, created_at, id;

-- ---- defaulters -----------------------------------------------------------------------

-- name: DefaulterFamilies :many
-- Families with a released challan that fell due on or before the cutoff and still has a balance, most
-- overdue first. Keyset on (oldest due date, family id).
WITH open AS (
    SELECT challans.family_id, challans.due_date,
           (challans.total - (COALESCE((SELECT sum(a.amount) FROM payment_allocations a
                                          JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id AND p.status = 'recorded'
                                         WHERE a.tenant_id = challans.tenant_id AND a.challan_id = challans.id), 0)
                           + COALESCE((SELECT sum(x.amount) FROM advance_applications x
                                        WHERE x.tenant_id = challans.tenant_id AND x.challan_id = challans.id), 0)))::bigint AS balance
    FROM challans
    WHERE challans.tenant_id = sqlc.arg('tenant_id') AND challans.status = 'released'
      AND challans.due_date <= sqlc.arg('cutoff')::date
)
SELECT family_id, min(due_date)::date AS oldest_due, sum(balance)::bigint AS overdue_total, count(*)::int AS challan_count
FROM open
WHERE balance > 0
GROUP BY family_id
HAVING sqlc.narg('after_due')::date IS NULL OR (min(due_date), family_id) > (sqlc.narg('after_due')::date, sqlc.narg('after_family')::uuid)
ORDER BY min(due_date), family_id
LIMIT sqlc.arg('page_limit');

-- name: OverdueChallansForFamilies :many
SELECT challans.id, challans.family_id, challans.student_id, challans.challan_no, challans.month, challans.due_date, challans.total,
       (COALESCE((SELECT sum(a.amount) FROM payment_allocations a
                    JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id AND p.status = 'recorded'
                   WHERE a.tenant_id = challans.tenant_id AND a.challan_id = challans.id), 0)
        + COALESCE((SELECT sum(x.amount) FROM advance_applications x
                     WHERE x.tenant_id = challans.tenant_id AND x.challan_id = challans.id), 0))::bigint AS paid
FROM challans
WHERE challans.tenant_id = sqlc.arg('tenant_id') AND challans.status = 'released'
  AND challans.family_id = ANY(sqlc.arg('family_ids')::uuid[]) AND challans.due_date <= sqlc.arg('cutoff')::date
ORDER BY challans.family_id, challans.due_date, challans.challan_no, challans.id;
