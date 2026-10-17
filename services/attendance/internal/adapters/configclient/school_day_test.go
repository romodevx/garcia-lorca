package configclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// calendarConfig serves a group of cycle cyc-1 and that cycle's calendar.
type calendarConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
	day       *configv1.GetSchoolDayResponse // nil = an older config without the RPC
	gotCycle  string
	gotDate   time.Time
	failGroup bool
}

func (c *calendarConfig) GetGroup(_ context.Context, r *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	if c.failGroup || r.Msg.GetId() != "grp-1" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: &configv1.Group{Id: "grp-1", CycleId: "cyc-1"}}), nil
}

func (c *calendarConfig) GetSchoolDay(_ context.Context, r *connect.Request[configv1.GetSchoolDayRequest]) (*connect.Response[configv1.GetSchoolDayResponse], error) {
	if c.day == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, nil)
	}
	c.gotCycle, c.gotDate = r.Msg.GetCycleId(), r.Msg.GetDate().AsTime()
	return connect.NewResponse(c.day), nil
}

func serveCalendar(t *testing.T, c *calendarConfig) *Client {
	t.Helper()
	path, h := configv1connect.NewConfigServiceHandler(c)
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client())
}

// T14: the group's cycle is resolved, then config's calendar answers for it.
func TestSchoolDayOfAsksTheGroupsCycleCalendar(t *testing.T) {
	sat := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		day  *configv1.GetSchoolDayResponse
		want service.SchoolDay
	}{
		{"open", &configv1.GetSchoolDayResponse{Open: true}, service.SchoolDay{Open: true}},
		{"weekend", &configv1.GetSchoolDayResponse{}, service.SchoolDay{}},
		{"holiday", &configv1.GetSchoolDayResponse{ClosedLabel: "Revolución"}, service.SchoolDay{Closed: "Revolución"}},
		{"older config without the RPC keeps saving", nil, service.SchoolDay{Open: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &calendarConfig{day: tt.day}
			got, err := serveCalendar(t, cfg).SchoolDayOf(context.Background(), "grp-1", sat)
			if err != nil {
				t.Fatalf("SchoolDayOf: %v", err)
			}
			if got != tt.want {
				t.Errorf("SchoolDayOf = %+v, want %+v", got, tt.want)
			}
			if tt.day != nil && (cfg.gotCycle != "cyc-1" || !cfg.gotDate.Equal(sat)) {
				t.Errorf("asked cycle %q date %v, want cyc-1 %v", cfg.gotCycle, cfg.gotDate, sat)
			}
		})
	}
}

func TestSchoolDayOfUnknownGroupIsNotFound(t *testing.T) {
	_, err := serveCalendar(t, &calendarConfig{failGroup: true}).SchoolDayOf(context.Background(), "grp-1", time.Now())
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("KindOf(%v) = %v, want NotFound", err, glerrors.KindOf(err))
	}
}
