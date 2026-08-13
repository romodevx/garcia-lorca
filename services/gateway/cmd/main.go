// Command gateway is the entrypoint for the García Lorca BFF / edge gateway.
//
// FASE 0: the gateway boots an HTTP server with its own health endpoint and a
// /readyz that calls the iam service end to end — the gateway half of the
// Sprint-1 pipeline-validation hello-world. The feature-flag middleware
// (internal/middleware) is wired but guards no module routes yet; modules plug
// in as they land.
package main

import (
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

	// flags is the authoritative source of module visibility (default-deny),
	// loaded from ENABLED_MODULES (comma-separated module names). The gateway
	// both enforces the boundary (ModuleGuard ⇒ 404) and reports it (GET
	// /features). Each module wires its routes behind the guard as it lands.
	flags := featureflag.NewInMemory(config.ParseEnabledModules(os.Getenv("ENABLED_MODULES")))

	iamHealth := client.NewIAMClient(iamBaseURL, http.DefaultClient)
	authClient := client.NewAuthClient(iamBaseURL, http.DefaultClient)
	studentsClient := client.NewStudentsClient(studentsBaseURL, http.DefaultClient)
	configClient := client.NewConfigClient(configBaseURL, http.DefaultClient)
	gradesClient := client.NewGradesClient(gradesBaseURL, http.DefaultClient)
	financeClient := client.NewFinanceClient(financeBaseURL, http.DefaultClient)
	attendanceClient := client.NewAttendanceClient(attendanceBaseURL, http.DefaultClient)
	commsClient := client.NewCommsClient(commsBaseURL, http.DefaultClient)

	// loginLimiter fronts POST /auth/login with a per-IP token bucket so an
	// attacker cannot brute-force credentials from a single source. It is
	// in-process (fine while the gateway runs at min-instances=1).
	loginRL := config.ParseLoginRateLimit(os.Getenv)
	loginLimiter := middleware.NewRateLimiter(loginRL.Burst, loginRL.RefillPerSec(), time.Now)

	mux := newMux(iamHealth, authClient, studentsClient, configClient, gradesClient, financeClient, attendanceClient, commsClient, flags, loginLimiter)

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
//	POST /api/v1/config/teachers/{id}/user → ModuleGuard(config) → RequireAuth →
//	                             rotation → RequirePermission(config:write) →
//	                             validate iam user (GetUser) then config SetTeacherUser
//	/healthz, /readyz          → liveness + iam reachability
//
// `flags` (via middleware.NewModuleGuard) is the module-visibility boundary:
// disabled module ⇒ 404, evaluated before auth so a hidden module is invisible.
func newMux(iamHealth *client.IAMClient, authClient *client.AuthClient, studentsClient *client.StudentsClient, configClient *client.ConfigClient, gradesClient *client.GradesClient, financeClient *client.FinanceClient, attendanceClient *client.AttendanceClient, commsClient *client.CommsClient, flags featureflag.Checker, loginLimiter *middleware.RateLimiter) *http.ServeMux {
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
	mux.Handle("POST /api/v1/auth/mobile/login", loginLimiter.Middleware(http.HandlerFunc(authH.MobileLogin)))
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
	mux.Handle("POST /api/v1/guardians/unlink", guardiansWrite(guardiansH.UnlinkGuardian))
	mux.Handle("GET /api/v1/students/{studentId}/guardians", guardiansRead(guardiansH.ListGuardiansForStudent))

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
	mux.Handle("POST /api/v1/config/levels", configWrite(configH.CreateLevel))
	mux.Handle("GET /api/v1/config/levels", configRead(configH.ListLevels))
	mux.Handle("POST /api/v1/config/grade-levels", configWrite(configH.CreateGradeLevel))
	mux.Handle("GET /api/v1/config/grade-levels", configRead(configH.ListGradeLevels))
	mux.Handle("POST /api/v1/config/subjects", configWrite(configH.CreateSubject))
	mux.Handle("GET /api/v1/config/subjects", configRead(configH.ListSubjects))
	mux.Handle("POST /api/v1/config/terms", configWrite(configH.CreateTerm))
	mux.Handle("GET /api/v1/config/terms", configRead(configH.ListTerms))
	mux.Handle("POST /api/v1/config/groups", configWrite(configH.CreateGroup))
	mux.Handle("GET /api/v1/config/groups", configRead(configH.ListGroups))
	mux.Handle("POST /api/v1/config/teachers", configWrite(configH.CreateTeacher))
	mux.Handle("GET /api/v1/config/teachers", configRead(configH.ListTeachers))
	mux.Handle("POST /api/v1/config/teaching-assignments", configWrite(configH.CreateTeachingAssignment))
	mux.Handle("GET /api/v1/config/teaching-assignments", configRead(configH.ListTeachingAssignments))

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

	// M05 finance (adeudos): flag-guarded, authenticated + rotation-enforced,
	// permission-gated. Everything that moves money or the catalog is a write
	// (finance:write): concept create/update, charge create/generate, discount,
	// cancel, payment. The catalog, charge list, account statement and overdue
	// report are reads (finance:read). Every route sits behind
	// guard.Protect("finance", …) so module_finance OFF ⇒ 404 (the module is
	// invisible before auth even runs). The more specific patterns
	// (/charges/generate, /charges/{id}/discount, /charges/{id}/cancel) win over
	// /charges under ServeMux's specificity rule.
	financeH := handler.NewFinanceHandler(financeClient)
	financeWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequirePermission("finance:write")(http.HandlerFunc(h))))
	}
	financeRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("finance", requireAuthChanged(middleware.RequirePermission("finance:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/finance/concepts", financeWrite(financeH.CreateConcept))
	mux.Handle("PUT /api/v1/finance/concepts/{id}", financeWrite(financeH.UpdateConcept))
	mux.Handle("GET /api/v1/finance/concepts", financeRead(financeH.ListConcepts))
	mux.Handle("POST /api/v1/finance/charges", financeWrite(financeH.CreateCharge))
	mux.Handle("POST /api/v1/finance/charges/generate", financeWrite(financeH.GenerateCharges))
	mux.Handle("POST /api/v1/finance/charges/{id}/discount", financeWrite(financeH.ApplyDiscount))
	mux.Handle("POST /api/v1/finance/charges/{id}/cancel", financeWrite(financeH.CancelCharge))
	mux.Handle("GET /api/v1/finance/charges", financeRead(financeH.ListCharges))
	mux.Handle("POST /api/v1/finance/payments", financeWrite(financeH.RegisterPayment))
	mux.Handle("GET /api/v1/finance/accounts/{studentId}", financeRead(financeH.GetAccount))
	mux.Handle("GET /api/v1/finance/overdue", financeRead(financeH.ListOverdue))

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
	familyH := handler.NewFamilyHandler(studentsClient, commsClient, financeClient, gradesClient, attendanceClient)
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
	mux.Handle("POST /api/v1/config/teachers/{id}/user", configWrite(adminH.LinkTeacherUser))
	return mux
}
