-- Crear rol de aplicación sin privilegios de RLS bypass ni superuser
CREATE ROLE apexrmm_app WITH LOGIN PASSWORD 'apexrmm_app_secret_2024' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;

-- Permisos de conexión a la base de datos
GRANT CONNECT ON DATABASE apexrmm TO apexrmm_app;
GRANT USAGE ON SCHEMA public TO apexrmm_app;

-- Permisos en las 11 tablas con tenant_id (SELECT/INSERT/UPDATE/DELETE)
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
  agents, telemetry, alerts, audit_log, backup_jobs,
  agent_software, agent_patches, agent_notes, agent_logs, agent_checks,
  registration_tokens, users
TO apexrmm_app;

-- Permisos en la tabla tenants (catálogo, lectura)
GRANT SELECT ON TABLE tenants TO apexrmm_app;

-- Permisos en secuencias (necesario para SERIAL/BIGSERIAL columns)
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO apexrmm_app;

-- Verificar
SELECT rolname, rolsuper, rolbypassrls, rolcanlogin FROM pg_roles WHERE rolname IN ('apexrmm', 'apexrmm_app');
