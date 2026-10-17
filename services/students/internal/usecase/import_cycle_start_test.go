package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// Owner decision (2026-10-17, T14): an imported pupil is enrolled from the
// FIRST day of the cycle, not the day of the import — they were in school
// since September and are billed from it like everybody else.

// The first day is read on the school's calendar: 00:00 in Morelia, so
// finance and attendance, which read the day in the school's zone, see the
// cycle's own first day and not the evening before.
func TestImportEnrollsFromTheCycleStartInTheSchoolsZone(t *testing.T) {
	mexico, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	f := newImportFixture(t)
	f.svc = f.svc.WithSchoolZone(mexico)

	f.run(t, false, row(1, importCURP))

	want := time.Date(2026, 8, 24, 0, 0, 0, 0, mexico)
	if got := f.repo.imported[0].Enrollment.EnrolledAt(); !got.Equal(want) {
		t.Fatalf("enrolled_at = %v; want %v, the cycle's first day in the school's zone", got, want)
	}
}

// An import run before the cycle opens is not dated in the future: the pupil
// is enrolled from the import, which billing and attendance already read as
// the whole cycle.
func TestImportBeforeTheCycleOpensIsDatedNow(t *testing.T) {
	early := time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)
	f := newImportFixtureAt(t, early)

	f.run(t, false, row(1, importCURP))

	if got := f.repo.imported[0].Enrollment.EnrolledAt(); !got.Equal(early) {
		t.Fatalf("enrolled_at = %v; want the import's own instant %v", got, early)
	}
}

// Config is asked about each cycle once, however many groups and pupils it holds.
func TestImportAsksConfigOncePerCycle(t *testing.T) {
	f := newImportFixture(t)
	counting := &cycleCountingCatalog{countingCatalog: f.catalog, calls: map[string]int{}}
	f.svc = usecase.NewImportService(f.repo, counting, fakeLevels{primariaLevel: "Primaria"}, tenant,
		sequentialIDs(), func() time.Time { return importNow })
	other := row(3, "MAPC070101HDFRNN01")
	other.GroupID = importGroupB

	f.run(t, false, row(1, importCURP), row(2, importCURPTwo), other)

	if counting.calls[importCycle] != 1 {
		t.Fatalf("cycle lookups = %v; want one per distinct cycle", counting.calls)
	}
}

// A group whose cycle config does not know cannot place a pupil.
func TestImportRejectsAGroupWhoseCycleIsUnknown(t *testing.T) {
	f := newImportFixture(t)
	delete(f.catalog.cycles, importCycle)

	got := f.run(t, false, row(1, importCURP))[0]

	if got.Outcome != usecase.ImportRejected || got.Reason != usecase.ReasonGroupNotFound {
		t.Fatalf("result = %+v; want REJECTED group_not_found", got)
	}
}

// Uploading the file again repairs the pupils an earlier import dated on its
// own day: their import enrollment in the row's cycle moves to the cycle's
// first day. The store decides who moves (origin import, same cycle, later
// than the first day); the use case asks once per cycle and reports it per row.
func TestReimportMovesEarlierImportsToTheCycleStart(t *testing.T) {
	f := newImportFixture(t)
	f.repo.existing[importCURP] = ports.ExistingPupil{StudentID: "stu-moved", ActiveGroupIDs: []string{importGroupA}}
	f.repo.existing[importCURPTwo] = ports.ExistingPupil{StudentID: "stu-fine", ActiveGroupIDs: []string{importGroupA}}
	f.repo.movable["stu-moved"] = true

	results := f.run(t, false, row(1, importCURP), row(2, importCURPTwo))

	if !results[0].MovedToCycleStart || results[1].MovedToCycleStart {
		t.Fatalf("moved = (%v, %v); want only the pupil dated after the cycle start", results[0].MovedToCycleStart, results[1].MovedToCycleStart)
	}
	if results[0].Outcome != usecase.ImportAlreadyImported {
		t.Fatalf("outcome = %v; a repaired pupil is still ALREADY_IMPORTED", results[0].Outcome)
	}
	want := []ports.CycleStartMove{{CycleID: importCycle, StartsAt: importCycleStart, StudentIDs: []string{"stu-moved", "stu-fine"}}}
	if !reflect.DeepEqual(f.repo.realigns, want) {
		t.Fatalf("moves asked = %+v; want %+v", f.repo.realigns, want)
	}

	again := f.run(t, false, row(1, importCURP), row(2, importCURPTwo))
	if again[0].MovedToCycleStart || again[1].MovedToCycleStart {
		t.Fatal("a second upload moved somebody again; the repair must be idempotent")
	}
}

// The preview says who WOULD move, and moves nobody: a file whose pupils are
// all already in can still be committed to repair them.
func TestReimportPreviewReportsTheMoveWithoutWriting(t *testing.T) {
	f := newImportFixture(t)
	f.repo.existing[importCURP] = ports.ExistingPupil{StudentID: "stu-moved", ActiveGroupIDs: []string{importGroupA}}
	f.repo.movable["stu-moved"] = true

	got := f.run(t, true, row(1, importCURP))[0]

	if !got.MovedToCycleStart {
		t.Fatalf("result = %+v; want the would-be move reported", got)
	}
	if len(f.repo.realigns) != 1 || !f.repo.realigns[0].DryRun || !f.repo.movable["stu-moved"] {
		t.Fatalf("moves = %+v; want one dry run that leaves the pupil in place", f.repo.realigns)
	}
}

// Rows the import does not place are not repaired: a pupil the file sends to a
// group config does not know, or a new pupil (the insert already dates it).
func TestReimportOnlyAsksAboutPlacedExistingPupils(t *testing.T) {
	f := newImportFixture(t)
	f.repo.existing[importCURP] = ports.ExistingPupil{StudentID: "stu-lost"}
	lost := row(1, importCURP)
	lost.GroupID = "4a000000-0000-0000-0000-0000000000ff"

	f.run(t, false, lost, row(2, importCURPTwo))

	if len(f.repo.realigns) != 0 {
		t.Fatalf("moves asked = %+v; want none", f.repo.realigns)
	}
}

// Who was moved, and by which upload, is on record: the batch id and the moved
// pupils' ids — ids only, never a name or a CURP.
func TestReimportLogsTheBatchAndTheMovedIDs(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	f := newImportFixture(t)
	f.repo.existing[importCURP] = ports.ExistingPupil{StudentID: "stu-moved", ActiveGroupIDs: []string{importGroupA}}
	f.repo.movable["stu-moved"] = true

	f.run(t, true, row(1, importCURP))
	if buf.Len() != 0 {
		t.Fatalf("a preview logged a move: %s", buf.String())
	}
	f.run(t, false, row(1, importCURP))

	got := buf.String()
	for _, want := range []string{importBatch, importCycle, "stu-moved"} {
		if !strings.Contains(got, want) {
			t.Errorf("log %s lacks %q", got, want)
		}
	}
	if strings.Contains(got, importCURP) || strings.Contains(got, "Hernández") {
		t.Errorf("the log carries personal data: %s", got)
	}
}

func TestReimportStopsWhenTheMoveFails(t *testing.T) {
	f := newImportFixture(t)
	f.repo.existing[importCURP] = ports.ExistingPupil{StudentID: "stu-moved", ActiveGroupIDs: []string{importGroupA}}
	f.repo.moveErr = errors.New("db down")

	_, err := f.svc.ImportStudents(context.Background(), []usecase.ImportRow{row(1, importCURP), row(2, importCURPTwo)}, false, importBatch)

	if err == nil || len(f.repo.imported) != 0 {
		t.Fatalf("err = %v, stored %d; want the failure reported before anything else is written", err, len(f.repo.imported))
	}
}

// --- helpers ---------------------------------------------------------------

type cycleCountingCatalog struct {
	*countingCatalog
	calls map[string]int
}

func (c *cycleCountingCatalog) CycleByID(ctx context.Context, id string) (ports.CycleRef, error) {
	c.calls[id]++
	return c.countingCatalog.CycleByID(ctx, id)
}

func sequentialIDs() func() string {
	ids := 0
	return func() string { ids++; return fmt.Sprintf("00000000-0000-0000-0000-%012d", ids) }
}
