package grpcadapter_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// The scholarship handlers translate only; these drive a real
// ScholarshipService over a tiny in-memory repository and read the wire, both
// directions (checklist #2).

type memScholarships struct {
	scholarships map[string]domain.Scholarship
	holdings     []domain.ScholarshipHolding
}

var _ ports.ScholarshipRepository = (*memScholarships)(nil)

func (m *memScholarships) CreateScholarship(_ context.Context, s domain.Scholarship) (domain.Scholarship, error) {
	m.scholarships[s.ID()] = s
	return s, nil
}

func (m *memScholarships) UpdateScholarship(_ context.Context, s domain.Scholarship) (domain.Scholarship, error) {
	m.scholarships[s.ID()] = s
	return s, nil
}

func (m *memScholarships) GetScholarship(_ context.Context, id string) (domain.Scholarship, error) {
	s, ok := m.scholarships[id]
	if !ok {
		return domain.Scholarship{}, glerrors.ErrNotFound
	}
	return s, nil
}

func (m *memScholarships) ListScholarships(context.Context, bool) ([]domain.Scholarship, error) {
	out := make([]domain.Scholarship, 0, len(m.scholarships))
	for _, s := range m.scholarships {
		out = append(out, s)
	}
	return out, nil
}

func (m *memScholarships) CountHolders(context.Context) (map[string]int, error) {
	out := map[string]int{}
	for _, h := range m.holdings {
		out[h.ScholarshipID]++
	}
	return out, nil
}

func (m *memScholarships) ListHolders(context.Context, string) ([]domain.ScholarshipHolding, error) {
	return m.holdings, nil
}

func (m *memScholarships) HeldBy(context.Context, []string) (map[string][]domain.HeldScholarship, error) {
	out := map[string][]domain.HeldScholarship{}
	for _, h := range m.holdings {
		out[h.StudentID] = append(out[h.StudentID], domain.HeldScholarship{Scholarship: m.scholarships[h.ScholarshipID], Since: h.StartsOn})
	}
	return out, nil
}

func (m *memScholarships) Assign(_ context.Context, hs []domain.ScholarshipHolding) ([]domain.ScholarshipHolding, error) {
	m.holdings = append(m.holdings, hs...)
	return hs, nil
}

func (m *memScholarships) Unassign(_ context.Context, _ string, ids []string, _ time.Time) (int, error) {
	return len(ids), nil
}

func (m *memScholarships) FreezeScholarships(context.Context, []domain.Charge) (int, error) {
	return 0, nil
}

type noLedger struct{}

func (noLedger) FreezeSettledScholarships(context.Context, []string) error { return nil }

type noConcepts struct{}

func (noConcepts) ListConcepts(context.Context, bool) ([]domain.ChargeConcept, error) {
	return nil, nil
}

func scholarshipHandler() *grpcadapter.FinanceHandler {
	n := 0
	ids := func() string { n++; return fmt.Sprintf("id-%d", n) }
	svc := usecase.NewScholarshipService(&memScholarships{scholarships: map[string]domain.Scholarship{}}, noLedger{},
		noConcepts{}, &memActivities{activities: map[string]domain.Activity{}}, knownStudents{}, testTenant, ids,
		usecase.NewSchoolClock(func() time.Time { return fixedNow }, schoolZone))
	return grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance")).WithScholarships(svc)
}

func TestScholarshipRoundTripsTheWire(t *testing.T) {
	h := scholarshipHandler()
	ctx := context.Background()
	in := &financev1.ScholarshipInput{Name: "Hermanos", Kind: "percent", Value: 5000, Targets: []string{"tuition", "enrollment"}, Active: true}
	created, err := h.CreateScholarship(ctx, connect.NewRequest(&financev1.CreateScholarshipRequest{Scholarship: in}))
	if err != nil {
		t.Fatalf("CreateScholarship: %v", err)
	}
	s := created.Msg.GetScholarship()
	if s.GetId() == "" || s.GetName() != "Hermanos" || s.GetKind() != "percent" || s.GetValue() != 5000 ||
		len(s.GetTargets()) != 2 || s.GetTargets()[1] != "enrollment" || !s.GetActive() {
		t.Fatalf("scholarship on the wire = %+v", s)
	}

	in.Kind, in.Value, in.Targets = "fixed_price", 100000, []string{"tuition"}
	updated, err := h.UpdateScholarship(ctx, connect.NewRequest(&financev1.UpdateScholarshipRequest{Id: s.GetId(), Scholarship: in}))
	if err != nil {
		t.Fatalf("UpdateScholarship: %v", err)
	}
	if u := updated.Msg.GetScholarship(); u.GetKind() != "fixed_price" || u.GetValue() != 100000 || len(u.GetTargets()) != 1 {
		t.Fatalf("updated = %+v", u)
	}

	assigned, err := h.AssignScholarship(ctx, connect.NewRequest(&financev1.AssignScholarshipRequest{
		ScholarshipId: s.GetId(), StudentIds: []string{"stu-1"},
	}))
	if err != nil {
		t.Fatalf("AssignScholarship: %v", err)
	}
	if hs := assigned.Msg.GetHolders(); len(hs) != 1 || hs[0].GetStudentId() != "stu-1" || hs[0].GetStartsOn() != fixedNow.Format(time.DateOnly) {
		t.Fatalf("holders = %+v", hs)
	}
	listed, err := h.ListScholarships(ctx, connect.NewRequest(&financev1.ListScholarshipsRequest{}))
	if err != nil || len(listed.Msg.GetScholarships()) != 1 || listed.Msg.GetScholarships()[0].GetAssignedCount() != 1 {
		t.Fatalf("ListScholarships = %+v, %v", listed, err)
	}
	targets, err := h.ListScholarshipTargets(ctx, connect.NewRequest(&financev1.ListScholarshipTargetsRequest{}))
	if err != nil || len(targets.Msg.GetTargets()) != 2 || targets.Msg.GetTargets()[0].GetKey() != "enrollment" ||
		targets.Msg.GetTargets()[0].GetGroup() != "enrollment" || targets.Msg.GetTargets()[0].GetLabel() == "" {
		t.Fatalf("targets = %+v, %v", targets, err)
	}
	removed, err := h.UnassignScholarship(ctx, connect.NewRequest(&financev1.UnassignScholarshipRequest{
		ScholarshipId: s.GetId(), StudentIds: []string{"stu-1"},
	}))
	if err != nil || removed.Msg.GetRemoved() != 1 {
		t.Fatalf("Unassign = %+v, %v", removed, err)
	}
}

func TestScholarshipHandlerRefusesBadInput(t *testing.T) {
	h := scholarshipHandler()
	ctx := context.Background()
	bad := &financev1.ScholarshipInput{Name: "Mala", Kind: "percent", Value: 10001, Targets: []string{"tuition"}}
	if _, err := h.CreateScholarship(ctx, connect.NewRequest(&financev1.CreateScholarshipRequest{Scholarship: bad})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("over 100 %%: %v", err)
	}
	if _, err := h.GetScholarship(ctx, connect.NewRequest(&financev1.GetScholarshipRequest{Id: "nope"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("unknown: %v", err)
	}
}

func TestScholarshipRPCsAreUnimplementedWhenUnwired(t *testing.T) {
	h := grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance"))
	if _, err := h.ListScholarships(context.Background(), connect.NewRequest(&financev1.ListScholarshipsRequest{})); connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("unwired: %v", err)
	}
}
