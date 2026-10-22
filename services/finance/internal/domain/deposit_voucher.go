package domain

// VoucherChoice is what the office said about a pupil's payment vouchers when
// registering money (ADR 0040 §4): this payment IS VoucherID (a payment by
// voucher), or, registered by hand at a counter that knows about vouchers
// (Aware), it is NOT any of Declined. A choice that is not Aware leaves the
// vouchers alone. The deposit transaction reads it under the pupil's locks;
// it is never stored.
type VoucherChoice struct {
	VoucherID string
	Declined  []string
	// Aware says the counter shows vouchers and can decline them; only then
	// does a payment by hand link one (or stop at VOUCHER_CHOICE_REQUIRED).
	Aware bool
}

// Any says whether the office said anything about vouchers.
func (c VoucherChoice) Any() bool { return c.Aware || c.VoucherID != "" || len(c.Declined) > 0 }

// WithVoucherChoice returns the deposit carrying the office's voucher choice.
func (d Deposit) WithVoucherChoice(c VoucherChoice) Deposit {
	c.Declined = append([]string(nil), c.Declined...)
	d.vouchers = c
	return d
}

// VoucherChoice is what the office said about vouchers for this deposit.
func (d Deposit) VoucherChoice() VoucherChoice {
	c := d.vouchers
	c.Declined = append([]string(nil), c.Declined...)
	return c
}
