# User-service ownership

This service owns social profiles and follows. Account registration has moved to auth-service at `POST /api/v1/auth/register` on port 8001. The former account-creation endpoint on port 8002 has been removed.

Only `GET /health` is currently implemented here. Profile, follow, and social-search endpoints remain future work. Profiles must reference an auth-created account ID; this service no longer generates account UUIDs or manages email, passwords, roles, permissions, sessions, or tokens.

Existing migrations and profile data are preserved. The historical profile email column remains solely to avoid deleting legacy data; application models no longer map it. Auth's ownership-transfer migration copies legacy identities using their original UUIDs, leaving profile records intact.

See [the ownership report](../../OWNERSHIP.md) for migration details, endpoint verification, and current folder trees.
