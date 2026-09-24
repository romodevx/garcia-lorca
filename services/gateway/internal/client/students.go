package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
)

// StudentsClient is the gateway's typed client for the students service over the
// generated Connect client. Handlers depend on its small methods and the plain
// structs below, never on the generated proto types.
type StudentsClient struct {
	rpc studentsv1connect.StudentsServiceClient
}

// CreateApplicantInput is the edge's view of a new-applicant request. LevelID is
// a config level id; the students service resolves it against config and
// supplies the level's name (ADR 0014).
type CreateApplicantInput struct {
	CURP         string
	FirstName    string
	LastName     string
	LevelID      string // a config level id; the service resolves its name
	GradeGroup   string
	GuardianName string
}

// StudentResult is the edge's view of a student, with enums flattened to their
// canonical lowercase names for JSON.
type StudentResult struct {
	ID              string
	CURP            string
	FirstName       string
	LastName        string
	Level           string // the level's name as the school wrote it
	LevelID         string // the config level id
	GradeGroup      string
	GuardianName    string
	AdmissionStatus string
	Profile         StudentProfile
}

// StudentProfile is the editable half of a pupil's file, as the edge speaks it.
type StudentProfile struct {
	BirthDate string         `json:"birthDate"`
	Sex       string         `json:"sex"`
	Email     string         `json:"email"`
	Phones    []StudentPhone `json:"phones"`
	Address   StudentAddress `json:"address"`
}

// StudentPhone is one of the pupil's numbers.
type StudentPhone struct {
	Kind   string `json:"kind"`
	Number string `json:"number"`
}

// StudentAddress is where the pupil lives.
type StudentAddress struct {
	Street       string `json:"street"`
	ExtNumber    string `json:"extNumber"`
	IntNumber    string `json:"intNumber"`
	Neighborhood string `json:"neighborhood"`
	PostalCode   string `json:"postalCode"`
	Municipality string `json:"municipality"`
	State        string `json:"state"`
}

// UpdateStudentInput is a correction of a pupil's file.
type UpdateStudentInput struct {
	ID        string
	FirstName string
	LastName  string
	CURP      string
	Profile   StudentProfile
}

// Enrollment is the edge's view of a student's placement in a group of a cycle,
// with the wire status enum flattened to its canonical lowercase name
// ("active"/"transferred"/"withdrawn") and the timestamp as time.Time. CycleID
// is DERIVED server-side from the group at enroll time — it is never a client
// argument.
type Enrollment struct {
	ID         string
	StudentID  string
	CycleID    string
	GroupID    string
	TermID     string
	Status     string
	EnrolledAt time.Time
}

// NewStudentsClient builds a StudentsClient targeting the students base URL over
// httpClient.
func NewStudentsClient(baseURL string, httpClient connect.HTTPClient) *StudentsClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &StudentsClient{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// CreateApplicant registers a new applicant via the students service. Connect
// error codes propagate (AlreadyExists for a duplicate CURP, InvalidArgument for
// bad input) so the handler maps them to HTTP status.
func (c *StudentsClient) CreateApplicant(ctx context.Context, in CreateApplicantInput) (StudentResult, error) {
	res, err := c.rpc.CreateApplicant(ctx, connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp:         in.CURP,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		LevelId:      in.LevelID,
		GradeGroup:   in.GradeGroup,
		GuardianName: in.GuardianName,
	}))
	if err != nil {
		return StudentResult{}, err
	}
	return toStudentResult(res.Msg.GetStudent()), nil
}

// ListStudents lists students, optionally filtered by admission status (""
// = all). Pagination and the level/grade filters are not exposed at the edge
// yet; the students service returns its default page. Connect error codes
// propagate so the handler maps them to HTTP status.
// listStudentsPageSize is the per-page size the gateway requests (the students
// service caps at 200). The web resolves student names from the full roster, so
// ListStudents follows the page token to return every student, not just page 1 —
// otherwise a school with more than one page shows raw ids where names belong.
const listStudentsPageSize = 200

// listStudentsMaxPages bounds the pagination loop so a misbehaving page token can
// never spin forever (200 × 100 = 20k students, far beyond any single school).
const listStudentsMaxPages = 100

func (c *StudentsClient) ListStudents(ctx context.Context, status string) ([]StudentResult, error) {
	var out []StudentResult
	pageToken := ""
	for page := 0; page < listStudentsMaxPages; page++ {
		res, err := c.rpc.ListStudents(ctx, connect.NewRequest(&studentsv1.ListStudentsRequest{
			Status:    admissionFromName(status),
			PageSize:  listStudentsPageSize,
			PageToken: pageToken,
		}))
		if err != nil {
			return nil, err
		}
		for _, s := range res.Msg.GetStudents() {
			out = append(out, toStudentResult(s))
		}
		pageToken = res.Msg.GetNextPageToken()
		if pageToken == "" {
			return out, nil
		}
	}
	// The token is still non-empty after the page cap: the roster is larger than
	// this loop can safely drain. Fail loudly instead of silently truncating —
	// a partial list that looks complete is worse than a clear error.
	return nil, fmt.Errorf("listing students: exceeded %d pages (%d per page) with more results pending", listStudentsMaxPages, listStudentsPageSize)
}

// GetStudent fetches a single student by id. Connect error codes propagate
// (NotFound for an unknown id) so the handler maps them to HTTP status.
func (c *StudentsClient) GetStudent(ctx context.Context, id string) (StudentResult, error) {
	res, err := c.rpc.GetStudent(ctx, connect.NewRequest(&studentsv1.GetStudentRequest{Id: id}))
	if err != nil {
		return StudentResult{}, err
	}
	return toStudentResult(res.Msg.GetStudent()), nil
}

// --- enrollments ----------------------------------------------------------

// Enroll places a student into a group of a cycle. The server derives the cycle
// from the group, so cycleID is never sent; the returned Enrollment carries it.
// termID may be empty. Connect error codes propagate (InvalidArgument for a
// missing group / term-not-in-cycle, AlreadyExists for a duplicate enrollment).
func (c *StudentsClient) Enroll(ctx context.Context, studentID, groupID, termID string) (Enrollment, error) {
	res, err := c.rpc.Enroll(ctx, connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID,
		GroupId:   groupID,
		TermId:    termID,
	}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// Placement is where a pupil landed: the enrolment plus the group the school's
// own capacity chose, by name.
type Placement struct {
	Enrollment Enrollment `json:"enrollment"`
	GroupName  string     `json:"groupName"`
}

// EnrollIntoGrade places a pupil in a grade and lets capacity pick the group.
func (c *StudentsClient) EnrollIntoGrade(ctx context.Context, studentID, gradeLevelID string) (Placement, error) {
	res, err := c.rpc.EnrollIntoGrade(ctx, connect.NewRequest(&studentsv1.EnrollIntoGradeRequest{
		StudentId: studentID, GradeLevelId: gradeLevelID,
	}))
	if err != nil {
		return Placement{}, err
	}
	return Placement{
		Enrollment: toEnrollment(res.Msg.GetEnrollment()),
		GroupName:  res.Msg.GetGroupName(),
	}, nil
}

// ListEnrollments lists enrollments filtered by any of studentID, cycleID,
// groupID ("" = no filter on that dimension).
func (c *StudentsClient) ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]Enrollment, error) {
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(&studentsv1.ListEnrollmentsRequest{
		StudentId: studentID,
		CycleId:   cycleID,
		GroupId:   groupID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]Enrollment, 0, len(res.Msg.GetEnrollments()))
	for _, e := range res.Msg.GetEnrollments() {
		out = append(out, toEnrollment(e))
	}
	return out, nil
}

// GetEnrollment fetches a single enrollment by id.
func (c *StudentsClient) GetEnrollment(ctx context.Context, id string) (Enrollment, error) {
	res, err := c.rpc.GetEnrollment(ctx, connect.NewRequest(&studentsv1.GetEnrollmentRequest{Id: id}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// TransferStudent moves an active enrollment to another group in the same cycle,
// recording a dated, reasoned history row. Connect error codes propagate
// (InvalidArgument for a non-active enrollment / cross-cycle or same group).
func (c *StudentsClient) TransferStudent(ctx context.Context, enrollmentID, toGroupID, reason string) (Enrollment, error) {
	res, err := c.rpc.TransferStudent(ctx, connect.NewRequest(&studentsv1.TransferStudentRequest{
		EnrollmentId: enrollmentID,
		ToGroupId:    toGroupID,
		Reason:       reason,
	}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// WithdrawEnrollment marks an active enrollment as withdrawn.
func (c *StudentsClient) WithdrawEnrollment(ctx context.Context, id string) (Enrollment, error) {
	res, err := c.rpc.WithdrawEnrollment(ctx, connect.NewRequest(&studentsv1.WithdrawEnrollmentRequest{Id: id}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// --- guardian links -------------------------------------------------------

// GuardianLink is the edge's view of a guardian↔student link. GuardianUserID
// references an iam user, which the students service validates by RPC (services
// own their own tables, so it is not an FK). Relationship is one of
// mother|father|guardian|other.
type GuardianLink struct {
	ID string
	// GuardianUserID is empty for a contact with no account: a grandmother the
	// school can call, rather than a family that can open the app.
	GuardianUserID string
	TenantID       string
	StudentID      string
	Relationship   string
	FullName       string
	Phone          string
	Email          string
	IsEmergency    bool
}

// ContactInput is a contact as the office typed it.
type ContactInput struct {
	StudentID    string
	Relationship string
	FullName     string
	Phone        string
	Email        string
	IsEmergency  bool
}

// LinkGuardian links a guardian (an iam user) to a student. Connect error codes
// propagate so the handler maps them to HTTP status: AlreadyExists for a
// duplicate link, and InvalidArgument for an unknown user, an unknown student, a
// user without the guardian role, or a bad relationship — the students service
// validates every reference in the request as an argument, so none of them is a
// NotFound.
func (c *StudentsClient) LinkGuardian(ctx context.Context, guardianUserID, studentID, relationship string) (GuardianLink, error) {
	res, err := c.rpc.LinkGuardian(ctx, connect.NewRequest(&studentsv1.LinkGuardianRequest{
		GuardianUserId: guardianUserID,
		StudentId:      studentID,
		Relationship:   relationship,
	}))
	if err != nil {
		return GuardianLink{}, err
	}
	return toGuardianLink(res.Msg.GetLink()), nil
}

// AddContact records somebody the school can reach about a pupil, with no
// account and no login.
func (c *StudentsClient) AddContact(ctx context.Context, in ContactInput) (GuardianLink, error) {
	res, err := c.rpc.AddContact(ctx, connect.NewRequest(&studentsv1.AddContactRequest{
		StudentId:    in.StudentID,
		Relationship: in.Relationship,
		FullName:     in.FullName,
		Phone:        in.Phone,
		Email:        in.Email,
		IsEmergency:  in.IsEmergency,
	}))
	if err != nil {
		return GuardianLink{}, err
	}
	return toGuardianLink(res.Msg.GetContact()), nil
}

// UpdateContact corrects a contact of a pupil (in.StudentID scopes it).
func (c *StudentsClient) UpdateContact(ctx context.Context, contactID string, in ContactInput) (GuardianLink, error) {
	res, err := c.rpc.UpdateContact(ctx, connect.NewRequest(&studentsv1.UpdateContactRequest{
		ContactId:    contactID,
		StudentId:    in.StudentID,
		Relationship: in.Relationship,
		FullName:     in.FullName,
		Phone:        in.Phone,
		Email:        in.Email,
		IsEmergency:  in.IsEmergency,
	}))
	if err != nil {
		return GuardianLink{}, err
	}
	return toGuardianLink(res.Msg.GetContact()), nil
}

// RemoveContact deletes a contact of a pupil that has no app access.
func (c *StudentsClient) RemoveContact(ctx context.Context, contactID, studentID string) error {
	_, err := c.rpc.RemoveContact(ctx, connect.NewRequest(&studentsv1.RemoveContactRequest{
		ContactId: contactID, StudentId: studentID,
	}))
	return err
}

// GrantContactAccess attaches an iam account to a contact the pupil already has,
// so that family can open the app. studentID is not decoration: the students
// service checks the contact belongs to it and answers NotFound when it does
// not, which is what keeps a contact of another family out of reach.
func (c *StudentsClient) GrantContactAccess(ctx context.Context, contactID, studentID, guardianUserID string) (GuardianLink, error) {
	res, err := c.rpc.GrantContactAccess(ctx, connect.NewRequest(&studentsv1.GrantContactAccessRequest{
		ContactId:      contactID,
		StudentId:      studentID,
		GuardianUserId: guardianUserID,
	}))
	if err != nil {
		return GuardianLink{}, err
	}
	return toGuardianLink(res.Msg.GetContact()), nil
}

// UnlinkGuardian removes a guardian↔student link. It is NOT idempotent: the
// students service answers NotFound when the pair is not linked, which the edge
// surfaces as 404 so an admin revoking access learns there was nothing to revoke.
func (c *StudentsClient) UnlinkGuardian(ctx context.Context, guardianUserID, studentID string) error {
	_, err := c.rpc.UnlinkGuardian(ctx, connect.NewRequest(&studentsv1.UnlinkGuardianRequest{
		GuardianUserId: guardianUserID,
		StudentId:      studentID,
	}))
	return err
}

// ListGuardiansForStudent lists the guardians linked to one student — the admin
// view of "who may see this child".
func (c *StudentsClient) ListGuardiansForStudent(ctx context.Context, studentID string) ([]GuardianLink, error) {
	res, err := c.rpc.ListGuardiansForStudent(ctx, connect.NewRequest(&studentsv1.ListGuardiansForStudentRequest{
		StudentId: studentID,
	}))
	if err != nil {
		return nil, err
	}
	return toGuardianLinks(res.Msg.GetLinks()), nil
}

// ListStudentsForGuardian lists the students linked to one guardian. This is the
// scoping primitive of the family surface: the caller's own children, derived
// from the authenticated principal, never from the request.
func (c *StudentsClient) ListStudentsForGuardian(ctx context.Context, guardianUserID string) ([]GuardianLink, error) {
	res, err := c.rpc.ListStudentsForGuardian(ctx, connect.NewRequest(&studentsv1.ListStudentsForGuardianRequest{
		GuardianUserId: guardianUserID,
	}))
	if err != nil {
		return nil, err
	}
	return toGuardianLinks(res.Msg.GetLinks()), nil
}

func toGuardianLinks(links []*studentsv1.GuardianLink) []GuardianLink {
	out := make([]GuardianLink, 0, len(links))
	for _, l := range links {
		out = append(out, toGuardianLink(l))
	}
	return out
}

func toGuardianLink(l *studentsv1.GuardianLink) GuardianLink {
	return GuardianLink{
		ID:             l.GetId(),
		TenantID:       l.GetTenantId(),
		GuardianUserID: l.GetGuardianUserId(),
		StudentID:      l.GetStudentId(),
		Relationship:   l.GetRelationship(),
		FullName:       l.GetFullName(),
		Phone:          l.GetPhone(),
		Email:          l.GetEmail(),
		IsEmergency:    l.GetIsEmergency(),
	}
}

func toEnrollment(e *studentsv1.Enrollment) Enrollment {
	return Enrollment{
		ID:         e.GetId(),
		StudentID:  e.GetStudentId(),
		CycleID:    e.GetCycleId(),
		GroupID:    e.GetGroupId(),
		TermID:     e.GetTermId(),
		Status:     enrollmentStatusName(e.GetStatus()),
		EnrolledAt: timeFromTS(e.GetEnrolledAt()),
	}
}

func toStudentResult(s *studentsv1.Student) StudentResult {
	return StudentResult{
		ID:              s.GetId(),
		CURP:            s.GetCurp(),
		FirstName:       s.GetFirstName(),
		LastName:        s.GetLastName(),
		Level:           s.GetLevelName(),
		LevelID:         s.GetLevelId(),
		GradeGroup:      s.GetGradeGroup(),
		GuardianName:    s.GetGuardianName(),
		AdmissionStatus: admissionName(s.GetAdmissionStatus()),
		Profile:         fromProtoProfile(s.GetProfile()),
	}
}

func fromProtoProfile(p *studentsv1.StudentProfile) StudentProfile {
	phones := make([]StudentPhone, 0, len(p.GetPhones()))
	for _, ph := range p.GetPhones() {
		phones = append(phones, StudentPhone{Kind: ph.GetKind(), Number: ph.GetNumber()})
	}
	a := p.GetAddress()
	return StudentProfile{
		BirthDate: p.GetBirthDate(), Sex: p.GetSex(), Email: p.GetEmail(), Phones: phones,
		Address: StudentAddress{
			Street: a.GetStreet(), ExtNumber: a.GetExtNumber(), IntNumber: a.GetIntNumber(),
			Neighborhood: a.GetNeighborhood(), PostalCode: a.GetPostalCode(),
			Municipality: a.GetMunicipality(), State: a.GetState(),
		},
	}
}

func toProtoProfile(p StudentProfile) *studentsv1.StudentProfile {
	phones := make([]*studentsv1.StudentPhone, 0, len(p.Phones))
	for _, ph := range p.Phones {
		phones = append(phones, &studentsv1.StudentPhone{Kind: ph.Kind, Number: ph.Number})
	}
	a := p.Address
	return &studentsv1.StudentProfile{
		BirthDate: p.BirthDate, Sex: p.Sex, Email: p.Email, Phones: phones,
		Address: &studentsv1.StudentAddress{
			Street: a.Street, ExtNumber: a.ExtNumber, IntNumber: a.IntNumber,
			Neighborhood: a.Neighborhood, PostalCode: a.PostalCode,
			Municipality: a.Municipality, State: a.State,
		},
	}
}

// StudentMedical is a pupil's medical record, as the edge speaks it.
type StudentMedical struct {
	BloodType  string `json:"bloodType"`
	Allergies  string `json:"allergies"`
	Conditions string `json:"conditions"`
}

// GetStudentMedical reads a pupil's medical record.
func (c *StudentsClient) GetStudentMedical(ctx context.Context, studentID string) (StudentMedical, error) {
	res, err := c.rpc.GetStudentMedical(ctx, connect.NewRequest(&studentsv1.GetStudentMedicalRequest{StudentId: studentID}))
	if err != nil {
		return StudentMedical{}, err
	}
	return fromProtoMedical(res.Msg.GetMedical()), nil
}

// UpdateStudentMedical saves a pupil's medical record.
func (c *StudentsClient) UpdateStudentMedical(ctx context.Context, studentID string, rec StudentMedical) (StudentMedical, error) {
	res, err := c.rpc.UpdateStudentMedical(ctx, connect.NewRequest(&studentsv1.UpdateStudentMedicalRequest{
		StudentId: studentID,
		Medical:   &studentsv1.StudentMedical{BloodType: rec.BloodType, Allergies: rec.Allergies, Conditions: rec.Conditions},
	}))
	if err != nil {
		return StudentMedical{}, err
	}
	return fromProtoMedical(res.Msg.GetMedical()), nil
}

func fromProtoMedical(m *studentsv1.StudentMedical) StudentMedical {
	return StudentMedical{BloodType: m.GetBloodType(), Allergies: m.GetAllergies(), Conditions: m.GetConditions()}
}

// UpdateStudent corrects a pupil's file. InvalidArgument (a malformed field)
// and AlreadyExists (a CURP another pupil holds) propagate for the handler.
func (c *StudentsClient) UpdateStudent(ctx context.Context, in UpdateStudentInput) (StudentResult, error) {
	res, err := c.rpc.UpdateStudent(ctx, connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: in.ID, FirstName: in.FirstName, LastName: in.LastName, Curp: in.CURP,
		Profile: toProtoProfile(in.Profile),
	}))
	if err != nil {
		return StudentResult{}, err
	}
	return toStudentResult(res.Msg.GetStudent()), nil
}

var admissionNames = map[studentsv1.AdmissionStatus]string{
	studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING:   "pending",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED:  "accepted",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED:  "enrolled",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED:  "rejected",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN: "withdrawn",
}

func admissionName(s studentsv1.AdmissionStatus) string { return admissionNames[s] }

// admissionFromName is the reverse of admissionName; an empty or unknown name
// maps to UNSPECIFIED, which the students service treats as "no status filter".
func admissionFromName(name string) studentsv1.AdmissionStatus {
	for status, n := range admissionNames {
		if n == name {
			return status
		}
	}
	return studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED
}

var enrollmentStatusNames = map[studentsv1.EnrollmentStatus]string{
	studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE:      "active",
	studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED: "transferred",
	studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN:   "withdrawn",
}

func enrollmentStatusName(s studentsv1.EnrollmentStatus) string { return enrollmentStatusNames[s] }
