package attendance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/attendance/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// Student ID cards and the gate scan (ADR-0023). The scan is a log kept beside the class register: the
// register stays the official record, the arrival shows beside it, and disagreements are listed for a person.

// Card is a student's ID card. Code is set for the active card only.
type Card struct {
	ID             string
	StudentID      string
	Status         string
	Code           *string
	IssuedAt       time.Time
	DisabledAt     *time.Time
	DisabledReason *string
}

// ScanInput is a scanned card. ScannedAt and ClientScanID come from an offline computer.
type ScanInput struct {
	Code         string
	ScannedAt    *time.Time
	ClientScanID string
}

// ScanResult is what the gate screen shows.
type ScanResult struct {
	Outcome   string // accepted or duplicate
	Status    string // the arrival status for the day: present or late
	ScannedAt time.Time
	ArrivalAt time.Time
	Student   ScanStudent
}

// ScanStudent is who was scanned.
type ScanStudent struct {
	ID             string
	RegistrationNo string
	FullName       string
	SectionName    string
}

// GateScan is one stored scan.
type GateScan struct {
	ID             string
	StudentID      string
	RegistrationNo string
	FullName       string
	SectionID      string
	SectionName    string
	ScannedAt      time.Time
	Status         string
	IsArrival      bool
}

// Mismatch is a place where the gate and the class register disagree.
type Mismatch struct {
	StudentID      string
	RegistrationNo string
	FullName       string
	SectionID      string
	SectionName    string
	Kind           string // scanned_but_absent or present_but_not_scanned
	RegisterStatus string
	GateArrivedAt  *time.Time
}

var errCardNotFound = apperr.NotFound("attendance.card_not_found", "Card not found.")

func toCard(r gen.GetCardRow, code *string) Card {
	c := Card{ID: db.UUIDString(r.ID), StudentID: db.UUIDString(r.StudentID), Status: r.Status, IssuedAt: r.IssuedAt.Time}
	if r.Status == "active" {
		c.Code = code
	}
	if r.DisabledAt.Valid {
		t := r.DisabledAt.Time
		c.DisabledAt = &t
	}
	if r.DisabledReason.Valid {
		c.DisabledReason = &r.DisabledReason.String
	}
	return c
}

// IssueCard gives a student a new ID card and disables their previous one in the same step, so a lost card
// stops working at once.
func (s *Service) IssueCard(ctx context.Context, p iam.Principal, studentID string) (Card, error) {
	var out Card
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		if err := s.students.RequireStudent(ctx, tx, p.TenantID, studentID); err != nil {
			return err
		}
		student, err := db.ParseUUID(studentID)
		if err != nil {
			return err
		}
		replaced, err := q.DisableActiveCards(ctx, gen.DisableActiveCardsParams{
			TenantID: tenant, StudentID: student, DisabledBy: actor, Reason: pgtype.Text{String: "replaced by a new card", Valid: true},
		})
		if err != nil {
			return fmt.Errorf("disable old card: %w", err)
		}
		row, err := q.InsertCard(ctx, gen.InsertCardParams{TenantID: tenant, StudentID: student, IssuedBy: actor})
		if err != nil {
			return fmt.Errorf("insert card: %w", err)
		}
		id := db.UUIDString(row.ID)
		code := cardCode(s.cardKey, p.TenantID, id)
		out = Card{ID: id, StudentID: studentID, Status: "active", Code: &code, IssuedAt: row.IssuedAt.Time}
		return s.audit(ctx, tx, p, "card.issued", "student_card", id, map[string]any{"student_id": studentID, "replaced": replaced})
	})
	return out, err
}

// ListCards returns a student's cards, newest first. Only the active card shows its code.
func (s *Service) ListCards(ctx context.Context, p iam.Principal, studentID string) ([]Card, error) {
	out := []Card{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		if err := s.students.RequireStudent(ctx, tx, p.TenantID, studentID); err != nil {
			return err
		}
		student, err := db.ParseUUID(studentID)
		if err != nil {
			return err
		}
		rows, err := q.ListCardsForStudent(ctx, gen.ListCardsForStudentParams{TenantID: tenant, StudentID: student})
		if err != nil {
			return fmt.Errorf("list cards: %w", err)
		}
		for _, r := range rows {
			code := cardCode(s.cardKey, p.TenantID, db.UUIDString(r.ID))
			out = append(out, toCard(gen.GetCardRow(r), &code))
		}
		return nil
	})
	return out, err
}

// DisableCard disables a card for good (lost, stolen). A scan of it marks nothing afterwards.
func (s *Service) DisableCard(ctx context.Context, p iam.Principal, cardID, reason string) (Card, error) {
	reason, err := validateCardReason(reason)
	if err != nil {
		return Card{}, err
	}
	var out Card
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(cardID)
		if err != nil {
			return errCardNotFound
		}
		locked, err := q.LockCard(ctx, gen.LockCardParams{TenantID: tenant, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return errCardNotFound
		}
		if err != nil {
			return fmt.Errorf("lock card: %w", err)
		}
		if locked.Status != "active" {
			return apperr.Conflict("attendance.card_already_disabled", "This card is already disabled.")
		}
		n, err := q.DisableCard(ctx, gen.DisableCardParams{TenantID: tenant, ID: id, DisabledBy: actor, Reason: pgtype.Text{String: reason, Valid: true}})
		if err != nil {
			return fmt.Errorf("disable card: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("disable card: %d rows changed, want 1", n)
		}
		if err := s.audit(ctx, tx, p, "card.disabled", "student_card", cardID, map[string]any{"student_id": db.UUIDString(locked.StudentID), "reason": reason}); err != nil {
			return err
		}
		row, err := q.GetCard(ctx, gen.GetCardParams{TenantID: tenant, ID: id})
		if err != nil {
			return fmt.Errorf("read card: %w", err)
		}
		out = toCard(row, nil)
		return nil
	})
	return out, err
}

// Scan records a student's arrival from a scanned card (US-ATT-003). The code is checked for its signature
// before anything is looked up. Nothing is marked for a forged, unknown or disabled card, on a non-school
// day, or for a student who is not enrolled in a section that day. A repeat within 2 minutes is ignored, and
// a client scan id makes an upload from an offline computer safe to repeat. Every accepted scan is audited
// with the signed-in user (BR-ATT-006), and so is a refusal of a genuine card; a forged code is logged.
func (s *Service) Scan(ctx context.Context, p iam.Principal, in ScanInput) (ScanResult, error) {
	if in.Code == "" {
		return ScanResult{}, apperr.Validation("attendance.scan_invalid", "The scanned code is required.",
			apperr.Field{Field: "code", Message: "Required."})
	}
	cardID, ok := verifyCardCode(s.cardKey, p.TenantID, in.Code)
	if !ok {
		slog.WarnContext(ctx, "gate scan refused: the code is not a valid card code", "user_id", p.UserID)
		return ScanResult{}, apperr.NotFound("attendance.card_invalid", "This card is not valid.")
	}
	var clientID pgtype.UUID
	var err error
	if in.ClientScanID != "" {
		if clientID, err = db.ParseUUID(in.ClientScanID); err != nil {
			return ScanResult{}, apperr.Validation("attendance.scan_invalid", "client_scan_id must be an id.",
				apperr.Field{Field: "client_scan_id", Message: "Must be an id."})
		}
	}
	out, err := s.scan(ctx, p, in, cardID, clientID)
	if err != nil {
		if e, ok := apperr.As(err); ok {
			switch e.Code {
			case "attendance.card_disabled", "attendance.not_enrolled", "attendance.not_school_day":
				s.auditRefusal(ctx, p, cardID, e.Code)
			}
		}
	}
	return out, err
}

// auditRefusal records that a genuine card was refused and why, in its own transaction (the scan's own was
// rolled back). Best effort: a failure here must not hide the refusal from the gate.
func (s *Service) auditRefusal(ctx context.Context, p iam.Principal, cardID, reason string) {
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		return s.audit(ctx, tx, p, "attendance.scan_refused", "student_card", cardID, map[string]any{"reason": reason})
	})
	if err != nil {
		slog.ErrorContext(ctx, "could not audit a refused gate scan", "err", err)
	}
}

func (s *Service) scan(ctx context.Context, p iam.Principal, in ScanInput, cardID string, clientID pgtype.UUID) (ScanResult, error) {
	var out ScanResult
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		cfg, err := loadSettings(ctx, q, p.TenantID)
		if err != nil {
			return err
		}
		// An offline computer may be away for days: the limit is the correction window.
		at, err := validateScanTime(in.ScannedAt, s.now(), time.Duration(cfg.EditWindowDays)*24*time.Hour)
		if err != nil {
			return err
		}
		local := at.In(pakistanZone)
		day := dateOnly(local)
		pgDay := pgtype.Date{Time: day, Valid: true}
		cid, err := db.ParseUUID(cardID)
		if err != nil {
			return err
		}

		// The same upload twice is one scan: return what the first one said, for the day it was made. A client id
		// that is reused for another card is a mistake, refused rather than answered with the wrong student.
		if replay, ok, err := s.replay(ctx, tx, q, p, tenant, clientID, cardID); err != nil {
			return err
		} else if ok {
			out = replay
			return nil
		}
		card, err := q.ShareCard(ctx, gen.ShareCardParams{TenantID: tenant, ID: cid})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("attendance.card_invalid", "This card is not valid.")
		}
		if err != nil {
			return fmt.Errorf("get card: %w", err)
		}
		if card.Status != "active" {
			return apperr.Conflict("attendance.card_disabled", "This card has been disabled.")
		}
		schoolDay, err := s.org.SchoolDay(ctx, tx, p.TenantID, day)
		if err != nil {
			return err
		}
		if !schoolDay.SchoolDay {
			return apperr.Conflict("attendance.not_school_day", "No attendance on this day: "+schoolDay.Reason+". (BR-ATT-001)")
		}
		studentID := db.UUIDString(card.StudentID)
		sectionID, err := s.students.SectionOnDay(ctx, tx, p.TenantID, studentID, day)
		if err != nil {
			return err
		}
		if sectionID == "" {
			return apperr.Conflict("attendance.not_enrolled", "This student is not enrolled in a section on this day.")
		}
		section, err := db.ParseUUID(sectionID)
		if err != nil {
			return err
		}
		if err := q.LockScanKey(ctx, gen.LockScanKeyParams{StudentID: studentID, Day: day.Format("2006-01-02")}); err != nil {
			return fmt.Errorf("lock scan key: %w", err)
		}
		// Re-check after the lock: another upload of the same id may just have committed.
		if replay, ok, err := s.replay(ctx, tx, q, p, tenant, clientID, cardID); err != nil {
			return err
		} else if ok {
			out = replay
			return nil
		}
		times, err := q.ScanTimesForStudentDay(ctx, gen.ScanTimesForStudentDayParams{TenantID: tenant, StudentID: card.StudentID, Day: pgDay})
		if err != nil {
			return fmt.Errorf("earlier scans: %w", err)
		}
		earlier := make([]time.Time, 0, len(times))
		for _, t := range times {
			earlier = append(earlier, t.Time)
		}
		if isDuplicateScan(earlier, at) {
			out, err = s.scanResult(ctx, tx, q, p, "duplicate", studentID, section, at, pgDay)
			return err
		}
		sections, err := s.org.DescribeSections(ctx, tx, p.TenantID, []string{sectionID})
		if err != nil {
			return err
		}
		start := cfg.MorningStart
		if sections[sectionID].Shift == "evening" {
			start = cfg.EveningStart
		}
		h, m, _ := parseClock(start)
		status := scanStatus(local, h*60+m, cfg.LateAfterMinutes)
		scanID, err := q.InsertScan(ctx, gen.InsertScanParams{
			TenantID: tenant, StudentID: card.StudentID, CardID: cid, SectionID: section, Day: pgDay,
			ScannedAt: pgtype.Timestamptz{Time: at, Valid: true}, Status: status, ScannedBy: actor, ClientScanID: clientID,
		})
		if db.IsUniqueViolation(err) {
			return errClientScanReused
		}
		if err != nil {
			return fmt.Errorf("insert scan: %w", err)
		}
		if err := s.audit(ctx, tx, p, "attendance.scan", "gate_scan", db.UUIDString(scanID), map[string]any{
			"student_id": studentID, "status": status, "scanned_at": at.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		out, err = s.scanResult(ctx, tx, q, p, "accepted", studentID, section, at, pgDay)
		return err
	})
	return out, err
}

var errClientScanReused = apperr.Conflict("attendance.client_scan_id_reused", "This client_scan_id was already used for a different card. Use a new id for each scan.")

// replay answers a repeated upload from the scan that was stored, for the day that scan was made (not the day
// of the retry), or says it is not a repeat. A client id already used for a different card is refused.
func (s *Service) replay(ctx context.Context, tx pgx.Tx, q *gen.Queries, p iam.Principal, tenant, clientID pgtype.UUID, cardID string) (ScanResult, bool, error) {
	if !clientID.Valid {
		return ScanResult{}, false, nil
	}
	prev, err := q.ScanByClientID(ctx, gen.ScanByClientIDParams{TenantID: tenant, ClientScanID: clientID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ScanResult{}, false, nil
	}
	if err != nil {
		return ScanResult{}, false, fmt.Errorf("scan by client id: %w", err)
	}
	if db.UUIDString(prev.CardID) != cardID {
		return ScanResult{}, false, errClientScanReused
	}
	day := pgtype.Date{Time: dateOnly(prev.ScannedAt.Time.In(pakistanZone)), Valid: true}
	r, err := s.scanResult(ctx, tx, q, p, "accepted", db.UUIDString(prev.StudentID), prev.SectionID, prev.ScannedAt.Time, day)
	return r, err == nil, err
}

// scanResult builds what the gate screen shows: who, where, and the arrival (the first scan of the day).
func (s *Service) scanResult(ctx context.Context, tx pgx.Tx, q *gen.Queries, p iam.Principal, outcome, studentID string, section pgtype.UUID, scannedAt time.Time, day pgtype.Date) (ScanResult, error) {
	tenant, _, err := ids(p)
	if err != nil {
		return ScanResult{}, err
	}
	student, err := db.ParseUUID(studentID)
	if err != nil {
		return ScanResult{}, err
	}
	first, err := q.FirstScanOfDay(ctx, gen.FirstScanOfDayParams{TenantID: tenant, StudentID: student, Day: day})
	if err != nil {
		return ScanResult{}, fmt.Errorf("first scan: %w", err)
	}
	briefs, err := s.students.StudentBriefs(ctx, tx, p.TenantID, []string{studentID})
	if err != nil {
		return ScanResult{}, err
	}
	secID := db.UUIDString(section)
	sections, err := s.org.DescribeSections(ctx, tx, p.TenantID, []string{secID})
	if err != nil {
		return ScanResult{}, err
	}
	b := briefs[studentID]
	return ScanResult{
		Outcome: outcome, Status: first.Status, ScannedAt: scannedAt.UTC(), ArrivalAt: first.ScannedAt.Time.UTC(),
		Student: ScanStudent{ID: studentID, RegistrationNo: b.RegistrationNo, FullName: b.FullName, SectionName: sectionLabel(sections[secID])},
	}, nil
}

type arrival struct {
	at     time.Time
	status string
}

// arrivalsOnDay returns the first gate scan of the day for each student in the given current records.
func arrivalsOnDay(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, day pgtype.Date, recs []gen.CurrentRecordsRow) (map[string]arrival, error) {
	out := map[string]arrival{}
	if len(recs) == 0 {
		return out, nil
	}
	ids := make([]pgtype.UUID, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.StudentID)
	}
	rows, err := q.ArrivalsOnDay(ctx, gen.ArrivalsOnDayParams{TenantID: tenant, Day: day, StudentIds: ids})
	if err != nil {
		return nil, fmt.Errorf("arrivals: %w", err)
	}
	for _, r := range rows {
		out[db.UUIDString(r.StudentID)] = arrival{at: r.ScannedAt.Time, status: r.Status}
	}
	return out, nil
}

// GateScans lists a day's scans, oldest first, marking each student's first as the arrival.
func (s *Service) GateScans(ctx context.Context, p iam.Principal, date time.Time) ([]GateScan, error) {
	if date.IsZero() {
		return nil, apperr.Validation("attendance.date_invalid", "A date is required.", apperr.Field{Field: "date", Message: "Required."})
	}
	out := []GateScan{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		rows, err := q.ListScansForDay(ctx, gen.ListScansForDayParams{TenantID: tenant, Day: pgtype.Date{Time: dateOnly(date), Valid: true}})
		if err != nil {
			return fmt.Errorf("list scans: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		studentIDs, secIDs := make([]string, 0, len(rows)), make([]string, 0, len(rows))
		for _, r := range rows {
			studentIDs = append(studentIDs, db.UUIDString(r.StudentID))
			secIDs = append(secIDs, db.UUIDString(r.SectionID))
		}
		briefs, err := s.students.StudentBriefs(ctx, tx, p.TenantID, studentIDs)
		if err != nil {
			return err
		}
		sections, err := s.org.DescribeSections(ctx, tx, p.TenantID, secIDs)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, r := range rows {
			stu, sec := db.UUIDString(r.StudentID), db.UUIDString(r.SectionID)
			out = append(out, GateScan{
				ID: db.UUIDString(r.ID), StudentID: stu, RegistrationNo: briefs[stu].RegistrationNo, FullName: briefs[stu].FullName,
				SectionID: sec, SectionName: sectionLabel(sections[sec]), ScannedAt: r.ScannedAt.Time, Status: r.Status, IsArrival: !seen[stu],
			})
			seen[stu] = true
		}
		return nil
	})
	return out, err
}

// Mismatches lists where the gate and the submitted class registers disagree (REQ-ATT-017). Nothing is changed.
func (s *Service) Mismatches(ctx context.Context, p iam.Principal, date time.Time) ([]Mismatch, error) {
	if date.IsZero() {
		return nil, apperr.Validation("attendance.date_invalid", "A date is required.", apperr.Field{Field: "date", Message: "Required."})
	}
	out := []Mismatch{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		rows, err := q.MismatchesOnDay(ctx, gen.MismatchesOnDayParams{TenantID: tenant, Day: pgtype.Date{Time: dateOnly(date), Valid: true}})
		if err != nil {
			return fmt.Errorf("mismatches: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		studentIDs, secIDs := make([]string, 0, len(rows)), make([]string, 0, len(rows))
		for _, r := range rows {
			studentIDs = append(studentIDs, db.UUIDString(r.StudentID))
			secIDs = append(secIDs, db.UUIDString(r.SectionID))
		}
		briefs, err := s.students.StudentBriefs(ctx, tx, p.TenantID, studentIDs)
		if err != nil {
			return err
		}
		sections, err := s.org.DescribeSections(ctx, tx, p.TenantID, secIDs)
		if err != nil {
			return err
		}
		for _, r := range rows {
			stu, sec := db.UUIDString(r.StudentID), db.UUIDString(r.SectionID)
			m := Mismatch{
				StudentID: stu, RegistrationNo: briefs[stu].RegistrationNo, FullName: briefs[stu].FullName, SectionID: sec,
				SectionName: sectionLabel(sections[sec]), Kind: r.Kind, RegisterStatus: r.Status,
			}
			if r.GateArrivedAt.Valid {
				t := r.GateArrivedAt.Time
				m.GateArrivedAt = &t
			}
			out = append(out, m)
		}
		return nil
	})
	return out, err
}
