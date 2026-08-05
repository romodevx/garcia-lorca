package postgres

import (
	"strings"

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
	}
}

func toDomainLevel(m levelModel) (domain.Level, error) {
	gran, err := domain.ParseEnrollmentGranularity(m.EnrollmentGranularity)
	if err != nil {
		return domain.Level{}, glerrors.Wrap(glerrors.KindInternal, "unknown enrollment granularity in db: "+m.EnrollmentGranularity, err)
	}
	return domain.NewLevel(m.ID, m.TenantID, m.Name, m.SEPCode, gran)
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

func toDomainTerm(m termModel) (domain.Term, error) {
	return domain.NewTerm(m.ID, m.TenantID, m.CycleID, m.LevelID, m.Name, m.Ordinal, m.StartsOn, m.EndsOn)
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
