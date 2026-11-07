package ledger

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/ledger/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

const (
	minReasonLen = 3
	maxReasonLen = 200
)

// Account is a ledger account of a school.
type Account struct {
	ID     string
	Code   string
	Name   string
	Type   string
	System bool
}

// PostedLine is a stored line, with its account's name.
type PostedLine struct {
	LineNo      int
	AccountCode string
	AccountName string
	Debit       money.Money
	Credit      money.Money
}

// PostedEntry is a stored journal entry with its lines.
type PostedEntry struct {
	ID         string
	EntryNo    int64
	OccurredOn time.Time
	Memo       string
	SourceType string
	SourceID   string // empty when none
	ReversesID string // empty unless this entry reverses another
	Lines      []PostedLine
	CreatedAt  time.Time
}

// EntryPage is one page of entries, newest first. NextCursor is empty on the last page.
type EntryPage struct {
	Items      []PostedEntry
	NextCursor string
}

// TrialBalanceLine is one account's totals up to the report date.
type TrialBalanceLine struct {
	Code   string
	Name   string
	Type   string
	Debit  money.Money
	Credit money.Money
}

// TrialBalance lists every account's totals. It balances when total debits equal total credits,
// which is always true if the ledger is sound; the report shows it so a person can see that.
type TrialBalance struct {
	AsOf        time.Time
	Lines       []TrialBalanceLine
	TotalDebit  money.Money
	TotalCredit money.Money
	Balanced    bool
}

// Service is the posting engine and the ledger reads.
type Service struct {
	pool *pgxpool.Pool
}

// NewService builds the ledger service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Post validates and stores a balanced journal entry inside the caller's open tenant transaction.
// The caller's business change and this entry commit together or not at all; the database also
// refuses to commit the entry if it does not balance. Post never begins or commits.
func (s *Service) Post(ctx context.Context, tx pgx.Tx, p iam.Principal, e Entry) (PostedEntry, error) {
	return s.post(ctx, tx, p, e, pgtype.UUID{})
}

// Reverse posts the mirror image of an existing entry, linked to it. The original is never touched.
// An entry can be reversed once, and a reversal cannot itself be reversed (post a new entry instead).
func (s *Service) Reverse(ctx context.Context, tx pgx.Tx, p iam.Principal, entryID, reason string, on time.Time) (PostedEntry, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < minReasonLen || len(reason) > maxReasonLen {
		return PostedEntry{}, apperr.Validation("ledger.reason_invalid", "A reason of 3 to 200 characters is required.",
			apperr.Field{Field: "reason", Message: "3 to 200 characters."})
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return PostedEntry{}, err
	}
	id, err := db.ParseUUID(entryID)
	if err != nil {
		return PostedEntry{}, apperr.NotFound("ledger.entry_not_found", "Journal entry not found.")
	}
	q := gen.New(tx)
	orig, err := q.GetEntry(ctx, gen.GetEntryParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return PostedEntry{}, apperr.NotFound("ledger.entry_not_found", "Journal entry not found.")
	}
	if err != nil {
		return PostedEntry{}, fmt.Errorf("get entry: %w", err)
	}
	if orig.ReversesEntryID.Valid {
		return PostedEntry{}, apperr.Validation("ledger.cannot_reverse_a_reversal",
			"This entry is itself a reversal. Post a new entry instead.")
	}
	if _, err := q.ReversalOfEntry(ctx, gen.ReversalOfEntryParams{TenantID: tenant, EntryID: id}); err == nil {
		return PostedEntry{}, apperr.Conflict("ledger.already_reversed", "This entry has already been reversed.")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return PostedEntry{}, fmt.Errorf("check reversal: %w", err)
	}

	rows, err := q.ListLinesForEntries(ctx, gen.ListLinesForEntriesParams{TenantID: tenant, EntryIds: []pgtype.UUID{id}})
	if err != nil {
		return PostedEntry{}, fmt.Errorf("load lines: %w", err)
	}
	lines := make([]Line, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, Line{AccountCode: r.AccountCode, Debit: money.Money(r.Debit), Credit: money.Money(r.Credit)})
	}
	return s.post(ctx, tx, p, Entry{
		OccurredOn: on,
		Memo:       fmt.Sprintf("Reversal of #%d: %s", orig.EntryNo, reason),
		SourceType: "ledger.reversal",
		SourceID:   entryID,
		Lines:      reverseLines(lines),
	}, id)
}

func (s *Service) post(ctx context.Context, tx pgx.Tx, p iam.Principal, e Entry, reverses pgtype.UUID) (PostedEntry, error) {
	if err := validateEntry(e); err != nil {
		return PostedEntry{}, err
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return PostedEntry{}, err
	}
	actor, err := db.ParseUUID(p.UserID)
	if err != nil {
		return PostedEntry{}, err
	}
	q := gen.New(tx)

	// The chart exists from the first post. Then every line must name a real account.
	if err := q.EnsureSystemAccounts(ctx, tenant); err != nil {
		return PostedEntry{}, fmt.Errorf("ensure system accounts: %w", err)
	}
	accounts, err := q.ListAccounts(ctx, tenant)
	if err != nil {
		return PostedEntry{}, fmt.Errorf("list accounts: %w", err)
	}
	byCode := make(map[string]gen.ListAccountsRow, len(accounts))
	for _, a := range accounts {
		byCode[a.Code] = a
	}
	var unknown []apperr.Field
	for i, l := range e.Lines {
		if _, ok := byCode[l.AccountCode]; !ok {
			unknown = append(unknown, apperr.Field{Field: fmt.Sprintf("lines[%d]", i), Message: "No account has this code."})
		}
	}
	if len(unknown) > 0 {
		return PostedEntry{}, apperr.Validation("ledger.unknown_account", "A line names an account that does not exist.", unknown...)
	}

	var sourceID pgtype.UUID
	if e.SourceID != "" {
		if sourceID, err = db.ParseUUID(e.SourceID); err != nil {
			return PostedEntry{}, err
		}
	}

	// The number is taken last, after every cheap check, and inside this transaction: a failure
	// anywhere rolls it back, so entry numbers have no gaps.
	no, err := q.NextEntryNo(ctx, tenant)
	if err != nil {
		return PostedEntry{}, fmt.Errorf("next entry number: %w", err)
	}
	row, err := q.InsertEntry(ctx, gen.InsertEntryParams{
		TenantID:        tenant,
		EntryNo:         no,
		OccurredOn:      pgtype.Date{Time: dateOnly(e.OccurredOn), Valid: true},
		Memo:            strings.TrimSpace(e.Memo),
		SourceType:      e.SourceType,
		SourceID:        sourceID,
		ReversesEntryID: reverses,
		CreatedBy:       actor,
	})
	if db.IsUniqueViolation(err) && reverses.Valid {
		return PostedEntry{}, apperr.Conflict("ledger.already_reversed", "This entry has already been reversed.")
	}
	if err != nil {
		return PostedEntry{}, fmt.Errorf("insert entry: %w", err)
	}

	posted := PostedEntry{
		ID:         db.UUIDString(row.ID),
		EntryNo:    no,
		OccurredOn: dateOnly(e.OccurredOn),
		Memo:       strings.TrimSpace(e.Memo),
		SourceType: e.SourceType,
		SourceID:   e.SourceID,
		CreatedAt:  row.CreatedAt.Time,
	}
	if reverses.Valid {
		posted.ReversesID = db.UUIDString(reverses)
	}
	for i, l := range e.Lines {
		acct := byCode[l.AccountCode]
		if err := q.InsertLine(ctx, gen.InsertLineParams{
			TenantID:  tenant,
			EntryID:   row.ID,
			LineNo:    int32(i + 1),
			AccountID: acct.ID,
			Debit:     int64(l.Debit),
			Credit:    int64(l.Credit),
		}); err != nil {
			return PostedEntry{}, fmt.Errorf("insert line %d: %w", i+1, err)
		}
		posted.Lines = append(posted.Lines, PostedLine{
			LineNo: i + 1, AccountCode: l.AccountCode, AccountName: acct.Name, Debit: l.Debit, Credit: l.Credit,
		})
	}
	return posted, nil
}

// ---- reads ----------------------------------------------------------------------------

// ListAccounts returns the school's accounts. A school that has never posted has none yet.
func (s *Service) ListAccounts(ctx context.Context, p iam.Principal) ([]Account, error) {
	out := []Account{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		rows, err := gen.New(tx).ListAccounts(ctx, tenant)
		if err != nil {
			return fmt.Errorf("list accounts: %w", err)
		}
		for _, r := range rows {
			out = append(out, Account{ID: db.UUIDString(r.ID), Code: r.Code, Name: r.Name, Type: r.Type, System: r.System})
		}
		return nil
	})
	return out, err
}

// GetEntry returns one entry with its lines, or a 404 if it is not in the caller's tenant.
func (s *Service) GetEntry(ctx context.Context, p iam.Principal, id string) (PostedEntry, error) {
	notFound := apperr.NotFound("ledger.entry_not_found", "Journal entry not found.")
	entryID, err := db.ParseUUID(id)
	if err != nil {
		return PostedEntry{}, notFound
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return PostedEntry{}, err
	}
	var out PostedEntry
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		row, err := q.GetEntry(ctx, gen.GetEntryParams{TenantID: tenant, ID: entryID})
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return fmt.Errorf("get entry: %w", err)
		}
		entries, err := withLines(ctx, q, tenant, []entryHeader{headerFromGet(row)})
		if err != nil {
			return err
		}
		out = entries[0]
		return nil
	})
	return out, err
}

// ListEntries returns one page of entries, newest first. limit is 1..200.
func (s *Service) ListEntries(ctx context.Context, p iam.Principal, limit int, cursor string) (EntryPage, error) {
	if limit < 1 || limit > 200 {
		return EntryPage{}, apperr.Validation("ledger.limit_invalid", "limit must be between 1 and 200.",
			apperr.Field{Field: "limit", Message: "Between 1 and 200."})
	}
	before, err := decodeCursor(cursor)
	if err != nil {
		return EntryPage{}, err
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return EntryPage{}, err
	}
	var page EntryPage
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		params := gen.ListEntriesPageParams{TenantID: tenant, PageLimit: int32(limit + 1)}
		if before != nil {
			params.BeforeEntryNo = pgtype.Int8{Int64: *before, Valid: true}
		}
		rows, err := q.ListEntriesPage(ctx, params)
		if err != nil {
			return fmt.Errorf("list entries: %w", err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		headers := make([]entryHeader, 0, len(rows))
		for _, r := range rows {
			headers = append(headers, headerFromPage(r))
		}
		page.Items, err = withLines(ctx, q, tenant, headers)
		if err != nil {
			return err
		}
		if more && len(page.Items) > 0 {
			page.NextCursor = encodeCursor(page.Items[len(page.Items)-1].EntryNo)
		}
		return nil
	})
	return page, err
}

// TrialBalance totals every account up to and including asOf.
func (s *Service) TrialBalance(ctx context.Context, p iam.Principal, asOf time.Time) (TrialBalance, error) {
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return TrialBalance{}, err
	}
	out := TrialBalance{AsOf: dateOnly(asOf), Lines: []TrialBalanceLine{}}
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).TrialBalance(ctx, gen.TrialBalanceParams{
			TenantID: tenant, AsOf: pgtype.Date{Time: out.AsOf, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("trial balance: %w", err)
		}
		for _, r := range rows {
			line := TrialBalanceLine{Code: r.Code, Name: r.Name, Type: r.Type, Debit: money.Money(r.Debit), Credit: money.Money(r.Credit)}
			out.Lines = append(out.Lines, line)
			if out.TotalDebit, err = out.TotalDebit.Add(line.Debit); err != nil {
				return fmt.Errorf("sum debits: %w", err)
			}
			if out.TotalCredit, err = out.TotalCredit.Add(line.Credit); err != nil {
				return fmt.Errorf("sum credits: %w", err)
			}
		}
		out.Balanced = out.TotalDebit == out.TotalCredit
		return nil
	})
	return out, err
}

// ---- helpers --------------------------------------------------------------------------

type entryHeader struct {
	ID         pgtype.UUID
	EntryNo    int64
	OccurredOn time.Time
	Memo       string
	SourceType string
	SourceID   pgtype.UUID
	Reverses   pgtype.UUID
	CreatedAt  time.Time
}

func headerFromGet(r gen.GetEntryRow) entryHeader {
	return entryHeader{r.ID, r.EntryNo, r.OccurredOn.Time, r.Memo, r.SourceType, r.SourceID, r.ReversesEntryID, r.CreatedAt.Time}
}

func headerFromPage(r gen.ListEntriesPageRow) entryHeader {
	return entryHeader{r.ID, r.EntryNo, r.OccurredOn.Time, r.Memo, r.SourceType, r.SourceID, r.ReversesEntryID, r.CreatedAt.Time}
}

// withLines attaches the lines to entry headers using one query for the whole page.
func withLines(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, headers []entryHeader) ([]PostedEntry, error) {
	ids := make([]pgtype.UUID, 0, len(headers))
	for _, h := range headers {
		ids = append(ids, h.ID)
	}
	rows, err := q.ListLinesForEntries(ctx, gen.ListLinesForEntriesParams{TenantID: tenant, EntryIds: ids})
	if err != nil {
		return nil, fmt.Errorf("load lines: %w", err)
	}
	byEntry := make(map[string][]PostedLine, len(headers))
	for _, r := range rows {
		key := db.UUIDString(r.EntryID)
		byEntry[key] = append(byEntry[key], PostedLine{
			LineNo: int(r.LineNo), AccountCode: r.AccountCode, AccountName: r.AccountName,
			Debit: money.Money(r.Debit), Credit: money.Money(r.Credit),
		})
	}
	out := make([]PostedEntry, 0, len(headers))
	for _, h := range headers {
		id := db.UUIDString(h.ID)
		out = append(out, PostedEntry{
			ID: id, EntryNo: h.EntryNo, OccurredOn: h.OccurredOn, Memo: h.Memo, SourceType: h.SourceType,
			SourceID: db.UUIDString(h.SourceID), ReversesID: db.UUIDString(h.Reverses),
			Lines: byEntry[id], CreatedAt: h.CreatedAt,
		})
	}
	return out, nil
}

var errCursor = apperr.Validation("ledger.cursor_invalid", "The cursor is not valid. Start again without a cursor.")

// encodeCursor makes an opaque cursor from the last entry number on a page.
func encodeCursor(entryNo int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(entryNo, 10)))
}

// decodeCursor returns nil for an empty cursor (the first page).
func decodeCursor(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errCursor
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || n < 1 {
		return nil, errCursor
	}
	return &n, nil
}

// dateOnly drops the clock, keeping the calendar date of t.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
