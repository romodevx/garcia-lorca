package handler_test

import (
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// ADR 0041: a family holds at most two app accounts. students refuses the
// third, but only AFTER iam has minted the account and mailed the link — so
// the edge refuses it first, before iam is asked, and no invitation goes out
// for a grant that would be refused. It holds whatever module_families says:
// the cap is a data rule.

func granterInFamily(accounts int) *fakeGranter {
	students := granterWithContact(plainContact())
	students.student = client.StudentResult{ID: "stu-1", AdmissionStatus: "enrolled", FamilyID: "fam-1"}
	students.family = client.Family{StudentIDs: []string{"stu-1", "stu-2"}, AccountCount: accounts}
	return students
}

func TestInviteRefusesAThirdFamilyAccountBeforeAskingIAM(t *testing.T) {
	iam := issuerWithGuardianRole()
	students := granterInFamily(2)

	rec := doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, "FAMILY_ACCOUNT_LIMIT", rec.Header().Get("X-Error-Code"))
	assert.Contains(t, rec.Body.String(), "esta familia ya tiene 2 cuentas de la app")
	assert.Empty(t, iam.invitedEmail, "iam must not be asked: it would mail an invitation")
	assert.Empty(t, students.grantContactID)
}

func TestInviteIntoAFamilyWithRoomAsksIAM(t *testing.T) {
	iam := issuerWithGuardianRole()
	students := granterInFamily(1)

	rec := doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "mateo@example.com", iam.invitedEmail)
}

// A pupil without a family has no cap to check: the family is not read.
func TestInviteWithoutAFamilyDoesNotReadOne(t *testing.T) {
	iam := issuerWithGuardianRole()
	students := granterWithContact(plainContact())

	rec := doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Zero(t, students.familyCalls)
}

// The contact's address is one of the family's two accounts already (the
// mother typed again on this pupil): granting it is not a third account, so
// the edge leaves the decision to students.
func TestInviteOfAnAccountTheFamilyAlreadyHasIsNotAThird(t *testing.T) {
	iam := issuerWithGuardianRole()
	iam.users = []client.User{{ID: "usr-mateo", Email: "mateo@example.com"}}
	students := granterInFamily(2)
	students.links = append(students.links, client.GuardianLink{
		ID: "gl-0", StudentID: "stu-1", Email: "MATEO@example.com ", GuardianUserID: "usr-mateo",
	})
	students.granted.GuardianUserID = "usr-mateo"

	rec := doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "mateo@example.com", iam.invitedEmail)
}

// A family that vanished between the two reads leaves nothing to cap: the
// students service stays the authority. Any other failure stops the invite.
func TestInviteFamilyReadFailures(t *testing.T) {
	iam := issuerWithGuardianRole()
	students := granterInFamily(2)
	students.familyErr = connect.NewError(connect.CodeNotFound, nil)
	rec := doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	iam = issuerWithGuardianRole()
	students = granterInFamily(2)
	students.familyErr = connect.NewError(connect.CodeUnavailable, nil)
	rec = doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, iam.invitedEmail)
}
