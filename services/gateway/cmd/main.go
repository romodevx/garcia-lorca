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
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/config"
	"github.com/garcia-lorca/services/gateway/internal/expensereport"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
	"github.com/garcia-lorca/services/gateway/internal/receipt"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/logger"
)

const (
	defaultAddr       = ":8080"
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	// studentImportWriteBudget is the import commit route's own write deadline
	// (set per request; the server-wide writeTimeout is too short to re-plan and
	// write 1,000 pupils, one transaction each). The detached commit ends a few
	// seconds before it, so the office always gets the result it started. When
	// the commit also builds the batch's families (module_families), the writes
	// stop handler.familiesBudget (20 s) earlier and the build gets that slice;
	// with under familiesMinBudget (5 s) left it is skipped (built:false).
	studentImportWriteBudget = 2 * time.Minute
)

const defaultSchoolTimezone = "America/Mexico_City"

func schoolLocation() (*time.Location, error) {
	name := os.Getenv("SCHOOL_TIMEZONE")
	if name == "" {
		name = defaultSchoolTimezone
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("SCHOOL_TIMEZONE %q: %w", name, err)
	}
	return location, nil
}

func main() {
	log := logger.Default()
	school, err := schoolLocation()
	if err != nil {
		log.Error("gateway timezone configuration is invalid", slog.Any("error", err))
		os.Exit(1)
	}

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
	// Re-authentication (deleting a level, ADR 0021) checks a password, so it is
	// throttled like a login — on its OWN bucket of the same size: it sits
	// behind an owner session, and sharing the login budget would let a
	// director's typos lock the school out of signing in.
	reauthLimiter := middleware.NewRateLimiter(loginRL.Burst, loginRL.RefillPerSec(), time.Now)
	deletionKey := config.ParseDeletionTokenKey(os.Getenv, log)

	mux := newMux(iamHealth, authClient, studentsClient, configClient, gradesClient, financeClient, attendanceClient, commsClient, uniformsClient, flags, loginLimiter, inviteLimiter, reauthLimiter, deletionKey, school)

	// ADR 0021: the level trash's retention sweep lives HERE, in the one
	// always-warm service (min-instances=1) — the data services scale to zero
	// and cannot be trusted to wake up on the hour. Hourly, on the process's
	// own context (the school is this deployment's tenant), logged each run.
	trashSweeper := handler.NewLevelDeletionHandler(handler.LevelDeletionDeps{
		Config: configClient, Students: studentsClient,
		Grades: gradesClient, Attendance: attendanceClient, Finance: financeClient, Uniforms: uniformsClient,
		Log: log,
	})
	levelarchive.StartPurger(context.Background(), trashSweeper.Sweep, log)

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
//	POST /api/v1/family/devices, DELETE /api/v1/family/devices/{token}
//	                           → same guards as /family/notifications; the device
//	                             owner is the PRINCIPAL (register = upsert that
//	                             moves the token to the caller; delete = own only).
//	GET  /api/v1/family/students/{studentId}/attendance?month=YYYY-MM
//	                           → ModuleGuard(attendance) → ModuleGuard(students) →
//	                             family:read → ownership (404) → one row per day.
//	/api/v1/config/...         → ModuleGuard(config) → RequireAuth → rotation →
//	                             RequirePermission(config:write on POST, config:read
//	                             on GET) → config (academic catalog) service
//	GET /api/v1/grades/period-sheet, PUT/DELETE /api/v1/grades/period-grades
//	                             (T15) → ModuleGuard(grades) → RequireAuth →
//	                             rotation → grades:read|grades:write
//	GET  /api/v1/teaching/grades/period-sheet, PUT/DELETE
//	     /api/v1/teaching/grades/period-grades
//	                           → ModuleGuard(grades+config) → teaching:read|write →
//	                             ownership of the assignment (404)
//	GET  /api/v1/family/students/{studentId}/grades/periods → grades+students →
//	                             family:read → ownership (404)
//	(The T6 monthly routes were retired in T15: 404.)
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
//	POST /api/v1/teachers/{id}/access → ModuleGuard(teachers) → ModuleGuard(iam) → RequireAuth →
//	                             rotation → RequireAllPermissions(teachers:write,
//	                             users:write) → subset guard → iam + config
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
func newMux(iamHealth *client.IAMClient, authClient *client.AuthClient, studentsClient *client.StudentsClient, configClient *client.ConfigClient, gradesClient *client.GradesClient, financeClient *client.FinanceClient, attendanceClient *client.AttendanceClient, commsClient *client.CommsClient, uniformsClient *client.UniformsClient, flags featureflag.Checker, loginLimiter, inviteLimiter, reauthLimiter *middleware.RateLimiter, deletionKey []byte, schoolLocations ...*time.Location) *http.ServeMux {
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
	// module_import_review (PLAN-import-avisos) changes what some handlers
	// read and answer, not only which routes exist: the import's row
	// decisions and the students JSON's openIssues/draft.
	importReviewOn := func(ctx context.Context) bool { return flags.Enabled(ctx, featureflag.Key("import_review")) }
	// module_families (ADR 0041) gates familyId on the students JSON and its
	// filter, besides the /api/v1/families/* routes.
	familiesOn := func(ctx context.Context) bool { return flags.Enabled(ctx, featureflag.Key("families")) }

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
	// It also checks the current password, so it is throttled like a
	// re-authentication: on the reauth bucket, keyed by the signed-in user.
	mux.Handle("POST /api/v1/auth/change-password", requireAuth(reauthLimiter.PerPrincipalMiddleware(http.HandlerFunc(authH.ChangePassword))))
	mux.HandleFunc("POST /api/v1/auth/logout", authH.Logout)
	mux.HandleFunc("GET /api/v1/features", handler.NewFeaturesHandler(flags).Features)

	// M03 students: flag-guarded, authenticated+rotation-enforced, permission-gated.
	// Create needs students:create; list/get (the enroll UI's student picker) need
	// students:read. Every route sits behind guard.Protect("students", …) so
	// module_students OFF ⇒ 404.
	// The list names each classroom with its grade, so Cobranza (students:read,
	// no config:read) reads "3° PRIMARIA · B" without being handed the catalog
	// (CC-211, CC-313).
	studentsH := handler.NewStudentsHandler(studentsClient,
		handler.WithStudentRoomLabels(handler.NewCycleRoomLabels(configClient, studentsClient)),
		handler.WithImportReview(importReviewOn),
		handler.WithFamilies(familiesOn, studentsClient))
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
	// note at any time — the students service applies those rules. Editing
	// (T6) is the same pair: the author edits their own at any time, a holder of
	// students:notes_void any note; every edit keeps the replaced text. The
	// author and editor are the authenticated caller, named from iam's directory.
	notesH := handler.NewNotesHandler(studentsClient, authClient)
	mux.Handle("GET /api/v1/students/{id}/notes", studentsRead(notesH.List))
	mux.Handle("POST /api/v1/students/{id}/notes", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:notes_write")(http.HandlerFunc(notesH.Add)))))
	mux.Handle("POST /api/v1/students/{id}/notes/{noteId}/void", guard.Protect("students", requireAuthChanged(
		middleware.RequireAnyPermission("students:notes_write", "students:notes_void")(http.HandlerFunc(notesH.Void)))))
	mux.Handle("PATCH /api/v1/students/{id}/notes/{noteId}", guard.Protect("students", requireAuthChanged(
		middleware.RequireAnyPermission("students:notes_write", "students:notes_void")(http.HandlerFunc(notesH.Edit)))))
	// Correcting the file (task 5) needs students:update.
	mux.Handle("PUT /api/v1/students/{id}", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:update")(http.HandlerFunc(studentsH.Update)))))
	// PLAN-import-avisos: a pupil's import issues live under module_students
	// AND module_import_review. Reading them is reading the pupil
	// (students:read); «Está bien así» changes the file's record, so it is
	// students:update, like the correction itself.
	importIssuesH := handler.NewImportIssuesHandler(studentsClient)
	importReview := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("students", guard.Protect("import_review", requireAuthChanged(
			middleware.RequirePermission(perm)(h))))
	}
	mux.Handle("GET /api/v1/students/{id}/import-issues", importReview("students:read", importIssuesH.List))
	mux.Handle("POST /api/v1/students/{id}/import-issues/{issueId}/accept", importReview("students:update", importIssuesH.Accept))

	// M03 enrollments (Phase 4): place a student into a group of a cycle and
	// transfer/withdraw them. Flag-guarded (module_students), authenticated +
	// rotation-enforced, permission-gated: writes (enroll/transfer/withdraw) need
	// students:enroll, reads (list/get) need students:read. Every route sits
	// behind guard.Protect("students", …) so module_students OFF ⇒ 404.
	enrollmentOptions := []handler.EnrollmentsOption{handler.WithWithdrawalPreview(financeClient)}
	if len(schoolLocations) > 0 {
		enrollmentOptions = append(enrollmentOptions, handler.WithSchoolLocation(schoolLocations[0]))
	}
	enrollH := handler.NewEnrollmentsHandler(studentsClient, enrollmentOptions...)
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
	mux.Handle("GET /api/v1/enrollments/{id}/withdrawal-preview", guard.Protect("students", requireAuthChanged(
		middleware.RequirePermission("students:enroll")(middleware.RequireAnyPermission("finance:read", "collections:read")(http.HandlerFunc(enrollH.PreviewWithdrawal))))))

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
	// subject. It is the ONLY road to the `guardian` role: iam refuses it on
	// every manual road (invite, create, assign; ISSUE-CC-115) and its
	// InviteGuardian picks the role itself, adding it to an account that already
	// exists at that address. The owner holds family:read so the anti-escalation
	// guard lets it delegate the role at all (ADR 0012).
	contactAccessH := handler.NewContactAccessHandler(authClient, studentsClient)
	mux.Handle("POST /api/v1/students/{studentId}/guardians/{contactId}/invite",
		guardiansWrite(contactAccessH.Invite))

	// Families of siblings (ADR 0041): module_students AND module_families, so
	// either one off ⇒ 404 before auth. Reading is reading the pupils
	// (students:read) and the family's adults are guardians:read. Renaming and
	// dismissing a suggestion are students:update. Building the family — create,
	// add, remove, dissolve — is guardians:write, owner-only like the adults'
	// access: the account sync hands every account of a family to each pupil
	// that enters it, so joining a child to the wrong family would open that
	// child to strangers (security gate H1). /suggestions is a literal segment,
	// so the mux prefers it over /{familyId}.
	familiesH := handler.NewFamiliesHandler(studentsClient)
	families := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("students", guard.Protect("families", requireAuthChanged(
			middleware.RequirePermission(perm)(h))))
	}
	mux.Handle("GET /api/v1/families", families("students:read", familiesH.List))
	mux.Handle("GET /api/v1/families/suggestions", families("students:read", familiesH.Suggestions))
	mux.Handle("GET /api/v1/families/{familyId}", families("students:read", familiesH.Get))
	mux.Handle("GET /api/v1/families/{familyId}/contacts", families("guardians:read", familiesH.Contacts))
	mux.Handle("POST /api/v1/families/suggestions/dismiss", families("students:update", familiesH.DismissSuggestion))
	mux.Handle("PATCH /api/v1/families/{familyId}", families("students:update", familiesH.Rename))
	mux.Handle("POST /api/v1/families", families("guardians:write", familiesH.Create))
	mux.Handle("POST /api/v1/families/{familyId}/students", families("guardians:write", familiesH.AddStudents))
	mux.Handle("DELETE /api/v1/families/{familyId}/students/{studentId}", families("guardians:write", familiesH.RemoveStudent))
	mux.Handle("DELETE /api/v1/families/{familyId}", families("guardians:write", familiesH.Dissolve))
	mux.Handle("POST /api/v1/families/{familyId}/contacts", families("guardians:write", familiesH.AddContact))
	mux.Handle("POST /api/v1/families/{familyId}/access/revoke", families("guardians:write", familiesH.RevokeAccess))

	// M04 config (academic catalog): flag-guarded, authenticated + rotation-
	// enforced, permission-gated. Writes need config:write, reads config:read;
	// every route sits behind guard.Protect("config", …) so module_config OFF ⇒
	// 404 (the module is invisible before auth even runs).
	configH := handler.NewConfigHandler(configClient).WithScaleGuard(gradesClient).WithTermLock(gradesClient).
		WithPeriodGradesGuard(gradesClient)
	configWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("config:write")(http.HandlerFunc(h))))
	}
	configRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("config:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/config/cycles", configWrite(configH.CreateCycle))
	mux.Handle("GET /api/v1/config/cycles", configRead(configH.ListCycles))
	mux.Handle("POST /api/v1/config/cycles/{id}/activate", configWrite(configH.ActivateCycle))
	mux.Handle("PATCH /api/v1/config/cycles/{id}", configWrite(configH.RenameCycle))
	mux.Handle("POST /api/v1/config/levels", configWrite(configH.CreateLevel))
	mux.Handle("GET /api/v1/config/levels", configRead(configH.ListLevels))
	mux.Handle("PATCH /api/v1/config/levels/{id}", configWrite(configH.PatchLevel))
	mux.Handle("PUT /api/v1/config/levels/order", configWrite(configH.ReorderLevels))
	mux.Handle("POST /api/v1/config/grade-levels", configWrite(configH.CreateGradeLevel))
	mux.Handle("GET /api/v1/config/grade-levels", configRead(configH.ListGradeLevels))
	// T8 (ADR 0036): a grade captured wrong is fixed without deleting its level.
	mux.Handle("PATCH /api/v1/config/grade-levels/{id}", configWrite(configH.RenameGradeLevel))
	mux.Handle("DELETE /api/v1/config/grade-levels/{id}", configWrite(configH.DeleteGradeLevel))
	mux.Handle("POST /api/v1/config/subjects", configWrite(configH.CreateSubject))
	mux.Handle("GET /api/v1/config/subjects", configRead(configH.ListSubjects))
	mux.Handle("PATCH /api/v1/config/subjects/{id}", configWrite(configH.RenameSubject))
	mux.Handle("DELETE /api/v1/config/subjects/{id}", configWrite(configH.DeleteSubject))
	mux.Handle("POST /api/v1/config/terms", configWrite(configH.CreateTerm))
	mux.Handle("GET /api/v1/config/terms", configRead(configH.ListTerms))
	mux.Handle("PUT /api/v1/config/terms/{id}/months", configWrite(configH.PutTermMonths))
	mux.Handle("POST /api/v1/config/terms/copy-months", configWrite(configH.CopyTermMonths))
	// T14: evaluation periods per grade (reads for whoever reads the catalog).
	mux.Handle("GET /api/v1/config/evaluation-periods", configRead(configH.ListEvaluationPeriods))
	mux.Handle("PUT /api/v1/config/grade-levels/{id}/evaluation-periods", configWrite(configH.PutGradeEvaluationPeriods))
	mux.Handle("POST /api/v1/config/evaluation-periods/copy", configWrite(configH.CopyEvaluationPeriods))
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
	// T8 (ADR 0036): renaming relabels the pupils' files (students); deleting
	// asks config, students, attendance and grades what they keep of the group
	// — whatever their module flags, since a disabled module's rows still name
	// it — and refuses naming every blocker.
	groupEditH := handler.NewGroupEditHandler(handler.GroupEditDeps{
		Config: configClient, Students: studentsClient, Attendance: attendanceClient, Grades: gradesClient,
	})
	mux.Handle("PATCH /api/v1/config/groups/{id}", configWrite(groupEditH.RenameGroup))
	mux.Handle("DELETE /api/v1/config/groups/{id}", configWrite(groupEditH.DeleteGroup))

	// ADR 0021 deleting a level: the owner's alone (levels:delete, owner-only in
	// iam), under module_config like the catalog it edits. The delete itself
	// re-checks the password, so it runs on the re-auth limiter. The saga
	// reaches every data service regardless of their module flags: a disabled
	// module's rows still belong to the level being deleted.
	levelDeletionH := handler.NewLevelDeletionHandler(handler.LevelDeletionDeps{
		Config: configClient, Students: studentsClient,
		Grades: gradesClient, Attendance: attendanceClient, Finance: financeClient, Uniforms: uniformsClient,
		Passwords: authClient, Users: authClient,
		Tokens: handler.NewDeletionTokens(deletionKey, time.Now),
	})
	levelsDelete := func(h http.Handler) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("levels:delete")(h)))
	}
	mux.Handle("GET /api/v1/config/levels/{id}/deletion-preview", levelsDelete(http.HandlerFunc(levelDeletionH.Preview)))
	mux.Handle("POST /api/v1/config/levels/{id}/delete", levelsDelete(reauthLimiter.Middleware(http.HandlerFunc(levelDeletionH.Delete))))
	mux.Handle("GET /api/v1/config/level-trash", levelsDelete(http.HandlerFunc(levelDeletionH.Trash)))
	mux.Handle("POST /api/v1/config/level-trash/{batchId}/restore", levelsDelete(http.HandlerFunc(levelDeletionH.Restore)))

	// ADR 0033 the legacy student import: it writes the catalog AND the roll,
	// so it lives under module_students, module_config and its own
	// module_student_import, and answers only to students:import (owner-only in
	// iam: it composes config:write + students:create + students:enroll).
	studentImportH := handler.NewStudentImportHandler(handler.StudentImportDeps{
		Catalog: configClient, Roster: studentsClient, Pricing: financeClient,
		NewID: handler.NewBatchID, WriteBudget: studentImportWriteBudget,
		ReviewEnabled: importReviewOn,
		// PLAN-familias-importacion: with module_families on, a commit by a
		// guardians:write holder builds the batch's families (best effort).
		Families: studentsClient, FamiliesEnabled: familiesOn,
	})
	studentImport := func(h http.Handler) http.Handler {
		return guard.Protect("students", guard.Protect("config", guard.Protect("student_import",
			requireAuthChanged(middleware.RequirePermission("students:import")(h)))))
	}
	mux.Handle("POST /api/v1/imports/students/preview", studentImport(http.HandlerFunc(studentImportH.Preview)))
	mux.Handle("POST /api/v1/imports/students/commit", studentImport(http.HandlerFunc(studentImportH.Commit)))
	// Undoing an import's families belongs to the import AND is a family
	// membership write (ADR 0041): the import's modules plus module_families
	// (404 before auth), and both students:import and guardians:write (it
	// takes pupils out of families, which is handing out access).
	// It has its own small per-principal budget (security C1): each call
	// rewrites families, and per IP would be one budget for the whole school
	// behind Cloud Run.
	undoFamiliesLimiter := middleware.NewRateLimiter(importFamiliesUndoBurst, importFamiliesUndoRefillPerSec, time.Now)
	mux.Handle("POST /api/v1/imports/students/{batchId}/families/undo",
		guard.Protect("families", studentImport(middleware.RequirePermission("guardians:write")(
			undoFamiliesLimiter.PerPrincipalMiddleware(http.HandlerFunc(studentImportH.UndoFamilies))))))

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
	// Carga académica: a whole teaching load at once, and freeing or handing a
	// slot to someone else. Same guard as creating one assignment.
	mux.Handle("POST /api/v1/teaching-assignments/bulk", teachersWrite(configH.AssignTeachingLoad))
	mux.Handle("DELETE /api/v1/teaching-assignments/{id}", teachersWrite(configH.DeleteTeachingAssignment))
	mux.Handle("PATCH /api/v1/teaching-assignments/{id}", teachersWrite(configH.ReassignTeachingAssignment))

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
	// The director's term picker for the legacy capture: only the terms of the
	// assignment's cycle AND group level. It reads the config catalog, so it
	// composes module_config too.
	gradesTermsH := handler.NewGradesTermsHandler(configClient)
	mux.Handle("GET /api/v1/grades/terms", guard.Protect("grades", guard.Protect("config",
		requireAuthChanged(middleware.RequirePermission("grades:read")(http.HandlerFunc(gradesTermsH.Terms))))))
	// T15 grades by evaluation period, director: any assignment of the school.
	// Scale, period, enrollment and level rules are the grades service's,
	// identical for the teacher routes below; the capturing user (principal,
	// named from iam's directory) is recorded. Monthly capture (T6) is retired.
	directorPeriodH := handler.NewPeriodGradesHandler(gradesClient, studentsClient, authClient, handler.AnyTenantAssignment, handler.ScopeSchool)
	mux.Handle("GET /api/v1/grades/period-sheet", gradesRead(directorPeriodH.Sheet))
	mux.Handle("PUT /api/v1/grades/period-grades", gradesWrite(directorPeriodH.Put))
	mux.Handle("DELETE /api/v1/grades/period-grades", gradesWrite(directorPeriodH.Clear))

	// T7 period closing and report cards. Reading the periods, their progress
	// and the report cards is grades:read; closing and reopening is
	// grades:close, which only the owner holds (iam 0023) — grades:write is not
	// enough. The closing director is named from iam's directory for the
	// snapshot. The PDF prints the school's details from config.
	gradesClose := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("grades", requireAuthChanged(middleware.RequirePermission("grades:close")(http.HandlerFunc(h))))
	}
	periodsH := handler.NewGradesPeriodsHandler(gradesClient, authClient)
	mux.Handle("GET /api/v1/grades/periods", gradesRead(periodsH.List))
	mux.Handle("GET /api/v1/grades/periods/{termId}/progress", gradesRead(periodsH.Progress))
	mux.Handle("POST /api/v1/grades/periods/{termId}/close", gradesClose(periodsH.Close))
	mux.Handle("POST /api/v1/grades/periods/{termId}/reopen", gradesClose(periodsH.Reopen))
	var reportCardOptions []handler.ReportCardsOption
	if len(schoolLocations) > 0 {
		reportCardOptions = append(reportCardOptions, handler.WithReportCardZone(schoolLocations[0]))
	}
	reportCardsH := handler.NewReportCardsHandler(gradesClient, configClient, studentsClient, reportCardOptions...)
	mux.Handle("GET /api/v1/grades/report-cards", gradesRead(reportCardsH.List))
	mux.Handle("GET /api/v1/grades/report-cards/{id}", gradesRead(reportCardsH.Get))
	mux.Handle("GET /api/v1/grades/report-cards/{id}/pdf", gradesRead(reportCardsH.PDF))

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
	// T15 grades by evaluation period, teacher: only the assignments the caller
	// teaches in the active cycle (OwnsAssignment ⇒ 404 otherwise). The
	// ownership check reads the config catalog, so these compose
	// module_grades + module_config.
	teacherPeriodH := handler.NewPeriodGradesHandler(gradesClient, studentsClient, authClient, teachingH.OwnsAssignment, handler.ScopeTeacher)
	mux.Handle("GET /api/v1/teaching/grades/period-sheet",
		teachingModules("teaching:read", []string{"grades", "config"}, teacherPeriodH.Sheet))
	mux.Handle("PUT /api/v1/teaching/grades/period-grades",
		teachingModules("teaching:write", []string{"grades", "config"}, teacherPeriodH.Put))
	mux.Handle("DELETE /api/v1/teaching/grades/period-grades",
		teachingModules("teaching:write", []string{"grades", "config"}, teacherPeriodH.Clear))

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
	// Roster and save need only module_attendance (the classes list also reads
	// the config catalog, hence module_config). A supplied groupId is verified against the caller's OWN set
	// before any roster/save call — a group outside it answers 404, never 403,
	// leaking nothing about whether another group exists.
	teachingAttH := handler.NewTeachingAttendanceHandler(teachingH, attendanceClient, authClient, handler.WithScanCards(studentsClient))
	// T3: the lists the caller takes (daily groups + per-class assignments). It
	// reads the config catalog for each group's level mode, hence module_config.
	mux.Handle("GET /api/v1/teaching/attendance/classes",
		teachingModules("attendance_own:read", []string{"attendance", "config"}, teachingAttH.Classes))
	mux.Handle("GET /api/v1/teaching/attendance/roster",
		teachingModules("attendance_own:read", []string{"attendance"}, teachingAttH.Roster))
	mux.Handle("POST /api/v1/teaching/attendance",
		teachingModules("attendance_own:write", []string{"attendance"}, teachingAttH.SaveAttendance))
	// T4: a scanned virtual card resolves to a row of one of the caller's OWN
	// lists (the roster's ownership rule). It reads only — the list is saved by
	// the route above — and needs the card module too.
	mux.Handle("POST /api/v1/teaching/attendance/scan",
		teachingModules("attendance_own:read", []string{"attendance", "student_card"}, teachingAttH.Scan))

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
	financeH := handler.NewFinanceHandler(financeClient, handler.WithNames(authClient), handler.WithCutExtras(flags),
		// ADR 0040: voucherId / declinedVoucherIds on a deposit exist only
		// while module_payment_vouchers is on (404 otherwise).
		handler.WithPaymentVoucherFlag(flags))
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
	// T9: correcting a registered payment answers to payments:edit alone — the
	// owner's by default, delegable by the director (iam 0026) — never to
	// payments:write (taking money at the listed price) nor finance:write.
	// Its history is readable by whoever may correct, and by finance:read.
	paymentsEdit := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequirePermission("payments:edit")(h)))
	}
	mux.Handle("POST /api/v1/finance/deposits/{id}/correct", paymentsEdit(financeH.CorrectDeposit))
	// Payment vouchers, the office's side (ADR 0040): behind
	// module_payment_vouchers AND module_finance. The queue and the file are a
	// school-wide read of financial data, on purpose under collections:read like
	// /finance/overdue (ADR 0015); rejecting is taking a decision on money,
	// payments:write. Applying one is POST /finance/deposits with voucherId.
	vouchersH := handler.NewPaymentVouchersHandler(financeClient, studentsClient, authClient)
	vouchersStaff := func(gate func(http.Handler) http.Handler, h http.HandlerFunc) http.Handler {
		return guard.Protect("payment_vouchers", gate(h))
	}
	mux.Handle("GET /api/v1/finance/payment-vouchers", vouchersStaff(collectionsRead, vouchersH.Queue))
	mux.Handle("GET /api/v1/finance/payment-vouchers/{id}/file", vouchersStaff(collectionsRead, vouchersH.File))
	mux.Handle("POST /api/v1/finance/payment-vouchers/{id}/reject", vouchersStaff(paymentsWrite, vouchersH.Reject))
	mux.Handle("GET /api/v1/finance/deposits/{id}/corrections", guard.Protect("finance",
		requireAuthChanged(middleware.RequireAnyPermission("payments:edit", "finance:read")(http.HandlerFunc(financeH.DepositCorrections)))))
	mux.Handle("GET /api/v1/finance/accounts/{studentId}", collectionsRead(http.HandlerFunc(financeH.GetAccount)))
	// T7: the student's ledger (one computation, finance's) and its PDF, behind
	// the same permissions as the account it details.
	statements := receipt.NewStatementBuilder(financeClient, studentsClient, configClient)
	statementH := handler.NewStatementHandler(statements)
	mux.Handle("GET /api/v1/finance/accounts/{studentId}/statement", collectionsRead(http.HandlerFunc(statementH.JSON)))
	mux.Handle("GET /api/v1/finance/accounts/{studentId}/statement/pdf", collectionsRead(http.HandlerFunc(statementH.PDF)))
	// The overdue list is school-wide on purpose: it is the counter's own report
	// (who to call), which the owner approved for this role (ADR 0015).
	mux.Handle("GET /api/v1/finance/overdue", collectionsRead(http.HandlerFunc(financeH.ListOverdue)))

	// T16 — Horarios: the school timetable. It lives in config, so every route
	// needs module_config AND module_schedule (default OFF). Reads answer to
	// schedule:read, writes to schedule:write (iam 0028: the owner's by
	// default, delegable). The board carries the catalog the calendar needs,
	// so schedule:read alone (no config:read) draws the whole screen. A
	// teacher's own timetable is self-scoped under /teaching (teaching:read):
	// the teacher comes from the principal, never from the request.
	scheduleH := handler.NewScheduleHandler(configClient, authClient)
	scheduleModule := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("config", guard.Protect("schedule", requireAuthChanged(middleware.RequirePermission(perm)(h))))
	}
	scheduleRead := func(h http.HandlerFunc) http.Handler { return scheduleModule("schedule:read", h) }
	scheduleWrite := func(h http.HandlerFunc) http.Handler { return scheduleModule("schedule:write", h) }
	mux.Handle("GET /api/v1/schedule", scheduleRead(scheduleH.Board))
	mux.Handle("GET /api/v1/schedule/day", scheduleRead(scheduleH.Day))
	mux.Handle("GET /api/v1/schedule/available-teachers", scheduleRead(scheduleH.AvailableTeachers))
	mux.Handle("GET /api/v1/schedule/events", scheduleRead(scheduleH.Events))
	mux.Handle("POST /api/v1/schedule/blocks", scheduleWrite(scheduleH.CreateBlock))
	mux.Handle("PUT /api/v1/schedule/blocks/{id}", scheduleWrite(scheduleH.UpdateBlock))
	mux.Handle("DELETE /api/v1/schedule/blocks/{id}", scheduleWrite(scheduleH.DeleteBlock))
	mux.Handle("POST /api/v1/schedule/overrides", scheduleWrite(scheduleH.CreateOverride))
	mux.Handle("DELETE /api/v1/schedule/overrides/{id}", scheduleWrite(scheduleH.DeleteOverride))
	mux.Handle("PUT /api/v1/schedule/settings/{levelId}", scheduleWrite(scheduleH.PutSettings))
	// T19 — teacher absences (ADR 0032): same permissions; the handler shapes
	// an absence's kind and note away from callers without schedule:write.
	mux.Handle("GET /api/v1/schedule/absences", scheduleRead(scheduleH.Absences))
	mux.Handle("GET /api/v1/schedule/uncovered", scheduleRead(scheduleH.Uncovered))
	mux.Handle("POST /api/v1/schedule/absences", scheduleWrite(scheduleH.CreateAbsence))
	mux.Handle("PUT /api/v1/schedule/absences/{id}", scheduleWrite(scheduleH.UpdateAbsence))
	mux.Handle("POST /api/v1/schedule/absences/{id}/cancel", scheduleWrite(scheduleH.CancelAbsence))
	mux.Handle("GET /api/v1/teaching/schedule", scheduleModule("teaching:read", scheduleH.OwnBoard))
	mux.Handle("GET /api/v1/teaching/schedule/day", scheduleModule("teaching:read", scheduleH.OwnDay))

	// T10 — Egresos: expenses, their categories and receipts, the period's
	// income/expenses/balance and its reports (JSON, PDF, Excel), and closing
	// the day's cash cut. The data lives in finance, so every route needs
	// module_finance AND module_expenses (default OFF). Reads answer to
	// expenses:read, writes and closing the cut to expenses:write (iam 0027:
	// the owner's by default, delegable). Reopening a closed cut is the
	// director's: finance:write. Reports carry aggregates only — no family is
	// named — which is why expenses:read may be delegated while finance:read
	// stays owner-only.
	expenseReports := expensereport.NewBuilder(financeClient, configClient)
	expensesH := handler.NewExpensesHandler(financeClient, expenseReports, authClient, time.Now)
	expensesModule := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", guard.Protect("expenses", requireAuthChanged(middleware.RequirePermission(perm)(h))))
	}
	expensesRead := func(h http.HandlerFunc) http.Handler { return expensesModule("expenses:read", h) }
	expensesWrite := func(h http.HandlerFunc) http.Handler { return expensesModule("expenses:write", h) }
	mux.Handle("GET /api/v1/expense-categories", expensesRead(expensesH.ListCategories))
	mux.Handle("POST /api/v1/expense-categories", expensesWrite(expensesH.CreateCategory))
	mux.Handle("PUT /api/v1/expense-categories/{id}", expensesWrite(expensesH.UpdateCategory))
	mux.Handle("GET /api/v1/expenses/summary", expensesRead(expensesH.Summary))
	mux.Handle("GET /api/v1/expenses/reports", expensesRead(expensesH.Report))
	mux.Handle("GET /api/v1/expenses/reports/pdf", expensesRead(expensesH.ReportPDF))
	mux.Handle("GET /api/v1/expenses/reports/xlsx", expensesRead(expensesH.ReportXLSX))
	mux.Handle("GET /api/v1/expenses", expensesRead(expensesH.ListExpenses))
	mux.Handle("POST /api/v1/expenses", expensesWrite(expensesH.CreateExpense))
	mux.Handle("GET /api/v1/expenses/{id}", expensesRead(expensesH.GetExpense))
	mux.Handle("PUT /api/v1/expenses/{id}", expensesWrite(expensesH.UpdateExpense))
	mux.Handle("POST /api/v1/expenses/{id}/cancel", expensesWrite(expensesH.CancelExpense))
	mux.Handle("PUT /api/v1/expenses/{id}/receipt", expensesWrite(expensesH.PutReceipt))
	mux.Handle("GET /api/v1/expenses/{id}/receipt", expensesRead(expensesH.GetReceipt))
	mux.Handle("DELETE /api/v1/expenses/{id}/receipt", expensesWrite(expensesH.DeleteReceipt))
	mux.Handle("POST /api/v1/finance/cash-up/close", expensesWrite(expensesH.CloseCashCut))
	mux.Handle("POST /api/v1/finance/cash-up/reopen", expensesModule("finance:write", expensesH.ReopenCashCut))

	// T17 — Actividades: extracurriculars with their schedule, price options
	// and periodicity, and the pupils enrolled in each. Behind module_finance
	// AND module_activities (default OFF). Reads answer to activities:read,
	// writes (create/edit, enrol, unenrol, change a pupil's option) to
	// activities:write (iam 0029: owner by default, delegable). The charges an
	// enrolment causes are ordinary finance charges, collected through
	// Registrar pago (finance:write) like any other.
	activitiesH := handler.NewActivitiesHandler(financeClient)
	activitiesModule := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", guard.Protect("activities", requireAuthChanged(middleware.RequirePermission(perm)(h))))
	}
	activitiesRead := func(h http.HandlerFunc) http.Handler { return activitiesModule("activities:read", h) }
	activitiesWrite := func(h http.HandlerFunc) http.Handler { return activitiesModule("activities:write", h) }
	mux.Handle("GET /api/v1/activities", activitiesRead(activitiesH.List))
	mux.Handle("POST /api/v1/activities", activitiesWrite(activitiesH.Create))
	mux.Handle("GET /api/v1/activities/{id}", activitiesRead(activitiesH.Get))
	mux.Handle("PUT /api/v1/activities/{id}", activitiesWrite(activitiesH.Update))
	mux.Handle("GET /api/v1/activities/{id}/enrollments", activitiesRead(activitiesH.ListEnrollments))
	mux.Handle("POST /api/v1/activities/{id}/enrollments", activitiesWrite(activitiesH.Enroll))
	mux.Handle("POST /api/v1/activities/{id}/enrollments/end", activitiesWrite(activitiesH.Unenroll))
	mux.Handle("PUT /api/v1/activities/{id}/enrollments/{studentId}", activitiesWrite(activitiesH.ChangeOption))

	// T18 — Becas: scholarships (percent, fixed amount off, fixed amount to
	// pay) on the inscription, the tuition, catalog concepts and activities,
	// and the pupils holding each. Behind module_finance AND
	// module_scholarships (default OFF). Reads answer to scholarships:read,
	// writes (create/edit, give, take away) to scholarships:write (iam 0030:
	// owner by default, delegable). What one takes off a charge is finance's,
	// worked out on every read and payment (ADR 0022).
	scholarshipsH := handler.NewScholarshipsHandler(financeClient)
	scholarshipsModule := func(perm string, h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", guard.Protect("scholarships", requireAuthChanged(middleware.RequirePermission(perm)(h))))
	}
	scholarshipsRead := func(h http.HandlerFunc) http.Handler { return scholarshipsModule("scholarships:read", h) }
	scholarshipsWrite := func(h http.HandlerFunc) http.Handler { return scholarshipsModule("scholarships:write", h) }
	mux.Handle("GET /api/v1/scholarships", scholarshipsRead(scholarshipsH.List))
	mux.Handle("POST /api/v1/scholarships", scholarshipsWrite(scholarshipsH.Create))
	mux.Handle("GET /api/v1/scholarships/targets", scholarshipsRead(scholarshipsH.Targets))
	mux.Handle("GET /api/v1/scholarships/{id}", scholarshipsRead(scholarshipsH.Get))
	mux.Handle("PUT /api/v1/scholarships/{id}", scholarshipsWrite(scholarshipsH.Update))
	mux.Handle("GET /api/v1/scholarships/{id}/students", scholarshipsRead(scholarshipsH.Holders))
	mux.Handle("POST /api/v1/scholarships/{id}/students", scholarshipsWrite(scholarshipsH.Assign))
	mux.Handle("POST /api/v1/scholarships/{id}/students/remove", scholarshipsWrite(scholarshipsH.Unassign))

	// M06 attendance (daily pass list): flag-guarded, authenticated + rotation-
	// enforced, permission-gated. Saving a group·day is the only write
	// (attendance:write) — it reconciles the whole day, so it also erases marks;
	// the roster and the per-enrollment history are reads (attendance:read).
	// Every route sits behind guard.Protect("attendance", …) so
	// module_attendance OFF ⇒ 404 (the module is invisible before auth even
	// runs). The more specific /attendance/roster wins over /attendance under
	// ServeMux's specificity rule.
	attendanceH := handler.NewAttendanceHandler(attendanceClient, configClient, studentsClient, authClient, handler.WithDirectorScanCards(studentsClient))
	attendanceWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("attendance", requireAuthChanged(middleware.RequirePermission("attendance:write")(http.HandlerFunc(h))))
	}
	attendanceRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("attendance", requireAuthChanged(middleware.RequirePermission("attendance:read")(http.HandlerFunc(h))))
	}
	mux.Handle("GET /api/v1/attendance/roster", attendanceRead(attendanceH.GetRoster))
	mux.Handle("POST /api/v1/attendance", attendanceWrite(attendanceH.SaveDaily))
	mux.Handle("GET /api/v1/attendance", attendanceRead(attendanceH.ListAttendance))
	// T12: the attendance report of a group over a date range (JSON, PDF,
	// Excel) — a read of the whole group, so attendance:read (the director's).
	// A teacher's own-group report is deferred (attendance_own:* does not open it).
	attendanceReportH := handler.NewAttendanceReportHandler(attendanceClient, configClient, studentsClient, configClient)
	mux.Handle("GET /api/v1/attendance/reports", attendanceRead(attendanceReportH.Report))
	mux.Handle("GET /api/v1/attendance/reports/pdf", attendanceRead(attendanceReportH.ReportPDF))
	mux.Handle("GET /api/v1/attendance/reports/xlsx", attendanceRead(attendanceReportH.ReportXLSX))
	// T4: resolve a scanned card to a row of any list (a read, like the roster),
	// behind module_student_card as well.
	mux.Handle("POST /api/v1/attendance/scan", guard.Protect("student_card", attendanceRead(attendanceH.Scan)))

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
	// ADR 0041 (T5): scope "family" reads students, so it needs module_students
	// as well as module_families; either off ⇒ that scope is refused with 400.
	familyAudienceOn := func(ctx context.Context) bool {
		return flags.Enabled(ctx, featureflag.Key("students")) && familiesOn(ctx)
	}
	announcementsH := handler.NewAnnouncementHandler(commsClient, studentsClient, configClient,
		handler.WithFamilyAudience(familyAudienceOn, studentsClient))
	mux.Handle("POST /api/v1/comms/announcements", commsWrite(announcementsH.Announce))
	// T11: the school's birthday greeting. Editing it answers to comms:write —
	// the permission that already sends announcements, held by the owner and
	// delegable to whoever writes to families. Reading it does too: the screen
	// that shows it is the screen that edits it. The daily RUN is deliberately
	// not here: Cloud Scheduler calls comms directly (deploy/terraform/
	// scheduler.tf), so no session can trigger a send.
	birthdayH := handler.NewBirthdayTemplateHandler(commsClient)
	mux.Handle("GET /api/v1/comms/birthday-template", commsWrite(birthdayH.Get))
	mux.Handle("PUT /api/v1/comms/birthday-template", commsWrite(birthdayH.Update))

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
	uniformsH := handler.NewUniformsHandler(uniformsClient).WithLevelGuard(configClient).WithNames(authClient)
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
	// C2-015: a return undoes a sale, so it takes what a sale takes.
	mux.Handle("POST /api/v1/uniforms/orders/{id}/return", uniformsSell(uniformsH.ReturnCounterSale))

	// Receipts (task 12): proof of what a deposit or a sale took, as JSON for the
	// 80 mm ticket the browser prints and as a PDF. A payment's answers to whoever
	// sees the money (the counter or finance); a sale's to whoever reads orders.
	// ISSUE-017: the pupil's group of the payment's day, with its grade and level.
	var receiptZone *time.Location
	if len(schoolLocations) > 0 {
		receiptZone = schoolLocations[0]
	}
	groupLabeler := handler.NewStudentGroupLabeler(studentsClient, configClient, receiptZone)
	receiptOptions := []receipt.Option{
		receipt.WithSchoolLocation(receiptZone),
		receipt.WithGroupLabeler(groupLabeler),
		// The sale receipt says who collected (T14-019), as the payment one does.
		receipt.WithUserNamer(authClient),
	}
	receiptBuilder := receipt.NewBuilder(financeClient, uniformsClient, studentsClient, financeClient, configClient, receiptOptions...)
	receiptsH := handler.NewReceiptsHandler(receiptBuilder)
	paymentReceipt := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequireAnyPermission("finance:read", "collections:read")(h)))
	}
	mux.Handle("GET /api/v1/receipts/payment/{id}", paymentReceipt(receiptsH.PaymentJSON))
	mux.Handle("GET /api/v1/receipts/payment/{id}/pdf", paymentReceipt(receiptsH.PaymentPDF))
	mux.Handle("GET /api/v1/receipts/sale/{id}", uniformsOrders(receiptsH.SaleJSON))
	mux.Handle("GET /api/v1/receipts/sale/{id}/pdf", uniformsOrders(receiptsH.SalePDF))
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
	familyH := handler.NewFamilyHandler(studentsClient, commsClient, financeClient, gradesClient, attendanceClient, configClient,
		handler.WithFamilyStatements(statements), handler.WithFamilyReceipts(receiptBuilder),
		// ISSUE-T14-108: the child's group named as the receipts name it.
		handler.WithFamilyGroupLabeler(groupLabeler, receiptZone))
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
	// T5 push registration for the family app. The guardian holds family:read,
	// not comms:write, so this is its own door; the owner is the principal. It
	// is guarded on comms (the device store) AND students, like the inbox it
	// feeds, so either module OFF hides it.
	familyDevicesH := handler.NewFamilyDevicesHandler(commsClient)
	mux.Handle("POST /api/v1/family/devices", familyComms(familyDevicesH.Register))
	mux.Handle("DELETE /api/v1/family/devices/{token}", familyComms(familyDevicesH.Unregister))
	// The students list is scoped by the principal alone (no {studentId}), so it
	// needs only module_students — the app calls it first to discover its children.
	mux.Handle("GET /api/v1/family/students",
		familyReadModules([]string{"students"}, familyH.ListStudents))
	// The by-child reads each compose module_students (ownership) with the module
	// that owns the data.
	mux.Handle("GET /api/v1/family/students/{studentId}/account",
		familyReadModules([]string{"finance", "students"}, familyH.Account))
	// T7: the child's ledger — the same computation the office reads, in the
	// family's view — and its PDF. Ownership is checked by the handler.
	mux.Handle("GET /api/v1/family/students/{studentId}/statement",
		familyReadModules([]string{"finance", "students"}, familyH.Statement))
	mux.Handle("GET /api/v1/family/students/{studentId}/statement/pdf",
		familyReadModules([]string{"finance", "students"}, familyH.StatementPDF))
	// How to pay by transfer: the school's CLABE and the child's own bank
	// reference, beside the statement it pays.
	familyPaymentH := handler.NewFamilyPaymentInfoHandler(studentsClient, studentsClient, configClient)
	mux.Handle("GET /api/v1/family/students/{studentId}/payment-info",
		familyReadModules([]string{"finance", "students"}, familyPaymentH.PaymentInfo))
	// ISSUE-T14-107: the receipt of one of the child's payments, the office's
	// receipt in the family's view. The handler checks the child is the
	// caller's; the builder that the deposit is that child's (404 either way).
	mux.Handle("GET /api/v1/family/students/{studentId}/receipts/{depositId}",
		familyReadModules([]string{"finance", "students"}, familyH.ReceiptJSON))
	mux.Handle("GET /api/v1/family/students/{studentId}/receipts/{depositId}/pdf",
		familyReadModules([]string{"finance", "students"}, familyH.ReceiptPDF))
	mux.Handle("GET /api/v1/family/students/{studentId}/profile",
		familyReadModules([]string{"students"}, familyH.Profile))
	mux.Handle("GET /api/v1/family/students/{studentId}/grades",
		familyReadModules([]string{"grades", "students"}, familyH.Grades))
	// T15: the child's grades by evaluation period, visible as soon as they
	// are captured.
	familyPeriodsH := handler.NewFamilyPeriodGradesHandler(studentsClient, gradesClient, configClient, configClient)
	mux.Handle("GET /api/v1/family/students/{studentId}/grades/periods",
		familyReadModules([]string{"grades", "students"}, familyPeriodsH.PeriodGrades))
	// T7: the child's report cards — the latest published version per period —
	// and their PDF. Ownership (404 for another family's child, or a card of
	// another pupil) is checked by the handler.
	mux.Handle("GET /api/v1/family/students/{studentId}/report-cards",
		familyReadModules([]string{"grades", "students"}, reportCardsH.FamilyList))
	mux.Handle("GET /api/v1/family/students/{studentId}/report-cards/{id}",
		familyReadModules([]string{"grades", "students"}, reportCardsH.FamilyGet))
	mux.Handle("GET /api/v1/family/students/{studentId}/report-cards/{id}/pdf",
		familyReadModules([]string{"grades", "students"}, reportCardsH.FamilyPDF))
	mux.Handle("GET /api/v1/family/students/{studentId}/attendance",
		familyReadModules([]string{"attendance", "students"}, familyH.Attendance))

	// Payment vouchers, the family's side (ADR 0040): the SECOND guardian
	// write (vouchers_own:write, iam 0033), for the caller's own child only
	// (404 otherwise, checked by the handler). Behind module_payment_vouchers,
	// module_finance (where they live) and module_students (the ownership
	// check). The upload has its OWN per-guardian budget: the login limiter is
	// per IP, and behind Cloud Run every request comes from one front end.
	familyVouchers := func(permission string, h http.Handler) http.Handler {
		wrapped := requireAuthChanged(middleware.RequirePermission(permission)(h))
		for _, module := range []string{"students", "finance", "payment_vouchers"} {
			wrapped = guard.Protect(module, wrapped)
		}
		return wrapped
	}
	voucherLimiter := middleware.NewRateLimiter(voucherUploadBurst, voucherUploadRefillPerSec, time.Now)
	mux.Handle("POST /api/v1/family/students/{studentId}/payment-vouchers",
		familyVouchers("vouchers_own:write", voucherLimiter.PerPrincipalMiddleware(http.HandlerFunc(vouchersH.FamilyUpload))))
	mux.Handle("GET /api/v1/family/students/{studentId}/payment-vouchers",
		familyVouchers("family:read", http.HandlerFunc(vouchersH.FamilyList)))

	// The virtual card (task 13), behind module_student_card (default OFF): the
	// family reads its own child's card and photo; staff verify a scanned code
	// (students:read) and revoke a card (students:update).
	cardH := handler.NewCardHandler(studentsClient, studentsClient, studentsClient, configClient, configClient,
		handler.WithCardImportReview(importReviewOn), handler.WithCardGroupLabeler(groupLabeler, receiptZone))
	mux.Handle("GET /api/v1/family/students/{studentId}/card",
		familyReadModules([]string{"student_card", "students"}, cardH.FamilyCard))
	mux.Handle("GET /api/v1/family/students/{studentId}/photo",
		familyReadModules([]string{"student_card", "students"}, cardH.FamilyPhoto))
	cardStaff := func(permission string, h http.HandlerFunc) http.Handler {
		return guard.Protect("student_card", guard.Protect("students",
			requireAuthChanged(middleware.RequirePermission(permission)(h))))
	}
	mux.Handle("POST /api/v1/students/card/verify", cardStaff("students:read", cardH.Verify))
	mux.Handle("POST /api/v1/students/{id}/card/revoke", cardStaff("students:update", cardH.Revoke))
	mux.Handle("POST /api/v1/students/{id}/card/reissue", cardStaff("students:update", cardH.Reissue))

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
	// T2: deleting is users:delete, owner-only (iam 0025) — users:write is not
	// enough. iam re-checks against the caller's own token.
	mux.Handle("DELETE /api/v1/users/{id}", iamAdmin("users:delete", adminH.DeleteUser))
	mux.Handle("POST /api/v1/users/{id}/roles", iamAdmin("users:write", adminH.SetUserRoles))
	mux.Handle("POST /api/v1/roles", iamAdmin("roles:write", adminH.CreateRole))
	mux.Handle("GET /api/v1/roles", iamAdmin("roles:read", adminH.ListRoles))
	mux.Handle("POST /api/v1/roles/{id}/permissions", iamAdmin("roles:write", adminH.SetRolePermissions))

	// The teacher↔user link mutates a teacher record, so it sits under
	// module_teachers + teachers:write (teachersWrite), not module_iam. The
	// handler itself performs the gateway-orchestrated cross-service validation
	// (iam GetUser) before the config write.
	mux.Handle(handler.TeacherUserLinkRoute, teachersWrite(adminH.LinkTeacherUser))

	// "Dar acceso al sistema" (T2): invite the teacher into the maestro role and
	// link the account, in one action. It edits a teacher AND mints an account,
	// so it needs both teachers:write and users:write; inside, the caller-subset
	// guard refuses a caller that does not itself hold the maestro permissions.
	// Not behind inviteLimiter: that budget guards the anonymous doors; like
	// POST /users/invite, this is an authenticated administrator's action.
	teacherAccessH := handler.NewTeacherAccessHandler(authClient, configClient)
	// Guarded on BOTH modules it composes: with Usuarios (module_iam) hidden,
	// minting an account from Docentes is hidden too (404), not half-working.
	mux.Handle(handler.TeacherAccessRoute, guard.Protect("teachers", guard.Protect("iam", requireAuthChanged(
		middleware.RequireAllPermissions("teachers:write", "users:write")(http.HandlerFunc(teacherAccessH.GrantAccess))))))
	return mux
}

// The family's payment-voucher upload budget, per guardian (ADR 0040): a
// handful in a burst (a month and its arrears, a retry), then about one every
// six minutes. Finance holds the real ceiling (three pending per child); this
// only stops a loop from filling the bucket.
// The undo of an import's families is idempotent and an office does it once
// or twice per file: five in a row, then one a minute, only stops a loop.
const (
	importFamiliesUndoBurst        = 5
	importFamiliesUndoRefillPerSec = 1.0 / 60
)

const (
	voucherUploadBurst        = 10
	voucherUploadRefillPerSec = 1.0 / 360
)
