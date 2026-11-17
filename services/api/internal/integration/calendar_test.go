package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/org"
)

// REQ-ORG-011, US-ORG-004: weekly off days, holidays over a range, working days; removal keeps history.
func TestREQ_ORG_011_SchoolCalendar(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "calendar")
	p := mustPrincipal(t, f, signIn(t, f, mobile, slug))
	_ = slug

	cal, err := f.org.GetCalendar(ctx, p, time.Time{}, time.Time{})
	if err != nil || len(cal.WeeklyOffDays) != 1 || cal.WeeklyOffDays[0] != 0 {
		t.Fatalf("default is Sunday off: %v %+v", err, cal)
	}
	if off, err := f.org.SetWeeklyOff(ctx, p, []int{6, 5, 5}); err != nil || len(off) != 2 {
		t.Fatalf("set weekly off: %v %v", off, err)
	}
	if _, err := f.org.SetWeeklyOff(ctx, p, []int{0, 1, 2, 3, 4, 5, 6}); err == nil {
		t.Fatal("seven off days must be refused")
	}

	from := time.Date(2030, time.March, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, time.March, 24, 0, 0, 0, 0, time.UTC)
	days, err := f.org.AddCalendarDays(ctx, p, org.CalendarDaysInput{From: from, To: &to, Kind: "holiday", Name: "Eid"})
	if err != nil || len(days) != 5 {
		t.Fatalf("a range of five: %v %+v", err, days)
	}
	// One clashing date refuses the whole request and adds nothing.
	later := time.Date(2030, time.March, 26, 0, 0, 0, 0, time.UTC)
	_, err = f.org.AddCalendarDays(ctx, p, org.CalendarDaysInput{From: later, To: &to, Kind: "holiday", Name: "Overlap"})
	if err == nil {
		t.Fatal("an inverted range must be refused")
	}
	overlapEnd := time.Date(2030, time.March, 22, 0, 0, 0, 0, time.UTC)
	before := time.Date(2030, time.March, 18, 0, 0, 0, 0, time.UTC)
	if _, err := f.org.AddCalendarDays(ctx, p, org.CalendarDaysInput{From: before, To: &overlapEnd, Kind: "holiday", Name: "Overlap"}); errCode(t, err) != "calendar.day_exists" {
		t.Fatalf("a clash: %v", err)
	}
	got, _ := f.org.GetCalendar(ctx, p, before, overlapEnd)
	if len(got.Days) != 3 { // only Eid's 20, 21, 22: nothing from the refused request
		t.Fatalf("a refused request adds nothing: %d days", len(got.Days))
	}

	// Remove one: history is kept, and the date is free again.
	if removed, err := f.org.RemoveCalendarDay(ctx, p, days[0].ID); err != nil || removed.Status != "removed" {
		t.Fatalf("remove: %v %+v", err, removed)
	}
	if _, err := f.org.RemoveCalendarDay(ctx, p, days[0].ID); errCode(t, err) != "calendar.already_removed" {
		t.Fatalf("remove twice: %v", err)
	}
	if _, err := f.org.RemoveCalendarDay(ctx, p, "5c9c6c26-e0e4-4938-af5c-f6fc35688270"); errCode(t, err) != "calendar.day_not_found" {
		t.Fatalf("unknown entry: %v", err)
	}
	if _, err := f.org.AddCalendarDays(ctx, p, org.CalendarDaysInput{From: from, Kind: "working", Name: "Working day"}); err != nil {
		t.Fatalf("the freed date: %v", err)
	}
	var kept int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM calendar_days WHERE tenant_id = $1 AND status = 'removed'`, tenantID).Scan(&kept)
	if kept != 1 {
		t.Fatalf("the removed entry is kept: %d", kept)
	}
	// Edits and deletes are refused by the database, for every role.
	for name, stmt := range map[string]string{
		"rename an entry":      `UPDATE calendar_days SET name = 'x' WHERE tenant_id = $1`,
		"delete an entry":      `DELETE FROM calendar_days WHERE tenant_id = $1`,
		"change a removed one": `UPDATE calendar_days SET status = 'active', removed_at = NULL WHERE tenant_id = $1 AND status = 'removed'`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, tenantID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
	// Two audited changes of each kind.
	var n int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action LIKE 'calendar.%'`, tenantID).Scan(&n)
	if n < 8 {
		t.Fatalf("calendar changes are audited, got %d rows", n)
	}
}

func TestREQ_ORG_011_CalendarOverHTTPAndIsolation(t *testing.T) {
	f := newFixture(t)
	a := newAttendWorld(t, f, "cal-a", 1)
	b := newAttendWorld(t, f, "cal-b", 1)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)

	if rec := do(t, h, http.MethodPost, "/api/v1/calendar/days", a.token, `{"from":"2030-08-14","kind":"holiday","name":"Independence Day"}`); rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "Independence Day") {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/calendar/days", a.token, `{"from":"2030-08-14","kind":"holiday","name":"Again"}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "calendar.day_exists") {
		t.Fatalf("clash: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/calendar/days", a.token, `{"from":"2030-08-14","kind":"party","name":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad kind: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/calendar?from=2030-08-01&to=2030-08-31", a.token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Independence Day") {
		t.Fatalf("read: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/calendar?from=2030-08-01&to=2030-08-31", b.token, ""); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Independence Day") {
		t.Fatalf("school B must not see A's holiday: %s", rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/calendar?from=2030-01-01&to=2032-01-01", a.token, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("a range over 366 days: %d", rec.Code)
	}
	// The limit is 366 days, both ends counted: 2030-01-01 to 2031-01-01 is 366 days and fits; one more does not.
	if rec := do(t, h, http.MethodGet, "/api/v1/calendar?from=2030-01-01&to=2031-01-01", a.token, ""); rec.Code != http.StatusOK {
		t.Fatalf("366 days: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/calendar?from=2030-01-01&to=2031-01-02", a.token, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("367 days: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/calendar/weekly-off", a.token, `{"days":[5]}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"weekly_off_days":[5]`) {
		t.Fatalf("weekly off: %d %s", rec.Code, rec.Body)
	}
}
