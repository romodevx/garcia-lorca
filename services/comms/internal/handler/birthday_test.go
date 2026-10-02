package handler_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
	"github.com/garcia-lorca/services/comms/internal/handler"
	"github.com/garcia-lorca/services/comms/internal/service"
)

type oneBirthday struct{}

func (oneBirthday) BirthdaysOn(_ context.Context, month, day int, _ bool) ([]service.Pupil, error) {
	if month == 10 && day == 1 {
		return []service.Pupil{{ID: studentID, FirstName: "Ana"}}, nil
	}
	return nil, nil
}

type guardianOf struct{}

func (guardianOf) GuardiansForStudent(context.Context, string) ([]string, error) {
	return []string{userID}, nil
}

type memTemplates struct{ saved *service.BirthdayTemplate }

func (m *memTemplates) BirthdayTemplate(context.Context) (service.BirthdayTemplate, bool, error) {
	if m.saved == nil {
		return service.BirthdayTemplate{}, false, nil
	}
	return *m.saved, true, nil
}

func (m *memTemplates) SaveBirthdayTemplate(_ context.Context, t service.BirthdayTemplate) (service.BirthdayTemplate, error) {
	m.saved = &t
	return t, nil
}

func newBirthdayHandler(t *testing.T, repo *fakeRepo) *handler.CommsHandler {
	t.Helper()
	loc, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewCommsService(repo, guardianOf{}, nil, testTenant,
		func() string { return deviceID },
		func() time.Time { return createdInstant },
		slog.New(slog.DiscardHandler),
		service.WithBirthdays(oneBirthday{}, &memTemplates{}, loc))
	return handler.NewCommsHandler(svc, testTenant)
}

func TestRunBirthdaysReportsTheRun(t *testing.T) {
	repo := &fakeRepo{}
	h := newBirthdayHandler(t, repo)
	res, err := h.RunBirthdays(context.Background(), connect.NewRequest(&commsv1.RunBirthdaysRequest{Date: "2026-10-01"}))
	if err != nil {
		t.Fatalf("RunBirthdays: %v", err)
	}
	m := res.Msg
	if m.GetDate() != "2026-10-01" || m.GetGreeted() != 1 || m.GetAlreadyGreeted() != 0 || m.GetWithoutGuardians() != 0 {
		t.Errorf("response = %+v", m)
	}
	if len(repo.notifications) != 1 || repo.notifications[0].Type != "birthday" {
		t.Errorf("notifications = %+v", repo.notifications)
	}
}

func TestRunBirthdaysMalformedDateIsInvalidArgument(t *testing.T) {
	h := newBirthdayHandler(t, &fakeRepo{})
	_, err := h.RunBirthdays(context.Background(), connect.NewRequest(&commsv1.RunBirthdaysRequest{Date: "ayer"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestBirthdayTemplateRoundTrip(t *testing.T) {
	h := newBirthdayHandler(t, &fakeRepo{})
	ctx := context.Background()

	got, err := h.GetBirthdayTemplate(ctx, connect.NewRequest(&commsv1.GetBirthdayTemplateRequest{}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tpl := got.Msg.GetTemplate(); !tpl.GetIsDefault() || tpl.GetBody() == "" || tpl.GetUpdatedAt() != nil {
		t.Errorf("default = %+v", tpl)
	}

	saved, err := h.UpdateBirthdayTemplate(ctx, connect.NewRequest(&commsv1.UpdateBirthdayTemplateRequest{
		Title: "¡Feliz día!", Body: "Felicidades, {nombre}.", UpdatedBy: userID,
	}))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	tpl := saved.Msg.GetTemplate()
	if tpl.GetIsDefault() || tpl.GetBody() != "Felicidades, {nombre}." || tpl.GetUpdatedBy() != userID || !tpl.GetUpdatedAt().AsTime().Equal(createdInstant) {
		t.Errorf("saved = %+v", tpl)
	}

	_, err = h.UpdateBirthdayTemplate(ctx, connect.NewRequest(&commsv1.UpdateBirthdayTemplateRequest{Title: "x", Body: " ", UpdatedBy: userID}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("empty body code = %v", connect.CodeOf(err))
	}
}
