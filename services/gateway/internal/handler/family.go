package handler

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The family surface is the SELF-SCOPED, guardian-facing read model: a parent
// reading their own children's news, account, profile, grades and attendance
// from a phone. It owns no service of its own — it composes the students,
// comms, finance, grades and attendance surfaces behind ONE scoping rule.
//
// The scoping is BY CONSTRUCTION, not by a check on request content: the
// guardian id always comes from the authenticated principal, so the set of
// readable students is whatever the caller is linked to and nothing else. No
// request field (body, query or path) can widen it. This is what makes
// /family/* safe for a parent-facing app, in contrast to the admin routes
// (/comms/notifications?studentId=…, /finance/accounts/{studentId}), which read
// any student by id and therefore stay owner-only.
//
// Routes that carry a {studentId} in the PATH cannot rely on that construction
// alone — the id is caller-supplied — so each one first calls assertLinkedChild
// and answers 404 for any id outside the caller's own children.

// --- consumer-defined reader interfaces -----------------------------------
//
// Each capability is its own small interface (defined where it is consumed), and
// the two composite interfaces below group the ones a single Connect client
// satisfies, so the constructor stays readable.

// GuardianLinkReader resolves the students one guardian is linked to. It is the
// scoping primitive of the family surface: the caller's own children, derived
// from the authenticated principal, never from the request.
type GuardianLinkReader interface {
	ListStudentsForGuardian(ctx context.Context, guardianUserID string) ([]client.GuardianLink, error)
}

// StudentProfileReader reads one child's core record and their enrollments.
// Grades and attendance are keyed by enrollment, not student, so the enrollment
// lookup is how the family surface translates the child id a guardian knows into
// the enrollment id those services key on.
type StudentProfileReader interface {
	GetStudent(ctx context.Context, id string) (client.StudentResult, error)
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
}

// NotificationReader reads one student's notification inbox.
type NotificationReader interface {
	ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]client.Notification, error)
}

// NotificationMarker acknowledges one notification as read. The gateway is
// trusted to call it on the guardian's behalf after the ownership check, so the
// guardian role needs no comms:write of its own.
type NotificationMarker interface {
	MarkNotificationRead(ctx context.Context, id string) (client.Notification, error)
}

// AccountReader reads one student's finance account statement.
type AccountReader interface {
	// GetFamilyAccount is the family's view of the statement: no charge before
	// the day it opens (T5).
	GetFamilyAccount(ctx context.Context, studentID string) (client.AccountStatement, error)
}

// GradeEntriesReader reads the grade entries of one enrollment.
type GradeEntriesReader interface {
	ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]client.GradeEntry, error)
}

// AttendanceRangeReader reads one enrollment's stored attendance exceptions over
// a date range, and which lists were taken for a set of groups (T5: the per-day
// view tells "present" from "nobody took the list").
type AttendanceRangeReader interface {
	ListAttendance(ctx context.Context, enrollmentID, from, to string) ([]client.AttendanceRecord, error)
	ListSessions(ctx context.Context, groupIDs []string, from, to string) ([]client.TakenList, error)
}

// FamilyCatalogReader reads the academic catalog the family grades screen needs
// to render: it turns the bare subject/term UUIDs on a grade entry into the
// human-readable names a parent can read. The Connect ConfigClient (the same one
// backing the /config/* routes) satisfies it. Passing "" lists the whole
// tenant-scoped catalog, which is what the id→name maps are built from.
type FamilyCatalogReader interface {
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
	ListTerms(ctx context.Context, cycleID, levelID string) ([]client.TermResult, error)
	// ListCycles bounds the attendance view's group windows to each
	// enrollment's school year.
	ListCycles(ctx context.Context) ([]client.CycleResult, error)
}

// TransferHistoryReader reads a student's group moves. A transfer rewrites the
// enrollment's group in place, so this is the only record of which group a
// pupil was in on a past day.
type TransferHistoryReader interface {
	ListTransfers(ctx context.Context, studentID string) ([]client.Transfer, error)
}

// FamilyStudentsReader is the students surface the family handler consumes: the
// guardian-link scoping primitive plus the per-child profile/enrollment reads.
// The Connect StudentsClient satisfies it.
type FamilyStudentsReader interface {
	GuardianLinkReader
	StudentProfileReader
	TransferHistoryReader
}

// FamilyCommsReader is the comms surface the family handler consumes: reading the
// inbox and acknowledging one notification. The Connect CommsClient satisfies it.
type FamilyCommsReader interface {
	NotificationReader
	NotificationMarker
}

// FamilyHandler is the guardian-facing, self-scoped family surface.
type FamilyHandler struct {
	students   FamilyStudentsReader
	comms      FamilyCommsReader
	finance    AccountReader
	grades     GradeEntriesReader
	attendance AttendanceRangeReader
	catalog    FamilyCatalogReader
	statements StatementSource
	now        func() time.Time
}

// FamilyOption configures a FamilyHandler (functional options).
type FamilyOption func(*FamilyHandler)

// WithFamilyClock replaces the wall clock the attendance view's "today" and
// default month come from (tests pin it).
func WithFamilyClock(now func() time.Time) FamilyOption {
	return func(h *FamilyHandler) { h.now = now }
}

// WithFamilyStatements wires the ledger the family's statement routes read (T7).
func WithFamilyStatements(src StatementSource) FamilyOption {
	return func(h *FamilyHandler) { h.statements = src }
}

// NewFamilyHandler wires the handler to the students (guardian links + profile),
// comms (inbox + acknowledgement), finance (account), grades, attendance and
// catalog (config, for grade subject/term names) readers. Any of
// finance/grades/attendance/catalog may be nil for a handler that only serves
// the routes that do not use them.
func NewFamilyHandler(students FamilyStudentsReader, comms FamilyCommsReader, finance AccountReader, grades GradeEntriesReader, attendance AttendanceRangeReader, catalog FamilyCatalogReader, opts ...FamilyOption) *FamilyHandler {
	h := &FamilyHandler{students: students, comms: comms, finance: finance, grades: grades, attendance: attendance, catalog: catalog, now: time.Now}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// --- ownership gate --------------------------------------------------------

// assertLinkedChild reports whether studentID is among the children the guardian
// is linked to. It is the ownership gate of every by-child family route: the
// child set is derived from the PRINCIPAL, so no path/query/body value can widen
// it. Callers treat a false result as NOT FOUND — the same answer a non-existent
// id gets — so a guardian can never probe whether another family's child exists.
func (h *FamilyHandler) assertLinkedChild(ctx context.Context, guardianUserID, studentID string) (bool, error) {
	return ownsChild(ctx, h.students, guardianUserID, studentID)
}

// ownsChild reports whether a guardian is linked to a pupil: the one question
// every by-child family route asks before answering (the card's too).
func ownsChild(ctx context.Context, links GuardianLinkReader, guardianUserID, studentID string) (bool, error) {
	linked, err := links.ListStudentsForGuardian(ctx, guardianUserID)
	if err != nil {
		return false, err
	}
	for _, l := range linked {
		if l.StudentID == studentID {
			return true, nil
		}
	}
	return false, nil
}

// linkedEnrollments resolves the enrollments of one already-ownership-checked
// child. Grades and attendance are keyed by enrollment, not student, so a child
// with more than one enrollment (a mid-cycle transfer, a prior cycle)
// contributes each, and the aggregate spans them.
func (h *FamilyHandler) linkedEnrollments(ctx context.Context, studentID string) ([]client.Enrollment, error) {
	return h.students.ListEnrollments(ctx, studentID, "", "")
}

// --- students list ---------------------------------------------------------

// ListStudents handles GET /api/v1/family/students — the core record of every
// child the CALLER is linked to, so the mobile app can discover the studentIds it
// then feeds to the by-child routes. A guardian with no linked children gets an
// empty list.
//
// It runs behind RequireAuth + rotation, the module_students guard and
// RequirePermission("family:read"). Scoped by construction exactly like
// /family/notifications: the child set is derived from the PRINCIPAL, so no
// path/query/body value can widen it. A duplicate link is fetched once — the same
// dedupe the notification fan-out applies — and the result is never nil, so the
// JSON carries [] rather than null.
func (h *FamilyHandler) ListStudents(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	links, err := h.students.ListStudentsForGuardian(r.Context(), p.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]familyStudentResponse, 0, len(links))
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		if seen[link.StudentID] {
			continue // a duplicate link must not fetch the child twice
		}
		seen[link.StudentID] = true
		student, err := h.students.GetStudent(r.Context(), link.StudentID)
		if err != nil {
			writeConnectError(w, err)
			return
		}
		out = append(out, toFamilyStudentResponse(student))
	}
	writeJSON(w, http.StatusOK, map[string]any{"students": out})
}

// --- notifications ---------------------------------------------------------

// Notifications handles GET /api/v1/family/notifications?unreadOnly=true — every
// notification of every child the CALLER is linked to, tagged with the child it
// belongs to, newest first. A guardian with no linked children gets an empty list.
//
// It must run behind RequireAuth (which injects the principal) plus the module
// guards of BOTH modules it composes (comms and students) and
// RequirePermission("family:read"). One child's inbox failing fails the whole
// read: a partial list would read as "your child has no news", which is worse
// than an error.
//
// The fan-out is sequential on purpose — a guardian has a handful of children, so
// the round-trips are few and the child order stays stable and reproducible. That
// fan-out is child-major, so the aggregate is re-sorted before it is written: the
// family app shows one chronological stream, not one block per child.
func (h *FamilyHandler) Notifications(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	links, err := h.students.ListStudentsForGuardian(r.Context(), p.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	unreadOnly := r.URL.Query().Get("unreadOnly") == unreadOnlyTrue
	dated := make([]datedNotification, 0, len(links))
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		if seen[link.StudentID] {
			continue // a duplicate link must not duplicate the child's news
		}
		seen[link.StudentID] = true
		notifications, err := h.comms.ListNotifications(r.Context(), link.StudentID, unreadOnly)
		if err != nil {
			writeConnectError(w, err)
			return
		}
		for _, n := range notifications {
			res := toNotificationResponse(n)
			if res.StudentID == "" {
				res.StudentID = link.StudentID // always tagged, even if comms omits it
			}
			dated = append(dated, newDatedNotification(res))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": newestFirst(dated)})
}

// MarkNotificationRead handles POST /api/v1/family/notifications/{notificationId}/read
// — a guardian acknowledging one of their own children's notifications.
//
// The guardian role holds family:read, NOT comms:write, so the admin
// /comms/notifications/{id}/read route is unreachable to it. Ownership is proved
// the same way the inbox is scoped: the notification must belong to one of the
// caller's linked children, found by the same fan-out Notifications does. An id
// outside that set answers 404 — never 403 — so a guardian cannot probe whether
// another family's notification exists. Only then is the mark applied, by the
// gateway on the guardian's behalf.
func (h *FamilyHandler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	notificationID := r.PathValue("notificationId")
	owned, err := h.ownsNotification(r.Context(), p.UserID, notificationID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !owned {
		writeNotFound(w)
		return
	}
	res, err := h.comms.MarkNotificationRead(r.Context(), notificationID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toNotificationResponse(res))
}

// ownsNotification reports whether notificationID is in the inbox of any child
// the guardian is linked to. It searches ALL notifications (not only unread) so
// re-marking an already-read notification is still recognised as owned, keeping
// the acknowledgement idempotent.
func (h *FamilyHandler) ownsNotification(ctx context.Context, guardianUserID, notificationID string) (bool, error) {
	links, err := h.students.ListStudentsForGuardian(ctx, guardianUserID)
	if err != nil {
		return false, err
	}
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		if seen[link.StudentID] {
			continue
		}
		seen[link.StudentID] = true
		notifications, err := h.comms.ListNotifications(ctx, link.StudentID, false)
		if err != nil {
			return false, err
		}
		for _, n := range notifications {
			if n.ID == notificationID {
				return true, nil
			}
		}
	}
	return false, nil
}

// --- account ---------------------------------------------------------------

// Account handles GET /api/v1/family/students/{studentId}/account — the child's
// full finance statement (charges, payments, outstanding balance), the same
// shape the admin /finance/accounts/{studentId} route returns, but scoped to the
// caller's own child.
func (h *FamilyHandler) Account(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	studentID := r.PathValue("studentId")
	if !h.guardOwnsChild(w, r, p.UserID, studentID) {
		return
	}
	res, err := h.finance.GetFamilyAccount(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	payments := make([]paymentResponse, 0, len(res.Payments))
	for _, pay := range res.Payments {
		payments = append(payments, toPaymentResponse(pay))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"charges":           toChargeResponses(res.Charges),
		"payments":          payments,
		"totalBalanceCents": res.TotalBalanceCents,
		// The family's credit. The school sees it on its own statement, and a
		// parent who overpaid seeing only the debt would be told they owe money
		// the school is already holding for them.
		"unallocatedCents": res.UnallocatedCents,
		// What the school is asking for TODAY, already with the surcharge that
		// has accrued and net of the early-payment reward — the same figure the
		// counter would quote. totalBalanceCents above is what the year still
		// owes (the statement's balance, credit netted — T5); a parent shown
		// only that one is told a number nobody is asking them for.
		"dueTodayCents": res.DueTodayCents,
		// Its parts, so a parent asking why is answered by the screen.
		"surchargeCents":     res.SurchargeCents,
		"earlyDiscountCents": res.EarlyDiscountCents,
		// The day it was worked out for: these move overnight.
		"asOf": res.AsOf,
	})
}

// --- profile ---------------------------------------------------------------

// familyStudentResponse is the child core-field projection shared by the student
// list and the profile: the fields a parent needs to identify and place a child,
// with the wire enums already flattened to their canonical names.
type familyStudentResponse struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"` // the paternal surname
	// MaternalLastName is empty when not captured (T3).
	MaternalLastName string `json:"maternalLastName"`
	Level            string `json:"level"`   // the school's name for the level
	LevelID          string `json:"levelId"` // the config level id
	GradeGroup       string `json:"gradeGroup"`
}

func toFamilyStudentResponse(s client.StudentResult) familyStudentResponse {
	return familyStudentResponse{
		ID:               s.ID,
		FirstName:        s.FirstName,
		LastName:         s.LastName,
		MaternalLastName: s.MaternalLastName,
		Level:            s.Level,
		LevelID:          s.LevelID,
		GradeGroup:       s.GradeGroup,
	}
}

// familyProfileResponse is the child's core record plus the current active
// enrollment, if any. The group and teacher DISPLAY NAMES are intentionally not
// resolved here.
//
// TODO(pendiente-backend): enrich CurrentEnrollment with the group and teacher
// display names via the config service. The ids are enough for the app to drive
// follow-up reads today, and resolving names would couple the family surface to
// config for a purely cosmetic gain.
type familyProfileResponse struct {
	familyStudentResponse
	CurrentEnrollment *enrollmentResponse `json:"currentEnrollment"`
}

// Profile handles GET /api/v1/family/students/{studentId}/profile — the child's
// core fields (name, level, grade/group) and their current active enrollment.
func (h *FamilyHandler) Profile(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	studentID := r.PathValue("studentId")
	if !h.guardOwnsChild(w, r, p.UserID, studentID) {
		return
	}
	student, err := h.students.GetStudent(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	enrollments, err := h.linkedEnrollments(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	profile := familyProfileResponse{familyStudentResponse: toFamilyStudentResponse(student)}
	if current, ok := currentEnrollment(enrollments); ok {
		res := toEnrollmentResponse(current)
		profile.CurrentEnrollment = &res
	}
	writeJSON(w, http.StatusOK, profile)
}

// currentEnrollment picks the child's active enrollment. A child normally has
// exactly one; if none is active (all transferred/withdrawn) there is no current
// placement and the profile carries a null enrollment.
func currentEnrollment(enrollments []client.Enrollment) (client.Enrollment, bool) {
	for _, e := range enrollments {
		if e.Status == "active" {
			return e, true
		}
	}
	return client.Enrollment{}, false
}

// --- grades ----------------------------------------------------------------

// familyGradeEntryResponse is the family-route-local grade projection: the same
// entry the admin route returns (bare subject/term ids kept) PLUS the
// human-readable subject and term names the parent-facing screen needs to
// render. The admin grades handler is untouched; only this surface enriches.
// A name that cannot be resolved degrades to "" — never an error.
type familyGradeEntryResponse struct {
	gradeEntryResponse
	SubjectName string `json:"subjectName"`
	TermName    string `json:"termName"`
}

// Grades handles GET /api/v1/family/students/{studentId}/grades?termId=… — the
// child's grade entries, optionally narrowed to one term. Grades are keyed by
// enrollment, so the child id is resolved to its enrollment(s) first and the
// entries are aggregated across them. Each entry is enriched with its subject
// and term display names, resolved once per request from the config catalog.
func (h *FamilyHandler) Grades(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	studentID := r.PathValue("studentId")
	if !h.guardOwnsChild(w, r, p.UserID, studentID) {
		return
	}
	enrollments, err := h.linkedEnrollments(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	termID := r.URL.Query().Get("termId")
	var entries []client.GradeEntry
	for _, e := range enrollments {
		found, err := h.grades.ListGradeEntries(r.Context(), e.ID, termID)
		if err != nil {
			writeConnectError(w, err)
			return
		}
		entries = append(entries, found...)
	}
	// Resolve the id→name maps ONCE, and only when there is something to enrich.
	// A catalog lookup miss leaves the maps empty, so the names degrade to "" and
	// the read still succeeds — the same graceful degradation /me applies.
	subjectNames, termNames := h.catalogNames(r.Context(), len(entries) > 0)
	out := make([]familyGradeEntryResponse, 0, len(entries))
	for _, entry := range entries {
		out = append(out, familyGradeEntryResponse{
			gradeEntryResponse: toGradeEntryResponse(entry),
			SubjectName:        subjectNames[entry.SubjectID],
			TermName:           termNames[entry.TermID],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"gradeEntries": out})
}

// catalogNames builds the subject and term id→name maps used to enrich a grades
// read. It is best-effort by design: a nil catalog, no entries to enrich, or a
// failed lookup each yields an empty map, and the caller reads a missing id as
// "" — a display gap, never a failed read. Both non-nil maps are always
// returned, so lookups never panic.
func (h *FamilyHandler) catalogNames(ctx context.Context, enrich bool) (subjects, terms map[string]string) {
	subjects, terms = map[string]string{}, map[string]string{}
	if !enrich || h.catalog == nil {
		return subjects, terms
	}
	if subs, err := h.catalog.ListSubjects(ctx, ""); err == nil {
		for _, s := range subs {
			subjects[s.ID] = s.Name
		}
	}
	if tms, err := h.catalog.ListTerms(ctx, "", ""); err == nil {
		for _, t := range tms {
			terms[t.ID] = t.Name
		}
	}
	return subjects, terms
}

// --- shared helpers --------------------------------------------------------

// guardOwnsChild runs the ownership gate and writes the failure response itself,
// so each by-child handler reads as a single guard line. It returns true only
// when the read may proceed; on an unlinked/guessed id it writes 404 (never 403),
// and on a lookup error it writes the mapped status.
func (h *FamilyHandler) guardOwnsChild(w http.ResponseWriter, r *http.Request, guardianUserID, studentID string) bool {
	owned, err := h.assertLinkedChild(r.Context(), guardianUserID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	if !owned {
		writeNotFound(w)
		return false
	}
	return true
}

// writeNotFound answers 404 with the standard status text, matching the shape
// writeConnectError produces for a NotFound code. It is how the family surface
// hides a child the caller is not linked to, leaking nothing about whether the
// id exists.
func writeNotFound(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

// datedNotification pairs one rendered notification with its parsed creation
// instant, so the aggregate is sorted without re-parsing RFC3339 per comparison.
type datedNotification struct {
	res     notificationResponse
	created time.Time
}

// newDatedNotification parses CreatedAt once. An absent or unparseable instant
// yields the zero time, which sorts last — a notification with no usable date
// belongs at the bottom of the stream, not the top.
func newDatedNotification(res notificationResponse) datedNotification {
	created, err := time.Parse(time.RFC3339, res.CreatedAt)
	if err != nil {
		return datedNotification{res: res}
	}
	return datedNotification{res: res, created: created}
}

// newestFirst orders the aggregated inbox by creation instant, descending, and
// projects it back to the response shape. The sort is stable so notifications
// created at the same instant keep the fan-out's child order, making the response
// reproducible. The result is never nil, so the JSON carries [] rather than null.
func newestFirst(dated []datedNotification) []notificationResponse {
	sort.SliceStable(dated, func(i, j int) bool {
		return dated[i].created.After(dated[j].created)
	})
	out := make([]notificationResponse, 0, len(dated))
	for _, d := range dated {
		out = append(out, d.res)
	}
	return out
}
