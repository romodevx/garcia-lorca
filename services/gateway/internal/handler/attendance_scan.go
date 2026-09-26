package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

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
// A scan is against the list ON SCREEN: the body names its date, and ownership,
// the mode check and the group's enrollments are all resolved for that day —
// never the server's school day, which differs from the screen's after 18:00
// local (schoolToday is UTC) and whenever a past list is corrected.
//
// Both surfaces (teacher /teaching/attendance/scan, director /attendance/scan)
// first run their own list gate (assertOwnsList / resolveList), and only then
// look the card up: a list the caller may not touch answers before any card is
// read. The teacher's scan also obeys the save's date window.

// ScanCardVerifier reads a scanned card code for attendance: whose card it is
// and whether the office revoked it. The pupil's admission status plays no
// part (owner decision, QA ISSUE-004): a pupil on the group's list counts
// whatever their admission. The Connect StudentsClient satisfies it.
type ScanCardVerifier interface {
	ScanCard(ctx context.Context, code string) (client.ScannedCard, error)
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
// code read the same ("invalid"). A real card of a pupil outside the group
// answers "foreign", on purpose — the owner wants a clear warning at the door.
// That does reveal that a code is SOME real card, but never whose (no name, no
// id), and card tokens carry 128 random bits, so finding real codes by
// guessing is impractical.
const (
	msgScanInvalid   = "Código no válido"
	msgScanSuspended = "Credencial suspendida"
	msgScanForeign   = "Este alumno no pertenece a este grupo"
)

// codeRefLen is how many hex characters of the code's SHA-256 a log line keeps:
// enough to correlate repeated scans, never enough to rebuild the code.
const codeRefLen = 8

// scanRequest is one scan against one list: the list of Date (YYYY-MM-DD).
type scanRequest struct {
	Date      string `json:"date"`
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
// one of the caller's OWN lists (assertOwnsList) of the body's date, a day the
// caller may still save (today back to attendanceWindowDays; never the future).
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
	req, date, ok := decodeScan(w, r)
	if !ok {
		return
	}
	access, ok := h.assertOwnsList(w, r, p, req.GroupID, req.Date, req.SubjectID)
	if !ok {
		return
	}
	if msg := teacherWindowRefusal(date, h.now()); msg != "" {
		writeBadRequest(w, msg)
		return
	}
	resolveScan(w, r, h.cards, h.attendance, access, req)
}

// Scan handles POST /api/v1/attendance/scan — resolve a card against any list
// the director may read (resolveList) of the body's date. The director has no
// date window.
//
// It runs behind RequireAuth + rotation, the module_attendance and
// module_student_card guards and RequirePermission("attendance:read").
func (h *AttendanceHandler) Scan(w http.ResponseWriter, r *http.Request) {
	if h.cards == nil {
		writeNotFound(w)
		return
	}
	req, _, ok := decodeScan(w, r)
	if !ok {
		return
	}
	access, ok := h.resolveList(w, r, req.GroupID, req.Date, req.SubjectID)
	if !ok {
		return
	}
	resolveScan(w, r, h.cards, h.svc, access, req)
}

// decodeScan reads the body and its required date, answering 400 itself for a
// malformed body or a date that is not YYYY-MM-DD.
func decodeScan(w http.ResponseWriter, r *http.Request) (scanRequest, time.Time, bool) {
	var req scanRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return scanRequest{}, time.Time{}, false
	}
	date, ok := parseListDate(req.Date)
	if !ok {
		writeBadRequest(w, msgInvalidDate)
		return scanRequest{}, time.Time{}, false
	}
	return req, date, true
}

// resolveScan reads the card and the group's active enrollments on the list's
// date — reusing the roster the gate already read, else one read concurrently
// with the card — and answers the verdict:
//
//   - 422 invalid: not a card code, or a code no card carried;
//   - 422 foreign: a card whose pupil has no active enrollment in the group —
//     judged before suspension, and answered with no data of that pupil;
//   - 409 suspended: the office revoked the card;
//   - 200: the row, with the pupil's name — whatever the pupil's admission
//     status, since the group's roster already lists them.
//
// A downstream failure is an error, never a verdict.
func resolveScan(w http.ResponseWriter, r *http.Request, cards ScanCardVerifier, rosters ListRosterReader, access listAccess, req scanRequest) {
	ctx := r.Context()
	var (
		card             client.ScannedCard
		roster           client.AttendanceRoster
		cardErr, listErr error
		reads            errgroup.Group
	)
	reads.Go(func() error {
		card, cardErr = cards.ScanCard(ctx, req.Code)
		return nil
	})
	reads.Go(func() error {
		roster, listErr = rosterOf(ctx, rosters, access, req.GroupID, req.Date, req.SubjectID)
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
	pupil := card.Student
	enrollmentID, enrolled := enrollmentOf(roster, pupil.ID)
	switch {
	case !enrolled:
		refuseScan(w, r, req, scanResultForeign, http.StatusUnprocessableEntity, msgScanForeign)
	case card.Revoked:
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
