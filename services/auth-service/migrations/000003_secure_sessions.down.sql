BEGIN;
-- Session/audit removal is intentionally not automatic: retain security history.
DO $$ BEGIN RAISE EXCEPTION 'Secure session migration requires an operator-reviewed rollback; restore application compatibility without deleting audit/session history'; END $$;
COMMIT;
