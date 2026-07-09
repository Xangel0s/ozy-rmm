-- Simulate exactly what withTenant does: BeginTx, set_config, query
BEGIN;
SELECT set_config('app.tenant_id', 'd5e2fb62-2f70-4ace-bf33-89fefeb52c88', true);
SELECT id, hostname, tenant_id FROM agents;
ROLLBACK;
