package usecase_test

import (
	"bytes"
	"context"
	"sort"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// fakeVoucherRepo is an in-memory ports.PaymentVoucherRepository. It keeps the
// rules the real one enforces in SQL (dedupe by digest while live, the pending
// ceiling), so a use-case test cannot pass against a service that relies on a
// rule the database would not hold.
type fakeVoucherRepo struct {
	vouchers map[string]domain.PaymentVoucher
	order    []string
	events   []domain.VoucherEvent
	// filters records every ListPaymentVouchers call.
	filters []ports.VoucherFilter
	// createErr, when set, fails CreatePaymentVoucher.
	createErr error
}

func newFakeVoucherRepo() *fakeVoucherRepo {
	return &fakeVoucherRepo{vouchers: map[string]domain.PaymentVoucher{}}
}

func (f *fakeVoucherRepo) put(v domain.PaymentVoucher) {
	if _, ok := f.vouchers[v.ID()]; !ok {
		f.order = append(f.order, v.ID())
	}
	f.vouchers[v.ID()] = v
}

func (f *fakeVoucherRepo) CreatePaymentVoucher(_ context.Context, v domain.PaymentVoucher, uploaded domain.VoucherEvent) (domain.PaymentVoucher, bool, error) {
	if f.createErr != nil {
		return domain.PaymentVoucher{}, false, f.createErr
	}
	if live, found, _ := f.LivePaymentVoucherByFile(context.Background(), v.StudentID(), v.File().SHA256); found {
		return live, true, nil
	}
	pending := 0
	for _, id := range f.order {
		if w := f.vouchers[id]; w.StudentID() == v.StudentID() && w.Pending() {
			pending++
		}
	}
	if pending >= domain.MaxPendingVouchersPerStudent {
		return domain.PaymentVoucher{}, false, domain.ErrVoucherLimit
	}
	f.put(v)
	f.events = append(f.events, uploaded)
	return v, false, nil
}

func (f *fakeVoucherRepo) LivePaymentVoucherByFile(_ context.Context, studentID string, sha []byte) (domain.PaymentVoucher, bool, error) {
	for _, id := range f.order {
		v := f.vouchers[id]
		if v.StudentID() == studentID && v.Status() != domain.VoucherRejected && bytes.Equal(v.File().SHA256, sha) {
			return v, true, nil
		}
	}
	return domain.PaymentVoucher{}, false, nil
}

func (f *fakeVoucherRepo) GetPaymentVoucher(_ context.Context, id string) (domain.PaymentVoucher, error) {
	v, ok := f.vouchers[id]
	if !ok {
		return domain.PaymentVoucher{}, glerrors.ErrNotFound
	}
	return v, nil
}

func (f *fakeVoucherRepo) ListPaymentVouchers(_ context.Context, filter ports.VoucherFilter) ([]domain.PaymentVoucher, error) {
	f.filters = append(f.filters, filter)
	wanted := map[string]bool{}
	for _, id := range filter.StudentIDs {
		wanted[id] = true
	}
	var out []domain.PaymentVoucher
	for _, id := range f.order {
		v := f.vouchers[id]
		if len(wanted) > 0 && !wanted[v.StudentID()] {
			continue
		}
		if filter.Status != "" && v.Status() != filter.Status {
			continue
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt().After(out[j].CreatedAt()) })
	return out, nil
}

func (f *fakeVoucherRepo) UpdatePaymentVoucher(_ context.Context, id string,
	change func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error),
) (domain.PaymentVoucher, error) {
	cur, ok := f.vouchers[id]
	if !ok {
		return domain.PaymentVoucher{}, glerrors.ErrNotFound
	}
	next, event, err := change(cur)
	if err != nil {
		return domain.PaymentVoucher{}, err
	}
	if event.Kind == "" {
		return cur, nil
	}
	f.put(next)
	f.events = append(f.events, event)
	return next, nil
}

func (f *fakeVoucherRepo) ListPaymentVoucherEvents(_ context.Context, id string) ([]domain.VoucherEvent, error) {
	var out []domain.VoucherEvent
	for _, e := range f.events {
		if e.VoucherID == id {
			out = append(out, e)
		}
	}
	return out, nil
}
