-- 006_simulate_agent_b.sql
-- Inserts a fake agent row for Tenant B to verify cross-tenant isolation
-- in the OTHER direction. Uses apexrmm (BYPASSRLS) for the insert.

INSERT INTO agents (id, tenant_id, hostname, os, cpu_model, status, last_seen)
VALUES (
  'b0000000-0000-0000-0000-000000000001',
  'd5e2fb62-2f70-4ace-bf33-89fefeb52c88',
  'B-MOCK-AGENT',
  'Microsoft Windows 11 Pro',
  'Mock CPU',
  'online',
  NOW()
)
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  hostname = EXCLUDED.hostname,
  cpu_model = EXCLUDED.cpu_model
RETURNING id, hostname, tenant_id, cpu_model;
