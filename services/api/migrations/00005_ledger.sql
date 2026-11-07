-- +goose Up
-- The ledger core (ADR-0004, ADR-0014): accounts, journal entries and journal lines.
-- Amounts are bigint paisa. Journal rows are append-only: no UPDATE or DELETE grant to the API
-- role, and a trigger that refuses both for every role. A correction is a reversing entry.
-- Balance is enforced by the database at commit, so a bug in Go cannot unbalance the books.

CREATE TABLE accounts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    code       text NOT NULL CHECK (code ~ '^[0-9]{4}$'),
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    type       text NOT NULL CHECK (type IN ('asset', 'liability', 'income', 'expense', 'equity')),
    system     boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, code)
);

CREATE TABLE journal_entries (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants (id),
    entry_no            bigint NOT NULL CHECK (entry_no > 0),
    occurred_on         date NOT NULL,
    memo                text NOT NULL CHECK (length(memo) BETWEEN 1 AND 300),
    source_type         text NOT NULL CHECK (source_type ~ '^[a-z][a-z_.]{0,39}$'),
    source_id           uuid,
    reverses_entry_id   uuid,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, entry_no),
    FOREIGN KEY (tenant_id, reverses_entry_id) REFERENCES journal_entries (tenant_id, id)
);
-- An entry can be reversed at most once.
CREATE UNIQUE INDEX journal_entries_one_reversal ON journal_entries (tenant_id, reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;
CREATE INDEX journal_entries_date_idx ON journal_entries (tenant_id, occurred_on, entry_no);

CREATE TABLE journal_lines (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    entry_id   uuid NOT NULL,
    line_no    integer NOT NULL CHECK (line_no > 0),
    account_id uuid NOT NULL,
    debit      bigint NOT NULL DEFAULT 0 CHECK (debit >= 0),
    credit     bigint NOT NULL DEFAULT 0 CHECK (credit >= 0),
    -- Exactly one side carries the amount: a line is either a debit or a credit, never both or neither.
    CHECK ((debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0)),
    UNIQUE (tenant_id, entry_id, line_no),
    FOREIGN KEY (tenant_id, entry_id) REFERENCES journal_entries (tenant_id, id),
    FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id)
);
CREATE INDEX journal_lines_account_idx ON journal_lines (tenant_id, account_id);

-- ---------------------------------------------------------------------------
-- Balance, enforced at COMMIT. DEFERRABLE INITIALLY DEFERRED runs the check after every line of
-- the transaction exists, so entries can be built line by line. An entry needs at least two lines
-- and total debits must equal total credits. SQLSTATE 23514 is "check violation".
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
CREATE FUNCTION assert_journal_entry_balanced() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    eid uuid;
    d   bigint;
    c   bigint;
    n   integer;
BEGIN
    IF TG_TABLE_NAME = 'journal_entries' THEN
        eid := NEW.id;
    ELSE
        eid := NEW.entry_id;
    END IF;
    SELECT COALESCE(SUM(debit), 0), COALESCE(SUM(credit), 0), COUNT(*)
      INTO d, c, n
      FROM journal_lines
     WHERE tenant_id = NEW.tenant_id AND entry_id = eid;
    IF n < 2 THEN
        RAISE EXCEPTION 'journal entry % needs at least two lines (has %)', eid, n USING ERRCODE = '23514';
    END IF;
    IF d <> c THEN
        RAISE EXCEPTION 'journal entry % is not balanced: debit % credit %', eid, d, c USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER journal_entries_balanced AFTER INSERT ON journal_entries
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_journal_entry_balanced();
CREATE CONSTRAINT TRIGGER journal_lines_balanced AFTER INSERT ON journal_lines
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_journal_entry_balanced();

-- Append-only for every role, including the owner (forbid_mutation comes from 00001).
CREATE TRIGGER journal_entries_append_only BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER journal_lines_append_only BEFORE UPDATE OR DELETE ON journal_lines
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER accounts_no_delete BEFORE DELETE ON accounts
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['accounts', 'journal_entries', 'journal_lines']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %I
               USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)
               WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- Grants: insert and read only. Accounts are not updated in this slice either.
GRANT SELECT, INSERT ON accounts, journal_entries, journal_lines TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS journal_lines;
DROP TABLE IF EXISTS journal_entries;
DROP TABLE IF EXISTS accounts;
DROP FUNCTION IF EXISTS assert_journal_entry_balanced();
