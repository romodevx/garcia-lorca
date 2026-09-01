-- finance (M16) — why a discount was given.
--
-- The reason has been captured at the boundary since Phase 7 and thrown away
-- ever since: `ApplyDiscount` took it and the use case dropped it on the floor
-- with a TODO, because there was no column. That was survivable while a discount
-- was a rare, deliberate act by the director.
--
-- The early-payment discount makes it unsurvivable. The system will now reduce
-- what a family owes BY ITSELF, on a rule about dates, and a parent asking "why
-- is September $3,100 and not $3,200?" deserves an answer that does not depend
-- on somebody remembering. Every discount now says why it exists, and the
-- automatic ones say it in the same place as the manual ones.
--
-- NOT NULL DEFAULT '' so every discount already given keeps its shape. An empty
-- reason on an old row means "we did not record it", which is the truth.
BEGIN;

ALTER TABLE charges ADD COLUMN discount_reason text NOT NULL DEFAULT '';

COMMIT;
