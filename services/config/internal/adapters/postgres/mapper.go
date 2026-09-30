package postgres

import (
	"math"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain: everything above the adapter speaks domain types only.
//
// Enum casing is the sharp edge here. The DDL CHECK on levels stores
// enrollment_granularity in UPPERCASE ('CYCLE'/'TERM'), while the domain's
// canonical String() is lowercase. Writes therefore ToUpper the domain value to
// satisfy the constraint; reads use the case-insensitive domain parsers. cycles'
// status is lowercase on both sides, so it maps verbatim.

// --- cycles ---------------------------------------------------------------

func fromDomainCycle(c domain.Cycle) cycleModel {
	return cycleModel{
		ID:       c.ID(),
		TenantID: c.TenantID(),
		Name:     c.Name(),
		Status:   c.Status().String(), // lowercase; matches the DDL CHECK verbatim.
		StartsOn: c.StartsOn(),
		EndsOn:   c.EndsOn(),
	}
}

func toDomainCycle(m cycleModel) (domain.Cycle, error) {
	status, err := domain.ParseCycleStatus(m.Status)
	if err != nil {
		return domain.Cycle{}, glerrors.Wrap(glerrors.KindInternal, "unknown cycle status in db: "+m.Status, err)
	}
	return domain.NewCycle(m.ID, m.TenantID, m.Name, status, m.StartsOn, m.EndsOn)
}

// --- levels ---------------------------------------------------------------

func fromDomainLevel(l domain.Level) levelModel {
	return levelModel{
		ID:       l.ID(),
		TenantID: l.TenantID(),
		Name:     l.Name(),
		SEPCode:  l.SEPCode(),
		// DDL CHECK requires UPPERCASE 'CYCLE'/'TERM'; domain String() is lowercase.
		EnrollmentGranularity: strings.ToUpper(l.Granularity().String()),
		AttendanceMode:        l.AttendanceMode().String(), // lowercase, as the CHECK spells it
		GradingEnabled:        l.Grading().Enabled(),
		GradeMin:              fromHundredths(l.Grading().MinHundredths()),
		GradeMax:              fromHundredths(l.Grading().MaxHundredths()),
		GradeDecimals:         l.Grading().Decimals(),
		GradePassing:          fromHundredths(l.Grading().PassingHundredths()),
	}
}

// hundredthsPerUnit converts a numeric(5,2) mark to exact hundredths. A value
// with at most two decimals up to 100 survives the float64 trip exactly once
// rounded, so math.Round is the whole conversion.
const hundredthsPerUnit = 100

func fromHundredths(h int) float64 { return float64(h) / hundredthsPerUnit }

func toHundredths(v float64) int { return int(math.Round(v * hundredthsPerUnit)) }

func toDomainLevel(m levelModel) (domain.Level, error) {
	gran, err := domain.ParseEnrollmentGranularity(m.EnrollmentGranularity)
	if err != nil {
		return domain.Level{}, glerrors.Wrap(glerrors.KindInternal, "unknown enrollment granularity in db: "+m.EnrollmentGranularity, err)
	}
	mode, err := domain.ParseAttendanceMode(m.AttendanceMode)
	if err != nil {
		return domain.Level{}, glerrors.Wrap(glerrors.KindInternal, "unknown attendance mode in db: "+m.AttendanceMode, err)
	}
	scale, err := domain.NewGradingScale(m.GradingEnabled, toHundredths(m.GradeMin), toHundredths(m.GradeMax), toHundredths(m.GradePassing), m.GradeDecimals)
	if err != nil {
		return domain.Level{}, glerrors.Wrap(glerrors.KindInternal, "inconsistent grading scale in db", err)
	}
	l, err := domain.NewLevel(m.ID, m.TenantID, m.Name, m.SEPCode, gran)
	if err != nil {
		return domain.Level{}, err
	}
	if l, err = l.WithAttendanceMode(mode); err != nil {
		return domain.Level{}, err
	}
	if l, err = l.WithGrading(scale); err != nil {
		return domain.Level{}, err
	}
	return l.WithDeleting(m.Status == levelStatusDeleting), nil
}

// --- grade levels ---------------------------------------------------------

func fromDomainGradeLevel(g domain.GradeLevel) gradeLevelModel {
	return gradeLevelModel{
		ID:       g.ID(),
		TenantID: g.TenantID(),
		LevelID:  g.LevelID(),
		Name:     g.Name(),
		Ordinal:  g.Ordinal(),
	}
}

func toDomainGradeLevel(m gradeLevelModel) (domain.GradeLevel, error) {
	return domain.NewGradeLevel(m.ID, m.TenantID, m.LevelID, m.Name, m.Ordinal)
}

// --- subjects -------------------------------------------------------------

func fromDomainSubject(s domain.Subject) subjectModel {
	return subjectModel{
		ID:           s.ID(),
		TenantID:     s.TenantID(),
		GradeLevelID: s.GradeLevelID(),
		Name:         s.Name(),
	}
}

func toDomainSubject(m subjectModel) (domain.Subject, error) {
	return domain.NewSubject(m.ID, m.TenantID, m.GradeLevelID, m.Name)
}

// --- terms ----------------------------------------------------------------

func fromDomainTerm(t domain.Term) termModel {
	return termModel{
		ID:       t.ID(),
		TenantID: t.TenantID(),
		CycleID:  t.CycleID(),
		LevelID:  t.LevelID(),
		Name:     t.Name(),
		Ordinal:  t.Ordinal(),
		StartsOn: t.StartsOn(),
		EndsOn:   t.EndsOn(),
	}
}

func toDomainTerm(m termModel, months []time.Time) (domain.Term, error) {
	t, err := domain.NewTerm(m.ID, m.TenantID, m.CycleID, m.LevelID, m.Name, m.Ordinal, m.StartsOn, m.EndsOn)
	if err != nil {
		return domain.Term{}, err
	}
	return t.WithMonths(months)
}

// fromDomainTermMonths is one term_months row per month of t.
func fromDomainTermMonths(t domain.Term) []termMonthModel {
	months := t.Months()
	rows := make([]termMonthModel, 0, len(months))
	for _, m := range months {
		rows = append(rows, termMonthModel{TenantID: t.TenantID(), TermID: t.ID(), CycleID: t.CycleID(), LevelID: t.LevelID(), Month: m})
	}
	return rows
}

// --- groups ---------------------------------------------------------------

func fromDomainGroup(g domain.Group) groupModel {
	return groupModel{
		ID:           g.ID(),
		TenantID:     g.TenantID(),
		CycleID:      g.CycleID(),
		GradeLevelID: g.GradeLevelID(),
		Name:         g.Name(),
		Capacity:     g.Capacity(),
	}
}

func toDomainGroup(m groupModel) (domain.Group, error) {
	return domain.NewGroup(m.ID, m.TenantID, m.CycleID, m.GradeLevelID, m.Name, m.Capacity)
}

// --- teachers -------------------------------------------------------------

// fromDomainTeacher maps a teacher to its row. The domain's optional userID
// ("" = not yet linked) becomes a SQL NULL (*string nil), matching the DDL's
// nullable user_id; a set userID is stored verbatim.
func fromDomainTeacher(t domain.Teacher) teacherModel {
	var userID *string
	if t.HasUser() {
		u := t.UserID()
		userID = &u
	}
	return teacherModel{
		ID:        t.ID(),
		TenantID:  t.TenantID(),
		FirstName: t.FirstName(),
		LastName:  t.LastName(),
		Email:     t.Email(),
		UserID:    userID,
	}
}

// toDomainTeacher maps a row back to a teacher, bridging a SQL NULL user_id back
// to the domain's "" (no IAM link).
func toDomainTeacher(m teacherModel) (domain.Teacher, error) {
	userID := ""
	if m.UserID != nil {
		userID = *m.UserID
	}
	return domain.RestoreTeacher(m.ID, m.TenantID, m.FirstName, m.LastName, m.Email, userID)
}

// --- teaching assignments -------------------------------------------------

func fromDomainTeachingAssignment(a domain.TeachingAssignment) teachingAssignmentModel {
	return teachingAssignmentModel{
		ID:        a.ID(),
		TenantID:  a.TenantID(),
		CycleID:   a.CycleID(),
		GroupID:   a.GroupID(),
		SubjectID: a.SubjectID(),
		TeacherID: a.TeacherID(),
	}
}

func toDomainTeachingAssignment(m teachingAssignmentModel) (domain.TeachingAssignment, error) {
	return domain.NewTeachingAssignment(m.ID, m.TenantID, m.CycleID, m.GroupID, m.SubjectID, m.TeacherID)
}

// --- non-working periods (M16 school calendar) -----------------------------

func fromDomainNonWorkingPeriod(p domain.NonWorkingPeriod) nonWorkingPeriodModel {
	return nonWorkingPeriodModel{
		ID:       p.ID(),
		TenantID: p.TenantID(),
		CycleID:  p.CycleID(),
		Label:    p.Label(),
		Kind:     p.Kind(),
		StartsOn: p.From(),
		EndsOn:   p.To(),
	}
}

func toDomainNonWorkingPeriod(m nonWorkingPeriodModel) (domain.NonWorkingPeriod, error) {
	return domain.NewNonWorkingPeriod(m.ID, m.TenantID, m.CycleID, m.Label, m.Kind, m.StartsOn, m.EndsOn)
}

// --- school events (M16) ---------------------------------------------------

func fromDomainSchoolEvent(e domain.SchoolEvent) schoolEventModel {
	m := schoolEventModel{
		ID:          e.ID(),
		TenantID:    e.TenantID(),
		CycleID:     e.CycleID(),
		Title:       e.Title(),
		Description: e.Description(),
		StartsOn:    e.From(),
		EndsOn:      e.To(),
	}
	if id := e.Audience().LevelID; id != "" {
		m.LevelID = &id
	}
	if id := e.Audience().GroupID; id != "" {
		m.GroupID = &id
	}
	return m
}

func toDomainSchoolEvent(m schoolEventModel) (domain.SchoolEvent, error) {
	audience := domain.EventAudience{}
	if m.LevelID != nil {
		audience.LevelID = *m.LevelID
	}
	if m.GroupID != nil {
		audience.GroupID = *m.GroupID
	}
	return domain.NewSchoolEvent(m.ID, m.TenantID, m.CycleID, m.Title, m.Description,
		m.StartsOn, m.EndsOn, audience)
}

// toDomainSchoolProfile rebuilds the school's details through the domain, so a
// stored row answers to the same rules as a typed one.
func toDomainSchoolProfile(m schoolProfileModel) (domain.SchoolProfile, error) {
	return domain.NewSchoolProfile(m.TenantID, domain.SchoolProfileInput{
		Name: m.Name, LegalName: m.LegalName, RFC: m.RFC, CCT: m.CCT,
		Address: m.Address, Phone: m.Phone, Legend: m.Legend,
	})
}

func fromDomainSchoolProfile(p domain.SchoolProfile) schoolProfileModel {
	return schoolProfileModel{
		TenantID: p.TenantID(), Name: p.Name(), LegalName: p.LegalName(), RFC: p.RFC(), CCT: p.CCT(),
		Address: p.Address(), Phone: p.Phone(), Legend: p.Legend(),
	}
}
