package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// stubTermMonths answers the two term-months RPCs over a real Connect
// round-trip, echoing what it was sent.
type stubTermMonths struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotCopy *configv1.CopyTermMonthsRequest
}

func (s *stubTermMonths) SetTermMonths(_ context.Context, req *connect.Request[configv1.SetTermMonthsRequest]) (*connect.Response[configv1.SetTermMonthsResponse], error) {
	if req.Msg.GetTermId() == "clash" {
		ce := connect.NewError(connect.CodeAlreadyExists, errors.New("Ese mes ya pertenece a otro periodo del nivel: 2026-09"))
		ce.Meta().Set("X-Error-Code", "MONTH_IN_OTHER_TERM")
		return nil, ce
	}
	return connect.NewResponse(&configv1.SetTermMonthsResponse{Term: &configv1.Term{
		Id: req.Msg.GetTermId(), Name: "Trimestre 1", Ordinal: 1, Months: req.Msg.GetMonths(),
	}}), nil
}

func (s *stubTermMonths) CopyTermMonths(_ context.Context, req *connect.Request[configv1.CopyTermMonthsRequest]) (*connect.Response[configv1.CopyTermMonthsResponse], error) {
	s.gotCopy = req.Msg
	return connect.NewResponse(&configv1.CopyTermMonthsResponse{Terms: []*configv1.Term{
		{Id: "t-9", LevelId: req.Msg.GetToLevelIds()[0], Ordinal: 1, Months: []string{"2026-09"}},
	}}), nil
}

func newTermMonthsStub(t *testing.T) (*client.ConfigClient, *stubTermMonths) {
	t.Helper()
	stub := &stubTermMonths{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client.NewConfigClient(srv.URL, srv.Client()), stub
}

func TestConfigClientSetTermMonths(t *testing.T) {
	c, _ := newTermMonthsStub(t)
	got, err := c.SetTermMonths(context.Background(), "t-1", []string{"2026-09", "2026-10"})
	require.NoError(t, err)
	assert.Equal(t, "t-1", got.ID)
	assert.Equal(t, []string{"2026-09", "2026-10"}, got.Months)

	_, err = c.SetTermMonths(context.Background(), "clash", []string{"2026-09"})
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeAlreadyExists, ce.Code())
	assert.Equal(t, "MONTH_IN_OTHER_TERM", ce.Meta().Get("X-Error-Code"), "the refusal code survives the hop")
}

func TestConfigClientCopyTermMonths(t *testing.T) {
	c, stub := newTermMonthsStub(t)
	got, err := c.CopyTermMonths(context.Background(), "cyc-1", "lvl-1", []string{"lvl-2", "lvl-3"})
	require.NoError(t, err)
	assert.Equal(t, "cyc-1", stub.gotCopy.GetCycleId())
	assert.Equal(t, "lvl-1", stub.gotCopy.GetFromLevelId())
	assert.Equal(t, []string{"lvl-2", "lvl-3"}, stub.gotCopy.GetToLevelIds())
	require.Len(t, got, 1)
	assert.Equal(t, []string{"2026-09"}, got[0].Months)
}
