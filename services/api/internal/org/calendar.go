package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// The school calendar (REQ-ORG-011, ADR-0022). The org module owns it; attendance (and later fees
// reminders and payroll) ask it whether a date is a school day, inside their own transaction.

const maxCalendarRangeDays = 366

var pakistanZone = time.FixedZone("PKT", 5*60*60)

// Calendar is the weekly off days and the active entries in a range.
type Calendar struct {
	WeeklyOffDays []int
	From          time.Time
	To            time.Time
	Days          []CalendarDay
}

func toCalendarDay(id pgtype.UUID, day pgtype.Date, kind, name, status string) CalendarDay {
	return CalendarDay{ID: db.UUIDString(id), Date: day.Time, Kind: kind, Name: name, Status: status}
}

// weeklyOff reads the weekly off days. A school that has never set them has the default, Sunday; nothing is
// written by a read.
func weeklyOff(ctx context.Context, q *gen.Queries, tenant pgtype.UUID) ([]int, error) {
	days, err := q.GetCalendarSettings(ctx, tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return []int{0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get calendar settings: %w", err)
	}
	out := make([]int, 0, len(days))
	for _, d := range days {
		out = append(out, int(d))
	}
	return out, nil
}

// GetCalendar returns the weekly off days and the active holidays and working days in a range
// (default: the current month in Pakistan).
func (s *Service) GetCalendar(ctx context.Context, p iam.Principal, from, to time.Time) (Calendar, error) {
	if from.IsZero() {
		now := time.Now().In(pakistanZone)
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	if to.IsZero() {
		to = time.Date(from.Year(), from.Month()+1, 0, 0, 0, 0, 0, time.UTC) // the last day of from's month
	}
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if to.Before(from) || to.Sub(from).Hours()/24 >= maxCalendarRangeDays {
		return Calendar{}, apperr.Validation("calendar.invalid", "Choose a range of up to 366 days, ending on or after it starts.",
			apperr.Field{Field: "to", Message: "Up to 366 days after from."})
	}
	out := Calendar{From: from, To: to, Days: []CalendarDay{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		if out.WeeklyOffDays, err = weeklyOff(ctx, q, tenant); err != nil {
			return err
		}
		rows, err := q.ListCalendarDays(ctx, gen.ListCalendarDaysParams{
			TenantID: tenant, FromDay: pgtype.Date{Time: from, Valid: true}, ToDay: pgtype.Date{Time: to, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("list calendar days: %w", err)
		}
		for _, r := range rows {
			out.Days = append(out.Days, toCalendarDay(r.ID, r.Day, r.Kind, r.Name, r.Status))
		}
		return nil
	})
	return out, err
}

// SetWeeklyOff sets the weekly off days (0 Sunday to 6 Saturday), audited.
func (s *Service) SetWeeklyOff(ctx context.Context, p iam.Principal, days []int) ([]int, error) {
	days, err := validateWeeklyOff(days)
	if err != nil {
		return nil, err
	}
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
		small := make([]int16, 0, len(days))
		for _, d := range days {
			small = append(small, int16(d))
		}
		if err := q.SetWeeklyOff(ctx, gen.SetWeeklyOffParams{TenantID: tenant, WeeklyOffDays: small, UpdatedBy: actor}); err != nil {
			return fmt.Errorf("set weekly off: %w", err)
		}
		return s.audit(ctx, tx, p, "calendar.weekly_off_set", "calendar", p.TenantID, map[string]any{"days": days})
	})
	return days, err
}

// AddCalendarDays adds a holiday or a special working day for one date or a range. A date that already has
// an active entry is a 409 and nothing is added.
func (s *Service) AddCalendarDays(ctx context.Context, p iam.Principal, in CalendarDaysInput) ([]CalendarDay, error) {
	dates, in, err := validateCalendarDays(in)
	if err != nil {
		return nil, err
	}
	var out []CalendarDay
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
		for _, d := range dates {
			row, err := q.InsertCalendarDay(ctx, gen.InsertCalendarDayParams{
				TenantID: tenant, Day: pgtype.Date{Time: d, Valid: true}, Kind: in.Kind, Name: in.Name, CreatedBy: actor,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Conflict("calendar.day_exists", d.Format("2006-01-02")+" already has an entry. Remove it first.")
			}
			if err != nil {
				return fmt.Errorf("insert calendar day: %w", err)
			}
			cd := toCalendarDay(row.ID, row.Day, row.Kind, row.Name, row.Status)
			out = append(out, cd)
			if err := s.audit(ctx, tx, p, "calendar.day_added", "calendar_day", cd.ID,
				map[string]any{"date": d.Format("2006-01-02"), "kind": in.Kind, "name": in.Name}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// RemoveCalendarDay stops an entry counting. It is kept as history.
func (s *Service) RemoveCalendarDay(ctx context.Context, p iam.Principal, id string) (CalendarDay, error) {
	var out CalendarDay
	notFound := apperr.NotFound("calendar.day_not_found", "Calendar entry not found.")
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
		uid, err := db.ParseUUID(id)
		if err != nil {
			return notFound
		}
		row, err := q.RemoveCalendarDay(ctx, gen.RemoveCalendarDayParams{TenantID: tenant, ID: uid, RemovedBy: actor})
		if errors.Is(err, pgx.ErrNoRows) {
			// Either it does not exist in this school, or it was already removed.
			if _, gerr := q.GetCalendarDay(ctx, gen.GetCalendarDayParams{TenantID: tenant, ID: uid}); errors.Is(gerr, pgx.ErrNoRows) {
				return notFound
			} else if gerr != nil {
				return fmt.Errorf("get calendar day: %w", gerr)
			}
			return apperr.Conflict("calendar.already_removed", "This entry has already been removed.")
		}
		if err != nil {
			return fmt.Errorf("remove calendar day: %w", err)
		}
		out = toCalendarDay(row.ID, row.Day, row.Kind, row.Name, "removed")
		return s.audit(ctx, tx, p, "calendar.day_removed", "calendar_day", id,
			map[string]any{"date": row.Day.Time.Format("2006-01-02"), "kind": row.Kind, "name": row.Name})
	})
	return out, err
}

// ---- used by other modules, inside their own tenant transaction -----------------------

// SchoolDay says whether a date is a school day and, if not, why (a holiday's name, or "Weekly off day").
func (s *Service) SchoolDay(ctx context.Context, tx pgx.Tx, tenantID string, date time.Time) (SchoolDayInfo, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return SchoolDayInfo{}, err
	}
	q := gen.New(tx)
	off, err := weeklyOff(ctx, q, tenant)
	if err != nil {
		return SchoolDayInfo{}, err
	}
	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	var entry *CalendarDay
	row, err := q.ActiveCalendarDay(ctx, gen.ActiveCalendarDayParams{TenantID: tenant, Day: pgtype.Date{Time: day, Valid: true}})
	switch {
	case err == nil:
		cd := toCalendarDay(row.ID, row.Day, row.Kind, row.Name, row.Status)
		entry = &cd
	case !errors.Is(err, pgx.ErrNoRows):
		return SchoolDayInfo{}, fmt.Errorf("get calendar day: %w", err)
	}
	ok, why := isSchoolDay(day, off, entry)
	return SchoolDayInfo{SchoolDay: ok, Reason: why}, nil
}

// ActiveSections lists every active section of an active class, for modules that visit each one.
func (s *Service) ActiveSections(ctx context.Context, tx pgx.Tx, tenantID string) ([]Section, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := gen.New(tx).ListActiveSectionsAll(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("list active sections: %w", err)
	}
	out := make([]Section, 0, len(rows))
	for _, r := range rows {
		out = append(out, Section{ID: db.UUIDString(r.ID), ClassID: db.UUIDString(r.ClassID), ClassName: r.ClassName, Name: r.Name, Shift: r.Shift, Status: r.Status})
	}
	return out, nil
}
