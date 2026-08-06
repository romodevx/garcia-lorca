package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// These tests assemble the exact production routes (newMux) against stub iam and
// students Connect servers, exercising the full security chain of the M03
// students route: ModuleGuard(students) → RequireAuth → RequirePermission.

type errStr string

func (e errStr) Error() string { return string(e) }

// stubIAM validates tokens by name: "with-perm" holds students:create,
// "no-perm" is authenticated but lacks it, anything else is invalid.
type stubIAM struct {
	iamv1connect.UnimplementedIamServiceHandler
}

func (stubIAM) ValidateToken(_ context.Context, req *connect.Request[iamv1.ValidateTokenRequest]) (*connect.Response[iamv1.ValidateTokenResponse], error) {
	perms := map[string][]string{
		"with-perm":     {"students:create"},
		"no-perm":       {"students:read"},
		"config-writer": {"config:write", "config:read"},
		"config-reader": {"config:read"},
	}
	granted, ok := perms[req.Msg.GetAccessToken()]
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("invalid token"))
	}
	return connect.NewResponse(&iamv1.ValidateTokenResponse{Claims: &iamv1.Claims{
		Subject: "u1", TenantId: "t1", Email: "ana@garcialorca.mx", Permissions: granted,
	}}), nil
}

// stubStudents returns a canned pending applicant for any CreateApplicant.
type stubStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
}

func (stubStudents) CreateApplicant(_ context.Context, req *connect.Request[studentsv1.CreateApplicantRequest]) (*connect.Response[studentsv1.CreateApplicantResponse], error) {
	return connect.NewResponse(&studentsv1.CreateApplicantResponse{Student: &studentsv1.Student{
		Id:              "stu-1",
		Curp:            req.Msg.GetCurp(),
		FirstName:       req.Msg.GetFirstName(),
		Level:           req.Msg.GetLevel(),
		AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING,
	}}), nil
}

// stubConfig echoes a canned cycle for CreateCycle and one cycle for ListCycles,
// enough to exercise the config route security chain end to end.
type stubConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
}

func (stubConfig) CreateCycle(_ context.Context, req *connect.Request[configv1.CreateCycleRequest]) (*connect.Response[configv1.CreateCycleResponse], error) {
	return connect.NewResponse(&configv1.CreateCycleResponse{Cycle: &configv1.Cycle{
		Id:     "cyc-1",
		Name:   req.Msg.GetName(),
		Status: req.Msg.GetStatus(),
	}}), nil
}

func (stubConfig) ListCycles(_ context.Context, _ *connect.Request[configv1.ListCyclesRequest]) (*connect.Response[configv1.ListCyclesResponse], error) {
	return connect.NewResponse(&configv1.ListCyclesResponse{Cycles: []*configv1.Cycle{
		{Id: "cyc-1", Name: "2026-2027", Status: configv1.CycleStatus_CYCLE_STATUS_DRAFT},
	}}), nil
}

func connectServer(t *testing.T, path string, handler http.Handler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTestGateway wires newMux against stub iam + students servers, with the given
// flag map (keyed by full flag key, e.g. "module_students").
func newTestGateway(t *testing.T, flags map[string]bool) *httptest.Server {
	t.Helper()
	iamPath, iamHandler := iamv1connect.NewIamServiceHandler(stubIAM{})
	iamSrv := connectServer(t, iamPath, iamHandler)
	stuPath, stuHandler := studentsv1connect.NewStudentsServiceHandler(stubStudents{})
	stuSrv := connectServer(t, stuPath, stuHandler)
	cfgPath, cfgHandler := configv1connect.NewConfigServiceHandler(stubConfig{})
	cfgSrv := connectServer(t, cfgPath, cfgHandler)

	// A generous burst keeps the limiter out of the way of these route tests;
	// rate-limit behavior is covered directly in the middleware package.
	loginLimiter := middleware.NewRateLimiter(1000, 1000, time.Now)
	mux := newMux(
		client.NewIAMClient(iamSrv.URL, iamSrv.Client()),
		client.NewAuthClient(iamSrv.URL, iamSrv.Client()),
		client.NewStudentsClient(stuSrv.URL, stuSrv.Client()),
		client.NewConfigClient(cfgSrv.URL, cfgSrv.Client()),
		featureflag.NewInMemory(flags),
		loginLimiter,
	)
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)
	return gw
}

const validBody = `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria"}`

func postStudent(t *testing.T, gw *httptest.Server, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, gw.URL+"/api/v1/students", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := gw.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestStudentsRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	// Even a caller with the right token must not learn the module exists.
	res := postStudent(t, gw, "with-perm", validBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestStudentsRouteEnabledNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := postStudent(t, gw, "", validBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestStudentsRouteEnabledMissingPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := postStudent(t, gw, "no-perm", validBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestStudentsRouteEnabledWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := postStudent(t, gw, "with-perm", validBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

// --- config catalog routes: ModuleGuard(config) → RequireAuth → RequirePermission

const validCycleBody = `{"name":"2026-2027","status":"draft","startsOn":"2026-08-01","endsOn":"2027-07-15"}`

func doGateway(t *testing.T, gw *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, gw.URL+path, nil)
	} else {
		r, err = http.NewRequest(method, gw.URL+path, strings.NewReader(body))
	}
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := gw.Client().Do(r)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestConfigRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_config OFF
	// Even a caller with config:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "config-writer", validCycleBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestConfigWriteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "", validCycleBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestConfigWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	// A read-only principal may not create.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "config-reader", validCycleBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestConfigWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "config-writer", validCycleBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

func TestConfigReadWithReadPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/config/cycles", "config-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestConfigReadDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_config OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/config/cycles", "config-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}
