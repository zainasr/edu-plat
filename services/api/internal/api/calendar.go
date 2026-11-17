package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// Handlers for the school calendar (REQ-ORG-011). Same shape as every handler: check the permission, call
// the service, map the result.

func dateParam(d *openapitypes.Date) time.Time {
	if d == nil {
		return time.Time{}
	}
	return d.Time
}

// GetCalendar returns the weekly off days and the listed days in a range.
func (s *Server) GetCalendar(ctx context.Context, req gen.GetCalendarRequestObject) (gen.GetCalendarResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermOrgView)
	if err != nil {
		return nil, err
	}
	cal, err := s.org.GetCalendar(ctx, p, dateParam(req.Params.From), dateParam(req.Params.To))
	if err != nil {
		return nil, err
	}
	out, err := toAPICalendar(cal)
	if err != nil {
		return nil, err
	}
	return gen.GetCalendar200JSONResponse(out), nil
}

// SetWeeklyOff sets the weekly off days.
func (s *Server) SetWeeklyOff(ctx context.Context, req gen.SetWeeklyOffRequestObject) (gen.SetWeeklyOffResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermCalendarManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	if _, err := s.org.SetWeeklyOff(ctx, p, req.Body.Days); err != nil {
		return nil, err
	}
	cal, err := s.org.GetCalendar(ctx, p, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	out, err := toAPICalendar(cal)
	if err != nil {
		return nil, err
	}
	return gen.SetWeeklyOff200JSONResponse(out), nil
}

// AddCalendarDays adds holidays or working days.
func (s *Server) AddCalendarDays(ctx context.Context, req gen.AddCalendarDaysRequestObject) (gen.AddCalendarDaysResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermCalendarManage)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Validation("request.body_required", "A JSON body is required.")
	}
	in := org.CalendarDaysInput{From: req.Body.From.Time, Kind: string(req.Body.Kind), Name: req.Body.Name}
	if req.Body.To != nil {
		to := req.Body.To.Time
		in.To = &to
	}
	days, err := s.org.AddCalendarDays(ctx, p, in)
	if err != nil {
		return nil, err
	}
	items, err := toAPICalendarDays(days)
	if err != nil {
		return nil, err
	}
	return gen.AddCalendarDays201JSONResponse{Items: items}, nil
}

// RemoveCalendarDay removes an entry.
func (s *Server) RemoveCalendarDay(ctx context.Context, req gen.RemoveCalendarDayRequestObject) (gen.RemoveCalendarDayResponseObject, error) {
	p, err := iam.Require(ctx, iam.PermCalendarManage)
	if err != nil {
		return nil, err
	}
	d, err := s.org.RemoveCalendarDay(ctx, p, req.DayId.String())
	if err != nil {
		return nil, err
	}
	out, err := toAPICalendarDay(d)
	if err != nil {
		return nil, err
	}
	return gen.RemoveCalendarDay200JSONResponse(out), nil
}

func toAPICalendar(c org.Calendar) (gen.Calendar, error) {
	days, err := toAPICalendarDays(c.Days)
	if err != nil {
		return gen.Calendar{}, err
	}
	return gen.Calendar{
		WeeklyOffDays: c.WeeklyOffDays, From: openapitypes.Date{Time: c.From}, To: openapitypes.Date{Time: c.To}, Days: days,
	}, nil
}

func toAPICalendarDays(list []org.CalendarDay) ([]gen.CalendarDay, error) {
	out := make([]gen.CalendarDay, 0, len(list))
	for _, d := range list {
		item, err := toAPICalendarDay(d)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func toAPICalendarDay(d org.CalendarDay) (gen.CalendarDay, error) {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return gen.CalendarDay{}, err
	}
	return gen.CalendarDay{Id: id, Date: openapitypes.Date{Time: d.Date}, Kind: gen.CalendarKind(d.Kind), Name: d.Name, Status: gen.CalendarDayStatus(d.Status)}, nil
}
