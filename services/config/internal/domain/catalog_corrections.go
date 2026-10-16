package domain

import (
	"strings"

	"github.com/garcia-lorca/pkg/blockers"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The refusal codes of the catalog corrections (T8, ADR 0036): fixing a grade
// or group captured wrong without deleting its level, and the school order.
const (
	CodeGradeNameTaken     RefusalCode = "GRADE_NAME_TAKEN"
	CodeGradeLevelInUse    RefusalCode = "GRADE_LEVEL_IN_USE"
	CodeGroupNameTaken     RefusalCode = "GROUP_NAME_TAKEN"
	CodeGroupInUse         RefusalCode = "GROUP_IN_USE"
	CodeLevelOrderMismatch RefusalCode = "LEVEL_ORDER_MISMATCH"
)

// The sentences a person reads for the refusals above.
const (
	msgGradeNameBlank     = "El nombre del grado no puede quedar vacío."
	msgGroupNameBlank     = "El nombre del grupo no puede quedar vacío."
	msgGradeNameTaken     = "Ya existe un grado con ese nombre en este nivel."
	msgGroupNameTaken     = "Ya existe un grupo con ese nombre en este grado y ciclo."
	msgLevelOrderMismatch = "La lista de niveles no coincide con los niveles del colegio. Recarga la página e inténtalo de nuevo."
)

// GradeNameTaken is the refusal for a grade name its level already has.
func GradeNameTaken() error {
	return refusal(glerrors.KindConflict, CodeGradeNameTaken, msgGradeNameTaken)
}

// GroupNameTaken is the refusal for a group name its grade already has in the
// cycle.
func GroupNameTaken() error {
	return refusal(glerrors.KindConflict, CodeGroupNameTaken, msgGroupNameTaken)
}

// Renamed returns the same grade — same id, level and ordinal — under a new,
// trimmed name. Groups, subjects and periods point at the id, so they follow.
func (g GradeLevel) Renamed(name string) (GradeLevel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return GradeLevel{}, glerrors.New(glerrors.KindInvalid, msgGradeNameBlank)
	}
	g.name = name
	return g, nil
}

// Renamed returns the same group — same id, cycle, grade and capacity — under a
// new, trimmed name. Enrollments, attendance and report cards key on the id.
func (g Group) Renamed(name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, glerrors.New(glerrors.KindInvalid, msgGroupNameBlank)
	}
	g.name = name
	return g, nil
}

// GradeLevelUsage is what a grade still holds. Every row is config's own and
// hangs from the grade by an ON DELETE RESTRICT foreign key.
type GradeLevelUsage struct {
	Groups            int64
	Subjects          int64
	EvaluationPeriods int64
}

// Refusal is nil when the grade holds nothing; otherwise the conflict that
// names what blocks deleting it and how many.
func (u GradeLevelUsage) Refusal(gradeName string) error {
	what := blockers.Describe(
		blockers.Count{N: u.Groups, One: "grupo", Many: "grupos"},
		blockers.Count{N: u.Subjects, One: "materia", Many: "materias"},
		blockers.Count{N: u.EvaluationPeriods, One: "periodo de evaluación", Many: "periodos de evaluación"},
	)
	if what == "" {
		return nil
	}
	return refusal(glerrors.KindConflict, CodeGradeLevelInUse,
		// The way out must always work: a subject with grades can never be
		// deleted, so renaming the grade is offered too (tech-lead, T8).
		"No se puede borrar el grado "+gradeName+": tiene "+what+". Bórralos primero; "+
			"si alguna materia ya tiene calificaciones, puedes cambiarle el nombre al grado.")
}

// GroupUsage is what config keeps of a group: its teaching assignments and the
// timetable blocks placed on them (which go with their assignment).
type GroupUsage struct {
	TeachingAssignments int64
	ScheduleBlocks      int64
}

// Refusal is nil when config keeps nothing of the group; otherwise the
// conflict naming it. Freeing the assignments frees their blocks too, so that
// is the one thing to do first.
func (u GroupUsage) Refusal(groupName string) error {
	what := blockers.Describe(
		blockers.Count{N: u.TeachingAssignments, One: "clase asignada", Many: "clases asignadas"},
		blockers.Count{N: u.ScheduleBlocks, One: "bloque de horario", Many: "bloques de horario"},
	)
	if what == "" {
		return nil
	}
	return refusal(glerrors.KindConflict, CodeGroupInUse,
		"No se puede borrar el grupo "+groupName+": tiene "+what+". "+blockers.FreeClassesFirst)
}

// CheckLevelOrder accepts proposed only when it is exactly the school's levels
// (current), each once, in any order. A partial order would leave the levels
// it forgot wherever they were, so it is refused whole.
func CheckLevelOrder(current, proposed []string) error {
	mismatch := refusal(glerrors.KindInvalid, CodeLevelOrderMismatch, msgLevelOrderMismatch)
	if len(proposed) == 0 || len(proposed) != len(current) {
		return mismatch
	}
	known := make(map[string]bool, len(current))
	for _, id := range current {
		known[id] = true
	}
	for _, id := range proposed {
		if !known[id] {
			return mismatch
		}
		delete(known, id)
	}
	return nil
}
