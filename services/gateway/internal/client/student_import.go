package client

import (
	"context"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// --- legacy student import (ADR 0033) ---------------------------------------

// PlacementInput is one (level, grade, group) an import needs. LevelID names an
// existing level; otherwise NewLevelName is matched (folded) or created.
type PlacementInput struct {
	LevelID      string
	NewLevelName string
	Ordinal      int
	Group        string
	Capacity     int
}

// PlacementResult answers one PlacementInput. With a dry run the ids of what
// would be created are empty. Error is a stable code ("level_not_found",
// "level_being_deleted", "invalid_placement") or "".
type PlacementResult struct {
	LevelID      string
	LevelName    string
	LevelCreated bool
	GradeLevelID string
	GradeName    string
	GradeCreated bool
	GroupID      string
	GroupName    string
	GroupCreated bool
	Capacity     int
	Error        string
}

// EnsurePlacements finds or creates the catalog an import needs, in one
// transaction (or, dry, reports what it would create).
func (c *ConfigClient) EnsurePlacements(ctx context.Context, cycleID string, in []PlacementInput, dryRun bool) ([]PlacementResult, error) {
	res, err := c.rpc.EnsurePlacements(ctx, connect.NewRequest(toEnsurePlacementsRequest(cycleID, in, dryRun)))
	if err != nil {
		return nil, err
	}
	return fromPlacements(res.Msg.GetPlacements()), nil
}

func toEnsurePlacementsRequest(cycleID string, in []PlacementInput, dryRun bool) *configv1.EnsurePlacementsRequest {
	req := &configv1.EnsurePlacementsRequest{CycleId: cycleID, DryRun: dryRun}
	for _, p := range in {
		req.Placements = append(req.Placements, &configv1.PlacementRequest{
			LevelId: p.LevelID, NewLevelName: p.NewLevelName,
			GradeOrdinal: int32(p.Ordinal), GroupName: p.Group, Capacity: int32(p.Capacity),
		})
	}
	return req
}

func fromPlacements(src []*configv1.Placement) []PlacementResult {
	out := make([]PlacementResult, 0, len(src))
	for _, p := range src {
		out = append(out, PlacementResult{
			LevelID: p.GetLevelId(), LevelName: p.GetLevelName(), LevelCreated: p.GetLevelCreated(),
			GradeLevelID: p.GetGradeLevelId(), GradeName: p.GetGradeName(), GradeCreated: p.GetGradeCreated(),
			GroupID: p.GetGroupId(), GroupName: p.GetGroupName(), GroupCreated: p.GetGroupCreated(),
			Capacity: int(p.GetCapacity()), Error: p.GetError(),
		})
	}
	return out
}

// ImportRowInput is one pupil of the legacy file, structured for students.
type ImportRowInput struct {
	RowNumber        int
	CURP             string
	FirstName        string
	LastName         string // paternal
	MaternalLastName string
	GroupID          string // "" only when validating, for a group not created yet
	BankReference    string
	Profile          StudentProfile
}

// Import row outcomes, as the edge names them.
const (
	ImportImported        = "imported"
	ImportAlreadyExists   = "already_exists"
	ImportAlreadyImported = "already_imported"
	ImportRejected        = "rejected"
)

// ImportRowResult is students' answer for one row.
type ImportRowResult struct {
	RowNumber        int
	Outcome          string
	StudentID        string
	EnrollmentNumber string
	Warnings         []string
	Reason           string
}

// ImportStudents creates (or, validating, judges) the pupils of a legacy file.
func (c *StudentsClient) ImportStudents(ctx context.Context, rows []ImportRowInput, validateOnly bool, batchID string) ([]ImportRowResult, error) {
	res, err := c.rpc.ImportStudents(ctx, connect.NewRequest(toImportStudentsRequest(rows, validateOnly, batchID)))
	if err != nil {
		return nil, err
	}
	return fromImportResults(res.Msg.GetResults()), nil
}

func toImportStudentsRequest(rows []ImportRowInput, validateOnly bool, batchID string) *studentsv1.ImportStudentsRequest {
	req := &studentsv1.ImportStudentsRequest{ValidateOnly: validateOnly, BatchId: batchID}
	for _, r := range rows {
		req.Rows = append(req.Rows, &studentsv1.ImportStudentRow{
			RowNumber: int32(r.RowNumber), Curp: r.CURP, FirstName: r.FirstName, LastName: r.LastName,
			MaternalLastName: r.MaternalLastName, GroupId: r.GroupID, BankReference: r.BankReference,
			Profile: toProtoProfile(r.Profile),
		})
	}
	return req
}

func fromImportResults(src []*studentsv1.ImportRowResult) []ImportRowResult {
	out := make([]ImportRowResult, 0, len(src))
	for _, r := range src {
		out = append(out, ImportRowResult{
			RowNumber: int(r.GetRowNumber()), Outcome: importOutcomeName(r.GetOutcome()),
			StudentID: r.GetStudentId(), EnrollmentNumber: r.GetEnrollmentNumber(),
			Warnings: r.GetWarnings(), Reason: r.GetReason(),
		})
	}
	return out
}

func importOutcomeName(o studentsv1.ImportRowOutcome) string {
	switch o {
	case studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_IMPORTED:
		return ImportImported
	case studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_ALREADY_EXISTS:
		return ImportAlreadyExists
	case studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_ALREADY_IMPORTED:
		return ImportAlreadyImported
	default:
		// An outcome this gateway does not know is not a success.
		return ImportRejected
	}
}
