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

// calendarConfig serves config's school calendar for a group.
type calendarConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
	day        *configv1.GetSchoolDayResponse
	code       connect.Code // non-zero: GetSchoolDay fails with it
	gotGroup   string
	gotDate    time.Time
	groupCalls int
}

func (c *calendarConfig) GetGroup(context.Context, *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	c.groupCalls++
	return connect.NewResponse(&configv1.GetGroupResponse{Group: &configv1.Group{Id: "grp-1", CycleId: "cyc-1"}}), nil
}

func (c *calendarConfig) GetSchoolDay(_ context.Context, r *connect.Request[configv1.GetSchoolDayRequest]) (*connect.Response[configv1.GetSchoolDayResponse], error) {
	c.gotGroup, c.gotDate = r.Msg.GetGroupId(), r.Msg.GetDate().AsTime()
	if c.code != 0 {
		return nil, connect.NewError(c.code, nil)
	}
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

var saturday = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

// T14: ONE round trip — config resolves the group's cycle and answers its rule.
func TestSchoolDayOfAsksConfigOnceForTheGroup(t *testing.T) {
	tests := []struct {
		name     string
		day      *configv1.GetSchoolDayResponse
		code     connect.Code
		want     service.SchoolDay
		wantKind glerrors.Kind // non-zero: an error of this kind
	}{
		{name: "open", day: &configv1.GetSchoolDayResponse{Open: true}, want: service.SchoolDay{Open: true}},
		{name: "weekend", day: &configv1.GetSchoolDayResponse{}, want: service.SchoolDay{}},
		{name: "holiday", day: &configv1.GetSchoolDayResponse{ClosedLabel: "Revolución"}, want: service.SchoolDay{Closed: "Revolución"}},
		{name: "older config without the RPC keeps saving", code: connect.CodeUnimplemented, want: service.SchoolDay{Open: true}},
		{name: "config unavailable is an error, never an open day", code: connect.CodeUnavailable, wantKind: glerrors.KindInternal},
		// The group was validated just before: a NotFound here is config's
		// inconsistency, never a 404 for the caller's save or read.
		{name: "not found is internal, not a 404", code: connect.CodeNotFound, wantKind: glerrors.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &calendarConfig{day: tt.day, code: tt.code}
			got, err := serveCalendar(t, cfg).SchoolDayOf(context.Background(), "grp-1", saturday)
			if tt.wantKind != 0 {
				if glerrors.KindOf(err) != tt.wantKind {
					t.Fatalf("KindOf(%v) = %v, want %v", err, glerrors.KindOf(err), tt.wantKind)
				}
				if got.Open {
					t.Error("a failed lookup reported an open day")
				}
				return
			}
			if err != nil {
				t.Fatalf("SchoolDayOf: %v", err)
			}
			if got != tt.want {
				t.Errorf("SchoolDayOf = %+v, want %+v", got, tt.want)
			}
			if cfg.gotGroup != "grp-1" || !cfg.gotDate.Equal(saturday) {
				t.Errorf("asked group %q date %v, want grp-1 %v", cfg.gotGroup, cfg.gotDate, saturday)
			}
			if cfg.groupCalls != 0 {
				t.Errorf("GetGroup called %d times; the calendar takes the group itself", cfg.groupCalls)
			}
		})
	}
}
