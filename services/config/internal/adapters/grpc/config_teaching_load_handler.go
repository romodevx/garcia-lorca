package grpcadapter

import (
	"context"
	"fmt"

	connect "connectrpc.com/connect"
	"github.com/google/uuid"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// The teaching-load RPCs check id FORMAT here, at the edge: a malformed id
// would otherwise reach Postgres as an invalid uuid literal and come back as an
// opaque internal error instead of an InvalidArgument naming the field. They
// also CANONICALIZE every id to lower case: uuids are case-insensitive and
// Postgres returns lower case, so the use case's duplicate check, snapshot
// lookups and "already yours" comparisons only hold on one spelling.

// AssignTeachingLoad gives one teacher a batch of slots in one transaction.
func (h *ConfigHandler) AssignTeachingLoad(ctx context.Context, req *connect.Request[configv1.AssignTeachingLoadRequest]) (*connect.Response[configv1.AssignTeachingLoadResponse], error) {
	in, err := toTeachingLoadRequest(req.Msg)
	if err != nil {
		return nil, connectErr(err)
	}
	res, err := h.catalog.AssignTeachingLoad(ctx, in)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(toProtoTeachingLoadResult(res)), nil
}

// DeleteTeachingAssignment frees a slot.
func (h *ConfigHandler) DeleteTeachingAssignment(ctx context.Context, req *connect.Request[configv1.DeleteTeachingAssignmentRequest]) (*connect.Response[configv1.DeleteTeachingAssignmentResponse], error) {
	id, err := canonicalUUID("id", req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	if err := h.catalog.DeleteTeachingAssignment(ctx, id); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteTeachingAssignmentResponse{}), nil
}

// ReassignTeachingAssignment hands an existing slot to another teacher.
func (h *ConfigHandler) ReassignTeachingAssignment(ctx context.Context, req *connect.Request[configv1.ReassignTeachingAssignmentRequest]) (*connect.Response[configv1.ReassignTeachingAssignmentResponse], error) {
	id, err := canonicalUUID("id", req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	teacherID, err := canonicalUUID("teacher_id", req.Msg.GetTeacherId())
	if err != nil {
		return nil, connectErr(err)
	}
	a, err := h.catalog.ReassignTeachingAssignment(ctx, id, teacherID)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.ReassignTeachingAssignmentResponse{TeachingAssignment: toProtoTeachingAssignment(a)}), nil
}

// toTeachingLoadRequest checks every id is a uuid, canonicalizes it, and maps
// the request inward. Emptiness and duplicates are the use case's to judge.
func toTeachingLoadRequest(msg *configv1.AssignTeachingLoadRequest) (usecase.TeachingLoadRequest, error) {
	teacherID, err := canonicalUUID("teacher_id", msg.GetTeacherId())
	if err != nil {
		return usecase.TeachingLoadRequest{}, err
	}
	items := make([]domain.TeachingSlot, 0, len(msg.GetItems()))
	for i, it := range msg.GetItems() {
		groupID, err := canonicalUUID(fmt.Sprintf("items[%d].group_id", i), it.GetGroupId())
		if err != nil {
			return usecase.TeachingLoadRequest{}, err
		}
		subjectID, err := canonicalUUID(fmt.Sprintf("items[%d].subject_id", i), it.GetSubjectId())
		if err != nil {
			return usecase.TeachingLoadRequest{}, err
		}
		items = append(items, domain.TeachingSlot{GroupID: groupID, SubjectID: subjectID})
	}
	return usecase.TeachingLoadRequest{
		TeacherID:        teacherID,
		Items:            items,
		ReplaceConflicts: msg.GetReplaceConflicts(),
	}, nil
}

func toProtoTeachingLoadResult(r usecase.TeachingLoadResult) *configv1.AssignTeachingLoadResponse {
	out := &configv1.AssignTeachingLoadResponse{
		Created:    make([]*configv1.TeachingAssignment, 0, len(r.Created)),
		Reassigned: make([]*configv1.TeachingAssignment, 0, len(r.Reassigned)),
		Skipped:    make([]*configv1.TeachingLoadItem, 0, len(r.Skipped)),
		Conflicts:  make([]*configv1.TeachingLoadConflict, 0, len(r.Conflicts)),
	}
	for _, a := range r.Created {
		out.Created = append(out.Created, toProtoTeachingAssignment(a))
	}
	for _, a := range r.Reassigned {
		out.Reassigned = append(out.Reassigned, toProtoTeachingAssignment(a))
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, &configv1.TeachingLoadItem{GroupId: s.GroupID, SubjectId: s.SubjectID})
	}
	for _, c := range r.Conflicts {
		out.Conflicts = append(out.Conflicts, &configv1.TeachingLoadConflict{
			GroupId:      c.Slot.GroupID,
			SubjectId:    c.Slot.SubjectID,
			AssignmentId: c.AssignmentID,
			TeacherId:    c.TeacherID,
			TeacherName:  c.TeacherName,
		})
	}
	return out
}

// canonicalUUIDLen is the length of the hyphenated 8-4-4-4-12 form. uuid.Parse
// also accepts urn:uuid: and braced forms; only the canonical one is on the wire.
const canonicalUUIDLen = 36

// canonicalUUID rejects an id that is not a hyphenated uuid, naming the field,
// and returns it in canonical lower case (the form Postgres returns).
func canonicalUUID(field, id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || len(id) != canonicalUUIDLen {
		return "", glerrors.New(glerrors.KindInvalid, field+" must be a valid uuid")
	}
	return parsed.String(), nil
}
