package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/ledger"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// pakistanZone is Asia/Karachi (UTC+5, no DST), the default report date zone (data model §1).
var pakistanZone = time.FixedZone("PKT", 5*60*60)

// Handlers for the ledger. All read-only: there is deliberately no endpoint that posts money
// (ADR-0014). Money is posted by fee and accounting actions inside their own transactions.

// ListLedgerAccounts lists the school's accounts.
func (s *Server) ListLedgerAccounts(ctx context.Context, _ gen.ListLedgerAccountsRequestObject) (gen.ListLedgerAccountsResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermLedgerView)
	if err != nil {
		return nil, err
	}
	list, err := s.ledger.ListAccounts(ctx, p)
	if err != nil {
		return nil, err
	}
	items := make([]gen.LedgerAccount, 0, len(list))
	for _, a := range list {
		id, err := uuid.Parse(a.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, gen.LedgerAccount{
			Id: id, Code: a.Code, Name: a.Name, Type: gen.LedgerAccountType(a.Type), System: a.System,
		})
	}
	return gen.ListLedgerAccounts200JSONResponse{Items: items}, nil
}

// ListJournalEntries lists entries, newest first.
func (s *Server) ListJournalEntries(ctx context.Context, req gen.ListJournalEntriesRequestObject) (gen.ListJournalEntriesResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermLedgerView)
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
	page, err := s.ledger.ListEntries(ctx, p, limit, cursor)
	if err != nil {
		return nil, err
	}
	items := make([]gen.JournalEntry, 0, len(page.Items))
	for _, e := range page.Items {
		out, err := toAPIEntry(e)
		if err != nil {
			return nil, err
		}
		items = append(items, out)
	}
	resp := gen.ListJournalEntries200JSONResponse{Items: items}
	if page.NextCursor != "" {
		next := page.NextCursor
		resp.NextCursor = &next
	}
	return resp, nil
}

// GetJournalEntry returns one entry with its lines.
func (s *Server) GetJournalEntry(ctx context.Context, req gen.GetJournalEntryRequestObject) (gen.GetJournalEntryResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermLedgerView)
	if err != nil {
		return nil, err
	}
	entry, err := s.ledger.GetEntry(ctx, p, req.EntryId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPIEntry(entry)
	if err != nil {
		return nil, err
	}
	return gen.GetJournalEntry200JSONResponse(out), nil
}

// GetTrialBalance totals every account up to a date (default: today in Pakistan).
func (s *Server) GetTrialBalance(ctx context.Context, req gen.GetTrialBalanceRequestObject) (gen.GetTrialBalanceResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermReportFinanceView)
	if err != nil {
		return nil, err
	}
	asOf := time.Now().In(pakistanZone)
	if req.Params.AsOf != nil {
		asOf = req.Params.AsOf.Time
	}
	tb, err := s.ledger.TrialBalance(ctx, p, asOf)
	if err != nil {
		return nil, err
	}
	lines := make([]gen.TrialBalanceLine, 0, len(tb.Lines))
	for _, l := range tb.Lines {
		lines = append(lines, gen.TrialBalanceLine{
			AccountCode: l.Code, AccountName: l.Name, AccountType: l.Type,
			Debit: toAPIMoney(l.Debit), Credit: toAPIMoney(l.Credit),
		})
	}
	return gen.GetTrialBalance200JSONResponse{
		AsOf:        openapitypes.Date{Time: tb.AsOf},
		Lines:       lines,
		TotalDebit:  toAPIMoney(tb.TotalDebit),
		TotalCredit: toAPIMoney(tb.TotalCredit),
		Balanced:    tb.Balanced,
	}, nil
}

// toAPIMoney is the one place an amount becomes the {amount, currency} object. The amount stays
// an integer number of paisa all the way to the client.
func toAPIMoney(m money.Money) gen.Money {
	return gen.Money{Amount: int64(m), Currency: gen.MoneyCurrency(money.Currency)}
}

func toAPIEntry(e ledger.PostedEntry) (gen.JournalEntry, error) {
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return gen.JournalEntry{}, err
	}
	out := gen.JournalEntry{
		Id:         id,
		EntryNo:    e.EntryNo,
		OccurredOn: openapitypes.Date{Time: e.OccurredOn},
		Memo:       e.Memo,
		SourceType: e.SourceType,
		CreatedAt:  e.CreatedAt,
		Lines:      make([]gen.JournalLine, 0, len(e.Lines)),
	}
	if e.SourceID != "" {
		src, err := uuid.Parse(e.SourceID)
		if err != nil {
			return gen.JournalEntry{}, err
		}
		out.SourceId = &src
	}
	if e.ReversesID != "" {
		rev, err := uuid.Parse(e.ReversesID)
		if err != nil {
			return gen.JournalEntry{}, err
		}
		out.ReversesEntryId = &rev
	}
	for _, l := range e.Lines {
		out.Lines = append(out.Lines, gen.JournalLine{
			LineNo: l.LineNo, AccountCode: l.AccountCode, AccountName: l.AccountName,
			Debit: toAPIMoney(l.Debit), Credit: toAPIMoney(l.Credit),
		})
	}
	return out, nil
}
