package report

import (
	"fmt"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The periods a report covers.
const (
	Daily   = "daily"
	Weekly  = "weekly"
	Monthly = "monthly"
	Yearly  = "yearly"
)

var monthNames = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// Period is a span of calendar days, both ends inclusive, at midnight UTC —
// the one notion of "day" the services share (see school-day-is-utc-day).
type Period struct {
	Kind     string
	From, To time.Time
}

// NewPeriod is the period of kind that holds anchor: the day itself, its week
// (Monday to Sunday), its month or its year.
func NewPeriod(kind string, anchor time.Time) (Period, error) {
	a := anchor.UTC()
	d := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	switch kind {
	case Daily:
		return Period{Kind: kind, From: d, To: d}, nil
	case Weekly:
		// time.Weekday counts from Sunday; a Mexican week starts on Monday.
		back := (int(d.Weekday()) + 6) % 7
		from := d.AddDate(0, 0, -back)
		return Period{Kind: kind, From: from, To: from.AddDate(0, 0, 6)}, nil
	case Monthly:
		from := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
		return Period{Kind: kind, From: from, To: from.AddDate(0, 1, -1)}, nil
	case Yearly:
		from := time.Date(d.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		return Period{Kind: kind, From: from, To: from.AddDate(1, 0, -1)}, nil
	default:
		return Period{}, glerrors.New(glerrors.KindInvalid, "period must be daily, weekly, monthly or yearly")
	}
}

// Label names the period in Spanish, as a heading reads it.
func (p Period) Label() string {
	switch p.Kind {
	case Weekly:
		if p.From.Year() != p.To.Year() {
			return fmt.Sprintf("Semana del %s al %s", longDay(p.From), longDay(p.To))
		}
		return fmt.Sprintf("Semana del %d de %s al %s", p.From.Day(), monthNames[p.From.Month()-1], longDay(p.To))
	case Monthly:
		name := monthNames[p.From.Month()-1]
		return fmt.Sprintf("%s%s de %d", strings.ToUpper(name[:1]), name[1:], p.From.Year())
	case Yearly:
		return fmt.Sprintf("Año %d", p.From.Year())
	default:
		return longDay(p.From)
	}
}

// longDay is "1 de octubre de 2026".
func longDay(t time.Time) string {
	return fmt.Sprintf("%d de %s de %d", t.Day(), monthNames[t.Month()-1], t.Year())
}
