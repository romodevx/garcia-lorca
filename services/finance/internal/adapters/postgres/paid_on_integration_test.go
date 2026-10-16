//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T5b part 2 · ADR 0035 — deposits.paid_on / payments.paid_on (migration 0028).
//
// paid_at mixed two meanings: midnight UTC of a day picked at the counter, and
// the real instant of a live payment. A live payment at 19:00 in Morelia on
// the 15th is 01:00 UTC on the 16th, and every reader that took the UTC date
// of paid_at judged it the NEXT day. paid_on is the civil day, stored apart.

const paidOnMigration = "0028_deposit_paid_on.sql"

// The instants of the night of 15→16 March 2033 (Mexico City is UTC-6, no
// DST): a day no other test in the shared container writes money on.
var (
	march15         = time.Date(2033, time.March, 15, 0, 0, 0, 0, time.UTC)
	march16         = march15.AddDate(0, 0, 1)
	evening15Live   = time.Date(2033, time.March, 16, 1, 0, 0, 0, time.UTC)  // 19:00 on the 15th
	afterMidnight16 = time.Date(2033, time.March, 16, 6, 30, 0, 0, time.UTC) // 00:30 on the 16th
)

func dayOf(t time.Time) string { return t.Format("2006-01-02") }

// The backfill: a paid_at exactly at midnight UTC is a day picked at the
// counter and keeps its UTC date; any other paid_at is a live instant and
// takes Morelia's date. Then NOT NULL, and an index per table.
func TestPaidOnBackfillReadsBothMeaningsOfPaidAt(t *testing.T) {
	db := freshDatabase(t, "finance_paid_on_backfill")
	before, err := migrationsBefore(paidOnMigration)
	require.NoError(t, err)
	for _, f := range before {
		applySQLFile(t, db, f)
	}

	const (
		tenant  = "33333333-3333-3333-3333-333333333333"
		student = "b0000000-0000-0000-0000-000000000001"
		concept = "b1000000-0000-0000-0000-000000000001"
		charge  = "b2000000-0000-0000-0000-000000000001"
	)
	exec(t, db, `INSERT INTO charge_concepts (id, tenant_id, name, default_amount_cents) VALUES (?, ?, 'colegiatura', 100000)`, concept, tenant)
	exec(t, db, `INSERT INTO charges (id, tenant_id, student_id, cycle_id, concept_id, description, amount_cents, due_date)
	             VALUES (?, ?, ?, ?, ?, 'colegiatura', 900000, '2033-03-15')`, charge, tenant, student, cycleOne, concept)

	rows := []struct {
		name   string
		id     string
		paidAt time.Time
		want   string
	}{
		{"a counter-picked day (midnight UTC)", "b3000000-0000-0000-0000-000000000001", march15, "2033-03-15"},
		{"a live payment at 19:00 in Morelia", "b3000000-0000-0000-0000-000000000002", evening15Live, "2033-03-15"},
		{"a live payment at 00:30 in Morelia", "b3000000-0000-0000-0000-000000000003", afterMidnight16, "2033-03-16"},
	}
	for i, r := range rows {
		exec(t, db, `INSERT INTO deposits (id, tenant_id, student_id, amount_cents, method, paid_at, folio)
		             VALUES (?, ?, ?, 1000, 'cash', ?, ?)`, r.id, tenant, student, r.paidAt, fmt.Sprintf("P-2033-%06d", i+1))
		exec(t, db, `INSERT INTO payments (id, tenant_id, charge_id, amount_cents, method, paid_at)
		             VALUES (?, ?, ?, 1000, 'cash', ?)`, strings.Replace(r.id, "b3", "b4", 1), tenant, charge, r.paidAt)
	}

	applySQLFile(t, db, filepath.Join(migrationDir, paidOnMigration))

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			for table, id := range map[string]string{"deposits": r.id, "payments": strings.Replace(r.id, "b3", "b4", 1)} {
				var got string
				require.NoError(t, db.Raw(`SELECT paid_on::text FROM `+table+` WHERE id = ?`, id).Scan(&got).Error)
				assert.Equal(t, r.want, got, table)
			}
		})
	}
	for _, table := range []string{"deposits", "payments"} {
		var notNull bool
		require.NoError(t, db.Raw(`SELECT attnotnull FROM pg_attribute
		                            WHERE attrelid = ?::regclass AND attname = 'paid_on'`, table).Scan(&notNull).Error)
		assert.True(t, notNull, table+".paid_on is NOT NULL")
		var indexed bool
		require.NoError(t, db.Raw(`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = ?
		                            AND indexdef LIKE '%(tenant_id, paid_on%')`, table).Scan(&indexed).Error)
		assert.True(t, indexed, table+" has a (tenant_id, paid_on) index")
	}
}

// paidOnFixture is a family with one charge due on the 15th and a live
// deposit taken at 19:00 in Morelia on the 15th (01:00 UTC on the 16th).
type paidOnFixture struct {
	repo    interface{ ports.FinanceRepository }
	charge  domain.Charge
	deposit domain.Deposit
}

func newPaidOnFixture(t *testing.T, suffix string, amount int64, paidAt, paidOn time.Time) paidOnFixture {
	t.Helper()
	return newPaidOnFixtureDue(t, suffix, amount, march15, paidAt, paidOn)
}

func newPaidOnFixtureDue(t *testing.T, suffix string, amount int64, due, paidAt, paidOn time.Time) paidOnFixture {
	t.Helper()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000d00"+suffix
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000d00"+suffix, "colegiatura día "+suffix,
		"61000000-0000-0000-0000-0000000d00"+suffix, student, "70000000-0000-0000-0000-0000000d00"+suffix, amount, 0, due)
	repo := appRepo(t, tenant)
	d := newDeposit(t, "62000000-0000-0000-0000-0000000d00"+suffix, tenant, student, amount, paidAt, "").WithPaidOn(paidOn)
	stored, _, err := repo.RegisterDeposit(context.Background(), d, func(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
		p := newPayment(t, "63000000-0000-0000-0000-0000000d00"+suffix, tenant, charge.ID(), amount, paidAt).WithPaidOn(paidOn)
		return ports.DepositWrite{Allocations: []domain.Payment{p}}, nil
	}, nil)
	require.NoError(t, err)
	return paidOnFixture{repo: repo, charge: charge, deposit: stored.Deposit}
}

// A deposit's day survives the round trip, apart from its instant.
func TestADepositsDayIsStoredApartFromItsInstant(t *testing.T) {
	f := newPaidOnFixture(t, "01", 123457, evening15Live, march15)
	got, err := f.repo.GetDeposit(context.Background(), f.deposit.ID())
	require.NoError(t, err)
	assert.Equal(t, "2033-03-15", dayOf(got.PaidOn()))
	assert.True(t, got.PaidAt().Equal(evening15Live), "paid_at keeps the instant")
}

// paid_by_due — what decides whether a scholarship is kept for paying on time
// (T18) — reads the DAY: a live payment at 19:00 on the due day counts as on
// time. And it reads the day, not the instant: a payment dated the 16th
// whose instant happens to fall before the 16th UTC is late.
func TestPaidByDueReadsTheDayNotTheInstant(t *testing.T) {
	cases := []struct {
		name   string
		suffix string
		paidAt time.Time
		paidOn time.Time
		byDue  bool
	}{
		{"19:00 in Morelia on the due day", "02", evening15Live, march15, true},
		{"dated the day after, instant still on the due day UTC", "03", time.Date(2033, time.March, 15, 23, 0, 0, 0, time.UTC), march16, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			const amount = 98765
			f := newPaidOnFixture(t, tt.suffix, amount, tt.paidAt, tt.paidOn)
			// ListCharges: the read that carries paid_by_due (withPaidTotals).
			listed, err := f.repo.ListCharges(context.Background(), f.charge.StudentID(), "")
			require.NoError(t, err)
			require.Len(t, listed, 1)
			got := listed[0]
			want := int64(0)
			if tt.byDue {
				want = amount
			}
			assert.Equal(t, int64(amount), got.PaidCents)
			assert.Equal(t, want, got.PaidByDueCents)
		})
	}
}

// The cash cut and the income reports put the live evening deposit on the
// school's day — the 15th — and not on the 16th. A day of its own (April), so
// the tenant-wide sums count only this test's money.
func TestTheCashCutListsALiveEveningDepositOnItsDay(t *testing.T) {
	const amount = 45679
	april15 := time.Date(2033, time.April, 15, 0, 0, 0, 0, time.UTC)
	april16 := april15.AddDate(0, 0, 1)
	evening := april15.Add(25 * time.Hour) // 19:00 on the 15th in Morelia
	f := newPaidOnFixtureDue(t, "04", amount, april15, evening, april15)
	// A payment straight against the charge (no deposit), live the same evening.
	direct := newPayment(t, "63000000-0000-0000-0000-0000000d0104", tenantA, f.charge.ID(), 1, evening).WithPaidOn(april15)
	_, _, _, err := f.repo.RegisterPayment(context.Background(), one(direct), anyStatus, nil)
	require.NoError(t, err)
	ctx := context.Background()
	exp := appRepo(t, tenantA)

	on15, err := f.repo.ListDepositsOn(ctx, april15)
	require.NoError(t, err)
	assert.True(t, hasDeposit(on15, f.deposit.ID()), "the 15th lists it")
	on16, err := f.repo.ListDepositsOn(ctx, april16)
	require.NoError(t, err)
	assert.False(t, hasDeposit(on16, f.deposit.ID()), "the 16th does not")

	loose15, err := f.repo.ListLoosePaymentsOn(ctx, april15)
	require.NoError(t, err)
	assert.True(t, hasLoose(loose15, direct.ID()), "the direct payment is on the 15th")
	loose16, err := f.repo.ListLoosePaymentsOn(ctx, april16)
	require.NoError(t, err)
	assert.False(t, hasLoose(loose16, direct.ID()))

	byMethod15, _, err := exp.IncomeByMethod(ctx, april15, april15)
	require.NoError(t, err)
	assert.Equal(t, int64(amount+1), byMethod15.TotalCents, "the 15th's income by method")
	byMethod16, _, err := exp.IncomeByMethod(ctx, april16, april16)
	require.NoError(t, err)
	assert.Zero(t, byMethod16.TotalCents, "nothing on the 16th")

	lines, err := f.repo.IncomeLines(ctx, april15, april15)
	require.NoError(t, err)
	var concept int64
	for _, l := range lines {
		if l.ConceptID == f.charge.ConceptID() {
			concept += l.AmountCents
		}
	}
	assert.Equal(t, int64(amount+1), concept, "the 15th's income by concept")

	// created_at is the container's real clock, so "closed" is any instant
	// before the rows were written.
	closedAt := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	changed, err := exp.DayChangedSince(ctx, april15, closedAt)
	require.NoError(t, err)
	assert.True(t, changed, "a closed 15th sees the evening's money")
	unchanged, err := exp.DayChangedSince(ctx, april16, closedAt)
	require.NoError(t, err)
	assert.False(t, unchanged, "the 16th is untouched")
}

// Money nobody dated is refused rather than stored with a guessed day.
func TestMoneyWithoutADayIsRefused(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000d0005"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000d0005", "colegiatura sin día",
		"61000000-0000-0000-0000-0000000d0005", student, "70000000-0000-0000-0000-0000000d0005", 1000, 0, march15)
	repo := appRepo(t, tenant)
	ctx := context.Background()

	undatedDeposit := newDeposit(t, "62000000-0000-0000-0000-0000000d0005", tenant, student, 1000, evening15Live, "")
	_, _, err := repo.RegisterDeposit(ctx, undatedDeposit.WithPaidOn(time.Time{}), func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
		return ports.DepositWrite{UnallocatedCents: 1000}, nil
	}, nil)
	assert.Equal(t, glerrors.KindInternal, glerrors.KindOf(err), "a deposit")

	undatedPayment := newPayment(t, "63000000-0000-0000-0000-0000000d0005", tenant, charge.ID(), 1000, evening15Live)
	_, _, _, err = repo.RegisterPayment(ctx, one(undatedPayment.WithPaidOn(time.Time{})), anyStatus, nil)
	assert.Equal(t, glerrors.KindInternal, glerrors.KindOf(err), "a payment")
}

func hasDeposit(ds []domain.Deposit, id string) bool {
	for _, d := range ds {
		if d.ID() == id {
			return true
		}
	}
	return false
}

func hasLoose(ps []ports.LoosePayment, id string) bool {
	for _, p := range ps {
		if p.Payment.ID() == id {
			return true
		}
	}
	return false
}

// anyStatus accepts any payment and reports the charge's derived status.
func anyStatus(c domain.Charge, paid int64) (string, error) { return c.DeriveStatus(paid), nil }

// --- a database of its own, for the backfill --------------------------------

// freshDatabase creates an empty database in the shared container and drops
// it with the test: the backfill has to run on rows written BEFORE the
// migration, which the suite's database (fully migrated in TestMain) has none of.
func freshDatabase(t *testing.T, name string) *gorm.DB {
	t.Helper()
	exec(t, superDB, `DROP DATABASE IF EXISTS `+name)
	exec(t, superDB, `CREATE DATABASE `+name)
	dsn := strings.Replace(superDSN, "dbname="+dbName, "dbname="+name, 1)
	db, err := open(dsn)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		exec(t, superDB, `DROP DATABASE IF EXISTS `+name)
	})
	return db
}

func migrationsBefore(name string) ([]string, error) {
	files, err := migrationFiles()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		if filepath.Base(f) >= name {
			break
		}
		out = append(out, f)
	}
	return out, nil
}

func applySQLFile(t *testing.T, db *gorm.DB, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(sql)).Error, "apply %s", filepath.Base(path))
}

func exec(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	require.NoError(t, db.Exec(sql, args...).Error, sql)
}
