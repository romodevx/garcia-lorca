package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// --- doubles ---------------------------------------------------------------

// fakeTeacherIAM is the iam half of "Dar acceso al sistema": the roles, the
// accounts that already exist, and the writes, each recorded so a test can prove
// a refused request wrote nothing.
type fakeTeacherIAM struct {
	roles      []client.Role
	users      []client.User
	getUserErr error

	invitedEmail   string
	invitedName    string
	invitedRoleIDs []string
	invitedUser    client.User
	invitation     client.Invitation
	inviteCalls    int

	resentUserID string
	resent       client.Invitation
	resendCalls  int

	assignedUserID string
	assignedRoleID string
	assignCalls    int
}

func (f *fakeTeacherIAM) ListRoles(context.Context) ([]client.Role, error) { return f.roles, nil }

// ListUsers leaves deleted accounts out, as iam does (T2).
func (f *fakeTeacherIAM) ListUsers(context.Context) ([]client.User, error) {
	out := make([]client.User, 0, len(f.users))
	for _, u := range f.users {
		if u.Status != "deleted" {
			out = append(out, u)
		}
	}
	return out, nil
}

func (f *fakeTeacherIAM) GetUser(_ context.Context, id string) (client.User, error) {
	if f.getUserErr != nil {
		return client.User{}, f.getUserErr
	}
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return client.User{}, connect.NewError(connect.CodeNotFound, errors.New("no user"))
}

func (f *fakeTeacherIAM) InviteUser(_ context.Context, email, displayName string, roleIDs []string) (client.User, client.Invitation, error) {
	f.inviteCalls++
	f.invitedEmail, f.invitedName, f.invitedRoleIDs = email, displayName, roleIDs
	return f.invitedUser, f.invitation, nil
}

func (f *fakeTeacherIAM) ResendInvitation(_ context.Context, userID string) (client.Invitation, error) {
	f.resendCalls++
	f.resentUserID = userID
	return f.resent, nil
}

func (f *fakeTeacherIAM) AssignRole(_ context.Context, userID, roleID string) error {
	f.assignCalls++
	f.assignedUserID, f.assignedRoleID = userID, roleID
	return nil
}

func (f *fakeTeacherIAM) writes() int { return f.inviteCalls + f.resendCalls + f.assignCalls }

// fakeTeacherLinkConfig is the config half: the teacher and the link write.
type fakeTeacherLinkConfig struct {
	teachers map[string]client.Teacher

	linkedTeacherID string
	linkedUserID    string
	linkCalls       int
	linkErr         error
}

func (f *fakeTeacherLinkConfig) GetTeacher(_ context.Context, id string) (client.Teacher, error) {
	t, ok := f.teachers[id]
	if !ok {
		return client.Teacher{}, connect.NewError(connect.CodeNotFound, errors.New("teacher not found"))
	}
	return t, nil
}

func (f *fakeTeacherLinkConfig) ListTeachers(context.Context) ([]client.Teacher, error) {
	out := make([]client.Teacher, 0, len(f.teachers))
	for _, t := range f.teachers {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeTeacherLinkConfig) SetTeacherUser(_ context.Context, teacherID, userID string) (client.Teacher, error) {
	f.linkCalls++
	f.linkedTeacherID, f.linkedUserID = teacherID, userID
	if f.linkErr != nil {
		return client.Teacher{}, f.linkErr
	}
	t := f.teachers[teacherID]
	t.UserID = userID
	return t, nil
}

// --- fixtures ---------------------------------------------------------------

const maestroRoleID = "00000000-0000-0000-0000-0000000000a4"

var maestroPerms = []string{"teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"}

// ownerCaller holds what an owner holds for this action: the route's two
// permissions plus the maestro role's four (the caller-subset guard).
var ownerCaller = append([]string{"teachers:write", "users:write", "finance:read"}, maestroPerms...)

func teacherIAM() *fakeTeacherIAM {
	return &fakeTeacherIAM{
		roles: []client.Role{
			{ID: "role-owner", Name: "owner", Permissions: []string{"finance:read", "users:write"}},
			{ID: maestroRoleID, Name: "maestro", Permissions: maestroPerms},
		},
		invitedUser: client.User{ID: "usr-new", Email: "gabriela@example.com", Status: "invited", RoleIDs: []string{maestroRoleID}},
		invitation:  client.Invitation{URL: "http://localhost:3002/invitacion/tok", Sent: false, Via: "local"},
	}
}

func teacherConfig(t client.Teacher) *fakeTeacherLinkConfig {
	return &fakeTeacherLinkConfig{teachers: map[string]client.Teacher{t.ID: t}}
}

func unlinkedTeacher() client.Teacher {
	return client.Teacher{ID: "tch-1", FirstName: "Gabriela", LastName: "Rojas", Email: "gabriela@example.com"}
}

func doTeacherAccess(t *testing.T, h *handler.TeacherAccessHandler, teacherID string, perms []string) *httptest.ResponseRecorder {
	t.Helper()
	return doTeacherAccessBody(t, h, teacherID, perms, "")
}

// confirmReuseBody is what the web resends once the director confirmed that
// an existing account may become the teacher's.
const confirmReuseBody = `{"confirmReuse": true}`

func doTeacherAccessBody(t *testing.T, h *handler.TeacherAccessHandler, teacherID string, perms []string, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(handler.TeacherAccessRoute, h.GrantAccess)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teachers/"+teacherID+"/access", strings.NewReader(body))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
		UserID: "usr-admin", TenantID: "t1", Permissions: perms,
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

type teacherAccessBody struct {
	Teacher struct {
		ID        string `json:"id"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Email     string `json:"email"`
		UserID    string `json:"userId"`
	} `json:"teacher"`
	UserID     string `json:"userId"`
	Email      string `json:"email"`
	Status     string `json:"status"`
	Invitation *struct {
		URL  string `json:"url"`
		Sent bool   `json:"sent"`
		Via  string `json:"via"`
	} `json:"invitation"`
}

func decodeTeacherAccess(t *testing.T, rec *httptest.ResponseRecorder) teacherAccessBody {
	t.Helper()
	var out teacherAccessBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return out
}

// --- happy paths -------------------------------------------------------------

// A teacher with no account: one is invited into maestro, linked to the
// teacher, and — in local, where nothing is mailed — the link comes back.
func TestTeacherAccessInvitesAndLinksANewAccount(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "gabriela@example.com", iam.invitedEmail)
	assert.Equal(t, "Gabriela Rojas", iam.invitedName)
	assert.Equal(t, []string{maestroRoleID}, iam.invitedRoleIDs)
	assert.Equal(t, "tch-1", cfg.linkedTeacherID)
	assert.Equal(t, "usr-new", cfg.linkedUserID)

	out := decodeTeacherAccess(t, rec)
	assert.Equal(t, "usr-new", out.Teacher.UserID)
	assert.Equal(t, "Gabriela", out.Teacher.FirstName)
	assert.Equal(t, "usr-new", out.UserID)
	assert.Equal(t, "gabriela@example.com", out.Email)
	assert.Equal(t, "invited", out.Status)
	require.NotNil(t, out.Invitation)
	assert.False(t, out.Invitation.Sent)
	assert.Equal(t, "http://localhost:3002/invitacion/tok", out.Invitation.URL)
}

// An account already exists for the teacher's address and the director
// confirmed the reuse: it is reused, not duplicated, and given the maestro role
// it lacked. It is active, so nothing is sent.
func TestTeacherAccessReusesAnActiveAccountByEmail(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	iam.users = []client.User{{ID: "usr-old", Email: "Gabriela@Example.com ", Status: "active", RoleIDs: []string{}}}
	rec := doTeacherAccessBody(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller, confirmReuseBody)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Zero(t, iam.inviteCalls, "an existing account is never duplicated")
	assert.Zero(t, iam.resendCalls, "an active account has nothing to redeem")
	assert.Equal(t, "usr-old", iam.assignedUserID)
	assert.Equal(t, maestroRoleID, iam.assignedRoleID)
	assert.Equal(t, "usr-old", cfg.linkedUserID)

	out := decodeTeacherAccess(t, rec)
	assert.Equal(t, "usr-old", out.UserID)
	assert.Equal(t, "active", out.Status)
	assert.Nil(t, out.Invitation)
}

// Pressing the button again for a teacher already linked to an account that
// never accepted: the invitation is sent again, nothing else changes.
func TestTeacherAccessAlreadyLinkedAndInvitedResends(t *testing.T) {
	linked := unlinkedTeacher()
	linked.UserID = "usr-new"
	iam, cfg := teacherIAM(), teacherConfig(linked)
	iam.users = []client.User{{ID: "usr-new", Email: "gabriela@example.com", Status: "invited", RoleIDs: []string{maestroRoleID}}}
	iam.resent = client.Invitation{URL: "http://localhost:3002/invitacion/tok2", Sent: false}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, iam.resendCalls)
	assert.Equal(t, "usr-new", iam.resentUserID)
	assert.Zero(t, iam.inviteCalls)
	assert.Zero(t, iam.assignCalls, "the role is already held")
	assert.Zero(t, cfg.linkCalls, "the link already exists")

	out := decodeTeacherAccess(t, rec)
	assert.Equal(t, "invited", out.Status)
	assert.Equal(t, "usr-new", out.Teacher.UserID)
	require.NotNil(t, out.Invitation)
	assert.Equal(t, "http://localhost:3002/invitacion/tok2", out.Invitation.URL)
}

// Already linked to an active account: a no-op 200 that says so.
func TestTeacherAccessAlreadyLinkedAndActiveIsANoOp(t *testing.T) {
	linked := unlinkedTeacher()
	linked.UserID = "usr-new"
	iam, cfg := teacherIAM(), teacherConfig(linked)
	iam.users = []client.User{{ID: "usr-new", Email: "gabriela@example.com", Status: "active", RoleIDs: []string{maestroRoleID}}}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Zero(t, iam.writes())
	assert.Zero(t, cfg.linkCalls)
	out := decodeTeacherAccess(t, rec)
	assert.Equal(t, "active", out.Status)
	assert.Nil(t, out.Invitation)
}

// --- refusals ------------------------------------------------------------------

func TestTeacherAccessRefusals(t *testing.T) {
	noEmail := unlinkedTeacher()
	noEmail.Email = "  "

	tests := []struct {
		name     string
		teacher  client.Teacher
		target   string
		perms    []string
		setup    func(iam *fakeTeacherIAM)
		wantCode int
	}{
		{
			name: "teacher without an email", teacher: noEmail, target: "tch-1",
			perms: ownerCaller, wantCode: http.StatusBadRequest,
		},
		{
			// Config's RLS makes another tenant's teacher indistinguishable
			// from a missing one: NotFound → 404.
			name: "teacher of another tenant (or none)", teacher: unlinkedTeacher(), target: "tch-other-tenant",
			perms: ownerCaller, wantCode: http.StatusNotFound,
		},
		{
			// The caller-subset guard: an admin holding teachers:write and
			// users:write but not attendance_own:write may not mint a maestro.
			name: "caller lacks a maestro permission", teacher: unlinkedTeacher(), target: "tch-1",
			perms:    []string{"teachers:write", "users:write", "teaching:read", "teaching:write", "attendance_own:read"},
			wantCode: http.StatusForbidden,
		},
		{
			// Resending hands over an account: the reach guard applies. The
			// address belongs to an invited account with a role the caller
			// does not hold (finance:read), so its link is not the caller's to pull.
			name: "existing invited account holds more than the caller", teacher: unlinkedTeacher(), target: "tch-1",
			perms: append([]string{"teachers:write", "users:write"}, maestroPerms...),
			setup: func(iam *fakeTeacherIAM) {
				iam.users = []client.User{{ID: "usr-boss", Email: "gabriela@example.com", Status: "invited", RoleIDs: []string{"role-owner"}}}
			},
			wantCode: http.StatusForbidden,
		},
		{
			name: "the maestro role is missing from the deployment", teacher: unlinkedTeacher(), target: "tch-1",
			perms: ownerCaller,
			setup: func(iam *fakeTeacherIAM) {
				iam.roles = iam.roles[:1]
			},
			wantCode: http.StatusInternalServerError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iam, cfg := teacherIAM(), teacherConfig(tc.teacher)
			if tc.setup != nil {
				tc.setup(iam)
			}
			rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), tc.target, tc.perms)
			assert.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
			assert.Zero(t, iam.writes(), "a refused request writes nothing to iam")
			assert.Zero(t, cfg.linkCalls, "a refused request links nothing")
		})
	}
}

func TestTeacherAccessWithoutEmailSaysSoInSpanish(t *testing.T) {
	noEmail := unlinkedTeacher()
	noEmail.Email = ""
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(teacherIAM(), teacherConfig(noEmail)), "tch-1", ownerCaller)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "El docente no tiene correo")
}

// The account is made before the link; a failed link surfaces its status, and
// pressing again reuses the account by address instead of minting a second.
func TestTeacherAccessPropagatesALinkConflict(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	cfg.linkErr = connect.NewError(connect.CodeAlreadyExists, errors.New("user already linked to another teacher"))
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// Every refusal happens before the first write: an address whose account is
// already the account of ANOTHER teacher is a 409 decided up front, not the
// link conflict config raises after iam has already been written to.
func TestTeacherAccessAccountOfAnotherTeacherIsRefusedBeforeAnyWrite(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	iam.users = []client.User{{ID: "usr-other", Email: "gabriela@example.com", Status: "invited", RoleIDs: []string{maestroRoleID}}}
	cfg.teachers["tch-2"] = client.Teacher{ID: "tch-2", FirstName: "Otra", Email: "gabriela@example.com", UserID: "usr-other"}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Zero(t, iam.writes(), "nothing is written to iam")
	assert.Zero(t, cfg.linkCalls)
}

// A link to an account iam no longer has is no link: the teacher is treated as
// unlinked — a fresh invitation, and the link is rewritten to the new account.
func TestTeacherAccessStaleLinkInvitesAndRelinks(t *testing.T) {
	stale := unlinkedTeacher()
	stale.UserID = "usr-gone"
	iam, cfg := teacherIAM(), teacherConfig(stale)
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, iam.inviteCalls)
	assert.Equal(t, 1, cfg.linkCalls, "the stale link is rewritten")
	assert.Equal(t, "usr-new", cfg.linkedUserID)
	assert.Equal(t, "usr-new", decodeTeacherAccess(t, rec).Teacher.UserID)
}

// The stale link falls through to the address: an account already there (a
// maestro one, so no confirmation is due) is reused and relinked.
func TestTeacherAccessStaleLinkReusesTheAccountByEmail(t *testing.T) {
	stale := unlinkedTeacher()
	stale.UserID = "usr-gone"
	iam, cfg := teacherIAM(), teacherConfig(stale)
	iam.users = []client.User{{ID: "usr-m", Email: "gabriela@example.com", Status: "active", RoleIDs: []string{maestroRoleID}}}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Zero(t, iam.writes())
	assert.Equal(t, "usr-m", cfg.linkedUserID)
}

// T2: a teacher linked to a DELETED account is given access by inviting the
// address again — which, in iam, brings that same account back (owner answer
// 3). Adding the maestro role to a deleted account would change nothing: it
// still could not sign in.
func TestTeacherAccessLinkedToADeletedAccountInvitesItBack(t *testing.T) {
	linked := unlinkedTeacher()
	linked.UserID = "usr-del"
	iam, cfg := teacherIAM(), teacherConfig(linked)
	iam.users = []client.User{{ID: "usr-del", Email: "gabriela@example.com", Status: "deleted", RoleIDs: []string{maestroRoleID}}}
	iam.invitedUser = client.User{ID: "usr-del", Email: "gabriela@example.com", Status: "invited", RoleIDs: []string{maestroRoleID}}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, iam.inviteCalls, "the address is invited again")
	assert.Equal(t, "gabriela@example.com", iam.invitedEmail)
	assert.Zero(t, iam.assignCalls, "no role is added to a deleted account")
	assert.Equal(t, "usr-del", cfg.linkedUserID, "the teacher keeps the same account")
}

// Only NotFound means "no such account". iam being down is an error, never a
// reason to mint a second account for a teacher who may well have one.
func TestTeacherAccessLinkedLookupFailureIsNotAStaleLink(t *testing.T) {
	linked := unlinkedTeacher()
	linked.UserID = "usr-new"
	iam, cfg := teacherIAM(), teacherConfig(linked)
	iam.getUserErr = connect.NewError(connect.CodeUnavailable, errors.New("iam down"))
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError, rec.Body.String())
	assert.Zero(t, iam.writes())
	assert.Zero(t, cfg.linkCalls)
}

// --- reusing an account that is not a maestro one asks first -----------------

// An address that already signs somebody in as something else (here: the
// owner's own account) is never silently turned into a teacher. The gateway
// answers 409 with a marker the web reads to ask the director, and writes nothing.
func TestTeacherAccessExistingOtherAccountAsksForConfirmation(t *testing.T) {
	for _, status := range []string{"active", "suspended", "invited"} {
		t.Run(status, func(t *testing.T) {
			iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
			iam.users = []client.User{{ID: "usr-boss", Email: "gabriela@example.com", Status: status, RoleIDs: []string{"role-owner"}}}
			rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			assert.Equal(t, "reuse-account", rec.Header().Get("X-Confirm-Required"))
			assert.Contains(t, rec.Body.String(), "Ya existe una cuenta con ese correo (owner). ¿Vincularla y darle acceso de maestro?")
			assert.Zero(t, iam.writes())
			assert.Zero(t, cfg.linkCalls)
		})
	}
}

// Confirmed: the account keeps its roles, gains maestro, and is linked.
func TestTeacherAccessConfirmedReuseLinksAndAddsMaestro(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	iam.users = []client.User{{ID: "usr-boss", Email: "gabriela@example.com", Status: "active", RoleIDs: []string{"role-owner"}}}
	rec := doTeacherAccessBody(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller, confirmReuseBody)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, maestroRoleID, iam.assignedRoleID)
	assert.Equal(t, "usr-boss", cfg.linkedUserID)
	assert.Zero(t, iam.inviteCalls)
}

// Confirming does not widen what the caller may do: linking an account whose
// CURRENT roles carry a permission the caller lacks would hand it over. 403,
// decided before any write, and before the confirmation is even asked for.
func TestTeacherAccessReuseOfABroaderAccountIsForbidden(t *testing.T) {
	notOwner := append([]string{"teachers:write", "users:write"}, maestroPerms...)
	for _, body := range []string{"", confirmReuseBody} {
		t.Run("body="+body, func(t *testing.T) {
			iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
			iam.users = []client.User{{ID: "usr-boss", Email: "gabriela@example.com", Status: "active", RoleIDs: []string{"role-owner"}}}
			rec := doTeacherAccessBody(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", notOwner, body)

			assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			assert.Empty(t, rec.Header().Get("X-Confirm-Required"))
			assert.Zero(t, iam.writes())
			assert.Zero(t, cfg.linkCalls)
		})
	}
}

// An account with no role at all is not a maestro account either: it asks.
func TestTeacherAccessRolelessAccountAsksForConfirmation(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	iam.users = []client.User{{ID: "usr-old", Email: "gabriela@example.com", Status: "active", RoleIDs: []string{}}}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, "reuse-account", rec.Header().Get("X-Confirm-Required"))
	assert.Contains(t, rec.Body.String(), "(sin rol)")
	assert.Zero(t, iam.writes())
}

// An account that is ONLY a maestro (the orphan a failed link leaves behind) is
// this very action's own work: it is reused without asking.
func TestTeacherAccessMaestroOnlyAccountIsReusedWithoutAsking(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	iam.users = []client.User{{ID: "usr-orphan", Email: "gabriela@example.com", Status: "invited", RoleIDs: []string{maestroRoleID}}}
	rec := doTeacherAccess(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, iam.resendCalls)
	assert.Equal(t, "usr-orphan", cfg.linkedUserID)
}

func TestTeacherAccessMalformedBodyIs400(t *testing.T) {
	iam, cfg := teacherIAM(), teacherConfig(unlinkedTeacher())
	rec := doTeacherAccessBody(t, handler.NewTeacherAccessHandler(iam, cfg), "tch-1", ownerCaller, `{"confirmReuse": "yes"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, iam.writes())
}
