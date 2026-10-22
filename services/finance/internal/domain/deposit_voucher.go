package domain

// VoucherChoice is what the office said about a pupil's payment vouchers when
// registering money (ADR 0040 §4): this payment IS VoucherID (a payment by
// voucher), or, registered by hand, it is NOT any of Declined. The deposit
// transaction reads it under the pupil's locks; it is never stored.
type VoucherChoice struct {
	VoucherID string
	Declined  []string
}

// Any says whether the office said anything about vouchers.
func (c VoucherChoice) Any() bool { return c.VoucherID != "" || len(c.Declined) > 0 }

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
