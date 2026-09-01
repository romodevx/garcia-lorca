-- uniforms (M16) — el módulo deja de ser sólo de ropa.
--
-- The school sells books, and supplies, and whatever else it decides to sell.
-- The module already does the hard parts — stock that cannot go negative, an
-- append-only ledger, a counter sale that bills finance — and none of that is
-- about clothing. What tied it to uniforms was the vocabulary.
--
-- A category is enough. The stock row keeps its size_label because a book has
-- one line ("Única") the same way a jumper has six, and renaming that column
-- across a working module would be churn with no behaviour behind it.
--
-- Everything already stored is a uniform, which is the truth about it.
BEGIN;

ALTER TABLE uniform_items ADD COLUMN category text NOT NULL DEFAULT 'uniform'
    CHECK (category IN ('uniform', 'book', 'supply', 'other'));

-- The catalogue is read by category once a school sells more than one kind of
-- thing: "los libros de primaria" is the question, not "todo el inventario".
CREATE INDEX idx_uniform_items_category ON uniform_items (tenant_id, category, active);

COMMIT;
