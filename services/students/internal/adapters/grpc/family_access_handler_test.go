package grpcadapter_test

import (
	"context"
	"errors"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// stubAccess is a FamilyAccessRepository for the wire tests: OnPupil runs fn
// family-less over the guardian store; OnFamily runs it over `members`, or
// answers NotFound for a family not in `families`. The sync logic itself is
// the use case's and the domain's (tested there).
type stubAccess struct {
	store    *fakeGuardianStore
	families map[string]bool
	members  []domain.FamilyMember
	detached int
	// onPupilErr, when set, is what OnPupil answers.
	onPupilErr  error
	accountsErr error
	// importLinks are the RecordImportLinks writes.
	importLinks []ports.FamilyImportLink
}

func (s *stubAccess) OnPupil(ctx context.Context, _ string, fn ports.FamilyAccessFunc) error {
	if s.onPupilErr != nil {
		return s.onPupilErr
	}
	return fn(ctx, &stubAccessTx{s: s})
}

func (s *stubAccess) OnFamily(ctx context.Context, familyID string, fn ports.FamilyAccessFunc) error {
	if !s.families[familyID] {
		return glerrors.ErrNotFound
	}
	return fn(ctx, &stubAccessTx{s: s, familyID: familyID})
}

// AccountIDs: the wire tests never need iam to vet a sibling's account.
func (s *stubAccess) AccountIDs(context.Context, string, []string) ([]string, error) {
	return nil, s.accountsErr
}

func (s *stubAccess) runSync(ctx context.Context, familyID string, sync ports.FamilyAccessFunc) error {
	if sync == nil {
		return nil
	}
	return sync(ctx, &stubAccessTx{s: s, familyID: familyID})
}

type stubAccessTx struct {
	s        *stubAccess
	familyID string
}

func (t *stubAccessTx) FamilyID() string { return t.familyID }
func (t *stubAccessTx) Members(context.Context) ([]domain.FamilyMember, error) {
	return t.s.members, nil
}
func (t *stubAccessTx) AttachAccount(ctx context.Context, l domain.GuardianLink) error {
	return t.s.store.attach(l)
}
func (t *stubAccessTx) InsertContact(ctx context.Context, l domain.GuardianLink) error {
	return t.s.store.Link(ctx, l)
}
func (t *stubAccessTx) RemoveLink(ctx context.Context, guardianUserID, studentID string) error {
	return t.s.store.unlink(guardianUserID, studentID)
}
func (t *stubAccessTx) DetachAccount(context.Context, string) (int, error) { return t.s.detached, nil }
func (t *stubAccessTx) RecordImportLinks(_ context.Context, links []ports.FamilyImportLink) error {
	t.s.importLinks = append(t.s.importLinks, links...)
	return nil
}

// overCapMembers is a family that already holds three accounts.
func overCapMembers(t *testing.T) []domain.FamilyMember {
	t.Helper()
	var contacts []domain.GuardianLink
	for i, u := range []string{"60000000-0000-0000-0000-0000000000b1", "60000000-0000-0000-0000-0000000000b2", "60000000-0000-0000-0000-0000000000b3"} {
		l, err := domain.NewGuardianLink(string(rune('a'+i)), testTenant, u, wirePupilA, domain.RelationshipOther)
		if err != nil {
			t.Fatal(err)
		}
		contacts = append(contacts, l)
	}
	return []domain.FamilyMember{{StudentID: wirePupilA, Status: domain.AdmissionEnrolled, Contacts: contacts}}
}

func errorCode(err error) string {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Meta().Get(grpcadapter.ErrorCodeMeta)
	}
	return ""
}

func TestFamilyAccountLimitOnTheWire(t *testing.T) {
	h, _, families := familiesHandler(t)
	families.members = overCapMembers(t)

	_, err := h.CreateFamily(context.Background(), connect.NewRequest(&studentsv1.CreateFamilyRequest{StudentIds: []string{wirePupilA}}))
	if familyRPCCode(err) != connect.CodeFailedPrecondition || errorCode(err) != domain.CodeFamilyAccountLimit {
		t.Errorf("CreateFamily over the cap: code=%v meta=%q (%v); want FailedPrecondition + %s",
			familyRPCCode(err), errorCode(err), err, domain.CodeFamilyAccountLimit)
	}
}

// The guardian RPCs map the same refusal the same way.
func TestFamilyAccountLimitOnTheGuardianRPCs(t *testing.T) {
	studentRepo := newFakeRepo()
	student := seedApplicant(t, studentRepo)
	student.AssignFamily(wireFamily)
	users := &fakeUserReader{byID: map[string]ports.UserRef{
		guardianUser: {ID: guardianUser, Status: "USER_STATUS_ACTIVE", Roles: []string{"guardian"}},
	}}
	store := newFakeGuardianStore()
	access := &familyStub{stubAccess{store: store, members: overCapMembers(t)}}
	guardians := usecase.NewGuardianService(studentRepo, store, access, users, testTenant, func() string { return "gl-1" })
	stu := usecase.NewStudentService(studentRepo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-x" })
	h := grpcadapter.NewStudentsHandler(stu, nil, guardians, usecase.NewHealthService("students"))

	_, err := h.LinkGuardian(context.Background(), connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: student.ID(), Relationship: "mother",
	}))
	if familyRPCCode(err) != connect.CodeFailedPrecondition || errorCode(err) != domain.CodeFamilyAccountLimit {
		t.Errorf("LinkGuardian over the cap: code=%v meta=%q (%v)", familyRPCCode(err), errorCode(err), err)
	}
}

// familyStub is stubAccess whose OnPupil scope is a family (wireFamily).
type familyStub struct{ stubAccess }

func (s *familyStub) OnPupil(ctx context.Context, _ string, fn ports.FamilyAccessFunc) error {
	return fn(ctx, &stubAccessTx{s: &s.stubAccess, familyID: wireFamily})
}

func TestAddFamilyContactOnTheWire(t *testing.T) {
	h, _, families := familiesHandler(t)
	families.families = map[string]bool{wireFamily: true}
	families.members = []domain.FamilyMember{
		{StudentID: wirePupilA, Status: domain.AdmissionEnrolled},
		{StudentID: wirePupilB, Status: domain.AdmissionPending},
	}
	ctx := context.Background()
	res, err := h.AddFamilyContact(ctx, connect.NewRequest(&studentsv1.AddFamilyContactRequest{
		FamilyId: wireFamily, Relationship: "mother", FullName: "Rosa Pérez", Phone: "443 111 2233", Email: "Rosa@Example.com", IsEmergency: true,
	}))
	if err != nil {
		t.Fatalf("AddFamilyContact: %v", err)
	}
	got := res.Msg.GetContacts()
	if len(got) != 2 || got[0].GetStudentId() != wirePupilA || got[1].GetStudentId() != wirePupilB {
		t.Fatalf("contacts = %+v", got)
	}
	if c := got[0]; c.GetFullName() != "Rosa Pérez" || c.GetPhone() != "4431112233" || c.GetEmail() != "rosa@example.com" ||
		c.GetRelationship() != "mother" || !c.GetIsEmergency() || c.GetGuardianUserId() != "" || c.GetTenantId() != testTenant {
		t.Errorf("contact = %+v", c)
	}

	if _, err := h.AddFamilyContact(ctx, connect.NewRequest(&studentsv1.AddFamilyContactRequest{
		FamilyId: "fa000000-0000-0000-0000-0000000000ff", Relationship: "mother", FullName: "Rosa",
	})); familyRPCCode(err) != connect.CodeNotFound {
		t.Errorf("unknown family: %v", err)
	}
	if _, err := h.AddFamilyContact(ctx, connect.NewRequest(&studentsv1.AddFamilyContactRequest{
		FamilyId: wireFamily, Relationship: "mother",
	})); familyRPCCode(err) != connect.CodeInvalidArgument {
		t.Errorf("nameless: %v", err)
	}
}

func TestRevokeFamilyAccessOnTheWire(t *testing.T) {
	h, _, families := familiesHandler(t)
	families.families = map[string]bool{wireFamily: true}
	families.detached = 3
	ctx := context.Background()
	res, err := h.RevokeFamilyAccess(ctx, connect.NewRequest(&studentsv1.RevokeFamilyAccessRequest{FamilyId: wireFamily, GuardianUserId: guardianUser}))
	if err != nil || res.Msg.GetContacts() != 3 {
		t.Fatalf("Revoke = %+v, %v; want 3", res, err)
	}
	families.detached = 0
	if _, err := h.RevokeFamilyAccess(ctx, connect.NewRequest(&studentsv1.RevokeFamilyAccessRequest{FamilyId: wireFamily, GuardianUserId: guardianUser})); familyRPCCode(err) != connect.CodeNotFound {
		t.Errorf("account not in the family: %v", err)
	}
	if _, err := h.RevokeFamilyAccess(ctx, connect.NewRequest(&studentsv1.RevokeFamilyAccessRequest{FamilyId: "fa000000-0000-0000-0000-0000000000ff", GuardianUserId: guardianUser})); familyRPCCode(err) != connect.CodeNotFound {
		t.Errorf("unknown family: %v", err)
	}
	if _, err := h.RevokeFamilyAccess(ctx, connect.NewRequest(&studentsv1.RevokeFamilyAccessRequest{FamilyId: wireFamily})); familyRPCCode(err) != connect.CodeInvalidArgument {
		t.Errorf("no account: %v", err)
	}
}

func TestFamilyAccessRPCsAreUnimplementedWhenNotWired(t *testing.T) {
	svc := usecase.NewStudentService(newFakeRepo(), &fakePublisher{}, testLevels, testTenant, func() string { return "x" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))
	ctx := context.Background()
	if _, err := h.AddFamilyContact(ctx, connect.NewRequest(&studentsv1.AddFamilyContactRequest{})); familyRPCCode(err) != connect.CodeUnimplemented {
		t.Errorf("AddFamilyContact: %v", err)
	}
	if _, err := h.RevokeFamilyAccess(ctx, connect.NewRequest(&studentsv1.RevokeFamilyAccessRequest{})); familyRPCCode(err) != connect.CodeUnimplemented {
		t.Errorf("RevokeFamilyAccess: %v", err)
	}
}

func TestUnlinkGuardianOnAFamilyPupilOnTheWire(t *testing.T) {
	studentRepo := newFakeRepo()
	student := seedApplicant(t, studentRepo)
	store := newFakeGuardianStore()
	access := &familyStub{stubAccess{store: store}}
	guardians := usecase.NewGuardianService(studentRepo, store, access, &fakeUserReader{}, testTenant, func() string { return "gl-1" })
	stu := usecase.NewStudentService(studentRepo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-x" })
	h := grpcadapter.NewStudentsHandler(stu, nil, guardians, usecase.NewHealthService("students"))

	_, err := h.UnlinkGuardian(context.Background(), connect.NewRequest(&studentsv1.UnlinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: student.ID(),
	}))
	if familyRPCCode(err) != connect.CodeFailedPrecondition || errorCode(err) != domain.CodeFamilyManagedAccess {
		t.Errorf("unlink in a family: code=%v meta=%q (%v); want FailedPrecondition + %s",
			familyRPCCode(err), errorCode(err), err, domain.CodeFamilyManagedAccess)
	}
}

// A pupil whose family kept changing while its locks were taken is a state to
// retry, not a duplicate: FailedPrecondition with FAMILY_BUSY.
func TestFamilyBusyOnTheWire(t *testing.T) {
	studentRepo := newFakeRepo()
	student := seedApplicant(t, studentRepo)
	store := newFakeGuardianStore()
	access := &stubAccess{store: store, onPupilErr: domain.ErrFamilyBusy}
	guardians := usecase.NewGuardianService(studentRepo, store, access, &fakeUserReader{}, testTenant, func() string { return "gl-1" })
	stu := usecase.NewStudentService(studentRepo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-x" })
	h := grpcadapter.NewStudentsHandler(stu, nil, guardians, usecase.NewHealthService("students"))

	_, err := h.UnlinkGuardian(context.Background(), connect.NewRequest(&studentsv1.UnlinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: student.ID(),
	}))
	if familyRPCCode(err) != connect.CodeFailedPrecondition || errorCode(err) != domain.CodeFamilyBusy {
		t.Errorf("code=%v meta=%q (%v); want FailedPrecondition + %s", familyRPCCode(err), errorCode(err), err, domain.CodeFamilyBusy)
	}
}
