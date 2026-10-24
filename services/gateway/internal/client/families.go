package client

import (
	"context"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// Family groups siblings (ADR 0041). StudentIDs is ordered by surname then
// first name; AccountCount is how many distinct app accounts its pupils carry
// (at most two, enforced by the students service).
type Family struct {
	ID           string
	Name         string
	StudentIDs   []string
	AccountCount int
	CreatedAt    string // RFC 3339
}

// FamilySuggestion is a group of pupils without a family that look like one.
// Reasons and Confidence are lowercase names (same_surnames, high, ...).
type FamilySuggestion struct {
	Signature    string
	ProposedName string
	StudentIDs   []string
	Reasons      []string
	Confidence   string
}

// FamilyContactInput is one adult recorded on every pupil of a family.
type FamilyContactInput struct {
	FamilyID     string
	Relationship string
	FullName     string
	Phone        string
	Email        string
	IsEmergency  bool
}

// ListFamilies returns every family of the school, ordered by name.
func (c *StudentsClient) ListFamilies(ctx context.Context) ([]Family, error) {
	res, err := c.rpc.ListFamilies(ctx, connect.NewRequest(&studentsv1.ListFamiliesRequest{}))
	if err != nil {
		return nil, err
	}
	return toFamilies(res.Msg.GetFamilies()), nil
}

// GetFamily reads one family; NotFound for an unknown id.
func (c *StudentsClient) GetFamily(ctx context.Context, familyID string) (Family, error) {
	res, err := c.rpc.GetFamily(ctx, connect.NewRequest(&studentsv1.GetFamilyRequest{FamilyId: familyID}))
	if err != nil {
		return Family{}, err
	}
	return toFamily(res.Msg.GetFamily()), nil
}

// CreateFamily groups pupils that have no family yet. An empty name lets the
// service name it after the first pupil's surnames.
func (c *StudentsClient) CreateFamily(ctx context.Context, name string, studentIDs []string) (Family, error) {
	res, err := c.rpc.CreateFamily(ctx, connect.NewRequest(&studentsv1.CreateFamilyRequest{Name: name, StudentIds: studentIDs}))
	if err != nil {
		return Family{}, err
	}
	return toFamily(res.Msg.GetFamily()), nil
}

// RenameFamily changes how the office calls a family.
func (c *StudentsClient) RenameFamily(ctx context.Context, familyID, name string) (Family, error) {
	res, err := c.rpc.RenameFamily(ctx, connect.NewRequest(&studentsv1.RenameFamilyRequest{FamilyId: familyID, Name: name}))
	if err != nil {
		return Family{}, err
	}
	return toFamily(res.Msg.GetFamily()), nil
}

// AddStudentsToFamily adds pupils without a family to an existing one.
func (c *StudentsClient) AddStudentsToFamily(ctx context.Context, familyID string, studentIDs []string) (Family, error) {
	res, err := c.rpc.AddStudentsToFamily(ctx, connect.NewRequest(&studentsv1.AddStudentsToFamilyRequest{FamilyId: familyID, StudentIds: studentIDs}))
	if err != nil {
		return Family{}, err
	}
	return toFamily(res.Msg.GetFamily()), nil
}

// RemoveStudentFromFamily takes one pupil out. stays is false when that was
// the last pupil and the family was dissolved.
func (c *StudentsClient) RemoveStudentFromFamily(ctx context.Context, familyID, studentID string) (family Family, stays bool, err error) {
	res, err := c.rpc.RemoveStudentFromFamily(ctx, connect.NewRequest(&studentsv1.RemoveStudentFromFamilyRequest{FamilyId: familyID, StudentId: studentID}))
	if err != nil {
		return Family{}, false, err
	}
	if res.Msg.GetFamily().GetId() == "" {
		return Family{}, false, nil
	}
	return toFamily(res.Msg.GetFamily()), true, nil
}

// DissolveFamily leaves every member without a family; no link is touched.
func (c *StudentsClient) DissolveFamily(ctx context.Context, familyID string) error {
	_, err := c.rpc.DissolveFamily(ctx, connect.NewRequest(&studentsv1.DissolveFamilyRequest{FamilyId: familyID}))
	return err
}

// SuggestFamilies proposes groups of pupils without a family, HIGH first.
func (c *StudentsClient) SuggestFamilies(ctx context.Context) ([]FamilySuggestion, error) {
	res, err := c.rpc.SuggestFamilies(ctx, connect.NewRequest(&studentsv1.SuggestFamiliesRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]FamilySuggestion, 0, len(res.Msg.GetSuggestions()))
	for _, s := range res.Msg.GetSuggestions() {
		reasons := make([]string, 0, len(s.GetReasons()))
		for _, r := range s.GetReasons() {
			if name, ok := suggestionReasonNames[r]; ok {
				reasons = append(reasons, name)
			}
		}
		out = append(out, FamilySuggestion{
			Signature: s.GetSignature(), ProposedName: s.GetProposedName(), StudentIDs: s.GetStudentIds(),
			Reasons: reasons, Confidence: suggestionConfidenceNames[s.GetConfidence()],
		})
	}
	return out, nil
}

// DismissFamilySuggestion records "not a family" for exactly these pupils, by
// userID — the authenticated caller, never a request field.
func (c *StudentsClient) DismissFamilySuggestion(ctx context.Context, studentIDs []string, userID string) error {
	_, err := c.rpc.DismissFamilySuggestion(ctx, connect.NewRequest(&studentsv1.DismissFamilySuggestionRequest{StudentIds: studentIDs, UserId: userID}))
	return err
}

// AddFamilyContact records one adult on every pupil of the family at once and
// returns one contact per pupil (the new one, or the one that already carried
// this email or phone).
func (c *StudentsClient) AddFamilyContact(ctx context.Context, in FamilyContactInput) ([]GuardianLink, error) {
	res, err := c.rpc.AddFamilyContact(ctx, connect.NewRequest(&studentsv1.AddFamilyContactRequest{
		FamilyId: in.FamilyID, Relationship: in.Relationship, FullName: in.FullName,
		Phone: in.Phone, Email: in.Email, IsEmergency: in.IsEmergency,
	}))
	if err != nil {
		return nil, err
	}
	return toGuardianLinks(res.Msg.GetContacts()), nil
}

// RevokeFamilyAccess takes one account away from every pupil of the family and
// returns how many contacts lost it. NotFound when no pupil carries it.
func (c *StudentsClient) RevokeFamilyAccess(ctx context.Context, familyID, guardianUserID string) (int, error) {
	res, err := c.rpc.RevokeFamilyAccess(ctx, connect.NewRequest(&studentsv1.RevokeFamilyAccessRequest{FamilyId: familyID, GuardianUserId: guardianUserID}))
	if err != nil {
		return 0, err
	}
	return int(res.Msg.GetContacts()), nil
}

// FamilyImportBuild is what building an import's families did: the families
// it created, the existing ones its pupils joined (as they now are), and how
// many groups it left to the office as suggestions.
type FamilyImportBuild struct {
	Created []Family
	Joined  []Family
	Skipped int
}

// FamilyImportUndo is what undoing an import's families did: pupils taken out,
// families dissolved, and families kept whole because a member has app access.
type FamilyImportUndo struct {
	Removed        int
	Dissolved      int
	KeptWithAccess int
}

// BuildFamiliesFromImport builds the very likely families of an import batch
// (PLAN-familias-importacion). Idempotent; a malformed batch is InvalidArgument.
func (c *StudentsClient) BuildFamiliesFromImport(ctx context.Context, batchID string) (FamilyImportBuild, error) {
	res, err := c.rpc.BuildFamiliesFromImport(ctx, connect.NewRequest(&studentsv1.BuildFamiliesFromImportRequest{BatchId: batchID}))
	if err != nil {
		return FamilyImportBuild{}, err
	}
	return FamilyImportBuild{
		Created: toFamilies(res.Msg.GetCreated()),
		Joined:  toFamilies(res.Msg.GetJoined()),
		Skipped: int(res.Msg.GetSkipped()),
	}, nil
}

// UndoFamiliesFromImport takes back what BuildFamiliesFromImport did for the
// batch. Idempotent.
func (c *StudentsClient) UndoFamiliesFromImport(ctx context.Context, batchID string) (FamilyImportUndo, error) {
	res, err := c.rpc.UndoFamiliesFromImport(ctx, connect.NewRequest(&studentsv1.UndoFamiliesFromImportRequest{BatchId: batchID}))
	if err != nil {
		return FamilyImportUndo{}, err
	}
	return FamilyImportUndo{
		Removed:        int(res.Msg.GetRemoved()),
		Dissolved:      int(res.Msg.GetDissolved()),
		KeptWithAccess: int(res.Msg.GetKeptWithAccess()),
	}, nil
}

func toFamilies(families []*studentsv1.Family) []Family {
	out := make([]Family, 0, len(families))
	for _, f := range families {
		out = append(out, toFamily(f))
	}
	return out
}

func toFamily(f *studentsv1.Family) Family {
	return Family{
		ID: f.GetId(), Name: f.GetName(), StudentIDs: f.GetStudentIds(),
		AccountCount: int(f.GetAccountCount()), CreatedAt: f.GetCreatedAt(),
	}
}

var suggestionReasonNames = map[studentsv1.FamilySuggestionReason]string{
	studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SAME_SURNAMES:        "same_surnames",
	studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_CONTACT_EMAIL: "shared_contact_email",
	studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_CONTACT_PHONE: "shared_contact_phone",
	studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_ACCOUNT:       "shared_account",
}

var suggestionConfidenceNames = map[studentsv1.FamilySuggestionConfidence]string{
	studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_MEDIUM: "medium",
	studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_HIGH:   "high",
}
