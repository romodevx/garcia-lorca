package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// stubPeriods answers the evaluation-period RPCs over a real Connect
// round-trip, recording what it was sent.
type stubPeriods struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotList   *configv1.ListEvaluationPeriodsRequest
	gotUpsert *configv1.UpsertEvaluationPeriodsRequest
	gotCopy   *configv1.CopyEvaluationPeriodsRequest
}

var (
	sept1 = time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	nov30 = time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)
)

func protoPeriod(grade string) *configv1.EvaluationPeriod {
	return &configv1.EvaluationPeriod{
		Id: "p-1", CycleId: "cyc-1", GradeLevelId: grade, Ordinal: 1, Name: "T1",
		StartsOn: timestamppb.New(sept1), EndsOn: timestamppb.New(nov30), TermId: "trm-1",
	}
}

func (s *stubPeriods) ListEvaluationPeriods(_ context.Context, req *connect.Request[configv1.ListEvaluationPeriodsRequest]) (*connect.Response[configv1.ListEvaluationPeriodsResponse], error) {
	s.gotList = req.Msg
	return connect.NewResponse(&configv1.ListEvaluationPeriodsResponse{Periods: []*configv1.EvaluationPeriod{protoPeriod("g-1")}}), nil
}

func (s *stubPeriods) UpsertEvaluationPeriods(_ context.Context, req *connect.Request[configv1.UpsertEvaluationPeriodsRequest]) (*connect.Response[configv1.UpsertEvaluationPeriodsResponse], error) {
	s.gotUpsert = req.Msg
	if req.Msg.GetGradeLevelId() == "clash" {
		ce := connect.NewError(connect.CodeAlreadyExists, errors.New("Los periodos de un grado no pueden encimarse"))
		ce.Meta().Set("X-Error-Code", "PERIOD_OVERLAP")
		return nil, ce
	}
	return connect.NewResponse(&configv1.UpsertEvaluationPeriodsResponse{Periods: []*configv1.EvaluationPeriod{protoPeriod(req.Msg.GetGradeLevelId())}}), nil
}

func (s *stubPeriods) CopyEvaluationPeriods(_ context.Context, req *connect.Request[configv1.CopyEvaluationPeriodsRequest]) (*connect.Response[configv1.CopyEvaluationPeriodsResponse], error) {
	s.gotCopy = req.Msg
	return connect.NewResponse(&configv1.CopyEvaluationPeriodsResponse{Periods: []*configv1.EvaluationPeriod{protoPeriod(req.Msg.GetToGradeLevelIds()[0])}}), nil
}

func newPeriodsStub(t *testing.T) (*client.ConfigClient, *stubPeriods) {
	t.Helper()
	stub := &stubPeriods{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client.NewConfigClient(srv.URL, srv.Client()), stub
}

func TestConfigClientEvaluationPeriods(t *testing.T) {
	c, stub := newPeriodsStub(t)
	ctx := context.Background()

	list, err := c.ListEvaluationPeriods(ctx, "cyc-1", "lvl-1", "")
	require.NoError(t, err)
	assert.Equal(t, "lvl-1", stub.gotList.GetLevelId())
	require.Len(t, list, 1)
	assert.Equal(t, client.EvaluationPeriod{
		ID: "p-1", CycleID: "cyc-1", GradeLevelID: "g-1", Ordinal: 1, Name: "T1", StartsOn: sept1, EndsOn: nov30, TermID: "trm-1",
	}, list[0])

	up, err := c.UpsertEvaluationPeriods(ctx, "cyc-1", "g-2", []client.EvaluationPeriodInput{{Ordinal: 1, Name: "T1", StartsOn: sept1, EndsOn: nov30}})
	require.NoError(t, err)
	require.Len(t, stub.gotUpsert.GetPeriods(), 1)
	assert.True(t, stub.gotUpsert.GetPeriods()[0].GetEndsOn().AsTime().Equal(nov30))
	assert.Equal(t, "g-2", up[0].GradeLevelID)

	_, err = c.UpsertEvaluationPeriods(ctx, "cyc-1", "clash", nil)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "PERIOD_OVERLAP", ce.Meta().Get("X-Error-Code"), "the refusal code survives the hop")

	cp, err := c.CopyEvaluationPeriods(ctx, "cyc-1", "g-1", []string{"g-3", "g-4"})
	require.NoError(t, err)
	assert.Equal(t, []string{"g-3", "g-4"}, stub.gotCopy.GetToGradeLevelIds())
	assert.Equal(t, "g-3", cp[0].GradeLevelID)
}
