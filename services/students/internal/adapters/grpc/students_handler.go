package grpcadapter

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ studentsv1connect.StudentsServiceHandler = (*StudentsHandler)(nil)

// StudentsHandler adapts the students use cases to the generated
// garcialorca.students.v1 Connect service. It only translates: proto messages ↔
// use-case calls, and domain error Kinds ↔ Connect codes. All logic lives in
// usecase/domain.
type StudentsHandler struct {
	svc      *usecase.StudentService
	enroll   *usecase.EnrollmentService
	guardian *usecase.GuardianService
	health   *usecase.HealthService
	notes    *usecase.NoteService
	cards    *usecase.CardService
	levels   *usecase.LevelArchiveService
	imports  *usecase.ImportService
}

// NewStudentsHandler wires the handler to the student, enrollment, guardian, and
// health use cases.
func NewStudentsHandler(
	svc *usecase.StudentService,
	enroll *usecase.EnrollmentService,
	guardian *usecase.GuardianService,
	health *usecase.HealthService,
) *StudentsHandler {
	return &StudentsHandler{svc: svc, enroll: enroll, guardian: guardian, health: health}
}

// WithNotes returns the handler serving pupils' notes through `notes`. Without
// it the note RPCs answer Unimplemented.
func (h *StudentsHandler) WithNotes(notes *usecase.NoteService) *StudentsHandler {
	cp := *h
	cp.notes = notes
	return &cp
}

// WithCards returns the handler serving the pupils' card codes through `cards`
// (task 13). Without it the card RPCs answer Unimplemented.
func (h *StudentsHandler) WithCards(cards *usecase.CardService) *StudentsHandler {
	cp := *h
	cp.cards = cards
	return &cp
}

func (h *StudentsHandler) Health(ctx context.Context, _ *connect.Request[studentsv1.HealthRequest]) (*connect.Response[studentsv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.HealthResponse{Status: status.Status}), nil
}

func (h *StudentsHandler) CreateApplicant(ctx context.Context, req *connect.Request[studentsv1.CreateApplicantRequest]) (*connect.Response[studentsv1.CreateApplicantResponse], error) {
	m := req.Msg
	student, err := h.svc.CreateApplicant(ctx, usecase.CreateApplicantInput{
		CURP:             m.GetCurp(),
		FirstName:        m.GetFirstName(),
		LastName:         m.GetLastName(),
		MaternalLastName: m.GetMaternalLastName(),
		LevelID:          m.GetLevelId(),
		GradeGroup:       m.GetGradeGroup(),
		GuardianName:     m.GetGuardianName(),
		BirthDate:        m.GetBirthDate(),
		BankReference:    m.GetBankReference(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.CreateApplicantResponse{Student: toProtoStudent(student)}), nil
}

// UpdateStudent corrects a pupil's file (names, CURP, profile).
func (h *StudentsHandler) UpdateStudent(ctx context.Context, req *connect.Request[studentsv1.UpdateStudentRequest]) (*connect.Response[studentsv1.UpdateStudentResponse], error) {
	m := req.Msg
	student, err := h.svc.UpdateDetails(ctx, usecase.UpdateDetailsInput{
		ID:               m.GetId(),
		FirstName:        m.GetFirstName(),
		LastName:         m.GetLastName(),
		MaternalLastName: m.GetMaternalLastName(),
		CURP:             m.GetCurp(),
		Profile:          toDomainProfileInput(m.GetProfile()),
		BankReference:    m.BankReference, // nil keeps the stored one
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.UpdateStudentResponse{Student: toProtoStudent(student)}), nil
}

// GetStudentMedical returns a pupil's medical record.
func (h *StudentsHandler) GetStudentMedical(ctx context.Context, req *connect.Request[studentsv1.GetStudentMedicalRequest]) (*connect.Response[studentsv1.GetStudentMedicalResponse], error) {
	rec, err := h.svc.GetMedical(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetStudentMedicalResponse{Medical: toProtoMedical(rec)}), nil
}

// UpdateStudentMedical saves a pupil's medical record.
func (h *StudentsHandler) UpdateStudentMedical(ctx context.Context, req *connect.Request[studentsv1.UpdateStudentMedicalRequest]) (*connect.Response[studentsv1.UpdateStudentMedicalResponse], error) {
	m := req.Msg.GetMedical()
	rec, err := h.svc.UpdateMedical(ctx, req.Msg.GetStudentId(), m.GetBloodType(), m.GetAllergies(), m.GetConditions())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.UpdateStudentMedicalResponse{Medical: toProtoMedical(rec)}), nil
}

func toProtoMedical(r domain.MedicalRecord) *studentsv1.StudentMedical {
	return &studentsv1.StudentMedical{BloodType: string(r.BloodType()), Allergies: r.Allergies(), Conditions: r.Conditions()}
}

// UploadStudentPhoto stores (or replaces) a pupil's photo.
func (h *StudentsHandler) UploadStudentPhoto(ctx context.Context, req *connect.Request[studentsv1.UploadStudentPhotoRequest]) (*connect.Response[studentsv1.UploadStudentPhotoResponse], error) {
	ref, err := h.svc.UploadPhoto(ctx, req.Msg.GetStudentId(), req.Msg.GetData())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.UploadStudentPhotoResponse{ContentType: ref.ContentType, SizeBytes: int32(ref.Size)}), nil
}

// GetStudentPhoto returns a pupil's photo, or NotFound when there is none.
func (h *StudentsHandler) GetStudentPhoto(ctx context.Context, req *connect.Request[studentsv1.GetStudentPhotoRequest]) (*connect.Response[studentsv1.GetStudentPhotoResponse], error) {
	photo, err := h.svc.Photo(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetStudentPhotoResponse{Data: photo.Data(), ContentType: photo.ContentType()}), nil
}

// DeleteStudentPhoto removes a pupil's photo.
func (h *StudentsHandler) DeleteStudentPhoto(ctx context.Context, req *connect.Request[studentsv1.DeleteStudentPhotoRequest]) (*connect.Response[studentsv1.DeleteStudentPhotoResponse], error) {
	if err := h.svc.DeletePhoto(ctx, req.Msg.GetStudentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.DeleteStudentPhotoResponse{}), nil
}

func (h *StudentsHandler) GetStudent(ctx context.Context, req *connect.Request[studentsv1.GetStudentRequest]) (*connect.Response[studentsv1.GetStudentResponse], error) {
	student, err := h.svc.GetStudent(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetStudentResponse{Student: toProtoStudent(student)}), nil
}

func (h *StudentsHandler) ListStudents(ctx context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	m := req.Msg
	students, next, err := h.svc.ListStudents(ctx, ports.ListFilter{
		LevelID:    m.GetLevelId(),
		Status:     protoconv.ToDomainStatus(m.GetStatus()),
		GradeGroup: m.GetGradeGroup(),
		IDs:        m.GetIds(),
		PageSize:   int(m.GetPageSize()),
		PageToken:  m.GetPageToken(),
		Birthday:   toBirthdayFilter(m.GetBirthday()),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.Student, 0, len(students))
	for _, s := range students {
		out = append(out, toProtoStudent(s))
	}
	return connect.NewResponse(&studentsv1.ListStudentsResponse{Students: out, NextPageToken: next}), nil
}

func (h *StudentsHandler) ChangeAdmissionStatus(ctx context.Context, req *connect.Request[studentsv1.ChangeAdmissionStatusRequest]) (*connect.Response[studentsv1.ChangeAdmissionStatusResponse], error) {
	m := req.Msg
	student, err := h.svc.ChangeAdmissionStatus(ctx, usecase.ChangeStatusInput{
		ID:      m.GetId(),
		Target:  protoconv.ToDomainStatus(m.GetTarget()),
		Reason:  m.GetReason(),
		CycleID: m.GetCycleId(),
	})
	if err != nil {
		// The frozen contract maps an illegal admission transition (KindConflict
		// here) to FailedPrecondition — the resource is in the wrong state — as
		// opposed to a duplicate on CreateApplicant, which is AlreadyExists.
		if glerrors.KindOf(err) == glerrors.KindConflict {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ChangeAdmissionStatusResponse{Student: toProtoStudent(student)}), nil
}

// Enroll places a student into a group of a cycle. The cycle is DERIVED
// server-side from the group (never a client arg); only student_id, group_id and
// the optional term_id are passed through. A duplicate (student, cycle) surfaces
// as AlreadyExists via the shared error mapper.
func (h *StudentsHandler) Enroll(ctx context.Context, req *connect.Request[studentsv1.EnrollRequest]) (*connect.Response[studentsv1.EnrollResponse], error) {
	m := req.Msg
	enrollment, err := h.enroll.Enroll(ctx, m.GetStudentId(), m.GetGroupId(), m.GetTermId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.EnrollResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

// EnrollIntoGrade places a pupil in a grade and lets capacity choose the group.
func (h *StudentsHandler) EnrollIntoGrade(ctx context.Context, req *connect.Request[studentsv1.EnrollIntoGradeRequest]) (*connect.Response[studentsv1.EnrollIntoGradeResponse], error) {
	m := req.Msg
	placed, err := h.enroll.EnrollIntoGrade(ctx, m.GetStudentId(), m.GetGradeLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.EnrollIntoGradeResponse{
		Enrollment: toProtoEnrollment(placed.Enrollment),
		GroupName:  placed.GroupName,
	}), nil
}

func (h *StudentsHandler) GetEnrollment(ctx context.Context, req *connect.Request[studentsv1.GetEnrollmentRequest]) (*connect.Response[studentsv1.GetEnrollmentResponse], error) {
	enrollment, err := h.enroll.GetEnrollment(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetEnrollmentResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

func (h *StudentsHandler) ListEnrollments(ctx context.Context, req *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	m := req.Msg
	enrollments, err := h.enroll.ListEnrollments(ctx, m.GetStudentId(), m.GetCycleId(), m.GetGroupId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.Enrollment, 0, len(enrollments))
	for _, e := range enrollments {
		out = append(out, toProtoEnrollment(e))
	}
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: out}), nil
}

// TransferStudent moves an active enrollment to another group in the SAME cycle.
// A non-active enrollment, a cross-cycle destination, or a same-group move are
// all InvalidArgument via the shared mapper.
func (h *StudentsHandler) TransferStudent(ctx context.Context, req *connect.Request[studentsv1.TransferStudentRequest]) (*connect.Response[studentsv1.TransferStudentResponse], error) {
	m := req.Msg
	enrollment, err := h.enroll.Transfer(ctx, m.GetEnrollmentId(), m.GetToGroupId(), m.GetReason())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.TransferStudentResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

// ListTransfers returns a pupil's group moves (student_id) or every move into
// a group (to_group_id), oldest first. Exactly one filter must be named; a
// malformed id is InvalidArgument.
func (h *StudentsHandler) ListTransfers(ctx context.Context, req *connect.Request[studentsv1.ListTransfersRequest]) (*connect.Response[studentsv1.ListTransfersResponse], error) {
	var (
		transfers []domain.EnrollmentTransfer
		err       error
	)
	studentID, groupID := req.Msg.GetStudentId(), req.Msg.GetToGroupId()
	switch {
	case groupID != "" && studentID != "":
		err = glerrors.New(glerrors.KindInvalid, "name student_id or to_group_id, not both")
	case groupID != "":
		transfers, err = h.enroll.ListTransfersIntoGroup(ctx, groupID)
	default:
		transfers, err = h.enroll.ListTransfers(ctx, studentID)
	}
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.EnrollmentTransfer, 0, len(transfers))
	for _, t := range transfers {
		out = append(out, &studentsv1.EnrollmentTransfer{
			Id:           t.ID(),
			EnrollmentId: t.EnrollmentID(),
			FromGroupId:  t.FromGroupID(),
			ToGroupId:    t.ToGroupID(),
			Reason:       t.Reason(),
			MovedAt:      timestamppb.New(t.MovedAt()),
		})
	}
	return connect.NewResponse(&studentsv1.ListTransfersResponse{Transfers: out}), nil
}

func (h *StudentsHandler) WithdrawEnrollment(ctx context.Context, req *connect.Request[studentsv1.WithdrawEnrollmentRequest]) (*connect.Response[studentsv1.WithdrawEnrollmentResponse], error) {
	var withdrawnOn time.Time
	if raw := req.Msg.GetWithdrawnOn(); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("withdrawn_on must be YYYY-MM-DD"))
		}
		withdrawnOn = parsed
	}
	enrollment, err := h.enroll.Withdraw(ctx, req.Msg.GetId(), withdrawnOn, req.Msg.GetReason())
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindConflict {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(glerrors.SafeMessage(err)))
		}
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.WithdrawEnrollmentResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

// --- guardian links --------------------------------------------------------

// LinkGuardian links an iam user (validated by RPC at the use-case layer) to a
// student. A guardian_user_id that is unknown to iam, or that does not hold the
// guardian role, is an InvalidArgument — not a NotFound: the resource addressed by
// the call is the student, and the user id is an argument. A duplicate link
// surfaces as AlreadyExists via the shared error mapper.
func (h *StudentsHandler) LinkGuardian(ctx context.Context, req *connect.Request[studentsv1.LinkGuardianRequest]) (*connect.Response[studentsv1.LinkGuardianResponse], error) {
	m := req.Msg
	link, err := h.guardian.LinkGuardian(ctx, m.GetGuardianUserId(), m.GetStudentId(), m.GetRelationship())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.LinkGuardianResponse{Link: toProtoGuardianLink(link)}), nil
}

// AddContact records somebody the school can reach about a pupil, with no
// account.
func (h *StudentsHandler) AddContact(ctx context.Context, req *connect.Request[studentsv1.AddContactRequest]) (*connect.Response[studentsv1.AddContactResponse], error) {
	m := req.Msg
	contact, err := h.guardian.AddContact(ctx, usecase.ContactInput{
		StudentID:    m.GetStudentId(),
		Relationship: m.GetRelationship(),
		FullName:     m.GetFullName(),
		Phone:        m.GetPhone(),
		Email:        m.GetEmail(),
		IsEmergency:  m.GetIsEmergency(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.AddContactResponse{Contact: toProtoGuardianLink(contact)}), nil
}

// UpdateContact corrects a contact of a pupil.
func (h *StudentsHandler) UpdateContact(ctx context.Context, req *connect.Request[studentsv1.UpdateContactRequest]) (*connect.Response[studentsv1.UpdateContactResponse], error) {
	m := req.Msg
	contact, err := h.guardian.UpdateContact(ctx, m.GetContactId(), m.GetStudentId(), usecase.ContactInput{
		Relationship: m.GetRelationship(),
		FullName:     m.GetFullName(),
		Phone:        m.GetPhone(),
		Email:        m.GetEmail(),
		IsEmergency:  m.GetIsEmergency(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.UpdateContactResponse{Contact: toProtoGuardianLink(contact)}), nil
}

// RemoveContact deletes a contact of a pupil that has no app access.
func (h *StudentsHandler) RemoveContact(ctx context.Context, req *connect.Request[studentsv1.RemoveContactRequest]) (*connect.Response[studentsv1.RemoveContactResponse], error) {
	if err := h.guardian.RemoveContact(ctx, req.Msg.GetContactId(), req.Msg.GetStudentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.RemoveContactResponse{}), nil
}

// GrantContactAccess attaches an iam account to a contact the school already
// recorded, so that family can open the app. The account must hold the guardian
// role and be ACTIVE — a user that does not is an InvalidArgument, since the
// resource the call addresses is the contact and the user id is an argument.
//
// A contact that belongs to a different pupil is NotFound, deliberately: the
// same answer as a contact that does not exist, so the reply never confirms that
// somebody else's contact is there.
func (h *StudentsHandler) GrantContactAccess(ctx context.Context, req *connect.Request[studentsv1.GrantContactAccessRequest]) (*connect.Response[studentsv1.GrantContactAccessResponse], error) {
	m := req.Msg
	contact, err := h.guardian.GrantContactAccess(ctx, m.GetContactId(), m.GetStudentId(), m.GetGuardianUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GrantContactAccessResponse{Contact: toProtoGuardianLink(contact)}), nil
}

// UnlinkGuardian removes a guardian ↔ student link. An unknown link is NotFound.
func (h *StudentsHandler) UnlinkGuardian(ctx context.Context, req *connect.Request[studentsv1.UnlinkGuardianRequest]) (*connect.Response[studentsv1.UnlinkGuardianResponse], error) {
	m := req.Msg
	if err := h.guardian.UnlinkGuardian(ctx, m.GetGuardianUserId(), m.GetStudentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.UnlinkGuardianResponse{}), nil
}

func (h *StudentsHandler) ListGuardiansForStudent(ctx context.Context, req *connect.Request[studentsv1.ListGuardiansForStudentRequest]) (*connect.Response[studentsv1.ListGuardiansForStudentResponse], error) {
	links, err := h.guardian.ListGuardiansForStudent(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ListGuardiansForStudentResponse{Links: toProtoGuardianLinks(links)}), nil
}

func (h *StudentsHandler) ListStudentsForGuardian(ctx context.Context, req *connect.Request[studentsv1.ListStudentsForGuardianRequest]) (*connect.Response[studentsv1.ListStudentsForGuardianResponse], error) {
	links, err := h.guardian.ListStudentsForGuardian(ctx, req.Msg.GetGuardianUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ListStudentsForGuardianResponse{Links: toProtoGuardianLinks(links)}), nil
}

// --- mapping ---------------------------------------------------------------

func toProtoStudent(s *domain.Student) *studentsv1.Student {
	return &studentsv1.Student{
		Id:               s.ID(),
		TenantId:         s.TenantID(),
		Curp:             s.CURP().String(),
		FirstName:        s.FirstName(),
		LastName:         s.LastName(),
		MaternalLastName: s.MaternalLastName(),
		LevelId:          s.Level().ID(),
		LevelName:        s.Level().Name(),
		GradeGroup:       s.GradeGroup(),
		GuardianName:     s.GuardianName(),
		AdmissionStatus:  protoconv.ToProtoStatus(s.Status()),
		Profile:          toProtoProfile(s.Profile()),
		EnrollmentNumber: s.EnrollmentNumber(),
		BankReference:    s.BankReference().String(),
	}
}

func toDomainProfileInput(p *studentsv1.StudentProfile) domain.ProfileInput {
	phones := make([]domain.Phone, 0, len(p.GetPhones()))
	for _, ph := range p.GetPhones() {
		phones = append(phones, domain.Phone{Kind: domain.PhoneKind(ph.GetKind()), Number: ph.GetNumber()})
	}
	a := p.GetAddress()
	return domain.ProfileInput{
		BirthDate: p.GetBirthDate(),
		Sex:       p.GetSex(),
		Email:     p.GetEmail(),
		Phones:    phones,
		Address: domain.Address{
			Street: a.GetStreet(), ExtNumber: a.GetExtNumber(), IntNumber: a.GetIntNumber(),
			Neighborhood: a.GetNeighborhood(), PostalCode: a.GetPostalCode(),
			Municipality: a.GetMunicipality(), State: a.GetState(),
		},
	}
}

func toProtoProfile(p domain.Profile) *studentsv1.StudentProfile {
	phones := make([]*studentsv1.StudentPhone, 0, len(p.Phones()))
	for _, ph := range p.Phones() {
		phones = append(phones, &studentsv1.StudentPhone{Kind: string(ph.Kind), Number: ph.Number})
	}
	born := ""
	if !p.BirthDate().IsZero() {
		born = p.BirthDate().Format("2006-01-02")
	}
	a := p.Address()
	return &studentsv1.StudentProfile{
		BirthDate: born,
		Sex:       string(p.Sex()),
		Email:     p.Email(),
		Phones:    phones,
		Address: &studentsv1.StudentAddress{
			Street: a.Street, ExtNumber: a.ExtNumber, IntNumber: a.IntNumber,
			Neighborhood: a.Neighborhood, PostalCode: a.PostalCode,
			Municipality: a.Municipality, State: a.State,
		},
	}
}

func toProtoEnrollment(e domain.Enrollment) *studentsv1.Enrollment {
	return &studentsv1.Enrollment{
		Id:         e.ID(),
		TenantId:   e.TenantID(),
		StudentId:  e.StudentID(),
		CycleId:    e.CycleID(),
		GroupId:    e.GroupID(),
		TermId:     e.TermID(),
		Status:     protoconv.ToProtoEnrollmentStatus(e.Status()),
		EnrolledAt: timestamppb.New(e.EnrolledAt()),
		EndedAt:    optionalTimestamp(e.EndedAt()),
		Origin:     string(e.Origin()),
		WithdrawnOn: func() string {
			if e.WithdrawnOn().IsZero() {
				return ""
			}
			return e.WithdrawnOn().Format("2006-01-02")
		}(),
		WithdrawalReason: e.WithdrawalReason(),
	}
}

// optionalTimestamp is nil for the zero time (an unset field on the wire).
func optionalTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// toProtoGuardianLink projects a domain GuardianLink onto the wire. The
// relationship travels as its canonical name (the DB CHECK vocabulary).
func toProtoGuardianLink(l domain.GuardianLink) *studentsv1.GuardianLink {
	return &studentsv1.GuardianLink{
		Id:             l.ID(),
		TenantId:       l.TenantID(),
		GuardianUserId: l.GuardianUserID(),
		StudentId:      l.StudentID(),
		Relationship:   l.Relationship().String(),
		FullName:       l.FullName(),
		Phone:          l.Phone(),
		Email:          l.Email(),
		IsEmergency:    l.IsEmergency(),
	}
}

func toProtoGuardianLinks(links []domain.GuardianLink) []*studentsv1.GuardianLink {
	out := make([]*studentsv1.GuardianLink, 0, len(links))
	for _, l := range links {
		out = append(out, toProtoGuardianLink(l))
	}
	return out
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks); the other Kinds
// carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	ce := connectErrForKind(err)
	if code := levelarchive.CodeOf(err); code != "" {
		ce.Meta().Set(ErrorCodeMeta, code)
	}
	return ce
}

// ErrorCodeMeta is the error-metadata key a refusal's machine code rides in
// (e.g. LEVEL_BEING_DELETED); the gateway forwards it as the X-Error-Code
// response header.
const ErrorCodeMeta = "X-Error-Code"

func connectErrForKind(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}

// toBirthdayFilter maps the wire's optional birthday filter; unset stays nil
// (any day).
func toBirthdayFilter(b *studentsv1.BirthdayFilter) *ports.BirthdayFilter {
	if b == nil {
		return nil
	}
	return &ports.BirthdayFilter{Month: int(b.GetMonth()), Day: int(b.GetDay()), IncludeFeb29: b.GetIncludeFeb_29()}
}
