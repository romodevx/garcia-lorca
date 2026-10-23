package grpcadapter

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// WithFamilies returns the handler serving the family RPCs (ADR 0041) through
// `families`. Without it they answer Unimplemented. Permissions and the
// module_families flag are the gateway's call.
func (h *StudentsHandler) WithFamilies(families *usecase.FamilyService) *StudentsHandler {
	cp := *h
	cp.families = families
	return &cp
}

var errFamiliesNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("families are not served here"))

// familyErr maps a family use-case error. A conflict (a pupil already in a
// family) is FailedPrecondition, as the contract says; the rest as usual.
func familyErr(err error) *connect.Error {
	if glerrors.KindOf(err) == glerrors.KindConflict {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New(glerrors.SafeMessage(err)))
	}
	return connectErr(err)
}

// ListFamilies returns every family, ordered by name.
func (h *StudentsHandler) ListFamilies(ctx context.Context, _ *connect.Request[studentsv1.ListFamiliesRequest]) (*connect.Response[studentsv1.ListFamiliesResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	families, err := h.families.ListFamilies(ctx)
	if err != nil {
		return nil, familyErr(err)
	}
	out := make([]*studentsv1.Family, 0, len(families))
	for _, f := range families {
		out = append(out, toProtoFamily(f))
	}
	return connect.NewResponse(&studentsv1.ListFamiliesResponse{Families: out}), nil
}

// GetFamily returns one family; unknown is NotFound.
func (h *StudentsHandler) GetFamily(ctx context.Context, req *connect.Request[studentsv1.GetFamilyRequest]) (*connect.Response[studentsv1.GetFamilyResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	f, err := h.families.GetFamily(ctx, req.Msg.GetFamilyId())
	if err != nil {
		return nil, familyErr(err)
	}
	return connect.NewResponse(&studentsv1.GetFamilyResponse{Family: toProtoFamily(f)}), nil
}

// CreateFamily groups pupils without a family.
func (h *StudentsHandler) CreateFamily(ctx context.Context, req *connect.Request[studentsv1.CreateFamilyRequest]) (*connect.Response[studentsv1.CreateFamilyResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	f, err := h.families.CreateFamily(ctx, req.Msg.GetName(), req.Msg.GetStudentIds())
	if err != nil {
		return nil, familyErr(err)
	}
	return connect.NewResponse(&studentsv1.CreateFamilyResponse{Family: toProtoFamily(f)}), nil
}

// RenameFamily renames a family.
func (h *StudentsHandler) RenameFamily(ctx context.Context, req *connect.Request[studentsv1.RenameFamilyRequest]) (*connect.Response[studentsv1.RenameFamilyResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	f, err := h.families.RenameFamily(ctx, req.Msg.GetFamilyId(), req.Msg.GetName())
	if err != nil {
		return nil, familyErr(err)
	}
	return connect.NewResponse(&studentsv1.RenameFamilyResponse{Family: toProtoFamily(f)}), nil
}

// AddStudentsToFamily adds pupils without a family to an existing one.
func (h *StudentsHandler) AddStudentsToFamily(ctx context.Context, req *connect.Request[studentsv1.AddStudentsToFamilyRequest]) (*connect.Response[studentsv1.AddStudentsToFamilyResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	f, err := h.families.AddStudentsToFamily(ctx, req.Msg.GetFamilyId(), req.Msg.GetStudentIds())
	if err != nil {
		return nil, familyErr(err)
	}
	return connect.NewResponse(&studentsv1.AddStudentsToFamilyResponse{Family: toProtoFamily(f)}), nil
}

// RemoveStudentFromFamily takes one pupil out; the family is unset in the
// response when the last pupil left and the family was dissolved.
func (h *StudentsHandler) RemoveStudentFromFamily(ctx context.Context, req *connect.Request[studentsv1.RemoveStudentFromFamilyRequest]) (*connect.Response[studentsv1.RemoveStudentFromFamilyResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	f, dissolved, err := h.families.RemoveStudentFromFamily(ctx, req.Msg.GetFamilyId(), req.Msg.GetStudentId())
	if err != nil {
		return nil, familyErr(err)
	}
	res := &studentsv1.RemoveStudentFromFamilyResponse{}
	if !dissolved {
		res.Family = toProtoFamily(f)
	}
	return connect.NewResponse(res), nil
}

// DissolveFamily leaves every member without a family.
func (h *StudentsHandler) DissolveFamily(ctx context.Context, req *connect.Request[studentsv1.DissolveFamilyRequest]) (*connect.Response[studentsv1.DissolveFamilyResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	if err := h.families.DissolveFamily(ctx, req.Msg.GetFamilyId()); err != nil {
		return nil, familyErr(err)
	}
	return connect.NewResponse(&studentsv1.DissolveFamilyResponse{}), nil
}

// SuggestFamilies proposes groups of pupils that look like siblings.
func (h *StudentsHandler) SuggestFamilies(ctx context.Context, _ *connect.Request[studentsv1.SuggestFamiliesRequest]) (*connect.Response[studentsv1.SuggestFamiliesResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	suggestions, err := h.families.SuggestFamilies(ctx)
	if err != nil {
		return nil, familyErr(err)
	}
	out := make([]*studentsv1.FamilySuggestion, 0, len(suggestions))
	for _, s := range suggestions {
		out = append(out, toProtoFamilySuggestion(s))
	}
	return connect.NewResponse(&studentsv1.SuggestFamiliesResponse{Suggestions: out}), nil
}

// DismissFamilySuggestion records "not a family" for exactly these pupils.
func (h *StudentsHandler) DismissFamilySuggestion(ctx context.Context, req *connect.Request[studentsv1.DismissFamilySuggestionRequest]) (*connect.Response[studentsv1.DismissFamilySuggestionResponse], error) {
	if h.families == nil {
		return nil, errFamiliesNotServed
	}
	if err := h.families.DismissFamilySuggestion(ctx, req.Msg.GetStudentIds(), req.Msg.GetUserId()); err != nil {
		return nil, familyErr(err)
	}
	return connect.NewResponse(&studentsv1.DismissFamilySuggestionResponse{}), nil
}

func toProtoFamily(f domain.Family) *studentsv1.Family {
	return &studentsv1.Family{
		Id:           f.ID(),
		TenantId:     f.TenantID(),
		Name:         f.Name(),
		StudentIds:   f.StudentIDs(),
		AccountCount: int32(f.AccountCount()),
		CreatedAt:    f.CreatedAt().UTC().Format(time.RFC3339),
	}
}

func toProtoFamilySuggestion(s domain.FamilySuggestion) *studentsv1.FamilySuggestion {
	reasons := make([]studentsv1.FamilySuggestionReason, 0, len(s.Reasons))
	for _, r := range s.Reasons {
		reasons = append(reasons, protoconv.ToProtoSuggestionReason(r))
	}
	return &studentsv1.FamilySuggestion{
		Signature:    s.Signature,
		ProposedName: s.ProposedName,
		StudentIds:   s.StudentIDs,
		Reasons:      reasons,
		Confidence:   protoconv.ToProtoSuggestionConfidence(s.Confidence),
	}
}
