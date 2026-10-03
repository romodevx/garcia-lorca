package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

type stubAbsences struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotCreate *configv1.CreateTeacherAbsenceRequest
	gotUpdate *configv1.UpdateTeacherAbsenceRequest
	gotList   *configv1.ListTeacherAbsencesRequest
}

func protoAbsence() *configv1.TeacherAbsence {
	return &configv1.TeacherAbsence{Id: "abs1", CycleId: "c1", TeacherId: "t1", Kind: "sick", StartsOn: timestamppb.New(mon5),
		EndsOn: timestamppb.New(mon5), StartMinute: 480, EndMinute: 600, Note: "cita",
		CreatedBy: &configv1.ScheduleActor{UserId: "u", Name: "Directora"}, CreatedAt: timestamppb.New(mon5)}
}

func (s *stubAbsences) CreateTeacherAbsence(_ context.Context, req *connect.Request[configv1.CreateTeacherAbsenceRequest]) (*connect.Response[configv1.CreateTeacherAbsenceResponse], error) {
	s.gotCreate = req.Msg
	return connect.NewResponse(&configv1.CreateTeacherAbsenceResponse{Absence: protoAbsence(), UncoveredCount: 4}), nil
}

func (s *stubAbsences) UpdateTeacherAbsence(_ context.Context, req *connect.Request[configv1.UpdateTeacherAbsenceRequest]) (*connect.Response[configv1.UpdateTeacherAbsenceResponse], error) {
	s.gotUpdate = req.Msg
	return connect.NewResponse(&configv1.UpdateTeacherAbsenceResponse{Absence: protoAbsence(), UncoveredCount: 2}), nil
}

func (s *stubAbsences) CancelTeacherAbsence(_ context.Context, req *connect.Request[configv1.CancelTeacherAbsenceRequest]) (*connect.Response[configv1.CancelTeacherAbsenceResponse], error) {
	a := protoAbsence()
	a.CancelledAt, a.CancelledBy, a.Note = timestamppb.New(mon5), req.Msg.GetActor(), req.Msg.GetCallerTeacherId()
	return connect.NewResponse(&configv1.CancelTeacherAbsenceResponse{Absence: a}), nil
}

func (s *stubAbsences) ListTeacherAbsences(_ context.Context, req *connect.Request[configv1.ListTeacherAbsencesRequest]) (*connect.Response[configv1.ListTeacherAbsencesResponse], error) {
	s.gotList = req.Msg
	return connect.NewResponse(&configv1.ListTeacherAbsencesResponse{Absences: []*configv1.TeacherAbsence{protoAbsence()}}), nil
}

func (s *stubAbsences) ListUncoveredClasses(_ context.Context, req *connect.Request[configv1.ListUncoveredClassesRequest]) (*connect.Response[configv1.ListUncoveredClassesResponse], error) {
	return connect.NewResponse(&configv1.ListUncoveredClassesResponse{Classes: []*configv1.UncoveredClass{{
		Date: req.Msg.GetFrom(), ScheduledClass: &configv1.ScheduledClass{Block: protoBlock(), TeacherId: "t1", StartsAt: 480, EndsAt: 530, Status: "uncovered", Absence: protoAbsence()},
	}}}), nil
}

func TestConfigClientTeacherAbsences(t *testing.T) {
	stub := &stubAbsences{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := client.NewConfigClient(srv.URL, srv.Client())
	ctx := context.Background()
	by := client.ScheduleActor{UserID: "u", Name: "Directora"}

	saved, err := c.CreateTeacherAbsence(ctx, client.TeacherAbsenceInput{CycleID: "c1", TeacherID: "t1", Kind: "sick", StartsOn: mon5, EndsOn: mon5,
		StartMinute: 480, EndMinute: 600, Note: "cita"}, by)
	require.NoError(t, err)
	assert.Equal(t, 4, saved.Uncovered)
	assert.Equal(t, int32(600), stub.gotCreate.GetAbsence().GetEndMinute())
	assert.False(t, stub.gotCreate.GetAbsence().GetAllDay())
	assert.Equal(t, "Directora", stub.gotCreate.GetActor().GetName())
	a := saved.Absence
	assert.Equal(t, client.TeacherAbsence{ID: "abs1", CycleID: "c1", TeacherID: "t1", Kind: "sick", StartsOn: mon5, EndsOn: mon5,
		StartMinute: 480, EndMinute: 600, Note: "cita", CreatedBy: by, CreatedAt: mon5}, a)

	upd, err := c.UpdateTeacherAbsence(ctx, "abs1", client.TeacherAbsenceInput{TeacherID: "t1", Kind: "other", StartsOn: mon5, EndsOn: mon5, AllDay: true}, by)
	require.NoError(t, err)
	assert.Equal(t, 2, upd.Uncovered)
	assert.Equal(t, "abs1", stub.gotUpdate.GetId())
	assert.True(t, stub.gotUpdate.GetAbsence().GetAllDay())

	cancelled, err := c.CancelTeacherAbsence(ctx, "abs1", by, "t9")
	require.NoError(t, err)
	assert.Equal(t, mon5, cancelled.CancelledAt)
	assert.Equal(t, "Directora", cancelled.CancelledBy.Name)
	assert.Equal(t, "t9", cancelled.Note, "the caller's teacher travels to config")

	list, err := c.ListTeacherAbsences(ctx, client.TeacherAbsenceQuery{CycleID: "c1", TeacherID: "t1", From: mon5, To: mon5, IncludeCancelled: true})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.True(t, stub.gotList.GetIncludeCancelled())

	unc, err := c.ListUncoveredClasses(ctx, "c1", mon5, mon5)
	require.NoError(t, err)
	require.Len(t, unc, 1)
	assert.Equal(t, mon5, unc[0].Date)
	assert.Equal(t, "uncovered", unc[0].Class.Status)
	require.NotNil(t, unc[0].Class.Absence)
	assert.Equal(t, "cita", unc[0].Class.Absence.Note)
}
