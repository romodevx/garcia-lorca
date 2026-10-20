-- config — the CLABE families transfer to, shown in the family app's account.
--
-- Eighteen digits or empty (not captured yet). Banxico's check digit is
-- verified by the domain; the table keeps the shape.
BEGIN;

ALTER TABLE school_profile
    ADD COLUMN clabe text NOT NULL DEFAULT '' CHECK (clabe = '' OR clabe ~ '^[0-9]{18}$');

COMMIT;
