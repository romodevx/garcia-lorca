package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// stubConfig implements the generated config handler with canned SetTeacherUser
// behavior, so the ConfigClient is tested over a REAL Connect round-trip.
type stubConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
}

func (stubConfig) SetTeacherUser(_ context.Context, req *connect.Request[configv1.SetTeacherUserRequest]) (*connect.Response[configv1.SetTeacherUserResponse], error) {
	if req.Msg.GetTeacherId() == "ghost" {
		return nil, connect.NewError(connect.CodeNotFound, errString("teacher not found"))
	}
	return connect.NewResponse(&configv1.SetTeacherUserResponse{Teacher: &configv1.Teacher{
		Id:     req.Msg.GetTeacherId(),
		UserId: req.Msg.GetUserId(),
	}}), nil
}

func newConfigStub(t *testing.T) (*client.ConfigClient, func()) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := configv1connect.NewConfigServiceHandler(stubConfig{})
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	return client.NewConfigClient(srv.URL, srv.Client()), srv.Close
}

func TestConfigClientSetTeacherUserLink(t *testing.T) {
	c, stop := newConfigStub(t)
	defer stop()

	tch, err := c.SetTeacherUser(context.Background(), "tch-1", "usr-1")
	if err != nil {
		t.Fatalf("SetTeacherUser: %v", err)
	}
	if tch.ID != "tch-1" || tch.UserID != "usr-1" {
		t.Errorf("teacher mismatch: %+v", tch)
	}
}

func TestConfigClientSetTeacherUserUnlink(t *testing.T) {
	c, stop := newConfigStub(t)
	defer stop()

	tch, err := c.SetTeacherUser(context.Background(), "tch-1", "")
	if err != nil {
		t.Fatalf("SetTeacherUser unlink: %v", err)
	}
	if tch.UserID != "" {
		t.Errorf("want unlinked, got userID=%q", tch.UserID)
	}
}

func TestConfigClientSetTeacherUserNotFound(t *testing.T) {
	c, stop := newConfigStub(t)
	defer stop()

	if _, err := c.SetTeacherUser(context.Background(), "ghost", "usr-1"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func (stubConfig) GetTeacher(_ context.Context, req *connect.Request[configv1.GetTeacherRequest]) (*connect.Response[configv1.GetTeacherResponse], error) {
	if req.Msg.GetId() == "ghost" {
		return nil, connect.NewError(connect.CodeNotFound, errString("teacher not found"))
	}
	return connect.NewResponse(&configv1.GetTeacherResponse{Teacher: &configv1.Teacher{
		Id: req.Msg.GetId(), FirstName: "Gabriela", LastName: "Rojas", Email: "gabriela@example.com", UserId: "usr-9",
	}}), nil
}

func TestConfigClientGetTeacher(t *testing.T) {
	c, stop := newConfigStub(t)
	defer stop()

	tch, err := c.GetTeacher(context.Background(), "tch-1")
	if err != nil {
		t.Fatalf("GetTeacher: %v", err)
	}
	want := client.Teacher{ID: "tch-1", FirstName: "Gabriela", LastName: "Rojas", Email: "gabriela@example.com", UserID: "usr-9"}
	if tch != want {
		t.Errorf("teacher = %+v, want %+v", tch, want)
	}
	if _, err := c.GetTeacher(context.Background(), "ghost"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("ghost: code = %v, want NotFound", connect.CodeOf(err))
	}
}
