-- students — the editable half of a pupil's file (task 5).
--
-- Birth date, sex, the pupil's own email and phones, and address. Every column
-- is optional (empty/NULL): the office fills what it has. What IS filled is
-- checked twice — by the domain (students/internal/domain/profile.go), and here
-- for the shapes a raw client could still get wrong:
--   * sex is '', F, M or X;
--   * the email is stored normalized (students_normalized_email, 0006);
--   * a postal code is five digits.
-- Phones are a small JSON array of {kind, number}: at most three, validated by
-- the domain, never queried by field.
--
-- Additive: existing rows keep empty values. RLS is the table's own (0001).

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE students ADD COLUMN birth_date date NULL;
ALTER TABLE students ADD COLUMN sex text NOT NULL DEFAULT ''
    CONSTRAINT students_sex_ck CHECK (sex IN ('', 'F', 'M', 'X'));
ALTER TABLE students ADD COLUMN email text NOT NULL DEFAULT ''
    CONSTRAINT students_email_normalized_ck CHECK (email = students_normalized_email(email));
ALTER TABLE students ADD COLUMN phones jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE students ADD COLUMN address_street text NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN address_ext_number text NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN address_int_number text NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN address_neighborhood text NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN address_postal_code text NOT NULL DEFAULT ''
    CONSTRAINT students_postal_code_ck CHECK (address_postal_code = '' OR address_postal_code ~ '^[0-9]{5}$');
ALTER TABLE students ADD COLUMN address_municipality text NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN address_state text NOT NULL DEFAULT '';

COMMIT;
