package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func (f *fakeCatalog) ApplyTermMonths(_ context.Context, creates, updates []domain.Term) error {
	for _, t := range creates {
		if err := f.claim("term|" + t.CycleID() + "|" + t.LevelID() + "|" + itoa(t.Ordinal())); err != nil {
			return err
		}
		f.terms[t.ID()] = t
	}
	for _, t := range updates {
		f.terms[t.ID()] = t
	}
	return nil
}

func seedTermVia(t *testing.T, h *grpcadapter.ConfigHandler, cycleID, levelID string, ordinal int32) string {
	t.Helper()
	res, err := h.CreateTerm(ctx(), connect.NewRequest(&configv1.CreateTermRequest{
		CycleId: cycleID, LevelId: levelID, Name: "Trimestre", Ordinal: ordinal,
		StartsOn: timestamppb.New(someDate), EndsOn: timestamppb.New(laterDate),
	}))
	require.NoError(t, err)
	return res.Msg.GetTerm().GetId()
}

func TestSetTermMonthsRoundTripsThroughEveryTermRead(t *testing.T) {
	h, _ := newHandler(t)
	cycleID, levelID := seedCycle(t, h), seedLevel(t, h)
	termID := seedTermVia(t, h, cycleID, levelID, 1)

	res, err := h.SetTermMonths(ctx(), connect.NewRequest(&configv1.SetTermMonthsRequest{
		TermId: termID, Months: []string{"2026-10", "2026-09"},
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09", "2026-10"}, res.Msg.GetTerm().GetMonths())

	got, err := h.GetTerm(ctx(), connect.NewRequest(&configv1.GetTermRequest{Id: termID}))
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09", "2026-10"}, got.Msg.GetTerm().GetMonths())

	list, err := h.ListTerms(ctx(), connect.NewRequest(&configv1.ListTermsRequest{CycleId: cycleID}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetTerms(), 1)
	assert.Equal(t, []string{"2026-09", "2026-10"}, list.Msg.GetTerms()[0].GetMonths())
}

func TestSetTermMonthsRefusalsCarryTheirCode(t *testing.T) {
	h, _ := newHandler(t)
	cycleID, levelID := seedCycle(t, h), seedLevel(t, h)
	t1 := seedTermVia(t, h, cycleID, levelID, 1)
	t2 := seedTermVia(t, h, cycleID, levelID, 2)
	_, err := h.SetTermMonths(ctx(), connect.NewRequest(&configv1.SetTermMonthsRequest{TermId: t2, Months: []string{"2026-12"}}))
	require.NoError(t, err)

	tests := []struct {
		name     string
		months   []string
		wantCode connect.Code
		wantMeta string
		wantMsg  string
	}{
		{"a month a sibling holds", []string{"2026-12"}, connect.CodeAlreadyExists, "MONTH_IN_OTHER_TERM", "Ese mes ya pertenece a otro periodo del nivel: 2026-12"},
		{"a month outside the cycle", []string{"2030-01"}, connect.CodeInvalidArgument, "MONTH_OUTSIDE_CYCLE", "El mes está fuera del ciclo: 2030-01"},
		{"a malformed month", []string{"2026-13"}, connect.CodeInvalidArgument, "", "Mes inválido (usa AAAA-MM): 2026-13"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.SetTermMonths(ctx(), connect.NewRequest(&configv1.SetTermMonthsRequest{TermId: t1, Months: tt.months}))
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, tt.wantCode, ce.Code())
			assert.Equal(t, tt.wantMeta, ce.Meta().Get(grpcadapter.ErrorCodeMeta))
			assert.Equal(t, tt.wantMsg, ce.Message())
		})
	}
}

func TestCopyTermMonthsReturnsTheTargetTerms(t *testing.T) {
	h, _ := newHandler(t)
	cycleID, from := seedCycle(t, h), seedLevel(t, h)
	lvl, err := h.CreateLevel(ctx(), connect.NewRequest(&configv1.CreateLevelRequest{
		Name: "Secundaria", SepCode: "SEP-SEC",
		EnrollmentGranularity: configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE,
	}))
	require.NoError(t, err)
	to := lvl.Msg.GetLevel().GetId()
	src := seedTermVia(t, h, cycleID, from, 1)
	_, err = h.SetTermMonths(ctx(), connect.NewRequest(&configv1.SetTermMonthsRequest{TermId: src, Months: []string{"2026-09"}}))
	require.NoError(t, err)

	res, err := h.CopyTermMonths(ctx(), connect.NewRequest(&configv1.CopyTermMonthsRequest{
		CycleId: cycleID, FromLevelId: from, ToLevelIds: []string{to},
	}))
	require.NoError(t, err)
	require.Len(t, res.Msg.GetTerms(), 1)
	got := res.Msg.GetTerms()[0]
	assert.Equal(t, to, got.GetLevelId())
	assert.Equal(t, int32(1), got.GetOrdinal())
	assert.Equal(t, []string{"2026-09"}, got.GetMonths())

	_, err = h.CopyTermMonths(ctx(), connect.NewRequest(&configv1.CopyTermMonthsRequest{
		CycleId: cycleID, FromLevelId: from, ToLevelIds: []string{from},
	}))
	assert.Equal(t, connect.CodeInvalidArgument, codeOf(err))
}
