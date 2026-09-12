package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// The guardian handler tests drive the real GuardianService through the handler
// with in-memory fakes for its seams (student repo, guardian repo, iam user
// reader), asserting proto↔usecase translation and the error→Connect-code
// mapping. Logic lives in usecase/domain; these tests own the wire boundary.

const (
	guardianUser    = "60000000-0000-0000-0000-000000000001"
	nonGuardianUser = "60000000-0000-0000-0000-000000000002"
)

// fakeGuardianStore is an in-memory GuardianRepository enforcing the
// (tenant, guardian, student) uniqueness the real table owns.
type fakeGuardianStore struct {
	links map[string]domain.GuardianLink // guardian|student
}

func newFakeGuardianStore() *fakeGuardianStore {
	return &fakeGuardianStore{links: map[string]domain.GuardianLink{}}
}

func (s *fakeGuardianStore) Link(_ context.Context, l domain.GuardianLink) error {
	key := l.GuardianUserID() + "|" + l.StudentID()
	if _, ok := s.links[key]; ok {
		return glerrors.New(glerrors.KindConflict, "guardian is already linked to this student")
	}
	s.links[key] = l
	return nil
}

func (s *fakeGuardianStore) ByID(_ context.Context, id string) (domain.GuardianLink, error) {
	for _, l := range s.links {
		if l.ID() == id {
			return l, nil
		}
	}
	return domain.GuardianLink{}, glerrors.ErrNotFound
}

func (s *fakeGuardianStore) AttachAccount(_ context.Context, l domain.GuardianLink) error {
	for key, stored := range s.links {
		if stored.ID() != l.ID() {
			continue
		}
		if stored.HasAccount() {
			return glerrors.New(glerrors.KindConflict, "this contact already has an account")
		}
		delete(s.links, key)
		s.links[l.GuardianUserID()+"|"+l.StudentID()] = l
		return nil
	}
	return glerrors.ErrNotFound
}

func (s *fakeGuardianStore) Unlink(_ context.Context, guardianUserID, studentID string) error {
	key := guardianUserID + "|" + studentID
	if _, ok := s.links[key]; !ok {
		return glerrors.ErrNotFound
	}
	delete(s.links, key)
	return nil
}

func (s *fakeGuardianStore) ListByStudent(_ context.Context, studentID string) ([]domain.GuardianLink, error) {
	out := make([]domain.GuardianLink, 0, len(s.links))
	for _, l := range s.links {
		if l.StudentID() == studentID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *fakeGuardianStore) ListByGuardian(_ context.Context, guardianUserID string) ([]domain.GuardianLink, error) {
	out := make([]domain.GuardianLink, 0, len(s.links))
	for _, l := range s.links {
		if l.GuardianUserID() == guardianUserID {
			out = append(out, l)
		}
	}
	return out, nil
}

// fakeUserReader is an in-memory UserReader: a missing user is
// glerrors.ErrNotFound (matching the real iam Connect client's NotFound mapping).
type fakeUserReader struct {
	byID map[string]ports.UserRef
}

func (u *fakeUserReader) ByID(_ context.Context, userID string) (ports.UserRef, error) {
	if ref, ok := u.byID[userID]; ok {
		return ref, nil
	}
	return ports.UserRef{}, glerrors.ErrNotFound
}

// guardianHandler wires the handler with a pre-seeded student, one iam user that
// holds the guardian role and one that does not, returning the handler and the
// seeded student id.
func guardianHandler(t *testing.T) (*grpcadapter.StudentsHandler, string) {
	t.Helper()
	studentRepo := newFakeRepo()
	student := seedApplicant(t, studentRepo)
	users := &fakeUserReader{byID: map[string]ports.UserRef{
		guardianUser:    {ID: guardianUser, Status: "USER_STATUS_ACTIVE", Roles: []string{"guardian"}},
		nonGuardianUser: {ID: nonGuardianUser, Status: "USER_STATUS_ACTIVE", Roles: []string{"teacher"}},
	}}
	ids := 0
	guardians := usecase.NewGuardianService(studentRepo, newFakeGuardianStore(), users, testTenant,
		func() string { ids++; return "gl-1" })
	stu := usecase.NewStudentService(studentRepo, &fakePublisher{}, testTenant, func() string { return "stu-x" })
	h := grpcadapter.NewStudentsHandler(stu, nil, guardians, usecase.NewHealthService("students"))
	return h, student.ID()
}

func TestLinkGuardianMapsTheLink(t *testing.T) {
	h, studentID := guardianHandler(t)
	res, err := h.LinkGuardian(context.Background(), connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser,
		StudentId:      studentID,
		Relationship:   "mother",
	}))
	if err != nil {
		t.Fatalf("LinkGuardian: %v", err)
	}
	got := res.Msg.GetLink()
	if got.GetGuardianUserId() != guardianUser || got.GetStudentId() != studentID {
		t.Errorf("link references not mapped 1:1: %+v", got)
	}
	if got.GetTenantId() != testTenant {
		t.Errorf("tenant_id = %q, want %q", got.GetTenantId(), testTenant)
	}
	if got.GetRelationship() != "mother" {
		t.Errorf("relationship = %q, want the canonical name", got.GetRelationship())
	}
	if got.GetId() == "" {
		t.Error("the link must carry its minted id")
	}
}

func TestLinkGuardianUnknownStudentIsInvalidArgument(t *testing.T) {
	h, _ := guardianHandler(t)
	_, err := h.LinkGuardian(context.Background(), connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser,
		StudentId:      "ghost",
		Relationship:   "mother",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestLinkGuardianUnknownUserIsInvalidArgument(t *testing.T) {
	h, studentID := guardianHandler(t)
	_, err := h.LinkGuardian(context.Background(), connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: "ghost",
		StudentId:      studentID,
		Relationship:   "mother",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestLinkGuardianNonGuardianUserIsInvalidArgument(t *testing.T) {
	h, studentID := guardianHandler(t)
	_, err := h.LinkGuardian(context.Background(), connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: nonGuardianUser,
		StudentId:      studentID,
		Relationship:   "mother",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestLinkGuardianUnknownRelationshipIsInvalidArgument(t *testing.T) {
	h, studentID := guardianHandler(t)
	_, err := h.LinkGuardian(context.Background(), connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser,
		StudentId:      studentID,
		Relationship:   "uncle",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestLinkGuardianDuplicateIsAlreadyExists(t *testing.T) {
	h, studentID := guardianHandler(t)
	req := &studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: studentID, Relationship: "mother",
	}
	if _, err := h.LinkGuardian(context.Background(), connect.NewRequest(req)); err != nil {
		t.Fatalf("first LinkGuardian: %v", err)
	}
	_, err := h.LinkGuardian(context.Background(), connect.NewRequest(req))
	assertCode(t, err, connect.CodeAlreadyExists)
}

func TestUnlinkGuardianAndUnknownLinkIsNotFound(t *testing.T) {
	h, studentID := guardianHandler(t)
	ctx := context.Background()
	if _, err := h.LinkGuardian(ctx, connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: studentID, Relationship: "father",
	})); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	if _, err := h.UnlinkGuardian(ctx, connect.NewRequest(&studentsv1.UnlinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: studentID,
	})); err != nil {
		t.Fatalf("UnlinkGuardian: %v", err)
	}
	// The link is gone: unlinking again is NotFound (the frozen contract).
	_, err := h.UnlinkGuardian(ctx, connect.NewRequest(&studentsv1.UnlinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: studentID,
	}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestListGuardianLinksBothWays(t *testing.T) {
	h, studentID := guardianHandler(t)
	ctx := context.Background()
	if _, err := h.LinkGuardian(ctx, connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUser, StudentId: studentID, Relationship: "mother",
	})); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	forStudent, err := h.ListGuardiansForStudent(ctx, connect.NewRequest(&studentsv1.ListGuardiansForStudentRequest{
		StudentId: studentID,
	}))
	if err != nil {
		t.Fatalf("ListGuardiansForStudent: %v", err)
	}
	if links := forStudent.Msg.GetLinks(); len(links) != 1 || links[0].GetGuardianUserId() != guardianUser {
		t.Errorf("ListGuardiansForStudent = %+v, want the seeded link", links)
	}

	forGuardian, err := h.ListStudentsForGuardian(ctx, connect.NewRequest(&studentsv1.ListStudentsForGuardianRequest{
		GuardianUserId: guardianUser,
	}))
	if err != nil {
		t.Fatalf("ListStudentsForGuardian: %v", err)
	}
	if links := forGuardian.Msg.GetLinks(); len(links) != 1 || links[0].GetStudentId() != studentID {
		t.Errorf("ListStudentsForGuardian = %+v, want the seeded link", links)
	}
}

// --- GrantContactAccess ----------------------------------------------------

// addContact records an account-less contact through the handler and returns its id.
func addContact(t *testing.T, h *grpcadapter.StudentsHandler, studentID string) string {
	t.Helper()
	res, err := h.AddContact(context.Background(), connect.NewRequest(&studentsv1.AddContactRequest{
		StudentId:    studentID,
		Relationship: "father",
		FullName:     "Mateo Contreras",
		Email:        "mateo@example.com",
	}))
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	return res.Msg.GetContact().GetId()
}

func TestGrantContactAccessMapsTheContact(t *testing.T) {
	h, studentID := guardianHandler(t)
	contactID := addContact(t, h, studentID)

	res, err := h.GrantContactAccess(context.Background(), connect.NewRequest(&studentsv1.GrantContactAccessRequest{
		ContactId:      contactID,
		StudentId:      studentID,
		GuardianUserId: guardianUser,
	}))
	if err != nil {
		t.Fatalf("GrantContactAccess: %v", err)
	}
	got := res.Msg.GetContact()
	if got.GetId() != contactID {
		t.Errorf("id = %q, want the contact that was already there (%q)", got.GetId(), contactID)
	}
	if got.GetGuardianUserId() != guardianUser {
		t.Errorf("guardian user id = %q, want %q", got.GetGuardianUserId(), guardianUser)
	}
	if got.GetFullName() != "Mateo Contreras" {
		t.Errorf("the person was lost in the mapping: %+v", got)
	}
}

// A user that does not hold the guardian role is a bad ARGUMENT, not a missing
// resource: the call addresses a contact that exists.
func TestGrantContactAccessNonGuardianIsInvalidArgument(t *testing.T) {
	h, studentID := guardianHandler(t)
	contactID := addContact(t, h, studentID)

	_, err := h.GrantContactAccess(context.Background(), connect.NewRequest(&studentsv1.GrantContactAccessRequest{
		ContactId:      contactID,
		StudentId:      studentID,
		GuardianUserId: nonGuardianUser,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument (err=%v)", connect.CodeOf(err), err)
	}
}

// Naming a pupil the contact does not belong to answers NotFound — never
// PermissionDenied, which would confirm the contact exists.
func TestGrantContactAccessWrongPupilIsNotFound(t *testing.T) {
	h, studentID := guardianHandler(t)
	contactID := addContact(t, h, studentID)

	_, err := h.GrantContactAccess(context.Background(), connect.NewRequest(&studentsv1.GrantContactAccessRequest{
		ContactId:      contactID,
		StudentId:      "00000000-0000-0000-0000-0000000000ff",
		GuardianUserId: guardianUser,
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want NotFound (err=%v)", connect.CodeOf(err), err)
	}
}
