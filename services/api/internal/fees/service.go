package fees

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/fees/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// pakistanZone is Asia/Karachi (UTC+5, no DST). "This month" means the month there.
var pakistanZone = time.FixedZone("PKT", 5*60*60)

// FeeType is a head a school charges for, such as Tuition.
type FeeType struct {
	ID        string
	Code      string
	Name      string
	Kind      string
	IsDeposit bool
	Status    string
}

// PlanLine is one version of a class plan line, with its fee type's name.
type PlanLine struct {
	FeeTypeID     string
	FeeTypeName   string
	Kind          string
	Amount        money.Money
	Frequency     string
	EffectiveFrom time.Time
	Removed       bool
}

// Plan is a class plan as it stands in one month.
type Plan struct {
	ClassID string
	Month   time.Time
	Lines   []PlanLine
}

// Override is one version of a student override.
type Override struct {
	FeeTypeID     string
	FeeTypeName   string
	Amount        money.Money
	EffectiveFrom time.Time
	Cleared       bool
	Reason        string
	CreatedAt     time.Time
}

// Structure is what a student is charged for one month.
type Structure struct {
	StudentID string
	ClassID   string
	Month     time.Time
	Lines     []Line
}

// Service holds the fee setup rules. It asks org about classes and students about a student's class,
// inside its own transaction, and never reads their tables (ADR-0013).
type Service struct {
	pool     *pgxpool.Pool
	org      *org.Service
	students *students.Service
	now      func() time.Time
}

// NewService builds the fees service.
func NewService(pool *pgxpool.Pool, structure *org.Service, enrolment *students.Service) *Service {
	return &Service{pool: pool, org: structure, students: enrolment, now: time.Now}
}

// ---- fee types ------------------------------------------------------------------------

// ListTypes returns the school's fee types, creating the suggested set the first time.
func (s *Service) ListTypes(ctx context.Context, p iam.Principal) ([]FeeType, error) {
	out := []FeeType{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		if err := q.EnsureDefaultFeeTypes(ctx, tenant); err != nil {
			return fmt.Errorf("ensure default fee types: %w", err)
		}
		rows, err := q.ListFeeTypes(ctx, tenant)
		if err != nil {
			return fmt.Errorf("list fee types: %w", err)
		}
		for _, r := range rows {
			out = append(out, FeeType{ID: db.UUIDString(r.ID), Code: r.Code, Name: r.Name, Kind: r.Kind, IsDeposit: r.IsDeposit, Status: r.Status})
		}
		return nil
	})
	return out, err
}

// CreateType adds a fee type. A security deposit kind is marked as a deposit (BR-FEE-004).
func (s *Service) CreateType(ctx context.Context, p iam.Principal, in FeeTypeInput) (FeeType, error) {
	in, err := validateFeeType(in)
	if err != nil {
		return FeeType{}, err
	}
	var out FeeType
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
		// Seed first, so a clash with a suggested code is reported the same way every time.
		if err := q.EnsureDefaultFeeTypes(ctx, tenant); err != nil {
			return fmt.Errorf("ensure default fee types: %w", err)
		}
		row, err := q.InsertFeeType(ctx, gen.InsertFeeTypeParams{
			TenantID: tenant, Code: in.Code, Name: in.Name, Kind: in.Kind,
			IsDeposit: in.Kind == "security_deposit", CreatedBy: actor,
		})
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("fees.type_exists", "A fee type with this code already exists.")
		}
		if err != nil {
			return fmt.Errorf("insert fee type: %w", err)
		}
		out = toFeeType(row.ID, row.Code, row.Name, row.Kind, row.IsDeposit, row.Status)
		return s.audit(ctx, tx, p, "fee_type.created", "fee_type", out.ID, map[string]any{"code": out.Code, "kind": out.Kind})
	})
	return out, err
}

// ArchiveType archives a fee type. It is never deleted.
func (s *Service) ArchiveType(ctx context.Context, p iam.Principal, id string) (FeeType, error) {
	notFound := apperr.NotFound("fees.type_not_found", "Fee type not found.")
	typeID, err := db.ParseUUID(id)
	if err != nil {
		return FeeType{}, notFound
	}
	var out FeeType
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		row, err := gen.New(tx).ArchiveFeeType(ctx, gen.ArchiveFeeTypeParams{TenantID: tenant, ID: typeID})
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return fmt.Errorf("archive fee type: %w", err)
		}
		out = toFeeType(row.ID, row.Code, row.Name, row.Kind, row.IsDeposit, row.Status)
		return s.audit(ctx, tx, p, "fee_type.archived", "fee_type", out.ID, map[string]any{"code": out.Code})
	})
	return out, err
}

// ---- class plans ----------------------------------------------------------------------

// SetPlanLine adds a new version to a class plan: a fee type, an amount and a frequency from a month,
// or a removal from that month. Earlier versions stay (BR-FEE-001).
func (s *Service) SetPlanLine(ctx context.Context, p iam.Principal, classID string, in PlanLineInput) (PlanLine, error) {
	in, err := validatePlanLine(in)
	if err != nil {
		return PlanLine{}, err
	}
	var out PlanLine
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
		class, err := s.org.DescribeClass(ctx, tx, p.TenantID, classID)
		if err != nil {
			return err
		}
		if class.Status != "active" {
			return apperr.Validation("fees.class_archived", "This class is archived; its plan cannot change.")
		}
		classUUID, err := db.ParseUUID(classID)
		if err != nil {
			return err
		}
		typeUUID, err := db.ParseUUID(in.FeeTypeID)
		if err != nil {
			return err
		}
		ft, err := q.GetFeeType(ctx, gen.GetFeeTypeParams{TenantID: tenant, ID: typeUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("fees.type_not_found", "Fee type not found.")
		}
		if err != nil {
			return fmt.Errorf("get fee type: %w", err)
		}

		amount, frequency := money.Money(0), "monthly" // a removal stores a placeholder; it is never read
		if in.Remove {
			versions, types, err := s.loadPlan(ctx, q, tenant, classUUID)
			if err != nil {
				return err
			}
			if !containsType(resolvePlan(in.EffectiveFrom, versions, types), in.FeeTypeID) {
				return apperr.Validation("fees.nothing_to_remove", "This fee type is not on the plan in that month.")
			}
		} else {
			if ft.Status != "active" {
				return apperr.Validation("fees.type_archived", "This fee type is archived and cannot be added to a plan.")
			}
			amount, frequency = *in.Amount, in.Frequency
		}

		if _, err := q.InsertPlanLine(ctx, gen.InsertPlanLineParams{
			TenantID: tenant, ClassID: classUUID, FeeTypeID: typeUUID, Amount: int64(amount),
			Frequency: frequency, EffectiveFrom: pgtype.Date{Time: in.EffectiveFrom, Valid: true},
			Removed: in.Remove, CreatedBy: actor,
		}); err != nil {
			if db.IsUniqueViolation(err) {
				return apperr.Conflict("fees.plan_line_exists",
					"This fee type already has a version for that month. Choose a later month.")
			}
			return fmt.Errorf("insert plan line: %w", err)
		}
		out = PlanLine{
			FeeTypeID: in.FeeTypeID, FeeTypeName: ft.Name, Kind: ft.Kind, Amount: amount,
			Frequency: frequency, EffectiveFrom: in.EffectiveFrom, Removed: in.Remove,
		}
		return s.audit(ctx, tx, p, "fee_plan.line_set", "class", classID, map[string]any{
			"fee_type": ft.Code, "amount": int64(amount), "frequency": frequency,
			"effective_from": in.EffectiveFrom.Format(time.DateOnly), "removed": in.Remove,
		})
	})
	return out, err
}

// GetPlan returns the class plan as it stands in a month (the current month when month is zero).
func (s *Service) GetPlan(ctx context.Context, p iam.Principal, classID string, month time.Time) (Plan, error) {
	month = s.monthOrNow(month)
	out := Plan{ClassID: classID, Month: month, Lines: []PlanLine{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		if _, err := s.org.DescribeClass(ctx, tx, p.TenantID, classID); err != nil {
			return err
		}
		classUUID, err := db.ParseUUID(classID)
		if err != nil {
			return err
		}
		versions, types, err := s.loadPlan(ctx, gen.New(tx), tenant, classUUID)
		if err != nil {
			return err
		}
		for _, v := range resolvePlan(month, versions, types) {
			out.Lines = append(out.Lines, PlanLine{
				FeeTypeID: v.FeeTypeID, FeeTypeName: v.Name, Kind: v.Kind, Amount: v.Amount,
				Frequency: v.Frequency, EffectiveFrom: v.EffectiveFrom,
			})
		}
		return nil
	})
	return out, err
}

// PlanHistory returns every version ever set on a class plan, newest first.
func (s *Service) PlanHistory(ctx context.Context, p iam.Principal, classID string) ([]PlanLine, error) {
	out := []PlanLine{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		if _, err := s.org.DescribeClass(ctx, tx, p.TenantID, classID); err != nil {
			return err
		}
		classUUID, err := db.ParseUUID(classID)
		if err != nil {
			return err
		}
		rows, err := gen.New(tx).ListPlanVersions(ctx, gen.ListPlanVersionsParams{TenantID: tenant, ClassID: classUUID})
		if err != nil {
			return fmt.Errorf("list plan versions: %w", err)
		}
		for _, r := range rows {
			out = append(out, PlanLine{
				FeeTypeID: db.UUIDString(r.FeeTypeID), FeeTypeName: r.FeeTypeName, Kind: r.Kind,
				Amount: money.Money(r.Amount), Frequency: r.Frequency, EffectiveFrom: r.EffectiveFrom.Time, Removed: r.Removed,
			})
		}
		return nil
	})
	return out, err
}

// ---- student overrides ----------------------------------------------------------------

// SetOverride adds a version to a student's overrides. The fee type must be on the student's class
// plan in that month, so an override changes an amount rather than inventing a charge.
func (s *Service) SetOverride(ctx context.Context, p iam.Principal, studentID string, in OverrideInput) (Override, error) {
	in, err := validateOverride(in)
	if err != nil {
		return Override{}, err
	}
	var out Override
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
		classID, err := s.students.CurrentClassID(ctx, tx, p.TenantID, studentID)
		if err != nil {
			return err
		}
		studentUUID, err := db.ParseUUID(studentID)
		if err != nil {
			return err
		}
		typeUUID, err := db.ParseUUID(in.FeeTypeID)
		if err != nil {
			return err
		}
		ft, err := q.GetFeeType(ctx, gen.GetFeeTypeParams{TenantID: tenant, ID: typeUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("fees.type_not_found", "Fee type not found.")
		}
		if err != nil {
			return fmt.Errorf("get fee type: %w", err)
		}
		classUUID, err := db.ParseUUID(classID)
		if err != nil {
			return err
		}
		versions, types, err := s.loadPlan(ctx, q, tenant, classUUID)
		if err != nil {
			return err
		}
		if !in.Clear && !containsType(resolvePlan(in.EffectiveFrom, versions, types), in.FeeTypeID) {
			return apperr.Validation("fees.override_needs_plan_line",
				"This fee type is not on the student's class plan in that month, so there is nothing to override.")
		}

		amount := money.Money(0)
		if in.Amount != nil {
			amount = *in.Amount
		}
		row, err := q.InsertOverride(ctx, gen.InsertOverrideParams{
			TenantID: tenant, StudentID: studentUUID, FeeTypeID: typeUUID, Amount: int64(amount),
			EffectiveFrom: pgtype.Date{Time: in.EffectiveFrom, Valid: true}, Cleared: in.Clear,
			Reason: in.Reason, CreatedBy: actor,
		})
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("fees.override_exists",
				"This student already has a version for that fee type and month. Choose a later month.")
		}
		if err != nil {
			return fmt.Errorf("insert override: %w", err)
		}
		out = Override{
			FeeTypeID: in.FeeTypeID, FeeTypeName: ft.Name, Amount: amount, EffectiveFrom: in.EffectiveFrom,
			Cleared: in.Clear, Reason: in.Reason, CreatedAt: row.CreatedAt.Time,
		}
		return s.audit(ctx, tx, p, "fee_override.set", "student", studentID, map[string]any{
			"fee_type": ft.Code, "amount": int64(amount), "cleared": in.Clear,
			"effective_from": in.EffectiveFrom.Format(time.DateOnly), "reason": in.Reason,
		})
	})
	return out, err
}

// ListOverrides returns every override version of a student, newest first.
func (s *Service) ListOverrides(ctx context.Context, p iam.Principal, studentID string) ([]Override, error) {
	out := []Override{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := s.students.RequireStudent(ctx, tx, p.TenantID, studentID); err != nil {
			return err
		}
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		studentUUID, err := db.ParseUUID(studentID)
		if err != nil {
			return err
		}
		rows, err := gen.New(tx).ListOverridesForStudent(ctx, gen.ListOverridesForStudentParams{TenantID: tenant, StudentID: studentUUID})
		if err != nil {
			return fmt.Errorf("list overrides: %w", err)
		}
		for _, r := range rows {
			out = append(out, Override{
				FeeTypeID: db.UUIDString(r.FeeTypeID), FeeTypeName: r.FeeTypeName, Amount: money.Money(r.Amount),
				EffectiveFrom: r.EffectiveFrom.Time, Cleared: r.Cleared, Reason: r.Reason, CreatedAt: r.CreatedAt.Time,
			})
		}
		return nil
	})
	return out, err
}

// FeeStructure returns what a student is charged for a month: the class plan with the student's
// overrides on top. It is what challan generation will read.
func (s *Service) FeeStructure(ctx context.Context, p iam.Principal, studentID string, month time.Time) (Structure, error) {
	month = s.monthOrNow(month)
	out := Structure{StudentID: studentID, Month: month, Lines: []Line{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		classID, err := s.students.CurrentClassID(ctx, tx, p.TenantID, studentID)
		if err != nil {
			return err
		}
		out.ClassID = classID
		classUUID, err := db.ParseUUID(classID)
		if err != nil {
			return err
		}
		studentUUID, err := db.ParseUUID(studentID)
		if err != nil {
			return err
		}
		versions, types, err := s.loadPlan(ctx, q, tenant, classUUID)
		if err != nil {
			return err
		}
		overrideRows, err := q.ListOverridesForStudent(ctx, gen.ListOverridesForStudentParams{TenantID: tenant, StudentID: studentUUID})
		if err != nil {
			return fmt.Errorf("list overrides: %w", err)
		}
		overrides := make([]OverrideVersion, 0, len(overrideRows))
		for _, r := range overrideRows {
			overrides = append(overrides, OverrideVersion{
				FeeTypeID: db.UUIDString(r.FeeTypeID), Amount: money.Money(r.Amount),
				EffectiveFrom: r.EffectiveFrom.Time, Cleared: r.Cleared,
			})
		}
		out.Lines = resolveStructure(month, resolvePlan(month, versions, types), overrides)
		return nil
	})
	return out, err
}

// ---- helpers --------------------------------------------------------------------------

// loadPlan reads all versions of a class plan and the school's fee types for the resolver.
func (s *Service) loadPlan(ctx context.Context, q *gen.Queries, tenant, class pgtype.UUID) ([]PlanVersion, map[string]TypeInfo, error) {
	rows, err := q.ListPlanVersions(ctx, gen.ListPlanVersionsParams{TenantID: tenant, ClassID: class})
	if err != nil {
		return nil, nil, fmt.Errorf("list plan versions: %w", err)
	}
	versions := make([]PlanVersion, 0, len(rows))
	for _, r := range rows {
		versions = append(versions, PlanVersion{
			FeeTypeID: db.UUIDString(r.FeeTypeID), Amount: money.Money(r.Amount), Frequency: r.Frequency,
			EffectiveFrom: r.EffectiveFrom.Time, Removed: r.Removed,
		})
	}
	typeRows, err := q.ListFeeTypes(ctx, tenant)
	if err != nil {
		return nil, nil, fmt.Errorf("list fee types: %w", err)
	}
	types := make(map[string]TypeInfo, len(typeRows))
	for _, t := range typeRows {
		types[db.UUIDString(t.ID)] = TypeInfo{Name: t.Name, Kind: t.Kind, Archived: t.Status != "active"}
	}
	return versions, types, nil
}

func containsType(plan []PlanVersion, feeTypeID string) bool {
	for _, v := range plan {
		if v.FeeTypeID == feeTypeID {
			return true
		}
	}
	return false
}

// monthOrNow returns the first day of the month asked for, or of the current month in Pakistan.
func (s *Service) monthOrNow(m time.Time) time.Time {
	if m.IsZero() {
		m = s.now().In(pakistanZone)
	}
	return monthStart(m)
}

func toFeeType(id pgtype.UUID, code, name, kind string, isDeposit bool, status string) FeeType {
	return FeeType{ID: db.UUIDString(id), Code: code, Name: name, Kind: kind, IsDeposit: isDeposit, Status: status}
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, p iam.Principal, action, entity, entityID string, summary map[string]any) error {
	return audit.Record(ctx, tx, audit.Entry{
		TenantID: p.TenantID, Source: audit.SourceAPI, ActorUserID: p.UserID, TokenID: p.TokenID,
		Action: action, Entity: entity, EntityID: entityID, Summary: summary,
	})
}
