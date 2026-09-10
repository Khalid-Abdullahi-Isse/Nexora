-- Preserve existing profiles; email is required for users created by the API.
ALTER TABLE profiles ADD COLUMN email VARCHAR(255);
ALTER TABLE profiles ADD CONSTRAINT profiles_email_not_blank CHECK (email IS NULL OR BTRIM(email) <> '');
CREATE UNIQUE INDEX profiles_email_unique_idx ON profiles (LOWER(email));
