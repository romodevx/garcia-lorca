package handler

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// FamilyStore is the students surface for families (ADR 0041).
// Consumer-defined; the StudentsClient implements it.
type FamilyStore interface {
	ListFamilies(ctx context.Context) ([]client.Family, error)
	GetFamily(ctx context.Context, familyID string) (client.Family, error)
	CreateFamily(ctx context.Context, name string, studentIDs []string) (client.Family, error)
	RenameFamily(ctx context.Context, familyID, name string) (client.Family, error)
	AddStudentsToFamily(ctx context.Context, familyID string, studentIDs []string) (client.Family, error)
	RemoveStudentFromFamily(ctx context.Context, familyID, studentID string) (client.Family, bool, error)
	DissolveFamily(ctx context.Context, familyID string) error
	SuggestFamilies(ctx context.Context) ([]client.FamilySuggestion, error)
	DismissFamilySuggestion(ctx context.Context, studentIDs []string, userID string) error
	AddFamilyContact(ctx context.Context, in client.FamilyContactInput) ([]client.GuardianLink, error)
	RevokeFamilyAccess(ctx context.Context, familyID, guardianUserID string) (int, error)
}

// FamilyMemberReader reads a family's pupils and their contacts.
type FamilyMemberReader interface {
	StudentsByIDs(ctx context.Context, ids []string) ([]client.StudentResult, error)
	ListGuardiansForStudent(ctx context.Context, studentID string) ([]client.GuardianLink, error)
}

// FamiliesService is everything the families edge asks students.
type FamiliesService interface {
	FamilyStore
	FamilyMemberReader
}

// FamiliesHandler serves /api/v1/families/*. Every route runs behind
// module_students AND module_families. Reading is students:read (the adults,
// guardians:read); renaming and dismissing a suggestion are students:update;
// creating, adding, removing, dissolving and the adults' access are
// guardians:write, because building a family hands its app accounts to every
// pupil in it (ADR 0041 Permisos, security gate H1).
type FamiliesHandler struct {
	svc FamiliesService
}

// NewFamiliesHandler wires the handler to students.
func NewFamiliesHandler(svc FamiliesService) *FamiliesHandler {
	return &FamiliesHandler{svc: svc}
}

const (
	// maxFamilyPupils mirrors the students service's cap on a family.
	maxFamilyPupils = 20
	// minSuggestionPupils: a suggestion groups at least two pupils.
	minSuggestionPupils = 2
	// contactReadConcurrency bounds the per-pupil contact reads.
	contactReadConcurrency = 4
)

type familyResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	StudentIDs   []string `json:"studentIds"`
	AccountCount int      `json:"accountCount"`
	CreatedAt    string   `json:"createdAt"`
}

type familyMemberResponse struct {
	ID               string `json:"id"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	MaternalLastName string `json:"maternalLastName"`
	Level            string `json:"level"`
	GradeGroup       string `json:"gradeGroup"`
	AdmissionStatus  string `json:"admissionStatus"`
	// Draft: imported without a CURP. A draft never receives the family's
	// accounts, which is why the screen has to say so.
	Draft bool `json:"draft"`
}

type familyDetailResponse struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	StudentIDs   []string               `json:"studentIds"`
	AccountCount int                    `json:"accountCount"`
	CreatedAt    string                 `json:"createdAt"`
	Members      []familyMemberResponse `json:"members"`
}

type familySuggestionResponse struct {
	Signature    string   `json:"signature"`
	ProposedName string   `json:"proposedName"`
	StudentIDs   []string `json:"studentIds"`
	Reasons      []string `json:"reasons"`
	Confidence   string   `json:"confidence"`
}

// familyContactRef is one contact row of the person: the (studentId,
// contactId) pair the existing invite route takes.
type familyContactRef struct {
	StudentID string `json:"studentId"`
	ContactID string `json:"contactId"`
}

// familyPersonResponse is one adult of the family, however many pupils carry
// them as a contact.
type familyPersonResponse struct {
	FullName       string             `json:"fullName"`
	Relationship   string             `json:"relationship"`
	Phone          string             `json:"phone"`
	Email          string             `json:"email"`
	GuardianUserID string             `json:"guardianUserId,omitempty"`
	HasApp         bool               `json:"hasApp"`
	Contacts       []familyContactRef `json:"contacts"`
}

func toFamilyResponse(f client.Family) familyResponse {
	ids := f.StudentIDs
	if ids == nil {
		ids = []string{}
	}
	return familyResponse{ID: f.ID, Name: f.Name, StudentIDs: ids, AccountCount: f.AccountCount, CreatedAt: f.CreatedAt}
}

// --- reads -----------------------------------------------------------------

// List handles GET /api/v1/families.
func (h *FamiliesHandler) List(w http.ResponseWriter, r *http.Request) {
	families, err := h.svc.ListFamilies(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]familyResponse, 0, len(families))
	for _, f := range families {
		out = append(out, toFamilyResponse(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"families": out})
}

// Get handles GET /api/v1/families/{familyId}: the family and its pupils,
// read in ONE ListStudents(ids) call and listed in the family's order.
func (h *FamiliesHandler) Get(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	family, err := h.svc.GetFamily(r.Context(), familyID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	pupils, err := h.svc.StudentsByIDs(r.Context(), family.StudentIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	base := toFamilyResponse(family)
	writeJSON(w, http.StatusOK, familyDetailResponse{
		ID: base.ID, Name: base.Name, StudentIDs: base.StudentIDs, AccountCount: base.AccountCount,
		CreatedAt: base.CreatedAt, Members: orderedMembers(family.StudentIDs, pupils),
	})
}

// orderedMembers lists the pupils in the family's order; a pupil students did
// not return is left out rather than invented.
func orderedMembers(order []string, pupils []client.StudentResult) []familyMemberResponse {
	byID := make(map[string]client.StudentResult, len(pupils))
	for _, p := range pupils {
		byID[p.ID] = p
	}
	out := make([]familyMemberResponse, 0, len(order))
	for _, id := range order {
		p, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, familyMemberResponse{
			ID: p.ID, FirstName: p.FirstName, LastName: p.LastName, MaternalLastName: p.MaternalLastName,
			Level: p.Level, GradeGroup: p.GradeGroup, AdmissionStatus: p.AdmissionStatus,
			Draft: p.Draft || p.AdmissionStatus == admissionDraft,
		})
	}
	return out
}

// Contacts handles GET /api/v1/families/{familyId}/contacts: the family's
// adults as PEOPLE. Each pupil's contacts are read (at most 20 pupils), and the
// rows that are the same person are folded into one entry carrying every
// (studentId, contactId) pair, adults with the app first.
func (h *FamiliesHandler) Contacts(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	family, err := h.svc.GetFamily(r.Context(), familyID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	perPupil, err := h.contactsOf(r.Context(), family.StudentIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": groupFamilyPeople(perPupil)})
}

// contactsOf reads each pupil's contacts concurrently, keeping the family's
// pupil order in the result.
func (h *FamiliesHandler) contactsOf(ctx context.Context, studentIDs []string) ([][]client.GuardianLink, error) {
	out := make([][]client.GuardianLink, len(studentIDs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(contactReadConcurrency)
	for i, id := range studentIDs {
		g.Go(func() error {
			links, err := h.svc.ListGuardiansForStudent(gctx, id)
			out[i] = links
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// familyPerson accumulates the contact rows of one adult.
type familyPerson struct {
	resp   familyPersonResponse
	emails map[string]bool
	phones map[string]bool
}

// groupFamilyPeople folds contact rows into people. Two rows are the same
// person when they carry the same account; otherwise when they carry the same
// email; otherwise — only when one of them has no email — the same phone. Two
// accounts are never folded, and two different emails sharing a home phone
// stay two people (the father and an aunt). Rows with an account are placed
// first, so every person with the app exists before a bare contact looks for
// its match.
func groupFamilyPeople(perPupil [][]client.GuardianLink) []familyPersonResponse {
	var withApp, without []client.GuardianLink
	for _, links := range perPupil {
		for _, l := range links {
			if l.GuardianUserID != "" {
				withApp = append(withApp, l)
			} else {
				without = append(without, l)
			}
		}
	}
	var people []*familyPerson
	for _, l := range append(withApp, without...) {
		p := findPerson(people, l)
		if p == nil {
			p = newFamilyPerson(l)
			people = append(people, p)
		}
		p.add(l)
	}
	out := make([]familyPersonResponse, 0, len(people))
	for _, p := range people {
		out = append(out, p.resp)
	}
	return out
}

func newFamilyPerson(l client.GuardianLink) *familyPerson {
	return &familyPerson{
		resp: familyPersonResponse{
			FullName: l.FullName, Relationship: l.Relationship, Phone: l.Phone, Email: l.Email,
			GuardianUserID: l.GuardianUserID, HasApp: l.GuardianUserID != "",
			Contacts: []familyContactRef{},
		},
		emails: map[string]bool{},
		phones: map[string]bool{},
	}
}

func (p *familyPerson) add(l client.GuardianLink) {
	p.resp.Contacts = append(p.resp.Contacts, familyContactRef{StudentID: l.StudentID, ContactID: l.ID})
	if e := normalEmail(l.Email); e != "" {
		p.emails[e] = true
	}
	if ph := normalPhone(l.Phone); ph != "" {
		p.phones[ph] = true
	}
}

func findPerson(people []*familyPerson, l client.GuardianLink) *familyPerson {
	for _, p := range people {
		if p.matches(l) {
			return p
		}
	}
	return nil
}

func (p *familyPerson) matches(l client.GuardianLink) bool {
	if l.GuardianUserID != "" {
		return p.resp.GuardianUserID == l.GuardianUserID
	}
	email := normalEmail(l.Email)
	if email != "" && len(p.emails) > 0 {
		return p.emails[email]
	}
	phone := normalPhone(l.Phone)
	return phone != "" && p.phones[phone]
}

func normalEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// normalPhone keeps the digits only: "55 1234 5678" and "5512345678" are one
// number.
func normalPhone(phone string) string {
	var b strings.Builder
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Suggestions handles GET /api/v1/families/suggestions.
func (h *FamiliesHandler) Suggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := h.svc.SuggestFamilies(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]familySuggestionResponse, 0, len(suggestions))
	for _, s := range suggestions {
		out = append(out, familySuggestionResponse{
			Signature: s.Signature, ProposedName: s.ProposedName, StudentIDs: s.StudentIDs,
			Reasons: s.Reasons, Confidence: s.Confidence,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": out})
}

// --- writes ----------------------------------------------------------------

type familyStudentsRequest struct {
	Name       string   `json:"name"`
	StudentIDs []string `json:"studentIds"`
}

// DismissSuggestion handles POST /api/v1/families/suggestions/dismiss. The
// dismissal is recorded for the signed-in caller; the body names only pupils.
func (h *FamiliesHandler) DismissSuggestion(w http.ResponseWriter, r *http.Request) {
	who, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ids, ok := decodeStudentIDs(w, r, minSuggestionPupils)
	if !ok {
		return
	}
	if err := h.svc.DismissFamilySuggestion(r.Context(), ids.StudentIDs, who.UserID); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Create handles POST /api/v1/families.
func (h *FamiliesHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeStudentIDs(w, r, 1)
	if !ok {
		return
	}
	family, err := h.svc.CreateFamily(r.Context(), req.Name, req.StudentIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toFamilyResponse(family))
}

// Rename handles PATCH /api/v1/families/{familyId}.
func (h *FamiliesHandler) Rename(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	family, err := h.svc.RenameFamily(r.Context(), familyID, req.Name)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFamilyResponse(family))
}

// AddStudents handles POST /api/v1/families/{familyId}/students.
func (h *FamiliesHandler) AddStudents(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	req, ok := decodeStudentIDs(w, r, 1)
	if !ok {
		return
	}
	family, err := h.svc.AddStudentsToFamily(r.Context(), familyID, req.StudentIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFamilyResponse(family))
}

// RemoveStudent handles DELETE /api/v1/families/{familyId}/students/{studentId}.
// The last pupil leaving dissolves the family: 204, nothing left to show.
func (h *FamiliesHandler) RemoveStudent(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	studentID, ok := pathUUID(w, r, "studentId")
	if !ok {
		return
	}
	family, stays, err := h.svc.RemoveStudentFromFamily(r.Context(), familyID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !stays {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, toFamilyResponse(family))
}

// Dissolve handles DELETE /api/v1/families/{familyId}.
func (h *FamiliesHandler) Dissolve(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	if err := h.svc.DissolveFamily(r.Context(), familyID); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddContact handles POST /api/v1/families/{familyId}/contacts: one adult on
// every pupil of the family at once.
func (h *FamiliesHandler) AddContact(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	var req struct {
		Relationship string `json:"relationship"`
		FullName     string `json:"fullName"`
		Phone        string `json:"phone"`
		Email        string `json:"email"`
		IsEmergency  bool   `json:"isEmergency"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	contacts, err := h.svc.AddFamilyContact(r.Context(), client.FamilyContactInput{
		FamilyID: familyID, Relationship: req.Relationship, FullName: req.FullName,
		Phone: req.Phone, Email: req.Email, IsEmergency: req.IsEmergency,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]guardianLinkResponse, 0, len(contacts))
	for _, c := range contacts {
		out = append(out, toGuardianLinkResponse(c))
	}
	writeJSON(w, http.StatusCreated, map[string]any{"contacts": out})
}

// RevokeAccess handles POST /api/v1/families/{familyId}/access/revoke: one
// account taken away from every pupil of the family.
func (h *FamiliesHandler) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	familyID, ok := pathUUID(w, r, "familyId")
	if !ok {
		return
	}
	var req struct {
		GuardianUserID string `json:"guardianUserId"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	guardianUserID, ok := canonicalUUID(req.GuardianUserID)
	if !ok {
		http.Error(w, "guardianUserId must be a uuid", http.StatusBadRequest)
		return
	}
	n, err := h.svc.RevokeFamilyAccess(r.Context(), familyID, guardianUserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"contacts": n})
}

// --- edge validation -------------------------------------------------------

// pathUUID reads a path id that must be a uuid; it answers 400 otherwise.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id, ok := canonicalUUID(r.PathValue(name))
	if !ok {
		http.Error(w, name+" must be a uuid", http.StatusBadRequest)
	}
	return id, ok
}

// decodeStudentIDs decodes {name?, studentIds} and checks studentIds holds
// between minIDs and maxFamilyPupils uuids, canonicalised.
func decodeStudentIDs(w http.ResponseWriter, r *http.Request, minIDs int) (familyStudentsRequest, bool) {
	var req familyStudentsRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return req, false
	}
	if len(req.StudentIDs) < minIDs || len(req.StudentIDs) > maxFamilyPupils {
		http.Error(w, "studentIds must hold between 1 and 20 pupils (2 to dismiss a suggestion)", http.StatusBadRequest)
		return req, false
	}
	for i, raw := range req.StudentIDs {
		id, ok := canonicalUUID(raw)
		if !ok {
			http.Error(w, "studentIds must be uuids", http.StatusBadRequest)
			return req, false
		}
		req.StudentIDs[i] = id
	}
	return req, true
}
