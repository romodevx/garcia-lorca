package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// typeBirthday is what the client switches on to draw a birthday greeting (T11).
const typeBirthday = "birthday"

// namePlaceholder is replaced by the pupil's first name in the school's text.
const namePlaceholder = "{nombre}"

// The greeting the school sends until it writes its own.
const (
	defaultBirthdayTitle = "¡Feliz cumpleaños, {nombre}!"
	defaultBirthdayBody  = "Toda la comunidad del Colegio García Lorca le desea a {nombre} un muy feliz cumpleaños. ¡Que tenga un día maravilloso!"
)

// The limits of the school's text, in characters: a push title is one line and
// a notification body a short paragraph.
const (
	maxBirthdayTitle = 80
	maxBirthdayBody  = 500
)

// Pupil is an attending pupil (active enrollment) whose birthday it is: the id
// the greeting is filed under and the first name it says.
type Pupil struct {
	ID        string
	FirstName string
}

// BirthdayDirectory answers "which pupils with an active enrollment were born
// on this calendar day". includeFeb29 also returns those born on February 29.
// Students owns the answer; the adapter reads it by RPC.
type BirthdayDirectory interface {
	BirthdaysOn(ctx context.Context, month, day int, includeFeb29 bool) ([]Pupil, error)
}

// BirthdayTemplate is the greeting the school sends. IsDefault is true while the
// school has not saved its own text.
type BirthdayTemplate struct {
	Title     string
	Body      string
	IsDefault bool
	UpdatedAt time.Time
	UpdatedBy string
}

// TemplateStore keeps the school's greeting. found is false until one is saved.
type TemplateStore interface {
	BirthdayTemplate(ctx context.Context) (t BirthdayTemplate, found bool, err error)
	SaveBirthdayTemplate(ctx context.Context, t BirthdayTemplate) (BirthdayTemplate, error)
}

// BirthdayRun is what one run did. A re-run of the same day reports everybody as
// AlreadyGreeted.
type BirthdayRun struct {
	Date             string
	Greeted          int
	AlreadyGreeted   int
	WithoutGuardians int
}

// WithBirthdays enables the daily birthday greeting: the directory that knows
// who was born when, where the school's text lives, and the school's time zone —
// which decides what "today" is.
func WithBirthdays(dir BirthdayDirectory, templates TemplateStore, school *time.Location) Option {
	return func(s *CommsService) {
		s.birthdays = dir
		s.templates = templates
		s.school = school
	}
}

// errBirthdaysOff is returned when the service was built without WithBirthdays.
var errBirthdaysOff = glerrors.New(glerrors.KindInternal, "birthday greetings are not configured")

// RunBirthdays greets the family of every attending pupil born on date, of any
// year. An empty date is today in the school's zone: at 21:00 in Mexico City it
// is already tomorrow in UTC, and the greeting must not go out a day early.
//
// Each greeting is claimed in the ledger as birthday:<pupil>:<year>, in the same
// transaction as its feed entry, so however many times the job runs a family is
// greeted once a year. A pupil with no family account is counted and NOT
// claimed: the feed is only read by guardians, and linking the family later
// that day and re-running must still reach them.
//
// One pupil that fails does not stop the others; the run then returns the
// error, so the scheduler retries and the ledger skips everybody already
// greeted.
func (s *CommsService) RunBirthdays(ctx context.Context, date string) (BirthdayRun, error) {
	if s.birthdays == nil || s.templates == nil || s.school == nil || s.guardians == nil {
		return BirthdayRun{}, errBirthdaysOff
	}
	day, err := s.greetingDay(date)
	if err != nil {
		return BirthdayRun{}, err
	}
	run := BirthdayRun{Date: day.Format(isoDateLayout)}

	template, err := s.BirthdayTemplate(ctx)
	if err != nil {
		return run, err
	}
	month, dayOfMonth, includeFeb29 := birthdayQuery(day)
	pupils, err := s.birthdays.BirthdaysOn(ctx, month, dayOfMonth, includeFeb29)
	if err != nil {
		return run, fmt.Errorf("listing the day's birthdays: %w", err)
	}

	var failures []error
	for _, p := range pupils {
		if err := s.greet(ctx, p, template, day.Year(), &run); err != nil {
			s.log.Warn("comms could not greet a pupil on their birthday",
				slog.String("student_id", p.ID), slog.Any("error", err))
			failures = append(failures, err)
		}
	}
	s.log.Info("comms birthday run",
		slog.String("date", run.Date),
		slog.Int("greeted", run.Greeted),
		slog.Int("already_greeted", run.AlreadyGreeted),
		slog.Int("without_guardians", run.WithoutGuardians))
	return run, errors.Join(failures...)
}

// greet files and pushes one pupil's greeting, counting the outcome on run.
func (s *CommsService) greet(ctx context.Context, p Pupil, t BirthdayTemplate, year int, run *BirthdayRun) error {
	guardianIDs, err := s.guardians.GuardiansForStudent(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("resolving guardians of %s: %w", p.ID, err)
	}
	if len(guardianIDs) == 0 {
		run.WithoutGuardians++
		return nil
	}
	n := Notification{
		ID:        s.newID(),
		StudentID: p.ID,
		Type:      typeBirthday,
		Title:     renderGreeting(t.Title, p.FirstName),
		Body:      renderGreeting(t.Body, p.FirstName),
		CreatedAt: s.now(),
	}
	stored, isNew, err := s.repo.RecordEvent(ctx, birthdayKey(p.ID, year), n)
	if err != nil {
		return fmt.Errorf("recording the greeting of %s: %w", p.ID, err)
	}
	if !isNew {
		run.AlreadyGreeted++
		return nil
	}
	run.Greeted++
	s.pushTo(ctx, stored, guardianIDs, map[string]string{dataType: typeBirthday, dataStudentID: p.ID})
	return nil
}

// greetingDay is the day to greet: raw when given, else today in the school's
// zone. A day that is not YYYY-MM-DD, or does not exist, is the caller's mistake.
//
// A day after today is refused: greeting it now would reach families early and
// claim the year's ledger key, so the real birthday would be skipped as already
// greeted. Past days replay (a missed morning is recovered that way).
func (s *CommsService) greetingDay(raw string) (time.Time, error) {
	y, m, d := s.now().In(s.school).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return today, nil
	}
	day, err := time.Parse(isoDateLayout, raw)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "date must be YYYY-MM-DD")
	}
	if day.After(today) {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "a birthday run cannot be for a future day")
	}
	return day, nil
}

// birthdayQuery is the calendar day whose pupils are greeted on day. February
// 28 of a year without a 29th also greets the leap-day pupils (owner decision:
// still in their own month).
func birthdayQuery(day time.Time) (month, dayOfMonth int, includeFeb29 bool) {
	month, dayOfMonth = int(day.Month()), day.Day()
	includeFeb29 = day.Month() == time.February && dayOfMonth == 28 && !isLeap(day.Year())
	return month, dayOfMonth, includeFeb29
}

// isLeap reports whether year has a February 29.
func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// birthdayKey is the ledger key of one pupil's greeting in one year.
func birthdayKey(studentID string, year int) string {
	return fmt.Sprintf("birthday:%s:%d", studentID, year)
}

// renderGreeting puts the pupil's first name wherever the school wrote {nombre},
// in name case: the office captures names in capitals, and a greeting that
// shouts "¡Feliz cumpleaños, SOFÍA!" reads like a form, not a card.
func renderGreeting(text, firstName string) string {
	name := nameCase(firstName)
	if name == "" {
		name = fallbackStudentName
	}
	return strings.ReplaceAll(text, namePlaceholder, name)
}

// nameParticles stay lowercase inside a name ("María de los Ángeles").
var nameParticles = map[string]bool{"de": true, "del": true, "la": true, "las": true, "los": true, "y": true}

// nameCase writes each word of a first name with its initial capital, except
// the particles after the first word.
func nameCase(raw string) string {
	words := strings.Fields(strings.ToLower(raw))
	for i, w := range words {
		if i > 0 && nameParticles[w] {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// BirthdayTemplate returns the school's greeting, or the default text while it
// has not written one.
func (s *CommsService) BirthdayTemplate(ctx context.Context) (BirthdayTemplate, error) {
	if s.templates == nil {
		return BirthdayTemplate{}, errBirthdaysOff
	}
	t, found, err := s.templates.BirthdayTemplate(ctx)
	if err != nil {
		return BirthdayTemplate{}, err
	}
	if !found {
		return BirthdayTemplate{Title: defaultBirthdayTitle, Body: defaultBirthdayBody, IsDefault: true}, nil
	}
	return t, nil
}

// UpdateBirthdayTemplate replaces the school's greeting. Title and body are
// trimmed, required, and bounded in characters; updatedBy is who saved it.
func (s *CommsService) UpdateBirthdayTemplate(ctx context.Context, title, body, updatedBy string) (BirthdayTemplate, error) {
	if s.templates == nil {
		return BirthdayTemplate{}, errBirthdaysOff
	}
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	switch {
	case title == "" || body == "":
		return BirthdayTemplate{}, glerrors.New(glerrors.KindInvalid, "the greeting needs a title and a message")
	case utf8.RuneCountInString(title) > maxBirthdayTitle:
		return BirthdayTemplate{}, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("the title is at most %d characters", maxBirthdayTitle))
	case utf8.RuneCountInString(body) > maxBirthdayBody:
		return BirthdayTemplate{}, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("the message is at most %d characters", maxBirthdayBody))
	case strings.TrimSpace(updatedBy) == "":
		return BirthdayTemplate{}, glerrors.New(glerrors.KindInvalid, "who saved the greeting is required")
	}
	return s.templates.SaveBirthdayTemplate(ctx, BirthdayTemplate{
		Title:     title,
		Body:      body,
		UpdatedAt: s.now(),
		UpdatedBy: strings.TrimSpace(updatedBy),
	})
}
