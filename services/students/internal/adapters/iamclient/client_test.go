package iamclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/iamclient"
)

// stubIamService is a minimal iam service. It embeds the generated Unimplemented
// handler (every RPC returns Unimplemented) and overrides only GetUser/ListRoles,
// so the adapter is exercised against the REAL Connect client over an in-process
// httptest server — no network, fully deterministic.
type stubIamService struct {
	iamv1connect.UnimplementedIamServiceHandler
	user         *iamv1.User
	roles        []*iamv1.Role
	userNotFound bool
	rolesFail    bool
	listRoleCall int
}

func (s *stubIamService) GetUser(_ context.Context, _ *connect.Request[iamv1.GetUserRequest]) (*connect.Response[iamv1.GetUserResponse], error) {
	if s.userNotFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "user not found"))
	}
	return connect.NewResponse(&iamv1.GetUserResponse{User: s.user}), nil
}

func (s *stubIamService) ListRoles(_ context.Context, _ *connect.Request[iamv1.ListRolesRequest]) (*connect.Response[iamv1.ListRolesResponse], error) {
	s.listRoleCall++
	if s.rolesFail {
		return nil, connect.NewError(connect.CodeUnavailable, glerrors.New(glerrors.KindInternal, "roles unavailable"))
	}
	return connect.NewResponse(&iamv1.ListRolesResponse{Roles: s.roles}), nil
}

func newClient(t *testing.T, svc *stubIamService) *iamclient.Client {
	t.Helper()
	path, handler := iamv1connect.NewIamServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return iamclient.New(srv.URL, srv.Client())
}

const (
	userID        = "u0000000-0000-0000-0000-000000000001"
	guardianRole  = "a0000000-0000-0000-0000-0000000000a1"
	teacherRoleID = "a0000000-0000-0000-0000-0000000000a2"
)

// iam's User carries role IDS, not names, so the adapter resolves them against
// ListRoles: the port speaks role NAMES.
func TestByIDProjectsStatusAndRoleNames(t *testing.T) {
	svc := &stubIamService{
		user: &iamv1.User{
			Id:      userID,
			Status:  iamv1.UserStatus_USER_STATUS_ACTIVE,
			RoleIds: []string{guardianRole, teacherRoleID},
		},
		roles: []*iamv1.Role{
			{Id: guardianRole, Name: "guardian"},
			{Id: teacherRoleID, Name: "teacher"},
		},
	}
	got, err := newClient(t, svc).ByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.ID != userID {
		t.Errorf("id mismatch: %q", got.ID)
	}
	if got.Status != iamv1.UserStatus_USER_STATUS_ACTIVE.String() {
		t.Errorf("status mismatch: %q", got.Status)
	}
	if len(got.Roles) != 2 || got.Roles[0] != "guardian" || got.Roles[1] != "teacher" {
		t.Errorf("role names mismatch: %+v", got.Roles)
	}
}

// A role id with no matching role is dropped rather than surfaced as an opaque
// uuid the use case would compare against a name.
func TestByIDDropsUnresolvableRoleIDs(t *testing.T) {
	svc := &stubIamService{
		user:  &iamv1.User{Id: userID, RoleIds: []string{guardianRole, "ghost"}},
		roles: []*iamv1.Role{{Id: guardianRole, Name: "guardian"}},
	}
	got, err := newClient(t, svc).ByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "guardian" {
		t.Errorf("want only the resolvable role, got %+v", got.Roles)
	}
}

// A user with no roles needs no ListRoles round-trip at all.
func TestByIDSkipsRoleLookupWhenUserHasNoRoles(t *testing.T) {
	svc := &stubIamService{user: &iamv1.User{Id: userID}}
	got, err := newClient(t, svc).ByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if len(got.Roles) != 0 {
		t.Errorf("want no roles, got %+v", got.Roles)
	}
	if svc.listRoleCall != 0 {
		t.Errorf("ListRoles must not be called for a role-less user (called %d times)", svc.listRoleCall)
	}
}

func TestByIDNotFoundMapsToErrNotFound(t *testing.T) {
	_, err := newClient(t, &stubIamService{userNotFound: true}).ByID(context.Background(), userID)
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v (kind %v)", err, glerrors.KindOf(err))
	}
}

// Any non-NotFound transport failure is KindInternal — never a bad argument.
func TestByIDTransportFailureIsInternal(t *testing.T) {
	svc := &stubIamService{
		user:      &iamv1.User{Id: userID, RoleIds: []string{guardianRole}},
		rolesFail: true,
	}
	_, err := newClient(t, svc).ByID(context.Background(), userID)
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("want KindInternal, got %v (kind %v)", err, glerrors.KindOf(err))
	}
}
