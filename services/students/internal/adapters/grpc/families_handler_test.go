package grpcadapter_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const (
	wireFamily = "fa000000-0000-0000-0000-000000000001"
	wirePupilA = "5a000000-0000-0000-0000-00000000000a"
	wirePupilB = "5a000000-0000-0000-0000-00000000000b"
	wireActor  = "a0000000-0000-0000-0000-000000000001"
)

var wireFamilyAt = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// memFamilies is an in-memory FamilyRepository over the handler's pupils.
type memFamilies struct {
	*stubAccess
	pupils    *fakeRepo
	byID      map[string]domain.Family
	inputs    ports.SuggestionInputs
	dismissed []ports.FamilyDismissal
}

func (m *memFamilies) ListFamilies(context.Context) ([]domain.Family, error) {
	out := make([]domain.Family, 0, len(m.byID))
	for _, f := range m.byID {
		out = append(out, f)
	}
	return out, nil
}

func (m *memFamilies) FamilyByID(_ context.Context, id string) (domain.Family, error) {
	f, ok := m.byID[id]
	if !ok {
		return domain.Family{}, glerrors.ErrNotFound
	}
	return domain.RestoreFamily(domain.FamilyParams{
		ID: f.ID(), TenantID: f.TenantID(), Name: f.Name(), StudentIDs: f.StudentIDs(), AccountCount: 1, CreatedAt: f.CreatedAt(),
	}), nil
}

func (m *memFamilies) CreateFamily(ctx context.Context, f domain.Family, sync ports.FamilyAccessFunc) error {
	if err := m.runSync(ctx, f.ID(), sync); err != nil {
		return err
	}
	m.byID[f.ID()] = f
	for _, id := range f.StudentIDs() {
		m.pupils.byID[id].AssignFamily(f.ID())
	}
	return nil
}

func (m *memFamilies) RenameFamily(_ context.Context, id, name string) error {
	f := m.byID[id]
	renamed, err := f.Rename(name)
	m.byID[id] = renamed
	return err
}

func (m *memFamilies) AddStudentsToFamily(context.Context, string, []string, ports.FamilyAccessFunc) error {
	return glerrors.New(glerrors.KindConflict, "a pupil is already in a family")
}

func (m *memFamilies) RemoveStudentFromFamily(_ context.Context, familyID, _ string) (bool, error) {
	if _, ok := m.byID[familyID]; !ok {
		return false, glerrors.ErrNotFound
	}
	delete(m.byID, familyID)
	return true, nil
}

func (m *memFamilies) DissolveFamily(_ context.Context, id string) error {
	if _, ok := m.byID[id]; !ok {
		return glerrors.ErrNotFound
	}
	delete(m.byID, id)
	return nil
}

func (m *memFamilies) SuggestionInputs(context.Context) (ports.SuggestionInputs, error) {
	return m.inputs, nil
}

func (m *memFamilies) DismissSuggestion(_ context.Context, d ports.FamilyDismissal) error {
	m.dismissed = append(m.dismissed, d)
	return nil
}

func familiesHandler(t *testing.T) (*grpcadapter.StudentsHandler, *fakeRepo, *memFamilies) {
	t.Helper()
	repo := newFakeRepo()
	for _, id := range []string{wirePupilA, wirePupilB} {
		repo.byID[id] = domain.Rehydrate(domain.NewApplicantParams{
			ID: id, TenantID: testTenant, FirstName: "Ana", LastName: "García", MaternalLastName: "López",
		}, domain.AdmissionEnrolled)
	}
	families := &memFamilies{stubAccess: &stubAccess{store: newFakeGuardianStore()}, pupils: repo, byID: map[string]domain.Family{}}
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	fam := usecase.NewFamilyService(repo, families, &fakeUserReader{}, testTenant, func() string { return wireFamily }, func() time.Time { return wireFamilyAt })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students")).WithFamilies(fam)
	return h, repo, families
}

func familyRPCCode(err error) connect.Code {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Code()
	}
	return connect.CodeUnknown
}

func TestFamilyRPCsAreUnimplementedWhenNotWired(t *testing.T) {
	svc := usecase.NewStudentService(newFakeRepo(), &fakePublisher{}, testLevels, testTenant, func() string { return "x" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))
	ctx := context.Background()
	calls := map[string]error{}
	_, calls["List"] = h.ListFamilies(ctx, connect.NewRequest(&studentsv1.ListFamiliesRequest{}))
	_, calls["Get"] = h.GetFamily(ctx, connect.NewRequest(&studentsv1.GetFamilyRequest{}))
	_, calls["Create"] = h.CreateFamily(ctx, connect.NewRequest(&studentsv1.CreateFamilyRequest{}))
	_, calls["Rename"] = h.RenameFamily(ctx, connect.NewRequest(&studentsv1.RenameFamilyRequest{}))
	_, calls["Add"] = h.AddStudentsToFamily(ctx, connect.NewRequest(&studentsv1.AddStudentsToFamilyRequest{}))
	_, calls["Remove"] = h.RemoveStudentFromFamily(ctx, connect.NewRequest(&studentsv1.RemoveStudentFromFamilyRequest{}))
	_, calls["Dissolve"] = h.DissolveFamily(ctx, connect.NewRequest(&studentsv1.DissolveFamilyRequest{}))
	_, calls["Suggest"] = h.SuggestFamilies(ctx, connect.NewRequest(&studentsv1.SuggestFamiliesRequest{}))
	_, calls["Dismiss"] = h.DismissFamilySuggestion(ctx, connect.NewRequest(&studentsv1.DismissFamilySuggestionRequest{}))
	for name, err := range calls {
		if familyRPCCode(err) != connect.CodeUnimplemented {
			t.Errorf("%s: want Unimplemented, got %v", name, err)
		}
	}
}

func TestCreateFamilyOnTheWire(t *testing.T) {
	h, _, _ := familiesHandler(t)
	res, err := h.CreateFamily(context.Background(), connect.NewRequest(&studentsv1.CreateFamilyRequest{
		StudentIds: []string{wirePupilA, wirePupilB},
	}))
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	f := res.Msg.GetFamily()
	if f.GetId() != wireFamily || f.GetTenantId() != testTenant || f.GetName() != "García López" ||
		!slices.Equal(f.GetStudentIds(), []string{wirePupilA, wirePupilB}) || f.GetAccountCount() != 1 ||
		f.GetCreatedAt() != "2026-10-23T12:00:00Z" {
		t.Errorf("family = %+v", f)
	}
	// The pupil now carries its family on the wire.
	got, err := h.GetStudent(context.Background(), connect.NewRequest(&studentsv1.GetStudentRequest{Id: wirePupilA}))
	if err != nil || got.Msg.GetStudent().GetFamilyId() != wireFamily {
		t.Errorf("student family_id = %q, %v", got.Msg.GetStudent().GetFamilyId(), err)
	}
}

func TestFamilyErrorsOnTheWire(t *testing.T) {
	h, repo, _ := familiesHandler(t)
	ctx := context.Background()
	if _, err := h.CreateFamily(ctx, connect.NewRequest(&studentsv1.CreateFamilyRequest{StudentIds: []string{"x"}})); familyRPCCode(err) != connect.CodeInvalidArgument {
		t.Errorf("malformed id: %v", err)
	}
	repo.byID[wirePupilB].AssignFamily("fa000000-0000-0000-0000-000000000009")
	// A pupil already in a family is FailedPrecondition, as the contract says.
	if _, err := h.CreateFamily(ctx, connect.NewRequest(&studentsv1.CreateFamilyRequest{StudentIds: []string{wirePupilB}})); familyRPCCode(err) != connect.CodeFailedPrecondition {
		t.Errorf("already in a family: %v", err)
	}
	if _, err := h.GetFamily(ctx, connect.NewRequest(&studentsv1.GetFamilyRequest{FamilyId: wireFamily})); familyRPCCode(err) != connect.CodeNotFound {
		t.Errorf("unknown family: %v", err)
	}
	if _, err := h.DissolveFamily(ctx, connect.NewRequest(&studentsv1.DissolveFamilyRequest{FamilyId: wireFamily})); familyRPCCode(err) != connect.CodeNotFound {
		t.Errorf("dissolve unknown: %v", err)
	}
}

func TestFamilyLifecycleOnTheWire(t *testing.T) {
	h, _, _ := familiesHandler(t)
	ctx := context.Background()
	if _, err := h.CreateFamily(ctx, connect.NewRequest(&studentsv1.CreateFamilyRequest{Name: "Los García", StudentIds: []string{wirePupilA}})); err != nil {
		t.Fatal(err)
	}
	renamed, err := h.RenameFamily(ctx, connect.NewRequest(&studentsv1.RenameFamilyRequest{FamilyId: wireFamily, Name: "García"}))
	if err != nil || renamed.Msg.GetFamily().GetName() != "García" {
		t.Fatalf("Rename = %+v, %v", renamed, err)
	}
	listed, err := h.ListFamilies(ctx, connect.NewRequest(&studentsv1.ListFamiliesRequest{}))
	if err != nil || len(listed.Msg.GetFamilies()) != 1 {
		t.Fatalf("List = %+v, %v", listed, err)
	}
	got, err := h.GetFamily(ctx, connect.NewRequest(&studentsv1.GetFamilyRequest{FamilyId: wireFamily}))
	if err != nil || got.Msg.GetFamily().GetId() != wireFamily {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := h.AddStudentsToFamily(ctx, connect.NewRequest(&studentsv1.AddStudentsToFamilyRequest{FamilyId: wireFamily, StudentIds: []string{wirePupilB}})); familyRPCCode(err) != connect.CodeFailedPrecondition {
		t.Errorf("add racing a conflict: %v", err)
	}
	removed, err := h.RemoveStudentFromFamily(ctx, connect.NewRequest(&studentsv1.RemoveStudentFromFamilyRequest{FamilyId: wireFamily, StudentId: wirePupilA}))
	if err != nil || removed.Msg.GetFamily() != nil {
		t.Errorf("removing the last pupil: family = %+v, %v; want none", removed.Msg.GetFamily(), err)
	}
}

func TestSuggestAndDismissOnTheWire(t *testing.T) {
	h, _, families := familiesHandler(t)
	ctx := context.Background()
	families.inputs = ports.SuggestionInputs{
		Pupils: []domain.SuggestionPupil{
			{ID: wirePupilA, LastName: "García", MaternalLastName: "López"},
			{ID: wirePupilB, LastName: "Soto", MaternalLastName: "Gil"},
		},
		Contacts: []domain.SuggestionContact{{StudentID: wirePupilA, GuardianUserID: "u"}, {StudentID: wirePupilB, GuardianUserID: "u"}},
	}
	res, err := h.SuggestFamilies(ctx, connect.NewRequest(&studentsv1.SuggestFamiliesRequest{}))
	if err != nil || len(res.Msg.GetSuggestions()) != 1 {
		t.Fatalf("Suggest = %+v, %v", res, err)
	}
	s := res.Msg.GetSuggestions()[0]
	if s.GetProposedName() != "García López" || !slices.Equal(s.GetStudentIds(), []string{wirePupilA, wirePupilB}) ||
		s.GetSignature() != domain.SuggestionSignature([]string{wirePupilA, wirePupilB}) ||
		s.GetConfidence() != studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_HIGH ||
		!slices.Equal(s.GetReasons(), []studentsv1.FamilySuggestionReason{studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_ACCOUNT}) {
		t.Errorf("suggestion = %+v", s)
	}
	if _, err := h.DismissFamilySuggestion(ctx, connect.NewRequest(&studentsv1.DismissFamilySuggestionRequest{
		StudentIds: []string{wirePupilA, wirePupilB}, UserId: wireActor,
	})); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if len(families.dismissed) != 1 || families.dismissed[0].DismissedBy != wireActor || families.dismissed[0].Signature != s.GetSignature() {
		t.Errorf("dismissed = %+v", families.dismissed)
	}
}

func TestListStudentsPassesTheFamilyFilter(t *testing.T) {
	h, repo, _ := familiesHandler(t)
	if _, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{FamilyId: wireFamily})); err != nil {
		t.Fatal(err)
	}
	if repo.lastFilter.FamilyID != wireFamily {
		t.Errorf("filter = %+v", repo.lastFilter)
	}
}
