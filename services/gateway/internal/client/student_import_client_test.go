package client

import (
	"slices"
	"testing"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

func TestToEnsurePlacementsRequestCarriesEveryPlacement(t *testing.T) {
	req := toEnsurePlacementsRequest("cyc-1", []PlacementInput{
		{LevelID: "lv-1", Ordinal: 3, Group: "A", Capacity: 30},
		{NewLevelName: "Maternal", Ordinal: 1, Group: "B", Capacity: 9},
	}, true)

	if req.GetCycleId() != "cyc-1" || !req.GetDryRun() || len(req.GetPlacements()) != 2 {
		t.Fatalf("request = %+v", req)
	}
	p := req.GetPlacements()[1]
	if p.GetNewLevelName() != "Maternal" || p.GetGradeOrdinal() != 1 || p.GetGroupName() != "B" || p.GetCapacity() != 9 {
		t.Errorf("placement = %+v", p)
	}
}

func TestFromPlacementsKeepsTheErrorCode(t *testing.T) {
	got := fromPlacements([]*configv1.Placement{
		{LevelId: "lv-1", GroupId: "g-1", GroupCreated: true, Capacity: 30},
		{Error: "level_being_deleted"},
	})
	if !got[0].GroupCreated || got[0].Capacity != 30 || got[1].Error != "level_being_deleted" {
		t.Errorf("placements = %+v", got)
	}
}

func TestImportRowsTravelAndComeBack(t *testing.T) {
	req := toImportStudentsRequest([]ImportRowInput{{
		RowNumber: 7, CURP: "AAMI210219MJCLLRA0", FirstName: "IRLANDA", LastName: "ALCAZAR",
		MaternalLastName: "MELENDEZ", GroupID: "g-1", BankReference: "231076",
		Profile: StudentProfile{Sex: "F", BirthDate: "2021-02-19"},
	}}, true, "batch-1")
	r := req.GetRows()[0]
	if !req.GetValidateOnly() || req.GetBatchId() != "batch-1" || r.GetRowNumber() != 7 ||
		r.GetBankReference() != "231076" || r.GetProfile().GetSex() != "F" {
		t.Errorf("request = %+v", req)
	}

	got := fromImportResults([]*studentsv1.ImportRowResult{
		{RowNumber: 7, Outcome: studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_IMPORTED, EnrollmentNumber: "2026-00042",
			Warnings: []string{"phone_invalid"}},
		{RowNumber: 8, Outcome: studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_ALREADY_IMPORTED},
		{RowNumber: 9, Outcome: studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_UNSPECIFIED},
	})
	if got[0].Outcome != ImportImported || got[0].EnrollmentNumber != "2026-00042" || !slices.Equal(got[0].Warnings, []string{"phone_invalid"}) {
		t.Errorf("row 7 = %+v", got[0])
	}
	if got[1].Outcome != ImportAlreadyImported {
		t.Errorf("row 8 = %+v", got[1])
	}
	if got[2].Outcome != ImportRejected {
		t.Errorf("an unknown outcome must not read as success: %+v", got[2])
	}
}
