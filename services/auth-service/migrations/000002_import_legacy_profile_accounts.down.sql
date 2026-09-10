-- Imported identities may now be referenced by sessions/roles. Automatic deletion
-- would lose data, so rollback deliberately requires an operator-reviewed plan.
DO $$ BEGIN
 RAISE EXCEPTION 'Account ownership transfer cannot be reversed automatically without risking identity data';
END $$;
