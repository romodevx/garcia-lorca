package handler

import (
	"context"
	"log/slog"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/pkg/blockers"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// Group corrections (T8, ADR 0036). A group captured wrong is renamed in place
// — allowed with pupils in it, everything keys on the id — or deleted when
// nothing anywhere keeps it. No service owns the whole answer: config keeps
// the group and its classes, students its enrollments, attendance its pass
// lists, grades its report cards. So the gateway asks each, refuses with one
// sentence naming every blocker, and only then lets config delete.
//
// The check reaches every service whatever its module flag: a disabled
// module's rows still name the group (same reasoning as the level saga).
//
// The race: a pupil enrolled between the check and config's delete would be
// left pointing at a deleted group. Config re-checks its own references with
// the group row locked; the other services' cannot be locked from here.
// ADR 0036 records why that window is accepted.

// GroupEditConfig is the slice of config the group corrections need.
type GroupEditConfig interface {
	GetGroup(ctx context.Context, id string) (client.GroupResult, error)
	RenameGroup(ctx context.Context, id, name string) (client.GroupResult, error)
	GroupUsage(ctx context.Context, groupID string) (client.ConfigGroupUsage, error)
	DeleteGroup(ctx context.Context, id string) error
}

// GroupEditStudents is what students keeps of a group and its file labels.
type GroupEditStudents interface {
	GroupUsage(ctx context.Context, groupID string) (client.StudentsGroupUsage, error)
	RefreshGroupName(ctx context.Context, groupID, name string) (int64, error)
}

// GroupEditAttendance is what attendance keeps of a group.
type GroupEditAttendance interface {
	GroupUsage(ctx context.Context, groupID string) (client.AttendanceGroupUsage, error)
}

// GroupEditGrades is what grades keeps of a group.
type GroupEditGrades interface {
	GroupReportCards(ctx context.Context, groupID string) (int64, error)
}

// GroupEditDeps are the services a group correction reaches.
type GroupEditDeps struct {
	Config     GroupEditConfig
	Students   GroupEditStudents
	Attendance GroupEditAttendance
	Grades     GroupEditGrades
}

// GroupEditHandler serves PATCH and DELETE /api/v1/config/groups/{id}.
type GroupEditHandler struct {
	deps GroupEditDeps
}

// NewGroupEditHandler wires the handler.
func NewGroupEditHandler(deps GroupEditDeps) *GroupEditHandler {
	return &GroupEditHandler{deps: deps}
}

// The refusal for deleting a group something keeps.
const (
	codeGroupInUse = "GROUP_IN_USE"
	// codeGroupRelabelPending: renamed in config, pupils' files not yet.
	codeGroupRelabelPending = "GROUP_RELABEL_PENDING"
	adviceKeepHistory       = " Ese historial se conserva; puedes cambiarle el nombre."
	adviceFreeClasses       = " " + blockers.FreeClassesFirst
)

// RenameGroup handles PATCH /api/v1/config/groups/{id} with {name}. Config
// renames the group; then the pupils actively enrolled in it get the stored
// name on their file (receipts, card and family app print it). A relabel
// failure is answered as such: the rename stands, and repeating the request
// (same name) finishes the relabel — both steps are idempotent.
func (h *GroupEditHandler) RenameGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a group id", http.StatusBadRequest)
		return
	}
	var req renameRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if _, ok := catalogName(req.Name); !ok {
		http.Error(w, "name is required (at most 80 characters)", http.StatusBadRequest)
		return
	}
	group, err := h.deps.Config.RenameGroup(r.Context(), id, req.Name)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if _, err := h.deps.Students.RefreshGroupName(r.Context(), group.ID, group.Name); err != nil {
		// Config already holds the new name: saying "could not rename" would
		// be false. The same request again finishes the relabel.
		slog.WarnContext(r.Context(), "group renamed in config but pupils' files not relabelled", "group_id", group.ID, "error", err)
		writeCodedError(w, http.StatusServiceUnavailable, codeGroupRelabelPending,
			"El grupo ya se llama "+group.Name+", pero no se pudo actualizar en la ficha de sus alumnos. Guarda otra vez para terminar.")
		return
	}
	writeJSON(w, http.StatusOK, toGroupResponse(group))
}

// groupUsage is everything every service keeps of one group.
type groupUsage struct {
	config     client.ConfigGroupUsage
	students   client.StudentsGroupUsage
	attendance client.AttendanceGroupUsage
	cards      int64
}

// refusal is the sentence for deleting the group, or "" when nothing keeps it.
// Classes can be freed; history (enrollments, attendance, report cards) is
// kept for good, so the way out then is a rename.
func (u groupUsage) refusal(groupName string) string {
	history := []blockers.Count{
		{N: u.students.Enrollments, One: "inscripción", Many: "inscripciones"},
		{N: u.students.Transfers, One: "cambio de grupo", Many: "cambios de grupo"},
		{N: u.attendance.Sessions, One: "lista de asistencia", Many: "listas de asistencia"},
		{N: u.attendance.Marks, One: "registro de asistencia", Many: "registros de asistencia"},
		{N: u.cards, One: "boleta", Many: "boletas"},
	}
	classes := []blockers.Count{
		{N: u.config.TeachingAssignments, One: "clase asignada", Many: "clases asignadas"},
		{N: u.config.ScheduleBlocks, One: "bloque de horario", Many: "bloques de horario"},
	}
	what := blockers.Describe(append(classes, history...)...)
	if what == "" {
		return ""
	}
	advice := adviceFreeClasses
	if blockers.Describe(history...) != "" {
		advice = adviceKeepHistory
	}
	return "No se puede borrar el grupo " + groupName + ": tiene " + what + "." + advice
}

// collectUsage asks every service, concurrently, what it keeps of the group.
// Any failure fails the whole answer: an unknown count never reads as zero.
func (h *GroupEditHandler) collectUsage(ctx context.Context, groupID string) (groupUsage, error) {
	var u groupUsage
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { u.config, err = h.deps.Config.GroupUsage(ctx, groupID); return err })
	g.Go(func() (err error) { u.students, err = h.deps.Students.GroupUsage(ctx, groupID); return err })
	g.Go(func() (err error) { u.attendance, err = h.deps.Attendance.GroupUsage(ctx, groupID); return err })
	g.Go(func() (err error) { u.cards, err = h.deps.Grades.GroupReportCards(ctx, groupID); return err })
	return u, g.Wait()
}

// DeleteGroup handles DELETE /api/v1/config/groups/{id}: 204 when nothing
// anywhere keeps the group, 409 GROUP_IN_USE naming each blocker otherwise.
func (h *GroupEditHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a group id", http.StatusBadRequest)
		return
	}
	group, err := h.deps.Config.GetGroup(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	usage, err := h.collectUsage(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if msg := usage.refusal(group.Name); msg != "" {
		writeCodedError(w, http.StatusConflict, codeGroupInUse, msg)
		return
	}
	if err := h.deps.Config.DeleteGroup(r.Context(), id); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
