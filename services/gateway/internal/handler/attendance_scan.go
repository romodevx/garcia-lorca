package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"

	connect "connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// attendance_scan.go is T4, attendance by QR: a scanned virtual card (task 13)
// is RESOLVED to a row of a pass list. The scan never writes attendance — the
// screen marks the row and the list is saved by the T3 routes, so a scan obeys
// exactly the rules a save does.
//
// Both surfaces (teacher /teaching/attendance/scan, director /attendance/scan)
// first run their own list gate (assertOwnsList / resolveList), and only then
// look the card up: a list the caller may not touch answers before any card is
// read, so the route cannot be used to probe codes.

// ScanCardVerifier reads a scanned card code: whose card it is and whether it
// counts. The Connect StudentsClient satisfies it.
type ScanCardVerifier interface {
	VerifyCard(ctx context.Context, code string) (client.StudentResult, bool, error)
}

// scanResultHeader tells the web which sound and colour a scan gets: two
// refusals share 422, so the status alone cannot.
const scanResultHeader = "X-Scan-Result"

// The scan verdicts, as scanResultHeader carries them.
const (
	scanResultOK        = "ok"
	scanResultInvalid   = "invalid"
	scanResultSuspended = "suspended"
	scanResultForeign   = "foreign"
)

// The scan refusals (text/plain). An unknown code and something that is not a
// code read the same, so nothing tells a guessed code from a real one.
const (
	msgScanInvalid   = "Código no válido"
	msgScanSuspended = "Credencial suspendida"
	msgScanForeign   = "Este alumno no pertenece a este grupo"
)

// codeRefLen is how many hex characters of the code's SHA-256 a log line keeps:
// enough to correlate repeated scans, never enough to rebuild the code.
const codeRefLen = 8

// scanRequest is one scan against one list.
type scanRequest struct {
	GroupID   string `json:"groupId"`
	SubjectID string `json:"subjectId"`
	Code      string `json:"code"`
}

// scanResponse is the list row the card resolved to.
type scanResponse struct {
	EnrollmentID string `json:"enrollmentId"`
	StudentID    string `json:"studentId"`
	Name         string `json:"name"`
}

// WithScanCards wires the card verification the teacher's scan route needs.
// Without it the route answers 404.
func WithScanCards(cards ScanCardVerifier) TeachingAttendanceOption {
	return func(h *TeachingAttendanceHandler) { h.cards = cards }
}

// WithDirectorScanCards wires the card verification the director's scan route
// needs. Without it the route answers 404.
func WithDirectorScanCards(cards ScanCardVerifier) AttendanceHandlerOption {
	return func(h *AttendanceHandler) { h.cards = cards }
}

// Scan handles POST /api/v1/teaching/attendance/scan — resolve a card against
// one of the caller's OWN lists (assertOwnsList, keyed on today's school day).
//
// It runs behind RequireAuth + rotation, the module_attendance and
// module_student_card guards and RequirePermission("attendance_own:read").
func (h *TeachingAttendanceHandler) Scan(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.cards == nil {
		writeNotFound(w)
		return
	}
	req, ok := decodeScan(w, r)
	if !ok {
		return
	}
	today := schoolToday(h.now()).Format(attendanceDateLayout)
	if _, ok := h.assertOwnsList(w, r, p, req.GroupID, today, req.SubjectID); !ok {
		return
	}
	resolveScan(w, r, h.cards, h.attendance, req, today)
}

// Scan handles POST /api/v1/attendance/scan — resolve a card against any list
// the director may read (resolveList, keyed on today's school day).
//
// It runs behind RequireAuth + rotation, the module_attendance and
// module_student_card guards and RequirePermission("attendance:read").
func (h *AttendanceHandler) Scan(w http.ResponseWriter, r *http.Request) {
	if h.cards == nil {
		writeNotFound(w)
		return
	}
	req, ok := decodeScan(w, r)
	if !ok {
		return
	}
	today := schoolToday(h.now()).Format(attendanceDateLayout)
	if _, ok := h.resolveList(w, r, req.GroupID, today, req.SubjectID); !ok {
		return
	}
	resolveScan(w, r, h.cards, h.svc, req, today)
}

func decodeScan(w http.ResponseWriter, r *http.Request) (scanRequest, bool) {
	var req scanRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return scanRequest{}, false
	}
	return req, true
}

// resolveScan reads the card and the group's active enrollments (one roster
// read, concurrently with the card) and answers the verdict:
//
//   - 422 invalid: not a card code, or a code no card carried;
//   - 422 foreign: a card whose pupil has no active enrollment in the group —
//     judged before suspension, and answered with no data of that pupil;
//   - 409 suspended: the pupil's card does not count (revoked, or the pupil is
//     no longer enrolled at the school);
//   - 200: the row, with the pupil's name.
//
// A downstream failure is an error, never a verdict.
func resolveScan(w http.ResponseWriter, r *http.Request, cards ScanCardVerifier, rosters ListRosterReader, req scanRequest, date string) {
	ctx := r.Context()
	var (
		pupil            client.StudentResult
		valid            bool
		roster           client.AttendanceRoster
		cardErr, listErr error
		reads            errgroup.Group
	)
	reads.Go(func() error {
		pupil, valid, cardErr = cards.VerifyCard(ctx, req.Code)
		return nil
	})
	reads.Go(func() error {
		roster, listErr = rosters.GetDailyRoster(ctx, req.GroupID, date, req.SubjectID)
		return nil
	})
	_ = reads.Wait() // both closures record their own error and return nil
	w.Header().Set("Cache-Control", "no-store")

	if isUnknownCode(cardErr) {
		refuseScan(w, r, req, scanResultInvalid, http.StatusUnprocessableEntity, msgScanInvalid)
		return
	}
	if cardErr != nil {
		writeConnectError(w, cardErr)
		return
	}
	if listErr != nil {
		writeConnectError(w, listErr)
		return
	}
	enrollmentID, enrolled := enrollmentOf(roster, pupil.ID)
	switch {
	case !enrolled:
		refuseScan(w, r, req, scanResultForeign, http.StatusUnprocessableEntity, msgScanForeign)
	case !valid:
		refuseScan(w, r, req, scanResultSuspended, http.StatusConflict, msgScanSuspended)
	default:
		w.Header().Set(scanResultHeader, scanResultOK)
		writeJSON(w, http.StatusOK, scanResponse{EnrollmentID: enrollmentID, StudentID: pupil.ID, Name: fullName(pupil)})
	}
}

// isUnknownCode reports the students' answer for a code that names no card:
// InvalidArgument (not a card code) or NotFound (no card carried it, or its
// pupil is gone).
func isUnknownCode(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument, connect.CodeNotFound:
		return err != nil
	default:
		return false
	}
}

// enrollmentOf finds the pupil's active enrollment among the roster's rows.
func enrollmentOf(roster client.AttendanceRoster, studentID string) (string, bool) {
	if studentID == "" {
		return "", false
	}
	for _, row := range roster.Rows {
		if row.StudentID == studentID {
			return row.EnrollmentID, true
		}
	}
	return "", false
}

// refuseScan answers a refusal and leaves a trace of it — by a hash prefix of
// the code, never the code, which is a credential.
func refuseScan(w http.ResponseWriter, r *http.Request, req scanRequest, result string, status int, msg string) {
	actor := ""
	if p, ok := middleware.PrincipalFrom(r.Context()); ok {
		actor = p.UserID
	}
	slog.InfoContext(r.Context(), "attendance scan refused",
		"result", result, "group_id", req.GroupID, "code_ref", codeRef(req.Code), "by_user_id", actor)
	w.Header().Set(scanResultHeader, result)
	http.Error(w, msg, status)
}

// codeRef is a short, one-way reference to a scanned code for the logs.
func codeRef(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])[:codeRefLen]
}
