package handler

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// --- deleting a level (ADR 0021) -------------------------------------------------
//
// A level's data lives in six services. Deleting it is a saga run from here:
// every service moves its rows into its own 7-day archive (never a deleted_at
// flag), in an order that never leaves a visible dangling reference —
//
//	grades → attendance → finance (tuition plans) → uniforms → students → config
//
// — dependents first, the structure and the level itself last. If a step fails,
// every step that ran (the failing one included: its commit may have landed
// before the error) is restored, newest first, and the director is told nothing
// was deleted. Restoring runs the other way: config first, so the structure
// exists before anything that points at it comes back.
//
// Money is never part of it: finance archives tuition plans only, and the
// preview reports charges and payments as what STAYS.

// LevelStructureService is config's side (the ConfigClient): the structure,
// the saga's lock and the trash's ledger.
type LevelStructureService interface {
	PreviewLevelDeletion(ctx context.Context, levelID string) (client.LevelStructure, error)
	BeginLevelDeletion(ctx context.Context, in client.BeginLevelDeletionRecord) (client.LevelDeletion, error)
	ArchiveLevel(ctx context.Context, in client.ArchiveLevelRecord) (client.LevelCounts, client.LevelDeletion, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (client.LevelRestoreReport, error)
	MarkLevelDeletionRestored(ctx context.Context, batchID string, partial bool) (client.LevelDeletion, error)
	CancelLevelDeletion(ctx context.Context, batchID string) (client.LevelDeletion, error)
	FailLevelDeletion(ctx context.Context, batchID string, pending []string, skipped []client.LevelRestoreSkip) (client.LevelDeletion, error)
	SweepLevelDeletions(ctx context.Context) ([]client.LevelDeletion, int64, error)
	MarkLevelDeletionPurged(ctx context.Context, batchID string) (client.LevelDeletion, error)
	PurgeLevelBatch(ctx context.Context, batchID string) (int64, error)
	ListLevelDeletions(ctx context.Context) ([]client.LevelDeletion, error)
	GetLevelDeletion(ctx context.Context, batchID string) (client.LevelDeletion, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
	BeginLevelRestore(ctx context.Context, batchID string) (client.LevelDeletion, error)
	AbandonLevelRestore(ctx context.Context, batchID string) (client.LevelDeletion, error)
}

// LevelPeopleService is students' side: its preview also names the people, and
// its restore names the enrollments it could not bring back.
type LevelPeopleService interface {
	PreviewLevelDeletion(ctx context.Context, scope client.LevelScope) (client.StudentsLevelPreview, error)
	ArchiveLevel(ctx context.Context, batchID string, scope client.LevelScope) (client.LevelCounts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (client.LevelRestoreReport, error)
	PurgeLevelBatch(ctx context.Context, batchID string) (int64, error)
}

// LevelDataService is grades', attendance's, finance's and uniforms' side.
// skipEnrollmentIDs keeps the rows of enrollments students could not restore
// archived (grades and attendance; the others have none).
type LevelDataService interface {
	PreviewLevelDeletion(ctx context.Context, scope client.LevelScope) (client.LevelCounts, error)
	ArchiveLevel(ctx context.Context, batchID string, scope client.LevelScope) (client.LevelCounts, error)
	RestoreLevelBatch(ctx context.Context, batchID string, skipEnrollmentIDs []string) (client.LevelRestoreReport, error)
	PurgeLevelBatch(ctx context.Context, batchID string) (int64, error)
}

// PasswordVerifier re-checks the caller's password the way sign-in does (iam).
type PasswordVerifier interface {
	VerifyPassword(ctx context.Context, accessToken, password string) error
}

// LevelDeletionDeps wires the handler.
type LevelDeletionDeps struct {
	Config     LevelStructureService
	Students   LevelPeopleService
	Grades     LevelDataService
	Attendance LevelDataService
	Finance    LevelDataService
	Uniforms   LevelDataService
	Passwords  PasswordVerifier
	Users      AttendanceUserReader
	Tokens     *DeletionTokens
	Now        func() time.Time
	NewID      func() string
	Log        *slog.Logger
	// SagaTimeout bounds the deletion saga (0: sagaTimeout). Compensation gets
	// its own, longer budget on a context detached from the saga's.
	SagaTimeout time.Duration
}

// LevelDeletionHandler serves the level deletion and its trash. Every route
// runs behind module_config + auth + rotation + levels:delete (owner-only).
type LevelDeletionHandler struct{ d LevelDeletionDeps }

// NewLevelDeletionHandler wires the handler; a nil clock, id mint or logger
// gets the production default.
func NewLevelDeletionHandler(d LevelDeletionDeps) *LevelDeletionHandler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewID == nil {
		d.NewID = newBatchID
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.SagaTimeout <= 0 {
		d.SagaTimeout = sagaTimeout
	}
	return &LevelDeletionHandler{d: d}
}

// The refusals of the level deletion (text/plain + X-Error-Code, like the rest).
const (
	codePreviewStale   = "PREVIEW_STALE"
	msgPreviewStale    = "Los datos cambiaron; revisa el resumen de nuevo"
	codeNameMismatch   = "NAME_MISMATCH"
	msgNameMismatch    = "El nombre no coincide con el del nivel"
	codeReauthFailed   = "REAUTH_FAILED"
	msgReauthFailed    = "La contraseña no es correcta"
	codeLevelNameTaken = "LEVEL_NAME_TAKEN"
	msgLevelNameTaken  = "Ya existe un nivel con ese nombre; renómbralo antes de restaurar"
	codeTrashExpired   = "TRASH_EXPIRED"
	msgTrashExpired    = "Pasaron más de 7 días; este nivel ya no se puede restaurar"
	msgDeleteFailed    = "No se pudo eliminar el nivel; no se borró nada. Intenta de nuevo"
	msgRestoreHalfway  = "La restauración quedó a medias; vuelve a intentarlo: lo ya restaurado no se duplica"

	codeDeletionInProgress = "DELETION_IN_PROGRESS"
	msgDeletionInProgress  = "Ya se está eliminando este nivel; espera un momento y revisa la Papelera."
	codeDeletionIncomplete = "DELETION_INCOMPLETE"
	codeRestoreIncomplete  = "RESTORE_INCOMPLETE"
	codeRestoreInProgress  = "RESTORE_IN_PROGRESS"
	msgRestoreInProgress   = "Ya se está restaurando este nivel; espera un momento y revisa la Papelera."
)

// Batch states, as config's ledger names them.
const (
	deletionStatusDeleted         = "deleted"
	deletionStatusFailedPartial   = "failed_partial"
	deletionStatusRestoredPartial = "restored_partial"
	deletionStatusRestoring       = "restoring"
	deletionStatusDeleting        = "deleting"
	deletionStatusPurged          = "purged"
)

// inTrash reports whether the director sees (and may restore) a batch.
func inTrash(status string) bool {
	return status == deletionStatusDeleted || status == deletionStatusFailedPartial || status == deletionStatusRestoredPartial
}

// sagaTimeout bounds a whole deletion. The saga runs on a context detached
// from the request: a director closing the tab mid-way must not leave half a
// level deleted.
const sagaTimeout = 2 * time.Minute

// compensationTimeout bounds undoing a failed saga (or a restore). It runs on
// a context detached from the saga's — whose deadline may be exactly why the
// saga failed — and is generous: a compensation cut short leaves data in the
// trash that the director must restore by hand.
const compensationTimeout = 5 * time.Minute

// The saga's steps, by the names config's ledger records (pending_steps).
const (
	stepGrades     = "grades"
	stepAttendance = "attendance"
	stepFinance    = "finance"
	stepUniforms   = "uniforms"
	stepStudents   = "students"
	stepConfig     = "config"
)

// restoreOrder is the way back: the structure first, then the people, then
// what hangs off them. It is the deletion order reversed.
var restoreOrder = []string{stepConfig, stepStudents, stepUniforms, stepFinance, stepAttendance, stepGrades}

// The kinds the preview and the trash show: every kind a service archives that
// a director would recognise. The rest (the level itself, term months, report
// card lines, grades' outbox) are signed into the token and recorded, but mean
// nothing to a person.
var shownKinds = []string{
	"gradeLevels", "groups", "subjects", "terms", "teachingAssignments", "students", "enrollments",
	"enrollmentTransfers", "attendanceSessions", "attendanceMarks", "monthlyGrades", "periodGrades", "gradeEntries",
	"periodClosures", "reportCards", "tuitionPlans", "uniformItems", "schoolEvents",
}

// keptKinds are finance's rows that STAY: reported, never archived.
var keptKinds = []string{"charges", "payments"}

// levelPlan is everything one deletion reaches, freshly resolved.
type levelPlan struct {
	name     string
	deleting bool // another deletion of this level runs (config's lock)
	scope    client.LevelScope
	counts   client.LevelCounts // what would be archived, every kind
	keeps    client.LevelCounts // money that stays
}

// resolve asks every service what deleting the level would reach: config for
// the structure, students for the people, then the rest with both.
func (h *LevelDeletionHandler) resolve(ctx context.Context, levelID string) (levelPlan, error) {
	st, err := h.d.Config.PreviewLevelDeletion(ctx, levelID)
	if err != nil {
		return levelPlan{}, err
	}
	plan := levelPlan{name: st.LevelName, deleting: st.Deleting, counts: client.LevelCounts{}, keeps: client.LevelCounts{}}
	plan.scope = client.LevelScope{
		LevelID: st.LevelID, GradeLevelIDs: st.GradeLevelIDs, GroupIDs: st.GroupIDs,
		SubjectIDs: st.SubjectIDs, TermIDs: st.TermIDs,
	}
	plan.counts.Add(st.Counts)
	people, err := h.d.Students.PreviewLevelDeletion(ctx, plan.scope)
	if err != nil {
		return levelPlan{}, err
	}
	plan.scope.StudentIDs, plan.scope.EnrollmentIDs = people.StudentIDs, people.EnrollmentIDs
	plan.counts.Add(people.Counts)
	for _, svc := range []LevelDataService{h.d.Grades, h.d.Attendance, h.d.Finance, h.d.Uniforms} {
		counts, err := svc.PreviewLevelDeletion(ctx, plan.scope)
		if err != nil {
			return levelPlan{}, err
		}
		plan.counts.Add(counts)
	}
	for _, k := range keptKinds {
		if n, ok := plan.counts[k]; ok {
			plan.keeps[k] = n
			delete(plan.counts, k)
		}
	}
	return plan, nil
}

// empty is a level nothing academic hangs off: no grades, no pupils, no
// enrollments. It still goes to the trash (its terms, plans and events with it).
func (p levelPlan) empty() bool {
	return p.counts["gradeLevels"] == 0 && p.counts["students"] == 0 && p.counts["enrollments"] == 0
}

// withoutKept drops the kept-money kinds: they are never a deletion count, even
// if a service reported them.
func withoutKept(c client.LevelCounts) client.LevelCounts {
	out := client.LevelCounts{}
	out.Add(c)
	for _, k := range keptKinds {
		delete(out, k)
	}
	return out
}

func shown(c client.LevelCounts) map[string]int64 {
	out := make(map[string]int64, len(shownKinds))
	for _, k := range shownKinds {
		out[k] = c[k]
	}
	return out
}

func kept(c client.LevelCounts) map[string]int64 {
	out := make(map[string]int64, len(keptKinds))
	for _, k := range keptKinds {
		out[k] = c[k]
	}
	return out
}

type levelDeletionPreview struct {
	LevelID      string           `json:"levelId"`
	LevelName    string           `json:"levelName"`
	Empty        bool             `json:"empty"`
	Counts       map[string]int64 `json:"counts"`
	Keeps        map[string]int64 `json:"keeps"`
	ConfirmToken string           `json:"confirmToken"`
	ExpiresAt    time.Time        `json:"expiresAt"`
}

// Preview: GET /api/v1/config/levels/{id}/deletion-preview
func (h *LevelDeletionHandler) Preview(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	levelID := r.PathValue("id")
	plan, err := h.resolve(r.Context(), levelID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if plan.deleting {
		writeCodedError(w, http.StatusConflict, codeDeletionInProgress, msgDeletionInProgress)
		return
	}
	token, exp := h.d.Tokens.Issue(p.TenantID, plan.scope.LevelID, plan.counts)
	writeJSON(w, http.StatusOK, levelDeletionPreview{
		LevelID: plan.scope.LevelID, LevelName: plan.name, Empty: plan.empty(),
		Counts: shown(plan.counts), Keeps: kept(plan.keeps), ConfirmToken: token, ExpiresAt: exp,
	})
}

type deleteLevelRequest struct {
	ConfirmName  string `json:"confirmName"`
	ConfirmToken string `json:"confirmToken"`
	Password     string `json:"password"`
}

type deleteLevelResponse struct {
	BatchID         string           `json:"batchId"`
	DeletedAt       time.Time        `json:"deletedAt"`
	RestorableUntil time.Time        `json:"restorableUntil"`
	Counts          map[string]int64 `json:"counts"`
}

// Delete: POST /api/v1/config/levels/{id}/delete
//
// The checks, all here on the server: the level is not already being deleted
// (409 DELETION_IN_PROGRESS), the confirm token is live (409 PREVIEW_STALE),
// the typed name is the level's (400 NAME_MISMATCH) and the password is theirs
// (401 REAUTH_FAILED, checked by iam in whichever mode sign-in runs). Then the
// freeze (BeginLevelDeletion) FIRST, and only under it the scope is resolved
// and its counts compared with the token's: a grade captured between a check
// and the lock would otherwise slip past the preview. Counts that moved let
// the freeze go and answer 409 PREVIEW_STALE. Only then the saga.
func (h *LevelDeletionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req deleteLevelRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	h.sweepLazily(r.Context())
	st, ok := h.confirmable(w, r, req)
	if !ok {
		return
	}

	// Detached from the request: a closed tab must not leave half a level.
	detached := context.WithoutCancel(r.Context())
	ctx, cancel := context.WithTimeout(detached, h.d.SagaTimeout)
	defer cancel()
	batch := h.d.NewID()
	// The lock: one saga per level, one saga per token. A double click's second
	// request stops here (DELETION_IN_PROGRESS), before any step runs.
	if _, err := h.d.Config.BeginLevelDeletion(ctx, client.BeginLevelDeletionRecord{
		BatchID: batch, LevelID: st.LevelID, TokenHash: TokenHash(req.ConfirmToken),
		DeletedBy: p.UserID, DeletedByName: takerName(ctx, h.d.Users, p),
	}); err != nil {
		writeConnectError(w, err)
		return
	}
	plan, err := h.resolve(ctx, st.LevelID)
	if err != nil {
		h.letGo(detached, batch)
		writeConnectError(w, err)
		return
	}
	if !h.d.Tokens.Verify(req.ConfirmToken, p.TenantID, plan.scope.LevelID, plan.counts) {
		h.letGo(detached, batch)
		writeCodedError(w, http.StatusConflict, codePreviewStale, msgPreviewStale)
		return
	}
	rec, err := h.runSaga(ctx, batch, plan)
	if err != nil {
		h.failDeletion(detached, w, batch, plan.scope.LevelID, err, rec)
		return
	}
	h.d.Log.InfoContext(ctx, "level deleted into the trash",
		slog.String("batch", batch), slog.String("level", plan.scope.LevelID), slog.String("by", p.UserID))
	writeJSON(w, http.StatusOK, deleteLevelResponse{
		BatchID: batch, DeletedAt: rec.deletion.DeletedAt, RestorableUntil: rec.deletion.RestorableUntil,
		Counts: shown(rec.deletion.Counts),
	})
}

// confirmable runs the checks that need no lock: the level exists and is not
// being deleted, the token is live, the name is the level's and the password
// the caller's. It writes the refusal and answers false on any of them.
func (h *LevelDeletionHandler) confirmable(w http.ResponseWriter, r *http.Request, req deleteLevelRequest) (client.LevelStructure, bool) {
	st, err := h.d.Config.PreviewLevelDeletion(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		writeConnectError(w, err)
		return st, false
	case st.Deleting:
		writeCodedError(w, http.StatusConflict, codeDeletionInProgress, msgDeletionInProgress)
		return st, false
	case !h.d.Tokens.Live(req.ConfirmToken):
		writeCodedError(w, http.StatusConflict, codePreviewStale, msgPreviewStale)
		return st, false
	case !strings.EqualFold(strings.TrimSpace(req.ConfirmName), strings.TrimSpace(st.LevelName)):
		writeCodedError(w, http.StatusBadRequest, codeNameMismatch, msgNameMismatch)
		return st, false
	}
	if err := h.d.Passwords.VerifyPassword(r.Context(), middleware.TokenFrom(r), req.Password); err != nil {
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			writeCodedError(w, http.StatusUnauthorized, codeReauthFailed, msgReauthFailed)
			return st, false
		}
		writeConnectError(w, err)
		return st, false
	}
	return st, true
}

// letGo releases a freeze taken for a deletion that never started (nothing
// was archived): the batch is cancelled. A failure only lingers until the
// sweep takes the batch for dead.
func (h *LevelDeletionHandler) letGo(detached context.Context, batch string) {
	ctx, cancel := context.WithTimeout(detached, compensationTimeout)
	defer cancel()
	if _, err := h.d.Config.CancelLevelDeletion(ctx, batch); err != nil {
		h.d.Log.ErrorContext(ctx, "level deletion freeze was not released",
			slog.String("batch", batch), slog.Any("error", err))
	}
}

// sagaStep is one service's archive.
type sagaStep struct {
	name    string
	archive func(ctx context.Context) error
}

// sagaRun is what the saga has done so far.
type sagaRun struct {
	ran      []string // the steps attempted, in order (the failing one included)
	deletion client.LevelDeletion
}

func (h *LevelDeletionHandler) runSaga(ctx context.Context, batch string, plan levelPlan) (sagaRun, error) {
	run := sagaRun{}
	summary := client.LevelCounts{}
	data := func(name string, svc LevelDataService) sagaStep {
		return sagaStep{name: name, archive: func(ctx context.Context) error {
			counts, err := svc.ArchiveLevel(ctx, batch, plan.scope)
			summary.Add(withoutKept(counts))
			return err
		}}
	}
	steps := []sagaStep{
		data(stepGrades, h.d.Grades),
		data(stepAttendance, h.d.Attendance),
		data(stepFinance, h.d.Finance),
		data(stepUniforms, h.d.Uniforms),
		{name: stepStudents, archive: func(ctx context.Context) error {
			counts, err := h.d.Students.ArchiveLevel(ctx, batch, plan.scope)
			summary.Add(counts)
			return err
		}},
		{name: stepConfig, archive: func(ctx context.Context) error {
			_, rec, err := h.d.Config.ArchiveLevel(ctx, client.ArchiveLevelRecord{
				BatchID: batch, LevelID: plan.scope.LevelID, Summary: summary,
			})
			run.deletion = rec
			return err
		}},
	}
	for _, s := range steps {
		run.ran = append(run.ran, s.name)
		if err := s.archive(ctx); err != nil {
			return run, fmt.Errorf("level deletion step %s: %w", s.name, err)
		}
	}
	return run, nil
}

// restoreOutcome is what restoring some steps of a batch achieved.
type restoreOutcome struct {
	reports []client.LevelRestoreReport
	skipped []client.LevelRestoreSkip
	pending []string // steps whose archive may still hold rows (failed or skipped)
	failed  []string // steps that answered an error
	missing []string // steps whose archive was not found where it had to be
	err     error    // the first error a step answered
}

func (o restoreOutcome) clean() bool {
	return len(o.pending) == 0 && len(o.missing) == 0
}

// restoreSteps restores the named steps of a batch in restoreOrder. Students'
// unrestored enrollments reach grades and attendance as skip ids. A step whose
// archive is NotFound is fine when tolerated (it may never have archived) and
// "missing" otherwise. stopOnError stops at the first failure (a restore must
// not bring back rows whose structure did not come back); a compensation keeps
// going to undo as much as it can.
func (h *LevelDeletionHandler) restoreSteps(ctx context.Context, batch string, plan restorePlan) restoreOutcome {
	return h.runRestoreSteps(ctx, batch, plan.steps, plan.tolerate, true)
}

// runRestoreSteps is restoreSteps with the compensation's knobs.
func (h *LevelDeletionHandler) runRestoreSteps(ctx context.Context, batch string, steps []string, tolerateMissing, stopOnError bool) restoreOutcome {
	want := make(map[string]bool, len(steps))
	for _, s := range steps {
		want[s] = true
	}
	var (
		out        restoreOutcome
		unrestored []string
	)
	data := map[string]LevelDataService{
		stepUniforms: h.d.Uniforms, stepFinance: h.d.Finance, stepAttendance: h.d.Attendance, stepGrades: h.d.Grades,
	}
	for _, step := range restoreOrder {
		if !want[step] {
			continue
		}
		var (
			rep client.LevelRestoreReport
			err error
		)
		switch step {
		case stepConfig:
			rep, err = h.d.Config.RestoreLevelBatch(ctx, batch)
		case stepStudents:
			rep, err = h.d.Students.RestoreLevelBatch(ctx, batch)
			unrestored = rep.Unrestored
		default:
			rep, err = data[step].RestoreLevelBatch(ctx, batch, unrestored)
		}
		switch {
		case err != nil && isNotFound(err) && tolerateMissing:
			continue
		case err != nil && isNotFound(err):
			out.missing = append(out.missing, step)
			if stopOnError {
				return out
			}
			continue
		case err != nil:
			h.d.Log.ErrorContext(ctx, "level restore step failed",
				slog.String("batch", batch), slog.String("step", step), slog.Any("error", err))
			out.failed = append(out.failed, step)
			out.pending = append(out.pending, step)
			if out.err == nil {
				out.err = err
			}
			if stopOnError {
				return out
			}
			continue
		}
		out.reports = append(out.reports, rep)
		if len(rep.Skipped) > 0 {
			h.d.Log.WarnContext(ctx, "level restore step skipped rows; they stay in the trash",
				slog.String("batch", batch), slog.String("step", step), slog.Any("skipped", rep.Skipped))
			out.skipped = append(out.skipped, rep.Skipped...)
			out.pending = append(out.pending, step)
		}
	}
	return out
}

// failDeletion compensates every step that ran — the failing one included, its
// commit may have landed — newest first, on a fresh context detached from the
// (possibly expired) saga's, with its own budget. Fully undone: the lock is
// released (cancelled) and the answer says nothing was deleted. Not fully: the
// batch is recorded failed_partial with the steps that still hold rows and what
// was skipped, so the trash lists it and Restaurar recovers it; the answer
// names the folio and never claims nothing was deleted.
func (h *LevelDeletionHandler) failDeletion(detached context.Context, w http.ResponseWriter, batch, levelID string, cause error, run sagaRun) {
	ctx, cancel := context.WithTimeout(detached, compensationTimeout)
	defer cancel()
	h.d.Log.ErrorContext(ctx, "level deletion failed; compensating",
		slog.String("batch", batch), slog.String("level", levelID), slog.Any("error", cause))
	out := h.runRestoreSteps(ctx, batch, run.ran, true, false)
	if out.clean() {
		if _, err := h.d.Config.CancelLevelDeletion(ctx, batch); err != nil {
			// Everything came back; only the lock lingers until the sweep
			// takes the saga for dead and lists it in the trash.
			h.d.Log.ErrorContext(ctx, "level deletion undone but its lock was not released",
				slog.String("batch", batch), slog.Any("error", err))
		}
		http.Error(w, msgDeleteFailed, http.StatusInternalServerError)
		return
	}
	h.d.Log.ErrorContext(ctx, "level deletion compensation incomplete; recorded in the trash",
		slog.String("batch", batch), slog.Any("pending", out.pending), slog.Any("failed", out.failed),
		slog.Any("skipped", out.skipped))
	msg := fmt.Sprintf("No se pudo eliminar el nivel y no se pudo deshacer por completo (folio %s). "+
		"Aparece en la Papelera como eliminación incompleta: Restaurar recupera lo que se alcanzó a borrar.", batch)
	if _, err := h.d.Config.FailLevelDeletion(ctx, batch, out.pending, out.skipped); err != nil {
		h.d.Log.ErrorContext(ctx, "level deletion could not be recorded as incomplete",
			slog.String("batch", batch), slog.Any("error", err))
		msg = fmt.Sprintf("No se pudo eliminar el nivel y no se pudo deshacer por completo. "+
			"Avisa a soporte con el folio %s; en unos minutos aparecerá en la Papelera para restaurarlo.", batch)
	}
	writeCodedError(w, http.StatusInternalServerError, codeDeletionIncomplete, msg)
}

type trashItem struct {
	BatchID         string           `json:"batchId"`
	LevelID         string           `json:"levelId"`
	LevelName       string           `json:"levelName"`
	DeletedAt       time.Time        `json:"deletedAt"`
	RestorableUntil time.Time        `json:"restorableUntil"`
	DeletedByName   string           `json:"deletedByName"`
	Counts          map[string]int64 `json:"counts"`
	Status          string           `json:"status"`
}

// Trash: GET /api/v1/config/level-trash — the batches still restorable or
// awaiting the sweep, newest first: deleted, failed_partial (a deletion that
// failed and could not be fully undone), restored_partial (rows that did not
// fit stay here for a retry) and restoring (a restore running right now).
func (h *LevelDeletionHandler) Trash(w http.ResponseWriter, r *http.Request) {
	h.sweepLazily(r.Context())
	recs, err := h.d.Config.ListLevelDeletions(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	items := make([]trashItem, 0, len(recs))
	for _, d := range recs {
		if !inTrash(d.Status) && d.Status != deletionStatusRestoring {
			continue
		}
		items = append(items, trashItem{
			BatchID: d.BatchID, LevelID: d.LevelID, LevelName: d.LevelName, DeletedAt: d.DeletedAt,
			RestorableUntil: d.RestorableUntil, DeletedByName: d.DeletedByName, Counts: shown(d.Counts), Status: d.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type restoreSkip struct {
	Kind   string `json:"kind"`
	Count  int64  `json:"count"`
	Reason string `json:"reason"`
}

type restoreResponse struct {
	Restored map[string]int64 `json:"restored"`
	Skipped  []restoreSkip    `json:"skipped"`
}

// Restore: POST /api/v1/config/level-trash/{batchId}/restore
//
// Config first marks the batch restoring (compare-and-set, the 7-day window
// judged on the database's clock): the sweep never purges a restoring batch,
// and a second restore is refused (409 RESTORE_IN_PROGRESS). The steps follow
// the status the restore started from. Config's structure comes back first,
// then students, whose unrestored enrollments grades and attendance then keep
// archived. A deleted batch asks every step and a missing archive is an error
// (410 TRASH_EXPIRED when nothing came back, 500 RESTORE_INCOMPLETE naming the
// missing part otherwise), never a silent success; a failed_partial batch asks
// only its pending steps, and a step that never archived is fine. Every step
// converges on retry. Only when every step answers is the batch closed:
// restored, or restored_partial if something did not fit (it stays in the
// trash). Any other end puts the batch back where it was (AbandonLevelRestore;
// config's watchdog does it after 30 minutes if even that fails).
func (h *LevelDeletionHandler) Restore(w http.ResponseWriter, r *http.Request) {
	batch := r.PathValue("batchId")
	h.sweepLazily(r.Context())
	rec, err := h.d.Config.GetLevelDeletion(r.Context(), batch)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !h.restorable(r.Context(), w, rec) {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), compensationTimeout)
	defer cancel()
	begun, err := h.d.Config.BeginLevelRestore(ctx, batch)
	if err != nil {
		if connectErrorCode(err) == codeTrashExpired {
			writeCodedError(w, http.StatusGone, codeTrashExpired, msgTrashExpired)
			return
		}
		writeConnectError(w, err)
		return
	}
	closed := false
	defer func() {
		if !closed {
			h.abandonRestore(ctx, batch)
		}
	}()
	out := h.restoreSteps(ctx, batch, restoreStepsFrom(begun))
	if !h.restoreAnswered(ctx, w, batch, out) {
		return
	}
	partial := len(out.skipped) > 0
	if _, err := h.d.Config.MarkLevelDeletionRestored(ctx, batch, partial); err != nil {
		writeConnectError(w, err)
		return
	}
	closed = true
	h.d.Log.InfoContext(ctx, "level restored from the trash",
		slog.String("batch", batch), slog.String("level", rec.LevelID), slog.Bool("partial", partial))
	writeJSON(w, http.StatusOK, mergeReports(out.reports))
}

// restorePlan is which steps a restore asks and whether a step with no
// archive is fine.
type restorePlan struct {
	steps    []string
	tolerate bool
}

// restoreStepsFrom picks the steps by the status the restore started from: a
// failed_partial batch asks only its pending steps; a restored_partial one
// tolerates the steps already done.
func restoreStepsFrom(begun client.LevelDeletion) restorePlan {
	switch begun.RestoringFrom {
	case deletionStatusFailedPartial:
		return restorePlan{steps: begun.PendingSteps, tolerate: true}
	case deletionStatusRestoredPartial:
		return restorePlan{steps: restoreOrder, tolerate: true}
	}
	return restorePlan{steps: restoreOrder}
}

// restoreAnswered writes the refusal of a restore that did not fully answer
// and reports false; true lets the batch close.
func (h *LevelDeletionHandler) restoreAnswered(ctx context.Context, w http.ResponseWriter, batch string, out restoreOutcome) bool {
	switch {
	case connectErrorCode(out.err) == codeTrashExpired:
		writeCodedError(w, http.StatusGone, codeTrashExpired, msgTrashExpired)
	case len(out.failed) > 0 && len(out.reports) == 0 && connectErrorCode(out.err) != "":
		// A coded refusal before anything came back (config's exact name clash).
		writeConnectError(w, out.err)
	case len(out.failed) > 0:
		http.Error(w, msgRestoreHalfway, http.StatusInternalServerError)
	case len(out.missing) > 0 && len(out.reports) == 0:
		writeCodedError(w, http.StatusGone, codeTrashExpired, msgTrashExpired)
	case len(out.missing) > 0:
		h.d.Log.ErrorContext(ctx, "level restore found archives missing",
			slog.String("batch", batch), slog.Any("missing", out.missing))
		writeCodedError(w, http.StatusInternalServerError, codeRestoreIncomplete, fmt.Sprintf(
			"Parte de este nivel ya no estaba en la papelera (%s); lo demás se restauró", strings.Join(out.missing, ", ")))
	default:
		return true
	}
	return false
}

// abandonRestore puts a batch whose restore did not close back where it was.
// A failure is logged: config's watchdog does the same after 30 minutes.
func (h *LevelDeletionHandler) abandonRestore(ctx context.Context, batch string) {
	if _, err := h.d.Config.AbandonLevelRestore(ctx, batch); err != nil {
		h.d.Log.ErrorContext(ctx, "level restore could not fall back; the watchdog will",
			slog.String("batch", batch), slog.Any("error", err))
	}
}

// restorable refuses a batch that is gone (404), purged (410), or whose level
// name ANOTHER level took meanwhile (409) — its own level, already back from a
// partial restore, is no clash. It writes the refusal. The 7-day window is
// config's to judge, on the database's clock.
func (h *LevelDeletionHandler) restorable(ctx context.Context, w http.ResponseWriter, rec client.LevelDeletion) bool {
	switch {
	case rec.Status == deletionStatusRestoring:
		writeCodedError(w, http.StatusConflict, codeRestoreInProgress, msgRestoreInProgress)
		return false
	case rec.Status == deletionStatusPurged:
		writeCodedError(w, http.StatusGone, codeTrashExpired, msgTrashExpired)
		return false
	case !inTrash(rec.Status):
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return false
	}
	levels, err := h.d.Config.ListLevels(ctx)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	for _, l := range levels {
		if l.ID == rec.LevelID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(l.Name), strings.TrimSpace(rec.LevelName)) {
			writeCodedError(w, http.StatusConflict, codeLevelNameTaken, msgLevelNameTaken)
			return false
		}
	}
	return true
}

func mergeReports(reports []client.LevelRestoreReport) restoreResponse {
	restored := client.LevelCounts{}
	skipped := []restoreSkip{}
	for _, rep := range reports {
		restored.Add(rep.Restored)
		for _, s := range rep.Skipped {
			skipped = append(skipped, restoreSkip{Kind: s.Kind, Count: s.Count, Reason: s.Reason})
		}
	}
	sort.SliceStable(skipped, func(i, j int) bool { return skipped[i].Kind < skipped[j].Kind })
	return restoreResponse{Restored: shown(restored), Skipped: skipped}
}

// newBatchID mints a random (v4) uuid for a saga.
func newBatchID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("level deletion: no randomness for a batch id: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- the retention sweep (ADR 0021) -------------------------------------------
//
// The data services scale to zero, so none of them can be trusted to wake up
// on the hour. The gateway runs warm (min-instances=1): it asks config — whose
// restorable_until is the one clock of the trash — which batches are past
// their window, has every service purge each one, and only then closes it in
// config. A batch a service could not purge stays open and is retried on the
// next run. The same sweep runs lazily before the trash is listed, a level is
// deleted or a batch is restored.

// sweepSteps are the services that hold a batch's archive.
func (h *LevelDeletionHandler) sweepSteps() []struct {
	name  string
	purge func(context.Context, string) (int64, error)
} {
	return []struct {
		name  string
		purge func(context.Context, string) (int64, error)
	}{
		{stepGrades, h.d.Grades.PurgeLevelBatch},
		{stepAttendance, h.d.Attendance.PurgeLevelBatch},
		{stepFinance, h.d.Finance.PurgeLevelBatch},
		{stepUniforms, h.d.Uniforms.PurgeLevelBatch},
		{stepStudents, h.d.Students.PurgeLevelBatch},
		{stepConfig, h.d.Config.PurgeLevelBatch},
	}
}

// Sweep purges every batch past its restorable_until and answers how many it
// closed. It also lets config turn sagas that died mid-way into failed_partial
// batches, restorable from the trash. Each run is logged.
func (h *LevelDeletionHandler) Sweep(ctx context.Context) (int64, error) {
	expired, stalled, err := h.d.Config.SweepLevelDeletions(ctx)
	if err != nil {
		return 0, fmt.Errorf("level trash sweep: %w", err)
	}
	if stalled > 0 {
		h.d.Log.WarnContext(ctx, "level deletions stuck mid-saga moved to the trash", slog.Int64("batches", stalled))
	}
	var (
		closed   int64
		firstErr error
	)
	for _, rec := range expired {
		if rec.Status == deletionStatusRestoring || rec.Status == deletionStatusDeleting {
			// Config never lists a running batch; an older config might.
			continue
		}
		if err := h.purgeBatch(ctx, rec.BatchID); err != nil {
			h.d.Log.ErrorContext(ctx, "level trash purge failed; retried on the next sweep",
				slog.String("batch", rec.BatchID), slog.Any("error", err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		closed++
		h.d.Log.InfoContext(ctx, "level trash batch purged", slog.String("batch", rec.BatchID))
	}
	return closed, firstErr
}

func (h *LevelDeletionHandler) purgeBatch(ctx context.Context, batch string) error {
	for _, s := range h.sweepSteps() {
		if _, err := s.purge(ctx, batch); err != nil {
			return fmt.Errorf("purging %s: %w", s.name, err)
		}
	}
	if _, err := h.d.Config.MarkLevelDeletionPurged(ctx, batch); err != nil {
		return fmt.Errorf("closing the purged batch: %w", err)
	}
	return nil
}

// sweepLazily runs the sweep before a trash operation. A failure is logged and
// never blocks the operation: the hourly run retries it.
func (h *LevelDeletionHandler) sweepLazily(ctx context.Context) {
	if _, err := h.Sweep(ctx); err != nil {
		h.d.Log.ErrorContext(ctx, "lazy level trash sweep failed", slog.Any("error", err))
	}
}
