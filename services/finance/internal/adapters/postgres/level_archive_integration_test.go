//go:build integration

// Integration tests for finance's half of the level trash (ADR 0021): a
// deleted level's tuition plans move to finance_deletion_archive and back,
// while money — concepts, charges, payments, deposits, folio counters and the
// outbox — stays byte-identical through the archive AND the restore.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
)

// Tenant H owns every row of this file, so the suite's other tests never see it.
const tenantH = "88888888-8888-8888-8888-888888888888"

const (
	flLevel    = "f5000000-0000-0000-0000-0000000000a1"
	flStudent  = "f5000000-0000-0000-0000-0000000000a2"
	flCycle    = "f5000000-0000-0000-0000-0000000000c1"
	flCycle2   = "f5000000-0000-0000-0000-0000000000c2"
	flConcept  = "f5000000-0000-0000-0000-0000000000c3"
	flCharge   = "f5000000-0000-0000-0000-0000000000c4"
	flCharge2  = "f5000000-0000-0000-0000-0000000000c5"
	byLevel    = "f5000000-0000-0000-0000-0000000000b1"
	byStudent  = "f5000000-0000-0000-0000-0000000000b2"
	flDeposit  = "f5000000-0000-0000-0000-0000000000c6"
	byChargeID = "f5000000-0000-0000-0000-0000000000b4"
)

var flScope = levelarchive.Scope{LevelID: flLevel, StudentIDs: []string{flStudent}}

// moneyTables must never change because of a level deletion.
var moneyTables = []string{
	"charge_concepts", "charges", "payments", "deposits", "deposit_folio_counters", "finance_outbox",
}

// levelRepo opens a pool for one test and closes it with the test.
func levelRepo(t *testing.T, tenant string) *pgadapter.Repository {
	t.Helper()
	db, err := open(appDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return pgadapter.NewRepository(db, tenant)
}

func seedFinanceLevel(t *testing.T) {
	t.Helper()
	for _, table := range append([]string{"finance_deletion_archive", "tuition_plans", "payments", "charges", "deposits", "charge_concepts"}, "finance_outbox") {
		require.NoError(t, superDB.Exec("DELETE FROM "+table+" WHERE tenant_id = ?", tenantH).Error, table)
	}
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tuition_plans (tenant_id, cycle_id, level_id, enrollment_cents, monthly_cents, months, first_billed_month, due_day)
		  VALUES (?, ?, ?, 100000, 250000, 10, 9, 10), (?, ?, ?, 90000, 240000, 10, 9, 10), (?, ?, ?, 80000, 200000, 10, 9, 10)`,
			[]any{tenantH, flCycle, flLevel, tenantH, flCycle2, flLevel, tenantH, flCycle, byLevel}},
		{`INSERT INTO charge_concepts (id, tenant_id, name, default_amount_cents) VALUES (?, ?, 'Colegiatura', 250000)`,
			[]any{flConcept, tenantH}},
		{`INSERT INTO charges (id, tenant_id, student_id, cycle_id, concept_id, description, amount_cents, due_date, status)
		  VALUES (?, ?, ?, ?, ?, 'Colegiatura septiembre', 250000, '2026-09-10', 'partially_paid'),
		         (?, ?, ?, ?, ?, 'Colegiatura septiembre', 250000, '2026-09-10', 'pending'),
		         (?, ?, ?, ?, ?, 'Colegiatura septiembre', 200000, '2026-09-10', 'pending')`,
			[]any{flCharge, tenantH, flStudent, flCycle, flConcept,
				flCharge2, tenantH, flStudent, flCycle2, flConcept,
				byChargeID, tenantH, byStudent, flCycle, flConcept}},
		{`INSERT INTO deposits (id, tenant_id, student_id, amount_cents, method, folio, paid_on) VALUES (?, ?, ?, 100000, 'cash', 'P-2026-000001', CURRENT_DATE)`,
			[]any{flDeposit, tenantH, flStudent}},
		{`INSERT INTO payments (tenant_id, charge_id, amount_cents, method, deposit_id, paid_on) VALUES (?, ?, 100000, 'cash', ?, CURRENT_DATE)`,
			[]any{tenantH, flCharge, flDeposit}},
	} {
		require.NoError(t, superDB.Exec(stmt.sql, stmt.args...).Error, stmt.sql)
	}
}

// moneyFingerprint hashes every money row of tenant H, column by column. Equal
// fingerprints mean byte-identical money.
func moneyFingerprint(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range moneyTables {
		var fp string
		require.NoError(t, superDB.Raw(
			"SELECT count(*)::text || ':' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '') FROM "+table+" x WHERE tenant_id = ?",
			tenantH).Scan(&fp).Error, table)
		out[table] = fp
	}
	return out
}

func countPlans(t *testing.T, level string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, superDB.Raw("SELECT count(*) FROM tuition_plans WHERE tenant_id = ? AND level_id = ?", tenantH, level).Scan(&n).Error)
	return n
}

func TestFinanceLevelPreviewCountsPlansAndWhatStays(t *testing.T) {
	seedFinanceLevel(t)
	money := moneyFingerprint(t)

	got, err := levelRepo(t, tenantH).PreviewLevelDeletion(context.Background(), flScope)
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"tuitionPlans": 2, "charges": 2, "payments": 1}, got)
	assert.Equal(t, money, moneyFingerprint(t))
	assert.Equal(t, int64(2), countPlans(t, flLevel), "a preview changes nothing")
}

func TestFinanceLevelArchiveRestoreKeepsMoneyIdentical(t *testing.T) {
	seedFinanceLevel(t)
	ctx := context.Background()
	repo := levelRepo(t, tenantH)
	const batch = "f5000000-0000-0000-0000-00000000f001"
	money := moneyFingerprint(t)

	counts, err := repo.ArchiveLevel(ctx, batch, flScope)
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"tuitionPlans": 2}, counts, "only plans are archived, never money")
	assert.Zero(t, countPlans(t, flLevel))
	assert.Equal(t, int64(1), countPlans(t, byLevel), "the bystander level's plan stays")
	assert.Equal(t, money, moneyFingerprint(t), "money is untouched by the archive")

	again, err := repo.ArchiveLevel(ctx, batch, flScope)
	require.NoError(t, err)
	assert.Equal(t, counts, again, "a retried step answers its first result")

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"tuitionPlans": 2}, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Equal(t, int64(2), countPlans(t, flLevel))
	assert.Equal(t, money, moneyFingerprint(t), "money is untouched by the restore")

	rep2, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, rep.Restored, rep2.Restored)
	assert.Equal(t, int64(2), countPlans(t, flLevel), "a retried restore inserts nothing twice")
}

func TestFinanceLevelRestoreSkipsATakenPlan(t *testing.T) {
	seedFinanceLevel(t)
	ctx := context.Background()
	repo := levelRepo(t, tenantH)
	const batch = "f5000000-0000-0000-0000-00000000f002"
	_, err := repo.ArchiveLevel(ctx, batch, flScope)
	require.NoError(t, err)

	// While the level sat in the trash, a new plan was typed for one cycle.
	require.NoError(t, superDB.Exec(`INSERT INTO tuition_plans (tenant_id, cycle_id, level_id, enrollment_cents, monthly_cents, months, first_billed_month, due_day)
		VALUES (?, ?, ?, 1, 1, 10, 9, 10)`, tenantH, flCycle, flLevel).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rep.Restored["tuitionPlans"])
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, "tuitionPlans", rep.Skipped[0].Kind)
	assert.Equal(t, int64(1), rep.Skipped[0].Count)
	assert.NotEmpty(t, rep.Skipped[0].Reason)
}

func TestFinanceLevelPurgeHonoursTheRetention(t *testing.T) {
	seedFinanceLevel(t)
	ctx := context.Background()
	repo := levelRepo(t, tenantH)
	const batch = "f5000000-0000-0000-0000-00000000f003"
	_, err := repo.ArchiveLevel(ctx, batch, flScope)
	require.NoError(t, err)

	purged, err := repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Zero(t, purged, "fresh trash is never purged")

	require.NoError(t, superDB.Exec("UPDATE finance_deletion_archive SET created_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	purged, err = repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Positive(t, purged)
	var left int64
	require.NoError(t, superDB.Raw("SELECT count(*) FROM finance_deletion_archive WHERE batch_id = ?", batch).Scan(&left).Error)
	assert.Zero(t, left)

	_, err = repo.RestoreLevelBatch(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "a purged batch is NotFound, never a silent success")
}

func TestFinanceLevelTrashIsTenantIsolated(t *testing.T) {
	seedFinanceLevel(t)
	ctx := context.Background()
	const batch = "f5000000-0000-0000-0000-00000000f004"
	_, err := levelRepo(t, tenantH).ArchiveLevel(ctx, batch, flScope)
	require.NoError(t, err)

	other := levelRepo(t, tenantB)
	counts, err := other.PreviewLevelDeletion(ctx, flScope)
	require.NoError(t, err)
	for kind, n := range counts {
		assert.Zero(t, n, "tenant B sees none of H's %s", kind)
	}
	_, err = other.RestoreLevelBatch(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "B cannot even see H's batch")
	assert.Zero(t, countPlans(t, flLevel))

	db, err := open(appDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	var visible int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM finance_deletion_archive").Scan(&visible).Error
	}))
	assert.Zero(t, visible, "RLS hides H's archive rows from B")
}

func archivedPlans(t *testing.T, batch string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, superDB.Raw("SELECT count(*) FROM finance_deletion_archive WHERE batch_id = ? AND table_name <> '_batch'", batch).Scan(&n).Error)
	return n
}

// A plan that did not fit stays in the trash; once the clash is gone a retry
// brings it back and the batch converges.
func TestFinanceLevelSkippedPlanStaysArchivedUntilARetryFits(t *testing.T) {
	seedFinanceLevel(t)
	ctx := context.Background()
	repo := levelRepo(t, tenantH)
	const batch = "f5000000-0000-0000-0000-00000000f005"
	_, err := repo.ArchiveLevel(ctx, batch, flScope)
	require.NoError(t, err)
	require.NoError(t, superDB.Exec(`INSERT INTO tuition_plans (tenant_id, cycle_id, level_id, enrollment_cents, monthly_cents, months, first_billed_month, due_day)
		VALUES (?, ?, ?, 1, 1, 10, 9, 10)`, tenantH, flCycle, flLevel).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, int64(1), archivedPlans(t, batch), "the skipped plan stays archived")

	require.NoError(t, superDB.Exec("DELETE FROM tuition_plans WHERE tenant_id = ? AND enrollment_cents = 1", tenantH).Error)
	again, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Empty(t, again.Skipped)
	assert.Equal(t, int64(2), again.Restored["tuitionPlans"], "the report accumulates")
	assert.Zero(t, archivedPlans(t, batch))
	assert.Equal(t, int64(2), countPlans(t, flLevel))

	third, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, again, third, "nothing left: the last report")
}

func TestFinanceLevelPurgeBatchTouchesOnlyThatBatch(t *testing.T) {
	seedFinanceLevel(t)
	ctx := context.Background()
	repo := levelRepo(t, tenantH)
	const doomed, kept = "f5000000-0000-0000-0000-00000000f006", "f5000000-0000-0000-0000-00000000f007"
	_, err := repo.ArchiveLevel(ctx, doomed, flScope)
	require.NoError(t, err)
	_, err = repo.ArchiveLevel(ctx, kept, levelarchive.Scope{LevelID: byLevel})
	require.NoError(t, err)

	n, err := repo.PurgeLevelBatch(ctx, doomed)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n, "two plans and the marker")
	var left int64
	require.NoError(t, superDB.Raw("SELECT count(*) FROM finance_deletion_archive WHERE batch_id = ?", doomed).Scan(&left).Error)
	assert.Zero(t, left)
	require.NoError(t, superDB.Raw("SELECT count(*) FROM finance_deletion_archive WHERE batch_id = ?", kept).Scan(&left).Error)
	assert.Positive(t, left)

	n, err = repo.PurgeLevelBatch(ctx, doomed)
	require.NoError(t, err)
	assert.Zero(t, n)
}
