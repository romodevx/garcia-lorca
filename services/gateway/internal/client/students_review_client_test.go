package client

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// PLAN-import-avisos: a pupil's open issues and draft state travel, and the
// draft status has its name (it read "" before).
func TestStudentResultCarriesTheImportReview(t *testing.T) {
	got := toStudentResult(&studentsv1.Student{Id: "stu-1", OpenIssues: 3, Draft: true,
		AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_DRAFT})
	if got.OpenIssues != 3 || !got.Draft || got.AdmissionStatus != "draft" {
		t.Errorf("student = %+v", got)
	}
	if admissionFromName("draft") != studentsv1.AdmissionStatus_ADMISSION_STATUS_DRAFT {
		t.Errorf("the draft filter does not reach students")
	}
}

func TestUpdateStudentCarriesTheActorAndAnswersThePromotion(t *testing.T) {
	req := toUpdateStudentRequest(UpdateStudentInput{ID: "stu-1", CURP: "", ActorUserID: "usr-7"})
	if req.GetActorUserId() != "usr-7" || req.GetId() != "stu-1" {
		t.Errorf("request = %+v", req)
	}
	got := fromUpdateStudentResponse(&studentsv1.UpdateStudentResponse{
		Student: &studentsv1.Student{Id: "stu-1"}, PromotedFromDraft: true, IntendedGroupId: "grp-1"})
	if got.ID != "stu-1" || !got.PromotedFromDraft || got.IntendedGroupID != "grp-1" {
		t.Errorf("result = %+v", got)
	}
}

func TestImportIssuesTravel(t *testing.T) {
	created := time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)
	got := toStudentImportIssue(&studentsv1.StudentImportIssue{
		Id: "iss-1", StudentId: "stu-1", Field: "curp", Code: "curp_missing", OriginalValue: "", StoredValue: "",
		CreatedAt: timestamppb.New(created), ResolvedAt: timestamppb.New(created.Add(time.Hour)),
		ResolvedBy: "usr-7", Resolution: "corrected", IntendedGroupId: "grp-1",
	})
	want := StudentImportIssue{ID: "iss-1", StudentID: "stu-1", Field: "curp", Code: "curp_missing",
		CreatedAt: created, ResolvedAt: created.Add(time.Hour), ResolvedBy: "usr-7", Resolution: "corrected", IntendedGroupID: "grp-1"}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.ResolvedAt.Equal(want.ResolvedAt) {
		t.Errorf("times = %v / %v", got.CreatedAt, got.ResolvedAt)
	}
	got.CreatedAt, got.ResolvedAt, want.CreatedAt, want.ResolvedAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	if got != want {
		t.Errorf("issue = %+v, want %+v", got, want)
	}
	if open := toStudentImportIssue(&studentsv1.StudentImportIssue{Id: "iss-2"}); !open.ResolvedAt.IsZero() {
		t.Errorf("an open issue has a resolution instant: %v", open.ResolvedAt)
	}
}
