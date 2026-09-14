#!/bin/sh
set -eu
# Values enter psql as variables, then SQL literals via format(%L), never SQL concatenation.
psql -X -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
\getenv auth_password AUTH_DATABASE_PASSWORD
\getenv post_password POST_DATABASE_PASSWORD
\getenv chat_password CHAT_DATABASE_PASSWORD
\getenv notification_password NOTIFICATION_DATABASE_PASSWORD
SELECT format('CREATE ROLE app_auth LOGIN PASSWORD %L', :'auth_password') \gexec
SELECT format('CREATE ROLE app_post LOGIN PASSWORD %L', :'post_password') \gexec
SELECT format('CREATE ROLE app_chat LOGIN PASSWORD %L', :'chat_password') \gexec
SELECT format('CREATE ROLE app_notification LOGIN PASSWORD %L', :'notification_password') \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SQL
