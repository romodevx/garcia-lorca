-- finance — who took the money (task 7).
--
-- A payment can now be captured with the date the family paid, which is not
-- always the day it is keyed. The school accepted that a late capture may
-- settle an inscription at the price of its date (the owner's decision), and
-- the counterweight is that every deposit says WHO keyed it: the cash-up shows
-- the cashier and the moment of capture beside the payment date.
--
--   * deposits.received_by / received_by_name: the account that took the money
--     and its name as it read then (an account renamed or removed later does
--     not rewrite the cash-up). NULL / '' on deposits from before this.
--   * payments.received_by already existed ("reserved for cashier
--     attribution") and is written from now on, for the payments a deposit
--     splits into and for a single-charge payment.
--
-- Schema only: no row is read or written, so no tenant needs binding.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE deposits ADD COLUMN received_by uuid NULL;
ALTER TABLE deposits ADD COLUMN received_by_name text NOT NULL DEFAULT '';

COMMIT;
