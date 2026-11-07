-- name: EnsureSystemAccounts :exec
-- The fixed chart for a school, created the first time the ledger is used. Safe to run twice.
-- Keep this list identical to the constants in internal/ledger/accounts.go; a test checks it.
INSERT INTO accounts (tenant_id, code, name, type, system)
SELECT sqlc.arg('tenant_id')::uuid, v.code, v.name, v.type, true
FROM (VALUES
    ('1000', 'Cash in Hand', 'asset'),
    ('1100', 'Bank', 'asset'),
    ('1200', 'Fee Receivable', 'asset'),
    ('2000', 'Advance Fees Received', 'liability'),
    ('2100', 'Security Deposits Held', 'liability'),
    ('4000', 'Fee Income', 'income'),
    ('5000', 'Discounts Given', 'expense')
) AS v(code, name, type)
ON CONFLICT (tenant_id, code) DO NOTHING;

-- name: ListAccounts :many
SELECT id, code, name, type, system
FROM accounts
WHERE tenant_id = sqlc.arg('tenant_id')
ORDER BY code;

-- name: ReversalOfEntry :one
-- A friendly check before inserting; the partial unique index journal_entries_one_reversal is the
-- guarantee when two reversals race.
SELECT id
FROM journal_entries
WHERE tenant_id = sqlc.arg('tenant_id') AND reverses_entry_id = sqlc.arg('entry_id');

-- name: NextEntryNo :one
-- Gapless: the counter moves inside the posting transaction, so a rolled-back post gives its
-- number back. A Postgres sequence would not (sequences never roll back).
INSERT INTO tenant_counters (tenant_id, name, value)
VALUES (sqlc.arg('tenant_id'), 'journal-entry', 1)
ON CONFLICT (tenant_id, name) DO UPDATE SET value = tenant_counters.value + 1
RETURNING value;

-- name: InsertEntry :one
INSERT INTO journal_entries (tenant_id, entry_no, occurred_on, memo, source_type, source_id,
                             reverses_entry_id, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('entry_no'), sqlc.arg('occurred_on'), sqlc.arg('memo'),
        sqlc.arg('source_type'), sqlc.narg('source_id'), sqlc.narg('reverses_entry_id'),
        sqlc.arg('created_by'))
RETURNING id, created_at;

-- name: InsertLine :exec
INSERT INTO journal_lines (tenant_id, entry_id, line_no, account_id, debit, credit)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('entry_id'), sqlc.arg('line_no'), sqlc.arg('account_id'),
        sqlc.arg('debit'), sqlc.arg('credit'));

-- name: GetEntry :one
SELECT id, entry_no, occurred_on, memo, source_type, source_id, reverses_entry_id, created_at
FROM journal_entries
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: ListEntriesPage :many
-- Newest first by entry number. A NULL cursor means the first page.
SELECT id, entry_no, occurred_on, memo, source_type, source_id, reverses_entry_id, created_at
FROM journal_entries
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('before_entry_no')::bigint IS NULL OR entry_no < sqlc.narg('before_entry_no')::bigint)
ORDER BY entry_no DESC
LIMIT sqlc.arg('page_limit');

-- name: ListLinesForEntries :many
SELECT l.entry_id, l.line_no, a.code AS account_code, a.name AS account_name, l.debit, l.credit
FROM journal_lines l
JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
WHERE l.tenant_id = sqlc.arg('tenant_id') AND l.entry_id = ANY(sqlc.arg('entry_ids')::uuid[])
ORDER BY l.entry_id, l.line_no;

-- name: TrialBalance :many
-- Debit and credit totals per account for entries on or before as_of. The sub-select applies the
-- date filter to the lines, so accounts with no activity still appear with zeros.
SELECT a.code, a.name, a.type,
       COALESCE(SUM(x.debit), 0)::bigint  AS debit,
       COALESCE(SUM(x.credit), 0)::bigint AS credit
FROM accounts a
LEFT JOIN (
    SELECT l.account_id, l.debit, l.credit
    FROM journal_lines l
    JOIN journal_entries e ON e.tenant_id = l.tenant_id AND e.id = l.entry_id
    WHERE l.tenant_id = sqlc.arg('tenant_id') AND e.occurred_on <= sqlc.arg('as_of')
) x ON x.account_id = a.id
WHERE a.tenant_id = sqlc.arg('tenant_id')
GROUP BY a.code, a.name, a.type
ORDER BY a.code;
