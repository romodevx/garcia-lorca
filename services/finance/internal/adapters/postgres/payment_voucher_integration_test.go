//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// ADR 0040 against real SQL (migration 0032). The container is shared by the
// whole package, so every id here lives in its own prefix space:
// vouchers b7…, pupils 87…, concepts 67…, charges 671…, deposits 672…,
// allocations 97…; and the voucher days are in 2032, which no other test uses.

const cycleVoucher = "70000000-0000-0000-0000-0000000000b7"

var (
	vchDay      = time.Date(2032, 2, 10, 0, 0, 0, 0, time.UTC)
	vchGuardian = domain.Receiver{UserID: "a0000000-0000-0000-0000-0000000000b1", Name: "Rosa Pérez"}
	vchCashier  = domain.Receiver{UserID: "a0000000-0000-0000-0000-0000000000b2", Name: "Encargada"}
)

func vchID(prefix string, n int) string { return fmt.Sprintf("%s-0000-0000-0000-%012d", prefix, n) }

func vchVoucherID(n int) string { return vchID("b7000000", n) }
func vchStudent(n int) string   { return vchID("87000000", n) }

func vchDigest(seed string) []byte {
	sum := sha256.Sum256([]byte(seed))
	return sum[:]
}

func vchNew(t *testing.T, id, student, seed string, claimed int64) domain.PaymentVoucher {
	t.Helper()
	v, err := domain.NewPaymentVoucher(id, domain.VoucherDraft{
		StudentID: student, ClaimedAmountCents: claimed, ClaimedPaidOn: vchDay,
	}, domain.VoucherFile{
		Key: "vouchers/" + id + "/obj", ContentType: "application/pdf", SizeBytes: 2048, SHA256: vchDigest(seed),
	}, vchGuardian, vchDay)
	if err != nil {
		t.Fatalf("NewPaymentVoucher: %v", err)
	}
	return v
}

func vchUploaded(v domain.PaymentVoucher) domain.VoucherEvent {
	return domain.VoucherEvent{VoucherID: v.ID(), Kind: domain.VoucherEventUploaded, By: v.UploadedBy()}
}

func vchCreate(t *testing.T, repo *pgadapter.Repository, v domain.PaymentVoucher) (domain.PaymentVoucher, bool) {
	t.Helper()
	stored, replayed, err := repo.CreatePaymentVoucher(context.Background(), v, vchUploaded(v))
	if err != nil {
		t.Fatalf("CreatePaymentVoucher(%s): %v", v.ID(), err)
	}
	return stored, replayed
}

// vchDeposit stores a real deposit for a pupil (one charge, paid in full), so
// a voucher has a deposit row its foreign key accepts. n keeps the ids apart.
func vchDeposit(t *testing.T, tenant, student string, n int) domain.Deposit {
	t.Helper()
	charge := seedConceptAndCharge(t, tenant, vchID("67000000", n), fmt.Sprintf("vch concept %d", n),
		vchID("67100000", n), student, cycleVoucher, 100000, 0, vchDay)
	d := newDeposit(t, vchID("67200000", n), tenant, student, 100000, vchDay, "")
	record, _, err := appRepo(t, tenant).RegisterDeposit(context.Background(), d,
		func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Allocations: []domain.Payment{
				newPayment(t, vchID("97000000", n), tenant, charge.ID(), 100000, vchDay),
			}}, nil
		}, nil)
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	return record.Deposit
}

// change returns an UpdatePaymentVoucher callback running one transition.
func change(kind string, step func(domain.PaymentVoucher) (domain.PaymentVoucher, error)) func(domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error) {
	return func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error) {
		next, err := step(cur)
		if err != nil {
			return domain.PaymentVoucher{}, domain.VoucherEvent{}, err
		}
		// No instant: the database stamps clock_timestamp(), in order.
		return next, domain.VoucherEvent{VoucherID: cur.ID(), Kind: kind, DepositID: next.DepositID(), By: vchCashier}, nil
	}
}

// Every column survives the round trip, and every mutable column is written on
// EACH update of the same row (the defect that shipped three times in finance:
// a column the update path forgot).
func TestPaymentVoucherRoundTripAndEveryUpdate(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	student := vchStudent(1)
	first := vchDeposit(t, tenantA, student, 1)
	second := vchDeposit(t, tenantA, student, 2)

	created, replayed := vchCreate(t, repo, vchNew(t, vchVoucherID(1), student, "round-trip", 250000))
	if replayed {
		t.Fatal("a first upload reported itself as a replay")
	}
	got, err := repo.GetPaymentVoucher(ctx, created.ID())
	if err != nil {
		t.Fatalf("GetPaymentVoucher: %v", err)
	}
	amount, hasAmount := got.ClaimedAmount()
	day, hasDay := got.ClaimedPaidOn()
	if got.StudentID() != student || got.UploadedBy() != vchGuardian || !hasAmount || amount.Int64() != 250000 ||
		!hasDay || !day.Equal(vchDay) || got.Status() != domain.VoucherPending || got.CreatedAt().IsZero() ||
		got.File().Key != "vouchers/"+created.ID()+"/obj" || got.File().ContentType != "application/pdf" ||
		got.File().SizeBytes != 2048 || !bytes.Equal(got.File().SHA256, vchDigest("round-trip")) {
		t.Fatalf("round trip lost something: %+v", got)
	}

	applied, err := repo.UpdatePaymentVoucher(ctx, created.ID(), change(domain.VoucherEventApplied,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
			return v.Apply(first.ID(), vchCashier, vchDay.Add(10*time.Hour))
		}))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.DepositID() != first.ID() || applied.DepositFolio() != first.Folio() || applied.DecidedBy() != vchCashier {
		t.Fatalf("applied = %+v (folio %q, want %q)", applied, applied.DepositFolio(), first.Folio())
	}
	moved, err := repo.UpdatePaymentVoucher(ctx, created.ID(), change(domain.VoucherEventMovedByCorrection,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) { return v.MoveTo(second.ID(), vchDay) }))
	if err != nil || moved.DepositID() != second.ID() || moved.DepositFolio() != second.Folio() {
		t.Fatalf("second update (move) lost the deposit: %+v, %v", moved, err)
	}
	released, err := repo.UpdatePaymentVoucher(ctx, created.ID(), change(domain.VoucherEventReleasedByVoid,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) { return v.Release(vchDay) }))
	if err != nil || released.Status() != domain.VoucherPending || released.DepositID() != "" ||
		released.Releases() != 1 || !released.DecidedAt().IsZero() || released.DecidedBy() != (domain.Receiver{}) {
		t.Fatalf("release: %+v, %v", released, err)
	}
	rejected, err := repo.UpdatePaymentVoucher(ctx, created.ID(), change(domain.VoucherEventRejected,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
			return v.Reject("  la transferencia es de otra escuela ", vchCashier, vchDay.Add(11*time.Hour))
		}))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	reread, err := repo.GetPaymentVoucher(ctx, created.ID())
	if err != nil || reread.Status() != domain.VoucherRejected || reread.RejectionReason() != "la transferencia es de otra escuela" ||
		reread.Releases() != 1 || reread.DecidedBy() != vchCashier || !reread.DecidedAt().Equal(vchDay.Add(11*time.Hour)) {
		t.Fatalf("rejected = %+v / reread %+v, %v", rejected, reread, err)
	}

	events, err := repo.ListPaymentVoucherEvents(ctx, created.ID())
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	wantKinds := []string{domain.VoucherEventUploaded, domain.VoucherEventApplied, domain.VoucherEventMovedByCorrection,
		domain.VoucherEventReleasedByVoid, domain.VoucherEventRejected}
	if len(events) != len(wantKinds) {
		t.Fatalf("events = %+v", events)
	}
	for i, k := range wantKinds {
		if events[i].Kind != k {
			t.Fatalf("event %d = %q, want %q", i, events[i].Kind, k)
		}
	}
	if events[1].DepositID != first.ID() || events[1].By != vchCashier {
		t.Fatalf("applied event = %+v", events[1])
	}
}

// An event without a kind is "nothing changed": the row and its history stay.
func TestUpdatePaymentVoucherWithoutChange(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	v, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(2), vchStudent(2), "no-change", 0))
	got, err := repo.UpdatePaymentVoucher(ctx, v.ID(), func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error) {
		return cur, domain.VoucherEvent{}, nil
	})
	if err != nil || got.Status() != domain.VoucherPending {
		t.Fatalf("got %+v, %v", got, err)
	}
	if events, _ := repo.ListPaymentVoucherEvents(ctx, v.ID()); len(events) != 1 {
		t.Fatalf("history = %+v, want the upload alone", events)
	}
	if _, err := repo.UpdatePaymentVoucher(ctx, vchVoucherID(999), func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error) {
		return cur, domain.VoucherEvent{}, nil
	}); !isNotFound(err) {
		t.Fatalf("missing voucher: %v", err)
	}
}

// The same file for the same pupil is the same voucher — until it is rejected.
func TestPaymentVoucherDedupeByFile(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	student := vchStudent(3)
	original, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(31), student, "dedupe", 100))

	again, replayed := vchCreate(t, repo, vchNew(t, vchVoucherID(32), student, "dedupe", 100))
	if !replayed || again.ID() != original.ID() {
		t.Fatalf("same file: replayed=%v id=%s, want the original %s", replayed, again.ID(), original.ID())
	}
	live, found, err := repo.LivePaymentVoucherByFile(ctx, student, vchDigest("dedupe"))
	if err != nil || !found || live.ID() != original.ID() {
		t.Fatalf("LivePaymentVoucherByFile = %+v, %v, %v", live, found, err)
	}
	// Another pupil may send the very same file (siblings, one transfer).
	if _, replayed := vchCreate(t, repo, vchNew(t, vchVoucherID(33), vchStudent(4), "dedupe", 100)); replayed {
		t.Fatal("a sibling's voucher is not the same voucher")
	}

	if _, err := repo.UpdatePaymentVoucher(ctx, original.ID(), change(domain.VoucherEventRejected,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
			return v.Reject("ilegible", vchCashier, vchDay)
		})); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, found, _ := repo.LivePaymentVoucherByFile(ctx, student, vchDigest("dedupe")); found {
		t.Fatal("a rejected voucher is not live")
	}
	resent, replayed := vchCreate(t, repo, vchNew(t, vchVoucherID(34), student, "dedupe", 100))
	if replayed || resent.ID() != vchVoucherID(34) {
		t.Fatalf("after a rejection the family may send it again: %+v replayed=%v", resent, replayed)
	}
}

// A pupil has at most three vouchers waiting, also when they arrive at once.
func TestPaymentVoucherPendingLimit(t *testing.T) {
	ctx := context.Background()
	student := vchStudent(5)
	const attempts = 6
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
		limited int
		others  []error
	)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := appRepo(t, tenantA)
			v := vchNew(t, vchVoucherID(50+i), student, fmt.Sprintf("limit-%d", i), 0)
			_, _, err := repo.CreatePaymentVoucher(ctx, v, vchUploaded(v))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case errors.Is(err, domain.ErrVoucherLimit):
				limited++
			default:
				others = append(others, err)
			}
		}(i)
	}
	wg.Wait()
	if len(others) > 0 || created != domain.MaxPendingVouchersPerStudent || limited != attempts-domain.MaxPendingVouchersPerStudent {
		t.Fatalf("created=%d limited=%d others=%v", created, limited, others)
	}
	// The same file as one already waiting is still its replay, not a refusal.
	repo := appRepo(t, tenantA)
	list, err := repo.ListPaymentVouchers(ctx, ports.VoucherFilter{StudentIDs: []string{student}})
	if err != nil || len(list) != domain.MaxPendingVouchersPerStudent {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	seed := ""
	for i := range attempts {
		if bytes.Equal(vchDigest(fmt.Sprintf("limit-%d", i)), list[0].File().SHA256) {
			seed = fmt.Sprintf("limit-%d", i)
		}
	}
	retry := vchNew(t, vchVoucherID(59), student, seed, 0)
	_, replayed, err := repo.CreatePaymentVoucher(ctx, retry, vchUploaded(retry))
	if err != nil || !replayed {
		t.Fatalf("a retry at the limit must replay: replayed=%v err=%v", replayed, err)
	}
}

// The database itself refuses a second voucher on one deposit.
func TestPaymentVoucherOneVoucherPerDeposit(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	student := vchStudent(6)
	deposit := vchDeposit(t, tenantA, student, 6)
	a, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(61), student, "one-a", 0))
	b, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(62), student, "one-b", 0))
	apply := change(domain.VoucherEventApplied, func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
		return v.Apply(deposit.ID(), vchCashier, vchDay)
	})
	if _, err := repo.UpdatePaymentVoucher(ctx, a.ID(), apply); err != nil {
		t.Fatalf("first link: %v", err)
	}
	_, err := repo.UpdatePaymentVoucher(ctx, b.ID(), apply)
	if !errors.Is(err, domain.ErrVoucherAlreadyApplied) {
		t.Fatalf("second voucher on one deposit: want ErrVoucherAlreadyApplied, got %v", err)
	}
	if got, _ := repo.GetPaymentVoucher(ctx, b.ID()); got.Status() != domain.VoucherPending {
		t.Fatalf("the refused voucher stays pending: %+v", got)
	}
}

// The shape CHECK keeps a status and its columns together, whoever writes.
func TestPaymentVoucherShapeCheck(t *testing.T) {
	insert := func(status, deposit, reason, decidedAt string) error {
		return superDB.Exec(fmt.Sprintf(`INSERT INTO payment_vouchers
			(id, tenant_id, student_id, uploaded_by, uploaded_by_name, file_key, file_content_type, file_size_bytes,
			 file_sha256, status, deposit_id, rejection_reason, decided_at)
			VALUES (gen_random_uuid(), '%s', '%s', '%s', 'x', 'k', 'image/png', 10, sha256(('shape'||gen_random_uuid()::text)::bytea),
			        '%s', %s, %s, %s)`, tenantA, vchStudent(7), vchGuardian.UserID, status, deposit, reason, decidedAt)).Error
	}
	tests := []struct {
		name                               string
		status, deposit, reason, decidedAt string
	}{
		{"applied without a deposit", "applied", "NULL", "NULL", "now()"},
		{"rejected without a reason", "rejected", "NULL", "NULL", "now()"},
		{"pending with a reason", "pending", "NULL", "'motivo'", "NULL"},
		{"pending already decided", "pending", "NULL", "NULL", "now()"},
		{"rejected with a blank reason", "rejected", "NULL", "'  a '", "now()"},
		{"unknown status", "lost", "NULL", "NULL", "NULL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := insert(tc.status, tc.deposit, tc.reason, tc.decidedAt); err == nil {
				t.Fatal("the CHECK let it through")
			}
		})
	}
	if err := insert("pending", "NULL", "NULL", "NULL"); err != nil {
		t.Fatalf("a well-formed pending row: %v", err)
	}
}

func TestPaymentVoucherListFilters(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	s1, s2 := vchStudent(8), vchStudent(9)
	a, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(81), s1, "list-a", 0))
	b, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(82), s2, "list-b", 0))
	c, _ := vchCreate(t, repo, vchNew(t, vchVoucherID(83), s1, "list-c", 0))
	if _, err := repo.UpdatePaymentVoucher(ctx, c.ID(), change(domain.VoucherEventRejected,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
			return v.Reject("ilegible", vchCashier, vchDay)
		})); err != nil {
		t.Fatal(err)
	}
	ids := func(vs []domain.PaymentVoucher) []string {
		out := make([]string, 0, len(vs))
		for _, v := range vs {
			out = append(out, v.ID())
		}
		return out
	}
	mine, err := repo.ListPaymentVouchers(ctx, ports.VoucherFilter{StudentIDs: []string{s1}})
	if err != nil || fmt.Sprint(ids(mine)) != fmt.Sprint([]string{a.ID(), c.ID()}) {
		t.Fatalf("s1's vouchers oldest first = %v, %v", ids(mine), err)
	}
	pending, err := repo.ListPaymentVouchers(ctx, ports.VoucherFilter{Status: domain.VoucherPending, StudentIDs: []string{s1, s2}})
	if err != nil || fmt.Sprint(ids(pending)) != fmt.Sprint([]string{a.ID(), b.ID()}) {
		t.Fatalf("pending of both = %v, %v", ids(pending), err)
	}
	queue, err := repo.ListPaymentVouchers(ctx, ports.VoucherFilter{Status: domain.VoucherPending})
	if err != nil || !containsVoucher(queue, a.ID()) || !containsVoucher(queue, b.ID()) || containsVoucher(queue, c.ID()) {
		t.Fatalf("the school's queue = %v, %v", ids(queue), err)
	}
	one, err := repo.ListPaymentVouchers(ctx, ports.VoucherFilter{StudentIDs: []string{s1, s2}, Limit: 1})
	if err != nil || len(one) != 1 {
		t.Fatalf("limit 1 = %v, %v", ids(one), err)
	}
	if bad, err := repo.ListPaymentVouchers(ctx, ports.VoucherFilter{StudentIDs: []string{"not-a-uuid"}}); err != nil || len(bad) != 0 {
		t.Fatalf("a malformed pupil id matches nothing: %v, %v", ids(bad), err)
	}
}

func containsVoucher(vs []domain.PaymentVoucher, id string) bool {
	for _, v := range vs {
		if v.ID() == id {
			return true
		}
	}
	return false
}

// RLS on both tables, for the non-owner app role: tenant B neither reads nor
// writes tenant A's vouchers or their history.
func TestRLSIsolatesPaymentVouchers(t *testing.T) {
	ctx := context.Background()
	a, _ := vchCreate(t, appRepo(t, tenantA), vchNew(t, vchVoucherID(91), vchStudent(10), "rls", 0))
	other := appRepo(t, tenantB)

	if _, err := other.GetPaymentVoucher(ctx, a.ID()); !isNotFound(err) {
		t.Fatalf("tenant B read A's voucher: %v", err)
	}
	if list, err := other.ListPaymentVouchers(ctx, ports.VoucherFilter{StudentIDs: []string{vchStudent(10)}}); err != nil || len(list) != 0 {
		t.Fatalf("tenant B listed A's voucher: %d, %v", len(list), err)
	}
	if _, err := other.UpdatePaymentVoucher(ctx, a.ID(), change(domain.VoucherEventRejected,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
			return v.Reject("ajeno", vchCashier, vchDay)
		})); !isNotFound(err) {
		t.Fatalf("tenant B rejected A's voucher: %v", err)
	}
	if events, err := other.ListPaymentVoucherEvents(ctx, a.ID()); err != nil || len(events) != 0 {
		t.Fatalf("tenant B read A's history: %v, %v", events, err)
	}
	if _, found, err := other.LivePaymentVoucherByFile(ctx, vchStudent(10), vchDigest("rls")); err != nil || found {
		t.Fatalf("tenant B found A's file: %v, %v", found, err)
	}

	db, err := open(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	asB := func(stmt string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
				return err
			}
			return tx.Exec(stmt).Error
		})
	}
	// WITH CHECK: B cannot write a row stamped with A's tenant, in either table.
	if err := asB(fmt.Sprintf(`INSERT INTO payment_vouchers
		(id, tenant_id, student_id, uploaded_by, uploaded_by_name, file_key, file_content_type, file_size_bytes, file_sha256, status)
		VALUES (gen_random_uuid(), '%s', '%s', '%s', 'x', 'k', 'image/png', 10, sha256('rls-b'::bytea), 'pending')`,
		tenantA, vchStudent(10), vchGuardian.UserID)); err == nil {
		t.Fatal("tenant B inserted a voucher as tenant A")
	}
	if err := asB(fmt.Sprintf(`INSERT INTO payment_voucher_events (tenant_id, voucher_id, kind)
		VALUES ('%s', '%s', 'rejected')`, tenantA, a.ID())); err == nil {
		t.Fatal("tenant B wrote A's voucher history")
	}
	var changed int64
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		res := tx.Exec("UPDATE payment_vouchers SET uploaded_by_name = 'B' WHERE id = ?", a.ID())
		changed = res.RowsAffected
		return res.Error
	}); err != nil || changed != 0 {
		t.Fatalf("tenant B updated A's voucher: %d rows, %v", changed, err)
	}
}

// Review (LOW): the history is append-only for the app role — it reads and
// adds its own tenant's events and can neither rewrite nor erase one — and an
// event's deposit is a real deposit.
func TestPaymentVoucherHistoryIsAppendOnly(t *testing.T) {
	v, _ := vchCreate(t, appRepo(t, tenantA), vchNew(t, vchVoucherID(95), vchStudent(11), "append-only", 0))
	db, err := open(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	asA := func(stmt string, args ...any) (int64, error) {
		var n int64
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
				return err
			}
			res := tx.Exec(stmt, args...)
			n = res.RowsAffected
			return res.Error
		})
		return n, err
	}
	for _, stmt := range []string{
		"UPDATE payment_voucher_events SET reason = 'reescrito' WHERE voucher_id = ?",
		"DELETE FROM payment_voucher_events WHERE voucher_id = ?",
	} {
		if n, err := asA(stmt, v.ID()); err == nil && n != 0 {
			t.Errorf("%q changed %d rows of the history", stmt, n)
		}
	}
	if n, err := asA("INSERT INTO payment_voucher_events (tenant_id, voucher_id, kind) VALUES (?, ?, 'rejected')", tenantA, v.ID()); err != nil || n != 1 {
		t.Fatalf("the app role must still append: %d, %v", n, err)
	}
	if events, _ := appRepo(t, tenantA).ListPaymentVoucherEvents(context.Background(), v.ID()); len(events) != 2 {
		t.Fatalf("history = %d events, want 2", len(events))
	}
	if err := superDB.Exec("INSERT INTO payment_voucher_events (tenant_id, voucher_id, kind, deposit_id) VALUES (?, ?, 'applied', gen_random_uuid())",
		tenantA, v.ID()).Error; err == nil {
		t.Fatal("an event named a deposit that does not exist")
	}
}
