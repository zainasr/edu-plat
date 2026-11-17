package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/attendance"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// gateWorld is an attendance world with a receptionist at the gate and everyone enrolled for the last week,
// so a scan taken at any time in the last day finds a school day and a roster.
type gateWorld struct {
	attendWorld
	gate iam.Principal
}

func newGateWorld(t *testing.T, f *fixture, label string, n int) gateWorld {
	t.Helper()
	w := newAttendWorld(t, f, label, n)
	w.backdate(t, f, ago(7))
	return gateWorld{attendWorld: w, gate: mustPrincipal(t, f, addStaff(t, f, w.tenantID, w.slug, "receptionist"))}
}

// scanAt is a scan time inside the allowed 24 hours: an hour ago, whatever time it is now.
func scanAt() time.Time { return time.Now().In(karachi).Add(-time.Hour).Truncate(time.Second) }

func minuteOfDay(t time.Time) int { l := t.In(karachi); return l.Hour()*60 + l.Minute() }

func clockText(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }

// startShiftAt sets the morning shift to start at the given minute of the day, with the given grace.
func (w gateWorld) startShiftAt(t *testing.T, f *fixture, startMin, grace int) {
	t.Helper()
	_, err := f.attendance.SetSettings(context.Background(), w.owner, attendance.SettingsInput{
		RegisterDueTime: "10:00", EditWindowDays: 7, LowAttendancePercent: 75,
		MorningStart: clockText(startMin), EveningStart: "23:00", LateAfterMinutes: grace,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (w gateWorld) card(t *testing.T, f *fixture, student string) attendance.Card {
	t.Helper()
	c, err := f.attendance.IssueCard(context.Background(), w.owner, student)
	if err != nil || c.Code == nil {
		t.Fatalf("issue card: %v", err)
	}
	return c
}

func (w gateWorld) scan(t *testing.T, f *fixture, code string, at time.Time, clientID string) (attendance.ScanResult, error) {
	return f.attendance.Scan(context.Background(), w.gate, attendance.ScanInput{Code: code, ScannedAt: &at, ClientScanID: clientID})
}

// US-ATT-003, REQ-ATT-011: a valid card is accepted with the student's details, present before the cutoff
// and late after it; the arrival shows on the class register.
func TestUS_ATT_003_GateScan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-basic", 3)
	at := scanAt()
	mod := minuteOfDay(at)
	onTime, late := w.card(t, f, w.students[0]), w.card(t, f, w.students[1])

	w.startShiftAt(t, f, min(mod+30, 1439), 0) // the shift starts after the scan: on time
	r, err := w.scan(t, f, *onTime.Code, at, "")
	if err != nil || r.Outcome != "accepted" || r.Status != "present" || r.Student.ID != w.students[0] || r.Student.FullName == "" ||
		r.Student.RegistrationNo == "" || !strings.Contains(r.Student.SectionName, "Class 5") || !r.ArrivalAt.Equal(at) {
		t.Fatalf("on time: %v %+v", err, r)
	}
	w.startShiftAt(t, f, max(mod-30, 0), 0) // the shift started before it: late
	r, err = w.scan(t, f, *late.Code, at.Add(time.Second), "")
	if err != nil || r.Status != "late" || r.Student.ID != w.students[1] {
		t.Fatalf("late: %v %+v", err, r)
	}

	// The class register shows the arrival beside each student, and is otherwise unchanged (REQ-ATT-017).
	day := at.In(karachi)
	reg := w.open(t, f, time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC))
	for _, rec := range reg.Records {
		switch rec.StudentID {
		case w.students[0]:
			if rec.GateArrivedAt == nil || rec.GateStatus != "present" || rec.Status != "present" {
				t.Fatalf("student 0: %+v", rec)
			}
		case w.students[1]:
			if rec.GateArrivedAt == nil || rec.GateStatus != "late" || rec.Status != "present" {
				t.Fatalf("a late scan does not change the register: %+v", rec)
			}
		default:
			if rec.GateArrivedAt != nil || rec.GateStatus != "" {
				t.Fatalf("an unscanned student has no arrival: %+v", rec)
			}
		}
	}
	// Every scan is audited with the user, and the log keeps who scanned.
	var audits int
	var scannedBy string
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'attendance.scan' AND actor_user_id = $2`, w.tenantID, w.gate.UserID).Scan(&audits)
	_ = f.admin.QueryRow(ctx, `SELECT scanned_by::text FROM gate_scans WHERE tenant_id = $1 LIMIT 1`, w.tenantID).Scan(&scannedBy)
	if audits != 2 || scannedBy != w.gate.UserID {
		t.Fatalf("audited with the signed-in user: %d rows, scanned_by %s", audits, scannedBy)
	}
	// The grace is inclusive: exactly start + grace is still on time.
	w.startShiftAt(t, f, max(mod-10, 0), 10)
	third := w.card(t, f, w.students[2])
	if r, err := w.scan(t, f, *third.Code, at.Add(2*time.Second), ""); err != nil || (mod >= 10 && r.Status != "present") {
		t.Fatalf("exactly at the grace: %v %+v", err, r)
	}
}

// US-ATT-003: a second scan within 2 minutes is ignored; a later one is a new row but keeps the first arrival.
func TestUS_ATT_003_DuplicateScans(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-dup", 1)
	c := w.card(t, f, w.students[0])
	at := scanAt()

	first, err := w.scan(t, f, *c.Code, at, "")
	if err != nil || first.Outcome != "accepted" {
		t.Fatalf("first: %v %+v", err, first)
	}
	dup, err := w.scan(t, f, *c.Code, at.Add(30*time.Second), "")
	if err != nil || dup.Outcome != "duplicate" || !dup.ArrivalAt.Equal(first.ArrivalAt) {
		t.Fatalf("within 2 minutes: %v %+v", err, dup)
	}
	later, err := w.scan(t, f, *c.Code, at.Add(3*time.Minute), "")
	if err != nil || later.Outcome != "accepted" || !later.ArrivalAt.Equal(first.ArrivalAt) {
		t.Fatalf("3 minutes later is another scan, the arrival is still the first: %v %+v", err, later)
	}
	day := at.In(karachi)
	scans, _ := f.attendance.GateScans(ctx, w.owner, time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC))
	if len(scans) != 2 || !scans[0].IsArrival || scans[1].IsArrival {
		t.Fatalf("two stored scans, only the first is the arrival: %+v", scans)
	}

	// A double tap at the same instant from several screens records one arrival, for every student, every time.
	other := newGateWorld(t, f, "gate-dup2", 8)
	for round, stu := range other.students {
		oc := other.card(t, f, stu)
		oat := scanAt().Add(time.Duration(round) * time.Second)
		outcomes := make([]string, 12)
		var wg sync.WaitGroup
		for i := range outcomes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := other.scan(t, f, *oc.Code, oat, "")
				if err != nil {
					outcomes[i] = "error: " + err.Error()
					return
				}
				outcomes[i] = r.Outcome
			}()
		}
		wg.Wait()
		accepted := 0
		for _, o := range outcomes {
			if o == "accepted" {
				accepted++
			} else if o != "duplicate" {
				t.Fatalf("round %d: unexpected %v", round, outcomes)
			}
		}
		var rows int
		_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM gate_scans WHERE tenant_id = $1 AND student_id = $2`, other.tenantID, stu).Scan(&rows)
		if accepted != 1 || rows != 1 {
			t.Fatalf("round %d: twelve taps, one scan: accepted %d, rows %d (%v)", round, accepted, rows, outcomes)
		}
	}
}

// BR-ATT-006: nothing is marked for a forged, foreign, disabled or unknown card, a closed day, or a
// student who is not enrolled.
func TestBR_ATT_006_ScanRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-refuse", 2)
	other := newGateWorld(t, f, "gate-refuse-b", 1)
	good := w.card(t, f, w.students[0])
	at := scanAt()
	rowsNow := func() int {
		var n int
		_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM gate_scans WHERE tenant_id = $1`, w.tenantID).Scan(&n)
		return n
	}

	flipped := (*good.Code)[:len(*good.Code)-1] + map[bool]string{true: "B", false: "A"}[(*good.Code)[len(*good.Code)-1] == 'A']
	foreign := other.card(t, f, other.students[0])
	for name, code := range map[string]string{"garbage": "not-a-card", "a forged signature": flipped, "another school's card": *foreign.Code} {
		if _, err := w.scan(t, f, code, at, ""); errCode(t, err) != "attendance.card_invalid" {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := w.scan(t, f, "", at, ""); errCode(t, err) != "attendance.scan_invalid" {
		t.Fatalf("an empty code is a bad request, not an unknown card: %v", err)
	}
	// A validly signed code for a card that does not exist (for example from a restored backup).
	phantom := (*good.Code)[:0] + "5c9c6c26e0e44938af5cf6fc35688270" + (*good.Code)[32:]
	if _, err := w.scan(t, f, phantom, at, ""); errCode(t, err) != "attendance.card_invalid" {
		t.Fatalf("a card id nobody issued: %v", err)
	}

	// Disabled: a lost card stops working at once, and a replacement works.
	if _, err := f.attendance.DisableCard(ctx, w.owner, good.ID, "lost on the bus"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.scan(t, f, *good.Code, at, ""); errCode(t, err) != "attendance.card_disabled" {
		t.Fatalf("a disabled card: %v", err)
	}
	replacement := w.card(t, f, w.students[0])
	if replacement.ID == good.ID || *replacement.Code == *good.Code {
		t.Fatal("a new card has a new code")
	}
	if _, err := w.scan(t, f, *replacement.Code, at, ""); err != nil {
		t.Fatalf("the replacement works: %v", err)
	}

	// A student not enrolled on the scan day.
	notYet := w.card(t, f, w.students[1])
	if _, err := f.admin.Exec(ctx, `UPDATE enrolments SET started_on = $3 WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, w.students[1], today().AddDate(0, 0, 5)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.scan(t, f, *notYet.Code, at, ""); errCode(t, err) != "attendance.not_enrolled" {
		t.Fatalf("not enrolled yet: %v", err)
	}
	// A closed day.
	day := at.In(karachi)
	if _, err := f.org.AddCalendarDays(ctx, w.owner, org.CalendarDaysInput{From: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Kind: "holiday", Name: "Closed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.scan(t, f, *replacement.Code, at.Add(10*time.Minute), ""); errCode(t, err) != "attendance.not_school_day" {
		t.Fatalf("a holiday: %v", err)
	}
	if n := rowsNow(); n != 1 {
		t.Fatalf("only the one good scan is stored, got %d", n)
	}
	// Scan time rules.
	future, old := time.Now().Add(10*time.Minute), time.Now().Add(-10*24*time.Hour)
	for name, tm := range map[string]time.Time{"the future": future, "ten days ago (past the 7 day window)": old} {
		if _, err := w.scan(t, f, *replacement.Code, tm, ""); errCode(t, err) != "attendance.scan_time_invalid" {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := f.attendance.Scan(ctx, w.gate, attendance.ScanInput{Code: *replacement.Code, ClientScanID: "not-an-id"}); errCode(t, err) != "attendance.scan_invalid" {
		t.Fatalf("bad client id: %v", err)
	}
}

// US-ATT-003 (offline): the same upload, sent again or at once, is one scan with the same answer.
func TestUS_ATT_003_OfflineUploadsAreIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-offline", 2)
	c := w.card(t, f, w.students[0])
	at := scanAt()
	const client = "6c9c6c26-e0e4-4938-af5c-f6fc35688271"

	const racers = 6
	results := make([]attendance.ScanResult, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = w.scan(t, f, *c.Code, at, client)
		}()
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("upload %d: %v", i, errs[i])
		}
		if results[i].Outcome != "accepted" || results[i].Student.ID != w.students[0] || !results[i].ScannedAt.Equal(at) {
			t.Fatalf("every repeat returns the first answer: %+v", results[i])
		}
	}
	var rows int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM gate_scans WHERE tenant_id = $1`, w.tenantID).Scan(&rows)
	if rows != 1 {
		t.Fatalf("one scan stored, got %d", rows)
	}
	// Even after the card was disabled, a repeat of an already stored upload still answers (it is history).
	if _, err := f.attendance.DisableCard(ctx, w.owner, c.ID, "lost afterwards"); err != nil {
		t.Fatal(err)
	}
	if r, err := w.scan(t, f, *c.Code, at, client); err != nil || r.Outcome != "accepted" {
		t.Fatalf("a repeat of a stored upload: %v %+v", err, r)
	}
}

// REQ-ATT-017: the gate and the class register disagree, and a person reviews it. Nothing changes.
func TestREQ_ATT_017_Mismatches(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-mismatch", 4)
	at := scanAt()
	day := time.Date(at.In(karachi).Year(), at.In(karachi).Month(), at.In(karachi).Day(), 0, 0, 0, 0, time.UTC)
	s0, s1, s2, s3 := w.students[0], w.students[1], w.students[2], w.students[3]
	for _, s := range []string{s0, s1} { // s0 and s1 are scanned at the gate; s2 and s3 are not
		c := w.card(t, f, s)
		if _, err := w.scan(t, f, *c.Code, at, ""); err != nil {
			t.Fatal(err)
		}
	}
	reg := w.open(t, f, day)
	w.mark(t, f, reg.ID, attendance.Mark{StudentID: s0, Status: "absent"}) // scanned but marked absent
	// s1 scanned and present: fine. s2 present but never scanned: flagged. s3 absent and not scanned: fine.
	w.mark(t, f, reg.ID, attendance.Mark{StudentID: s3, Status: "absent"})

	// Until the register is submitted nothing is compared.
	if list, _ := f.attendance.Mismatches(ctx, w.owner, day); len(list) != 0 {
		t.Fatalf("an open register is not compared: %+v", list)
	}
	w.submit(t, f, reg.ID)
	list, err := f.attendance.Mismatches(ctx, w.owner, day)
	if err != nil || len(list) != 2 {
		t.Fatalf("mismatches: %v %+v", err, list)
	}
	kinds := map[string]string{}
	for _, m := range list {
		kinds[m.StudentID] = m.Kind
	}
	if kinds[s0] != "scanned_but_absent" || kinds[s2] != "present_but_not_scanned" {
		t.Fatalf("kinds: %v", kinds)
	}
	for _, m := range list {
		if m.StudentID == s0 && (m.GateArrivedAt == nil || m.RegisterStatus != "absent") {
			t.Fatalf("the absent one carries the arrival: %+v", m)
		}
	}
	// The register is untouched by all this.
	if got, _ := f.attendance.GetRegister(ctx, w.owner, reg.ID); statusOf(got, s0) != "absent" || statusOf(got, s2) != "present" {
		t.Fatalf("nothing is changed automatically: %+v", got.Records)
	}

	// A school that does not use the gate is not flooded: no scans that day, no "present but not scanned".
	quiet := newGateWorld(t, f, "gate-quiet", 3)
	qreg := quiet.open(t, f, day)
	quiet.submit(t, f, qreg.ID)
	if list, _ := f.attendance.Mismatches(ctx, quiet.owner, day); len(list) != 0 {
		t.Fatalf("no scans, no mismatches: %+v", list)
	}
}

// ADR-0002: school B cannot see A's scans, cards or students, and B's gate cannot use A's cards.
func TestRLS_GateIsIsolatedBetweenSchools(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newGateWorld(t, f, "gate-iso-a", 1)
	b := newGateWorld(t, f, "gate-iso-b", 1)
	c := a.card(t, f, a.students[0])
	at := scanAt()
	if _, err := a.scan(t, f, *c.Code, at, ""); err != nil {
		t.Fatal(err)
	}
	day := time.Date(at.In(karachi).Year(), at.In(karachi).Month(), at.In(karachi).Day(), 0, 0, 0, 0, time.UTC)

	if _, err := b.scan(t, f, *c.Code, at, ""); errCode(t, err) != "attendance.card_invalid" {
		t.Fatalf("B scanned A's card: %v", err)
	}
	if _, err := f.attendance.IssueCard(ctx, b.owner, a.students[0]); errCode(t, err) != "students.not_found" {
		t.Fatalf("B issued a card for A's student: %v", err)
	}
	if _, err := f.attendance.ListCards(ctx, b.owner, a.students[0]); errCode(t, err) != "students.not_found" {
		t.Fatalf("B listed A's cards: %v", err)
	}
	if _, err := f.attendance.DisableCard(ctx, b.owner, c.ID, "attack"); errCode(t, err) != "attendance.card_not_found" {
		t.Fatalf("B disabled A's card: %v", err)
	}
	if scans, _ := f.attendance.GateScans(ctx, b.owner, day); len(scans) != 0 {
		t.Fatalf("B sees A's scans: %+v", scans)
	}
	if list, _ := f.attendance.Mismatches(ctx, b.owner, day); len(list) != 0 {
		t.Fatalf("B sees A's mismatches: %+v", list)
	}
	if cards, _ := f.attendance.ListCards(ctx, a.owner, a.students[0]); len(cards) != 1 || cards[0].Status != "active" {
		t.Fatalf("A's card was touched: %+v", cards)
	}
}

// Cards and scans are history: the database refuses edits, deletes, a second active card, and a repeated id.
func TestBR_ATT_006_TheDatabaseProtectsCardsAndScans(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-db", 1)
	c := w.card(t, f, w.students[0])
	if _, err := w.scan(t, f, *c.Code, scanAt(), "6c9c6c26-e0e4-4938-af5c-f6fc35688272"); err != nil {
		t.Fatal(err)
	}
	for name, stmt := range map[string]string{
		"change a scan":        `UPDATE gate_scans SET status = 'present' WHERE tenant_id = $1`,
		"delete a scan":        `DELETE FROM gate_scans WHERE tenant_id = $1`,
		"delete a card":        `DELETE FROM student_cards WHERE tenant_id = $1`,
		"edit the student":     `UPDATE student_cards SET student_id = gen_random_uuid() WHERE tenant_id = $1`,
		"a second active card": `INSERT INTO student_cards (tenant_id, student_id) SELECT tenant_id, student_id FROM student_cards WHERE tenant_id = $1`,
		"the same client id":   `INSERT INTO gate_scans (tenant_id, student_id, card_id, section_id, day, scanned_at, status, scanned_by, client_scan_id) SELECT tenant_id, student_id, card_id, section_id, day, scanned_at, status, scanned_by, client_scan_id FROM gate_scans WHERE tenant_id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, w.tenantID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
	if _, err := f.attendance.DisableCard(ctx, w.owner, c.ID, "gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE student_cards SET status = 'active', disabled_at = NULL, disabled_reason = NULL WHERE tenant_id = $1`, w.tenantID); err == nil {
		t.Fatal("a disabled card can never be active again")
	}
	if _, err := f.attendance.DisableCard(ctx, w.owner, c.ID, "again"); errCode(t, err) != "attendance.card_already_disabled" {
		t.Fatalf("disable twice: %v", err)
	}
	if _, err := f.attendance.DisableCard(ctx, w.owner, c.ID[:0]+"5c9c6c26-e0e4-4938-af5c-f6fc35688270", "nobody"); errCode(t, err) != "attendance.card_not_found" {
		t.Fatalf("unknown card: %v", err)
	}
	err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM gate_scans`)
		return err
	})
	if err == nil {
		t.Fatal("the API role may not delete scans")
	}
}

// Issuing a card disables the previous one in the same step; cards list newest first with the code only on the active one.
func TestREQ_ATT_011_IssueAndListCards(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-cards", 1)
	first := w.card(t, f, w.students[0])
	second := w.card(t, f, w.students[0])
	cards, err := f.attendance.ListCards(ctx, w.owner, w.students[0])
	if err != nil || len(cards) != 2 || cards[0].ID != second.ID || cards[0].Status != "active" || cards[0].Code == nil ||
		cards[1].ID != first.ID || cards[1].Status != "disabled" || cards[1].Code != nil || cards[1].DisabledReason == nil {
		t.Fatalf("cards: %v %+v", err, cards)
	}
	// The code is the same every time, so a card can be reprinted.
	if again, _ := f.attendance.ListCards(ctx, w.owner, w.students[0]); *again[0].Code != *second.Code {
		t.Fatal("the code must be stable")
	}
	var audits int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'card.issued'`, w.tenantID).Scan(&audits)
	if audits != 2 {
		t.Fatalf("two issues audited, got %d", audits)
	}
	if _, err := f.attendance.IssueCard(ctx, w.owner, "5c9c6c26-e0e4-4938-af5c-f6fc35688270"); errCode(t, err) != "students.not_found" {
		t.Fatalf("unknown student: %v", err)
	}
}

// Each shift has its own start: an evening section is judged against the evening start.
func TestREQ_ATT_002_EveningShiftUsesItsOwnStart(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-evening", 1)
	evening, err := f.org.CreateSection(ctx, w.owner, org.SectionInput{ClassID: mustClassID(t, f, w.attendWorld), Name: "E", Shift: "evening"})
	if err != nil {
		t.Fatal(err)
	}
	in := enrolInput(t, evening.ID)
	in.FullName = "Evening Student"
	stu, err := f.students.Create(ctx, w.owner, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE enrolments SET started_on = $2 WHERE tenant_id = $1`, w.tenantID, ago(7)); err != nil {
		t.Fatal(err)
	}
	at := scanAt()
	mod := minuteOfDay(at)
	// Morning started long ago (so a morning scan is late); evening starts later than the scan (on time).
	if _, err := f.attendance.SetSettings(ctx, w.owner, attendance.SettingsInput{
		RegisterDueTime: "10:00", EditWindowDays: 7, LowAttendancePercent: 75,
		MorningStart: clockText(max(mod-30, 0)), EveningStart: clockText(min(mod+30, 1439)), LateAfterMinutes: 0,
	}); err != nil {
		t.Fatal(err)
	}
	mc, ec := w.card(t, f, w.students[0]), w.card(t, f, stu.ID)
	if r, err := w.scan(t, f, *mc.Code, at, ""); err != nil || r.Status != "late" {
		t.Fatalf("a morning student after the morning start: %v %+v", err, r)
	}
	if r, err := w.scan(t, f, *ec.Code, at.Add(time.Second), ""); err != nil || r.Status != "present" {
		t.Fatalf("an evening student before the evening start: %v %+v", err, r)
	}
}

// The gate over HTTP: shapes and status codes, including the refusals a gate screen shows.
func TestREQ_API_GateOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newGateWorld(t, f, "gate-http", 1)
	gateToken := addStaff(t, f, w.tenantID, w.slug, "receptionist")
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)

	rec := do(t, h, http.MethodPost, "/api/v1/students/"+w.students[0]+"/cards", w.token, "")
	var card struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	mustJSON(t, rec.Body.Bytes(), &card)
	if rec.Code != http.StatusCreated || card.Code == "" {
		t.Fatalf("issue: %d %s", rec.Code, rec.Body)
	}
	at := scanAt().UTC().Format(time.RFC3339)
	body := `{"code":"` + card.Code + `","scanned_at":"` + at + `","client_scan_id":"6c9c6c26-e0e4-4938-af5c-f6fc35688273"}`
	first := do(t, h, http.MethodPost, "/api/v1/attendance/scan", gateToken, body)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"outcome":"accepted"`) || !strings.Contains(first.Body.String(), `"full_name"`) {
		t.Fatalf("scan: %d %s", first.Code, first.Body)
	}
	if again := do(t, h, http.MethodPost, "/api/v1/attendance/scan", gateToken, body); again.Code != http.StatusOK || again.Body.String() != first.Body.String() {
		t.Fatalf("the repeat of an upload: %d %s", again.Code, again.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/attendance/scan", gateToken, `{"code":"junk"}`); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "attendance.card_invalid") {
		t.Fatalf("junk: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/attendance/scan", gateToken, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no code: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/student-cards/"+card.ID+"/disable", w.token, `{"reason":"lost it"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"disabled"`) {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/attendance/scan", gateToken, strings.Replace(body, "6c9c6c26-e0e4-4938-af5c-f6fc35688273", "6c9c6c26-e0e4-4938-af5c-f6fc35688274", 1)); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "attendance.card_disabled") {
		t.Fatalf("a disabled card: %d %s", rec.Code, rec.Body)
	}
	d := today().Format("2006-01-02")
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/gate-scans?date="+d, w.token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items"`) {
		t.Fatalf("gate scans: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/mismatches", w.token, ""); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "attendance.date_invalid") {
		t.Fatalf("a missing date: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/settings", w.token, ""); !strings.Contains(rec.Body.String(), `"morning_start":"08:00"`) || !strings.Contains(rec.Body.String(), `"late_after_minutes":10`) {
		t.Fatalf("settings carry the shift times: %s", rec.Body)
	}
}

// Review of slice 6b (1, 2): a retried upload answers with the scan that was stored, for the day it was
// made, even after midnight or with no scanned_at; a client id reused for another card is refused.
func TestUS_ATT_003_ReplaysAnswerFromTheStoredScan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-replay", 2)
	c0, c1 := w.card(t, f, w.students[0]), w.card(t, f, w.students[1])
	const client = "6c9c6c26-e0e4-4938-af5c-f6fc35688281"

	// The first upload was made a day and a half ago (a computer that was offline).
	earlier := time.Now().Add(-36 * time.Hour).Truncate(time.Second)
	first, err := w.scan(t, f, *c0.Code, earlier, client)
	if err != nil {
		t.Fatal(err)
	}
	// The retry has no scanned_at, so "now" is a different day: it must still return the first answer.
	again, err := f.attendance.Scan(ctx, w.gate, attendance.ScanInput{Code: *c0.Code, ClientScanID: client})
	if err != nil || again.Outcome != "accepted" || !again.ScannedAt.Equal(first.ScannedAt) || !again.ArrivalAt.Equal(first.ArrivalAt) {
		t.Fatalf("a retry on another day: %v %+v", err, again)
	}
	// The same id with another card is a mistake: refused, and nothing is recorded for that card.
	if _, err := w.scan(t, f, *c1.Code, earlier, client); errCode(t, err) != "attendance.client_scan_id_reused" {
		t.Fatalf("a reused client id: %v", err)
	}
	var rows int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM gate_scans WHERE tenant_id = $1`, w.tenantID).Scan(&rows)
	if rows != 1 {
		t.Fatalf("one scan stored: %d", rows)
	}
	// Two screens racing with the same id for two students: one wins, the other gets 409, never a 500. Many rounds,
	// because the clash happens only when both reach the insert together.
	for round := range 25 {
		shared := fmt.Sprintf("6c9c6c26-e0e4-4938-af5c-%012d", 100+round)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, code := range []string{*c0.Code, *c1.Code} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = w.scan(t, f, code, scanAt().Add(-time.Duration(10+round*6)*time.Minute), shared)
			}()
		}
		wg.Wait()
		wins := 0
		for _, e := range errs {
			if e == nil {
				wins++
			} else if errCode(t, e) != "attendance.client_scan_id_reused" {
				t.Fatalf("round %d: unexpected: %v", round, e)
			}
		}
		if wins != 1 {
			t.Fatalf("round %d: exactly one wins: %v", round, errs)
		}
	}
}

// Review of slice 6b (3): a computer offline for days can still upload (the limit is the correction window).
func TestUS_ATT_003_OfflineGapOfDays(t *testing.T) {
	f := newFixture(t)
	w := newGateWorld(t, f, "gate-gap", 1)
	c := w.card(t, f, w.students[0])
	threeDays := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	if r, err := w.scan(t, f, *c.Code, threeDays, ""); err != nil || r.Outcome != "accepted" {
		t.Fatalf("a scan from three days ago: %v %+v", err, r)
	}
}

// Review of slice 6b (6): a refusal of a genuine card is audited; a forged code is only logged.
func TestBR_ATT_006_RefusalsAreAudited(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-audit", 1)
	c := w.card(t, f, w.students[0])
	if _, err := f.attendance.DisableCard(ctx, w.owner, c.ID, "lost it"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.scan(t, f, *c.Code, scanAt(), ""); errCode(t, err) != "attendance.card_disabled" {
		t.Fatal("expected a refusal")
	}
	if _, err := w.scan(t, f, "forged-code", scanAt(), ""); errCode(t, err) != "attendance.card_invalid" {
		t.Fatal("expected a refusal")
	}
	var refused int
	var reason, by string
	_ = f.admin.QueryRow(ctx, `SELECT count(*), COALESCE(max(summary->>'reason'), ''), COALESCE(max(actor_user_id::text), '') FROM audit_log WHERE tenant_id = $1 AND action = 'attendance.scan_refused'`, w.tenantID).Scan(&refused, &reason, &by)
	if refused != 1 || reason != "attendance.card_disabled" || by != w.gate.UserID {
		t.Fatalf("the disabled card's refusal is audited with the user: %d %q %q", refused, reason, by)
	}
}

// A scanner or person may change the case of what it reads.
func TestREQ_ATT_011_CodeCaseDoesNotMatter(t *testing.T) {
	f := newFixture(t)
	w := newGateWorld(t, f, "gate-case", 1)
	c := w.card(t, f, w.students[0])
	if r, err := w.scan(t, f, strings.ToLower(*c.Code), scanAt(), ""); err != nil || r.Outcome != "accepted" {
		t.Fatalf("lower case: %v %+v", err, r)
	}
}

// Review of slice 6b (7): a card is disabled "at once". While a disable holds the card's row (as DisableCard
// does), a scan waits for it and then sees the card disabled; it never reads the old "active" and stores a scan
// after the disable has completed.
func TestUS_ATT_003_ADisableInFlightStopsAScan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-disable-race", 1)
	c := w.card(t, f, w.students[0])

	tx, err := f.admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM student_cards WHERE id = $1 FOR UPDATE`, c.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := w.scan(t, f, *c.Code, scanAt(), "")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("the scan must wait for the disable that holds the card, but returned: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	if _, err := tx.Exec(ctx, `UPDATE student_cards SET status = 'disabled', disabled_at = now(), disabled_reason = 'lost in a race' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; errCode(t, err) != "attendance.card_disabled" {
		t.Fatalf("after the disable commits the scan is refused: %v", err)
	}
	var stored int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM gate_scans WHERE card_id = $1`, c.ID).Scan(&stored)
	if stored != 0 {
		t.Fatalf("nothing is stored for a disabled card: %d", stored)
	}
}

// Review of slice 6b (8): a student scanned and then moved to another section the same day does not make the
// old section look like it uses the gate, so its other students are not flagged.
func TestREQ_ATT_017_ATransferDoesNotFlagTheOldSection(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newGateWorld(t, f, "gate-transfer", 4)
	second, err := f.org.CreateSection(ctx, w.owner, org.SectionInput{ClassID: mustClassID(t, f, w.attendWorld), Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	mover := w.students[0]
	c := w.card(t, f, mover)
	if _, err := f.attendance.Scan(ctx, w.gate, attendance.ScanInput{Code: *c.Code}); err != nil { // scanned now, in section A
		t.Fatal(err)
	}
	a := w.open(t, f, today())
	if _, err := f.students.ChangeSection(ctx, w.owner, mover, second.ID, "moved to B"); err != nil {
		t.Fatal(err)
	}
	w.submit(t, f, a.ID) // the mover is recorded as left in A
	list, err := f.attendance.Mismatches(ctx, w.owner, today())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.SectionID == w.sectionID {
			t.Fatalf("section A only ever scanned a student who left it, so nobody there is flagged: %+v", m)
		}
	}
}

// Review of slice 6b (9, 10): a client that does not know the shift fields keeps what is saved, and issuing a
// card with the same Idempotency-Key twice makes one card.
func TestREQ_API_GateSettingsAndIdempotentIssue(t *testing.T) {
	f := newFixture(t)
	w := newGateWorld(t, f, "gate-compat", 1)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))

	if rec := do(t, h, http.MethodPut, "/api/v1/attendance/settings", w.token,
		`{"register_due_time":"10:00","edit_window_days":7,"low_attendance_percent":75,"morning_start":"07:30","evening_start":"12:30","late_after_minutes":5}`); rec.Code != http.StatusOK {
		t.Fatalf("full settings: %d %s", rec.Code, rec.Body)
	}
	// An older client sends only the first three fields: the shift times stay as saved.
	rec := do(t, h, http.MethodPut, "/api/v1/attendance/settings", w.token, `{"register_due_time":"09:30","edit_window_days":5,"low_attendance_percent":70}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"morning_start":"07:30"`) || !strings.Contains(rec.Body.String(), `"late_after_minutes":5`) ||
		!strings.Contains(rec.Body.String(), `"register_due_time":"09:30"`) {
		t.Fatalf("an old client: %d %s", rec.Code, rec.Body)
	}

	url := "/api/v1/students/" + w.students[0] + "/cards"
	first := keyed(t, h, url, w.token, "issue-card-key-001", "")
	second := keyed(t, h, url, w.token, "issue-card-key-001", "")
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || second.Body.String() != first.Body.String() || second.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("issue twice with one key: %d %d", first.Code, second.Code)
	}
	cards, _ := f.attendance.ListCards(context.Background(), w.owner, w.students[0])
	if len(cards) != 1 {
		t.Fatalf("one card, got %d", len(cards))
	}
}
