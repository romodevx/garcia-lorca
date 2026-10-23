package client_test

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// stubFamilies records what each family RPC received and answers fixed data.
type stubFamilies struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	create  *studentsv1.CreateFamilyRequest
	dismiss *studentsv1.DismissFamilySuggestionRequest
	contact *studentsv1.AddFamilyContactRequest
	revoke  *studentsv1.RevokeFamilyAccessRequest
	list    []*studentsv1.ListStudentsRequest
	// dissolveOnRemove answers RemoveStudentFromFamily without a family.
	dissolveOnRemove bool
}

func protoFamily() *studentsv1.Family {
	return &studentsv1.Family{
		Id: "fam-1", Name: "GARCÍA LÓPEZ", StudentIds: []string{"s-1", "s-2"},
		AccountCount: 1, CreatedAt: "2026-10-20T15:04:05Z",
	}
}

func (s *stubFamilies) ListFamilies(context.Context, *connect.Request[studentsv1.ListFamiliesRequest]) (*connect.Response[studentsv1.ListFamiliesResponse], error) {
	return connect.NewResponse(&studentsv1.ListFamiliesResponse{Families: []*studentsv1.Family{protoFamily()}}), nil
}

func (s *stubFamilies) CreateFamily(_ context.Context, req *connect.Request[studentsv1.CreateFamilyRequest]) (*connect.Response[studentsv1.CreateFamilyResponse], error) {
	s.create = req.Msg
	return connect.NewResponse(&studentsv1.CreateFamilyResponse{Family: protoFamily()}), nil
}

func (s *stubFamilies) RemoveStudentFromFamily(context.Context, *connect.Request[studentsv1.RemoveStudentFromFamilyRequest]) (*connect.Response[studentsv1.RemoveStudentFromFamilyResponse], error) {
	if s.dissolveOnRemove {
		return connect.NewResponse(&studentsv1.RemoveStudentFromFamilyResponse{}), nil
	}
	return connect.NewResponse(&studentsv1.RemoveStudentFromFamilyResponse{Family: protoFamily()}), nil
}

func (s *stubFamilies) SuggestFamilies(context.Context, *connect.Request[studentsv1.SuggestFamiliesRequest]) (*connect.Response[studentsv1.SuggestFamiliesResponse], error) {
	return connect.NewResponse(&studentsv1.SuggestFamiliesResponse{Suggestions: []*studentsv1.FamilySuggestion{{
		Signature: "abc", ProposedName: "PÉREZ RUIZ", StudentIds: []string{"s-3", "s-4"},
		Reasons: []studentsv1.FamilySuggestionReason{
			studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SAME_SURNAMES,
			studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_CONTACT_EMAIL,
			studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_CONTACT_PHONE,
			studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_ACCOUNT,
		},
		Confidence: studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_HIGH,
	}}}), nil
}

func (s *stubFamilies) DismissFamilySuggestion(_ context.Context, req *connect.Request[studentsv1.DismissFamilySuggestionRequest]) (*connect.Response[studentsv1.DismissFamilySuggestionResponse], error) {
	s.dismiss = req.Msg
	return connect.NewResponse(&studentsv1.DismissFamilySuggestionResponse{}), nil
}

func (s *stubFamilies) AddFamilyContact(_ context.Context, req *connect.Request[studentsv1.AddFamilyContactRequest]) (*connect.Response[studentsv1.AddFamilyContactResponse], error) {
	s.contact = req.Msg
	return connect.NewResponse(&studentsv1.AddFamilyContactResponse{Contacts: []*studentsv1.GuardianLink{
		{Id: "c-1", StudentId: "s-1", FullName: "ANA LÓPEZ"},
		{Id: "c-2", StudentId: "s-2", FullName: "ANA LÓPEZ"},
	}}), nil
}

func (s *stubFamilies) RevokeFamilyAccess(_ context.Context, req *connect.Request[studentsv1.RevokeFamilyAccessRequest]) (*connect.Response[studentsv1.RevokeFamilyAccessResponse], error) {
	s.revoke = req.Msg
	return connect.NewResponse(&studentsv1.RevokeFamilyAccessResponse{Contacts: 2}), nil
}

func (s *stubFamilies) ListStudents(_ context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	s.list = append(s.list, req.Msg)
	if req.Msg.GetPageToken() == "" {
		return connect.NewResponse(&studentsv1.ListStudentsResponse{
			Students:      []*studentsv1.Student{{Id: "s-1", FamilyId: "fam-1"}},
			NextPageToken: "p2",
		}), nil
	}
	return connect.NewResponse(&studentsv1.ListStudentsResponse{Students: []*studentsv1.Student{{Id: "s-2", FamilyId: "fam-1"}}}), nil
}

func familiesClient(t *testing.T, stub *stubFamilies) *client.StudentsClient {
	t.Helper()
	path, h := studentsv1connect.NewStudentsServiceHandler(stub)
	return client.NewStudentsClient(serveAt(t, path, h), http.DefaultClient)
}

// ADR 0041: a family reaches the edge with its pupils and account count.
func TestFamilyClientMapsTheFamily(t *testing.T) {
	ctx := context.Background()
	stub := &stubFamilies{}
	students := familiesClient(t, stub)

	families, err := students.ListFamilies(ctx)
	require.NoError(t, err)
	assert.Equal(t, []client.Family{{
		ID: "fam-1", Name: "GARCÍA LÓPEZ", StudentIDs: []string{"s-1", "s-2"},
		AccountCount: 1, CreatedAt: "2026-10-20T15:04:05Z",
	}}, families)

	created, err := students.CreateFamily(ctx, "GARCÍA", []string{"s-1", "s-2"})
	require.NoError(t, err)
	assert.Equal(t, "fam-1", created.ID)
	assert.Equal(t, "GARCÍA", stub.create.GetName())
	assert.Equal(t, []string{"s-1", "s-2"}, stub.create.GetStudentIds())
}

// Removing the last pupil dissolves the family: the client says so instead of
// handing back an empty family that looks like a real one.
func TestRemoveStudentFromFamilyReportsADissolvedFamily(t *testing.T) {
	ctx := context.Background()
	stub := &stubFamilies{}
	students := familiesClient(t, stub)

	family, stays, err := students.RemoveStudentFromFamily(ctx, "fam-1", "s-1")
	require.NoError(t, err)
	assert.True(t, stays)
	assert.Equal(t, "fam-1", family.ID)

	stub.dissolveOnRemove = true
	_, stays, err = students.RemoveStudentFromFamily(ctx, "fam-1", "s-2")
	require.NoError(t, err)
	assert.False(t, stays)
}

func TestFamilySuggestionsSpeakLowercaseNames(t *testing.T) {
	stub := &stubFamilies{}
	got, err := familiesClient(t, stub).SuggestFamilies(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, client.FamilySuggestion{
		Signature: "abc", ProposedName: "PÉREZ RUIZ", StudentIDs: []string{"s-3", "s-4"},
		Reasons:    []string{"same_surnames", "shared_contact_email", "shared_contact_phone", "shared_account"},
		Confidence: "high",
	}, got[0])
}

func TestFamilyWritesCarryTheirArguments(t *testing.T) {
	ctx := context.Background()
	stub := &stubFamilies{}
	students := familiesClient(t, stub)

	require.NoError(t, students.DismissFamilySuggestion(ctx, []string{"s-3", "s-4"}, "usr-1"))
	assert.Equal(t, []string{"s-3", "s-4"}, stub.dismiss.GetStudentIds())
	assert.Equal(t, "usr-1", stub.dismiss.GetUserId())

	contacts, err := students.AddFamilyContact(ctx, client.FamilyContactInput{
		FamilyID: "fam-1", Relationship: "mother", FullName: "ANA LÓPEZ", Phone: "5512345678",
		Email: "ana@example.com", IsEmergency: true,
	})
	require.NoError(t, err)
	assert.Len(t, contacts, 2)
	assert.Equal(t, "fam-1", stub.contact.GetFamilyId())
	assert.Equal(t, "mother", stub.contact.GetRelationship())
	assert.Equal(t, "ana@example.com", stub.contact.GetEmail())
	assert.True(t, stub.contact.GetIsEmergency())

	n, err := students.RevokeFamilyAccess(ctx, "fam-1", "usr-9")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, "usr-9", stub.revoke.GetGuardianUserId())
}

// The directory's family filter drains every page, and each pupil carries
// its family id.
func TestListStudentsInFamilyFiltersAndPaginates(t *testing.T) {
	stub := &stubFamilies{}
	got, err := familiesClient(t, stub).ListStudentsInFamily(context.Background(), "fam-1", "enrolled")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "fam-1", got[0].FamilyID)
	require.Len(t, stub.list, 2)
	for _, req := range stub.list {
		assert.Equal(t, "fam-1", req.GetFamilyId())
		assert.Equal(t, studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED, req.GetStatus())
	}
}
