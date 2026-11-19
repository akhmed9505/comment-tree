\getenv admin_user PGUSER
\getenv database PGDATABASE
\getenv migration_user MIGRATION_USER
\getenv migration_password MIGRATION_PASSWORD
\getenv app_user APP_DB_USER
\getenv app_password APP_DB_PASSWORD

BEGIN;
-- Reject names PostgreSQL would truncate before altering any roles.
SELECT 1 / CASE WHEN octet_length(:'migration_user') BETWEEN 1 AND 63
    AND octet_length(:'app_user') BETWEEN 1 AND 63 THEN 1 ELSE 0 END;
SELECT format('CREATE ROLE %I', name)
FROM (VALUES (:'migration_user'), (:'app_user')) roles(name)
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = name) \gexec

SELECT format('ALTER ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT PASSWORD %L',
    :'migration_user', :'migration_password') \gexec
SELECT format('ALTER ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT PASSWORD %L',
    :'app_user', :'app_password') \gexec
SELECT format('REVOKE %I FROM %I', parent.rolname, member.rolname)
FROM pg_auth_members membership
JOIN pg_roles parent ON parent.oid = membership.roleid
JOIN pg_roles member ON member.oid = membership.member
WHERE member.rolname IN (:'migration_user', :'app_user') \gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', :'database', :'admin_user') \gexec
ALTER SCHEMA public OWNER TO pg_database_owner;
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC, %I, %I', :'database', :'migration_user', :'app_user') \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO %I, %I', :'database', :'migration_user', :'app_user') \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SELECT format('REVOKE ALL ON SCHEMA public FROM %I, %I', :'migration_user', :'app_user') \gexec
SELECT format('GRANT USAGE, CREATE ON SCHEMA public TO %I', :'migration_user') \gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'app_user') \gexec

-- Transfer only project objects when upgrading an existing volume.
SELECT format('ALTER TABLE %s OWNER TO %I', oid::regclass, :'migration_user')
FROM pg_class WHERE oid IN (to_regclass('public.comments'), to_regclass('public.goose_db_version')) \gexec
SELECT format('ALTER TEXT SEARCH CONFIGURATION public.comment_search OWNER TO %I', :'migration_user')
WHERE EXISTS (SELECT FROM pg_ts_config WHERE cfgnamespace = 'public'::regnamespace AND cfgname = 'comment_search') \gexec
COMMIT;
