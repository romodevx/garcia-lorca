// Command gateway is the entrypoint for the García Lorca BFF / edge gateway.
//
// FASE 0: the gateway boots an HTTP server with its own health endpoint and a
// /readyz that calls the iam service end to end — the gateway half of the
// Sprint-1 pipeline-validation hello-world. The feature-flag middleware
// (internal/middleware) is wired but guards no module routes yet; modules plug
// in as they land.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/config"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/logger"
)

const (
	defaultAddr       = ":8080"
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() {
	log := logger.Default()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	iamBaseURL := os.Getenv("IAM_BASE_URL")
	if iamBaseURL == "" {
		iamBaseURL = "http://localhost:8081"
	}
	studentsBaseURL := os.Getenv("STUDENTS_BASE_URL")
	if studentsBaseURL == "" {
		studentsBaseURL = "http://localhost:8083"
	}
	configBaseURL := os.Getenv("CONFIG_BASE_URL")
	if configBaseURL == "" {
		configBaseURL = "http://localhost:8085"
	}
	gradesBaseURL := os.Getenv("GRADES_BASE_URL")
	if gradesBaseURL == "" {
		gradesBaseURL = "http://localhost:8094"
	}
	financeBaseURL := os.Getenv("FINANCE_BASE_URL")
	if financeBaseURL == "" {
		financeBaseURL = "http://localhost:8093"
	}
	attendanceBaseURL := os.Getenv("ATTENDANCE_BASE_URL")
	if attendanceBaseURL == "" {
		attendanceBaseURL = "http://localhost:8095"
	}
	commsBaseURL := os.Getenv("COMMS_BASE_URL")
	if commsBaseURL == "" {
		commsBaseURL = "http://localhost:8096"
	}
	uniformsBaseURL := os.Getenv("UNIFORMS_BASE_URL")
	if uniformsBaseURL == "" {
		uniformsBaseURL = "http://localhost:8099"
	}

	// flags is the authoritative source of module visibility (default-deny),
	// loaded from ENABLED_MODULES (comma-separated module names). The gateway
	// both enforces the boundary (ModuleGuard ⇒ 404) and reports it (GET
	// /features). Each module wires its routes behind the guard as it lands.
	flags := featureflag.NewInMemory(config.ParseEnabledModules(os.Getenv("ENABLED_MODULES")))

	// Each downstream client gets an ID-token-authenticated HTTP client whose
	// OIDC audience is that service's own base URL, so the invoker-guarded
	// (INTERNAL_ONLY) Cloud Run services accept the call. Local http:// targets
	// fall back to http.DefaultClient (see pkg/gcpauth).
	ctx := context.Background()
	iamHealth := client.NewIAMClient(iamBaseURL, gcpauth.HTTPClient(ctx, iamBaseURL))
	authClient := client.NewAuthClient(iamBaseURL, gcpauth.HTTPClient(ctx, iamBaseURL))
	studentsClient := client.NewStudentsClient(studentsBaseURL, gcpauth.HTTPClient(ctx, studentsBaseURL))
	configClient := client.NewConfigClient(configBaseURL, gcpauth.HTTPClient(ctx, configBaseURL))
	gradesClient := client.NewGradesClient(gradesBaseURL, gcpauth.HTTPClient(ctx, gradesBaseURL))
	financeClient := client.NewFinanceClient(financeBaseURL, gcpauth.HTTPClient(ctx, financeBaseURL))
	attendanceClient := client.NewAttendanceClient(attendanceBaseURL, gcpauth.HTTPClient(ctx, attendanceBaseURL))
	commsClient := client.NewCommsClient(commsBaseURL, gcpauth.HTTPClient(ctx, commsBaseURL))
	uniformsClient := client.NewUniformsClient(uniformsBaseURL, gcpauth.HTTPClient(ctx, uniformsBaseURL))

	// loginLimiter fronts POST /auth/login with a per-IP token bucket so an
	// attacker cannot brute-force credentials from a single source. It is
	// in-process (fine while the gateway runs at min-instances=1).
	loginRL := config.ParseLoginRateLimit(os.Getenv)
	loginLimiter := middleware.NewRateLimiter(loginRL.Burst, loginRL.RefillPerSec(), time.Now)
	// A SEPARATE bucket for the invitation doors. Same size, different budget:
	// see the comment at the routes for why sharing the login one would let a
	// public endpoint lock the school out of logging in.
	inviteLimiter := middleware.NewRateLimiter(loginRL.Burst, loginRL.RefillPerSec(), time.Now)

	mux := newMux(iamHealth, authClient, studentsClient, configClient, gradesClient, financeClient, attendanceClient, commsClient, uniformsClient, flags, loginLimiter, inviteLimiter)

	log.Info("gateway starting",
		slog.String("addr", addr),
		slog.String("iam", iamBaseURL),
		slog.String("students", studentsBaseURL),
		slog.String("config", configBaseURL),
		slog.String("grades", gradesBaseURL),
		slog.String("finance", financeBaseURL),
		slog.String("attendance", attendanceBaseURL),
		slog.String("comms", commsBaseURL))
	// CORS wraps the whole mux: the browser web client calls the gateway
	// cross-origin with credentials (auth cookies), which requires reflected
	// Allow-Origin + Allow-Credentials. WEB_ORIGIN lists the allowed origins;
	// shared envs set it explicitly (see config.ParseAllowedOrigins).
	webOrigins := config.ParseAllowedOrigins(os.Getenv("WEB_ORIGIN"))
	srv := &http.Server{
		Addr:              addr,
		Handler:           middleware.CORS(webOrigins)(mux),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("gateway server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// newMux builds the gateway HTTP handler. Extracted from main so an integration
// test can assemble the exact production routes against a test iam server.
//
//	POST /api/v1/auth/login    → per-IP rate limit → proxy to iam Login (unauthenticated)
//	POST /api/v1/auth/refresh  → rotate both auth cookies via iam RefreshToken
//	POST /api/v1/auth/logout   → expire both auth cookies (unauthenticated)
//	POST /api/v1/auth/mobile/login   → per-IP rate limit (SAME limiter as web
//	                             login) → iam Login → tokens in the BODY, no
//	                             cookies (unauthenticated)
//	POST /api/v1/auth/mobile/refresh → iam RefreshToken with the refresh token read
//	                             from the BODY → rotated pair in the body, no
//	                             cookies (the refresh token itself authorizes it)
//	GET  /api/v1/me            → RequireAuth → the caller's principal
//	GET  /api/v1/features      → resolved module-flag map (frontend gating)
//	POST /api/v1/students      → ModuleGuard(students) → RequireAuth →
//	                             RequirePermission(students:create) → students service
//	/api/v1/enrollments/...    → ModuleGuard(students) → RequireAuth → rotation →
//	                             RequirePermission(students:enroll on write,
//	                             students:read on list/get) → students service
//	/api/v1/guardians, /guardians/unlink, /students/{studentId}/guardians
//	                           → ModuleGuard(students) → RequireAuth → rotation →
//	                             RequirePermission(guardians:write on link/unlink,
//	                             guardians:read on the list) → students service.
//	                             Admin surface: it decides who may see a child.
//	GET  /api/v1/family/notifications → ModuleGuard(comms) → ModuleGuard(students)
//	                             → RequireAuth → rotation →
//	                             RequirePermission(family:read) → students
//	                             ListStudentsForGuardian(PRINCIPAL) then comms
//	                             ListNotifications per child, newest first. It
//	                             composes two modules, so EITHER flag OFF ⇒ 404.
//	                             Scoped by construction: the caller reads only their
//	                             own children, no request field can widen it.
//	/api/v1/config/...         → ModuleGuard(config) → RequireAuth → rotation →
//	                             RequirePermission(config:write on POST, config:read
//	                             on GET) → config (academic catalog) service
//	/api/v1/grades/...         → ModuleGuard(grades) → RequireAuth → rotation →
//	                             RequirePermission(grades:write on capture,
//	                             grades:read on roster/list) → grades service
//	/api/v1/finance/...        → ModuleGuard(finance) → RequireAuth → rotation →
//	                             RequirePermission(finance:write on concept/charge/
//	                             discount/cancel/payment, finance:read on the
//	                             catalog, charge list, account and overdue reads)
//	                             → finance service
//	/api/v1/attendance/...     → ModuleGuard(attendance) → RequireAuth → rotation →
//	                             RequirePermission(attendance:write on saving a
//	                             day, attendance:read on roster/list) →
//	                             attendance service
//	/api/v1/comms/...          → ModuleGuard(comms) → RequireAuth → rotation →
//	                             RequirePermission(comms:write on device
//	                             registration and marking a notification read,
//	                             comms:read on the notification list) → comms
//	                             service
//	/api/v1/users/..., /roles/... → ModuleGuard(iam) → RequireAuth → rotation →
//	                             RequirePermission(users:*/roles:* by verb) → iam
//	                             admin (user/role CRUD)
//	/api/v1/teachers/..., /teaching-assignments/... → ModuleGuard(teachers) →
//	                             RequireAuth → rotation →
//	                             RequirePermission(teachers:read|write) → config
//	                             (staff roster + assignments)
//	POST /api/v1/teachers/{id}/user → ModuleGuard(teachers) → RequireAuth →
//	                             rotation → RequirePermission(teachers:write) →
//	                             validate iam user (GetUser) then config SetTeacherUser
//	POST /api/v1/auth/google   → per-IP rate limit (the SAME limiter as the
//	                             password login) → iam LoginWithIDToken → OUR
//	                             HttpOnly cookies (unauthenticated)
//	/healthz, /readyz          → liveness + iam reachability
//
// `flags` (via middleware.NewModuleGuard) is the module-visibility boundary:
// disabled module ⇒ 404, evaluated before auth so a hidden module is invisible.
func newMux(iamHealth *client.IAMClient, authClient *client.AuthClient, studentsClient *client.StudentsClient, configClient *client.ConfigClient, gradesClient *client.GradesClient, financeClient *client.FinanceClient, attendanceClient *client.AttendanceClient, commsClient *client.CommsClient, uniformsClient *client.UniformsClient, flags featureflag.Checker, loginLimiter, inviteLimiter *middleware.RateLimiter) *http.ServeMux {
	mux := http.NewServeMux()
	handler.NewHealthHandler(iamHealth).Register(mux)

	// Reuse the same iam client that backs the admin /users and /roles routes to
	// enrich /me with the caller's real display name + role name.
	authH := handler.NewAuthHandler(authClient, handler.WithDirectory(authClient))
	requireAuth := middleware.RequireAuth(authClient)
	// requireAuthChanged is the DEFAULT building block for authenticated routes:
	// it authenticates AND enforces the forced password rotation, so a new route
	// is safe unless it deliberately opts out (only change-password does, below).
	requireAuthChanged := func(h http.Handler) http.Handler {
		return requireAuth(middleware.RequirePasswordChanged(h))
	}
	guard := middleware.NewModuleGuard(flags)

	mux.Handle("POST /api/v1/auth/login", loginLimiter.Middleware(http.HandlerFunc(authH.Login)))
	mux.HandleFunc("POST /api/v1/auth/refresh", authH.Refresh)
	// Phase 9b mobile auth: the same iam Login/RefreshToken, different credential
	// posture (tokens in the body, no cookies — see handler/auth.go). Both are
	// unauthenticated like their web siblings, and MobileLogin shares the SAME
	// per-IP loginLimiter instance, so a brute-force attempt cannot dodge the
	// budget by switching to the mobile door.
	// The identity-provider exchange shares the SAME per-IP limiter as the
	// password login. It is a cheaper call for us — no bcrypt — but it is still
	// a door into a session, and a brute-force budget that a second door can
	// dodge is not a budget.
	// The invitation doors get their OWN limiter instance, never the login one.
	//
	// Sharing it would have been one line and a real hole: behind Cloud Run the
	// limiter keys on the connection's remote address, which is the Google front
	// end for every request (TRUST_PROXY is off until XFF is verified — see
	// deploy/README.md), so that bucket is in practice ONE budget for the whole
	// school. Hanging a public password-reset endpoint off it would let any
	// client spend the budget and lock everyone out of logging in, during a
	// school day.
	invitationH := handler.NewInvitationHandler(authClient)
	mux.Handle("POST /api/v1/auth/invitations/accept", inviteLimiter.Middleware(http.HandlerFunc(invitationH.Accept)))
	mux.Handle("POST /api/v1/auth/password-reset", inviteLimiter.Middleware(http.HandlerFunc(invitationH.RequestReset)))

	mux.Handle("POST /api/v1/auth/google", loginLimiter.Middleware(http.HandlerFunc(authH.GoogleLogin)))
	mux.Handle("POST /api/v1/auth/mobile/login", loginLimiter.Middleware(http.HandlerFunc(authH.MobileLogin)))
	// The provider door for the family app. Same limiter as every other login
	// route: it is cheaper for us (no bcrypt) but it is still a door to a
	// session, and a brute-force budget a second door walks around is not a
	// budget.
	mux.Handle("POST /api/v1/auth/mobile/google", loginLimiter.Middleware(http.HandlerFunc(authH.MobileGoogleLogin)))
	mux.HandleFunc("POST /api/v1/auth/mobile/refresh", authH.MobileRefresh)
	mux.Handle("GET /api/v1/me", requireAuthChanged(http.HandlerFunc(authH.Me)))
	// change-password is the one deliberate exception: it stays reachable while
	// the must-change flag is set, so it authenticates but does NOT enforce.
	mux.Handle("POST /api/v1/auth/change-password", requireAuth(http.HandlerFunc(authH.ChangePassword)))
	mux.HandleFunc("POST /api/v1/auth/logout", authH.Logout)
	mux.HandleFunc("GET /api/v1/features", handler.NewFeaturesHandler(flags).Features)

	// M03 students: flag-guarded, authenticated+rotation-enforced, permission-gated.
	// Create needs students:create; list/get (the enroll UI's student picker) need
	// students:read. Every route sits behind guard.Protect("students", …) so
	// module_students OFF ⇒ 404.
	studentsH := handler.NewStudentsHandler(studentsClient)
	studentsRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("students", requireAuthChanged(middleware.RequirePermission("students:read")(http.HandlerFunc(h))))
	}
	studentsCreate := requireAuthChanged(middleware.RequirePermission("students:create")(
		http.HandlerFunc(studentsH.Create)))
	mux.Handle("POST /api/v1/students", guard.Protect("students", studentsCreate))
	mux.Handle("GET /api/v1/students", studentsRead(studentsH.List))
	mux.Handle("GET /api/v1/students/{id}", studentsRead(studentsH.Get))
	// A pupil's medical record answers to its own permissions (task 5).
	medicalH := handler.NewMedicalHandler(studentsClient)
	mux.Handle("GET /api/v1/students/{id}/medical", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("medical:read")(http.HandlerFunc(medicalH.Get)))))
	mux.Handle("PUT /api/v1/students/{id}/medical", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("medical:write")(http.HandlerFunc(medicalH.Put)))))
	// A pupil's photo (task 5): the bucket is private, so these routes are the
	// only way to the bytes. Reading needs students:read; changing it needs
	// students:update.
	photoH := handler.NewPhotoHandler(studentsClient)
	mux.Handle("GET /api/v1/students/{id}/photo", studentsRead(photoH.Get))
	mux.Handle("PUT /api/v1/students/{id}/photo", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:update")(http.HandlerFunc(photoH.Put)))))
	mux.Handle("DELETE /api/v1/students/{id}/photo", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:update")(http.HandlerFunc(photoH.Delete)))))
	// Notes on a pupil (task 6). Reading them is reading the pupil; writing
	// needs students:notes_write. Voiding needs either permission: the author
	// voids their own within 15 minutes, a holder of students:notes_void any
	// note at any time — the students service applies those rules. The author
	// is the authenticated caller, named from iam's directory.
	notesH := handler.NewNotesHandler(studentsClient, authClient)
	mux.Handle("GET /api/v1/students/{id}/notes", studentsRead(notesH.List))
	mux.Handle("POST /api/v1/students/{id}/notes", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:notes_write")(http.HandlerFunc(notesH.Add)))))
	mux.Handle("POST /api/v1/students/{id}/notes/{noteId}/void", guard.Protect("students", requireAuthChanged(
		middleware.RequireAnyPermission("students:notes_write", "students:notes_void")(http.HandlerFunc(notesH.Void)))))
	// Correcting the file (task 5) needs students:update.
	mux.Handle("PUT /api/v1/students/{id}", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:update")(http.HandlerFunc(studentsH.Update)))))

	// M03 enrollments (Phase 4): place a student into a group of a cycle and
	// transfer/withdraw them. Flag-guarded (module_students), authenticated +
	// rotation-enforced, permission-gated: writes (enroll/transfer/withdraw) need
	// students:enroll, reads (list/get) need students:read. Every route sits
	// behind guard.Protect("students", …) so module_students OFF ⇒ 404.
	enrollH := handler.NewEnrollmentsHandler(studentsClient)
	studentsWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("students", requireAuthChanged(middleware.RequirePermission("students:enroll")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/enrollments", studentsWrite(enrollH.Enroll))
	// The panel's path: a level and a grade, with the group chosen by capacity.
	mux.Handle("POST /api/v1/enrollments/by-grade", studentsWrite(enrollH.EnrollIntoGrade))
	mux.Handle("GET /api/v1/enrollments", studentsRead(enrollH.ListEnrollments))
	mux.Handle("GET /api/v1/enrollments/{id}", studentsRead(enrollH.GetEnrollment))
	mux.Handle("POST /api/v1/enrollments/transfer", studentsWrite(enrollH.Transfer))
	mux.Handle("POST /api/v1/enrollments/withdraw", studentsWrite(enrollH.Withdraw))

	// Phase 9b guardian links: WHO MAY SEE A CHILD, so it is an admin action with
	// its own permission pair (guardians:write / guardians:read) — students:* is
	// deliberately insufficient. Flag-guarded under module_students (the links live
	// in the students service), authenticated + rotation-enforced. The guardian's
	// own self-scoped view is /api/v1/family/* below, not these routes.
	guardiansH := handler.NewGuardiansHandler(studentsClient)
	guardiansWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("students", requireAuthChanged(middleware.RequirePermission("guardians:write")(http.HandlerFunc(h))))
	}
	guardiansRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("students", requireAuthChanged(middleware.RequirePermission("guardians:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/guardians", guardiansWrite(guardiansH.LinkGuardian))
	mux.Handle("POST /api/v1/contacts", guardiansWrite(guardiansH.AddContact))
	mux.Handle("PUT /api/v1/students/{studentId}/contacts/{contactId}", guardiansWrite(guardiansH.UpdateContact))
	mux.Handle("DELETE /api/v1/students/{studentId}/contacts/{contactId}", guardiansWrite(guardiansH.RemoveContact))
	mux.Handle("POST /api/v1/guardians/unlink", guardiansWrite(guardiansH.UnlinkGuardian))
	mux.Handle("GET /api/v1/students/{studentId}/guardians", guardiansRead(guardiansH.ListGuardiansForStudent))

	// Giving a family the app: one action for the school, two services
	// underneath (iam mints the account, students attaches it to the contact the
	// office already typed). It decides WHO MAY SEE A CHILD, so it sits on
	// guardians:write beside the links above — not on users:write, even though it
	// creates an account, because the account is a means and the child is the
	// subject. Before this existed there was no way at all: the anti-escalation
	// guard refuses to assign `guardian` to anybody, which is why the owner now
	// holds family:read (ADR 0012).
	contactAccessH := handler.NewContactAccessHandler(authClient, studentsClient)
	mux.Handle("POST /api/v1/students/{studentId}/guardians/{contactId}/invite",
		guardiansWrite(contactAccessH.Invite))

	// M04 config (academic catalog): flag-guarded, authenticated + rotation-
	// enforced, permission-gated. Writes need config:write, reads config:read;
	// every route sits behind guard.Protect("config", …) so module_config OFF ⇒
	// 404 (the module is invisible before auth even runs).
	configH := handler.NewConfigHandler(configClient)
	configWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("config:write")(http.HandlerFunc(h))))
	}
	configRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("config:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/config/cycles", configWrite(configH.CreateCycle))
	mux.Handle("GET /api/v1/config/cycles", configRead(configH.ListCycles))
	mux.Handle("POST /api/v1/config/cycles/{id}/activate", configWrite(configH.ActivateCycle))
	mux.Handle("POST /api/v1/config/levels", configWrite(configH.CreateLevel))
	mux.Handle("GET /api/v1/config/levels", configRead(configH.ListLevels))
	mux.Handle("POST /api/v1/config/grade-levels", configWrite(configH.CreateGradeLevel))
	mux.Handle("GET /api/v1/config/grade-levels", configRead(configH.ListGradeLevels))
	mux.Handle("POST /api/v1/config/subjects", configWrite(configH.CreateSubject))
	mux.Handle("GET /api/v1/config/subjects", configRead(configH.ListSubjects))
	mux.Handle("POST /api/v1/config/terms", configWrite(configH.CreateTerm))
	mux.Handle("GET /api/v1/config/terms", configRead(configH.ListTerms))
	mux.Handle("POST /api/v1/config/events", configWrite(configH.CreateSchoolEvent))
	mux.Handle("GET /api/v1/config/events", configRead(configH.ListSchoolEvents))
	mux.Handle("GET /api/v1/config/school-profile", configRead(configH.GetSchoolProfile))
	mux.Handle("PUT /api/v1/config/school-profile", configWrite(configH.PutSchoolProfile))
	mux.Handle("DELETE /api/v1/config/events/{id}", configWrite(configH.DeleteSchoolEvent))
	mux.Handle("POST /api/v1/config/calendar", configWrite(configH.CreateNonWorkingPeriod))
	mux.Handle("GET /api/v1/config/calendar", configRead(configH.ListNonWorkingPeriods))
	mux.Handle("DELETE /api/v1/config/calendar/{id}", configWrite(configH.DeleteNonWorkingPeriod))
	mux.Handle("POST /api/v1/config/groups", configWrite(configH.CreateGroup))
	mux.Handle("GET /api/v1/config/groups", configRead(configH.ListGroups))

	// M14 teachers (staff roster + who teaches what): the same config service
	// answers these, but they are a MODULE of their own now, so they carry their
	// own flag and their own permission. A school can hand somebody the staff
	// roster without handing them the cycles, levels, grades and groups too —
	// which is exactly what config:read used to force.
	teachersWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("teachers", requireAuthChanged(middleware.RequirePermission("teachers:write")(http.HandlerFunc(h))))
	}
	teachersRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("teachers", requireAuthChanged(middleware.RequirePermission("teachers:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/teachers", teachersWrite(configH.CreateTeacher))
	mux.Handle("GET /api/v1/teachers", teachersRead(configH.ListTeachers))
	mux.Handle("POST /api/v1/teaching-assignments", teachersWrite(configH.CreateTeachingAssignment))
	mux.Handle("GET /api/v1/teaching-assignments", teachersRead(configH.ListTeachingAssignments))

	// M07 grades (grade capture): flag-guarded, authenticated + rotation-enforced,
	// permission-gated. Capture is a write (grades:write); roster + list are reads
	// (grades:read). Every route sits behind guard.Protect("grades", …) so
	// module_grades OFF ⇒ 404 (the module is invisible before auth even runs).
	gradesH := handler.NewGradesHandler(gradesClient)
	gradesWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("grades", requireAuthChanged(middleware.RequirePermission("grades:write")(http.HandlerFunc(h))))
	}
	gradesRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("grades", requireAuthChanged(middleware.RequirePermission("grades:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/grades", gradesWrite(gradesH.CaptureGrade))
	mux.Handle("GET /api/v1/grades/roster", gradesRead(gradesH.GetRoster))
	mux.Handle("GET /api/v1/grades", gradesRead(gradesH.ListGradeEntries))

	// M07 teaching (teacher self-scoped grades): the maestro-facing surface that
	// exposes ONLY the caller's own groups+subjects, scoped by construction from
	// the principal (the Teacher whose user_id == principal.UserID → that teacher's
	// assignments). teaching:read/teaching:write are self-scoped, so — unlike
	// grades:read/grades:write, which read and write ANY group — they are safe to
	// grant to a non-owner Maestro role. The config client (teacher directory +
	// assignments + group/subject catalog for name enrichment) is reused here, the
	// same one backing /config/*, alongside the grades client (roster + capture)
	// and the students client (roster student-name enrichment — a Maestro holds no
	// students:read, so the trusted gateway resolves the names on its behalf).
	//
	// Assignments composes module_grades + module_config (it reads the config
	// catalog); roster and capture touch only the grades service, so they need only
	// module_grades. A supplied assignmentId is verified against the caller's OWN
	// set before any roster/capture call — an id outside it answers 404, never 403,
	// leaking nothing about whether another teacher's assignment exists.
	teachingH := handler.NewTeachingHandler(configClient, gradesClient, studentsClient)
	teachingModules := func(perm string, modules []string, h http.HandlerFunc) http.Handler {
		wrapped := requireAuthChanged(middleware.RequirePermission(perm)(http.HandlerFunc(h)))
		for _, module := range modules {
			wrapped = guard.Protect(module, wrapped)
		}
		return wrapped
	}
	mux.Handle("GET /api/v1/teaching/assignments",
		teachingModules("teaching:read", []string{"grades", "config"}, teachingH.Assignments))
	// Terms reads the config catalog (terms of the caller's taught cycles), so like
	// assignments it composes module_grades + module_config.
	mux.Handle("GET /api/v1/teaching/terms",
		teachingModules("teaching:read", []string{"grades", "config"}, teachingH.Terms))
	mux.Handle("GET /api/v1/teaching/roster",
		teachingModules("teaching:read", []string{"grades"}, teachingH.Roster))
	mux.Handle("POST /api/v1/teaching/grades",
		teachingModules("teaching:write", []string{"grades"}, teachingH.CaptureGrade))

	// M07 teaching-attendance (teacher self-scoped daily pass list): the maestro
	// takes attendance ONLY for the groups they teach. Attendance is per GROUP, so
	// the scope is the DISTINCT group_ids across the caller's assignments — derived
	// by construction from the principal (via the same config-backed teaching scope
	// handler), so no request field can widen it. attendance_own:read /
	// attendance_own:write are self-scoped, so — unlike attendance:read /
	// attendance:write, which touch ANY group — they are safe to grant to a
	// non-owner Maestro role. The attendance client (roster + save, the same one
	// backing /attendance/*) and the students client (roster student-name
	// enrichment — a Maestro holds no students:read) are reused via the scope
	// handler.
	//
	// Groups composes module_attendance + module_config (it enriches group names);
	// roster and save touch only the attendance service, so they need only
	// module_attendance. A supplied groupId is verified against the caller's OWN set
	// before any roster/save call — a group outside it answers 404, never 403,
	// leaking nothing about whether another group exists.
	teachingAttH := handler.NewTeachingAttendanceHandler(teachingH, attendanceClient)
	mux.Handle("GET /api/v1/teaching/groups",
		teachingModules("attendance_own:read", []string{"attendance", "config"}, teachingAttH.Groups))
	mux.Handle("GET /api/v1/teaching/attendance/roster",
		teachingModules("attendance_own:read", []string{"attendance"}, teachingAttH.Roster))
	mux.Handle("POST /api/v1/teaching/attendance",
		teachingModules("attendance_own:write", []string{"attendance"}, teachingAttH.SaveAttendance))

	// M05 finance (adeudos): flag-guarded, authenticated + rotation-enforced,
	// permission-gated. Everything that moves money or the catalog is a write
	// (finance:write): concept create/update, charge create/generate, discount,
	// cancel, payment. The catalog, charge list, account statement and overdue
	// report are reads (finance:read). Every route sits behind
	// guard.Protect("finance", …) so module_finance OFF ⇒ 404 (the module is
	// invisible before auth even runs). The more specific patterns
	// (/charges/generate, /charges/{id}/discount, /charges/{id}/cancel) win over
	// /charges under ServeMux's specificity rule.
	// Who takes the money is named from iam's directory (task 7).
	financeH := handler.NewFinanceHandler(financeClient, handler.WithNames(authClient))
	financeWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequirePermission("finance:write")(http.HandlerFunc(h))))
	}
	financeRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequirePermission("finance:read")(http.HandlerFunc(h))))
	}
	// Task 9 (ADR 0015): the counter answers to finance:read OR collections:read
	// — balances, what is due, what is late, the day's cut — and taking money to
	// finance:write OR payments:write. The Cobranza role holds only the second of
	// each; everything that prices, bills, discounts, cancels, voids or reports
	// income stays finance:* (owner).
	collectionsRead := func(h http.Handler) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequireAnyPermission("finance:read", "collections:read")(h)))
	}
	paymentsWrite := func(h http.Handler) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequireAnyPermission("finance:write", "payments:write")(h)))
	}
	// A family's charges and deposits are the counter's one pupil at a time;
	// the whole ledger (a cycle, every deposit) stays finance:read's.
	onePupil := middleware.RequireStudentScopeUnless("finance:read")
	// M15 tuition: pricing a level and running the month are WRITES on the
	// ledger, so they answer to finance:write; reading the table is finance:read.
	// The screen lives in Configuración because that is where a director looks
	// for it, but the money stays in the finance module and behind its flag.
	mux.Handle("PUT /api/v1/finance/tuition-plans", financeWrite(financeH.UpsertTuitionPlan))
	mux.Handle("GET /api/v1/finance/tuition-plans", financeRead(financeH.ListTuitionPlans))
	mux.Handle("POST /api/v1/finance/billing-runs", financeWrite(financeH.RunBilling))
	mux.Handle("POST /api/v1/finance/concepts", financeWrite(financeH.CreateConcept))
	mux.Handle("PUT /api/v1/finance/concepts/{id}", financeWrite(financeH.UpdateConcept))
	mux.Handle("GET /api/v1/finance/concepts", financeRead(financeH.ListConcepts))
	mux.Handle("POST /api/v1/finance/charges", financeWrite(financeH.CreateCharge))
	mux.Handle("POST /api/v1/finance/charges/generate", financeWrite(financeH.GenerateCharges))
	mux.Handle("POST /api/v1/finance/charges/{id}/discount", financeWrite(financeH.ApplyDiscount))
	mux.Handle("POST /api/v1/finance/charges/{id}/cancel", financeWrite(financeH.CancelCharge))
	// One pupil's charges are the counter's; a whole cycle's ledger is finance.
	mux.Handle("GET /api/v1/finance/charges", collectionsRead(onePupil(http.HandlerFunc(financeH.ListCharges))))
	// Paying one charge by id takes its stored amount, skipping the daily late
	// fee and the inscription climb a deposit prices in: finance:write only. The
	// counter (payments:write) collects through deposits.
	mux.Handle("POST /api/v1/finance/payments", financeWrite(financeH.RegisterPayment))
	mux.Handle("POST /api/v1/finance/deposits", paymentsWrite(http.HandlerFunc(financeH.RegisterDeposit)))
	mux.Handle("POST /api/v1/finance/deposits/preview", collectionsRead(http.HandlerFunc(financeH.PreviewDeposit)))
	mux.Handle("GET /api/v1/finance/deposits", collectionsRead(onePupil(http.HandlerFunc(financeH.ListDeposits))))
	mux.Handle("GET /api/v1/finance/expected", collectionsRead(http.HandlerFunc(financeH.ExpectedToday)))
	mux.Handle("GET /api/v1/finance/cash-up", collectionsRead(http.HandlerFunc(financeH.DailyCashUp)))
	mux.Handle("GET /api/v1/finance/income", financeRead(financeH.IncomeByConcept))
	mux.Handle("POST /api/v1/finance/deposits/{id}/void", financeWrite(financeH.VoidDeposit))
	mux.Handle("GET /api/v1/finance/accounts/{studentId}", collectionsRead(http.HandlerFunc(financeH.GetAccount)))
	// The overdue list is school-wide on purpose: it is the counter's own report
	// (who to call), which the owner approved for this role (ADR 0015).
	mux.Handle("GET /api/v1/finance/overdue", collectionsRead(http.HandlerFunc(financeH.ListOverdue)))

	// M06 attendance (daily pass list): flag-guarded, authenticated + rotation-
	// enforced, permission-gated. Saving a group·day is the only write
	// (attendance:write) — it reconciles the whole day, so it also erases marks;
	// the roster and the per-enrollment history are reads (attendance:read).
	// Every route sits behind guard.Protect("attendance", …) so
	// module_attendance OFF ⇒ 404 (the module is invisible before auth even
	// runs). The more specific /attendance/roster wins over /attendance under
	// ServeMux's specificity rule.
	attendanceH := handler.NewAttendanceHandler(attendanceClient)
	attendanceWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("attendance", requireAuthChanged(middleware.RequirePermission("attendance:write")(http.HandlerFunc(h))))
	}
	attendanceRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("attendance", requireAuthChanged(middleware.RequirePermission("attendance:read")(http.HandlerFunc(h))))
	}
	mux.Handle("GET /api/v1/attendance/roster", attendanceRead(attendanceH.GetRoster))
	mux.Handle("POST /api/v1/attendance", attendanceWrite(attendanceH.SaveDaily))
	mux.Handle("GET /api/v1/attendance", attendanceRead(attendanceH.ListAttendance))

	// M10 comms (push + notification inbox): flag-guarded, authenticated +
	// rotation-enforced, permission-gated. Registering a device and marking a
	// notification read both mutate state (comms:write); listing a student's
	// notifications is a read (comms:read). Every route sits behind
	// guard.Protect("comms", …) so module_comms OFF ⇒ 404 (the module is
	// invisible before auth even runs).
	//
	// SCOPING CAVEAT: comms:read takes a studentId from the query, so it can
	// expose ANY student's notifications to any holder of the permission — it stays
	// owner-only. The guardian-facing read is /api/v1/family/notifications below,
	// which is scoped by construction and needs no such caveat.
	commsH := handler.NewCommsHandler(commsClient)
	commsWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("comms", requireAuthChanged(middleware.RequirePermission("comms:write")(http.HandlerFunc(h))))
	}
	commsRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("comms", requireAuthChanged(middleware.RequirePermission("comms:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/comms/devices", commsWrite(commsH.RegisterDevice))
	mux.Handle("GET /api/v1/comms/notifications", commsRead(commsH.ListNotifications))
	mux.Handle("POST /api/v1/comms/notifications/{id}/read", commsWrite(commsH.MarkRead))
	// An announcement is a comms write, but the audience is resolved here at the
	// edge: config and students already answer "who is in 1° A", and comms
	// deliberately does not know what a group is.
	announcementsH := handler.NewAnnouncementHandler(commsClient, studentsClient, configClient)
	mux.Handle("POST /api/v1/comms/announcements", commsWrite(announcementsH.Announce))

	// M12 uniforms (the school's uniform shop). Stage one: the catalogue, the
	// storeroom and the counter sale. Every route sits behind
	// guard.Protect("uniforms", ...) so module_uniforms OFF => 404 -- the module
	// is invisible before auth even runs, not merely refused.
	//
	// THE READ PERMISSION IS SPLIT, and that split is the point:
	//
	//   uniforms:stock   the catalogue, stock counts, receipts, adjustments.
	//                    Resolves NO student data at all -- garments, sizes and
	//                    numbers. Grantable without reservation, which is what
	//                    makes a "Caja/Almacen" role possible.
	//   uniforms:orders  the order list, which carries every pupil's name and
	//                    purchase history for the whole school with no
	//                    per-object scoping. That is the literal criterion in
	//                    iam's ownerOnlyPermissions, so it is OWNER-ONLY until
	//                    per-object scoping exists for it.
	//
	// A single uniforms:read would have forced a choice between a storeroom role
	// that cannot work and handing the school roll to whoever holds it. The
	// storeroom does not need names.
	//
	// The counter sale needs uniforms:write AND finance:write, because it moves
	// goods and takes money in one act. RequireAllPermissions names the missing
	// one so the panel can disable the entry point and SAY WHY -- otherwise a
	// cashier keys in six lines with a parent waiting and eats a bare 403.
	uniformsH := handler.NewUniformsHandler(uniformsClient)
	uniformsStock := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("uniforms", requireAuthChanged(middleware.RequirePermission("uniforms:stock")(http.HandlerFunc(h))))
	}
	uniformsWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("uniforms", requireAuthChanged(middleware.RequirePermission("uniforms:write")(http.HandlerFunc(h))))
	}
	uniformsOrders := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("uniforms", requireAuthChanged(middleware.RequirePermission("uniforms:orders")(http.HandlerFunc(h))))
	}
	uniformsSell := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("uniforms", requireAuthChanged(
			middleware.RequireAllPermissions("uniforms:write", "finance:write")(http.HandlerFunc(h))))
	}
	// The school's levels, for the inventory's level chips: any uniforms reader
	// may see them, without config:read (ADR 0014).
	uniformsRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("uniforms", requireAuthChanged(
			middleware.RequireAnyPermission("uniforms:stock", "uniforms:orders")(http.HandlerFunc(h))))
	}
	mux.Handle("GET /api/v1/uniforms/levels", uniformsRead(handler.NewUniformLevelsHandler(configClient).List))
	mux.Handle("GET /api/v1/uniforms/items", uniformsStock(uniformsH.ListItems))
	mux.Handle("POST /api/v1/uniforms/items", uniformsWrite(uniformsH.CreateItem))
	mux.Handle("PUT /api/v1/uniforms/items/{id}", uniformsWrite(uniformsH.UpdateItem))
	mux.Handle("POST /api/v1/uniforms/items/{id}/sizes", uniformsWrite(uniformsH.UpsertSizes))
	mux.Handle("GET /api/v1/uniforms/stock", uniformsStock(uniformsH.ListStock))
	mux.Handle("POST /api/v1/uniforms/stock/receive", uniformsWrite(uniformsH.ReceiveStock))
	mux.Handle("POST /api/v1/uniforms/stock/adjust", uniformsWrite(uniformsH.AdjustStock))
	mux.Handle("POST /api/v1/uniforms/counter-sale", uniformsSell(uniformsH.CounterSale))
	mux.Handle("GET /api/v1/uniforms/orders", uniformsOrders(uniformsH.ListOrders))
	mux.Handle("GET /api/v1/uniforms/orders/{id}", uniformsOrders(uniformsH.GetOrder))
	mux.Handle("GET /api/v1/uniforms/settings", uniformsStock(uniformsH.GetSettings))
	mux.Handle("PUT /api/v1/uniforms/settings", uniformsWrite(uniformsH.UpdateSettings))

	// Phase 9b family surface: the guardian's own children's news, account,
	// profile, grades and attendance, composed at the edge from the students
	// guardian links + each owning module. Every route needs family:read.
	//
	// A composite route READS FROM SEVERAL MODULES, so it is guarded on each one it
	// touches (the guards nest): any one disabled ⇒ 404. Guarding only some would
	// leave a route that still calls a disabled module's service, so disabling that
	// module would break the route instead of hiding it — a disabled module must be
	// invisible and removable without taking others down with it. Every by-child
	// route also composes module_students, because the ownership check itself reads
	// the guardian links from the students service.
	//
	// The child set is derived from the PRINCIPAL, and every {studentId} in a path
	// is checked against it (assertLinkedChild → 404 for a non-linked id), so the
	// caller can only ever read their own children — no request field can widen the
	// scope. That is why these routes, unlike the admin /comms and /finance ones,
	// are safe to grant to a parent.
	// configClient (the same one backing the /config/* routes) is reused here only
	// to resolve grade subject/term ids to names for the family grades screen.
	familyH := handler.NewFamilyHandler(studentsClient, commsClient, financeClient, gradesClient, attendanceClient, configClient)
	// familyReadModules composes family:read behind the given module guards, in
	// addition to module_students which every family route needs for the ownership
	// check. The guards nest right-to-left, so the outermost listed module is
	// evaluated first; order does not affect the result (all must be ON).
	familyReadModules := func(modules []string, h http.HandlerFunc) http.Handler {
		wrapped := requireAuthChanged(middleware.RequirePermission("family:read")(http.HandlerFunc(h)))
		for _, module := range modules {
			wrapped = guard.Protect(module, wrapped)
		}
		return wrapped
	}
	// /family/notifications and the guardian mark-read compose comms (the inbox and
	// the acknowledgement) with students (the guardian links).
	familyComms := func(h http.HandlerFunc) http.Handler {
		return familyReadModules([]string{"comms", "students"}, h)
	}
	mux.Handle("GET /api/v1/family/notifications", familyComms(familyH.Notifications))
	mux.Handle("POST /api/v1/family/notifications/{notificationId}/read", familyComms(familyH.MarkNotificationRead))
	// The students list is scoped by the principal alone (no {studentId}), so it
	// needs only module_students — the app calls it first to discover its children.
	mux.Handle("GET /api/v1/family/students",
		familyReadModules([]string{"students"}, familyH.ListStudents))
	// The by-child reads each compose module_students (ownership) with the module
	// that owns the data.
	mux.Handle("GET /api/v1/family/students/{studentId}/account",
		familyReadModules([]string{"finance", "students"}, familyH.Account))
	mux.Handle("GET /api/v1/family/students/{studentId}/profile",
		familyReadModules([]string{"students"}, familyH.Profile))
	mux.Handle("GET /api/v1/family/students/{studentId}/grades",
		familyReadModules([]string{"grades", "students"}, familyH.Grades))
	mux.Handle("GET /api/v1/family/students/{studentId}/attendance",
		familyReadModules([]string{"attendance", "students"}, familyH.Attendance))

	// Phase 6 iam admin (user/role CRUD): flag-guarded (module_iam), authenticated
	// + rotation-enforced, permission-gated. User routes need users:read (GET) /
	// users:write (POST); role routes need roles:read / roles:write. Every route
	// sits behind guard.Protect("iam", …) so module_iam OFF ⇒ 404 (the admin
	// surface is invisible before auth even runs). The AuthClient carries both the
	// auth and the admin surface; the ConfigClient is the teacher linker.
	adminH := handler.NewAdminHandler(authClient, configClient)
	iamAdmin := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("iam", requireAuthChanged(middleware.RequirePermission(perm)(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/users", iamAdmin("users:write", adminH.CreateUser))
	// Invite is the preferred way to add somebody: no password is invented for
	// them. It is users:write like create, and the caller-subset guard inside the
	// handler is what stops an admin inviting an owner-powered account to an
	// address it controls.
	mux.Handle("POST /api/v1/users/invite", iamAdmin("users:write", adminH.InviteUser))
	mux.Handle("POST /api/v1/users/{id}/invite", iamAdmin("users:write", adminH.ResendInvitation))
	mux.Handle("GET /api/v1/users", iamAdmin("users:read", adminH.ListUsers))
	mux.Handle("GET /api/v1/users/{id}", iamAdmin("users:read", adminH.GetUser))
	mux.Handle("POST /api/v1/users/{id}/status", iamAdmin("users:write", adminH.SetUserStatus))
	mux.Handle("POST /api/v1/users/{id}/roles", iamAdmin("users:write", adminH.SetUserRoles))
	mux.Handle("POST /api/v1/roles", iamAdmin("roles:write", adminH.CreateRole))
	mux.Handle("GET /api/v1/roles", iamAdmin("roles:read", adminH.ListRoles))
	mux.Handle("POST /api/v1/roles/{id}/permissions", iamAdmin("roles:write", adminH.SetRolePermissions))

	// The teacher↔user link mutates a CONFIG teacher, so it sits under
	// module_config + config:write (reusing configWrite), not module_iam. The
	// handler itself performs the gateway-orchestrated cross-service validation
	// (iam GetUser) before the config write.
	mux.Handle("POST /api/v1/teachers/{id}/user", teachersWrite(adminH.LinkTeacherUser))
	return mux
}
