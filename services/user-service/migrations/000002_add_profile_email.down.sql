DROP INDEX IF EXISTS profiles_email_unique_idx;
ALTER TABLE profiles DROP CONSTRAINT IF EXISTS profiles_email_not_blank;
ALTER TABLE profiles DROP COLUMN IF EXISTS email;
