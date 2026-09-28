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

// LevelStructureService is config's side (the ConfigClient).
type LevelStructureService interface {
	PreviewLevelDeletion(ctx context.Context, levelID string) (client.LevelStructure, error)
	ArchiveLevel(ctx context.Context, in client.ArchiveLevelRecord) (client.LevelCounts, client.LevelDeletion, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (client.LevelRestoreReport, error)
	MarkLevelDeletionRestored(ctx context.Context, batchID string) (client.LevelDeletion, error)
	ListLevelDeletions(ctx context.Context) ([]client.LevelDeletion, error)
	GetLevelDeletion(ctx context.Context, batchID string) (client.LevelDeletion, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
}

// LevelPeopleService is students' side: its preview also names the people.
type LevelPeopleService interface {
	PreviewLevelDeletion(ctx context.Context, scope client.LevelScope) (client.StudentsLevelPreview, error)
	ArchiveLevel(ctx context.Context, batchID string, scope client.LevelScope) (client.LevelCounts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (client.LevelRestoreReport, error)
}

// LevelDataService is grades', attendance's, finance's and uniforms' side.
type LevelDataService interface {
	PreviewLevelDeletion(ctx context.Context, scope client.LevelScope) (client.LevelCounts, error)
	ArchiveLevel(ctx context.Context, batchID string, scope client.LevelScope) (client.LevelCounts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (client.LevelRestoreReport, error)
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
)

// Batch states, as config's ledger names them.
const (
	deletionStatusDeleted = "deleted"
	deletionStatusPurged  = "purged"
)

// sagaTimeout bounds a whole deletion or its compensation. The saga runs on a
// context detached from the request: a director closing the tab mid-way must
// not leave half a level deleted.
const sagaTimeout = 2 * time.Minute

// The kinds the preview and the trash show. Services report more (term months,
// transfers, report card lines…); those are signed into the token and recorded,
// but these are what a person reads.
var shownKinds = []string{
	"gradeLevels", "groups", "subjects", "terms", "teachingAssignments", "students", "enrollments",
	"attendanceMarks", "monthlyGrades", "reportCards", "tuitionPlans", "uniformItems", "schoolEvents",
}

// keptKinds are finance's rows that STAY: reported, never archived.
var keptKinds = []string{"charges", "payments"}

// levelPlan is everything one deletion reaches, freshly resolved.
type levelPlan struct {
	name   string
	scope  client.LevelScope
	counts client.LevelCounts // what would be archived, every kind
	keeps  client.LevelCounts // money that stays
}

// resolve asks every service what deleting the level would reach: config for
// the structure, students for the people, then the rest with both.
func (h *LevelDeletionHandler) resolve(ctx context.Context, levelID string) (levelPlan, error) {
	st, err := h.d.Config.PreviewLevelDeletion(ctx, levelID)
	if err != nil {
		return levelPlan{}, err
	}
	plan := levelPlan{name: st.LevelName, counts: client.LevelCounts{}, keeps: client.LevelCounts{}}
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
// Three checks, all here on the server, in this order: the preview the
// director confirmed is still what would be deleted (409 PREVIEW_STALE), the
// typed name is the level's (400 NAME_MISMATCH), and the password is theirs
// (401 REAUTH_FAILED, checked by iam in whichever mode sign-in runs). Only then
// the saga.
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
	plan, err := h.resolve(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !h.d.Tokens.Verify(req.ConfirmToken, p.TenantID, plan.scope.LevelID, plan.counts) {
		writeCodedError(w, http.StatusConflict, codePreviewStale, msgPreviewStale)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.ConfirmName), strings.TrimSpace(plan.name)) {
		writeCodedError(w, http.StatusBadRequest, codeNameMismatch, msgNameMismatch)
		return
	}
	if err := h.d.Passwords.VerifyPassword(r.Context(), middleware.TokenFrom(r), req.Password); err != nil {
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			writeCodedError(w, http.StatusUnauthorized, codeReauthFailed, msgReauthFailed)
			return
		}
		writeConnectError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), sagaTimeout)
	defer cancel()
	batch := h.d.NewID()
	rec, err := h.runSaga(ctx, batch, plan, p.UserID, takerName(ctx, h.d.Users, p))
	if err != nil {
		h.failDeletion(ctx, w, batch, plan.scope.LevelID, err, rec)
		return
	}
	h.d.Log.InfoContext(ctx, "level deleted into the trash",
		slog.String("batch", batch), slog.String("level", plan.scope.LevelID), slog.String("by", p.UserID))
	writeJSON(w, http.StatusOK, deleteLevelResponse{
		BatchID: batch, DeletedAt: rec.deletion.DeletedAt, RestorableUntil: rec.deletion.RestorableUntil,
		Counts: shown(rec.deletion.Counts),
	})
}

// sagaStep is one service's archive and its way back.
type sagaStep struct {
	name    string
	archive func(ctx context.Context) error
	restore func(ctx context.Context) error
}

// sagaRun is what the saga has done so far.
type sagaRun struct {
	ran      []sagaStep // the steps attempted, in order (the failing one included)
	deletion client.LevelDeletion
}

func (h *LevelDeletionHandler) runSaga(ctx context.Context, batch string, plan levelPlan, by, byName string) (sagaRun, error) {
	run := sagaRun{}
	summary := client.LevelCounts{}
	data := func(name string, svc LevelDataService) sagaStep {
		return sagaStep{
			name: name,
			archive: func(ctx context.Context) error {
				counts, err := svc.ArchiveLevel(ctx, batch, plan.scope)
				summary.Add(withoutKept(counts))
				return err
			},
			restore: func(ctx context.Context) error { _, err := svc.RestoreLevelBatch(ctx, batch); return err },
		}
	}
	steps := []sagaStep{
		data("grades", h.d.Grades),
		data("attendance", h.d.Attendance),
		data("finance", h.d.Finance),
		data("uniforms", h.d.Uniforms),
		{
			name: "students",
			archive: func(ctx context.Context) error {
				counts, err := h.d.Students.ArchiveLevel(ctx, batch, plan.scope)
				summary.Add(counts)
				return err
			},
			restore: func(ctx context.Context) error { _, err := h.d.Students.RestoreLevelBatch(ctx, batch); return err },
		},
		{
			name: "config",
			archive: func(ctx context.Context) error {
				_, rec, err := h.d.Config.ArchiveLevel(ctx, client.ArchiveLevelRecord{
					BatchID: batch, LevelID: plan.scope.LevelID, DeletedBy: by, DeletedByName: byName, Summary: summary,
				})
				run.deletion = rec
				return err
			},
			restore: h.restoreConfig(batch),
		},
	}
	for _, s := range steps {
		run.ran = append(run.ran, s)
		if err := s.archive(ctx); err != nil {
			return run, fmt.Errorf("level deletion step %s: %w", s.name, err)
		}
	}
	return run, nil
}

// restoreConfig undoes config's step. A batch config never recorded answers
// NotFound, which here means there was nothing to undo; a recorded one is also
// closed, so it does not linger in the trash beside the level it describes.
func (h *LevelDeletionHandler) restoreConfig(batch string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if _, err := h.d.Config.RestoreLevelBatch(ctx, batch); err != nil && !isNotFound(err) {
			return err
		}
		if _, err := h.d.Config.MarkLevelDeletionRestored(ctx, batch); err != nil && !isNotFound(err) {
			return err
		}
		return nil
	}
}

// failDeletion compensates every step that ran, newest first, and answers 500.
// If the compensation fails too, the answer carries the batch so the rest can
// be restored by hand; either way the log has the whole story.
func (h *LevelDeletionHandler) failDeletion(ctx context.Context, w http.ResponseWriter, batch, levelID string, cause error, run sagaRun) {
	h.d.Log.ErrorContext(ctx, "level deletion failed; compensating",
		slog.String("batch", batch), slog.String("level", levelID), slog.Any("error", cause))
	var failed []string
	for i := len(run.ran) - 1; i >= 0; i-- {
		s := run.ran[i]
		if err := s.restore(ctx); err != nil {
			failed = append(failed, s.name)
			h.d.Log.ErrorContext(ctx, "level deletion compensation step failed",
				slog.String("batch", batch), slog.String("step", s.name), slog.Any("error", err))
		}
	}
	if len(failed) > 0 {
		http.Error(w, fmt.Sprintf("No se pudo eliminar el nivel y no se pudo deshacer por completo (%s). Avisa a soporte con el folio %s",
			strings.Join(failed, ", "), batch), http.StatusInternalServerError)
		return
	}
	http.Error(w, msgDeleteFailed, http.StatusInternalServerError)
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
// awaiting the sweep (deleted, not restored, not purged), newest first.
func (h *LevelDeletionHandler) Trash(w http.ResponseWriter, r *http.Request) {
	recs, err := h.d.Config.ListLevelDeletions(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	items := make([]trashItem, 0, len(recs))
	for _, d := range recs {
		if d.Status != deletionStatusDeleted {
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
func (h *LevelDeletionHandler) Restore(w http.ResponseWriter, r *http.Request) {
	batch := r.PathValue("batchId")
	rec, err := h.d.Config.GetLevelDeletion(r.Context(), batch)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !h.restorable(r.Context(), w, rec) {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), sagaTimeout)
	defer cancel()
	cfg, err := h.d.Config.RestoreLevelBatch(ctx, batch)
	if err != nil {
		if connectErrorCode(err) == codeTrashExpired {
			writeCodedError(w, http.StatusGone, codeTrashExpired, msgTrashExpired)
			return
		}
		writeConnectError(w, err)
		return
	}
	reports := []client.LevelRestoreReport{cfg}
	for _, s := range []struct {
		name    string
		restore func(context.Context, string) (client.LevelRestoreReport, error)
	}{
		{"students", h.d.Students.RestoreLevelBatch},
		{"uniforms", h.d.Uniforms.RestoreLevelBatch},
		{"finance", h.d.Finance.RestoreLevelBatch},
		{"attendance", h.d.Attendance.RestoreLevelBatch},
		{"grades", h.d.Grades.RestoreLevelBatch},
	} {
		rep, err := s.restore(ctx, batch)
		if err != nil {
			h.d.Log.ErrorContext(ctx, "level restore step failed; the batch stays in the trash",
				slog.String("batch", batch), slog.String("step", s.name), slog.Any("error", err))
			http.Error(w, msgRestoreHalfway, http.StatusInternalServerError)
			return
		}
		reports = append(reports, rep)
	}
	if _, err := h.d.Config.MarkLevelDeletionRestored(ctx, batch); err != nil {
		writeConnectError(w, err)
		return
	}
	h.d.Log.InfoContext(ctx, "level restored from the trash", slog.String("batch", batch), slog.String("level", rec.LevelID))
	writeJSON(w, http.StatusOK, mergeReports(reports))
}

// restorable refuses a batch that is gone (404), expired or purged (410), or
// whose level name another level took meanwhile (409). It writes the refusal.
func (h *LevelDeletionHandler) restorable(ctx context.Context, w http.ResponseWriter, rec client.LevelDeletion) bool {
	switch {
	case rec.Status == deletionStatusPurged, rec.Status == deletionStatusDeleted && h.d.Now().After(rec.RestorableUntil):
		writeCodedError(w, http.StatusGone, codeTrashExpired, msgTrashExpired)
		return false
	case rec.Status != deletionStatusDeleted:
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return false
	}
	levels, err := h.d.Config.ListLevels(ctx)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	for _, l := range levels {
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
