package fees

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/fees/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// Challans: draft, review, release (spec §4, ADR-0017). A draft posts nothing. Release assigns the
// numbers and posts the receivable in the same transaction, and from then on the challan never changes.

const (
	maxReleaseBatch = 200
	defaultPage     = 50
	maxPage         = 200
)

// ChallanLine is one charge on a challan, copied from the fee structure when the draft was made.
type ChallanLine struct {
	FeeTypeID   string
	FeeTypeName string
	Kind        string
	Frequency   string
	Amount      money.Money
}

// Challan is a monthly voucher for one student.
type Challan struct {
	ID        string
	StudentID string
	FamilyID  string
	Month     time.Time
	DueDate   time.Time
	Status    string
	// NewAdmission marks the month of the student's first enrolment, an exception for the reviewer.
	// It is worked out when the challan is read; nothing is stored.
	NewAdmission bool
	Total        money.Money
	ChallanNo    *int64
	ConsumerNo   *string
	Lines        []ChallanLine
	ReleasedAt   *time.Time
	VoidReason   *string
	CreatedAt    time.Time
}

// GenerateItem is what happened (or would happen) for one student.
type GenerateItem struct {
	StudentID      string
	StudentName    string
	RegistrationNo string
	Outcome        string // will_generate, generated, has_challan, nothing_to_bill
	NewAdmission   bool
	Total          money.Money
	ChallanID      string
}

// GenerateResult is the outcome of a generate run.
type GenerateResult struct {
	Month     time.Time
	DueDate   time.Time
	DryRun    bool
	Generated int
	Items     []GenerateItem
}

// ChallanPage is one page of challans.
type ChallanPage struct {
	Items      []Challan
	NextCursor string
}

// ChallanFilter narrows a list.
type ChallanFilter struct {
	Month     time.Time
	Status    string
	StudentID string
	Limit     int
	Cursor    string
}

// Generate drafts the month's challans, or previews them with DryRun (REQ-FEE-003). It is idempotent:
// a student who already has a draft or released challan for the month is skipped.
func (s *Service) Generate(ctx context.Context, p iam.Principal, in GenerateInput) (GenerateResult, error) {
	month, due, err := validateGenerate(in, s.now().In(pakistanZone))
	if err != nil {
		return GenerateResult{}, err
	}
	out := GenerateResult{Month: month, DueDate: due, DryRun: in.DryRun, Items: []GenerateItem{}}
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		actor, err := db.ParseUUID(p.UserID)
		if err != nil {
			return err
		}
		monthDate := pgtype.Date{Time: month, Valid: true}
		billable, err := s.students.BillableStudents(ctx, tx, p.TenantID, month.AddDate(0, 1, -1))
		if err != nil {
			return err
		}
		liveRows, err := q.LiveChallanStudents(ctx, gen.LiveChallanStudentsParams{TenantID: tenant, Month: monthDate})
		if err != nil {
			return fmt.Errorf("list live challans: %w", err)
		}
		live := make(map[string]bool, len(liveRows))
		for _, id := range liveRows {
			live[db.UUIDString(id)] = true
		}

		type classPlan struct {
			versions []PlanVersion
			types    map[string]TypeInfo
			anchors  map[string]time.Time
		}
		plans := map[string]classPlan{}
		planFor := func(classID string) (classPlan, error) {
			if cp, ok := plans[classID]; ok {
				return cp, nil
			}
			classUUID, err := db.ParseUUID(classID)
			if err != nil {
				return classPlan{}, err
			}
			versions, types, err := s.loadPlan(ctx, q, tenant, classUUID)
			if err != nil {
				return classPlan{}, err
			}
			firsts, err := q.FirstVersionMonths(ctx, gen.FirstVersionMonthsParams{TenantID: tenant, ClassID: classUUID})
			if err != nil {
				return classPlan{}, fmt.Errorf("first plan versions: %w", err)
			}
			anchors := make(map[string]time.Time, len(firsts))
			for _, f := range firsts {
				anchors[db.UUIDString(f.FeeTypeID)] = f.FirstFrom.Time
			}
			cp := classPlan{versions: versions, types: types, anchors: anchors}
			plans[classID] = cp
			return cp, nil
		}

		for _, st := range billable {
			item := GenerateItem{
				StudentID: st.ID, StudentName: st.FullName, RegistrationNo: st.RegistrationNo,
				NewAdmission: monthStart(st.FirstEnrolledOn).Equal(month),
			}
			if live[st.ID] {
				item.Outcome = "has_challan"
				out.Items = append(out.Items, item)
				continue
			}
			cp, err := planFor(st.ClassID)
			if err != nil {
				return err
			}
			studentUUID, err := db.ParseUUID(st.ID)
			if err != nil {
				return err
			}
			overrides, err := loadOverrides(ctx, q, tenant, studentUUID)
			if err != nil {
				return err
			}
			lines := dueLines(month, resolveStructure(month, resolvePlan(month, cp.versions, cp.types), overrides), cp.anchors, st.FirstEnrolledOn)
			if len(lines) == 0 {
				item.Outcome = "nothing_to_bill"
				out.Items = append(out.Items, item)
				continue
			}
			total, err := challanTotal(lines)
			if err != nil {
				return err
			}
			item.Total = total
			if in.DryRun {
				item.Outcome = "will_generate"
				out.Items = append(out.Items, item)
				continue
			}
			familyUUID, err := db.ParseUUID(st.FamilyID)
			if err != nil {
				return err
			}
			id, err := q.InsertChallan(ctx, gen.InsertChallanParams{
				TenantID: tenant, StudentID: studentUUID, FamilyID: familyUUID, Month: monthDate,
				DueDate: pgtype.Date{Time: due, Valid: true}, Total: int64(total), CreatedBy: actor,
			})
			if errors.Is(err, pgx.ErrNoRows) { // another request drafted it first
				item.Outcome, item.Total = "has_challan", 0
				out.Items = append(out.Items, item)
				continue
			}
			if err != nil {
				return fmt.Errorf("insert challan: %w", err)
			}
			for i, l := range lines {
				feeTypeUUID, err := db.ParseUUID(l.FeeTypeID)
				if err != nil {
					return err
				}
				if err := q.InsertChallanLine(ctx, gen.InsertChallanLineParams{
					TenantID: tenant, ChallanID: id, LineNo: int32(i + 1), FeeTypeID: feeTypeUUID,
					FeeTypeName: l.Name, Kind: l.Kind, Frequency: l.Frequency, Amount: int64(l.Amount),
				}); err != nil {
					return fmt.Errorf("insert challan line: %w", err)
				}
			}
			item.Outcome, item.ChallanID = "generated", db.UUIDString(id)
			out.Generated++
			if err := s.audit(ctx, tx, p, "challan.drafted", "challan", item.ChallanID,
				map[string]any{"student_id": st.ID, "month": month.Format("2006-01"), "total": int64(total), "lines": len(lines)}); err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	return out, err
}

// Release makes draft challans real, all or nothing (REQ-FEE-004). Each gets the next challan number
// and a consumer number, and posts debit Fee Receivable, credit Fee Income in this same transaction
// (BR-FEE-008). Challans are all locked, in id order, before any number is taken, so two overlapping
// releases cannot deadlock.
func (s *Service) Release(ctx context.Context, p iam.Principal, ids []string) ([]Challan, error) {
	if len(ids) == 0 || len(ids) > maxReleaseBatch {
		return nil, apperr.Validation("fees.release_invalid", "Send between 1 and 200 challan ids.")
	}
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	if len(slices.Compact(slices.Clone(ordered))) != len(ordered) {
		return nil, apperr.Validation("fees.release_invalid", "Each challan id may appear only once.")
	}
	var out []Challan
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		actor, err := db.ParseUUID(p.UserID)
		if err != nil {
			return err
		}
		on := s.now().In(pakistanZone)
		// Lock every challan first, in id order, and only then take numbers. The number counter is one
		// row per school and is held until commit, so taking it between challan locks would let two
		// overlapping batches wait on each other (deadlock).
		uids := make([]pgtype.UUID, 0, len(ordered))
		for _, id := range ordered {
			uid, err := db.ParseUUID(id)
			if err != nil {
				return errChallanNotFound
			}
			row, err := q.LockChallan(ctx, gen.LockChallanParams{TenantID: tenant, ID: uid})
			if errors.Is(err, pgx.ErrNoRows) {
				return errChallanNotFound
			}
			if err != nil {
				return fmt.Errorf("lock challan: %w", err)
			}
			if row.Status != "draft" {
				return errNotDraft
			}
			uids = append(uids, uid)
		}
		for i, id := range ordered {
			uid := uids[i]
			no, err := q.NextChallanNo(ctx, tenant)
			if err != nil {
				return fmt.Errorf("next challan number: %w", err)
			}
			if _, err := q.ReleaseChallan(ctx, gen.ReleaseChallanParams{
				TenantID: tenant, ID: uid, ChallanNo: pgtype.Int8{Int64: no, Valid: true},
				ConsumerNo: pgtype.Text{String: consumerNumber(no), Valid: true}, ReleasedBy: actor,
			}); err != nil {
				return fmt.Errorf("release challan: %w", err)
			}
			c, err := q.GetChallan(ctx, gen.GetChallanParams{TenantID: tenant, ID: uid})
			if err != nil {
				return fmt.Errorf("read challan: %w", err)
			}
			if _, err := s.ledger.Post(ctx, tx, p, receivableEntry(id, no, money.Money(c.Total), on)); err != nil {
				return fmt.Errorf("post receivable: %w", err)
			}
			if err := s.audit(ctx, tx, p, "challan.released", "challan", id,
				map[string]any{"challan_no": no, "total": c.Total, "student_id": db.UUIDString(c.StudentID)}); err != nil {
				return err
			}
		}
		out, err = s.readChallans(ctx, tx, p.TenantID, tenant, ordered)
		return err
	})
	return out, err
}

// Void cancels a draft. A released challan is never voided here (BR-FEE-001).
func (s *Service) Void(ctx context.Context, p iam.Principal, id, reason string) (Challan, error) {
	reason, err := validateVoidReason(reason)
	if err != nil {
		return Challan{}, err
	}
	var out Challan
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		uid, err := db.ParseUUID(id)
		if err != nil {
			return errChallanNotFound
		}
		row, err := q.LockChallan(ctx, gen.LockChallanParams{TenantID: tenant, ID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			return errChallanNotFound
		}
		if err != nil {
			return fmt.Errorf("lock challan: %w", err)
		}
		if row.Status != "draft" {
			return errNotDraft
		}
		if _, err := q.VoidChallan(ctx, gen.VoidChallanParams{TenantID: tenant, ID: uid, Reason: pgtype.Text{String: reason, Valid: true}}); err != nil {
			return fmt.Errorf("void challan: %w", err)
		}
		if err := s.audit(ctx, tx, p, "challan.voided", "challan", id, map[string]any{"reason": reason}); err != nil {
			return err
		}
		list, err := s.readChallans(ctx, tx, p.TenantID, tenant, []string{id})
		if err != nil {
			return err
		}
		out = list[0]
		return nil
	})
	return out, err
}

// GetChallan returns one challan with its lines.
func (s *Service) GetChallan(ctx context.Context, p iam.Principal, id string) (Challan, error) {
	var out Challan
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		if _, err := db.ParseUUID(id); err != nil {
			return errChallanNotFound
		}
		list, err := s.readChallans(ctx, tx, p.TenantID, tenant, []string{id})
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return errChallanNotFound
		}
		out = list[0]
		return nil
	})
	return out, err
}

// ListChallans returns a page, newest first.
func (s *Service) ListChallans(ctx context.Context, p iam.Principal, f ChallanFilter) (ChallanPage, error) {
	limit := f.Limit
	if limit == 0 {
		limit = defaultPage
	}
	if limit < 1 || limit > maxPage {
		return ChallanPage{}, apperr.Validation("fees.limit_invalid", "limit must be between 1 and 200.")
	}
	if f.Status != "" && f.Status != "draft" && f.Status != "released" && f.Status != "void" {
		return ChallanPage{}, apperr.Validation("fees.status_invalid", "status must be draft, released or void.")
	}
	after, err := decodeChallanCursor(f.Cursor)
	if err != nil {
		return ChallanPage{}, err
	}
	var page ChallanPage
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		params := gen.ListChallansPageParams{TenantID: tenant, PageLimit: int32(limit + 1)}
		if !f.Month.IsZero() {
			params.Month = pgtype.Date{Time: monthStart(f.Month), Valid: true}
		}
		if f.Status != "" {
			params.Status = pgtype.Text{String: f.Status, Valid: true}
		}
		if f.StudentID != "" {
			sid, err := db.ParseUUID(f.StudentID)
			if err != nil {
				return apperr.Validation("fees.student_invalid", "student_id must be an id.")
			}
			params.StudentID = sid
		}
		if after != nil {
			params.AfterCreatedAt = pgtype.Timestamptz{Time: after.createdAt, Valid: true}
			params.AfterID = after.id
		}
		rows, err := q.ListChallansPage(ctx, params)
		if err != nil {
			return fmt.Errorf("list challans: %w", err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		ids := make([]pgtype.UUID, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		lines, err := loadChallanLines(ctx, q, tenant, ids)
		if err != nil {
			return err
		}
		page.Items = make([]Challan, 0, len(rows))
		for _, r := range rows {
			page.Items = append(page.Items, toChallan(challanRow(r), lines[db.UUIDString(r.ID)]))
		}
		if err := s.markNewAdmissions(ctx, tx, p.TenantID, page.Items); err != nil {
			return err
		}
		if more && len(rows) > 0 {
			last := rows[len(rows)-1]
			page.NextCursor = encodeChallanCursor(last.CreatedAt.Time, db.UUIDString(last.ID))
		}
		return nil
	})
	return page, err
}

// ---- helpers --------------------------------------------------------------------------

var (
	errChallanNotFound = apperr.NotFound("challans.not_found", "Challan not found.")
	errNotDraft        = apperr.Conflict("challans.not_draft", "Only a draft challan can be released or voided. Released challans never change.")
	errChallanCursor   = apperr.Validation("challans.cursor_invalid", "The cursor is not valid. Start again without a cursor.")
)

// challanRow is the shape shared by GetChallan and ListChallansPage rows.
type challanRow = gen.GetChallanRow

func toChallan(r challanRow, lines []ChallanLine) Challan {
	c := Challan{
		ID: db.UUIDString(r.ID), StudentID: db.UUIDString(r.StudentID), FamilyID: db.UUIDString(r.FamilyID),
		Month: r.Month.Time, DueDate: r.DueDate.Time, Status: r.Status, Total: money.Money(r.Total),
		Lines: lines, CreatedAt: r.CreatedAt.Time,
	}
	if lines == nil {
		c.Lines = []ChallanLine{}
	}
	if r.ChallanNo.Valid {
		c.ChallanNo = &r.ChallanNo.Int64
	}
	if r.ConsumerNo.Valid {
		c.ConsumerNo = &r.ConsumerNo.String
	}
	if r.ReleasedAt.Valid {
		c.ReleasedAt = &r.ReleasedAt.Time
	}
	if r.VoidReason.Valid {
		c.VoidReason = &r.VoidReason.String
	}
	return c
}

// readChallans loads the given challans (in the order asked) with their lines.
func (s *Service) readChallans(ctx context.Context, tx pgx.Tx, tenantID string, tenant pgtype.UUID, ids []string) ([]Challan, error) {
	q := gen.New(tx)
	uuids := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, u)
	}
	lines, err := loadChallanLines(ctx, q, tenant, uuids)
	if err != nil {
		return nil, err
	}
	out := make([]Challan, 0, len(uuids))
	for _, u := range uuids {
		r, err := q.GetChallan(ctx, gen.GetChallanParams{TenantID: tenant, ID: u})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get challan: %w", err)
		}
		out = append(out, toChallan(r, lines[db.UUIDString(u)]))
	}
	return out, s.markNewAdmissions(ctx, tx, tenantID, out)
}

// markNewAdmissions sets NewAdmission on each challan by asking the students module for the date the
// student first enrolled (ADR-0013). A challan for that month is a new admission.
func (s *Service) markNewAdmissions(ctx context.Context, tx pgx.Tx, tenantID string, list []Challan) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.StudentID)
	}
	first, err := s.students.FirstEnrolmentDates(ctx, tx, tenantID, ids)
	if err != nil {
		return err
	}
	for i := range list {
		if d, ok := first[list[i].StudentID]; ok {
			list[i].NewAdmission = monthStart(d).Equal(monthStart(list[i].Month))
		}
	}
	return nil
}

func loadChallanLines(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, ids []pgtype.UUID) (map[string][]ChallanLine, error) {
	out := map[string][]ChallanLine{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListChallanLines(ctx, gen.ListChallanLinesParams{TenantID: tenant, ChallanIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list challan lines: %w", err)
	}
	for _, r := range rows {
		key := db.UUIDString(r.ChallanID)
		out[key] = append(out[key], ChallanLine{
			FeeTypeID: db.UUIDString(r.FeeTypeID), FeeTypeName: r.FeeTypeName, Kind: r.Kind,
			Frequency: r.Frequency, Amount: money.Money(r.Amount),
		})
	}
	return out, nil
}

type challanCursor struct {
	createdAt time.Time
	id        pgtype.UUID
}

func encodeChallanCursor(createdAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeChallanCursor(s string) (*challanCursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errChallanCursor
	}
	ts, idStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, errChallanCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return nil, errChallanCursor
	}
	id, err := db.ParseUUID(idStr)
	if err != nil {
		return nil, errChallanCursor
	}
	return &challanCursor{createdAt: createdAt, id: id}, nil
}
