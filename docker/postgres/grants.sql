\getenv app_user APP_DB_USER
BEGIN;
SELECT format('REVOKE ALL ON TABLE public.comments, public.goose_db_version FROM PUBLIC, %I', :'app_user') \gexec
SELECT format('REVOKE ALL ON SEQUENCE %s FROM PUBLIC, %I', oid::regclass, :'app_user')
FROM pg_class WHERE oid IN (
    pg_get_serial_sequence('public.comments', 'id')::regclass,
    pg_get_serial_sequence('public.goose_db_version', 'id')::regclass
) \gexec
SELECT format('GRANT SELECT, INSERT, DELETE ON TABLE public.comments TO %I', :'app_user') \gexec
SELECT format('GRANT USAGE ON SEQUENCE %s TO %I', pg_get_serial_sequence('public.comments', 'id')::regclass, :'app_user') \gexec
COMMIT;
