-- One-time ownership transfer only. Runtime services do not query each other's tables.
-- Old profile creation did not collect passwords. Imported accounts remain inactive
-- with an unusable password marker until a future verified password-setup process.
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'profiles' AND column_name = 'email') THEN
  -- Abort on conflicting identities rather than overwriting accounts or profile IDs.
  IF EXISTS (
   SELECT 1 FROM public.profiles p JOIN public.users u
    ON u.id = p.user_id OR LOWER(u.email) = LOWER(BTRIM(p.email))
   WHERE p.email IS NOT NULL
     AND (u.id <> p.user_id OR LOWER(u.email) <> LOWER(BTRIM(p.email)))
  ) THEN
   RAISE EXCEPTION 'Legacy profile/account identity conflict: reconcile IDs and emails before migration';
  END IF;
  INSERT INTO public.users (id,email,password_hash,status,created_at,updated_at)
  SELECT p.user_id,LOWER(BTRIM(p.email)),'!legacy-password-not-set','inactive',p.created_at,p.updated_at
  FROM public.profiles p WHERE p.email IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM public.users u WHERE u.id = p.user_id);
 END IF;
END $$;
